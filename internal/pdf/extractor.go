package pdf

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"sort"
	"strings"

	"github.com/klippa-app/go-pdfium"
	"github.com/klippa-app/go-pdfium/enums"
	"github.com/klippa-app/go-pdfium/references"
	"github.com/klippa-app/go-pdfium/requests"
	"github.com/klippa-app/go-pdfium/responses"
)

// renderDPI is the DPI used when rendering pages as images for OCR.
const renderDPI = 200

// rowYTolerance is the maximum vertical distance (in points) for text rects
// to be considered part of the same row.
const rowYTolerance = 3.0

// TextElement represents a text element with formatting information.
type TextElement struct {
	Text     string
	FontSize float64
	FontName string
	X, Y     float64
	IsBold   bool
	Page     int
}

// ExtractedImage represents an image extracted from a PDF.
type ExtractedImage struct {
	Data    []byte
	Format  string
	PageNum int
	Width   int
	Height  int
}

// Extractor extracts text with formatting from PDF files.
type Extractor struct{}

// NewExtractor creates a new PDF text extractor.
func NewExtractor() *Extractor {
	return &Extractor{}
}

// ExtractText extracts text elements from a PDF with formatting information.
func (e *Extractor) ExtractText(input io.Reader) ([]TextElement, error) {
	data, err := io.ReadAll(input)
	if err != nil {
		return nil, fmt.Errorf("reading input: %w", err)
	}

	doc, instance, cleanup, err := openDocument(data)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	pageCount, err := pageCount(instance, doc)
	if err != nil {
		return nil, err
	}

	var elements []TextElement
	for i := 0; i < pageCount; i++ {
		height, err := pageHeight(instance, doc, i)
		if err != nil {
			continue
		}

		rects, err := pageRects(instance, doc, i)
		if err != nil {
			continue
		}

		for _, rect := range rects {
			elements = append(elements, rectToTextElement(rect, i+1, height))
		}
	}

	return elements, nil
}

// ExtractTextByRow extracts text grouped by rows from a PDF.
func (e *Extractor) ExtractTextByRow(input io.Reader) ([][]TextElement, error) {
	data, err := io.ReadAll(input)
	if err != nil {
		return nil, fmt.Errorf("reading input: %w", err)
	}

	doc, instance, cleanup, err := openDocument(data)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	pageCount, err := pageCount(instance, doc)
	if err != nil {
		return nil, err
	}

	var rows [][]TextElement
	for i := 0; i < pageCount; i++ {
		height, err := pageHeight(instance, doc, i)
		if err != nil {
			continue
		}

		rects, err := pageRects(instance, doc, i)
		if err != nil {
			continue
		}

		pageRows := groupRectsIntoRows(rects, i+1, height)
		rows = append(rows, pageRows...)
	}

	return rows, nil
}

// ExtractImages extracts embedded images from a PDF file using PDFium.
// If no embedded images are found, an error is returned.
func (e *Extractor) ExtractImages(input io.Reader) ([]ExtractedImage, error) {
	data, err := io.ReadAll(input)
	if err != nil {
		return nil, fmt.Errorf("reading input: %w", err)
	}

	doc, instance, cleanup, err := openDocument(data)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	pageCount, err := pageCount(instance, doc)
	if err != nil {
		return nil, err
	}

	var images []ExtractedImage
	for i := 0; i < pageCount; i++ {
		pageImages, err := extractPageImages(instance, doc, i)
		if err != nil {
			continue
		}
		images = append(images, pageImages...)
	}

	if len(images) == 0 {
		return nil, fmt.Errorf("no images found in PDF")
	}

	return images, nil
}

// ExtractImagesAsPages renders each page as an image (PNG) at renderDPI.
// Each page becomes one ExtractedImage, suitable for OCR of scanned documents.
func (e *Extractor) ExtractImagesAsPages(input io.Reader) ([]ExtractedImage, error) {
	data, err := io.ReadAll(input)
	if err != nil {
		return nil, fmt.Errorf("reading input: %w", err)
	}

	doc, instance, cleanup, err := openDocument(data)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	pageCount, err := pageCount(instance, doc)
	if err != nil {
		return nil, err
	}

	images := make([]ExtractedImage, 0, pageCount)
	for i := 0; i < pageCount; i++ {
		renderResp, err := instance.RenderPageInDPI(&requests.RenderPageInDPI{
			DPI: renderDPI,
			Page: requests.Page{
				ByIndex: &requests.PageByIndex{Document: doc, Index: i},
			},
		})
		if err != nil {
			return nil, fmt.Errorf("rendering page %d: %w", i+1, err)
		}

		var buf bytes.Buffer
		if err := png.Encode(&buf, renderResp.Result.Image); err != nil {
			renderResp.Cleanup()
			return nil, fmt.Errorf("encoding page %d as png: %w", i+1, err)
		}
		renderResp.Cleanup()

		images = append(images, ExtractedImage{
			Data:    buf.Bytes(),
			Format:  "png",
			PageNum: i + 1,
			Width:   renderResp.Result.Width,
			Height:  renderResp.Result.Height,
		})
	}

	return images, nil
}

