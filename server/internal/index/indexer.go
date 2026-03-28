package index

import (
	"database/sql"

	_ "modernc.org/sqlite"
)

type Indexer struct {
	db *sql.DB
}

type Entry struct {
	LogIndex       uint64
	DocID          string
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
	doc_id              TEXT NOT NULL,
	issuer_entity_id     TEXT NOT NULL,
	event_id             TEXT NOT NULL UNIQUE,
	recorded_at_unix_ns  INTEGER NOT NULL
);`

var notaryIndexDDL = []string{
	`CREATE INDEX IF NOT EXISTS idx_notary_index_doc_hash ON notary_index(doc_hash);`,
	`CREATE INDEX IF NOT EXISTS idx_notary_index_doc_id ON notary_index(doc_id);`,
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

func (idx *Indexer) Close() error {
	return idx.db.Close()
}
