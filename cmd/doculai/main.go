package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/edwsel/doculai/internal/converter"
	"github.com/edwsel/doculai/internal/pdf"
	"github.com/edwsel/doculai/pkg/doculai"
)

const longDescription = `Convert HTML or PDF files to Markdown.

Examples:
  # HTML to Markdown
  doculai -i input.html -o output.md

  # PDF with text to Markdown
  doculai -i document.pdf -o output.md

  # PDF with images to Markdown (with VLLM OCR)
  doculai -i scan.pdf -o output.md \
    --vllm-model gpt-4o \
    --vllm-url https://api.openai.com/v1 \
    --vllm-key sk-... \
    --vllm-provider openai

  # Read from stdin, write to stdout
  cat input.html | doculai -t html > output.md

  # Verbosity (logs go to stderr, stdout stays clean)
  doculai -i input.html -o output.md -vv
`

func main() {
	var (
		inputFile       string
		outputFile      string
		inputType       string
		vllmModel       string
		vllmURL         string
		vllmKey         string
		vllmProvider    string
		vllmPrompt      string
		withReasoning   bool
		vllmConcurrency int
		quiet           bool
		vCount          int
	)

	cmd := &cli.Command{
		Name:                   "doculai",
		Usage:                  "Convert HTML or PDF files to Markdown.",
		Description:            longDescription,
		UseShortOptionHandling: true,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "input",
				Aliases:     []string{"i"},
				Destination: &inputFile,
				Usage:       "Input file (or use stdin if not provided)",
			},
			&cli.StringFlag{
				Name:        "output",
				Aliases:     []string{"o"},
				Destination: &outputFile,
				Usage:       "Output file (or use stdout if not provided)",
			},
			&cli.StringFlag{
				Name:        "type",
				Aliases:     []string{"t"},
				Value:       "auto",
				Destination: &inputType,
				Usage:       "Input type: auto, html, pdf, image",
			},
			&cli.StringFlag{
				Name:        "vllm-model",
				Destination: &vllmModel,
				Usage:       "VLLM model name",
			},
			&cli.StringFlag{
				Name:        "vllm-url",
				Destination: &vllmURL,
				Usage:       "VLLM API URL",
			},
			&cli.StringFlag{
				Name:        "vllm-key",
				Destination: &vllmKey,
				Usage:       "VLLM API key",
			},
			&cli.StringFlag{
				Name:        "vllm-provider",
				Value:       "openai",
				Destination: &vllmProvider,
				Usage:       "VLLM provider: openai, ollama",
			},
			&cli.StringFlag{
				Name:        "vllm-prompt",
				Destination: &vllmPrompt,
				Usage:       "Custom system prompt (overrides default)",
			},
			&cli.BoolFlag{
				Name:        "with-reasoning",
				Destination: &withReasoning,
				Usage:       "Enable reasoning for reasoning-capable models (disabled by default)",
			},
			&cli.IntFlag{
				Name:        "vllm-concurrency",
				Value:       converter.DefaultVLLMConcurrency,
				Destination: &vllmConcurrency,
				Usage:       "Max parallel VLLM page OCR requests",
			},
			&cli.BoolFlag{
				Name:        "quiet",
				Aliases:     []string{"q"},
				Destination: &quiet,
				Usage:       "Quiet mode: log only errors to stderr",
			},
			&cli.BoolFlag{
				Name:    "verbose",
				Aliases: []string{"v"},
				Config: cli.BoolConfig{
					Count: &vCount,
				},
				Usage: "Increase verbosity (repeatable: -v info, -vv debug, -vvv trace)",
			},
		},
		// ExitErrHandler owns process termination. Operational errors from the
		// Action are already logged via slog; we surface them as silent exit
		// code 1 by returning cli.Exit("", 1). Framework-level errors (e.g.
		// flag parsing) carry a message and are printed to stderr so the user
		// gets feedback. stdout therefore stays clean for Markdown output.
		ExitErrHandler: func(_ context.Context, _ *cli.Command, err error) {
			if err == nil {
				return
			}
			code := 1
			var ec cli.ExitCoder
			if errors.As(err, &ec) {
				code = ec.ExitCode()
			}
			if msg := err.Error(); msg != "" {
				fmt.Fprintln(os.Stderr, msg)
			}
			os.Exit(code)
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return run(ctx, runConfig{
				inputFile:       inputFile,
				outputFile:      outputFile,
				inputType:       inputType,
				quiet:           quiet,
				vCount:          vCount,
				vllmModel:       vllmModel,
				vllmURL:         vllmURL,
				vllmKey:         vllmKey,
				vllmProvider:    vllmProvider,
				vllmPrompt:      vllmPrompt,
				withReasoning:   withReasoning,
				vllmConcurrency: vllmConcurrency,
			})
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		// ExitErrHandler above handles exit codes; this is a defensive backstop.
		os.Exit(1)
	}
}

