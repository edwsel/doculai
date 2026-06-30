package image

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"path/filepath"
	"strings"
)

// FormattedImage represents an image prepared for VLLM processing.
type FormattedImage struct {
	Data     []byte
	MIMEType string
	Base64   string
	Width    int
	Height   int
	PageNum  int
}

// Formatter prepares images for VLLM API consumption.
type Formatter struct {
	normalizer *Normalizer
}

// NewFormatter creates a new image formatter.
func NewFormatter() *Formatter {
	return &Formatter{
		normalizer: NewNormalizer(),
	}
}

// NewFormatterWithNormalizer creates a formatter with a custom normalizer.
func NewFormatterWithNormalizer(normalizer *Normalizer) *Formatter {
	return &Formatter{
		normalizer: normalizer,
	}
}

// FormatImage prepares a single image for VLLM: normalizes and converts to base64.
func (f *Formatter) FormatImage(input io.Reader, pageNum int) (*FormattedImage, error) {
	// Normalize image
	data, err := f.normalizer.NormalizeImage(input)
	if err != nil {
		return nil, fmt.Errorf("normalizing image: %w", err)
	}

	// Get dimensions
	width, height, err := f.normalizer.GetImageDimensions(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("getting dimensions: %w", err)
	}

	// Convert to base64
	base64Str := base64.StdEncoding.EncodeToString(data)

	return &FormattedImage{
		Data:     data,
		MIMEType: "image/jpeg",
		Base64:   base64Str,
		Width:    width,
		Height:   height,
		PageNum:  pageNum,
	}, nil
}

// FormatImages prepares multiple images for VLLM.
func (f *Formatter) FormatImages(images []ImageInput) ([]*FormattedImage, error) {
	var result []*FormattedImage
	for i, img := range images {
		formatted, err := f.FormatImage(img.Reader, img.PageNum)
		if err != nil {
			return nil, fmt.Errorf("formatting image %d: %w", i, err)
		}
		result = append(result, formatted)
	}
	return result, nil
}

// ToBase64URL creates a data URL from a formatted image.
func (f *Formatter) ToBase64URL(img *FormattedImage) string {
	return fmt.Sprintf("data:%s;base64,%s", img.MIMEType, img.Base64)
}

// ToOpenAIMessage creates an OpenAI-compatible image message content.
func (f *Formatter) ToOpenAIMessage(img *FormattedImage) map[string]interface{} {
	return map[string]interface{}{
		"type": "image_url",
		"image_url": map[string]string{
			"url": f.ToBase64URL(img),
		},
	}
}

// ToOllamaMessage creates an Ollama-compatible image message content.
func (f *Formatter) ToOllamaMessage(img *FormattedImage) []string {
	return []string{img.Base64}
}

// ImageInput represents an image input with metadata.
type ImageInput struct {
	Reader  io.Reader
	PageNum int
	Format  string
}

// DetectMIMEType detects the MIME type from image data.
func DetectMIMEType(data []byte) string {
	if len(data) == 0 {
		return "application/octet-stream"
	}

	// Check magic numbers
	switch {
	case len(data) >= 2 && data[0] == 0xFF && data[1] == 0xD8:
		return "image/jpeg"
	case len(data) >= 8 && string(data[:8]) == "\x89PNG\r\n\x1a\n":
		return "image/png"
	case len(data) >= 6 && (string(data[:6]) == "GIF87a" || string(data[:6]) == "GIF89a"):
		return "image/gif"
	case len(data) >= 4 && string(data[:4]) == "RIFF":
		return "image/webp"
	case len(data) >= 2 && (data[0] == 'B' && data[1] == 'M'):
		return "image/bmp"
	case len(data) >= 4 && string(data[:4]) == "\x00\x00\x01\x00":
		return "image/x-icon"
	}

	return "application/octet-stream"
}

// DetectMIMETypeFromFilename detects MIME type from filename extension.
func DetectMIMETypeFromFilename(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	mimeType := mime.TypeByExtension(ext)
	if mimeType != "" {
		return mimeType
	}
	return "application/octet-stream"
}
