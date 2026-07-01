package structure

import (
	"fmt"
	"sort"
	"strings"

	"github.com/edwsel/doculai/internal/pdf"
)

// ElementType represents the type of structured element.
type ElementType int

const (
	ElementHeading1 ElementType = iota
	ElementHeading2
	ElementHeading3
	ElementParagraph
	ElementUnorderedList
	ElementOrderedList
	ElementTable
	ElementImage
)

// StructuredElement represents a detected structural element.
type StructuredElement struct {
	Type  ElementType
	Text  string
	Items []string   // For lists
	Table [][]string // For tables
	Image *ImageRef  // For images
}

// ImageRef represents an image reference.
type ImageRef struct {
	Alt  string
	Path string
	Data []byte
}

// Detector detects structure in PDF text elements.
type Detector struct{}

// NewDetector creates a new structure detector.
func NewDetector() *Detector {
	return &Detector{}
}

// Detect analyzes text elements and detects structure.
func (d *Detector) Detect(elements []pdf.TextElement) []StructuredElement {
	if len(elements) == 0 {
		return nil
	}

	// Sort elements by page, then by Y position (top to bottom), then by X
	sorted := d.sortElements(elements)

	// Group elements into rows
	rows := d.groupIntoRows(sorted)

	// Calculate average font size for heading detection
	avgFontSize := d.calculateAverageFontSize(sorted)

	// Detect structure
	result := make([]StructuredElement, 0, len(rows))
	i := 0
	for i < len(rows) {
		row := rows[i]

		// Check for headings
		if heading := d.detectHeading(row, avgFontSize); heading != nil {
			result = append(result, *heading)
			i++
			continue
		}

		// Check for lists
		if list, nextIndex := d.detectList(rows, i); list != nil {
			result = append(result, *list)
			i = nextIndex
			continue
		}

		// Check for tables
		if table, nextIndex := d.detectTable(rows, i); table != nil {
			result = append(result, *table)
			i = nextIndex
			continue
		}

		// Default to paragraph
		result = append(result, StructuredElement{
			Type: ElementParagraph,
			Text: d.joinRowText(row),
		})
		i++
	}

	return result
}

// sortElements sorts elements by page, Y (descending), and X (ascending).
func (d *Detector) sortElements(elements []pdf.TextElement) []pdf.TextElement {
	sorted := make([]pdf.TextElement, len(elements))
	copy(sorted, elements)

	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Page != sorted[j].Page {
			return sorted[i].Page < sorted[j].Page
		}
		if sorted[i].Y != sorted[j].Y {
			return sorted[i].Y > sorted[j].Y // PDF Y coordinates: higher = higher on page
		}
		return sorted[i].X < sorted[j].X
	})

	return sorted
}

// groupIntoRows groups text elements into rows based on Y position.
func (d *Detector) groupIntoRows(elements []pdf.TextElement) [][]pdf.TextElement {
	if len(elements) == 0 {
		return nil
	}

	var rows [][]pdf.TextElement
	var currentRow []pdf.TextElement
	var currentY float64

	for _, elem := range elements {
		if len(currentRow) == 0 {
			currentRow = append(currentRow, elem)
			currentY = elem.Y
			continue
		}

		// Check if element is on the same row (within threshold)
		if abs(elem.Y-currentY) < 3.0 {
			currentRow = append(currentRow, elem)
		} else {
			rows = append(rows, currentRow)
			currentRow = []pdf.TextElement{elem}
			currentY = elem.Y
		}
	}

	if len(currentRow) > 0 {
		rows = append(rows, currentRow)
	}

	return rows
}

// calculateAverageFontSize calculates the average font size.
func (d *Detector) calculateAverageFontSize(elements []pdf.TextElement) float64 {
	if len(elements) == 0 {
		return 12.0 // Default font size
	}

	var sum float64
	for _, elem := range elements {
		sum += elem.FontSize
	}
	return sum / float64(len(elements))
}

// detectHeading detects if a row is a heading.
func (d *Detector) detectHeading(row []pdf.TextElement, avgFontSize float64) *StructuredElement {
	if len(row) == 0 {
		return nil
	}

	// Calculate average font size for this row
	var rowFontSize float64
	var isBold bool
	for _, elem := range row {
		rowFontSize += elem.FontSize
		if elem.IsBold {
			isBold = true
		}
	}
	rowFontSize /= float64(len(row))

	text := d.joinRowText(row)
	if strings.TrimSpace(text) == "" {
		return nil
	}

	// Determine heading level based on font size
	var headingType ElementType
	switch {
	case rowFontSize > avgFontSize*1.3:
		headingType = ElementHeading1
	case rowFontSize > avgFontSize*1.15:
		headingType = ElementHeading2
	case rowFontSize > avgFontSize*1.05 || isBold:
		headingType = ElementHeading3
	default:
		return nil
	}

	return &StructuredElement{
		Type: headingType,
		Text: text,
	}
}

