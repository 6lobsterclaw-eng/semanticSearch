package indexer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kelindar/search"
	"semantic-search/internal/storage"
)

type Indexer struct {
	db       *storage.DB
	embedder *Embedder
}

func NewIndexer(db *storage.DB, modelPath string) (*Indexer, error) {
	emb, err := NewEmbedder(modelPath)
	if err != nil {
		return nil, err
	}
	return &Indexer{
		db:       db,
		embedder: emb,
	}, nil
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
	vec, err := idx.embedder.Embed(content)
	if err != nil {
		return err
	}

	// Add to kelindar search index
	idx.embedder.AddDocument(doc.ID, vec, doc.Title)

	// Store embedding in DB
	return idx.db.InsertEmbedding(ctx, doc.ID, vec)
}

func (idx *Indexer) SaveIndex(path string) error {
	return idx.embedder.SaveIndex(path)
}

func (idx *Indexer) LoadIndex(path string) error {
	return idx.embedder.LoadIndex(path)
}

// Search searches indexed documents using kelindar/search
func (idx *Indexer) Search(query string, k int) []search.Result[string] {
	vec, err := idx.embedder.Embed(query)
	if err != nil {
		return nil
	}
	return idx.embedder.Search(vec, k)
}
