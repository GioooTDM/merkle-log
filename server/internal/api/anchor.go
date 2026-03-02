package api

import (
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
			"published_at_utc": rec.PublishedAtUTC,
			"tree_size":        rec.TreeSize,
			"root_hash_hex":    rec.RootHashHex,
			"checkpoint_hash":  rec.CheckpointHash,
		})
	}
}
