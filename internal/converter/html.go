package converter

import (
	"io"

	"github.com/JohannesKaufmann/html-to-markdown/v2/converter"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/base"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/commonmark"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/table"
)

// HTMLConverter converts HTML to Markdown.
type HTMLConverter struct{}

// NewHTMLConverter creates a new HTML converter.
func NewHTMLConverter() *HTMLConverter {
	return &HTMLConverter{}
}

// Convert converts HTML input to Markdown.
func (c *HTMLConverter) Convert(input io.Reader, opts Options) (string, error) {
	content, err := io.ReadAll(input)
	if err != nil {
		return "", err
	}

	logger := opts.logger()
	logger.Info("converting html", "bytes", len(content))

	// Use html-to-markdown/v2 with plugins for better conversion
	conv := converter.NewConverter(
		converter.WithPlugins(
			base.NewBasePlugin(),
			commonmark.NewCommonmarkPlugin(),
			table.NewTablePlugin(),
		),
	)

	markdown, err := conv.ConvertString(string(content))
	if err != nil {
		return "", err
	}

	logger.Info("converted html", "bytes_out", len(markdown))
	return markdown, nil
}

// Supports returns true for HTML MIME types.
func (c *HTMLConverter) Supports(mimeType string) bool {
	return mimeType == "text/html"
}
