package pdf

import (
	"strings"
	"testing"
)

func TestExtractor_ExtractText(t *testing.T) {
	extractor := NewExtractor()

	t.Run("invalid_pdf", func(t *testing.T) {
		invalidPDF := strings.NewReader("not a pdf")
		_, err := extractor.ExtractText(invalidPDF)
		if err == nil {
			t.Error("Expected error for invalid PDF, got nil")
		}
	})

	t.Run("empty_reader", func(t *testing.T) {
		empty := strings.NewReader("")
		_, err := extractor.ExtractText(empty)
		if err == nil {
			t.Error("Expected error for empty reader, got nil")
		}
	})
}

func TestExtractor_ExtractTextByRow(t *testing.T) {
	extractor := NewExtractor()

	t.Run("invalid_pdf", func(t *testing.T) {
		invalidPDF := strings.NewReader("not a pdf")
		_, err := extractor.ExtractTextByRow(invalidPDF)
		if err == nil {
			t.Error("Expected error for invalid PDF, got nil")
		}
	})
}

func TestExtractor_ExtractImages(t *testing.T) {
	extractor := NewExtractor()

	t.Run("invalid_pdf", func(t *testing.T) {
		invalidPDF := strings.NewReader("not a pdf")
		_, err := extractor.ExtractImages(invalidPDF)
		if err == nil {
			t.Error("Expected error for invalid PDF, got nil")
		}
	})

	t.Run("empty_reader", func(t *testing.T) {
		empty := strings.NewReader("")
		_, err := extractor.ExtractImages(empty)
		if err == nil {
			t.Error("Expected error for empty reader, got nil")
		}
	})
}

func TestExtractor_GetPageCount(t *testing.T) {
	extractor := NewExtractor()

	t.Run("invalid_pdf", func(t *testing.T) {
		invalidPDF := strings.NewReader("not a pdf")
		_, err := extractor.GetPageCount(invalidPDF)
		if err == nil {
			t.Error("Expected error for invalid PDF, got nil")
		}
	})

	t.Run("empty_reader", func(t *testing.T) {
		empty := strings.NewReader("")
		_, err := extractor.GetPageCount(empty)
		if err == nil {
			t.Error("Expected error for empty reader, got nil")
		}
	})
}

func TestExtractor_GetPageDimensions(t *testing.T) {
	extractor := NewExtractor()

	t.Run("invalid_pdf", func(t *testing.T) {
		invalidPDF := strings.NewReader("not a pdf")
		_, err := extractor.GetPageDimensions(invalidPDF)
		if err == nil {
			t.Error("Expected error for invalid PDF, got nil")
		}
	})
}

func Test_isBoldFont(t *testing.T) {
	tests := []struct {
		fontName string
		expected bool
	}{
		{"Arial-Bold", true},
		{"Times-Bold", true},
		{"Helvetica-Heavy", true},
		{"Courier-Black", true},
		{"Georgia-Medium", true},
		{"Verdana-Demi", true},
		{"Arial-Extra", true},
		{"Arial", false},
		{"Times-Roman", false},
		{"Helvetica", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.fontName, func(t *testing.T) {
			result := isBoldFont(tt.fontName)
			if result != tt.expected {
				t.Errorf("isBoldFont(%q) = %v, want %v", tt.fontName, result, tt.expected)
			}
		})
	}
}

func Test_detectImageFormat(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected string
	}{
		{
			name:     "JPEG",
			data:     []byte{0xFF, 0xD8, 0xFF},
			expected: "jpeg",
		},
		{
			name:     "PNG",
			data:     []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A},
			expected: "png",
		},
		{
			name:     "GIF87a",
			data:     []byte("GIF87a"),
			expected: "gif",
		},
		{
			name:     "GIF89a",
			data:     []byte("GIF89a"),
			expected: "gif",
		},
		{
			name:     "BMP",
			data:     []byte{'B', 'M'},
			expected: "bmp",
		},
		{
			name:     "empty",
			data:     []byte{},
			expected: "unknown",
		},
		{
			name:     "unknown",
			data:     []byte{0x00, 0x01, 0x02},
			expected: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectImageFormat(tt.data)
			if result != tt.expected {
				t.Errorf("detectImageFormat() = %q, want %q", result, tt.expected)
			}
		})
	}
}