type runConfig struct {
	inputFile       string
	outputFile      string
	inputType       string
	quiet           bool
	vCount          int
	vllmModel       string
	vllmURL         string
	vllmKey         string
	vllmProvider    string
	vllmPrompt      string
	withReasoning   bool
	vllmConcurrency int
}

func run(_ context.Context, cfg runConfig) error {
	// Resolve verbosity conflict: -q/--quiet wins over -v.
	if cfg.quiet && cfg.vCount > 0 {
		fmt.Fprintln(os.Stderr, "Warning: -v ignored because -q/--quiet is set")
		cfg.vCount = 0
	}

	// Build the structured logger (text format to stderr only).
	level := levelFromVerbosity(cfg.quiet, cfg.vCount)
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

	// Create converter options (per-call values take priority over
	// instance-level options; CLI flags always populate these).
	opts := converter.Options{
		VLLMModel:       cfg.vllmModel,
		VLLMURL:         cfg.vllmURL,
		VLLMKey:         cfg.vllmKey,
		VLLMProvider:    cfg.vllmProvider,
		VLLMPrompt:      cfg.vllmPrompt,
		VLLMReasoning:   cfg.withReasoning,
		VLLMConcurrency: cfg.vllmConcurrency,
		Logger:          logger,
	}

	// Create doculai instance with VLLM server/model/key/provider at the
	// instance level (acts as fallback for per-call Options).
	d := doculai.New(
		doculai.WithLogger(logger),
		doculai.WithVLLMServer(cfg.vllmURL),
		doculai.WithVLLMModel(cfg.vllmModel),
		doculai.WithVLLMKey(cfg.vllmKey),
		doculai.WithVLLMProvider(cfg.vllmProvider),
		doculai.WithVLLMConcurrency(cfg.vllmConcurrency),
	)

	// Directory input: walk recursively and merge per-file sections.
	if cfg.inputFile != "" {
		info, err := os.Stat(cfg.inputFile)
		if err != nil {
			logger.Error("opening input file", "err", err)
			return cli.Exit("", 1)
		}
		if info.IsDir() {
			result, err := convertDirectory(cfg.inputFile, d, opts, logger)
			if err != nil {
				logger.Error("converting directory", "err", err)
				return cli.Exit("", 1)
			}
			return writeOutput(result, cfg.outputFile, logger)
		}
	}

	// Determine input source.
	var input io.Reader
	var mimeType string

	if cfg.inputFile != "" {
		f, err := os.Open(cfg.inputFile)
		if err != nil {
			logger.Error("opening input file", "err", err)
			return cli.Exit("", 1)
		}
		defer func() { _ = f.Close() }()
		input = f

		// Detect MIME type from file extension if auto.
		if cfg.inputType == "auto" {
			mimeType = detectMimeTypeFromFile(cfg.inputFile)
		}
	} else {
		// Read from stdin
		input = os.Stdin
		if cfg.inputType == "auto" {
			logger.Error("reading from stdin requires explicit input type with -t")
			return cli.Exit("", 1)
		}
	}

	// Determine MIME type from explicit flag.
	if cfg.inputType != "auto" {
		mimeType = mimeTypeFromString(cfg.inputType)
	}

	mimeType, input, err := resolveMimeAndInput(mimeType, input, cfg.inputType, cfg.inputFile, logger)
	if err != nil {
		return err
	}

	// Read the full input so converters can re-read as needed.
	data, err := io.ReadAll(input)
	if err != nil {
		logger.Error("reading input", "err", err)
		return cli.Exit("", 1)
	}

	result, err := convertOne(data, mimeType, opts, d)
	if err != nil {
		logger.Error("converting", "err", err)
		return cli.Exit("", 1)
	}

	return writeOutput(result, cfg.outputFile, logger)
}

