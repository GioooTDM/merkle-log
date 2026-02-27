package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"sync"
	"time"

	formatsLog "github.com/transparency-dev/formats/log"
	"github.com/transparency-dev/tessera"
)

// AnchorWorker periodically publishes the latest checkpoint
// through an AnchorPublisher.
type AnchorWorker struct {
	reader    tessera.LogReader
	publisher AnchorPublisher
	interval  time.Duration
	lastHash  string
	mu        sync.Mutex
}

func NewAnchorWorker(reader tessera.LogReader, publisher AnchorPublisher, interval time.Duration) (*AnchorWorker, error) {
	if reader == nil {
		return nil, fmt.Errorf("nil log reader")
	}
	if publisher == nil {
		return nil, fmt.Errorf("nil anchor publisher")
	}
	if interval <= 0 {
		return nil, fmt.Errorf("anchor interval must be > 0")
	}
	return &AnchorWorker{
		reader:    reader,
		publisher: publisher,
		interval:  interval,
	}, nil
}

func (w *AnchorWorker) Run(ctx context.Context) {
	// First publication on startup, then periodic ticks.
	if err := w.anchorOnce(ctx); err != nil {
		// Do not stop the worker on one failure.
		log.Printf("anchor publish failed: %v", err)
	}

	t := time.NewTicker(w.interval)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := w.anchorOnce(ctx); err != nil {
				log.Printf("anchor publish failed: %v", err)
			}
		}
	}
}

// buildAnchorRecord parses cpRaw into a structured AnchorRecord.
// cpHash is the SHA-256 hex digest of cpRaw, already computed by the caller.
func buildAnchorRecord(cpRaw []byte, cpHash string) (AnchorRecord, error) {
	var cp formatsLog.Checkpoint
	if _, err := cp.Unmarshal(cpRaw); err != nil {
		return AnchorRecord{}, fmt.Errorf("parse checkpoint: %w", err)
	}

	return AnchorRecord{
		PublishedAtUTC: time.Now().UTC().Format(time.RFC3339Nano),
		TreeSize:       cp.Size,
		RootHashHex:    hex.EncodeToString(cp.Hash),
		CheckpointHash: cpHash,
		CheckpointRaw:  string(cpRaw),
	}, nil
}

func (w *AnchorWorker) anchorOnce(ctx context.Context) error {
	_, _, err := w.publishCheckpoint(ctx, false)
	return err
}

// PublishNow forces an immediate publication even if the interval has not elapsed.
func (w *AnchorWorker) PublishNow(ctx context.Context) (AnchorRecord, error) {
	rec, _, err := w.publishCheckpoint(ctx, true)
	return rec, err
}

func (w *AnchorWorker) publishCheckpoint(ctx context.Context, force bool) (AnchorRecord, bool, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	cpRaw, err := w.reader.ReadCheckpoint(ctx)
	if err != nil {
		return AnchorRecord{}, false, fmt.Errorf("read checkpoint: %w", err)
	}

	h := sha256.Sum256(cpRaw)
	cpHash := hex.EncodeToString(h[:])
	if !force && cpHash == w.lastHash {
		// No new checkpoint to publish.
		return AnchorRecord{}, false, nil
	}

	rec, err := buildAnchorRecord(cpRaw, cpHash)
	if err != nil {
		return AnchorRecord{}, false, err
	}

	if err := w.publisher.PublishCheckpoint(ctx, rec); err != nil {
		return AnchorRecord{}, false, err
	}
	w.lastHash = cpHash
	return rec, true, nil
}
