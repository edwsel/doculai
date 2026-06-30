package vllm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAIProvider_ExtractMarkdown(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Verify it hits /chat/completions
			if r.URL.Path != "/chat/completions" {
				t.Errorf("Path = %q, expected /chat/completions", r.URL.Path)
			}
			if r.Header.Get("Authorization") != "Bearer test-key" {
				t.Errorf("Authorization = %q, expected Bearer test-key", r.Header.Get("Authorization"))
			}

			var req map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("Failed to decode: %v", err)
			}
			model, _ := req["model"].(string)
			if model != "gpt-4o" {
				t.Errorf("model = %v, expected gpt-4o", req["model"])
			}

			resp := map[string]interface{}{
				"choices": []map[string]interface{}{
					{
						"message": map[string]string{
							"content": "# Extracted Text\n\nThis is markdown.",
						},
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		provider := newOpenAIProvider(server.URL, "test-key", nil)
		result, err := provider.ExtractMarkdown(context.Background(), "data:image/jpeg;base64,test", Options{
			Model: "gpt-4o",
		})
		if err != nil {
			t.Fatalf("ExtractMarkdown() error = %v", err)
		}
		expected := "# Extracted Text\n\nThis is markdown."
		if result != expected {
			t.Errorf("ExtractMarkdown() = %q, expected %q", result, expected)
		}
	})

	t.Run("api error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]string{
					"message": "Invalid API key",
					"type":    "authentication_error",
				},
			})
		}))
		defer server.Close()

		provider := newOpenAIProvider(server.URL, "invalid-key", nil)
		_, err := provider.ExtractMarkdown(context.Background(), "data", Options{})
		if err == nil {
			t.Error("Expected error for API error, got nil")
		}
	})

	t.Run("no choices", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := map[string]interface{}{
				"choices": []interface{}{},
			}
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		provider := newOpenAIProvider(server.URL, "test-key", nil)
		_, err := provider.ExtractMarkdown(context.Background(), "data", Options{})
		if err == nil {
			t.Error("Expected error for no choices, got nil")
		}
	})

	t.Run("network error", func(t *testing.T) {
		provider := newOpenAIProvider("http://invalid-url-test", "key", &http.Client{
			Timeout: 0, // No timeout — will fail on invalid URL
		})
		// Use context with timeout to avoid hanging
		ctx, cancel := context.WithTimeout(context.Background(), 0)
		defer cancel()
		_, err := provider.ExtractMarkdown(ctx, "data", Options{})
		if err == nil {
			t.Error("Expected error for network error, got nil")
		}
	})
}

func TestOpenAIProvider_IsConfigured(t *testing.T) {
	provider := newOpenAIProvider("https://api.openai.com/v1", "key", nil)
	if !provider.IsConfigured() {
		t.Error("IsConfigured() = false, expected true")
	}
}

func TestOllamaProvider_ExtractMarkdown(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("Failed to decode: %v", err)
			}
			model, _ := req["model"].(string)
			if model != "llava" {
				t.Errorf("model = %v, expected llava", req["model"])
			}
			stream, _ := req["stream"].(bool)
			if stream {
				t.Error("stream should be false")
			}

			resp := map[string]interface{}{
				"message": map[string]string{
					"content": "# Extracted Text\n\nFrom Ollama.",
				},
				"done": true,
			}
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		provider := NewOllamaProvider(server.URL)
		result, err := provider.ExtractMarkdown(context.Background(), "base64data", Options{Model: "llava"})
		if err != nil {
			t.Fatalf("ExtractMarkdown() error = %v", err)
		}
		if result != "# Extracted Text\n\nFrom Ollama." {
			t.Errorf("ExtractMarkdown() = %q, expected markdown", result)
		}
	})

	t.Run("api error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("internal error"))
		}))
		defer server.Close()

		provider := NewOllamaProvider(server.URL)
		_, err := provider.ExtractMarkdown(context.Background(), "data", Options{Model: "llava"})
		if err == nil {
			t.Error("Expected error for API error, got nil")
		}
	})
}

func TestExtractMarkdown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]string{
						"content": "Page content",
					},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	result, err := ExtractMarkdown(context.Background(), "image1", Options{
		Model:    "gpt-4o",
		URL:      server.URL,
		Key:      "test-key",
		Provider: "openai",
	})
	if err != nil {
		t.Fatalf("ExtractMarkdown() error = %v", err)
	}
	if result != "Page content" {
		t.Errorf("ExtractMarkdown() = %q, expected Page content", result)
	}
}
