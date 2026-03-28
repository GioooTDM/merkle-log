package api

import (
	"errors"
	"net/http"
	"os"
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

	entry, err := h.eventReader.ReadRawByIndex(r.Context(), idx)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.Error(w, "Entry not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to read entry", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(entry)
}
