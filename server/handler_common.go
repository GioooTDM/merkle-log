package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"merkle-log/server/internal/hashutil"
	"merkle-log/server/internal/logread"
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
	return hashutil.SHA256Hex(data)
}

func jsonResponse(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("jsonResponse encode error: %v", err)
	}
}

// readEntryByIndex keeps the handler-facing entry lookup API.
// The protocol-level read path is centralized in internal/logread.
func (h *NotaryHandler) readEntryByIndex(ctx context.Context, idx uint64) ([]byte, error) {
	size, err := logread.PublishedTreeSize(ctx, h.reader)
	if err != nil {
		return nil, err
	}
	return logread.ReadLogEntryByIndex(ctx, h.reader, size, idx)
}
