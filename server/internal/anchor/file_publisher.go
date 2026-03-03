package anchor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// FilePublisher is a fake blockchain publisher backed by a text file (JSONL).
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

	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(rec); err != nil {
		return fmt.Errorf("write anchor record: %w", err)
	}
	return nil
}

func (p *FilePublisher) LatestCheckpoint(ctx context.Context) (Record, error) {
	select {
	case <-ctx.Done():
		return Record{}, ctx.Err()
	default:
	}

	f, err := os.Open(p.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Record{}, ErrNoPublishedCheckpoint
		}
		return Record{}, fmt.Errorf("open anchor file: %w", err)
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	var (
		rec   Record
		found bool
	)
	for {
		select {
		case <-ctx.Done():
			return Record{}, ctx.Err()
		default:
		}

		err := dec.Decode(&rec)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return Record{}, fmt.Errorf("decode anchor record: %w", err)
		}
		found = true
	}
	if !found {
		return Record{}, ErrNoPublishedCheckpoint
	}
	return rec, nil
}
