package pdf

import (
	"os"
	"strings"
	"testing"
)

func TestInspector_HasText(t *testing.T) {
	inspector := NewInspector()

	// Test with a PDF that has text (create a simple test)
	// Since we can't easily create a real PDF in tests,
	// we'll test with an invalid PDF first
	t.Run("invalid_pdf", func(t *testing.T) {
		invalidPDF := strings.NewReader("not a pdf")
		_, err := inspector.HasText(invalidPDF)
		if err == nil {
			t.Error("Expected error for invalid PDF, got nil")
		}
	})

	t.Run("empty_reader", func(t *testing.T) {
		empty := strings.NewReader("")
		_, err := inspector.HasText(empty)
		if err == nil {
			t.Error("Expected error for empty reader, got nil")
		}
	})
}

func TestInspector_saveToTemp(t *testing.T) {
	content := "test content"
	reader := strings.NewReader(content)

	path, err := SaveToTemp(reader)
	if err != nil {
		t.Fatalf("saveToTemp() error = %v", err)
	}
	defer os.Remove(path)

	// Verify file exists
	_, err = os.Stat(path)
	if err != nil {
		t.Errorf("Temporary file not created: %v", err)
	}

	// Verify content
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	if string(data) != content {
		t.Errorf("File content = %q, want %q", string(data), content)
	}
}
