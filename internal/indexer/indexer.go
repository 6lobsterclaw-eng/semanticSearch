package indexer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"semantic-search/internal/storage"
)

type Indexer struct {
	db       *storage.DB
	embedder *Embedder
}

func NewIndexer(db *storage.DB) *Indexer {
	return &Indexer{
		db:       db,
		embedder: NewEmbedder(),
	}
}

func (idx *Indexer) IndexDir(ctx context.Context, dirPath string) error {
	var files []string

	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".pdf" || ext == ".md" {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return err
	}

	for _, file := range files {
		if err := idx.indexFile(ctx, file); err != nil {
			continue
		}
	}

	return nil
}

func (idx *Indexer) indexFile(ctx context.Context, path string) error {
	ext := strings.ToLower(filepath.Ext(path))

	var content string
	var err error

	switch ext {
	case ".pdf":
		content, err = ExtractText(path)
	case ".md":
		content, err = ExtractMarkdown(path)
	default:
		return nil
	}

	if err != nil {
		return err
	}

	title := ExtractTitle(path)

	doc := storage.Document{
		ID:        uuid.New().String(),
		Path:      path,
		Content:   content,
		Title:     title,
		IndexedAt: time.Now().Unix(),
	}

	if err := idx.db.InsertDocument(ctx, doc); err != nil {
		return err
	}

	// Generate embedding
	emb, err := idx.embedder.Embed(ctx, content)
	if err != nil {
		return err
	}

	return idx.db.InsertEmbedding(ctx, doc.ID, emb)
}
