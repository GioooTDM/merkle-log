package index

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

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

// GetIndexesByDocID returns all log_index values for a given doc_id, ordered ascending.
func (idx *Indexer) GetIndexesByDocID(docID string) ([]uint64, error) {
	docID = strings.TrimSpace(docID)
	if docID == "" {
		return nil, fmt.Errorf("empty doc_id")
	}

	rows, err := idx.db.Query(`
		SELECT log_index
		FROM notary_index
		WHERE doc_id = ?
		ORDER BY log_index ASC
	`, docID)
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

// GetLatestIndexByDocID returns the highest log_index for a given doc_id.
func (idx *Indexer) GetLatestIndexByDocID(docID string) (uint64, bool, error) {
	docID = strings.TrimSpace(docID)
	if docID == "" {
		return 0, false, fmt.Errorf("empty doc_id")
	}

	var i int64
	err := idx.db.QueryRow(`
		SELECT log_index
		FROM notary_index
		WHERE doc_id = ?
		ORDER BY log_index DESC
		LIMIT 1
	`, docID).Scan(&i)
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
