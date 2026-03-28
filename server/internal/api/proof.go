package api

import (
	"errors"
	"net/http"
	"strconv"

	"merkle-log/server/internal/proofsvc"
)

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

	resp, err := h.proofService.InclusionProof(r.Context(), idx)
	if err != nil {
		if errors.Is(err, proofsvc.ErrCheckpointUnavailable) {
			http.Error(w, "Checkpoint not available", http.StatusServiceUnavailable)
			return
		}
		if errors.Is(err, proofsvc.ErrIndexOutOfRange) {
			http.Error(w, "Index out of range", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to build proof", http.StatusInternalServerError)
		return
	}

	jsonResponse(w, resp)
}

func (h *Handler) GetConsistencyProof(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed. Only GET", http.StatusMethodNotAllowed)
		return
	}

	from, err := parseUintQuery(r, "from")
	if err != nil {
		http.Error(w, "Invalid from: "+err.Error(), http.StatusBadRequest)
		return
	}
	to, err := parseUintQuery(r, "to")
	if err != nil {
		http.Error(w, "Invalid to: "+err.Error(), http.StatusBadRequest)
		return
	}
	if from > to {
		http.Error(w, "Invalid range: from must be <= to", http.StatusBadRequest)
		return
	}

	resp, err := h.proofService.ConsistencyProof(r.Context(), from, to)
	if err != nil {
		if errors.Is(err, proofsvc.ErrCheckpointUnavailable) {
			http.Error(w, "Checkpoint not available", http.StatusServiceUnavailable)
			return
		}
		if errors.Is(err, proofsvc.ErrSizeOutOfRange) {
			http.Error(w, "Requested 'to' size is beyond published checkpoint", http.StatusBadRequest)
			return
		}
		http.Error(w, "Failed to build consistency proof", http.StatusInternalServerError)
		return
	}

	jsonResponse(w, resp)
}

func parseUintQuery(r *http.Request, key string) (uint64, error) {
	raw := r.URL.Query().Get(key)
	if raw == "" {
		return 0, strconv.ErrSyntax
	}
	v, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, err
	}
	return v, nil
}
