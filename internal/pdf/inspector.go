package pdf

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/klippa-app/go-pdfium/requests"
)

// minTextChars is the minimum amount of non-empty text required across the
// document to consider it a text-based PDF.
const minTextChars = 20

// Inspector checks PDF content and structure.
type Inspector struct{}

// NewInspector creates a new PDF inspector.
func NewInspector() *Inspector {
	return &Inspector{}
}

// HasText checks if the PDF contains meaningful text content.
func (i *Inspector) HasText(input io.Reader) (bool, error) {
	data, err := io.ReadAll(input)
	if err != nil {
		return false, fmt.Errorf("reading input: %w", err)
	}

	doc, instance, cleanup, err := openDocument(data)
	if err != nil {
		return false, err
	}
	defer cleanup()

	pageCount, err := pageCount(instance, doc)
	if err != nil {
		return false, err
	}

	totalChars := 0
	for i := 0; i < pageCount; i++ {
		textResp, err := instance.GetPageText(&requests.GetPageText{
			Page: requests.Page{
				ByIndex: &requests.PageByIndex{Document: doc, Index: i},
			},
		})
		if err != nil {
			continue
		}
		totalChars += len(strings.TrimSpace(textResp.Text))
	}

	return totalChars > minTextChars, nil
}

// SaveToTemp saves reader content to a temporary file.
func SaveToTemp(input io.Reader) (string, error) {
	tmpFile, err := os.CreateTemp("", "doculai-*.pdf")
	if err != nil {
		return "", err
	}
	defer func() {
		_ = tmpFile.Close()
	}()

	_, err = io.Copy(tmpFile, input)
	if err != nil {
		_ = os.Remove(tmpFile.Name())
		return "", err
	}

	return tmpFile.Name(), nil
}
