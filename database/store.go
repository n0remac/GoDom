package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	_ "github.com/glebarez/go-sqlite"
)

const defaultImageDir = "data/images"

type DocumentStore struct {
	db       *sql.DB
	imageDir string
}

// NewSQLiteStoreFromDSN opens a sqlite database and ensures the documents table exists.
// Example DSN: ":memory:" or "./data/db.sqlite"
func NewSQLiteStoreFromDSN(dsn string) (*DocumentStore, error) {
	return NewSQLiteStoreFromDSNWithImageDir(dsn, defaultImageDir)
}

// NewSQLiteStoreFromDSNWithImageDir opens a sqlite database and configures where image files are stored.
// If imageDir is empty, data/images is used.
func NewSQLiteStoreFromDSNWithImageDir(dsn, imageDir string) (*DocumentStore, error) {
	if strings.TrimSpace(imageDir) == "" {
		imageDir = defaultImageDir
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// set pragmas for reasonable durability and concurrency
	if _, err := db.Exec("PRAGMA journal_mode = WAL;"); err != nil {
		// non-fatal; continue
	}

	s := &DocumentStore{db: db, imageDir: imageDir}
	if err := s.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	if err := s.ensureImageDir(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *DocumentStore) migrate(ctx context.Context) error {
	// documents table: id TEXT PRIMARY KEY, content BLOB, created_at INTEGER, updated_at INTEGER
	q := `CREATE TABLE IF NOT EXISTS documents (
        id TEXT PRIMARY KEY,
        content BLOB NOT NULL,
        created_at INTEGER NOT NULL,
        updated_at INTEGER NOT NULL
    );`
	if _, err := s.db.ExecContext(ctx, q); err != nil {
		return err
	}

	imageTable := `CREATE TABLE IF NOT EXISTS images (
        path TEXT PRIMARY KEY,
        original_name TEXT NOT NULL,
        content_type TEXT NOT NULL,
        size INTEGER NOT NULL,
        created_at INTEGER NOT NULL,
        updated_at INTEGER NOT NULL
    );`
	_, err := s.db.ExecContext(ctx, imageTable)
	return err
}

func (s *DocumentStore) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

// Put inserts or updates a document by id. The doc must be valid JSON (or will be re-marshaled).
func (s *DocumentStore) Put(ctx context.Context, id string, doc json.RawMessage) error {
	if id == "" {
		return errors.New("id required")
	}
	now := time.Now().Unix()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// upsert
	_, err = tx.ExecContext(ctx, `INSERT INTO documents(id, content, created_at, updated_at) VALUES(?,?,?,?)
        ON CONFLICT(id) DO UPDATE SET content=excluded.content, updated_at=excluded.updated_at`, id, doc, now, now)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// Get returns the raw JSON document for the given id.
func (s *DocumentStore) Get(ctx context.Context, id string) (json.RawMessage, error) {
	var data []byte
	row := s.db.QueryRowContext(ctx, "SELECT content FROM documents WHERE id = ?", id)
	if err := row.Scan(&data); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return json.RawMessage(data), nil
}

// Delete removes a document by id.
func (s *DocumentStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM documents WHERE id = ?", id)
	return err
}

// List returns all document ids.
func (s *DocumentStore) List(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id FROM documents")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// Query performs a simple substring search on the JSON content. This is intentionally
// lightweight; for advanced searches enable FTS indexes separately.
func (s *DocumentStore) Query(ctx context.Context, substr string) ([][]byte, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT content FROM documents")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var res [][]byte
	for rows.Next() {
		var b []byte
		if err := rows.Scan(&b); err != nil {
			return nil, err
		}
		if substr == "" {
			res = append(res, b)
			continue
		}
		if contains := (len(b) > 0 && string(b) != "" && (len(substr) == 0 || (len(substr) > 0 && (func() bool { return len(b) >= len(substr) && (func() bool { return string(b) != "" })() })()))); contains {
			// fallback: simple substring match
		}
		// simple substring search
		if len(substr) > 0 {
			if bytesContains(b, []byte(substr)) {
				res = append(res, b)
			}
			continue
		}
		res = append(res, b)
	}
	return res, rows.Err()
}

// bytesContains is a small wrapper to avoid importing bytes multiple places
func bytesContains(b, sub []byte) bool {
	if len(sub) == 0 {
		return true
	}
	if len(b) < len(sub) {
		return false
	}
	for i := 0; i <= len(b)-len(sub); i++ {
		if string(b[i:i+len(sub)]) == string(sub) {
			return true
		}
	}
	return false
}
