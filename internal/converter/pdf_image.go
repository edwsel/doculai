package converter

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"golang.org/x/sync/errgroup"

	"doculai/internal/image"
	"doculai/internal/pdf"
	"doculai/internal/vllm"
)

// ocrMaxRetries caps the number of retries after the initial attempt for a
// single page (i.e. up to ocrMaxRetries+1 total attempts per page).
const ocrMaxRetries = 4

// ocrMaxElapsedTime bounds the total time spent retrying a single page.
const ocrMaxElapsedTime = 30 * time.Second

// extractFunc abstracts the per-image VLLM OCR call so the parallel
// orchestration can be unit-tested without real PDF fixtures or a live
// provider. It mirrors vllm.ExtractMarkdown.
type extractFunc func(ctx context.Context, imageBase64 string, opts vllm.Options) (string, error)

// backoffFactory builds a fresh, context-bound backoff policy for a single
// page. Each goroutine needs its own instance because backoff state is not
// safe for concurrent use. Exposed as a parameter for deterministic testing.
type backoffFactory func(ctx context.Context) backoff.BackOff

// defaultBackoffFactory is the production retry policy: exponential backoff
// capped at ocrMaxRetries retries and ocrMaxElapsedTime, stopped early when
// the context (e.g. fail-fast from errgroup) is canceled.
func defaultBackoffFactory() backoffFactory {
	return func(ctx context.Context) backoff.BackOff {
		eb := backoff.NewExponentialBackOff()
		eb.MaxElapsedTime = ocrMaxElapsedTime
		return backoff.WithContext(backoff.WithMaxRetries(eb, ocrMaxRetries), ctx)
	}
}

// PDFImageConverter converts PDF with images (scanned documents) to Markdown via VLLM.
type PDFImageConverter struct {
	extractor *pdf.Extractor
	formatter *image.Formatter
}

// NewPDFImageConverter creates a new PDF image converter.
func NewPDFImageConverter() *PDFImageConverter {
	return &PDFImageConverter{
		extractor: pdf.NewExtractor(),
		formatter: image.NewFormatter(),
	}
}

// Convert converts PDF images to Markdown.
func (c *PDFImageConverter) Convert(input io.Reader, opts Options) (string, error) {
	logger := opts.logger()

	// Read input into buffer to allow multiple reads
	data, err := io.ReadAll(input)
	if err != nil {
		return "", fmt.Errorf("reading input: %w", err)
	}

	// Get page count and dimensions for metadata
	pageCount, err := c.extractor.GetPageCount(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("getting page count: %w", err)
	}

	pageDimensions, err := c.extractor.GetPageDimensions(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("getting page dimensions: %w", err)
	}

	logger.Info("pdf meta", "pages", pageCount, "dims", len(pageDimensions))

	// Check if VLLM is configured
	if !hasVLLMConfig(opts) {
		// Return metadata without OCR
		logger.Warn("vllm not configured, returning metadata only")
		return formatWithoutOCR(pageCount, pageDimensions), nil
	}

	// Render each page as an image (for scanned documents)
	images, err := c.extractor.ExtractImagesAsPages(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("extracting images: %w", err)
	}

	for _, img := range images {
		logger.Debug("rendered page", "page", img.PageNum, "dpi", 200, "bytes", len(img.Data))
	}

	// If no images found, return metadata
	if len(images) == 0 {
		logger.Warn("no images rendered, returning metadata only")
		return formatWithoutOCR(pageCount, pageDimensions), nil
	}

	// Normalize and format images for VLLM
	var formattedImages []*image.FormattedImage
	for i, img := range images {
		if len(img.Data) == 0 {
			continue
		}
		formatted, err := c.formatter.FormatImage(bytes.NewReader(img.Data), img.PageNum)
		if err != nil {
			return "", fmt.Errorf("formatting image %d: %w", i, err)
		}
		logger.Debug("normalized image", "page", formatted.PageNum, "width", formatted.Width, "height", formatted.Height)
		formattedImages = append(formattedImages, formatted)
	}

	// If no valid images after formatting, return metadata
	if len(formattedImages) == 0 {
		logger.Warn("no valid images after formatting, returning metadata only")
		return formatWithoutOCR(pageCount, pageDimensions), nil
	}

	// Build VLLM options
	vllmOpts := vllm.Options{
		Model:        opts.VLLMModel,
		URL:          opts.VLLMURL,
		Key:          opts.VLLMKey,
		Provider:     opts.VLLMProvider,
		SystemPrompt: opts.VLLMPrompt,
		Reasoning:    opts.VLLMReasoning,
		Concurrency:  opts.VLLMConcurrency,
		Logger:       opts.logger(),
	}

	// Run per-page OCR in parallel (bounded concurrency, ordered results,
	// per-page retry with exponential backoff, fail-fast on first final error).
	ctx := context.Background()
	results, err := c.ocrParallel(ctx, formattedImages, vllmOpts, logger, vllm.ExtractMarkdown, defaultBackoffFactory())
	if err != nil {
		return "", err
	}

	// Combine results with page separators (preserves page order).
	return strings.Join(results, "\n\n---\n\n"), nil
}

