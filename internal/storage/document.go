package storage

import (
	"context"
	"database/sql"

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
