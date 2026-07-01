package main

import (
	"image"
	"image/color"
	"image/png"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/edwsel/doculai/internal/converter"
	"github.com/edwsel/doculai/pkg/doculai"
	"github.com/edwsel/doculai/test/mock"
)

func writePNG(t *testing.T, path string, w, h int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			img.Set(x, y, color.RGBA{uint8(x % 256), uint8(y % 256), 128, 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatalf("encode png %s: %v", path, err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestDetectMimeTypeFromFile_Images(t *testing.T) {
	tests := map[string]string{
		"doc.html":   "text/html",
		"doc.htm":    "text/html",
		"doc.pdf":    "application/pdf",
		"photo.png":  "image/png",
		"photo.jpg":  "image/jpeg",
		"photo.jpeg": "image/jpeg",
		"anim.gif":   "image/gif",
		"pic.webp":   "image/webp",
		"pic.bmp":    "image/bmp",
		"notes.txt":  "",
		"noext":      "",
		"README.MD":  "", // not a recognized doc/image extension
	}
	for name, want := range tests {
		t.Run(name, func(t *testing.T) {
			if got := detectMimeTypeFromFile(name); got != want {
				t.Errorf("detectMimeTypeFromFile(%q) = %q, want %q", name, got, want)
			}
		})
	}
}

func TestMimeTypeFromString_Image(t *testing.T) {
	if got := mimeTypeFromString("image"); got != "image/*" {
		t.Errorf("mimeTypeFromString(image) = %q, want image/*", got)
	}
	for _, in := range []string{"html", "pdf", "image", "IMAGE", "Image"} {
		if got := mimeTypeFromString(in); got == "" {
			t.Errorf("mimeTypeFromString(%q) unexpectedly empty", in)
		}
	}
	if got := mimeTypeFromString("unknown"); got != "" {
		t.Errorf("mimeTypeFromString(unknown) = %q, want empty", got)
	}
}

func TestConvertDirectory_MixedFiles(t *testing.T) {
	server := mock.MockVLLMServer()
	defer server.Close()

	dir := t.TempDir()
	// Nested structure to verify recursion + sorted sections.
	writeFile(t, filepath.Join(dir, "a.html"), "<h1>First</h1>")
	writeFile(t, filepath.Join(dir, "b.txt"), "just some notes") // unrecognized -> skipped
	writePNG(t, filepath.Join(dir, "sub", "c.png"), 8, 8)

	opts := converter.Options{
		VLLMModel: "gpt-4o", VLLMURL: server.URL, VLLMKey: "k", VLLMProvider: "openai",
		Logger: quietLogger(),
	}
	d := doculai.New(doculai.WithLogger(opts.Logger))

	result, err := convertDirectory(dir, d, opts, quietLogger(), "")
	if err != nil {
		t.Fatalf("convertDirectory() error = %v", err)
	}

	// Both recognized files produce sections in sorted relative-path order.
	if !strings.Contains(result, "## File: a.html") {
		t.Errorf("missing section for a.html:\n%s", result)
	}
	if !strings.Contains(result, "## File: sub/c.png") {
		t.Errorf("missing section for sub/c.png:\n%s", result)
	}
	// HTML content converted to Markdown.
	if !strings.Contains(result, "# First") {
		t.Errorf("missing converted HTML content:\n%s", result)
	}
	// Image OCR passthrough from mock.
	if !strings.Contains(result, "Mock Response") {
		t.Errorf("missing image OCR content:\n%s", result)
	}
	// The unrecognized txt file must be skipped (no section header).
	if strings.Contains(result, "## File: b.txt") {
		t.Errorf("unrecognized b.txt should be skipped:\n%s", result)
	}
	// Sections separated by a horizontal rule.
	if !strings.Contains(result, "\n\n---\n\n") {
		t.Errorf("expected sections joined by horizontal rule:\n%s", result)
	}
	// Sorted order: a.html section should appear before sub/c.png section.
	if iA, iC := strings.Index(result, "## File: a.html"), strings.Index(result, "## File: sub/c.png"); iA == -1 || iC == -1 || iA > iC {
		t.Errorf("expected a.html before sub/c.png (sorted), got indices %d, %d", iA, iC)
	}
}

func TestConvertDirectory_SkipUnrecognized(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "readme.md"), "# nothing")   // .md not recognized as image/html/pdf
	writeFile(t, filepath.Join(dir, "data.bin"), "\x00\x01\x02") // unknown magic

	d := doculai.New(doculai.WithLogger(quietLogger()))
	_, err := convertDirectory(dir, d, converter.Options{Logger: quietLogger()}, quietLogger(), "")
	if err != nil {
		t.Fatalf("convertDirectory() error = %v", err)
	}
	// No recognized files -> no error, empty result is acceptable.
}

func TestConvertDirectory_FailFastOnOCRConfigMissing(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.html"), "<h1>First</h1>")
	writePNG(t, filepath.Join(dir, "b.png"), 8, 8) // needs VLLM, none configured

	// No VLLM config: ImageConverter errors -> batch stops.
	d := doculai.New(doculai.WithLogger(quietLogger()))
	_, err := convertDirectory(dir, d, converter.Options{Logger: quietLogger()}, quietLogger(), "")
	if err == nil {
		t.Fatal("expected fail-fast error for image without VLLM config, got nil")
	}
	if !strings.Contains(err.Error(), "converting b.png") {
		t.Errorf("error should be wrapped with the offending file, got: %v", err)
	}
}

func TestConvertOne_RoutesByMime(t *testing.T) {
	d := doculai.New(doculai.WithLogger(quietLogger()))

	t.Run("html", func(t *testing.T) {
		out, err := convertOne([]byte("<p>hello</p>"), "text/html", converter.Options{Logger: quietLogger()}, d, "")
		if err != nil {
			t.Fatalf("convertOne html: %v", err)
		}
		if !strings.Contains(out, "hello") {
			t.Errorf("unexpected html output: %q", out)
		}
	})

	t.Run("unsupported mime errors", func(t *testing.T) {
		_, err := convertOne([]byte("x"), "application/unknown", converter.Options{Logger: quietLogger()}, d, "")
		if err == nil {
			t.Fatal("expected error for unsupported MIME, got nil")
		}
	})
}
