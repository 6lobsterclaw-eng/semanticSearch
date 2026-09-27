# Semantic Search App

A desktop application for semantic search of PDF and Markdown files.

## Features

- **Local embeddings** - Uses rembed for offline embedding generation (no API needed)
- **Vector search** - Powered by kelindar/search
- **Desktop UI** - Native GUI using Fyne

## Tech Stack

- **Go** - Language
- **rembed** - Local embedding (pure Go, no CGO)
- **kelindar/search** - Vector index
- **Fyne** - Desktop UI
- **SQLite** - Document storage

## Build

### Prerequisites

- Go 1.21+
- Windows 10/11

### Build Commands

```bash
# Install dependencies
go mod tidy

# Build for Windows
go build -o search.exe ./cmd/search-app/

# Or use Makefile
make build
```

## Usage

1. Run `search.exe`
2. Click "Select Folder" to choose a directory with PDF/MD files
3. Type a query in the search box
4. Results show with relevance scores

## Development

```bash
# Run tests
go test ./...

# Run locally
go run ./cmd/search-app/
```

## Files

- `internal/indexer/` - Text extraction (PDF, MD)
- `internal/search/` - Search logic
- `ui/` - Fyne desktop UI
