package integration

import (
	"strings"
	"testing"

	"github.com/edwsel/doculai/internal/converter"
	"github.com/edwsel/doculai/pkg/doculai"
)

func TestDoculai_ConvertHTML(t *testing.T) {
	d := doculai.New()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple heading",
			input:    "<h1>Hello World</h1>",
			expected: "# Hello World",
		},
		{
			name:     "paragraph",
			input:    "<p>This is a test.</p>",
			expected: "This is a test.",
		},
		{
			name:     "bold text",
			input:    "<html><body><strong>Bold</strong></body></html>",
			expected: "**Bold**",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := d.Convert(strings.NewReader(tt.input), converter.Options{})
			if err != nil {
				t.Fatalf("Convert() error = %v", err)
			}

			result = strings.TrimSpace(result)
			if !strings.Contains(result, tt.expected) {
				t.Errorf("Convert() = %q, expected to contain %q", result, tt.expected)
			}
		})
	}
}

func TestDoculai_ConvertWithType(t *testing.T) {
	d := doculai.New()

	t.Run("html explicit type", func(t *testing.T) {
		input := "<h1>Test</h1>"
		result, err := d.ConvertWithType(strings.NewReader(input), "text/html", converter.Options{})
		if err != nil {
			t.Fatalf("ConvertWithType() error = %v", err)
		}
		if !strings.Contains(result, "# Test") {
			t.Errorf("ConvertWithType() = %q, expected to contain # Test", result)
		}
	})

	t.Run("unsupported type", func(t *testing.T) {
		_, err := d.ConvertWithType(strings.NewReader("data"), "application/unknown", converter.Options{})
		if err == nil {
			t.Error("Expected error for unsupported type, got nil")
		}
	})
}

func TestDoculai_ConvertPDF(t *testing.T) {
	d := doculai.New()

	t.Run("invalid pdf", func(t *testing.T) {
		_, err := d.ConvertWithType(strings.NewReader("not a pdf"), "application/pdf", converter.Options{})
		if err == nil {
			t.Error("Expected error for invalid PDF, got nil")
		}
	})
}

func TestDoculai_DetectMimeType(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"<html><body>Test</body></html>", "text/html"},
		{"<!DOCTYPE html><html></html>", "text/html"},
		{"%PDF-1.4\n1 0 obj", "application/pdf"},
		{"Some random text", "application/octet-stream"},
		{"", "application/octet-stream"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if tt.expected == "application/octet-stream" {
				d := doculai.New()
				_, err := d.Convert(strings.NewReader(tt.input), converter.Options{})
				if err == nil {
					t.Error("Expected error for unsupported format")
				}
			}
		})
	}
}
