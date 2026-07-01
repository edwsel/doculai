package doculai

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/edwsel/doculai/internal/converter"
)

func TestNew_WithLoggerInjectsIntoConvert(t *testing.T) {
	var buf bytes.Buffer
	custom := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	d := New(WithLogger(custom))
	if d.logger != custom {
		t.Fatal("WithLogger did not store the logger on the instance")
	}

	_, err := d.Convert(strings.NewReader("<h1>Hi</h1>"), converter.Options{})
	if err != nil {
		t.Fatalf("Convert() error = %v", err)
	}

	if !strings.Contains(buf.String(), "converting html") {
		t.Errorf("expected injected logger to capture converter INFO output, got:\n%s", buf.String())
	}
}

func TestNew_DefaultIsSilent(t *testing.T) {
	d := New()
	if d.logger == nil {
		t.Fatal("New() should always have a non-nil (discard) logger")
	}

	// Default logger must not error or produce observable output.
	_, err := d.Convert(strings.NewReader("<h1>Hi</h1>"), converter.Options{})
	if err != nil {
		t.Fatalf("Convert() error = %v", err)
	}
}

func TestConvert_PerCallLoggerWins(t *testing.T) {
	var instanceBuf, callBuf bytes.Buffer
	instanceLogger := slog.New(slog.NewTextHandler(&instanceBuf, nil))
	callLogger := slog.New(slog.NewTextHandler(&callBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	d := New(WithLogger(instanceLogger))
	_, err := d.Convert(strings.NewReader("<h1>Hi</h1>"), converter.Options{Logger: callLogger})
	if err != nil {
		t.Fatalf("Convert() error = %v", err)
	}

	// The per-call logger takes priority over the instance logger.
	if !strings.Contains(callBuf.String(), "converting html") {
		t.Errorf("expected per-call logger to receive converter logs, got:\n%s", callBuf.String())
	}
	if instanceBuf.Len() != 0 {
		t.Errorf("instance logger should not be used when a per-call logger is set, got:\n%s", instanceBuf.String())
	}
}
