package index

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Indexer struct {
	db *sql.DB
}

type Entry struct {
	LogIndex       uint64
	DocUID         string
	EventID        string
	DocHash        string
	LeafHash       string
	IssuerEntityID string
	RecordedAt     string
}

const createNotaryIndexTableSQL = `
CREATE TABLE IF NOT EXISTS notary_index (
	log_index            INTEGER PRIMARY KEY,
	doc_hash             TEXT NOT NULL,
	leaf_hash            TEXT NOT NULL,
	doc_uid              TEXT NOT NULL,
	issuer_entity_id     TEXT NOT NULL,
	event_id             TEXT NOT NULL UNIQUE,
	recorded_at_unix_ns  INTEGER NOT NULL
);`

var notaryIndexDDL = []string{
	`CREATE INDEX IF NOT EXISTS idx_notary_index_doc_hash ON notary_index(doc_hash);`,
	`CREATE INDEX IF NOT EXISTS idx_notary_index_doc_uid ON notary_index(doc_uid);`,
	`CREATE INDEX IF NOT EXISTS idx_notary_index_issuer_entity_id ON notary_index(issuer_entity_id);`,
	`CREATE INDEX IF NOT EXISTS idx_notary_index_leaf_hash ON notary_index(leaf_hash);`,
	`CREATE INDEX IF NOT EXISTS idx_notary_index_recorded_at_unix_ns ON notary_index(recorded_at_unix_ns);`,
}

func New(dbPath string) (*Indexer, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	// IMPORTANTE!!!
	// SQLite non supporta writer concorrenti: limitare il pool a una connessione
	// serializza le scritture in Go prima che raggiungano il file, evitando SQLITE_BUSY.
	db.SetMaxOpenConns(1)

	if err := createNotaryIndexSchema(db); err != nil {
		db.Close()
		return nil, err
	}

	return &Indexer{db: db}, nil
}

