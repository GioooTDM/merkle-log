package logread

import (
	"context"
	"os"

	formatsLog "github.com/transparency-dev/formats/log"
	"github.com/transparency-dev/tessera"
	"github.com/transparency-dev/tessera/api"
	"github.com/transparency-dev/tessera/api/layout"
)

// ReadPublishedCheckpoint reads and parses the latest published checkpoint.
func ReadPublishedCheckpoint(ctx context.Context, reader tessera.LogReader) ([]byte, formatsLog.Checkpoint, error) {
	cpRaw, err := reader.ReadCheckpoint(ctx)
	if err != nil {
		return nil, formatsLog.Checkpoint{}, err
	}
	var cp formatsLog.Checkpoint
	if _, err := cp.Unmarshal(cpRaw); err != nil {
		return nil, formatsLog.Checkpoint{}, err
	}
	return cpRaw, cp, nil
}

// PublishedTreeSize returns the size committed by the latest published checkpoint.
func PublishedTreeSize(ctx context.Context, reader tessera.LogReader) (uint64, error) {
	_, cp, err := ReadPublishedCheckpoint(ctx, reader)
	if err != nil {
		return 0, err
	}
	return cp.Size, nil
}

// ReadLogEntryByIndex reads a single log entry for a fixed, published tree size snapshot.
func ReadLogEntryByIndex(ctx context.Context, reader tessera.LogReader, treeSize, idx uint64) ([]byte, error) {
	if idx >= treeSize {
		return nil, os.ErrNotExist
	}

	bundleWidth := uint64(layout.EntryBundleWidth)
	bundleIdx := idx / bundleWidth
	offset := idx % bundleWidth
	partial := layout.PartialTileSize(0 /*level*/, bundleIdx, treeSize) // p = partial size (0 se bundle pieno, 1..255 se parziale)

	raw, err := reader.ReadEntryBundle(ctx, bundleIdx, partial)
	if err != nil && partial != 0 {
		// Fallback al bundle pieno: alcuni store non servono i bundle parziali.
		raw, err = reader.ReadEntryBundle(ctx, bundleIdx, 0)
	}
	if err != nil {
		return nil, err
	}

	var eb api.EntryBundle
	if err := eb.UnmarshalText(raw); err != nil {
		return nil, err
	}
	if int(offset) >= len(eb.Entries) {
		return nil, os.ErrNotExist
	}
	return eb.Entries[offset], nil
}