// resolveMimeAndInput handles magic-number sniffing for the "image/*" family
// placeholder and for unknown extensions in auto mode. It returns the resolved
// MIME type and a fresh reader positioned at the start of the content.
func resolveMimeAndInput(
	mimeType string,
	input io.Reader,
	inputType, inputFile string,
	logger *slog.Logger,
) (string, io.Reader, error) {
	// "-t image" is a placeholder family; resolve the concrete subtype
	// by sniffing the content magic numbers.
	if mimeType == "image/*" {
		data, err := io.ReadAll(input)
		if err != nil {
			logger.Error("reading input", "err", err)
			return "", nil, cli.Exit("", 1)
		}
		mimeType = converter.DetectMimeType(data)
		if !strings.HasPrefix(mimeType, "image/") {
			logger.Error("input is not a recognized image", "detected", mimeType)
			return "", nil, cli.Exit("", 1)
		}
		return mimeType, bytes.NewReader(data), nil
	}

	if mimeType == "" && inputFile != "" {
		// Unknown extension in auto mode: sniff content magic numbers.
		data, err := io.ReadAll(input)
		if err != nil {
			logger.Error("reading input", "err", err)
			return "", nil, cli.Exit("", 1)
		}
		mimeType = converter.DetectMimeType(data)
		return mimeType, bytes.NewReader(data), nil
	}

	if mimeType == "" || mimeType == "application/octet-stream" {
		logger.Error("unsupported input type", "type", inputType, "mime", mimeType)
		return "", nil, cli.Exit("", 1)
	}

	return mimeType, input, nil
}

// levelFromVerbosity maps the CLI verbosity flags to an slog.Level.
//
//	-q/--quiet      -> LevelError (errors only)
//	(no flags)      -> LevelWarn  (errors + warnings, default)
//	-v              -> LevelInfo  (progress)
//	-vv             -> LevelDebug (details)
//	-vvv and above  -> trace (LevelDebug-8, dumps)
func levelFromVerbosity(quiet bool, vCount int) slog.Level {
	switch {
	case quiet:
		return slog.LevelError
	case vCount >= 3:
		return slog.LevelDebug - 8 // trace
	case vCount == 2:
		return slog.LevelDebug
	case vCount == 1:
		return slog.LevelInfo
	default:
		return slog.LevelWarn
	}
}

// detectMimeTypeFromFile detects MIME type from file extension.
func detectMimeTypeFromFile(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".html", ".htm":
		return "text/html"
	case ".pdf":
		return "application/pdf"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".bmp":
		return "image/bmp"
	default:
		return ""
	}
}

// mimeTypeFromString converts a string type to MIME type. "image" maps to the
// "image/*" family placeholder, which the caller resolves to a concrete subtype
// by sniffing content magic numbers.
func mimeTypeFromString(inputType string) string {
	switch strings.ToLower(inputType) {
	case "html":
		return "text/html"
	case "pdf":
		return "application/pdf"
	case "image":
		return "image/*"
	default:
		return ""
	}
}

