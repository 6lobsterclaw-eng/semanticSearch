# Semantic Search

A portable semantic search desktop application for PDF and Markdown documents.

## Features

- **PDF & Markdown indexing** - Extract and index text from PDF and MD files
- **Semantic search** - Find documents by meaning, not just keywords
- **SQLite storage** - Local database for fast search
- **Portable Windows .exe** - No installation required

## Installation

Download `search.exe` from the releases and run directly.

## Usage

```bash
# Index a directory
search.exe index ./docs

# Search documents
search.exe search "your query"
```

## Development

```bash
# Build
make build

# Test
make test

# Cross-compile to Windows
make build-windows
```

## Requirements

- Embedding API key (optional): Set `EMBEDDING_API_KEY` environment variable
- Without API key, uses zero-vector fallback (semantic search won't work offline)

## License

MIT
