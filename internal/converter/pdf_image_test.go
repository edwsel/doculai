package converter

import (
	"strings"
	"testing"

	"doculai/internal/pdf"
)

func TestPDFImageConverter_Convert(t *testing.T) {
	converter := NewPDFImageConverter()

	t.Run("invalid_pdf", func(t *testing.T) {
		invalidPDF := strings.NewReader("not a pdf")
		_, err := converter.Convert(invalidPDF, Options{})
		if err == nil {
			t.Error("Expected error for invalid PDF, got nil")
		}
	})

	t.Run("empty_reader", func(t *testing.T) {
		empty := strings.NewReader("")
		_, err := converter.Convert(empty, Options{})
		if err == nil {
			t.Error("Expected error for empty reader, got nil")
		}
	})
}

func TestPDFImageConverter_Supports(t *testing.T) {
	converter := NewPDFImageConverter()

	if !converter.Supports("application/pdf") {
		t.Error("Supports(application/pdf) = false, want true")
	}

	if converter.Supports("text/html") {
		t.Error("Supports(text/html) = true, want false")
	}
}

func TestPDFImageConverter_hasVLLMConfig(t *testing.T) {
	tests := []struct {
		name     string
		opts     Options
		expected bool
	}{
		{
			name: "complete config",
			opts: Options{
				VLLMModel:    "gpt-4o",
				VLLMURL:      "https://api.openai.com/v1",
				VLLMProvider: "openai",
			},
			expected: true,
		},
		{
			name: "missing model",
			opts: Options{
				VLLMURL:      "https://api.openai.com/v1",
				VLLMProvider: "openai",
			},
			expected: false,
		},
		{
			name: "missing url",
			opts: Options{
				VLLMModel:    "gpt-4o",
				VLLMProvider: "openai",
			},
			expected: false,
		},
		{
			name: "missing provider",
			opts: Options{
				VLLMModel: "gpt-4o",
				VLLMURL:   "https://api.openai.com/v1",
			},
			expected: false,
		},
		{
			name:     "empty config",
			opts:     Options{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasVLLMConfig(tt.opts)
			if result != tt.expected {
				t.Errorf("hasVLLMConfig() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestPDFImageConverter_formatWithoutOCR(t *testing.T) {
	dimensions := []pdf.PageDimension{
		{PageNum: 1, Width: 612, Height: 792},
		{PageNum: 2, Width: 612, Height: 792},
	}

	result := formatWithoutOCR(2, dimensions)

	// Check that result contains expected elements
	expectedElements := []string{
		"# PDF Document (Image-based)",
		"**Pages:** 2",
		"Page 1: 612 x 792",
		"Page 2: 612 x 792",
		"[Image: page 1]",
		"[Image: page 2]",
		"VLLM OCR",
	}

	for _, expected := range expectedElements {
		if !strings.Contains(result, expected) {
			t.Errorf("formatWithoutOCR() missing expected element: %q", expected)
		}
	}
}
