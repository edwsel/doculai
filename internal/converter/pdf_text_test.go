package converter

import (
	"strings"
	"testing"
)

func TestPDFTextConverter_Convert(t *testing.T) {
	converter := NewPDFTextConverter()

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

func TestPDFTextConverter_Supports(t *testing.T) {
	converter := NewPDFTextConverter()

	if !converter.Supports("application/pdf") {
		t.Error("Supports(application/pdf) = false, want true")
	}

	if converter.Supports("text/html") {
		t.Error("Supports(text/html) = true, want false")
	}
}
