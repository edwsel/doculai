package converter

import (
	"bytes"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOptions_logger_NilReturnsDiscard(t *testing.T) {
	l := Options{}.logger()
	if l == nil {
		t.Fatal("logger() returned nil; expected a non-nil discard logger")
	}
	// Must be safe to call without panicking and silently drop output.
	l.Info("should be discarded")
}

func TestOptions_logger_NilIsStable(t *testing.T) {
	// Every call on a nil-configured Options must return a usable logger.
	for i := 0; i < 3; i++ {
		Options{}.logger().Debug("stable")
	}
}

func TestOptions_logger_SetReturnsSameInstance(t *testing.T) {
	custom := slog.New(slog.NewTextHandler(io.Discard, nil))
	got := Options{Logger: custom}.logger()
	if got != custom {
		t.Fatal("logger() did not return the configured logger instance")
	}
}

func TestPDFImageConverter_LogsWarnWithoutVLLM(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "test", "fixtures", "pdf-text", "sample.pdf"))
	if err != nil {
		t.Skipf("fixture not found, skipping: %v", err)
	}

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	conv := NewPDFImageConverter()
	_, err = conv.Convert(bytes.NewReader(data), Options{Logger: logger})
	if err != nil {
		t.Fatalf("Convert() error = %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "level=WARN") || !strings.Contains(out, "vllm not configured") {
		t.Errorf("expected WARN log about missing VLLM config, got:\n%s", out)
	}
}
