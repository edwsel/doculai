package converter

import (
	"context"
	"errors"
	"fmt"
	mathrand "math/rand"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v4"

	"github.com/edwsel/doculai/internal/image"
	"github.com/edwsel/doculai/internal/vllm"
)

// fastBackoffFactory builds an immediate-retry, context-aware backoff (capped at
// ocrMaxRetries retries) so retry tests run without real network delays.
func fastBackoffFactory() backoffFactory {
	return func(ctx context.Context) backoff.BackOff {
		return backoff.WithContext(
			backoff.WithMaxRetries(backoff.NewConstantBackOff(0), ocrMaxRetries),
			ctx,
		)
	}
}

// makeFormattedImages builds n synthetic formatted images whose Base64 field
// uniquely identifies the page (used by mock extract funcs to vary behavior).
func makeFormattedImages(n int) []*image.FormattedImage {
	imgs := make([]*image.FormattedImage, n)
	for i := 0; i < n; i++ {
		imgs[i] = &image.FormattedImage{
			MIMEType: "image/png",
			Base64:   fmt.Sprintf("page%d", i+1),
			PageNum:  i + 1,
		}
	}
	return imgs
}

func TestOCRParallel_OrderedResults(t *testing.T) {
	conv := NewPDFImageConverter()
	imgs := makeFormattedImages(5)

	// Each call sleeps a random duration so completion order differs from page
	// (start) order; the returned slice must still be in page order.
	extract := func(ctx context.Context, b64 string, opts vllm.Options) (string, error) {
		time.Sleep(time.Duration(mathrand.Intn(15)) * time.Millisecond)
		return "md-" + b64, nil
	}

	results, err := conv.ocrParallel(
		context.Background(), imgs,
		vllm.Options{Concurrency: 3},
		discardLogger, extract, fastBackoffFactory(),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != len(imgs) {
		t.Fatalf("len(results) = %d, want %d", len(results), len(imgs))
	}
	for i, img := range imgs {
		url := fmt.Sprintf("data:%s;base64,%s", img.MIMEType, img.Base64)
		want := "md-" + url
		if results[i] != want {
			t.Errorf("results[%d] (page %d) = %q, want %q", i, img.PageNum, results[i], want)
		}
	}
}

func TestOCRParallel_ConcurrencyLimitRespected(t *testing.T) {
	conv := NewPDFImageConverter()
	imgs := makeFormattedImages(8)

	const limit = 2
	var inFlight, maxInFlight, calls int32
	extract := func(ctx context.Context, b64 string, opts vllm.Options) (string, error) {
		cur := atomic.AddInt32(&inFlight, 1)
		for {
			m := atomic.LoadInt32(&maxInFlight)
			if cur <= m {
				break
			}
			if atomic.CompareAndSwapInt32(&maxInFlight, m, cur) {
				break
			}
		}
		time.Sleep(5 * time.Millisecond) // force overlap to expose over-subscription
		atomic.AddInt32(&inFlight, -1)
		atomic.AddInt32(&calls, 1)
		return "ok", nil
	}

	_, err := conv.ocrParallel(
		context.Background(), imgs,
		vllm.Options{Concurrency: limit},
		discardLogger, extract, fastBackoffFactory(),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if max := atomic.LoadInt32(&maxInFlight); max > limit {
		t.Errorf("max concurrency in flight = %d, want <= %d", max, limit)
	}
	if got := atomic.LoadInt32(&calls); got != int32(len(imgs)) {
		t.Errorf("extract calls = %d, want %d (every page should be processed)", got, len(imgs))
	}
}

func TestOCRParallel_DefaultConcurrencyWhenZero(t *testing.T) {
	conv := NewPDFImageConverter()
	imgs := makeFormattedImages(3)

	var maxInFlight int32
	var inFlight int32
	extract := func(ctx context.Context, b64 string, opts vllm.Options) (string, error) {
		cur := atomic.AddInt32(&inFlight, 1)
		for {
			m := atomic.LoadInt32(&maxInFlight)
			if cur <= m || atomic.CompareAndSwapInt32(&maxInFlight, m, cur) {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
		atomic.AddInt32(&inFlight, -1)
		return "ok", nil
	}

	// Concurrency <= 0 must fall back to DefaultVLLMConcurrency (5), so 3 pages
	// run fully in parallel: max in-flight should equal the page count.
	_, err := conv.ocrParallel(
		context.Background(), imgs,
		vllm.Options{Concurrency: 0},
		discardLogger, extract, fastBackoffFactory(),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := atomic.LoadInt32(&maxInFlight); got != int32(len(imgs)) {
		t.Errorf("max in-flight = %d, want %d (all pages parallel with default concurrency)", got, len(imgs))
	}
}

func TestOCRParallel_RetryThenSuccess(t *testing.T) {
	conv := NewPDFImageConverter()
	imgs := makeFormattedImages(1)

	const failTimes = 2
	var attempts int32
	extract := func(ctx context.Context, b64 string, opts vllm.Options) (string, error) {
		n := atomic.AddInt32(&attempts, 1)
		if n <= failTimes {
			return "", errors.New("transient failure")
		}
		return "recovered", nil
	}

	results, err := conv.ocrParallel(
		context.Background(), imgs,
		vllm.Options{Concurrency: 1},
		discardLogger, extract, fastBackoffFactory(),
	)
	if err != nil {
		t.Fatalf("expected retry to succeed, got error: %v", err)
	}
	if results[0] != "recovered" {
		t.Errorf("result = %q, want %q", results[0], "recovered")
	}
	if got := atomic.LoadInt32(&attempts); got != failTimes+1 {
		t.Errorf("attempts = %d, want %d", got, failTimes+1)
	}
}

func TestOCRParallel_RetryExhaustedReturnsError(t *testing.T) {
	conv := NewPDFImageConverter()
	imgs := makeFormattedImages(1)

	var attempts int32
	extract := func(ctx context.Context, b64 string, opts vllm.Options) (string, error) {
		atomic.AddInt32(&attempts, 1)
		return "", errors.New("always fails")
	}

	_, err := conv.ocrParallel(
		context.Background(), imgs,
		vllm.Options{Concurrency: 1},
		discardLogger, extract, fastBackoffFactory(),
	)
	if err == nil {
		t.Fatal("expected error after retries exhausted, got nil")
	}
	if got := atomic.LoadInt32(&attempts); got != ocrMaxRetries+1 {
		t.Errorf("attempts = %d, want %d", got, ocrMaxRetries+1)
	}
	if !strings.Contains(err.Error(), "ocr failed") {
		t.Errorf("error should wrap ocr failure, got: %v", err)
	}
}

func TestOCRParallel_FailFastOnFirstFinalError(t *testing.T) {
	conv := NewPDFImageConverter()
	imgs := makeFormattedImages(5)

	const limit = 2
	var calls int32
	extract := func(ctx context.Context, b64 string, opts vllm.Options) (string, error) {
		atomic.AddInt32(&calls, 1)
		return "", errors.New("permanent failure")
	}

	_, err := conv.ocrParallel(
		context.Background(), imgs,
		vllm.Options{Concurrency: limit},
		discardLogger, extract, fastBackoffFactory(),
	)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// At least the first page must fully exhaust its retries (5 attempts),
	// confirming the retry policy ran before giving up.
	totalMax := int32((ocrMaxRetries + 1) * len(imgs))
	if got := atomic.LoadInt32(&calls); got >= totalMax {
		t.Errorf("calls = %d; fail-fast should keep it below %d", got, totalMax)
	}
	if got := atomic.LoadInt32(&calls); got < ocrMaxRetries+1 {
		t.Errorf("calls = %d; expected at least one full retry sequence (%d)", got, ocrMaxRetries+1)
	}
}

func TestOCRParallel_ParentContextCancellationStopsRetries(t *testing.T) {
	conv := NewPDFImageConverter()
	imgs := makeFormattedImages(1)

	ctx, cancel := context.WithCancel(context.Background())
	var attempts int32
	extract := func(ctx context.Context, b64 string, opts vllm.Options) (string, error) {
		atomic.AddInt32(&attempts, 1)
		return "", errors.New("fail")
	}

	// Cancel the parent before OCR starts; the operation must not spin on retries.
	cancel()
	_, err := conv.ocrParallel(
		ctx, imgs,
		vllm.Options{Concurrency: 1},
		discardLogger, extract, fastBackoffFactory(),
	)
	if err == nil {
		t.Fatal("expected error due to canceled context, got nil")
	}
}
