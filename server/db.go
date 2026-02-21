package main

import (
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

type Indexer struct {
	db *sql.DB
}

func NewIndexer(dbPath string) (*Indexer, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	// TODO: attenzione doc_hash è PRIMARY KEY, non si può notarizzare due volte lo stesso documento
	// 		 una PK alternativa potrebbe essere log_index o event_id
	query := `
    CREATE TABLE IF NOT EXISTS notary_index (
        doc_hash  TEXT PRIMARY KEY,
        leaf_hash TEXT,
        log_index INTEGER,
		doc_uid   TEXT NOT NULL,
		event_id  TEXT NOT NULL UNIQUE
    );`

	if _, err := db.Exec(query); err != nil {
		db.Close()
		return nil, err
	}
	return &Indexer{db: db}, nil
}

// func (idx *Indexer) AddEntry(docHash, leafHash string, logIndex uint64) error {
// 	query := `INSERT INTO notary_index (doc_hash, leaf_hash, log_index) VALUES (?, ?, ?)`
// 	_, err := idx.db.Exec(query, docHash, leafHash, logIndex)
// 	return err
// }

func (idx *Indexer) AddEntry(docUID, eventID, docHash, leafHash string, logIndex uint64) error {
	// Aggiorniamo la query per includere i nuovi campi
	query := `INSERT INTO notary_index (doc_uid, event_id, doc_hash, leaf_hash, log_index) 
              VALUES (?, ?, ?, ?, ?)`

	_, err := idx.db.Exec(query, docUID, eventID, docHash, leafHash, logIndex)
	return err
}

func (idx *Indexer) GetByDocHash(docHash string) (string, uint64, error) {
	var leafHash string
	var logIndex uint64
	query := `SELECT leaf_hash, log_index FROM notary_index WHERE doc_hash = ?`
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
func (i *Indexer) GetIndexesByDocUID(docUID string) ([]uint64, error) {
	docUID = strings.TrimSpace(docUID)
	if docUID == "" {
		return nil, fmt.Errorf("empty doc_uid")
	}

	rows, err := i.db.Query(`
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
		var idx int64
		if err := rows.Scan(&idx); err != nil {
			return nil, err
		}
		if idx < 0 {
			continue
		}
		out = append(out, uint64(idx))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (idx *Indexer) Close() error {
	return idx.db.Close()
}
