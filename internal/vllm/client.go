// Package vllm implements an OpenAI-compatible and Ollama client for sending
// images to vision language models for OCR.
package vllm

import "context"

// ExtractMarkdown is a convenience function that creates a provider from the
// given options, sends the image for OCR, and returns the markdown text.
// This is the primary entry point used by converters.
func ExtractMarkdown(ctx context.Context, imageBase64 string, opts Options) (string, error) {
	factory := NewProviderFactory()
	provider, err := factory.CreateProvider(opts)
	if err != nil {
		return "", err
	}
	return provider.ExtractMarkdown(ctx, imageBase64, opts)
}
