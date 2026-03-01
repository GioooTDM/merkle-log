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

// readEntryByIndex keeps the handler-facing entry lookup API.
// The protocol-level read path is centralized in log_read_helpers.go.
func (h *NotaryHandler) readEntryByIndex(ctx context.Context, idx uint64) ([]byte, error) {
	size, err := publishedTreeSize(ctx, h.reader)
	if err != nil {
		return nil, err
	}
	return readLogEntryByIndex(ctx, h.reader, size, idx)
}
