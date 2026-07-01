<div align="center">

# doculai

**Convert HTML and PDF documents to clean Markdown — built for AI services and agentic applications.**

A fast, dependency-light document-to-Markdown engine for Go — an open analog of
Microsoft MarkItDown. Handles real-world PDFs (text layer **and** scanned
images), reconstructs document structure, and optionally OCRs pages with
Vision Language Models through any OpenAI-compatible or Ollama endpoint.

Designed as a **document preprocessing layer** for LLM pipelines: RAG systems,
AI agents, tool-use chains, chatbots, and any service that needs reliable
Markdown from heterogeneous input formats.

[Features](#features) •
[Use with AI](#use-with-ai) •
[Quick start](#quick-start) •
[Installation](#installation) •
[CLI](#cli-usage) •
[Go API](#go-api) •
[VLLM](#vllm-configuration) •
[Architecture](#architecture) •
[Contributing](#contributing)

</div>

---

## Features

- 🧾 **HTML → Markdown** — headings, lists, tables, links, and images via
  `html-to-markdown`.
- 📄 **PDF (text layer) → Markdown** — font-size-based heading detection,
  ordered/unordered lists with nesting, and table reconstruction from column
  alignment.
- 🖼️ **PDF (scanned) → Markdown** — page rendering with `go-pdfium` (WASM via
  `wazero`) plus optional OCR through any VLLM endpoint.
- 📷 **Images → Markdown** — standalone PNG, JPEG, GIF, WEBP, and BMP files
  are OCR'd straight to Markdown via a VLLM.
- 📁 **Directory batch** — point `-i` at a directory to convert every
  recognized file recursively (HTML / PDF / image), merged into one document
  with `## File: <relpath>` section headers.
- ⚡ **Parallel OCR with retry** — concurrent page processing with bounded
  concurrency, exponential backoff retries, and ordered output.
- 🔌 **Provider agnostic** — works with OpenAI, OpenAI-compatible gateways,
  and local Ollama instances.
- 🪶 **Graceful degradation** — text PDFs convert fully without a VLLM; image
  PDFs produce metadata + placeholders when no model is configured.
- 🎛️ **CLI & library** — ship it as a binary, or embed it in any Go program.
- 🔇 **Pipe-friendly logging** — all logs go to **stderr**; **stdout** always
  emits clean Markdown so `doculai ... > out.md` just works.

## Use with AI

`doculai` converts documents to **clean, structured Markdown** — the format
LLMs consume best. Embed it in any Go service or call the CLI from your pipeline.

### RAG (Retrieval-Augmented Generation)

```go
// Ingest a user-uploaded PDF into a RAG pipeline
d := doculai.New(doculai.WithLogger(logger))

markdown, err := d.Convert(file, doculai.Options{})
if err != nil {
    return err
}

// Chunk and embed the Markdown for your vector store
chunks := chunker.Split(markdown, 512)
embeddings := embed(chunks)
store.Upsert(embeddings)
```

### Agentic tools (MCP / function-calling)

Give your AI agent a "read document" tool powered by `doculai`:

```go
// Agent tool handler
func readDocument(input io.Reader) (string, error) {
    d := doculai.New()
    return d.Convert(input, doculai.Options{})
}
// The agent receives clean Markdown it can reason over
```

Works with any agent framework — OpenAI function-calling, Anthropic tools,
LangChain, AutoGen, or custom MCP servers.

### Batch document processing

```bash
# Preprocess a directory of documents for a knowledge base
find ./docs -name "*.pdf" -o -name "*.html" | while read f; do
  doculai -i "$f" -o "./processed/$(basename "$f" .pdf).md" -v
done
```

### Why Markdown for LLMs?

- **Structure preservation** — headings, lists, and tables map directly to
  tokens the model understands, reducing hallucination.
- **Consistent formatting** — every input type (HTML, text PDF, scanned PDF)
  becomes a single predictable format.
- **No binary artifacts** — pure text, easy to chunk, embed, and pass in
  prompts or tool responses.

---

## Quick start

```bash
# Build
go build -o doculai ./cmd/doculai

# HTML or text-based PDF
doculai -i document.pdf -o output.md

# Scanned PDF via an OpenAI-compatible VLM
doculai -i scan.pdf -o output.md \
  --vllm-model gpt-4o \
  --vllm-url  https://api.openai.com/v1 \
  --vllm-key  "$OPENAI_API_KEY" \
  --vllm-provider openai

# Or stream through pipes
cat input.html | doculai -t html > output.md
```

## Installation

### From source

```bash
git clone https://github.com/edwsel/doculai.git
cd doculai
go build -o doculai ./cmd/doculai
```

### As a Go dependency

```bash
go get github.com/edwsel/doculai
```

> **Note:** PDF support uses [`go-pdfium`](https://github.com/klippa-app/go-pdfium),
> which embeds the Pdfium WASM binary via [`wazero`](https://wazero.io/) — no
> native libraries or external processes required.

## CLI usage

```text
doculai — Convert HTML or PDF files to Markdown.

Usage:
  doculai [options]

Examples:
  doculai -i input.html -o output.md
  doculai -i document.pdf -o output.md
  doculai -i scan.pdf -o output.md --vllm-model gpt-4o --vllm-provider openai ...
  cat input.html | doculai -t html > output.md
```

### Flags

| Flag                              | Default   | Description                                                                 |
| --------------------------------- | --------- | --------------------------------------------------------------------------- |
| `-i, --input`                     | _stdin_   | Input file **or directory**. A directory is walked recursively. Reads from stdin if omitted. |
| `-o, --output`                    | _stdout_  | Output file. Writes to stdout if omitted.                                   |
| `-t, --type`                      | `auto`    | Input type: `auto`, `html`, `pdf`, `image`. stdin requires an explicit type. `image` sniffs the subtype from content. |
| `--vllm-model`                    | —         | VLLM model name (e.g. `gpt-4o`, `llava`).                                   |
| `--vllm-url`                      | —         | VLLM API URL (e.g. `https://api.openai.com/v1`).                            |
| `--vllm-key`                      | —         | VLLM API key.                                                               |
| `--vllm-provider`                 | `openai`  | Provider: `openai` or `ollama`.                                             |
| `--vllm-prompt`                   | —         | Custom system prompt (overrides the built-in OCR prompt).                   |
| `--with-reasoning`                | _off_     | Enable reasoning for reasoning-capable models (`o3`, `gpt-5`, …).           |
| `--vllm-concurrency`              | `5`       | Max parallel VLLM page OCR requests.                                        |
| `--image-dir`                     | —         | Directory for saving extracted images.                                      |
| `-q, --quiet`                     | _off_     | Log only errors to stderr.                                                  |
| `-v`                              | _off_     | Increase verbosity (repeatable: `-v` info, `-vv` debug, `-vvv` trace).      |

> `-q` and `-v` are mutually exclusive; if both are passed, `-q` wins and a
> warning is printed to stderr.

### Logging

All logs are emitted to **stderr** as structured `log/slog` text, leaving
**stdout** clean for Markdown output.

| Verbosity | Level               | What you see                                                                 |
| --------- | ------------------- | ---------------------------------------------------------------------------- |
| `-q`      | `LevelError`        | Errors only.                                                                 |
| _(none)_  | `LevelWarn`         | Errors and warnings.                                                         |
| `-v`      | `LevelInfo`         | Progress: MIME type, converter choice, text presence, page/image counts.    |
| `-vv`     | `LevelDebug`        | Details: page rendering, image sizes, HTTP request summary, response times. |
| `-vvv+`   | trace (`Debug-8`)   | Dumps: response prefix bytes (up to ~512 B).                                 |

### Examples

```bash
# Reasoning-capable model
doculai -i scan.pdf -o output.md \
  --vllm-model o3 \
  --vllm-url https://api.openai.com/v1 \
  --vllm-key "$OPENAI_API_KEY" \
  --vllm-provider openai \
  --with-reasoning

# Local Ollama VLM
ollama run llava
doculai -i scan.pdf -o output.md \
  --vllm-model llava \
  --vllm-url http://localhost:11434 \
  --vllm-provider ollama

# Throttle OCR concurrency
doculai -i scan.pdf -o output.md \
  --vllm-model gpt-4o --vllm-provider openai \
  --vllm-url https://api.openai.com/v1 --vllm-key "$OPENAI_API_KEY" \
  --vllm-concurrency 2

# Image → Markdown (standalone PNG/JPEG/GIF/WEBP/BMP via OCR)
doculai -i photo.png -o output.md \
  --vllm-model gpt-4o --vllm-provider openai \
  --vllm-url https://api.openai.com/v1 --vllm-key "$OPENAI_API_KEY"

# Directory batch — recursive, sorted, merged with "## File: <relpath>" headers
doculai -i docs/ -o output.md \
  --vllm-model gpt-4o --vllm-provider openai \
  --vllm-url https://api.openai.com/v1 --vllm-key "$OPENAI_API_KEY" -v
```

## Go API

The public API lives in [`pkg/doculai`](pkg/doculai). It is silent by default
and fully configurable through functional options.

### Minimal

```go
package main

import (
    "os"

    "github.com/edwsel/doculai/pkg/doculai"
)

func main() {
    d := doculai.New()

    input, err := os.Open("document.html")
    if err != nil {
        panic(err)
    }
    defer input.Close()

    markdown, err := d.Convert(input, doculai.Options{})
    if err != nil {
        panic(err)
    }

    if err := os.WriteFile("output.md", []byte(markdown), 0o644); err != nil {
        panic(err)
    }
}
```

### With structured logging

The library ships with a discard logger; inject your own to get progress output.

```go
logger := slog.New(slog.NewTextHandler(os.Stderr,
    &slog.HandlerOptions{Level: slog.LevelInfo}))

d := doculai.New(doculai.WithLogger(logger))
```

### With an explicit input type

```go
markdown, err := d.ConvertWithType(input, "application/pdf", doculai.Options{})
```

### Functional options

Instance-level options act as **fallbacks**: per-call `Options` always win, and
empty/zero values fall back to the instance configuration.

| Option                       | Sets                          |
| ---------------------------- | ----------------------------- |
| `WithLogger(l)`              | Structured logger.            |
| `WithVLLMServer(url)`        | API endpoint URL.             |
| `WithVLLMModel(model)`       | Model name.                   |
| `WithVLLMKey(key)`           | API key.                      |
| `WithVLLMProvider(provider)` | `openai` or `ollama`.         |
| `WithVLLMConcurrency(n)`     | Max parallel page OCR requests. |

```go
d := doculai.New(
    doculai.WithLogger(logger),
    doculai.WithVLLMServer("https://api.openai.com/v1"),
    doculai.WithVLLMModel("gpt-4o"),
    doculai.WithVLLMKey(os.Getenv("OPENAI_API_KEY")),
    doculai.WithVLLMProvider("openai"),
    doculai.WithVLLMConcurrency(4),
)
```

### `converter.Options` (per-call)

```go
type Options struct {
    VLLMModel       string       // Model name (optional)
    VLLMURL         string       // Provider URL (optional)
    VLLMKey         string       // API key (optional)
    VLLMProvider    string       // "openai" or "ollama"
    VLLMPrompt      string       // Custom system prompt (overrides default)
    VLLMReasoning   bool         // Enable reasoning for capable models
    VLLMConcurrency int          // Parallel OCR requests (<=0 → default)
    Logger          *slog.Logger // Per-call logger (overrides instance)
}
```

## VLLM configuration

`doculai` supports two provider styles through a single abstraction.

### OpenAI-compatible providers

Works with OpenAI, Azure OpenAI, and any server exposing the OpenAI Chat
Completions API.

```bash
export OPENAI_API_KEY="sk-..."

doculai -i scan.pdf -o output.md \
  --vllm-model gpt-4o \
  --vllm-url https://api.openai.com/v1 \
  --vllm-key "$OPENAI_API_KEY" \
  --vllm-provider openai
```

Tested with: `gpt-4o`, `gpt-4-vision-preview`, `claude-3-opus` (via gateway),
reasoning models `o3` / `gpt-5` (use `--with-reasoning`).

### Ollama (local)

```bash
ollama run llava

doculai -i scan.pdf -o output.md \
  --vllm-model llava \
  --vllm-url http://localhost:11434 \
  --vllm-provider ollama
```

Supported local models include `llava`, `bakllava`, `llava-phi3`.

### Custom system prompt

```go
opts := doculai.Options{
    VLLMProvider: "openai",
    VLLMPrompt:   "Your custom OCR instruction…",
}
```

```bash
doculai -i scan.pdf -o output.md \
  --vllm-prompt "Extract text preserving table layout exactly" ...
```

### Parallel OCR and retry

Scanned PDFs are processed in parallel with deterministic ordering:

- **Concurrency:** `--vllm-concurrency N` (default `5`), bounded by
  `errgroup.SetLimit`. Results are written to a pre-sized slice **by page
  index**, so output order is preserved regardless of completion timing.
- **Retry:** exponential backoff per page via `cenkalti/backoff/v4`,
  `WithMaxRetries(NewExponentialBackOff(), 4)` (up to 5 attempts),
  `MaxElapsedTime = 30s`. Retries are logged at `WARN`.
- **Fail-fast:** the first terminal failure cancels in-flight requests through
  the shared context; `context.Canceled` / `DeadlineExceeded` are **not**
  retried. The CLI exits with a non-zero status and an `ocr failed: ...` error.

### Graceful degradation (no VLLM)

When no VLLM is configured:

- **Text PDFs** — full conversion with structure detection.
- **Image PDFs** — page metadata plus `[Image: page N]` placeholders.
- **Standalone images** — no fallback: OCR is the only extraction path, so the
  conversion (and the CLI) exits with an error.

## Environment variables

| Variable         | Description                                  | Default                       |
| ---------------- | -------------------------------------------- | ----------------------------- |
| `OPENAI_API_KEY` | API key for OpenAI-compatible providers.     | —                             |
| `OLLAMA_HOST`    | URL for Ollama (used by the `ollama` client). | `http://localhost:11434`      |

## Architecture

```
doculai/
├── cmd/doculai/              # CLI application
├── internal/
│   ├── converter/            # Conversion engine
│   │   ├── converter.go    # Converter interface + factory
│   │   ├── html.go         # HTML → Markdown
│   │   ├── pdf_text.go     # PDF (text) → Markdown
│   │   ├── pdf_image.go    # PDF (images) → Markdown via VLLM
│   │   └── image.go        # Image → Markdown via VLLM OCR
│   ├── pdf/
│   │   ├── extractor.go    # Extract text/images from PDF
│   │   └── inspector.go    # Detect whether a PDF has a text layer
│   ├── image/
│   │   ├── normalize.go    # Image normalization (resize, optimize)
│   │   └── format.go       # Prepare images for VLLM input
│   ├── vllm/
│   │   ├── client.go       # OpenAI-compatible + Ollama client
│   │   └── provider.go     # Provider abstraction
│   └── structure/
│       ├── detector.go     # Detect headings, lists, tables
│       └── formatter.go    # Emit Markdown
├── pkg/
│   └── doculai/            # Public API
└── test/
    ├── fixtures/           # Test data
    ├── integration/        # Integration tests
    └── mock/               # Mock VLLM server
```

### Data flow

```
Input file or directory (-i)
    │   (directory → recursive walk, sorted; merged with "## File: <relpath>"
    │    sections joined by horizontal rules; unrecognized files are skipped)
    │
    ▼
Detect type (HTML / PDF / image)
    │
    ├── HTML ───────────────────────► html-to-markdown ─► Markdown
    │
    ├── Image ─► Normalize ─► VLLM OCR ─► Markdown (no VLLM → error)
    │
    └── PDF ──► Inspect text layer
                  │
                  ├── Has text ─► Extract text + structure ─► Markdown
                  │                  (headings, lists, tables)
                  │
                  └── No text ──► Render pages ─► Normalize ─► VLLM OCR ─► Markdown
```

### PDF structure detection

- **Headings** — by font size relative to the page average:
  - `> 1.30×` → H1
  - `> 1.15×` → H2
  - `> 1.05×` or bold → H3
- **Lists**
  - Unordered: `-`, `•`, `*`, `◦`, `▪`
  - Ordered: `1.`, `2.`, `a)`, `b)`
  - Nesting resolved by horizontal X coordinate
- **Tables** — detected by column alignment; requires ≥ 2 rows and ≥ 2 columns.
- **Images** — embedded images plus full-page renders at DPI 200 (`RenderPageInDPI`)
  for scans; emitted as files (via `--image-dir`) or base64.

## Testing

```bash
# All tests
go test ./...

# With coverage
go test -cover ./...

# Integration tests
go test ./test/integration/...

# A specific package
go test ./internal/converter/...
```

### Mock VLLM server

```go
import "github.com/edwsel/doculai/test/mock"

server := mock.MockVLLMServer()
defer server.Close()
// Use server.URL as the VLLM URL.
```

## Dependencies

| Module                                                | Purpose                                            |
| ----------------------------------------------------- | -------------------------------------------------- |
| `github.com/JohannesKaufmann/html-to-markdown/v2`     | HTML → Markdown conversion.                        |
| `github.com/klippa-app/go-pdfium`                     | PDF text/image extraction and page rendering (WASM). |
| `golang.org/x/image`                                  | Image processing and normalization.                |
| `github.com/sashabaranov/go-openai`                   | OpenAI-compatible client.                          |
| `github.com/cenkalti/backoff/v4`                      | Exponential backoff retries for OCR.               |
| `golang.org/x/sync`                                   | Bounded concurrency via `errgroup`.                |
| `github.com/urfave/cli/v3`                            | CLI framework.                                     |

## Extending

### Adding a new format

1. Create `internal/converter/newformat.go` implementing the `Converter`
   interface.
2. Register it in the factory (`pkg/doculai/doculai.go`).
3. Add tests.
4. Update this README.

### Adding a new VLLM provider

1. Create `internal/vllm/newprovider.go` implementing the `Provider` interface.
2. Register it in `ProviderFactory`.
3. Add tests.

## Contributing

1. Fork the repository and create a feature branch.
2. Add tests for your changes.
3. Ensure everything passes: `go test ./...`.
4. Open a pull request describing the change.

## License

Apache-2.0. See [LICENSE.md](LICENSE.md).
