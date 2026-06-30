package image

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

func createTestImage(width, height int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			img.Set(x, y, color.RGBA{uint8(x % 256), uint8(y % 256), 128, 255})
		}
	}
	return img
}

func TestNormalizer_NormalizeImage(t *testing.T) {
	normalizer := NewNormalizer()

	t.Run("small image no resize", func(t *testing.T) {
		img := createTestImage(100, 100)
		var buf bytes.Buffer
		err := png.Encode(&buf, img)
		if err != nil {
			t.Fatalf("Failed to encode test image: %v", err)
		}

		result, err := normalizer.NormalizeImage(bytes.NewReader(buf.Bytes()))
		if err != nil {
			t.Fatalf("NormalizeImage() error = %v", err)
		}

		if len(result) == 0 {
			t.Error("NormalizeImage() returned empty data")
		}
	})

	t.Run("large image resize", func(t *testing.T) {
		img := createTestImage(2000, 1500)
		var buf bytes.Buffer
		err := png.Encode(&buf, img)
		if err != nil {
			t.Fatalf("Failed to encode test image: %v", err)
		}

		result, err := normalizer.NormalizeImage(bytes.NewReader(buf.Bytes()))
		if err != nil {
			t.Fatalf("NormalizeImage() error = %v", err)
		}

		if len(result) == 0 {
			t.Error("NormalizeImage() returned empty data")
		}

		// Check dimensions
		w, h, err := normalizer.GetImageDimensions(bytes.NewReader(result))
		if err != nil {
			t.Fatalf("GetImageDimensions() error = %v", err)
		}

		if w > 1024 || h > 1024 {
			t.Errorf("Image dimensions %dx%d exceed max 1024x1024", w, h)
		}
	})

	t.Run("invalid image", func(t *testing.T) {
		_, err := normalizer.NormalizeImage(strings.NewReader("not an image"))
		if err == nil {
			t.Error("Expected error for invalid image, got nil")
		}
	})
}

func TestNormalizer_GetImageDimensions(t *testing.T) {
	normalizer := NewNormalizer()

	t.Run("valid image", func(t *testing.T) {
		img := createTestImage(100, 200)
		var buf bytes.Buffer
		err := png.Encode(&buf, img)
		if err != nil {
			t.Fatalf("Failed to encode test image: %v", err)
		}

		w, h, err := normalizer.GetImageDimensions(bytes.NewReader(buf.Bytes()))
		if err != nil {
			t.Fatalf("GetImageDimensions() error = %v", err)
		}

		if w != 100 || h != 200 {
			t.Errorf("GetImageDimensions() = %dx%d, expected 100x200", w, h)
		}
	})

	t.Run("invalid image", func(t *testing.T) {
		_, _, err := normalizer.GetImageDimensions(strings.NewReader("not an image"))
		if err == nil {
			t.Error("Expected error for invalid image, got nil")
		}
	})
}

func TestFormatter_FormatImage(t *testing.T) {
	formatter := NewFormatter()

	t.Run("valid image", func(t *testing.T) {
		img := createTestImage(100, 100)
		var buf bytes.Buffer
		err := png.Encode(&buf, img)
		if err != nil {
			t.Fatalf("Failed to encode test image: %v", err)
		}

		result, err := formatter.FormatImage(bytes.NewReader(buf.Bytes()), 1)
		if err != nil {
			t.Fatalf("FormatImage() error = %v", err)
		}

		if result.MIMEType != "image/jpeg" {
			t.Errorf("MIMEType = %q, expected image/jpeg", result.MIMEType)
		}

		if result.PageNum != 1 {
			t.Errorf("PageNum = %d, expected 1", result.PageNum)
		}

		if result.Base64 == "" {
			t.Error("Base64 is empty")
		}

		if result.Width != 100 || result.Height != 100 {
			t.Errorf("Dimensions = %dx%d, expected 100x100", result.Width, result.Height)
		}
	})

	t.Run("invalid image", func(t *testing.T) {
		_, err := formatter.FormatImage(strings.NewReader("not an image"), 1)
		if err == nil {
			t.Error("Expected error for invalid image, got nil")
		}
	})
}

func TestFormatter_ToBase64URL(t *testing.T) {
	formatter := NewFormatter()

	img := &FormattedImage{
		MIMEType: "image/jpeg",
		Base64:   "dGVzdA==",
	}

	result := formatter.ToBase64URL(img)
	expected := "data:image/jpeg;base64,dGVzdA=="
	if result != expected {
		t.Errorf("ToBase64URL() = %q, expected %q", result, expected)
	}
}

func TestDetectMIMEType(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected string
	}{
		{"JPEG", []byte{0xFF, 0xD8, 0xFF}, "image/jpeg"},
		{"PNG", []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, "image/png"},
		{"GIF87a", []byte("GIF87a"), "image/gif"},
		{"BMP", []byte{'B', 'M'}, "image/bmp"},
		{"empty", []byte{}, "application/octet-stream"},
		{"unknown", []byte{0x00, 0x01}, "application/octet-stream"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectMIMEType(tt.data)
			if result != tt.expected {
				t.Errorf("DetectMIMEType() = %q, expected %q", result, tt.expected)
			}
		})
	}
}

func TestDetectMIMETypeFromFilename(t *testing.T) {
	tests := []struct {
		filename string
		expected string
	}{
		{"image.jpg", "image/jpeg"},
		{"image.jpeg", "image/jpeg"},
		{"image.png", "image/png"},
		{"image.gif", "image/gif"},
		{"image.bmp", "image/bmp"},
		{"image.unknown", "application/octet-stream"},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			result := DetectMIMETypeFromFilename(tt.filename)
			if result != tt.expected {
				t.Errorf("DetectMIMETypeFromFilename(%q) = %q, expected %q", tt.filename, result, tt.expected)
			}
		})
	}
}
