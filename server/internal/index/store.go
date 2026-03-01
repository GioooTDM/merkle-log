package index

import (
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

type Indexer struct {
	db *sql.DB
}

const createNotaryIndexTableSQL = `
CREATE TABLE IF NOT EXISTS notary_index (
	log_index INTEGER PRIMARY KEY,
	doc_hash  TEXT NOT NULL,
	leaf_hash TEXT NOT NULL,
	doc_uid   TEXT NOT NULL,
	event_id  TEXT NOT NULL UNIQUE
);`

var notaryIndexDDL = []string{
	`CREATE INDEX IF NOT EXISTS idx_notary_index_doc_hash ON notary_index(doc_hash);`,
	`CREATE INDEX IF NOT EXISTS idx_notary_index_doc_uid ON notary_index(doc_uid);`,
	`CREATE INDEX IF NOT EXISTS idx_notary_index_leaf_hash ON notary_index(leaf_hash);`,
}

func New(dbPath string) (*Indexer, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

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

func (idx *Indexer) AddEntry(docUID, eventID, docHash, leafHash string, logIndex uint64) error {
	docUID = strings.TrimSpace(docUID)
	eventID = strings.TrimSpace(eventID)
	docHash = strings.TrimSpace(docHash)
	leafHash = strings.TrimSpace(leafHash)

	query := `INSERT INTO notary_index (log_index, doc_uid, event_id, doc_hash, leaf_hash)
	          VALUES (?, ?, ?, ?, ?)`

	_, err := idx.db.Exec(query, logIndex, docUID, eventID, docHash, leafHash)
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

func (idx *Indexer) Close() error {
	return idx.db.Close()
}