// ocrParallel runs VLLM OCR over every formatted image with bounded
// concurrency. Results are written into a pre-sized slice indexed by page
// position so the returned slice is always in page order regardless of
// completion order. The first page whose retry policy is exhausted cancels all
// in-flight page requests via the derived context (fail-fast) and its error is
// returned, wrapped.
func (c *PDFImageConverter) ocrParallel(
	ctx context.Context,
	images []*image.FormattedImage,
	vllmOpts vllm.Options,
	logger *slog.Logger,
	extract extractFunc,
	newBackOff backoffFactory,
) ([]string, error) {
	concurrency := vllmOpts.Concurrency
	if concurrency <= 0 {
		concurrency = DefaultVLLMConcurrency
	}

	logger.Info("ocr parallel", "pages", len(images), "concurrency", concurrency)

	results := make([]string, len(images))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(concurrency)

	for i, img := range images {
		base64URL := c.formatter.ToBase64URL(img)

		g.Go(func() error {
			// Don't even start if a sibling already failed (fail-fast).
			if gctx.Err() != nil {
				return gctx.Err()
			}

			b := newBackOff(gctx)
			notify := func(err error, next time.Duration) {
				logger.Warn("ocr retry", "page", img.PageNum, "next_backoff", next, "err", err)
			}

			var markdown string
			err := backoff.RetryNotify(func() error {
				if gctx.Err() != nil {
					return gctx.Err()
				}
				md, err := extract(gctx, base64URL, vllmOpts)
				if err != nil {
					return err
				}
				markdown = md
				return nil
			}, b, notify)

			if err != nil {
				// If the parent context was canceled (fail-fast from a sibling
				// failure), this page did not exhaust its own retries; log at
				// debug and surface the context error without a noisy warning.
				if gctx.Err() != nil {
					logger.Debug("ocr canceled", "page", img.PageNum)
					return err
				}
				return fmt.Errorf("extracting markdown from page %d: %w", img.PageNum, err)
			}

			results[i] = markdown
			logger.Info("ocr done", "page", img.PageNum, "chars", len(markdown))
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, fmt.Errorf("ocr failed: %w", err)
	}
	return results, nil
}

// Supports returns true for PDF MIME types.
func (c *PDFImageConverter) Supports(mimeType string) bool {
	return mimeType == "application/pdf"
}

// hasVLLMConfig checks if VLLM configuration is provided.
func hasVLLMConfig(opts Options) bool {
	return opts.VLLMModel != "" &&
		opts.VLLMURL != "" &&
		opts.VLLMProvider != ""
}

// formatWithoutOCR returns metadata when VLLM is not configured.
func formatWithoutOCR(pageCount int, dimensions []pdf.PageDimension) string {
	var sb strings.Builder

	sb.WriteString("# PDF Document (Image-based)\n\n")
	sb.WriteString(fmt.Sprintf("**Pages:** %d\n\n", pageCount))

	if len(dimensions) > 0 {
		sb.WriteString("## Page Dimensions\n\n")
		for _, dim := range dimensions {
			sb.WriteString(fmt.Sprintf("- Page %d: %.0f x %.0f points\n", dim.PageNum, dim.Width, dim.Height))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## Content\n\n")
	for i := 1; i <= pageCount; i++ {
		sb.WriteString(fmt.Sprintf("[Image: page %d]\n\n", i))
	}

	sb.WriteString("\n---\n\n")
	sb.WriteString("*Note: This PDF appears to be image-based. To extract text, configure VLLM OCR with:\n")
	sb.WriteString("  --vllm-model <model> --vllm-url <url> --vllm-key <key> --vllm-provider <openai|ollama>*")

	return sb.String()
}
