// Package doculai exposes the public API for converting documents to Markdown.
package doculai

import (
	"bytes"
	"io"
	"log/slog"

	"github.com/edwsel/doculai/internal/converter"
)

// discardLogger is the silent default logger used when none is provided.
var discardLogger = slog.New(slog.NewTextHandler(io.Discard, nil))

// Option configures a Doculai instance.
type Option func(*config)

// config holds the resolved configuration for a Doculai instance.
type config struct {
	logger          *slog.Logger
	vllmURL         string
	vllmKey         string
	vllmModel       string
	vllmProvider    string
	vllmConcurrency int
}

// WithLogger sets the structured logger used by converters. By default (no
// option or a nil logger) conversion is completely silent.
func WithLogger(l *slog.Logger) Option {
	return func(c *config) {
		if l != nil {
			c.logger = l
		}
	}
}

// WithVLLMConcurrency sets the maximum number of parallel page OCR requests
// used by the PDF image converter. A value <= 0 means "use the converter
// default" (see converter.DefaultVLLMConcurrency). This instance value is only
// applied when the per-call Options.VLLMConcurrency is not set (zero).
func WithVLLMConcurrency(n int) Option {
	return func(c *config) {
		c.vllmConcurrency = n
	}
}

// WithVLLMServer sets the VLLM API endpoint URL (e.g.
// "https://api.openai.com/v1"). This instance value is only applied when the
// per-call Options.VLLMURL is not set (empty string).
func WithVLLMServer(url string) Option {
	return func(c *config) {
		c.vllmURL = url
	}
}

// WithVLLMModel sets the VLLM model name (e.g. "gpt-4o"). This instance value
// is only applied when the per-call Options.VLLMModel is not set (empty
// string).
func WithVLLMModel(model string) Option {
	return func(c *config) {
		c.vllmModel = model
	}
}

// WithVLLMKey sets the VLLM API key. This instance value is only applied when
// the per-call Options.VLLMKey is not set (empty string).
func WithVLLMKey(key string) Option {
	return func(c *config) {
		c.vllmKey = key
	}
}

// WithVLLMProvider sets the VLLM provider type ("openai" or "ollama"). This
// instance value is only applied when the per-call Options.VLLMProvider is not
// set (empty string).
func WithVLLMProvider(provider string) Option {
	return func(c *config) {
		c.vllmProvider = provider
	}
}

// Doculai is the main converter that handles various input formats.
type Doculai struct {
	factory         *converter.Factory
	logger          *slog.Logger
	vllmURL         string
	vllmKey         string
	vllmModel       string
	vllmProvider    string
	vllmConcurrency int
}

// New creates a new Doculai instance. The variadic options make New()
// (no arguments) backwards compatible.
func New(opts ...Option) *Doculai {
	cfg := config{logger: discardLogger}
	for _, opt := range opts {
		opt(&cfg)
	}

	factory := converter.NewFactory()
	factory.Register(converter.NewHTMLConverter())
	factory.Register(converter.NewPDFConverter())
	factory.Register(converter.NewImageConverter())

	return &Doculai{
		factory:         factory,
		logger:          cfg.logger,
		vllmURL:         cfg.vllmURL,
		vllmKey:         cfg.vllmKey,
		vllmModel:       cfg.vllmModel,
		vllmProvider:    cfg.vllmProvider,
		vllmConcurrency: cfg.vllmConcurrency,
	}
}

// applyInstance injects instance-level defaults into a per-call Options struct
// without overriding caller-supplied values. Per-call fields always win; empty
// string / zero values fall back to the instance configuration.
func (d *Doculai) applyInstance(opts converter.Options) converter.Options {
	if opts.Logger == nil {
		opts.Logger = d.logger
	}
	if opts.VLLMURL == "" {
		opts.VLLMURL = d.vllmURL
	}
	if opts.VLLMKey == "" {
		opts.VLLMKey = d.vllmKey
	}
	if opts.VLLMModel == "" {
		opts.VLLMModel = d.vllmModel
	}
	if opts.VLLMProvider == "" {
		opts.VLLMProvider = d.vllmProvider
	}
	if opts.VLLMConcurrency == 0 {
		opts.VLLMConcurrency = d.vllmConcurrency
	}
	return opts
}

// Convert converts input data to Markdown.
func (d *Doculai) Convert(input io.Reader, opts converter.Options) (string, error) {
	opts = d.applyInstance(opts)

	// Read input to detect MIME type
	data, err := io.ReadAll(input)
	if err != nil {
		return "", err
	}

	mimeType := converter.DetectMimeType(data)

	// Get appropriate converter
	conv, ok := d.factory.GetConverter(mimeType)
	if !ok {
		return "", converter.ErrUnsupportedFormat
	}

	// Convert using the detected converter
	return conv.Convert(io.NopCloser(bytes.NewReader(data)), opts)
}

// ConvertWithType converts input with explicit MIME type.
func (d *Doculai) ConvertWithType(input io.Reader, mimeType string, opts converter.Options) (string, error) {
	opts = d.applyInstance(opts)

	conv, ok := d.factory.GetConverter(mimeType)
	if !ok {
		return "", converter.ErrUnsupportedFormat
	}

	return conv.Convert(input, opts)
}
