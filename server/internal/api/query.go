package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"merkle-log/server/internal/index"
)

func requireQueryParam(w http.ResponseWriter, r *http.Request, key, missingMsg string) (string, bool) {
	value := strings.TrimSpace(r.URL.Query().Get(key))
	if value == "" {
		http.Error(w, missingMsg, http.StatusBadRequest)
		return "", false
	}
	return value, true
}

type entriesQuery struct {
	docID          string
	issuerEntityID string
	dateFromRaw    string
	dateToRaw      string
	fromInclusive  time.Time
	toExclusive    time.Time
	limit          int
	offset         int
	order          string
}

type entriesResponse struct {
	DocID          string            `json:"doc_id,omitempty"`
	IssuerEntityID string            `json:"issuer_entity_id,omitempty"`
	DateFrom       string            `json:"date_from,omitempty"`
	DateTo         string            `json:"date_to,omitempty"`
	Indexes        []uint64          `json:"indexes"`
	Count          int               `json:"count"`
	TotalCount     int               `json:"total_count"`
	Offset         int               `json:"offset"`
	Limit          int               `json:"limit"`
	HasMore        bool              `json:"has_more"`
	Entries        []json.RawMessage `json:"entries"`
}

func parseEntriesQuery(r *http.Request) (entriesQuery, error) {
	q := r.URL.Query()
	out := entriesQuery{
		docID:          strings.TrimSpace(q.Get("doc_id")),
		issuerEntityID: strings.TrimSpace(q.Get("issuer_entity_id")),
		dateFromRaw:    strings.TrimSpace(q.Get("date_from")),
		dateToRaw:      strings.TrimSpace(q.Get("date_to")),
		order:          strings.ToLower(strings.TrimSpace(q.Get("order"))),
	}

	if out.order == "" {
		out.order = "desc"
	}
	if out.order != "asc" && out.order != "desc" {
		return entriesQuery{}, fmt.Errorf("invalid order: must be asc or desc")
	}

	if rawOffset := strings.TrimSpace(q.Get("offset")); rawOffset != "" {
		offset, err := strconv.Atoi(rawOffset)
		if err != nil || offset < 0 {
			return entriesQuery{}, fmt.Errorf("invalid offset: must be >= 0")
		}
		out.offset = offset
	}

	if rawLimit := strings.TrimSpace(q.Get("limit")); rawLimit != "" {
		limit, err := strconv.Atoi(rawLimit)
		if err != nil || limit <= 0 {
			return entriesQuery{}, fmt.Errorf("invalid limit: must be > 0")
		}
		out.limit = limit
	}

	var err error
	out.fromInclusive, err = parseISODateStart(out.dateFromRaw)
	if err != nil {
		return entriesQuery{}, fmt.Errorf("invalid date_from: %w", err)
	}
	toStart, err := parseISODateStart(out.dateToRaw)
	if err != nil {
		return entriesQuery{}, fmt.Errorf("invalid date_to: %w", err)
	}
	if !toStart.IsZero() {
		out.toExclusive = toStart.Add(24 * time.Hour)
	}
	if !out.fromInclusive.IsZero() && !toStart.IsZero() && out.fromInclusive.After(toStart) {
		return entriesQuery{}, fmt.Errorf("invalid range: date_from must be <= date_to")
	}

	return out, nil
}

func (q entriesQuery) toIndexSearchParams() index.SearchParams {
	return index.SearchParams{
		DocID:          q.docID,
		IssuerEntityID: q.issuerEntityID,
		FromInclusive:  q.fromInclusive,
		ToExclusive:    q.toExclusive,
		Limit:          q.limit,
		Offset:         q.offset,
		Order:          q.order,
	}
}

func (q entriesQuery) toResponse(indexes []uint64, entries []json.RawMessage, totalCount int) entriesResponse {
	limit := q.limit
	if limit == 0 {
		limit = len(indexes)
	}
	return entriesResponse{
		DocID:          q.docID,
		IssuerEntityID: q.issuerEntityID,
		DateFrom:       q.dateFromRaw,
		DateTo:         q.dateToRaw,
		Indexes:        indexes,
		Count:          len(entries),
		TotalCount:     totalCount,
		Offset:         q.offset,
		Limit:          limit,
		HasMore:        q.offset+len(indexes) < totalCount,
		Entries:        entries,
	}
}

// TODO: attenzione, ci potrebbero essere più eventi notarizzati con lo stesso doc hash. Lo stesso documento può essere notarizzato più volte in contesti diversi.
func (h *Handler) GetByDoc(w http.ResponseWriter, r *http.Request) {
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

func (h *Handler) GetByLeaf(w http.ResponseWriter, r *http.Request) {
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

func (h *Handler) GetIndexesByDocID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed. Only GET", http.StatusMethodNotAllowed)
		return
	}

	docID, ok := requireQueryParam(w, r, "doc_id", "Missing doc_id")
	if !ok {
		return
	}

	indexes, err := h.indexer.GetIndexesByDocID(docID)
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	if len(indexes) == 0 {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	jsonResponse(w, map[string]any{
		"doc_id":  docID,
		"indexes": indexes,
		"count":   len(indexes),
	})
}

func (h *Handler) SearchEntries(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed. Only GET", http.StatusMethodNotAllowed)
		return
	}

	query, err := parseEntriesQuery(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	searchResult, err := h.indexer.SearchIndexes(query.toIndexSearchParams())
	if err != nil {
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}

	entries := make([]json.RawMessage, 0, len(searchResult.Indexes))
	okIndexes := make([]uint64, 0, len(searchResult.Indexes))
	for _, idx := range searchResult.Indexes {
		raw, err := h.eventReader.ReadRawByIndex(r.Context(), idx)
		if err != nil {
			continue
		}
		entries = append(entries, json.RawMessage(raw))
		okIndexes = append(okIndexes, idx)
	}

	jsonResponse(w, query.toResponse(okIndexes, entries, searchResult.TotalCount))
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
