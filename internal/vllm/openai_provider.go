package vllm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	openai "github.com/sashabaranov/go-openai"
)

// openaiProvider implements Provider using the go-openai SDK for OpenAI-compatible APIs.
type openaiProvider struct {
	client  *openai.Client
	baseURL string
}

// newOpenAIProvider creates an OpenAI-compatible provider using the go-openai SDK.
// The baseURL should be the API root (e.g. "https://api.openai.com/v1" or
// "https://routerai.ru/api/v1"); the SDK appends "/chat/completions" automatically.
func newOpenAIProvider(baseURL, apiKey string, httpClient *http.Client) *openaiProvider {
	config := openai.DefaultConfig(apiKey)
	config.BaseURL = baseURL
	if httpClient != nil {
		config.HTTPClient = httpClient
	}
	return &openaiProvider{
		client:  openai.NewClientWithConfig(config),
		baseURL: baseURL,
	}
}

// ExtractMarkdown sends an image to the OpenAI-compatible API and returns
// the extracted markdown text.
func (p *openaiProvider) ExtractMarkdown(ctx context.Context, imageBase64 string, opts Options) (string, error) {
	systemPrompt := GetSystemPrompt(opts)

	req := openai.ChatCompletionRequest{
		Model: opts.Model,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: systemPrompt,
			},
			{
				Role: openai.ChatMessageRoleUser,
				MultiContent: []openai.ChatMessagePart{
					{
						Type: openai.ChatMessagePartTypeText,
						Text: "Extract all text from this image and format it as markdown.",
					},
					{
						Type: openai.ChatMessagePartTypeImageURL,
						ImageURL: &openai.ChatMessageImageURL{
							URL: imageBase64,
						},
					},
				},
			},
		},
		MaxTokens: 4096,
	}

	if opts.Reasoning {
		req.ReasoningEffort = "medium"
	} else {
		req.ReasoningEffort = "none"
	}

	logger := opts.logger()

	// Estimate payload size from the marshaled request (the base64 image dominates).
	var payloadBytes int
	if b, err := json.Marshal(req); err == nil {
		payloadBytes = len(b)
	}
	logger.Debug("vllm request", "provider", "openai", "model", opts.Model, "url", p.baseURL, "payload_bytes", payloadBytes)

	start := time.Now()
	resp, err := p.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return "", fmt.Errorf("openai chat completion: %w", err)
	}

	logger.Debug("vllm response", "provider", "openai", "status", http.StatusOK, "duration", time.Since(start))

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	// TRACE: log a truncated preview of the response content.
	if logger.Enabled(ctx, LevelTrace) {
		preview := resp.Choices[0].Message.Content
		if len(preview) > 512 {
			preview = preview[:512]
		}
		logger.Log(ctx, LevelTrace, "vllm response body", "preview", preview, "chars", len(resp.Choices[0].Message.Content))
	}

	return resp.Choices[0].Message.Content, nil
}

// IsConfigured checks if the provider has a valid configuration.
func (p *openaiProvider) IsConfigured() bool {
	return p.client != nil
}
