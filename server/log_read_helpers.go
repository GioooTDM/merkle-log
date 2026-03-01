package main

import (
	"context"
	"os"

	formatsLog "github.com/transparency-dev/formats/log"
	"github.com/transparency-dev/tessera"
	"github.com/transparency-dev/tessera/api"
	"github.com/transparency-dev/tessera/api/layout"
)

// These helpers are the single protocol-level read path for checkpoint/tree-size/entries.
// Handler and DB alignment code should call these functions instead of duplicating logic.
func readPublishedCheckpoint(ctx context.Context, reader tessera.LogReader) ([]byte, formatsLog.Checkpoint, error) {
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

func publishedTreeSize(ctx context.Context, reader tessera.LogReader) (uint64, error) {
	_, cp, err := readPublishedCheckpoint(ctx, reader)
	if err != nil {
		return 0, err
	}
	return cp.Size, nil
}

// readLogEntryByIndex legge una singola entry dal log dato il suo indice assoluto.
// Tenta prima la lettura con tile parziale; se fallisce, riprova con tile completo
// (caso in cui il tile è stato nel frattempo completato da un'altra scrittura).
func readLogEntryByIndex(ctx context.Context, reader tessera.LogReader, treeSize, idx uint64) ([]byte, error) {
	if idx >= treeSize {
		return nil, os.ErrNotExist
	}

	bundleIdx := idx / EntryBundleWidth
	offset := idx % EntryBundleWidth
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
