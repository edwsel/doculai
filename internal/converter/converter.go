package converter

import (
	"errors"
	"io"
	"log/slog"
)

// discardLogger is the silent default logger used when none is provided.
var discardLogger = slog.New(slog.NewTextHandler(io.Discard, nil))

// ErrUnsupportedFormat is returned when no converter supports the given format.
var ErrUnsupportedFormat = errors.New("unsupported format")

// DefaultVLLMConcurrency is the number of page OCR requests run in parallel
// when Options.VLLMConcurrency is not set (<=0).
const DefaultVLLMConcurrency = 5

// Converter defines the interface for converting various input formats to Markdown.
type Converter interface {
	Convert(input io.Reader, opts Options) (string, error)
	Supports(mimeType string) bool
}

// Options holds configuration for the conversion process.
type Options struct {
	VLLMModel        string // Модель VLLM (опционально)
	VLLMURL          string // URL VLLM провайдера (опционально)
	VLLMKey          string // API ключ (опционально)
	VLLMProvider     string // "openai" или "ollama"
	VLLMPrompt       string // Пользовательский системный промпт (опционально, переопределяет базовый)
	VLLMReasoning    bool   // Включить reasoning для моделей с поддержкой (по умолчанию отключён)
	VLLMConcurrency  int    // Параллельные запросы OCR (<=0 → дефолт, см. DefaultVLLMConcurrency)
	Logger           *slog.Logger
}

// logger returns the configured logger, or a silent discard logger if none is set.
// This avoids nil-check boilerplate at every log site inside converters.
func (o Options) logger() *slog.Logger {
	if o.Logger != nil {
		return o.Logger
	}
	return discardLogger
}

// Factory creates appropriate converters based on input type.
type Factory struct {
	converters []Converter
}

// NewFactory creates a new converter factory with all available converters.
func NewFactory() *Factory {
	return &Factory{
		converters: []Converter{},
	}
}

// Register adds a converter to the factory.
func (f *Factory) Register(c Converter) {
	f.converters = append(f.converters, c)
}

// GetConverter returns a converter that supports the given MIME type.
func (f *Factory) GetConverter(mimeType string) (Converter, bool) {
	for _, c := range f.converters {
		if c.Supports(mimeType) {
			return c, true
		}
	}
	return nil, false
}

// DetectMimeType attempts to detect the MIME type from the input data.
func DetectMimeType(data []byte) string {
	if len(data) == 0 {
		return "application/octet-stream"
	}

	// HTML detection
	if isHTML(data) {
		return "text/html"
	}

	// PDF detection (PDF files start with %PDF)
	if len(data) >= 4 && string(data[:4]) == "%PDF" {
		return "application/pdf"
	}

	return "application/octet-stream"
}

func isHTML(data []byte) bool {
	// Simple HTML detection by looking for common HTML tags
	htmlSignatures := [][]byte{
		[]byte("<!DOCTYPE html"),
		[]byte("<!DOCTYPE HTML"),
		[]byte("<html"),
		[]byte("<HTML"),
		[]byte("<!doctype html"),
		[]byte("<!DOCTYPE HTML"),
		[]byte("<h1"),
		[]byte("<H1"),
		[]byte("<p"),
		[]byte("<P"),
		[]byte("<div"),
		[]byte("<DIV"),
		[]byte("<span"),
		[]byte("<SPAN"),
	}

	for _, sig := range htmlSignatures {
		if len(data) >= len(sig) {
			// Case-insensitive comparison for first len(sig) bytes
			match := true
			for i := 0; i < len(sig); i++ {
				if data[i] != sig[i] {
					match = false
					break
				}
			}
			if match {
				return true
			}
		}
	}
	return false
}
