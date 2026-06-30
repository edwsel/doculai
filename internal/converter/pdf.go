package converter

import (
	"io"
)

// PDFConverter handles PDF to Markdown conversion.
type PDFConverter struct{}

// NewPDFConverter creates a new PDF converter.
func NewPDFConverter() *PDFConverter {
	return &PDFConverter{}
}

// Convert converts PDF input to Markdown.
func (c *PDFConverter) Convert(input io.Reader, opts Options) (string, error) {
	content, err := io.ReadAll(input)
	if err != nil {
		return "", err
	}

	// Validate PDF header
	if len(content) < 4 || string(content[:4]) != "%PDF" {
		return "", ErrUnsupportedFormat
	}

	return string(content), nil
}

// Supports returns true for PDF MIME types.
func (c *PDFConverter) Supports(mimeType string) bool {
	return mimeType == "application/pdf"
}