// GetPageCount returns the number of pages in a PDF.
func (e *Extractor) GetPageCount(input io.Reader) (int, error) {
	data, err := io.ReadAll(input)
	if err != nil {
		return 0, fmt.Errorf("reading input: %w", err)
	}

	doc, instance, cleanup, err := openDocument(data)
	if err != nil {
		return 0, err
	}
	defer cleanup()

	return pageCount(instance, doc)
}

// GetPageDimensions returns the dimensions of each page.
func (e *Extractor) GetPageDimensions(input io.Reader) ([]PageDimension, error) {
	data, err := io.ReadAll(input)
	if err != nil {
		return nil, fmt.Errorf("reading input: %w", err)
	}

	doc, instance, cleanup, err := openDocument(data)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	pageCount, err := pageCount(instance, doc)
	if err != nil {
		return nil, err
	}

	dimensions := make([]PageDimension, 0, pageCount)
	for i := 0; i < pageCount; i++ {
		sizeResp, err := instance.FPDF_GetPageSizeByIndex(&requests.FPDF_GetPageSizeByIndex{
			Document: doc,
			Index:    i,
		})
		if err != nil {
			continue
		}
		dimensions = append(dimensions, PageDimension{
			PageNum: i + 1,
			Width:   sizeResp.Width,
			Height:  sizeResp.Height,
		})
	}

	return dimensions, nil
}

// PageDimension represents the dimensions of a PDF page.
type PageDimension struct {
	PageNum int
	Width   float64
	Height  float64
}

// pageCount returns the page count for an open document.
func pageCount(instance pdfium.Pdfium, doc references.FPDF_DOCUMENT) (int, error) {
	resp, err := instance.FPDF_GetPageCount(&requests.FPDF_GetPageCount{Document: doc})
	if err != nil {
		return 0, fmt.Errorf("getting page count: %w", err)
	}
	return resp.PageCount, nil
}

// pageHeight returns the height (in points) of a page by index.
func pageHeight(instance pdfium.Pdfium, doc references.FPDF_DOCUMENT, index int) (float64, error) {
	resp, err := instance.FPDF_GetPageSizeByIndex(&requests.FPDF_GetPageSizeByIndex{
		Document: doc,
		Index:    index,
	})
	if err != nil {
		return 0, err
	}
	return resp.Height, nil
}

// pageRects returns the structured text rects of a page by index.
func pageRects(instance pdfium.Pdfium, doc references.FPDF_DOCUMENT, index int) ([]*responses.GetPageTextStructuredRect, error) {
	resp, err := instance.GetPageTextStructured(&requests.GetPageTextStructured{
		Page: requests.Page{
			ByIndex: &requests.PageByIndex{Document: doc, Index: index},
		},
		Mode:                   requests.GetPageTextStructuredModeRects,
		CollectFontInformation: true,
	})
	if err != nil {
		return nil, err
	}
	return resp.Rects, nil
}

// rectToTextElement converts a PDFium text rect into a TextElement. The page
// height is used to convert the top-down coordinate into the bottom-up Y used
// by the structure detector (higher Y = higher on page).
func rectToTextElement(rect *responses.GetPageTextStructuredRect, pageNum int, pageHeight float64) TextElement {
	el := TextElement{
		Text: rect.Text,
		X:    rect.PointPosition.Left,
		Y:    pageHeight - rect.PointPosition.Top,
		Page: pageNum,
	}
	if rect.FontInformation != nil {
		fontSize := rect.FontInformation.Size
		if fontSize == 0 {
			fontSize = rect.FontInformation.RenderedSize
		}
		el.FontSize = fontSize
		el.FontName = rect.FontInformation.Name
		el.IsBold = isBold(rect.FontInformation.Name, rect.FontInformation.Weight, rect.FontInformation.Flags)
	}
	return el
}

// groupRectsIntoRows groups text rects of a page into rows by vertical
// position. Rects are sorted top-to-bottom, left-to-right before grouping.
func groupRectsIntoRows(rects []*responses.GetPageTextStructuredRect, pageNum int, pageHeight float64) [][]TextElement {
	if len(rects) == 0 {
		return nil
	}

	sorted := make([]*responses.GetPageTextStructuredRect, len(rects))
	copy(sorted, rects)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].PointPosition.Top != sorted[j].PointPosition.Top {
			return sorted[i].PointPosition.Top < sorted[j].PointPosition.Top
		}
		return sorted[i].PointPosition.Left < sorted[j].PointPosition.Left
	})

	var rows [][]TextElement
	var current []TextElement
	var currentTop float64

	for _, rect := range sorted {
		element := rectToTextElement(rect, pageNum, pageHeight)
		if len(current) == 0 {
			current = append(current, element)
			currentTop = rect.PointPosition.Top
			continue
		}
		if absFloat(rect.PointPosition.Top-currentTop) <= rowYTolerance {
			current = append(current, element)
		} else {
			rows = append(rows, current)
			current = []TextElement{element}
			currentTop = rect.PointPosition.Top
		}
	}
	if len(current) > 0 {
		rows = append(rows, current)
	}

	return rows
}

