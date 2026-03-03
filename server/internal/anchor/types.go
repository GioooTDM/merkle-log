package anchor

import (
	"context"
	"errors"
)

var ErrNoPublishedCheckpoint = errors.New("no published checkpoint")

const (
	// PayloadPrefix is the 4-byte domain-separation prefix required by the
	// on-chain payload contract.
	PayloadPrefix = "PNOT"
	// PayloadVersion is the initial payload format version.
	PayloadVersion byte = 0x01
)

// Publisher is blockchain-agnostic.
// A real implementation can publish the same record to any chain.
type Publisher interface {
	PublishCheckpoint(ctx context.Context, rec Record) error
	LatestCheckpoint(ctx context.Context) (AnchoredCheckpoint, error)
}

// Record represents one checkpoint publication event.
// It only contains protocol-level fields shared by all publishers.
type Record struct {
	DomainSeparator string `json:"domain_separator"`
	Version         uint8  `json:"version"`
	TreeSize        uint64 `json:"tree_size"`
	RootHashHex     string `json:"root_hash_hex"`
	CheckpointHash  string `json:"checkpoint_hash"`
}

// AnchoredCheckpoint combines protocol checkpoint fields with anchoring metadata.
type AnchoredCheckpoint struct {
	Record
	TxID        string `json:"tx_id"`
	BlockNumber uint64 `json:"block_number"`
}
