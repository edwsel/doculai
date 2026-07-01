package converter

import (
	"io"

	"github.com/edwsel/doculai/internal/pdf"
	"github.com/edwsel/doculai/internal/structure"
)

// PDFTextConverter converts PDF with text to Markdown.
type PDFTextConverter struct {
	extractor *pdf.Extractor
	detector  *structure.Detector
	formatter *structure.Formatter
}

// NewPDFTextConverter creates a new PDF text converter.
func NewPDFTextConverter() *PDFTextConverter {
	return &PDFTextConverter{
		extractor: pdf.NewExtractor(),
		detector:  structure.NewDetector(),
		formatter: structure.NewFormatter(),
	}
}

// Convert converts PDF text to Markdown.
func (c *PDFTextConverter) Convert(input io.Reader, opts Options) (string, error) {
	logger := opts.logger()

	// Extract text elements with formatting
	elements, err := c.extractor.ExtractText(input)
	if err != nil {
		return "", err
	}
	logger.Info("pdf text extracted", "elements", len(elements))

	// Detect structure
	structured := c.detector.Detect(elements)
	logger.Debug("structure detected", "elements", len(structured))

	// Format to Markdown
	markdown := c.formatter.ToMarkdown(structured)

	return markdown, nil
}

// Supports returns true for PDF MIME types.
func (c *PDFTextConverter) Supports(mimeType string) bool {
	return mimeType == "application/pdf"
}
