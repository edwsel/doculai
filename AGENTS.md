# AGENTS.md

## Архитектура

Пакет `doculai` конвертирует входные данные (HTML, PDF с текстом, PDF с изображениями) в Markdown.

### Слои

```
doculai/
├── cmd/doculai/              # CLI приложение
├── internal/
│   ├── converter/            # Основной движок конвертации
│   │   ├── converter.go    # Интерфейс и фабрика
│   │   ├── html.go         # HTML -> Markdown
│   │   ├── pdf_text.go     # PDF (текст) -> Markdown
│   │   ├── pdf_image.go    # PDF (изображения) -> Markdown через VLLM
│   │   └── image.go        # Standalone image -> Markdown через VLLM
│   ├── pdf/
│   │   ├── extractor.go    # Извлечение текста/изображений из PDF
│   │   └── inspector.go    # Проверка наличия текста в PDF
│   ├── image/
│   │   ├── normalize.go    # Нормализация изображений (resize, optimize)
│   │   └── format.go       # Подготовка к VLLM формату
│   ├── vllm/
│   │   ├── client.go       # OpenAI-compatible + Ollama клиент
│   │   └── provider.go     # Абстракция провайдера
│   └── structure/
│       ├── detector.go     # Детекция структуры PDF (headings, lists, tables)
│       └── formatter.go    # Форматирование в Markdown
├── pkg/
│   └── doculai/            # Публичное API
└── test/
    ├── fixtures/           # Тестовые данные
    ├── integration/        # Интеграционные тесты
    └── mock/               # Mock VLLM сервер
```

### Поток данных

```
Входной файл (или директория)
    ↓
-i <директория>? → WalkDir (отсортированно) → пофайлово:
    │   расширение → если неизвестно, sniff контента (converter.DetectMimeType)
    │   PDF → convertPDF (текст/изображение)
    │   HTML / image → d.ConvertWithType (фабрика)
    │   нераспознанное → молча пропустить (DEBUG)
    │   объединение секций `## File: <relpath>` через `\n\n---\n\n`
    ↓
Определение типа (HTML/PDF/image)
    ↓
┌─────────────────┐
│      HTML       │
│  -> html-to-md  │
│  -> Markdown    │
└─────────────────┘
    ↓
┌─────────────────┐
│       PDF       │
│  -> Проверка    │
│    текста       │
└─────────────────┘
    ↓
┌─────────────────┐    ┌─────────────────┐
│  Текст есть     │    │  Текста нет     │
│  -> Извлечь     │    │  -> Извлечь     │
│    текст с      │    │    изображения  │
│    форматом     │    │  -> Нормализовать│
│  -> Детектировать│    │    изображения  │
│    структуру    │    │  -> VLLM OCR    │
│  -> Markdown    │    │  -> Markdown    │
│    (hierarchy,  │    │                 │
│     lists,      │    │                 │
│     tables)     │    │                 │
└─────────────────┘    └─────────────────┘
    ↓
┌─────────────────┐
│     Image       │
│ (png/jpg/gif/   │
│  webp/bmp)      │
│  -> нет VLLM?   │  -> ERROR (OCR обязателен)
│  -> Нормализовать│
│  -> VLLM OCR    │
│  -> Markdown    │
└─────────────────┘
```

## Интерфейсы

### Converter

```go
type Converter interface {
    Convert(input io.Reader, opts Options) (string, error)
    Supports(mimeType string) bool
}
```

### VLLMProvider

```go
type Provider interface {
    BuildRequest(imageBase64 string, opts Options) ([]byte, error)
    ParseResponse(data []byte) (string, error)
    GetURL() string
    GetHeaders() map[string]string
    IsConfigured() bool
}
```

### StructureDetector

```go
type Detector struct{}

