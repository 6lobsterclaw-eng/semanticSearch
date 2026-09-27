# Semantic Search App

A desktop application for semantic search of PDF and Markdown files.

## Features

- **Fully offline** - Uses local GGUF model for embeddings (no API, no internet needed after download)
- **Vector search** - Powered by kelindar/search
- **Desktop UI** - Native Windows GUI using Walk

## Tech Stack

- **Go** - Language
- **kelindar/search/llama** - Local GGUF embeddings
- **kelindar/search** - Vector index
- **walk** - Native Windows GUI
- **SQLite** - Document storage

## GGUF Model

You need to download a GGUF embedding model. Recommended:

- **[BX其次-ai/MiniLM-L6-gguf](https://huggingface.co/BX次ai/MiniLM-L6-gguf)** - Small, fast
- Or search HuggingFace for "embedding gguf" models

Place the `.gguf` file in a folder, e.g., `model/`.

## Build

### Prerequisites

- Go 1.21+
- Windows 10/11 with MinGW-w64

### Build Commands

```bash
# Install dependencies
go mod tidy

# Build for Windows
GOOS=windows GOARCH=amd64 CGO_ENABLED=1 go build -o search.exe ./cmd/search-app/

# Or use Makefile
make build
```

## Usage

1. Run `search.exe`
2. Click "Select Model" to choose your GGUF model file
3. Click "Select Folder" to choose a directory with PDF/MD files
4. Type a query in the search box
5. Results show with relevance scores

## Development

```bash
# Run tests
go test ./...

# Run locally (Windows only)
go run ./cmd/search-app/
```

## Files

- `internal/indexer/` - Text extraction (PDF, MD)
- `internal/search/` - Search logic
- `ui/` - Walk desktop UI
