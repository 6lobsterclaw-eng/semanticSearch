package search

import (
	"context"
	"math"
	"sort"

	"semantic-search/internal/indexer"
	"semantic-search/internal/storage"
)

type Result struct {
	DocID   string
	Path    string
	Title   string
	Score   float64
	Preview string
}

type Searcher struct {
	db       *storage.DB
	embedder *indexer.Embedder
}

func NewSearcher(db *storage.DB) *Searcher {
	return &Searcher{
		db:       db,
		embedder: indexer.NewEmbedder(),
	}
}

func (s *Searcher) Search(ctx context.Context, query string, limit int) ([]Result, error) {
	// Generate query embedding
	queryEmb, err := s.embedder.Embed(ctx, query)
	if err != nil {
		return nil, err
	}

	// Get all documents
	docs, err := s.db.GetAllDocuments(ctx)
	if err != nil {
		return nil, err
	}

	// Compute similarities
	var results []Result
	for _, doc := range docs {
		emb, err := s.db.GetEmbedding(ctx, doc.ID)
		if err != nil || emb == nil {
			continue
		}

		score := cosineSimilarity(queryEmb, emb)
		results = append(results, Result{
			DocID:   doc.ID,
			Path:    doc.Path,
			Title:   doc.Title,
			Score:   score,
			Preview: truncate(doc.Content, 200),
		})
	}

	// Sort by score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

func cosineSimilarity(a, b indexer.Embedding) float64 {
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