func (d *Detector) Detect(elements []pdf.TextElement) []StructuredElement
```

## Добавление нового конвертера

1. Создать файл `internal/converter/newformat.go`
2. Реализовать интерфейс `Converter`
3. Зарегистрировать в фабрике (`pkg/doculai/doculai.go`)
4. Добавить тесты
5. Обновить AGENTS.md

## VLLM Configuration

### System Prompt

Базовый системный промпт находится в `internal/vllm/prompt.go`:

```go
const DefaultSystemPrompt = `You are a document OCR assistant...`
```

**Переопределение промпта:**

```go
opts := converter.Options{
    VLLMModel:     "gpt-4o",
    VLLMURL:       "https://api.openai.com/v1",
    VLLMKey:       os.Getenv("OPENAI_API_KEY"),
    VLLMProvider:  "openai",
    VLLMPrompt:    "Ваш кастомный промпт...", // Переопределяет DefaultSystemPrompt
    VLLMReasoning: false,                     // По умолчанию отключён; true для reasoning-моделей
}
```

### OpenAI-compatible endpoints

```go
opts := converter.Options{
    VLLMModel:    "gpt-4o",
    VLLMURL:      "https://api.openai.com/v1",
    VLLMKey:      os.Getenv("OPENAI_API_KEY"),
    VLLMProvider: "openai",
}
```

### Ollama endpoints

```go
opts := converter.Options{
    VLLMModel:    "llava",
    VLLMURL:      "http://localhost:11434",
    VLLMProvider: "ollama",
}
```

### Environment variables

- `OPENAI_API_KEY` - API ключ для OpenAI
- `OLLAMA_HOST` - URL для Ollama (по умолчанию: http://localhost:11434)

### Graceful degradation

Если VLLM не настроен:
- PDF с текстом: полная конвертация
- PDF с изображениями: извлечение метаданных + плейсхолдеры

### Parallel OCR & retry

OCR страниц PDF-скана (`internal/converter/pdf_image.go`, `ocrParallel`) идёт
**параллельно** с ограничением concurrency:

- Конфигурация: `converter.Options.VLLMConcurrency` (API/CLI), по умолчанию
  `converter.DefaultVLLMConcurrency = 5`. В `pkg/doculai` — опция
  `WithVLLMConcurrency(n)` (инстанс-уровень, применяется если в вызове `0`).
- CLI: `--vllm-concurrency N` (int, по умолчанию 5).
- Реализация: `golang.org/x/sync/errgroup` (`errgroup.WithContext` + `g.SetLimit`),
  результаты складываются в pre-sized slice **по индексу страницы** → итог всегда
  в порядке страниц независимо от времени завершения.
- Retry на каждую страницу: `cenkalti/backoff/v4`, экспоненциальный backoff,
  `backoff.WithMaxRetries(NewExponentialBackOff(), 4)` (до 5 попыток),
  `MaxElapsedTime = 30s`. Каждая попытка-повтор логируется на `WARN`
  (`ocr retry`). `context.Canceled`/`DeadlineExceeded` (fail-fast от errgroup)
  **не ретраятся** — `backoff.WithContext` останавливает повторы.
- Политика ошибок: **fail-fast** — первый финальный сбой (после исчерпания retry)
  отменяет in-flight запросы через ctx, ошибка оборачивается `ocr failed: ...`,
  процесс выходит с ненулевым кодом.

## PDF Structure Detection

### Headings

Детекция по размеру шрифта (относительно среднего):
- > 1.3x среднего → Heading 1
- > 1.15x среднего → Heading 2
- > 1.05x или bold → Heading 3

### Lists

- Маркированные: `-`, `•`, `*`, `◦`, `▪`
- Нумерованные: `1.`, `2.`, `a)`, `b)`
- Вложенность по отступам (X координата)

### Tables

- Детекция по выравниванию по столбцам
- Минимум 2 строки, 2 колонки
- Разделение по X координатам

### Images

- Извлечение встроенных изображений и рендер страниц через `go-pdfium` (`internal/pdf/extractor.go`: `ExtractImages`, `ExtractImagesAsPages`)
- Сканы: рендер страницы целиком через `RenderPageInDPI` (DPI=200) → PNG
- Сохранение в директорию или base64
- Плейсхолдеры: `[Image: page N]`

## Logging

Весь лог выводится в **stderr** (формат `log/slog` text). stdout остаётся чистым для Markdown, поэтому пайпы `> out.md` работают корректно.

### CLI — вербосность

| Флаг | slog.Level | Что пишется |
|---|---|---|
| `--quiet` / `-q` | `LevelError` | только ошибки |
| (без флага, дефолт) | `LevelWarn` | ошибки + предупреждения |
| `-v` | `LevelInfo` | прогресс: MIME-тип, выбор конвертера, наличие текста в PDF, число страниц/изображений, `ocr parallel pages=N concurrency=M`, `ocr done page=N` |
| `-vv` | `LevelDebug` | детали: рендер страниц (DPI/страница), размеры изображений, HTTP-запрос к VLLM (модель/URL/размер payload), длительность ответа |
| `-vvv` и выше | trace (`LevelDebug-8`) | дамп: префиксы ответов (до ~512 байт) |

- `-v` повторяемый (`-vv`, `-vvv`) — реализован через `cli.BoolFlag` со счётчиком (`cli.BoolConfig{Count: &vCount}`) + `UseShortOptionHandling: true` (бандлинг `-vv`→`-v -v` средствами `urfave/cli/v3`).
- `-q` / `--quiet` — алиасы на один BoolFlag (`quiet`).
- Конфликт `-q` + `-v`: `-q` имеет приоритет, `-v` игнорируется (warning в stderr).

```bash
# По умолчанию (WARN) — тихо
doculai -i document.pdf -o output.md

# Прогресс конвертации в stderr
doculai -i document.pdf -o output.md -v

# Полный отладочный дамп
doculai -i scan.pdf -o output.md -vvv --vllm-model gpt-4o ...
```

### API — инъекция логгера

По умолчанию пакет **молчит** (discard-логгер). Логгер передаётся через functional option:

```go
import (
    "log/slog"
    "os"
    "github.com/edwsel/doculai/pkg/doculai"
)

logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
d := doculai.New(doculai.WithLogger(logger))
```

- `doculai.New()` без аргументов обратно совместим (возвращает инстанс с discard-логгером).
- Приоритет: если в `converter.Options.Logger` на конкретном вызове передан логгер — он побеждает над инстансным (`if opts.Logger == nil { opts.Logger = d.logger }`).
- Логгер пробрасывается в конвертеры через поле `converter.Options.Logger` и далее в `vllm.Options.Logger` (`pdf_image.go` копирует его в `vllmOpts`). Хелперы `Options.logger()` возвращают discard-логгер при `nil`, поэтому nil-чеки в коде не нужны.
- Уровень trace в пакете `vllm`: `LevelTrace = slog.LevelDebug - 8`. Перед тяжёлым форматированием (дамп ответов) проверяется `logger.Enabled(ctx, LevelTrace)`.

## Тестирование

### Запуск тестов

```bash
# Все тесты
go test ./...

# С покрытием
go test -cover ./...

# Интеграционные тесты
go test ./test/integration/...

# Конкретный пакет
go test ./internal/converter/...
```

### Линтинг (golangci-lint)

Конфигурация: `.golangci.yml` (best practices: errcheck, govet, staticcheck,
revive, gosec, gocritic, errorlint, exhaustive, prealloc и др.). Тестовые файлы
ослаблены (gosec/dupl/gocritic/prealloc/errcheck/unparam/gocyclo отключены для
`*_test.go` и `test/`).

```bash
# Полная проверка
golangci-lint run ./...

# Авто-исправление (форматирование, goimports, gofumpt)
golangci-lint run --fix ./...
```

```

### Структура тестов

```
test/
├── fixtures/
│   ├── html/           # HTML тестовые файлы
│   ├── pdf-text/       # PDF с текстом
│   ├── pdf-image/      # PDF-сканы
│   └── pdf-structure/  # PDF со сложной структурой
├── integration/        # Интеграционные тесты
│   └── conversion_test.go
└── mock/
    └── vllm_server.go  # Mock VLLM сервер
```

### Mock VLLM сервер

```go
import "github.com/edwsel/doculai/test/mock"

server := mock.MockVLLMServer()
defer server.Close()

// Используйте server.URL как VLLM URL
```

## Примеры использования

### API

```go
package main

import (
    "os"
    "github.com/edwsel/doculai/pkg/doculai"
)

func main() {
    converter := doculai.New()
    
    input, _ := os.Open("document.html")
    markdown, err := converter.Convert(input, doculai.Options{})
    if err != nil {
        panic(err)
    }
    
    os.WriteFile("output.md", []byte(markdown), 0644)
}
```

### CLI

```bash
# HTML -> Markdown
doculai -i input.html -o output.md

# PDF с текстом -> Markdown
doculai -i document.pdf -o output.md

# PDF с изображениями -> Markdown (с VLLM)
doculai -i scan.pdf -o output.md \
  --vllm-model gpt-4o \
  --vllm-url https://api.openai.com/v1 \
  --vllm-key $OPENAI_KEY \
  --vllm-provider openai

# PDF с reasoning-моделью (o3, gpt-5 и т.п.) — включить reasoning
doculai -i scan.pdf -o output.md \
  --vllm-model o3 \
  --vllm-url https://api.openai.com/v1 \
  --vllm-key $OPENAI_KEY \
  --vllm-provider openai \
  --with-reasoning

# Ограничить параллелизм OCR (по умолчанию 5)
doculai -i scan.pdf -o output.md \
  --vllm-model gpt-4o \
  --vllm-url https://api.openai.com/v1 \
  --vllm-key $OPENAI_KEY \
  --vllm-provider openai \
  --vllm-concurrency 2

# Изображение -> Markdown (PNG/JPEG/GIF/WEBP/BMP, через VLLM OCR)
doculai -i photo.png -o output.md \
  --vllm-model gpt-4o \
  --vllm-url https://api.openai.com/v1 \
  --vllm-key $OPENAI_KEY \
  --vllm-provider openai

# Директория -> Markdown (рекурсивный обход, отсортированно)
# распознанные файлы (html/pdf/image) объединяются в один документ,
# нераспознанные молча пропускаются; секции вида `## File: <relpath>`
doculai -i docs/ -o output.md \
  --vllm-model gpt-4o \
  --vllm-url https://api.openai.com/v1 \
  --vllm-key $OPENAI_KEY \
  --vllm-provider openai -v

# Чтение из stdin
cat input.html | doculai -t html > output.md

# Изображение из stdin (подтип определяется по сигнатуре контента)
cat photo.png | doculai -t image --vllm-model gpt-4o ... > output.md
```

## Расширение

### Добавление нового формата

1. Создать файл `internal/converter/newformat.go`
2. Реализовать интерфейс `Converter`
3. Зарегистрировать в `pkg/doculai/doculai.go`
4. Добавить тесты
5. Обновить документацию

### Добавление нового VLLM провайдера

1. Создать файл `internal/vllm/newprovider.go`
2. Реализовать интерфейс `Provider`
3. Зарегистрировать в `ProviderFactory`
4. Добавить тесты

## Лицензия

Apache-2.0 (см. `LICENSE.md`)
