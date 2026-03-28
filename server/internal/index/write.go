package index

import (
	"fmt"
	"strings"
	"time"
)

func (idx *Indexer) AddEntry(entry Entry) error {
	entry.DocID = strings.TrimSpace(entry.DocID)
	entry.EventID = strings.TrimSpace(entry.EventID)
	entry.DocHash = strings.TrimSpace(entry.DocHash)
	entry.LeafHash = strings.TrimSpace(entry.LeafHash)
	entry.IssuerEntityID = strings.TrimSpace(entry.IssuerEntityID)
	entry.RecordedAt = strings.TrimSpace(entry.RecordedAt)

	recordedAtTime, err := time.Parse(time.RFC3339Nano, entry.RecordedAt)
	if err != nil {
		return fmt.Errorf("invalid recorded_at %q: %w", entry.RecordedAt, err)
	}

	query := `INSERT INTO notary_index (log_index, doc_id, event_id, doc_hash, leaf_hash, issuer_entity_id, recorded_at_unix_ns)
	          VALUES (?, ?, ?, ?, ?, ?, ?)`

	_, err = idx.db.Exec(query, entry.LogIndex, entry.DocID, entry.EventID, entry.DocHash, entry.LeafHash, entry.IssuerEntityID, recordedAtTime.UnixNano())
	return err
}
