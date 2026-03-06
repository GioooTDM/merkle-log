package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"
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

func (h *NotaryHandler) GetEntriesByDate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed. Only GET", http.StatusMethodNotAllowed)
		return
	}

	dateFrom := strings.TrimSpace(r.URL.Query().Get("date_from"))
	dateTo := strings.TrimSpace(r.URL.Query().Get("date_to"))
	if dateFrom == "" && dateTo == "" {
		http.Error(w, "Missing date_from/date_to", http.StatusBadRequest)
		return
	}

	fromStart, err := parseISODateStart(dateFrom)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid date_from: %v", err), http.StatusBadRequest)
		return
	}
	toStart, err := parseISODateStart(dateTo)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid date_to: %v", err), http.StatusBadRequest)
		return
	}

	var toEndExclusive time.Time
	if !toStart.IsZero() {
		toEndExclusive = toStart.Add(24 * time.Hour)
	}
	if !fromStart.IsZero() && !toStart.IsZero() && fromStart.After(toStart) {
		http.Error(w, "Invalid range: date_from must be <= date_to", http.StatusBadRequest)
		return
	}

	indexes, err := h.indexer.GetIndexesByRecordedAtRange(fromStart, toEndExclusive)
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	if len(indexes) == 0 {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	entries := make([]json.RawMessage, 0, len(indexes))
	okIndexes := make([]uint64, 0, len(indexes))
	for _, idx := range indexes {
		b, err := h.readEntryByIndex(r.Context(), idx)
		if err != nil {
			continue
		}
		entries = append(entries, json.RawMessage(b))
		okIndexes = append(okIndexes, idx)
	}
	if len(entries) == 0 {
		http.Error(w, "No entries available for selected date range", http.StatusNotFound)
		return
	}

	jsonResponse(w, map[string]any{
		"date_from": dateFrom,
		"date_to":   dateTo,
		"indexes":   okIndexes,
		"count":     len(entries),
		"entries":   entries,
	})
}

func (h *NotaryHandler) GetEntriesByIssuer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed. Only GET", http.StatusMethodNotAllowed)
		return
	}

	issuerEntityID, ok := requireQueryParam(w, r, "issuer_entity_id", "Missing issuer_entity_id")
	if !ok {
		return
	}

	dateFrom := strings.TrimSpace(r.URL.Query().Get("date_from"))
	dateTo := strings.TrimSpace(r.URL.Query().Get("date_to"))

	fromStart, err := parseISODateStart(dateFrom)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid date_from: %v", err), http.StatusBadRequest)
		return
	}
	toStart, err := parseISODateStart(dateTo)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid date_to: %v", err), http.StatusBadRequest)
		return
	}

	var toEndExclusive time.Time
	if !toStart.IsZero() {
		toEndExclusive = toStart.Add(24 * time.Hour)
	}
	if !fromStart.IsZero() && !toStart.IsZero() && fromStart.After(toStart) {
		http.Error(w, "Invalid range: date_from must be <= date_to", http.StatusBadRequest)
		return
	}

	indexes, err := h.indexer.GetIndexesByIssuerEntityID(issuerEntityID, fromStart, toEndExclusive)
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	if len(indexes) == 0 {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	entries := make([]json.RawMessage, 0, len(indexes))
	okIndexes := make([]uint64, 0, len(indexes))
	for _, idx := range indexes {
		raw, err := h.readEntryByIndex(r.Context(), idx)
		if err != nil {
			continue
		}
		entries = append(entries, json.RawMessage(raw))
		okIndexes = append(okIndexes, idx)
	}
	if len(entries) == 0 {
		http.Error(w, "No entries available for this issuer", http.StatusNotFound)
		return
	}

	jsonResponse(w, map[string]any{
		"issuer_entity_id": issuerEntityID,
		"date_from":        dateFrom,
		"date_to":          dateTo,
		"indexes":          okIndexes,
		"count":            len(entries),
		"entries":          entries,
	})
}

func parseISODateStart(raw string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return time.Time{}, err
	}
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), nil
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
