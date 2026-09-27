package storage

import (
	"context"
	"database/sql"
	"math"

	_ "modernc.org/sqlite"
)

type Document struct {
	ID        string
	Path      string
	Content   string
	Title     string
	IndexedAt int64
}

type DB struct {
	conn *sql.DB
}

func NewDB(path string) (*DB, error) {
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	return &DB{conn: conn}, nil
}

func (db *DB) InitSchema(ctx context.Context) error {
	_, err := db.conn.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS documents (
			id TEXT PRIMARY KEY,
			path TEXT NOT NULL UNIQUE,
			content TEXT,
			title TEXT,
			indexed_at INTEGER
		);
		CREATE TABLE IF NOT EXISTS embeddings (
			doc_id TEXT PRIMARY KEY,
			vector BLOB,
			FOREIGN KEY (doc_id) REFERENCES documents(id)
		);
	`)
	return err
}

func (db *DB) InsertDocument(ctx context.Context, doc Document) error {
	_, err := db.conn.ExecContext(ctx,
		`INSERT OR REPLACE INTO documents (id, path, content, title, indexed_at) VALUES (?, ?, ?, ?, ?)`,
		doc.ID, doc.Path, doc.Content, doc.Title, doc.IndexedAt)
	return err
}

func (db *DB) Close() error {
	return db.conn.Close()
}

func (db *DB) InsertEmbedding(ctx context.Context, docID string, emb []float32) error {
	// Serialize float32 to bytes
	data := make([]byte, len(emb)*4)
	for i, v := range emb {
		bits := math.Float32bits(v)
		data[i*4] = byte(bits)
		data[i*4+1] = byte(bits >> 8)
		data[i*4+2] = byte(bits >> 16)
		data[i*4+3] = byte(bits >> 24)
	}

	_, err := db.conn.ExecContext(ctx,
		`INSERT OR REPLACE INTO embeddings (doc_id, vector) VALUES (?, ?)`,
		docID, data)
	return err
}

func (db *DB) GetAllDocuments(ctx context.Context) ([]Document, error) {
	rows, err := db.conn.QueryContext(ctx, "SELECT id, path, content, title, indexed_at FROM documents")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var docs []Document
	for rows.Next() {
		var doc Document
		if err := rows.Scan(&doc.ID, &doc.Path, &doc.Content, &doc.Title, &doc.IndexedAt); err != nil {
			continue
		}
		docs = append(docs, doc)
	}
	return docs, nil
}

func (db *DB) GetEmbedding(ctx context.Context, docID string) ([]float32, error) {
	var data []byte
	err := db.conn.QueryRowContext(ctx, "SELECT vector FROM embeddings WHERE doc_id = ?", docID).Scan(&data)
	if err != nil {
		return nil, err
	}

	emb := make([]float32, len(data)/4)
	for i := 0; i < len(emb); i++ {
		bits := uint32(data[i*4]) | uint32(data[i*4+1])<<8 | uint32(data[i*4+2])<<16 | uint32(data[i*4+3])<<24
		emb[i] = math.Float32frombits(bits)
	}
	return emb, nil
}
