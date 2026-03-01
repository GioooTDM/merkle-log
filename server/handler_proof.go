package main

import (
	"context"
	"encoding/hex"
	"errors"
	"net/http"
	"os"

	"merkle-log/server/internal/logread"

	tclient "github.com/transparency-dev/tessera/client"
)

func (h *NotaryHandler) GetEntry(w http.ResponseWriter, r *http.Request) {
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

func (h *NotaryHandler) GetProof(w http.ResponseWriter, r *http.Request) {
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

	// Tile fetcher richiesto dal client.ProofBuilder:
	// - se p!=0 e il parziale non esiste, deve fare fallback al full.
	tileF := func(ctx context.Context, level, index uint64, p uint8) ([]byte, error) {
		b, err := h.reader.ReadTile(ctx, level, index, p)
		if err == nil {
			return b, nil
		}
		// Fallback: prova tile full se il parziale manca (o comunque se p!=0).
		if p != 0 {
			b2, err2 := h.reader.ReadTile(ctx, level, index, 0)
			if err2 == nil {
				return b2, nil
			}
			// Se vuoi rispettare "os.ErrNotExist", prova a propagare quello.
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

	pb, err := tclient.NewProofBuilder(r.Context(), cp.Size, tileF)
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
