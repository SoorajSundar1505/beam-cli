package history

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"beam/internal/storage"
)

type Kind string

const (
	KindText  Kind = "text"
	KindImage Kind = "image"
	KindFile  Kind = "file"
)

type Item struct {
	ID        int64
	Kind      Kind
	Text      string
	MIME      string
	Filename  string
	Size      int64
	BlobPath  string
	CreatedAt time.Time
}

type Store struct {
	db *sql.DB
}

func Open() (*Store, error) {
	path, err := storage.HistoryPath()
	if err != nil {
		return nil, err
	}
	return OpenPath(path)
}

func OpenPath(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS clipboard (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  kind TEXT NOT NULL,
  text_content TEXT,
  mime TEXT,
  filename TEXT,
  size INTEGER,
  blob_path TEXT,
  created_at INTEGER NOT NULL
);
CREATE VIRTUAL TABLE IF NOT EXISTS clipboard_fts USING fts5(
  text_content,
  filename,
  content='clipboard',
  content_rowid='id'
);
CREATE TRIGGER IF NOT EXISTS clipboard_ai AFTER INSERT ON clipboard BEGIN
  INSERT INTO clipboard_fts(rowid, text_content, filename) VALUES (new.id, new.text_content, new.filename);
END;
CREATE TRIGGER IF NOT EXISTS clipboard_ad AFTER DELETE ON clipboard BEGIN
  INSERT INTO clipboard_fts(clipboard_fts, rowid, text_content, filename) VALUES('delete', old.id, old.text_content, old.filename);
END;
`)
	return err
}

func (s *Store) Add(item Item) (int64, error) {
	if item.CreatedAt.IsZero() {
		item.CreatedAt = time.Now()
	}
	res, err := s.db.Exec(
		`INSERT INTO clipboard (kind, text_content, mime, filename, size, blob_path, created_at) VALUES (?,?,?,?,?,?,?)`,
		string(item.Kind), item.Text, item.MIME, item.Filename, item.Size, item.BlobPath, item.CreatedAt.Unix(),
	)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if err := s.prune(500); err != nil {
		return id, err
	}
	return id, nil
}

func (s *Store) prune(max int) error {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM clipboard`).Scan(&n); err != nil {
		return err
	}
	if n <= max {
		return nil
	}
	rows, err := s.db.Query(`SELECT id, blob_path FROM clipboard ORDER BY created_at ASC LIMIT ?`, n-max)
	if err != nil {
		return err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		var blob string
		if err := rows.Scan(&id, &blob); err != nil {
			return err
		}
		ids = append(ids, id)
		if blob != "" {
			_ = os.Remove(blob)
		}
	}
	for _, id := range ids {
		if _, err := s.db.Exec(`DELETE FROM clipboard WHERE id = ?`, id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) List(limit int) ([]Item, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(
		`SELECT id, kind, text_content, mime, filename, size, blob_path, created_at FROM clipboard ORDER BY created_at DESC, id DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanItems(rows)
}

func (s *Store) Get(id int64) (*Item, error) {
	row := s.db.QueryRow(
		`SELECT id, kind, text_content, mime, filename, size, blob_path, created_at FROM clipboard WHERE id = ?`,
		id,
	)
	it, err := scanItem(row)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("clipboard item %d not found", id)
	}
	return it, err
}

func (s *Store) Search(query string, limit int) ([]Item, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return s.List(limit)
	}
	if limit <= 0 {
		limit = 50
	}
	// Query the source table directly. FTS token/prefix matching cannot provide
	// true substring matching and an older database may have an incomplete FTS
	// index. INSTR also avoids treating user input as a LIKE pattern.
	rows, err := s.db.Query(`
SELECT id, kind, text_content, mime, filename, size, blob_path, created_at
FROM clipboard
WHERE instr(lower(coalesce(text_content, '')), lower(?)) > 0
   OR instr(lower(coalesce(filename, '')), lower(?)) > 0
ORDER BY created_at DESC, id DESC
LIMIT ?`, query, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanItems(rows)
}

func (s *Store) GetLatest() (*Item, error) {
	row := s.db.QueryRow(
		`SELECT id, kind, text_content, mime, filename, size, blob_path, created_at FROM clipboard ORDER BY created_at DESC, id DESC LIMIT 1`,
	)
	it, err := scanItem(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return it, err
}

type scanner interface {
	Scan(dest ...any) error
}

func scanItem(row scanner) (*Item, error) {
	var it Item
	var kind string
	var created int64
	var text, mime, filename, blob sql.NullString
	var size sql.NullInt64
	if err := row.Scan(&it.ID, &kind, &text, &mime, &filename, &size, &blob, &created); err != nil {
		return nil, err
	}
	it.Kind = Kind(kind)
	it.Text = text.String
	it.MIME = mime.String
	it.Filename = filename.String
	it.Size = size.Int64
	it.BlobPath = blob.String
	it.CreatedAt = time.Unix(created, 0)
	return &it, nil
}

func scanItems(rows *sql.Rows) ([]Item, error) {
	var out []Item
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *it)
	}
	return out, rows.Err()
}
