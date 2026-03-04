package anchor

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// FilePublisher is a fake blockchain publisher backed by a text file.
// Each line is a space-separated record in this order:
// published_at_utc domain_separator version tree_size root_hash_hex checkpoint_hash
type FilePublisher struct {
	path string
}

func NewFilePublisher(path string) (*FilePublisher, error) {
	if path == "" {
		return nil, fmt.Errorf("empty anchor file path")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create anchor file dir: %w", err)
	}
	return &FilePublisher{path: path}, nil
}

func (p *FilePublisher) PublishCheckpoint(ctx context.Context, rec Record) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	f, err := os.OpenFile(p.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open anchor file: %w", err)
	}
	defer f.Close()

	line, err := formatRecordLine(rec)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(f, line); err != nil {
		return fmt.Errorf("write anchor record: %w", err)
	}
	return nil
}

func (p *FilePublisher) LatestCheckpoint(ctx context.Context) (AnchoredCheckpoint, error) {
	select {
	case <-ctx.Done():
		return AnchoredCheckpoint{}, ctx.Err()
	default:
	}

	f, err := os.Open(p.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return AnchoredCheckpoint{}, ErrNoPublishedCheckpoint
		}
		return AnchoredCheckpoint{}, fmt.Errorf("open anchor file: %w", err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	var (
		lastLine    string
		blockNumber uint64
	)
	for sc.Scan() {
		select {
		case <-ctx.Done():
			return AnchoredCheckpoint{}, ctx.Err()
		default:
		}
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		blockNumber++
		lastLine = line
	}
	if err := sc.Err(); err != nil {
		return AnchoredCheckpoint{}, fmt.Errorf("scan anchor file: %w", err)
	}
	if lastLine == "" {
		return AnchoredCheckpoint{}, ErrNoPublishedCheckpoint
	}

	rec, err := parseRecordLine(lastLine)
	if err != nil {
		return AnchoredCheckpoint{}, fmt.Errorf("decode anchor record: %w", err)
	}
	txHash := sha256.Sum256([]byte(lastLine))
	return AnchoredCheckpoint{
		Record:      rec,
		TxID:        hex.EncodeToString(txHash[:]),
		BlockNumber: blockNumber,
	}, nil
}

func formatRecordLine(rec Record) (string, error) {
	return fmt.Sprintf(
		"%s %s %d %d %s %s",
		time.Now().UTC().Format(time.RFC3339Nano),
		rec.DomainSeparator,
		rec.Version,
		rec.TreeSize,
		rec.RootHashHex,
		rec.CheckpointHash,
	), nil
}

func parseRecordLine(line string) (Record, error) {
	fields := strings.Fields(line)
	if len(fields) != 6 {
		return Record{}, fmt.Errorf("invalid field count: got %d, want 6", len(fields))
	}

	version64, err := strconv.ParseUint(fields[2], 10, 8)
	if err != nil {
		return Record{}, fmt.Errorf("parse version: %w", err)
	}
	treeSize, err := strconv.ParseUint(fields[3], 10, 64)
	if err != nil {
		return Record{}, fmt.Errorf("parse tree size: %w", err)
	}

	return Record{
		DomainSeparator: fields[1],
		Version:         uint8(version64),
		TreeSize:        treeSize,
		RootHashHex:     fields[4],
		CheckpointHash:  fields[5],
	}, nil
}
