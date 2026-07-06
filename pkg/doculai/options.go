package doculai

import (
	"bytes"
	"io"
	"log/slog"

	"github.com/edwsel/doculai/internal/converter"
)

// Options is the public, import-safe configuration for document conversion.
// It mirrors internal converter.Options without exposing the internal package,
// so external modules can convert content without a replace directive.
type Options struct {
	// VLLMModel is the VLLM model name (optional).
	VLLMModel string
	// VLLMURL is the VLLM provider endpoint URL (optional).
	VLLMURL string
	// VLLMKey is the VLLM API key (optional).
	VLLMKey string
	// VLLMProvider is the VLLM provider type ("openai" or "ollama").
	VLLMProvider string
	// VLLMPrompt overrides the base system prompt (optional).
	VLLMPrompt string
	// VLLMReasoning enables reasoning for models that support it.
	VLLMReasoning bool
	// VLLMConcurrency bounds parallel OCR requests (<=0 → default).
	VLLMConcurrency int
	// Logger is the structured logger. Nil falls back to the instance logger.
	Logger *slog.Logger
}

// ErrUnsupportedFormat is returned when no converter supports the input format.
var ErrUnsupportedFormat = converter.ErrUnsupportedFormat

// toConverter builds an internal converter.Options from the public Options.
func (o Options) toConverter() converter.Options {
	return converter.Options{
		VLLMModel:       o.VLLMModel,
		VLLMURL:         o.VLLMURL,
		VLLMKey:         o.VLLMKey,
		VLLMProvider:    o.VLLMProvider,
		VLLMPrompt:      o.VLLMPrompt,
		VLLMReasoning:   o.VLLMReasoning,
		VLLMConcurrency: o.VLLMConcurrency,
		Logger:          o.Logger,
	}
}

// ConvertBytes converts raw bytes to Markdown, auto-detecting the MIME type.
// opts is applied with instance-level defaults for any unset field.
func (d *Doculai) ConvertBytes(data []byte, opts Options) (string, error) {
	return d.Convert(bytes.NewReader(data), opts.toConverter())
}

// ConvertBytesWithType converts raw bytes using an explicit MIME type.
// opts is applied with instance-level defaults for any unset field.
func (d *Doculai) ConvertBytesWithType(data []byte, mimeType string, opts Options) (string, error) {
	return d.ConvertWithType(io.NopCloser(bytes.NewReader(data)), mimeType, opts.toConverter())
}
