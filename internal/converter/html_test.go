package converter

import (
	"strings"
	"testing"
)

func TestHTMLConverter_Convert(t *testing.T) {
	converter := NewHTMLConverter()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple heading",
			input:    "<h1>Hello World</h1>",
			expected: "# Hello World\n",
		},
		{
			name:     "bold text",
			input:    "<strong>Bold Text</strong>",
			expected: "**Bold Text**\n",
		},
		{
			name:     "paragraph",
			input:    "<p>This is a paragraph.</p>",
			expected: "This is a paragraph.\n",
		},
		{
			name:     "unordered list",
			input:    "<ul><li>Item 1</li><li>Item 2</li></ul>",
			expected: "- Item 1\n- Item 2\n",
		},
		{
			name:     "ordered list",
			input:    "<ol><li>First</li><li>Second</li></ol>",
			expected: "1. First\n2. Second\n",
		},
		{
			name:     "table",
			input:    "<table><tr><th>A</th><th>B</th></tr><tr><td>1</td><td>2</td></tr></table>",
			expected: "| A | B |\n|---|---|\n| 1 | 2 |",
		},
		{
			name:     "link",
			input:    "<a href=\"https://example.com\">Example</a>",
			expected: "[Example](https://example.com)\n",
		},
		{
			name:     "image",
			input:    "<img src=\"image.png\" alt=\"Test\" />",
			expected: "![Test](image.png)\n",
		},
		{
			name:     "mixed content",
			input:    "<h1>Title</h1><p>Some <strong>bold</strong> text.</p><ul><li>Item</li></ul>",
			expected: "# Title\n\nSome **bold** text.\n\n- Item\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := converter.Convert(strings.NewReader(tt.input), Options{})
			if err != nil {
				t.Fatalf("Convert() error = %v", err)
			}

			// Normalize newlines for comparison
			result = strings.TrimSpace(result)
			expected := strings.TrimSpace(tt.expected)

			if result != expected {
				t.Errorf("Convert() = %q, want %q", result, expected)
			}
		})
	}
}

func TestHTMLConverter_Supports(t *testing.T) {
	converter := NewHTMLConverter()

	if !converter.Supports("text/html") {
		t.Error("Supports(text/html) = false, want true")
	}

	if converter.Supports("application/pdf") {
		t.Error("Supports(application/pdf) = true, want false")
	}
}
