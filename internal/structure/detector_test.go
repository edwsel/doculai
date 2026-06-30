package structure

import (
	"testing"

	"doculai/internal/pdf"
)

func TestDetector_sortElements(t *testing.T) {
	detector := NewDetector()

	input := []pdf.TextElement{
		{Text: "C", Page: 1, Y: 50, X: 30},
		{Text: "A", Page: 1, Y: 100, X: 10},
		{Text: "B", Page: 1, Y: 100, X: 20},
		{Text: "D", Page: 2, Y: 100, X: 10},
	}

	result := detector.sortElements(input)

	// Should be sorted by page, then Y (desc), then X (asc)
	expected := []string{"A", "B", "C", "D"}
	for i, exp := range expected {
		if result[i].Text != exp {
			t.Errorf("sortElements()[%d].Text = %q, expected %q", i, result[i].Text, exp)
		}
	}
}

func TestDetector_groupIntoRows(t *testing.T) {
	detector := NewDetector()

	input := []pdf.TextElement{
		{Text: "A", Y: 100},
		{Text: "B", Y: 100},
		{Text: "C", Y: 95},
		{Text: "D", Y: 50},
	}

	rows := detector.groupIntoRows(input)

	if len(rows) != 3 {
		t.Fatalf("groupIntoRows() returned %d rows, expected 3", len(rows))
	}

	if len(rows[0]) != 2 || rows[0][0].Text != "A" || rows[0][1].Text != "B" {
		t.Errorf("Row 0 = %v, expected [A, B]", rows[0])
	}

	if len(rows[1]) != 1 || rows[1][0].Text != "C" {
		t.Errorf("Row 1 = %v, expected [C]", rows[1])
	}

	if len(rows[2]) != 1 || rows[2][0].Text != "D" {
		t.Errorf("Row 2 = %v, expected [D]", rows[2])
	}
}

func TestDetector_calculateAverageFontSize(t *testing.T) {
	detector := NewDetector()

	t.Run("normal", func(t *testing.T) {
		input := []pdf.TextElement{
			{FontSize: 10},
			{FontSize: 20},
			{FontSize: 30},
		}
		result := detector.calculateAverageFontSize(input)
		if result != 20.0 {
			t.Errorf("calculateAverageFontSize() = %f, expected 20.0", result)
		}
	})

	t.Run("empty", func(t *testing.T) {
		result := detector.calculateAverageFontSize([]pdf.TextElement{})
		if result != 12.0 {
			t.Errorf("calculateAverageFontSize() = %f, expected 12.0", result)
		}
	})
}

func TestFormatter_formatTable(t *testing.T) {
	formatter := NewFormatter()

	t.Run("simple table", func(t *testing.T) {
		table := [][]string{
			{"A", "B"},
			{"1", "2"},
		}
		result := formatter.formatTable(table)
		expected := "| A | B |\n| --- | --- |\n| 1 | 2 |\n\n"
		if result != expected {
			t.Errorf("formatTable() = %q, expected %q", result, expected)
		}
	})

	t.Run("empty table", func(t *testing.T) {
		result := formatter.formatTable([][]string{})
		if result != "" {
			t.Errorf("formatTable() = %q, expected empty string", result)
		}
	})

	t.Run("uneven columns", func(t *testing.T) {
		table := [][]string{
			{"A", "B", "C"},
			{"1", "2"},
		}
		result := formatter.formatTable(table)
		expected := "| A | B | C |\n| --- | --- | --- |\n| 1 | 2 |  |\n\n"
		if result != expected {
			t.Errorf("formatTable() = %q, expected %q", result, expected)
		}
	})
}

func Test_abs(t *testing.T) {
	tests := []struct {
		input    float64
		expected float64
	}{
		{5.0, 5.0},
		{-5.0, 5.0},
		{0.0, 0.0},
	}

	for _, tt := range tests {
		result := abs(tt.input)
		if result != tt.expected {
			t.Errorf("abs(%f) = %f, expected %f", tt.input, result, tt.expected)
		}
	}
}
