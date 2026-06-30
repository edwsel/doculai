package vllm

import (
	"testing"
)

func TestOllamaProvider_IsConfigured(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected bool
	}{
		{"configured", "http://localhost:11434", true},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := NewOllamaProvider(tt.url)
			if provider.IsConfigured() != tt.expected {
				t.Errorf("IsConfigured() = %v, expected %v", provider.IsConfigured(), tt.expected)
			}
		})
	}
}

func TestProviderFactory_CreateProvider(t *testing.T) {
	factory := NewProviderFactory()

	t.Run("openai", func(t *testing.T) {
		provider, err := factory.CreateProvider(Options{
			Provider: "openai",
			URL:      "https://api.openai.com/v1",
			Key:      "key",
		})
		if err != nil {
			t.Fatalf("CreateProvider() error = %v", err)
		}
		if provider == nil {
			t.Fatal("CreateProvider() returned nil")
		}
		if !provider.IsConfigured() {
			t.Error("Provider should be configured")
		}
	})

	t.Run("ollama", func(t *testing.T) {
		provider, err := factory.CreateProvider(Options{
			Provider: "ollama",
			URL:      "http://localhost:11434",
		})
		if err != nil {
			t.Fatalf("CreateProvider() error = %v", err)
		}
		if provider == nil {
			t.Fatal("CreateProvider() returned nil")
		}
		if !provider.IsConfigured() {
			t.Error("Provider should be configured")
		}
	})

	t.Run("unsupported", func(t *testing.T) {
		_, err := factory.CreateProvider(Options{
			Provider: "unsupported",
		})
		if err == nil {
			t.Error("Expected error for unsupported provider, got nil")
		}
	})
}

func TestProviderFactory_IsValidProvider(t *testing.T) {
	factory := NewProviderFactory()

	tests := []struct {
		provider string
		valid    bool
	}{
		{"openai", true},
		{"ollama", true},
		{"unsupported", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			if factory.IsValidProvider(tt.provider) != tt.valid {
				t.Errorf("IsValidProvider(%q) = %v, expected %v", tt.provider, factory.IsValidProvider(tt.provider), tt.valid)
			}
		})
	}
}

func TestHasVLLMConfig(t *testing.T) {
	tests := []struct {
		name     string
		opts     Options
		expected bool
	}{
		{
			name: "complete",
			opts: Options{
				Model:    "gpt-4o",
				URL:      "https://api.openai.com/v1",
				Provider: "openai",
			},
			expected: true,
		},
		{
			name: "missing model",
			opts: Options{
				URL:      "https://api.openai.com/v1",
				Provider: "openai",
			},
			expected: false,
		},
		{
			name: "missing url",
			opts: Options{
				Model:    "gpt-4o",
				Provider: "openai",
			},
			expected: false,
		},
		{
			name: "missing provider",
			opts: Options{
				Model: "gpt-4o",
				URL:   "https://api.openai.com/v1",
			},
			expected: false,
		},
		{
			name:     "empty",
			opts:     Options{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if HasVLLMConfig(tt.opts) != tt.expected {
				t.Errorf("HasVLLMConfig() = %v, expected %v", HasVLLMConfig(tt.opts), tt.expected)
			}
		})
	}
}
