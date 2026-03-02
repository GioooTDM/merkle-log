package api

import (
	"encoding/json"
	"log"
	"net/http"
	"sort"
	"strings"
)

func requireQueryParam(w http.ResponseWriter, r *http.Request, key, missingMsg string) (string, bool) {
	value := strings.TrimSpace(r.URL.Query().Get(key))
	if value == "" {
		http.Error(w, missingMsg, http.StatusBadRequest)
		return "", false
	}
	return value, true
}

// TODO: attenzione, ci potrebbero essere più eventi notarizzati con lo stesso doc hash. Lo stesso documento può essere notarizzato più volte in contesti diversi.
func (h *NotaryHandler) GetByDoc(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed. Only GET", http.StatusMethodNotAllowed)
		return
	}

	hash, ok := requireQueryParam(w, r, "hash", "Parametro 'hash' mancante")
	if !ok {
		return
	}

	leafHash, logIndex, err := h.indexer.GetByDocHash(hash)
	if err != nil {
		http.Error(w, "Doc not found", http.StatusNotFound)
		return
	}

	jsonResponse(w, map[string]any{
		"log_index": logIndex,
		"leaf_hash": leafHash,
	})
}

func (h *NotaryHandler) GetByLeaf(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed. Only GET", http.StatusMethodNotAllowed)
		return
	}

	hash, ok := requireQueryParam(w, r, "hash", "Parametro 'hash' mancante")
	if !ok {
		return
	}

	logIndex, err := h.indexer.GetByLeafHash(hash)
	if err != nil {
		http.Error(w, "Leaf not found", http.StatusNotFound)
		return
	}

	jsonResponse(w, map[string]any{"log_index": logIndex})
}

func (h *NotaryHandler) GetIndexesByDocUID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed. Only GET", http.StatusMethodNotAllowed)
		return
	}

	docUID, ok := requireQueryParam(w, r, "doc_uid", "Missing doc_uid")
	if !ok {
		return
	}

	indexes, err := h.indexer.GetIndexesByDocUID(docUID)
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	if len(indexes) == 0 {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	jsonResponse(w, map[string]any{
		"doc_uid": docUID,
		"indexes": indexes,
		"count":   len(indexes),
	})
}

func (h *NotaryHandler) GetEntriesByDocUID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed. Only GET", http.StatusMethodNotAllowed)
		return
	}

	docUID, ok := requireQueryParam(w, r, "doc_uid", "Missing doc_uid")
	if !ok {
		return
	}

	// 1) recupera indici da DB
	indexes, err := h.indexer.GetIndexesByDocUID(docUID)
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	if len(indexes) == 0 {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	// Ordine stabile: crescente (poi il client può reverse per "latest first")
	sort.Slice(indexes, func(i, j int) bool { return indexes[i] < indexes[j] })

	// 2) recupera entry dal log
	entries := make([]json.RawMessage, 0, len(indexes))
	okIndexes := make([]uint64, 0, len(indexes))

	for _, idx := range indexes {
		b, err := h.readEntryByIndex(r.Context(), idx)
		if err != nil {
			// Se vuoi essere "strict", puoi fallire subito:
			// http.Error(w, "Failed to read entry from log", http.StatusInternalServerError); return
			// Per ora: salta entry non leggibile
			continue
		}
		if !entryMatchesDocUID(b, docUID) {
			log.Printf("index mismatch: requested doc_uid=%q index=%d", docUID, idx)
			continue
		}
		entries = append(entries, json.RawMessage(b))
		okIndexes = append(okIndexes, idx)
	}

	if len(entries) == 0 {
		http.Error(w, "No entries available for this doc_uid", http.StatusNotFound)
		return
	}

	// 3) response
	jsonResponse(w, map[string]any{
		"doc_uid": docUID,
		"indexes": okIndexes,
		"count":   len(entries),
		"entries": entries,
	})
}

func entryMatchesDocUID(raw []byte, wantDocUID string) bool {
	var entry struct {
		DocUID string `json:"doc_uid"`
	}
	if err := json.Unmarshal(raw, &entry); err != nil {
		return false
	}
	return strings.TrimSpace(entry.DocUID) == strings.TrimSpace(wantDocUID)
}
