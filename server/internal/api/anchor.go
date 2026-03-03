package api

import (
	"errors"
	"log"
	"net/http"

	"merkle-log/server/internal/anchor"
)

func forceAnchorHandler(worker *anchor.Worker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed. Only POST", http.StatusMethodNotAllowed)
			return
		}
		if worker == nil {
			http.Error(w, "Anchoring disabled on this server", http.StatusServiceUnavailable)
			return
		}

		rec, err := worker.PublishNow(r.Context())
		if err != nil {
			log.Printf("force anchor publish failed: %v", err)
			http.Error(w, "Failed to publish checkpoint", http.StatusInternalServerError)
			return
		}

		jsonResponse(w, map[string]any{
			"published":        true,
			"domain_separator": rec.DomainSeparator,
			"version":          rec.Version,
			"tree_size":        rec.TreeSize,
			"root_hash_hex":    rec.RootHashHex,
			"checkpoint_hash":  rec.CheckpointHash,
		})
	}
}

func latestAnchorHandler(worker *anchor.Worker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed. Only GET", http.StatusMethodNotAllowed)
			return
		}
		if worker == nil {
			http.Error(w, "Anchoring disabled on this server", http.StatusServiceUnavailable)
			return
		}

		rec, err := worker.LatestPublishedCheckpoint(r.Context())
		if err != nil {
			if errors.Is(err, anchor.ErrNoPublishedCheckpoint) {
				http.Error(w, "No anchored checkpoint available", http.StatusNotFound)
				return
			}
			log.Printf("read latest anchored checkpoint failed: %v", err)
			http.Error(w, "Failed to read latest anchored checkpoint", http.StatusInternalServerError)
			return
		}

		jsonResponse(w, map[string]any{
			"domain_separator": rec.DomainSeparator,
			"version":          rec.Version,
			"tree_size":        rec.TreeSize,
			"root_hash_hex":    rec.RootHashHex,
			"checkpoint_hash":  rec.CheckpointHash,
		})
	}
}
