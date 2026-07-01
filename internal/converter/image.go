package converter

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/edwsel/doculai/internal/image"
	"github.com/edwsel/doculai/internal/vllm"
)

// ImageConverter converts standalone raster images (PNG, JPEG, GIF, WEBP, BMP)
// to Markdown via VLLM OCR. Unlike PDF image conversion there is no metadata
// fallback: a standalone image carries no text layer, so OCR is the only way
// to extract content and the converter errors out when VLLM is not configured.
type ImageConverter struct {
	formatter *image.Formatter
}

// NewImageConverter creates a new image converter.
func NewImageConverter() *ImageConverter {
	return &ImageConverter{
		formatter: image.NewFormatter(),
	}
}

// Convert normalizes the image and runs a single VLLM OCR pass, returning the
// extracted Markdown.
func (c *ImageConverter) Convert(input io.Reader, opts Options) (string, error) {
	logger := opts.logger()

	data, err := io.ReadAll(input)
	if err != nil {
		return "", fmt.Errorf("reading image: %w", err)
	}
	if len(data) == 0 {
		return "", fmt.Errorf("reading image: empty input")
	}

	// OCR is the only extraction path for a standalone image.
	if !hasVLLMConfig(opts) {
		return "", fmt.Errorf("image OCR requires VLLM configuration (--vllm-model, --vllm-url, --vllm-provider)")
	}

	logger.Info("image ocr", "bytes", len(data))

	formatted, err := c.formatter.FormatImage(bytes.NewReader(data), 1)
	if err != nil {
		return "", fmt.Errorf("formatting image: %w", err)
	}
	logger.Debug("normalized image", "width", formatted.Width, "height", formatted.Height)

	vllmOpts := vllm.Options{
		Model:        opts.VLLMModel,
		URL:          opts.VLLMURL,
		Key:          opts.VLLMKey,
		Provider:     opts.VLLMProvider,
		SystemPrompt: opts.VLLMPrompt,
		Reasoning:    opts.VLLMReasoning,
		Concurrency:  opts.VLLMConcurrency,
		Logger:       opts.logger(),
	}

	markdown, err := vllm.ExtractMarkdown(context.Background(), c.formatter.ToBase64URL(formatted), vllmOpts)
	if err != nil {
		return "", fmt.Errorf("image ocr failed: %w", err)
	}

	logger.Info("image ocr done", "chars", len(markdown))
	return markdown, nil
}

// Supports returns true for the raster image MIME types this converter can OCR.
// image/x-icon (ICO) is intentionally excluded because no ICO decoder is
// registered for image.Decode.
func (c *ImageConverter) Supports(mimeType string) bool {
	switch mimeType {
	case "image/png", "image/jpeg", "image/gif", "image/webp", "image/bmp":
		return true
	}
	return false
}