// detectList detects if rows form a list.
func (d *Detector) detectList(rows [][]pdf.TextElement, startIndex int) (element *StructuredElement, next int) {
	if startIndex >= len(rows) {
		return nil, startIndex
	}

	firstRow := rows[startIndex]
	firstText := d.joinRowText(firstRow)

	// Check for list markers
	isUnordered, isOrdered := d.hasListMarker(firstText)
	if !isUnordered && !isOrdered {
		return nil, startIndex
	}

	// Collect all list items
	var items []string
	currentItem := d.cleanListMarker(firstText)
	baseX := d.getRowX(firstRow)

	for i := startIndex + 1; i < len(rows); i++ {
		row := rows[i]
		rowText := d.joinRowText(row)
		rowX := d.getRowX(row)

		// Check if this row continues the list
		if isUnordered && d.hasUnorderedMarker(rowText) {
			if currentItem != "" {
				items = append(items, currentItem)
			}
			currentItem = d.cleanListMarker(rowText)
			continue
		}

		if isOrdered && d.hasOrderedMarker(rowText) {
			if currentItem != "" {
				items = append(items, currentItem)
			}
			currentItem = d.cleanListMarker(rowText)
			continue
		}

		// Check if this is a continuation of the current item (indented or same X)
		if abs(rowX-baseX) < 5.0 || rowX > baseX {
			if currentItem != "" {
				currentItem += " " + rowText
			} else {
				currentItem = rowText
			}
			continue
		}

		// Not a list item anymore
		break
	}

	if currentItem != "" {
		items = append(items, currentItem)
	}

	if len(items) == 0 {
		return nil, startIndex
	}

	listType := ElementUnorderedList
	if isOrdered {
		listType = ElementOrderedList
	}

	return &StructuredElement{
		Type:  listType,
		Items: items,
	}, startIndex + len(items)
}

// detectTable detects if rows form a table.
func (d *Detector) detectTable(rows [][]pdf.TextElement, startIndex int) (element *StructuredElement, next int) {
	if startIndex >= len(rows) {
		return nil, startIndex
	}

	// Need at least 2 rows for a table
	if startIndex+1 >= len(rows) {
		return nil, startIndex
	}

	// Check if rows have consistent column structure
	firstRow := rows[startIndex]
	if len(firstRow) < 2 {
		return nil, startIndex
	}

	// Collect potential table rows
	tableRows := make([][]string, 0, len(rows)-startIndex)
	endIndex := startIndex

	for i := startIndex; i < len(rows); i++ {
		row := rows[i]
		if len(row) < 2 {
			break
		}

		// Check if row has consistent spacing (table-like structure)
		if !d.isTableRow(row) {
			break
		}

		var cells []string
		for _, elem := range row {
			cells = append(cells, strings.TrimSpace(elem.Text))
		}
		tableRows = append(tableRows, cells)
		endIndex = i + 1
	}

	if len(tableRows) < 2 {
		return nil, startIndex
	}

	return &StructuredElement{
		Type:  ElementTable,
		Table: tableRows,
	}, endIndex
}

// isTableRow checks if a row has table-like structure.
func (d *Detector) isTableRow(row []pdf.TextElement) bool {
	if len(row) < 2 {
		return false
	}

	// Check for consistent spacing between elements
	for i := 1; i < len(row); i++ {
		gap := row[i].X - (row[i-1].X + float64(len(row[i-1].Text))*5) // Approximate width
		if gap < 10 {                                                  // Too close, probably not a table
			return false
		}
	}

	return true
}

// hasListMarker checks if text has a list marker.
func (d *Detector) hasListMarker(text string) (ordered, marked bool) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false, false
	}

	// Unordered markers
	unorderedMarkers := []string{"- ", "• ", "* ", "◦ ", "▪ "}
	for _, marker := range unorderedMarkers {
		if strings.HasPrefix(trimmed, marker) {
			return true, false
		}
	}

	// Ordered markers (1., 2., a), b))
	if len(trimmed) > 2 {
		// Number followed by . or )
		firstChar := trimmed[0]
		if (firstChar >= '0' && firstChar <= '9') || (firstChar >= 'a' && firstChar <= 'z') || (firstChar >= 'A' && firstChar <= 'Z') {
			if trimmed[1] == '.' || trimmed[1] == ')' {
				return false, true
			}
		}
	}

	return false, false
}

// hasUnorderedMarker checks for unordered list markers.
func (d *Detector) hasUnorderedMarker(text string) bool {
	trimmed := strings.TrimSpace(text)
	unorderedMarkers := []string{"- ", "• ", "* ", "◦ ", "▪ "}
	for _, marker := range unorderedMarkers {
		if strings.HasPrefix(trimmed, marker) {
			return true
		}
	}
	return false
}

