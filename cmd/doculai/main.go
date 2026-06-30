package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/urfave/cli/v3"

	"doculai/internal/converter"
	"doculai/internal/pdf"
	"doculai/pkg/doculai"
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
		imageDir        string
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
				Usage:       "Input type: auto, html, pdf",
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
			&cli.StringFlag{
				Name:        "image-dir",
				Destination: &imageDir,
				Usage:       "Directory for saving images",
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
			if ec, ok := err.(cli.ExitCoder); ok {
				code = ec.ExitCode()
			}
			if msg := err.Error(); msg != "" {
				fmt.Fprintln(os.Stderr, msg)
			}
			os.Exit(code)
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			// Resolve verbosity conflict: -q/--quiet wins over -v.
			if quiet && vCount > 0 {
				fmt.Fprintln(os.Stderr, "Warning: -v ignored because -q/--quiet is set")
				vCount = 0
			}

			// Build the structured logger (text format to stderr only).
			level := levelFromVerbosity(quiet, vCount)
			logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))

			// Determine input source
			var input io.Reader
			var mimeType string

			if inputFile != "" {
				f, err := os.Open(inputFile)
				if err != nil {
					logger.Error("opening input file", "err", err)
					return cli.Exit("", 1)
				}
				defer f.Close()
				input = f

				// Detect MIME type from file extension if auto
				if inputType == "auto" {
					mimeType = detectMimeTypeFromFile(inputFile)
				}
			} else {
				// Read from stdin
				input = os.Stdin
				if inputType == "auto" {
					logger.Error("reading from stdin requires explicit input type with -t")
					return cli.Exit("", 1)
				}
			}

			// Determine MIME type from explicit flag
			if inputType != "auto" {
				mimeType = mimeTypeFromString(inputType)
			}

			if mimeType == "" {
				logger.Error("unsupported input type", "type", inputType)
				return cli.Exit("", 1)
			}

			// Create converter options (per-call values take priority over
			// instance-level options; CLI flags always populate these).
			opts := converter.Options{
				VLLMModel:       vllmModel,
				VLLMURL:         vllmURL,
				VLLMKey:         vllmKey,
				VLLMProvider:    vllmProvider,
				VLLMPrompt:      vllmPrompt,
				VLLMReasoning:   withReasoning,
				VLLMConcurrency: vllmConcurrency,
				Logger:          logger,
			}

			// Create doculai instance with VLLM server/model/key/provider at the
			// instance level (acts as fallback for per-call Options).
			d := doculai.New(
				doculai.WithLogger(logger),
				doculai.WithVLLMServer(vllmURL),
				doculai.WithVLLMModel(vllmModel),
				doculai.WithVLLMKey(vllmKey),
				doculai.WithVLLMProvider(vllmProvider),
				doculai.WithVLLMConcurrency(vllmConcurrency),
			)

			// Convert
			var result string
			var err error

			if mimeType == "application/pdf" {
				// For PDF, we need to check if it has text or is image-based
				result, err = convertPDF(input, opts, imageDir)
			} else {
				result, err = d.ConvertWithType(input, mimeType, opts)
			}

			if err != nil {
				logger.Error("converting", "err", err)
				return cli.Exit("", 1)
			}

			// Write output
			if outputFile != "" {
				if err := os.WriteFile(outputFile, []byte(result), 0644); err != nil {
					logger.Error("writing output file", "err", err)
					return cli.Exit("", 1)
				}
				logger.Info("converted", "output", outputFile)
			} else {
				fmt.Print(result)
			}
			return nil
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		// ExitErrHandler above handles exit codes; this is a defensive backstop.
		os.Exit(1)
	}
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
	default:
		return ""
	}
}

// mimeTypeFromString converts a string type to MIME type.
func mimeTypeFromString(inputType string) string {
	switch strings.ToLower(inputType) {
	case "html":
		return "text/html"
	case "pdf":
		return "application/pdf"
	default:
		return ""
	}
}

// convertPDF handles PDF conversion with text/image detection.
func convertPDF(input io.Reader, opts converter.Options, imageDir string) (string, error) {
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