// convertPDF handles PDF conversion with text/image detection.
func convertPDF(input io.Reader, opts converter.Options) (string, error) {
	// Read all data to allow multiple passes
	data, err := io.ReadAll(input)
	if err != nil {
		return "", fmt.Errorf("reading PDF: %w", err)
	}

	// Check if PDF has text
	inspector := pdf.NewInspector()
	hasText, err := inspector.HasText(strings.NewReader(string(data)))
	if err != nil {
		return "", fmt.Errorf("inspecting PDF: %w", err)
	}

	logger := opts.Logger
	logger.Info("pdf inspection", "has_text", hasText)

	if hasText {
		// Use text converter
		textConverter := converter.NewPDFTextConverter()
		return textConverter.Convert(strings.NewReader(string(data)), opts)
	}

	// Use image converter (with or without VLLM)
	imageConverter := converter.NewPDFImageConverter()
	return imageConverter.Convert(strings.NewReader(string(data)), opts)
}

// convertOne dispatches a single file's already-read data to the appropriate
// converter based on its MIME type. PDFs go through text/image routing; HTML
// and images resolve through the factory via ConvertWithType.
func convertOne(data []byte, mimeType string, opts converter.Options, d *doculai.Doculai) (string, error) {
	if mimeType == "application/pdf" {
		return convertPDF(bytes.NewReader(data), opts)
	}
	return d.ConvertWithType(bytes.NewReader(data), mimeType, opts)
}

// convertDirectory walks dir recursively, converts every recognized file, and
// merges the results into a single Markdown document. Files are processed
// sequentially in sorted relative-path order. Per-file results are prefixed
// with a "## File: <relpath>" header and joined with a horizontal rule.
// Unrecognized files are silently skipped (DEBUG log only). On the first
// conversion error the batch stops (fail-fast, matching OCR pipeline policy).
func convertDirectory(dir string, d *doculai.Doculai, opts converter.Options, logger *slog.Logger) (string, error) {
	logger.Info("batch directory", "dir", dir)

	var paths []string
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("walking directory: %w", err)
	}
	sort.Strings(paths)

	sections := make([]string, 0, len(paths))
	for _, path := range paths {
		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			rel = path
		}
		rel = filepath.ToSlash(rel)

		info, err := os.Stat(path)
		if err != nil {
			return "", fmt.Errorf("stat %s: %w", rel, err)
		}
		if info.Size() == 0 {
			logger.Debug("batch skip empty", "file", rel)
			continue
		}

		// Extension first; fall back to content sniffing for unknown extensions.
		mimeType := detectMimeTypeFromFile(path)
		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("reading %s: %w", rel, err)
		}
		if mimeType == "" {
			mimeType = converter.DetectMimeType(data)
		}

		if mimeType == "" || mimeType == "application/octet-stream" {
			logger.Debug("batch skip unrecognized", "file", rel, "mime", mimeType)
			continue
		}

		logger.Info("batch file", "file", rel, "mime", mimeType)

		converted, err := convertOne(data, mimeType, opts, d)
		if err != nil {
			return "", fmt.Errorf("converting %s: %w", rel, err)
		}

		sections = append(sections, fmt.Sprintf("## File: %s\n\n%s", rel, converted))
	}

	logger.Info("batch done", "files", len(sections))
	return strings.Join(sections, "\n\n---\n\n"), nil
}

// writeOutput writes the conversion result to the output file (when given) or
// to stdout, preserving clean stdout for piping.
func writeOutput(result, outputFile string, logger *slog.Logger) error {
	if outputFile != "" {
		if err := os.WriteFile(outputFile, []byte(result), 0o600); err != nil {
			logger.Error("writing output file", "err", err)
			return cli.Exit("", 1)
		}
		logger.Info("converted", "output", outputFile)
		return nil
	}
	fmt.Print(result)
	return nil
}
