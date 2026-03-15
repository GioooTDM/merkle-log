package api

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"

	"merkle-log/server/internal/logread"

	tclient "github.com/transparency-dev/tessera/client"
)

func (h *Handler) GetEntry(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed. Only GET", http.StatusMethodNotAllowed)
		return
	}

	idx, err := parseIndexFromPath(r.URL.Path, "/get-entry/")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	entry, err := h.readEntryByIndex(r.Context(), idx)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.Error(w, "Entry not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to read entry", http.StatusInternalServerError)
		return
	}

	// Se le tue entry sono JSON, ok. Altrimenti usa application/octet-stream.
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(entry)
}

func (h *Handler) GetProof(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed. Only GET", http.StatusMethodNotAllowed)
		return
	}

	idx, err := parseIndexFromPath(r.URL.Path, "/get-proof/")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Checkpoint pubblicato -> tree size "commit-tato"
	cpRaw, cp, err := logread.ReadPublishedCheckpoint(r.Context(), h.reader)
	if err != nil {
		http.Error(w, "Checkpoint not available", http.StatusServiceUnavailable)
		return
	}

	if idx >= cp.Size {
		http.Error(w, "Index out of range", http.StatusNotFound)
		return
	}

	pb, err := tclient.NewProofBuilder(r.Context(), cp.Size, h.tileFetcher())
	if err != nil {
		http.Error(w, "Failed to init proof builder", http.StatusInternalServerError)
		return
	}

	hashes, err := pb.InclusionProof(r.Context(), idx)
	if err != nil {
		http.Error(w, "Failed to build proof", http.StatusInternalServerError)
		return
	}

	proofHex := make([]string, len(hashes))
	for i := range hashes {
		proofHex[i] = hex.EncodeToString(hashes[i])
	}

	jsonResponse(w, map[string]any{
		"log_index":  idx,
		"tree_size":  cp.Size,
		"root_hash":  hex.EncodeToString(cp.Hash),
		"checkpoint": string(cpRaw),
		"proof":      proofHex,
	})
}

func (h *Handler) GetConsistencyProof(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed. Only GET", http.StatusMethodNotAllowed)
		return
	}

	from, err := parseUintQuery(r, "from")
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid from: %v", err), http.StatusBadRequest)
		return
	}
	to, err := parseUintQuery(r, "to")
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid to: %v", err), http.StatusBadRequest)
		return
	}
	if from > to {
		http.Error(w, "Invalid range: from must be <= to", http.StatusBadRequest)
		return
	}

	_, cp, err := logread.ReadPublishedCheckpoint(r.Context(), h.reader)
	if err != nil {
		http.Error(w, "Checkpoint not available", http.StatusServiceUnavailable)
		return
	}
	if to > cp.Size {
		http.Error(w, "Requested 'to' size is beyond published checkpoint", http.StatusBadRequest)
		return
	}

	pb, err := tclient.NewProofBuilder(r.Context(), cp.Size, h.tileFetcher())
	if err != nil {
		http.Error(w, "Failed to init proof builder", http.StatusInternalServerError)
		return
	}

	hashes, err := pb.ConsistencyProof(r.Context(), from, to)
	if err != nil {
		http.Error(w, "Failed to build consistency proof", http.StatusInternalServerError)
		return
	}

	proofHex := make([]string, len(hashes))
	for i := range hashes {
		proofHex[i] = hex.EncodeToString(hashes[i])
	}

	jsonResponse(w, map[string]any{
		"from_tree_size": from,
		"to_tree_size":   to,
		"proof":          proofHex,
	})
}

func parseUintQuery(r *http.Request, key string) (uint64, error) {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return 0, fmt.Errorf("missing query param '%s'", key)
	}
	v, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, err
	}
	return v, nil
}

// tileFetcher returns a TileFetcherFunc compatible with tessera client proof builder.
// If partial tiles are unavailable, it falls back to the corresponding full tile.
func (h *Handler) tileFetcher() func(ctx context.Context, level, index uint64, p uint8) ([]byte, error) {
	return func(ctx context.Context, level, index uint64, p uint8) ([]byte, error) {
		b, err := h.reader.ReadTile(ctx, level, index, p)
		if err == nil {
			return b, nil
		}
		if p != 0 {
			b2, err2 := h.reader.ReadTile(ctx, level, index, 0)
			if err2 == nil {
				return b2, nil
			}
			if errors.Is(err2, os.ErrNotExist) {
				return nil, os.ErrNotExist
			}
			return nil, err2
		}
		if errors.Is(err, os.ErrNotExist) {
			return nil, os.ErrNotExist
		}
		return nil, err
	}
}
