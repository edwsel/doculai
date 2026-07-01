package doculai

import (
	"strings"
	"testing"

	"github.com/edwsel/doculai/internal/converter"
)

func TestWithVLLMServer_StoresURL(t *testing.T) {
	d := New(WithVLLMServer("https://api.example.com/v1"))
	if d.vllmURL != "https://api.example.com/v1" {
		t.Errorf("vllmURL = %q, want https://api.example.com/v1", d.vllmURL)
	}
}

func TestWithVLLMModel_StoresModel(t *testing.T) {
	d := New(WithVLLMModel("gpt-4o"))
	if d.vllmModel != "gpt-4o" {
		t.Errorf("vllmModel = %q, want gpt-4o", d.vllmModel)
	}
}

func TestWithVLLMKey_StoresKey(t *testing.T) {
	d := New(WithVLLMKey("sk-test"))
	if d.vllmKey != "sk-test" {
		t.Errorf("vllmKey = %q, want sk-test", d.vllmKey)
	}
}

func TestWithVLLMProvider_StoresProvider(t *testing.T) {
	d := New(WithVLLMProvider("ollama"))
	if d.vllmProvider != "ollama" {
		t.Errorf("vllmProvider = %q, want ollama", d.vllmProvider)
	}
}

func TestApplyInstance_VLLMFieldsFromInstance(t *testing.T) {
	d := New(
		WithVLLMServer("https://api.example.com/v1"),
		WithVLLMModel("gpt-4o"),
		WithVLLMKey("sk-test"),
		WithVLLMProvider("openai"),
		WithVLLMConcurrency(3),
	)

	opts := d.applyInstance(converter.Options{})

	if opts.VLLMURL != "https://api.example.com/v1" {
		t.Errorf("VLLMURL = %q, want instance value", opts.VLLMURL)
	}
	if opts.VLLMModel != "gpt-4o" {
		t.Errorf("VLLMModel = %q, want instance value", opts.VLLMModel)
	}
	if opts.VLLMKey != "sk-test" {
		t.Errorf("VLLMKey = %q, want instance value", opts.VLLMKey)
	}
	if opts.VLLMProvider != "openai" {
		t.Errorf("VLLMProvider = %q, want instance value", opts.VLLMProvider)
	}
	if opts.VLLMConcurrency != 3 {
		t.Errorf("VLLMConcurrency = %d, want 3", opts.VLLMConcurrency)
	}
}

func TestApplyInstance_PerCallValuesWin(t *testing.T) {
	d := New(
		WithVLLMServer("https://instance.example.com"),
		WithVLLMModel("instance-model"),
		WithVLLMKey("instance-key"),
		WithVLLMProvider("ollama"),
		WithVLLMConcurrency(2),
	)

	opts := d.applyInstance(converter.Options{
		VLLMURL:         "https://call.example.com",
		VLLMModel:       "call-model",
		VLLMKey:         "call-key",
		VLLMProvider:    "openai",
		VLLMConcurrency: 9,
	})

	if opts.VLLMURL != "https://call.example.com" {
		t.Errorf("VLLMURL = %q, per-call value should win", opts.VLLMURL)
	}
	if opts.VLLMModel != "call-model" {
		t.Errorf("VLLMModel = %q, per-call value should win", opts.VLLMModel)
	}
	if opts.VLLMKey != "call-key" {
		t.Errorf("VLLMKey = %q, per-call value should win", opts.VLLMKey)
	}
	if opts.VLLMProvider != "openai" {
		t.Errorf("VLLMProvider = %q, per-call value should win", opts.VLLMProvider)
	}
	if opts.VLLMConcurrency != 9 {
		t.Errorf("VLLMConcurrency = %d, per-call value should win", opts.VLLMConcurrency)
	}
}

func TestApplyInstance_PartialPerCallOverride(t *testing.T) {
	d := New(
		WithVLLMServer("https://instance.example.com"),
		WithVLLMModel("instance-model"),
		WithVLLMKey("instance-key"),
		WithVLLMProvider("ollama"),
	)

	// Only override the model per-call; everything else falls back to instance.
	opts := d.applyInstance(converter.Options{
		VLLMModel: "call-model",
	})

	if opts.VLLMModel != "call-model" {
		t.Errorf("VLLMModel = %q, per-call value should win", opts.VLLMModel)
	}
	if opts.VLLMURL != "https://instance.example.com" {
		t.Errorf("VLLMURL = %q, should fall back to instance", opts.VLLMURL)
	}
	if opts.VLLMKey != "instance-key" {
		t.Errorf("VLLMKey = %q, should fall back to instance", opts.VLLMKey)
	}
	if opts.VLLMProvider != "ollama" {
		t.Errorf("VLLMProvider = %q, should fall back to instance", opts.VLLMProvider)
	}
}

func TestConvert_InstanceVLLMConfigPropagated(t *testing.T) {
	// Configure VLLM entirely at the instance level, then convert HTML (which
	// doesn't use VLLM but exercises applyInstance via Convert).
	d := New(
		WithVLLMServer("https://api.example.com/v1"),
		WithVLLMModel("gpt-4o"),
		WithVLLMKey("sk-test"),
		WithVLLMProvider("openai"),
	)

	// Capture the opts that would reach a converter by using a wrapper.
	// Since HTML conversion ignores VLLM opts, we just verify it doesn't error.
	_, err := d.Convert(strings.NewReader("<h1>Hi</h1>"), converter.Options{})
	if err != nil {
		t.Fatalf("Convert() error = %v", err)
	}
}
