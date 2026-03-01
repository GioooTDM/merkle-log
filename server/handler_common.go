package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	formatsLog "github.com/transparency-dev/formats/log"
)

func parseIndexFromPath(path, prefix string) (uint64, error) {
	idxStr := strings.Trim(strings.TrimPrefix(path, prefix), "/")
	if idxStr == "" {
		return 0, fmt.Errorf("missing index")
	}
	idx, err := strconv.ParseUint(idxStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid index")
	}
	return idx, nil
}

func hashBytes(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func jsonResponse(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("jsonResponse encode error: %v", err)
	}
}

func (h *NotaryHandler) readPublishedCheckpoint(ctx context.Context) ([]byte, formatsLog.Checkpoint, error) {
	return readPublishedCheckpoint(ctx, h.reader)
}

func (h *NotaryHandler) publishedSize(ctx context.Context) (uint64, error) {
	return publishedTreeSize(ctx, h.reader)
}

func (h *NotaryHandler) readEntryByIndex(ctx context.Context, idx uint64) ([]byte, error) {
	size, err := h.publishedSize(ctx)
	if err != nil {
		return nil, err
	}
	return readLogEntryByIndex(ctx, h.reader, size, idx)
}
