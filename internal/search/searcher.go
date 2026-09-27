package search

import (
	"context"
	"sort"

	"github.com/kelindar/search"
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

func NewSearcher(db *storage.DB) (*Searcher, error) {
	emb, err := indexer.NewEmbedder()
	if err != nil {
		return nil, err
	}
	return &Searcher{
		db:       db,
		embedder: emb,
	}, nil
}

func (s *Searcher) Search(ctx context.Context, query string, limit int) ([]Result, error) {
	// Generate query embedding
	queryEmb, err := s.embedder.Embed(query)
	if err != nil {
		return nil, err
	}

	// Get all documents
	docs, err := s.db.GetAllDocuments(ctx)
	if err != nil {
		return nil, err
	}

	// Compute similarities using kelindar/search
	var results []Result
	for _, doc := range docs {
		emb, err := s.db.GetEmbedding(ctx, doc.ID)
		if err != nil || emb == nil {
			continue
		}

		// Convert to search.Vector for kelindar
		vec := make(search.Vector, len(emb))
		for i, v := range emb {
			vec[i] = v
		}

		// Use kelindar's cosine similarity
		score := cosineSimilarity(queryEmb, vec)
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

func cosineSimilarity(a, b search.Vector) float64 {
	var dot float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
	}
	return dot // Already L2-normalized by rembed
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