// extractPageImages extracts embedded image objects from a single page.
func extractPageImages(instance pdfium.Pdfium, doc references.FPDF_DOCUMENT, index int) ([]ExtractedImage, error) {
	loadedPage, err := instance.FPDF_LoadPage(&requests.FPDF_LoadPage{Document: doc, Index: index})
	if err != nil {
		return nil, err
	}
	defer instance.FPDF_ClosePage(&requests.FPDF_ClosePage{Page: loadedPage.Page})

	page := requests.Page{ByReference: &loadedPage.Page}

	countResp, err := instance.FPDFPage_CountObjects(&requests.FPDFPage_CountObjects{Page: page})
	if err != nil {
		return nil, err
	}

	var images []ExtractedImage
	for objIdx := 0; objIdx < countResp.Count; objIdx++ {
		objResp, err := instance.FPDFPage_GetObject(&requests.FPDFPage_GetObject{Page: page, Index: objIdx})
		if err != nil {
			continue
		}

		typeResp, err := instance.FPDFPageObj_GetType(&requests.FPDFPageObj_GetType{PageObject: objResp.PageObject})
		if err != nil || typeResp.Type != enums.FPDF_PAGEOBJ_IMAGE {
			continue
		}

		dataResp, err := instance.FPDFImageObj_GetImageDataDecoded(&requests.FPDFImageObj_GetImageDataDecoded{
			ImageObject: objResp.PageObject,
		})
		if err != nil || len(dataResp.Data) == 0 {
			continue
		}

		data := dataResp.Data
		format := detectImageFormat(data)
		if format != "jpeg" && format != "png" {
			if normalized, ok := normalizeToJPEG(data); ok {
				data = normalized
				format = "jpeg"
			}
		}

		width, height := imageSize(instance, objResp.PageObject)

		images = append(images, ExtractedImage{
			Data:    data,
			Format:  format,
			PageNum: index + 1,
			Width:   width,
			Height:  height,
		})
	}

	return images, nil
}

// normalizeToJPEG decodes the image data and re-encodes it as JPEG. It returns
// the encoded bytes and true on success.
func normalizeToJPEG(data []byte) ([]byte, bool) {
	decodedImg, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, false
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, decodedImg, &jpeg.Options{Quality: 85}); err != nil {
		return nil, false
	}
	return buf.Bytes(), true
}

// imageSize returns the pixel dimensions of an image object.
func imageSize(instance pdfium.Pdfium, obj references.FPDF_PAGEOBJECT) (int, int) {
	resp, err := instance.FPDFImageObj_GetImagePixelSize(&requests.FPDFImageObj_GetImagePixelSize{
		ImageObject: obj,
	})
	if err != nil {
		return 0, 0
	}
	return int(resp.Width), int(resp.Height)
}

// fontFlagForceBold is PDF font descriptor flag bit 19 (ForceBold).
const fontFlagForceBold = 1 << 18

// isBold reports whether the font is bold based on descriptor flags, weight,
// or the font name as a fallback.
func isBold(name string, weight, flags int) bool {
	if flags&fontFlagForceBold != 0 {
		return true
	}
	if weight >= 700 {
		return true
	}
	return isBoldFont(name)
}

// isBoldFont checks if a font name indicates bold text.
func isBoldFont(fontName string) bool {
	fontName = strings.ToLower(fontName)
	boldIndicators := []string{"bold", "heavy", "black", "medium", "demi", "extra"}
	for _, indicator := range boldIndicators {
		if strings.Contains(fontName, indicator) {
			return true
		}
	}
	return false
}

// absFloat returns the absolute value of a float64.
func absFloat(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// detectImageFormat detects image format from magic bytes.
func detectImageFormat(data []byte) string {
	if len(data) == 0 {
		return "unknown"
	}

	// Check magic numbers
	switch {
	case len(data) >= 2 && data[0] == 0xFF && data[1] == 0xD8:
		return "jpeg"
	case len(data) >= 8 && string(data[:8]) == "\x89PNG\r\n\x1a\n":
		return "png"
	case len(data) >= 6 && (string(data[:6]) == "GIF87a" || string(data[:6]) == "GIF89a"):
		return "gif"
	case len(data) >= 4 && string(data[:4]) == "RIFF":
		return "webp"
	case len(data) >= 2 && data[0] == 'B' && data[1] == 'M':
		return "bmp"
	case len(data) >= 4 && string(data[:4]) == "\x00\x00\x01\x00":
		return "ico"
	}

	return "unknown"
}
