package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/transparency-dev/tessera"
)

const startupTailCheckCount = 10

// ValidateAlignedWithLog verifica che l'indice SQLite sia allineato con il log di trasparenza.
// Controlla che il numero di righe coincida con la tree size pubblicata, e valida le ultime
// [startupTailCheckCount] entry confrontando i metadati del DB con il contenuto del log.
// TODO: controllare anche 10 righe casuali.
func (idx *Indexer) ValidateAlignedWithLog(ctx context.Context, reader tessera.LogReader) error {
	if idx == nil || idx.db == nil {
		return fmt.Errorf("indexer not initialized")
	}
	if reader == nil {
		return fmt.Errorf("nil log reader")
	}

	size, err := publishedTreeSize(ctx, reader)
	if err != nil {
		return err
	}

	if err := idx.checkRowCount(size); err != nil {
		return err
	}

	if size == 0 {
		return nil
	}

	return idx.checkTailEntries(ctx, reader, size)
}

// checkRowCount verifica che il numero di righe nel DB corrisponda alla tree size del log.
func (idx *Indexer) checkRowCount(treeSize uint64) error {
	dbCount, err := idx.countIndexedRows()
	if err != nil {
		return err
	}
	if uint64(dbCount) != treeSize {
		return fmt.Errorf("db/log mismatch: db rows=%d log tree_size=%d", dbCount, treeSize)
	}
	return nil
}

// checkTailEntries valida le ultime N entry confrontando il DB con il contenuto del log.
func (idx *Indexer) checkTailEntries(ctx context.Context, reader tessera.LogReader, treeSize uint64) error {
	tail := uint64(startupTailCheckCount)
	if treeSize < tail {
		tail = treeSize
	}
	start := treeSize - tail

	for i := start; i < treeSize; i++ {
		if err := idx.checkSingleEntry(ctx, reader, treeSize, i); err != nil {
			return err
		}
	}
	return nil
}

// checkSingleEntry confronta una singola riga del DB con la corrispondente entry nel log.
func (idx *Indexer) checkSingleEntry(ctx context.Context, reader tessera.LogReader, treeSize, logIndex uint64) error {
	row, err := idx.getIndexedRowByLogIndex(logIndex)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("db/log mismatch: missing db row for log_index=%d", logIndex)
		}
		return err
	}

	entryRaw, err := readLogEntryByIndex(ctx, reader, treeSize, logIndex)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("db/log mismatch: log entry missing at index=%d", logIndex)
		}
		return fmt.Errorf("read log entry at index=%d: %w", logIndex, err)
	}

	return validateIndexedRowAgainstEntry(logIndex, row, entryRaw)
}

// indexedRow contiene i metadati di una entry indicizzata nel DB.
type indexedRow struct {
	DocUID   string
	EventID  string
	DocHash  string
	LeafHash string
}

func (idx *Indexer) countIndexedRows() (int64, error) {
	var n int64
	if err := idx.db.QueryRow(`SELECT COUNT(*) FROM notary_index`).Scan(&n); err != nil {
		return 0, fmt.Errorf("count db rows: %w", err)
	}
	if n < 0 {
		return 0, fmt.Errorf("invalid negative db count")
	}
	return n, nil
}

func (idx *Indexer) getIndexedRowByLogIndex(logIndex uint64) (indexedRow, error) {
	var row indexedRow
	err := idx.db.QueryRow(`
		SELECT doc_uid, event_id, doc_hash, leaf_hash
		FROM notary_index
		WHERE log_index = ?
	`, logIndex).Scan(&row.DocUID, &row.EventID, &row.DocHash, &row.LeafHash)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return indexedRow{}, sql.ErrNoRows
		}
		return indexedRow{}, fmt.Errorf("read db row for log_index=%d: %w", logIndex, err)
	}
	return row, nil
}

// logEntry è la struttura attesa per ogni entry nel log di trasparenza.
type logEntry struct {
	DocUID      string       `json:"doc_uid"`
	EventID     string       `json:"event_id"`
	PayloadHash *payloadHash `json:"payload_hash,omitempty"`
}

type payloadHash struct {
	Value string `json:"value"`
}

// validateIndexedRowAgainstEntry confronta una riga del DB con la corrispondente entry del log,
// verificando doc_uid, event_id, doc_hash e leaf_hash.
func validateIndexedRowAgainstEntry(idx uint64, row indexedRow, entryRaw []byte) error {
	var entry logEntry
	if err := json.Unmarshal(entryRaw, &entry); err != nil {
		return fmt.Errorf("db/log mismatch at index=%d: entry is not valid JSON: %w", idx, err)
	}

	if strings.TrimSpace(row.DocUID) != strings.TrimSpace(entry.DocUID) {
		return fmt.Errorf("db/log mismatch at index=%d: doc_uid db=%q log=%q", idx, row.DocUID, entry.DocUID)
	}
	if strings.TrimSpace(row.EventID) != strings.TrimSpace(entry.EventID) {
		return fmt.Errorf("db/log mismatch at index=%d: event_id db=%q log=%q", idx, row.EventID, entry.EventID)
	}

	logDocHash := ""
	if entry.PayloadHash != nil && strings.TrimSpace(entry.PayloadHash.Value) != "" {
		h, err := parsePayloadHashValue(entry.PayloadHash.Value)
		if err != nil {
			return fmt.Errorf("db/log mismatch at index=%d: invalid payload_hash in log entry: %w", idx, err)
		}
		logDocHash = h
	}
	if strings.ToLower(strings.TrimSpace(row.DocHash)) != logDocHash {
		return fmt.Errorf("db/log mismatch at index=%d: doc_hash db=%q log=%q", idx, row.DocHash, logDocHash)
	}

	logLeafHash := hashBytes(entryRaw)
	if strings.ToLower(strings.TrimSpace(row.LeafHash)) != logLeafHash {
		return fmt.Errorf("db/log mismatch at index=%d: leaf_hash db=%q log=%q", idx, row.LeafHash, logLeafHash)
	}

	return nil
}