// hasOrderedMarker checks for ordered list markers.
func (d *Detector) hasOrderedMarker(text string) bool {
	trimmed := strings.TrimSpace(text)
	if len(trimmed) < 2 {
		return false
	}
	firstChar := trimmed[0]
	if (firstChar >= '0' && firstChar <= '9') || (firstChar >= 'a' && firstChar <= 'z') || (firstChar >= 'A' && firstChar <= 'Z') {
		if trimmed[1] == '.' || trimmed[1] == ')' {
			return true
		}
	}
	return false
}

// cleanListMarker removes list marker from text.
func (d *Detector) cleanListMarker(text string) string {
	trimmed := strings.TrimSpace(text)

	// Remove unordered markers
	unorderedMarkers := []string{"- ", "• ", "* ", "◦ ", "▪ "}
	for _, marker := range unorderedMarkers {
		if strings.HasPrefix(trimmed, marker) {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, marker))
		}
	}

	// Remove ordered markers
	if len(trimmed) > 2 {
		firstChar := trimmed[0]
		if (firstChar >= '0' && firstChar <= '9') || (firstChar >= 'a' && firstChar <= 'z') || (firstChar >= 'A' && firstChar <= 'Z') {
			if trimmed[1] == '.' || trimmed[1] == ')' {
				return strings.TrimSpace(trimmed[2:])
			}
		}
	}

	return trimmed
}

// getRowX returns the X position of the first element in a row.
func (d *Detector) getRowX(row []pdf.TextElement) float64 {
	if len(row) == 0 {
		return 0
	}
	return row[0].X
}

// joinRowText joins text elements in a row.
func (d *Detector) joinRowText(row []pdf.TextElement) string {
	parts := make([]string, 0, len(row))
	for _, elem := range row {
		parts = append(parts, elem.Text)
	}
	return strings.Join(parts, " ")
}

// abs returns the absolute value of a float64.
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// Formatter formats structured elements into Markdown.
type Formatter struct{}

// NewFormatter creates a new Markdown formatter.
func NewFormatter() *Formatter {
	return &Formatter{}
}

// ToMarkdown converts structured elements to Markdown.
func (f *Formatter) ToMarkdown(elements []StructuredElement) string {
	var sb strings.Builder

	for _, elem := range elements {
		switch elem.Type {
		case ElementHeading1:
			sb.WriteString(fmt.Sprintf("# %s\n\n", elem.Text))
		case ElementHeading2:
			sb.WriteString(fmt.Sprintf("## %s\n\n", elem.Text))
		case ElementHeading3:
			sb.WriteString(fmt.Sprintf("### %s\n\n", elem.Text))
		case ElementParagraph:
			sb.WriteString(fmt.Sprintf("%s\n\n", elem.Text))
		case ElementUnorderedList:
			for _, item := range elem.Items {
				sb.WriteString(fmt.Sprintf("- %s\n", item))
			}
			sb.WriteString("\n")
		case ElementOrderedList:
			for i, item := range elem.Items {
				sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, item))
			}
			sb.WriteString("\n")
		case ElementTable:
			sb.WriteString(f.formatTable(elem.Table))
		case ElementImage:
			if elem.Image != nil {
				sb.WriteString(fmt.Sprintf("![%s](%s)\n\n", elem.Image.Alt, elem.Image.Path))
			}
		}
	}

	return sb.String()
}

// formatTable formats a table as Markdown.
func (f *Formatter) formatTable(table [][]string) string {
	if len(table) == 0 {
		return ""
	}

	var sb strings.Builder

	// Find maximum number of columns
	maxCols := 0
	for _, row := range table {
		if len(row) > maxCols {
			maxCols = len(row)
		}
	}

	// Header row (first row)
	if len(table) > 0 {
		header := table[0]
		sb.WriteString("| ")
		for i, cell := range header {
			if i > 0 {
				sb.WriteString(" | ")
			}
			sb.WriteString(cell)
		}
		// Pad empty cells
		for i := len(header); i < maxCols; i++ {
			sb.WriteString(" | ")
		}
		sb.WriteString(" |\n")

		// Separator
		sb.WriteString("| ")
		for i := 0; i < maxCols; i++ {
			if i > 0 {
				sb.WriteString(" | ")
			}
			sb.WriteString("---")
		}
		sb.WriteString(" |\n")
	}

	// Data rows
	for i := 1; i < len(table); i++ {
		sb.WriteString("| ")
		for j, cell := range table[i] {
			if j > 0 {
				sb.WriteString(" | ")
			}
			sb.WriteString(cell)
		}
		// Pad empty cells
		for j := len(table[i]); j < maxCols; j++ {
			sb.WriteString(" | ")
		}
		sb.WriteString(" |\n")
	}

	sb.WriteString("\n")
	return sb.String()
}
