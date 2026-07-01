package mock

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
)

// MockVLLMServer creates a mock VLLM server for testing.
func MockVLLMServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid request"})
			return
		}

		w.Header().Set("Content-Type", "application/json")

		// Determine provider type from request structure
		if _, ok := req["stream"]; ok {
			// Ollama request
			resp := map[string]interface{}{
				"message": map[string]string{
					"content": "# Mock Response\n\nThis is a mock VLLM response from Ollama.",
				},
				"done": true,
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

		// OpenAI request
		resp := map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]string{
						"content": "# Mock Response\n\nThis is a mock VLLM response from OpenAI.",
					},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

// MockVLLMErrorServer creates a mock VLLM server that returns errors.
func MockVLLMErrorServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]string{
				"message": "Internal server error",
				"type":    "server_error",
			},
		})
	}))
}

// MockVLLMRateLimitServer creates a mock VLLM server that simulates rate limiting.
func MockVLLMRateLimitServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]string{
				"message": "Rate limit exceeded",
				"type":    "rate_limit_error",
			},
		})
	}))
}