func createNotaryIndexSchema(db *sql.DB) error {
	if _, err := db.Exec(createNotaryIndexTableSQL); err != nil {
		return err
	}
	for _, q := range notaryIndexDDL {
		if _, err := db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

func (idx *Indexer) AddEntry(entry Entry) error {
	entry.DocUID = strings.TrimSpace(entry.DocUID)
	entry.EventID = strings.TrimSpace(entry.EventID)
	entry.DocHash = strings.TrimSpace(entry.DocHash)
	entry.LeafHash = strings.TrimSpace(entry.LeafHash)
	entry.IssuerEntityID = strings.TrimSpace(entry.IssuerEntityID)
	entry.RecordedAt = strings.TrimSpace(entry.RecordedAt)

	recordedAtTime, err := time.Parse(time.RFC3339Nano, entry.RecordedAt)
	if err != nil {
		return fmt.Errorf("invalid recorded_at %q: %w", entry.RecordedAt, err)
	}

	query := `INSERT INTO notary_index (log_index, doc_uid, event_id, doc_hash, leaf_hash, issuer_entity_id, recorded_at_unix_ns)
	          VALUES (?, ?, ?, ?, ?, ?, ?)`

	_, err = idx.db.Exec(query, entry.LogIndex, entry.DocUID, entry.EventID, entry.DocHash, entry.LeafHash, entry.IssuerEntityID, recordedAtTime.UnixNano())
	return err
}

// TODO: questa funzione restituisce solo l'ultimo log_index corrispondente al doc_hash cercato. Lo stesso doc_hash può comparire in diverse log_index
func (idx *Indexer) GetByDocHash(docHash string) (string, uint64, error) {
	var leafHash string
	var logIndex uint64
	query := `
		SELECT leaf_hash, log_index
		FROM notary_index
		WHERE doc_hash = ?
		ORDER BY log_index DESC
		LIMIT 1
	`
	err := idx.db.QueryRow(query, docHash).Scan(&leafHash, &logIndex)
	return leafHash, logIndex, err
}

func (idx *Indexer) GetByLeafHash(leafHash string) (uint64, error) {
	var logIndex uint64
	query := `SELECT log_index FROM notary_index WHERE leaf_hash = ?`
	err := idx.db.QueryRow(query, leafHash).Scan(&logIndex)
	return logIndex, err
}

// GetIndexesByDocUID returns all log_index values for a given doc_uid, ordered ascending.
func (idx *Indexer) GetIndexesByDocUID(docUID string) ([]uint64, error) {
	docUID = strings.TrimSpace(docUID)
	if docUID == "" {
		return nil, fmt.Errorf("empty doc_uid")
	}

	rows, err := idx.db.Query(`
		SELECT log_index
		FROM notary_index
		WHERE doc_uid = ?
		ORDER BY log_index ASC
	`, docUID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []uint64
	for rows.Next() {
		var i int64
		if err := rows.Scan(&i); err != nil {
			return nil, err
		}
		if i < 0 {
			continue
		}
		out = append(out, uint64(i))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// GetLatestIndexByDocUID returns the highest log_index for a given doc_uid.
func (idx *Indexer) GetLatestIndexByDocUID(docUID string) (uint64, bool, error) {
	docUID = strings.TrimSpace(docUID)
	if docUID == "" {
		return 0, false, fmt.Errorf("empty doc_uid")
	}

	var i int64
	err := idx.db.QueryRow(`
		SELECT log_index
		FROM notary_index
		WHERE doc_uid = ?
		ORDER BY log_index DESC
		LIMIT 1
	`, docUID).Scan(&i)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	if i < 0 {
		return 0, false, fmt.Errorf("invalid negative log_index %d", i)
	}
	return uint64(i), true, nil
}

// GetIndexesByRecordedAtRange returns all log_index values in a time range.
// Bounds are inclusive for fromInclusive and exclusive for toExclusive.
// Zero-value bounds are treated as unbounded.
func (idx *Indexer) GetIndexesByRecordedAtRange(fromInclusive, toExclusive time.Time) ([]uint64, error) {
	query := `
		SELECT log_index
		FROM notary_index
		WHERE 1=1`
	args := make([]any, 0, 2)

	if !fromInclusive.IsZero() {
		query += ` AND recorded_at_unix_ns >= ?`
		args = append(args, fromInclusive.UnixNano())
	}
	if !toExclusive.IsZero() {
		query += ` AND recorded_at_unix_ns < ?`
		args = append(args, toExclusive.UnixNano())
	}
	query += ` ORDER BY log_index ASC`

	rows, err := idx.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []uint64
	for rows.Next() {
		var i int64
		if err := rows.Scan(&i); err != nil {
			return nil, err
		}
		if i < 0 {
			continue
		}
		out = append(out, uint64(i))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// GetIndexesByIssuerEntityID returns all log_index values for an issuer in a time range.
// Bounds are inclusive for fromInclusive and exclusive for toExclusive.
// Zero-value bounds are treated as unbounded.
func (idx *Indexer) GetIndexesByIssuerEntityID(issuerEntityID string, fromInclusive, toExclusive time.Time) ([]uint64, error) {
	issuerEntityID = strings.TrimSpace(issuerEntityID)
	if issuerEntityID == "" {
		return nil, fmt.Errorf("empty issuer_entity_id")
	}

	query := `
		SELECT log_index
		FROM notary_index
		WHERE issuer_entity_id = ?`
	args := make([]any, 0, 3)
	args = append(args, issuerEntityID)

	if !fromInclusive.IsZero() {
		query += ` AND recorded_at_unix_ns >= ?`
		args = append(args, fromInclusive.UnixNano())
	}
	if !toExclusive.IsZero() {
		query += ` AND recorded_at_unix_ns < ?`
		args = append(args, toExclusive.UnixNano())
	}
	query += ` ORDER BY log_index ASC`

	rows, err := idx.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []uint64
	for rows.Next() {
		var i int64
		if err := rows.Scan(&i); err != nil {
			return nil, err
		}
		if i < 0 {
			continue
		}
		out = append(out, uint64(i))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (idx *Indexer) Close() error {
	return idx.db.Close()
}
