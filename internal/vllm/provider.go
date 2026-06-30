package vllm

import (
	"context"
	"fmt"
	"io"
	"log/slog"
)

// LevelTrace is a custom verbosity level below LevelDebug, used for fine-grained
// diagnostic dumps (e.g. truncated HTTP response bodies).
const LevelTrace = slog.LevelDebug - 8

// discardLogger is the silent default logger used when none is provided.
var discardLogger = slog.New(slog.NewTextHandler(io.Discard, nil))

// Provider defines the interface for VLLM providers.
type Provider interface {
	// ExtractMarkdown sends an image to the provider and returns extracted markdown.
	ExtractMarkdown(ctx context.Context, imageBase64 string, opts Options) (string, error)
	// IsConfigured checks if the provider is properly configured.
	IsConfigured() bool
}

// Options holds configuration for VLLM processing.
type Options struct {
	Model        string // Model name (e.g., "gpt-4o", "llava")
	URL          string // API endpoint base URL (e.g., "https://api.openai.com/v1")
	Key          string // API key
	Provider     string // Provider type: "openai" or "ollama"
	SystemPrompt string // Custom system prompt (optional, overrides default)
	Reasoning    bool   // Enable reasoning for reasoning-capable models (disabled by default)
	Concurrency  int    // Max parallel page OCR requests; <=0 means use the caller default
	Logger       *slog.Logger
}

// logger returns the configured logger, or a silent discard logger if none is set.
func (o Options) logger() *slog.Logger {
	if o.Logger != nil {
		return o.Logger
	}
	return discardLogger
}

// OllamaProvider implements the Ollama API provider using direct HTTP calls.
type OllamaProvider struct {
	url string
}

// NewOllamaProvider creates a new Ollama provider.
func NewOllamaProvider(url string) *OllamaProvider {
	return &OllamaProvider{
		url: url,
	}
}

// IsConfigured checks if the provider is configured.
func (p *OllamaProvider) IsConfigured() bool {
	return p.url != ""
}

// ProviderFactory creates providers based on configuration.
type ProviderFactory struct{}

// NewProviderFactory creates a new provider factory.
func NewProviderFactory() *ProviderFactory {
	return &ProviderFactory{}
}

// CreateProvider creates a provider based on the configuration.
func (f *ProviderFactory) CreateProvider(opts Options) (Provider, error) {
	switch opts.Provider {
	case "openai":
		return newOpenAIProvider(opts.URL, opts.Key, nil), nil
	case "ollama":
		return NewOllamaProvider(opts.URL), nil
	default:
		return nil, fmt.Errorf("unsupported provider: %s", opts.Provider)
	}
}

// IsValidProvider checks if the provider type is valid.
func (f *ProviderFactory) IsValidProvider(provider string) bool {
	switch provider {
	case "openai", "ollama":
		return true
	default:
		return false
	}
}

// HasVLLMConfig checks if VLLM configuration is complete.
func HasVLLMConfig(opts Options) bool {
	return opts.Model != "" && opts.URL != "" && opts.Provider != ""
}
