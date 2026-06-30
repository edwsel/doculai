package vllm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ollamaRequest is the request body for the Ollama chat API.
type ollamaRequest struct {
	Model    string                 `json:"model"`
	Messages []ollamaRequestMessage `json:"messages"`
	Stream   bool                   `json:"stream"`
}

type ollamaRequestMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

// ollamaResponse is the response from the Ollama chat API.
type ollamaResponse struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
	Done bool `json:"done"`
}

// ExtractMarkdown sends an image to the Ollama API via direct HTTP and returns
// the extracted markdown text.
func (p *OllamaProvider) ExtractMarkdown(ctx context.Context, imageBase64 string, opts Options) (string, error) {
	systemPrompt := GetSystemPrompt(opts)

	reqBody := ollamaRequest{
		Model:  opts.Model,
		Stream: false,
		Messages: []ollamaRequestMessage{
			{
				Role:    "system",
				Content: systemPrompt,
			},
			{
				Role: "user",
				Content: []map[string]interface{}{
					{
						"type": "text",
						"text": "Extract all text from this image and format it as markdown.",
					},
					{
						"type":      "image_url",
						"image_url": map[string]string{"url": imageBase64},
					},
				},
			},
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshaling ollama request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.url, bytes.NewReader(jsonData))
	if err != nil {
		return "", fmt.Errorf("creating ollama request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	logger := opts.logger()
	logger.Debug("vllm request", "provider", "ollama", "model", opts.Model, "url", p.url, "payload_bytes", len(jsonData))

	start := time.Now()
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("sending ollama request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading ollama response: %w", err)
	}

	logger.Debug("vllm response", "provider", "ollama", "status", resp.StatusCode, "duration", time.Since(start))

	if logger.Enabled(ctx, LevelTrace) {
		preview := string(body)
		if len(preview) > 512 {
			preview = preview[:512]
		}
		logger.Log(ctx, LevelTrace, "vllm response body", "preview", preview)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ollama error: status %d, body: %s", resp.StatusCode, string(body))
	}

	var ollamaResp ollamaResponse
	if err := json.Unmarshal(body, &ollamaResp); err != nil {
		return "", fmt.Errorf("unmarshaling ollama response: %w", err)
	}

	return ollamaResp.Message.Content, nil
}

var _ io.Reader = bytes.NewReader(nil)
