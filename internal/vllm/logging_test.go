package vllm

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestOpenAIProvider_Logging verifies the injected logger reaches the provider
// and that request/response events are emitted at DEBUG.
func TestOpenAIProvider_Logging(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]string{"content": "hello"}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	t.Run("debug events", func(t *testing.T) {
		var buf bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

		_, err := ExtractMarkdown(context.Background(), "data:image/png;base64,AAAA", Options{
			Model: "gpt-4o", URL: server.URL, Key: "k", Provider: "openai", Logger: logger,
		})
		if err != nil {
			t.Fatalf("ExtractMarkdown() error = %v", err)
		}

		out := buf.String()
		for _, want := range []string{"vllm request", "vllm response", "provider=openai", "model=gpt-4o"} {
			if !strings.Contains(out, want) {
				t.Errorf("expected log to contain %q, got:\n%s", want, out)
			}
		}
	})

	t.Run("trace body only at trace level", func(t *testing.T) {
		// At INFO the trace body dump must be suppressed.
		var infoBuf bytes.Buffer
		infoLogger := slog.New(slog.NewTextHandler(&infoBuf, &slog.HandlerOptions{Level: slog.LevelInfo}))
		_, _ = ExtractMarkdown(context.Background(), "data:image/png;base64,AAAA", Options{
			Model: "gpt-4o", URL: server.URL, Key: "k", Provider: "openai", Logger: infoLogger,
		})
		if strings.Contains(infoBuf.String(), "vllm response body") {
			t.Errorf("trace body should not appear at INFO level, got:\n%s", infoBuf.String())
		}

		// At TRACE the dump is emitted.
		var traceBuf bytes.Buffer
		traceLogger := slog.New(slog.NewTextHandler(&traceBuf, &slog.HandlerOptions{Level: LevelTrace}))
		_, _ = ExtractMarkdown(context.Background(), "data:image/png;base64,AAAA", Options{
			Model: "gpt-4o", URL: server.URL, Key: "k", Provider: "openai", Logger: traceLogger,
		})
		if !strings.Contains(traceBuf.String(), "vllm response body") {
			t.Errorf("expected trace body dump at TRACE level, got:\n%s", traceBuf.String())
		}
	})

	t.Run("silent without logger", func(t *testing.T) {
		// Default (discard) logger must not panic and must succeed.
		_, err := ExtractMarkdown(context.Background(), "data:image/png;base64,AAAA", Options{
			Model: "gpt-4o", URL: server.URL, Key: "k", Provider: "openai",
		})
		if err != nil {
			t.Fatalf("ExtractMarkdown() error = %v", err)
		}
	})
}
