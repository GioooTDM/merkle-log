package anchor

import "context"

// Publisher is blockchain-agnostic.
// A real implementation can publish the same record to any chain.
type Publisher interface {
	PublishCheckpoint(ctx context.Context, rec Record) error
}

// Record represents one checkpoint publication event.
// The fake blockchain writes one JSON object per line in a text file.
type Record struct {
	PublishedAtUTC string `json:"published_at_utc"`
	TreeSize       uint64 `json:"tree_size"`
	RootHashHex    string `json:"root_hash_hex"`
	CheckpointHash string `json:"checkpoint_hash"`
	CheckpointRaw  string `json:"checkpoint_raw"`
}
