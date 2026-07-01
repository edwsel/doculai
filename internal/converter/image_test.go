package converter

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"github.com/edwsel/doculai/test/mock"
)

// newPNG encodes a small deterministic RGBA image as PNG for use as a
// converter fixture.
func newPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			img.Set(x, y, color.RGBA{uint8(x % 256), uint8(y % 256), 128, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func TestImageConverter_Supports(t *testing.T) {
	c := NewImageConverter()

	for _, mt := range []string{"image/png", "image/jpeg", "image/gif", "image/webp", "image/bmp"} {
		if !c.Supports(mt) {
			t.Errorf("Supports(%q) = false, want true", mt)
		}
	}

	// image/x-icon is excluded (no ICO decoder registered); non-image types
	// are never supported.
	for _, mt := range []string{"text/html", "application/pdf", "image/x-icon", "image/tiff", ""} {
		if c.Supports(mt) {
			t.Errorf("Supports(%q) = true, want false", mt)
		}
	}
}

func TestImageConverter_Convert_NoVLLM(t *testing.T) {
	c := NewImageConverter()
	_, err := c.Convert(bytes.NewReader(newPNG(t, 8, 8)), Options{})
	if err == nil {
		t.Fatal("expected error without VLLM config, got nil")
	}
	if !strings.Contains(err.Error(), "requires VLLM") {
		t.Errorf("error should mention required VLLM config, got: %v", err)
	}
}

func TestImageConverter_Convert_EmptyInput(t *testing.T) {
	c := NewImageConverter()
	_, err := c.Convert(bytes.NewReader(nil), Options{
		VLLMModel: "gpt-4o", VLLMURL: "https://example.com", VLLMProvider: "openai",
	})
	if err == nil {
		t.Fatal("expected error for empty input, got nil")
	}
	if !strings.Contains(err.Error(), "empty input") {
		t.Errorf("error should mention empty input, got: %v", err)
	}
}

func TestImageConverter_Convert_WithMockVLLM(t *testing.T) {
	server := mock.MockVLLMServer()
	defer server.Close()

	c := NewImageConverter()
	out, err := c.Convert(bytes.NewReader(newPNG(t, 16, 16)), Options{
		VLLMModel:    "gpt-4o",
		VLLMURL:      server.URL,
		VLLMKey:      "test-key",
		VLLMProvider: "openai",
	})
	if err != nil {
		t.Fatalf("Convert() error = %v", err)
	}
	if !strings.Contains(out, "Mock Response") {
		t.Errorf("expected mock markdown passthrough, got %q", out)
	}
}

func TestImageConverter_Convert_InvalidImage(t *testing.T) {
	server := mock.MockVLLMServer()
	defer server.Close()

	c := NewImageConverter()
	_, err := c.Convert(strings.NewReader("not an image"), Options{
		VLLMModel: "gpt-4o", VLLMURL: server.URL, VLLMKey: "k", VLLMProvider: "openai",
	})
	if err == nil {
		t.Fatal("expected error for undecodable image, got nil")
	}
	if !strings.Contains(err.Error(), "formatting image") {
		t.Errorf("error should be wrapped from formatting, got: %v", err)
	}
}

func TestDetectMimeType(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected string
	}{
		{"html", []byte("<html><body>Test</body></html>"), "text/html"},
		{"pdf", []byte("%PDF-1.4\n1 0 obj"), "application/pdf"},
		{"png", []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, "image/png"},
		{"jpeg", []byte{0xFF, 0xD8, 0xFF, 0xE0}, "image/jpeg"},
		{"gif", []byte("GIF89a..."), "image/gif"},
		{"bmp", []byte{'B', 'M', 0, 0}, "image/bmp"},
		{"empty", []byte{}, "application/octet-stream"},
		{"unknown text", []byte("Some random text"), "application/octet-stream"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DetectMimeType(tt.data); got != tt.expected {
				t.Errorf("DetectMimeType() = %q, want %q", got, tt.expected)
			}
		})
	}
}
