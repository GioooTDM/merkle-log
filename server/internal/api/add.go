package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"merkle-log/server/internal/event"
	"merkle-log/server/internal/index"

	"github.com/transparency-dev/tessera"
)

type addEventRequestOverview struct {
	EventType   string  `json:"event_type"`
	DocUID      string  `json:"doc_uid"`
	PrevEventID *string `json:"prev_event_id,omitempty"`
}

func (h *NotaryHandler) AddEvent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed. Only POST", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Read error", http.StatusInternalServerError)
		return
	}

	req, err := inspectAddEventRequest(body)
	if err != nil {
		http.Error(w, "Invalid add payload: "+err.Error(), http.StatusBadRequest)
		return
	}

	// TODO: questo controllo non è atomico con append+indexing. Due POST /add quasi simultanee
	// sullo stesso doc_uid possono ancora leggere la stessa head e creare un fork.
	docVersion, err := h.resolveDocumentVersion(r.Context(), req)
	if err != nil {
		http.Error(w, "Invalid document chain: "+err.Error(), http.StatusBadRequest)
		return
	}

	prepared, canonicalBody, err := event.PrepareAddEventForNotarizationWithMode(body, time.Now().UTC(), docVersion, h.useIssuedAtAsRecordedAt)
	if err != nil {
		http.Error(w, "Invalid add payload: "+err.Error(), http.StatusBadRequest)
		return
	}

	docHash, err := prepared.DocHash()
	if err != nil {
		log.Printf("prepare/index metadata mismatch for event_id=%s doc_uid=%s: %v", prepared.EventID, prepared.DocUID, err)
		http.Error(w, "Internal error preparing index metadata", http.StatusInternalServerError)
		return
	}

	leafHash := hashBytes(canonicalBody)

	// TODO: queste operazioni sono bloccanti per il server? Può gestire più richieste parallelamente?
	// 2. Append al Merkle Log (operazione asincrona che restituisce un future)
	future := h.appender.Add(r.Context(), tessera.NewEntry(canonicalBody))
	idx, err := future() // Chiamiamo il future per attendere l'effettivo inserimento e ottenere l'indice
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 3. Indicizzazione asincrona (opzionale) o sincrona
	if err := h.indexer.AddEntry(index.Entry{
		LogIndex:       idx.Index,
		DocUID:         prepared.DocUID,
		EventID:        prepared.EventID,
		DocHash:        docHash,
		LeafHash:       leafHash,
		IssuerEntityID: prepared.Issuer.EntityID,
		RecordedAt:     prepared.RecordedAt,
	}); err != nil {
		log.Printf("indexing error for event_id=%s doc_uid=%s index=%d: %v", prepared.EventID, prepared.DocUID, idx.Index, err)
		// Non blocchiamo la risposta se il log è ok ma l'indice fallisce,
		// ma in produzione andrebbe gestito meglio
	}

	jsonResponse(w, map[string]any{
		"log_index":      idx.Index,
		"notarized_json": json.RawMessage(canonicalBody),
	})
}

func inspectAddEventRequest(raw []byte) (addEventRequestOverview, error) {
	if len(raw) == 0 {
		return addEventRequestOverview{}, fmt.Errorf("empty body")
	}

	var req addEventRequestOverview
	if err := json.Unmarshal(raw, &req); err != nil {
		return addEventRequestOverview{}, fmt.Errorf("invalid JSON structure: %w", err)
	}

	req.EventType = strings.ToUpper(strings.TrimSpace(req.EventType))
	req.DocUID = strings.TrimSpace(req.DocUID)
	if req.PrevEventID != nil {
		trimmedPrev := strings.TrimSpace(*req.PrevEventID)
		req.PrevEventID = &trimmedPrev
	}

	return req, nil
}

type chainHeadEntry struct {
	EventID    string `json:"event_id"`
	DocUID     string `json:"doc_uid"`
	DocVersion int    `json:"doc_version"`
}

func (h *NotaryHandler) resolveDocumentVersion(ctx context.Context, req addEventRequestOverview) (int, error) {
	if req.DocUID == "" {
		return 0, fmt.Errorf("doc_uid is required")
	}

	switch req.EventType {
	case "CREATE":
		_, found, err := h.indexer.GetLatestIndexByDocUID(req.DocUID)
		if err != nil {
			return 0, fmt.Errorf("lookup latest entry for doc_uid %q: %w", req.DocUID, err)
		}
		if found {
			return 0, fmt.Errorf("doc_uid %q already exists", req.DocUID)
		}
		return 1, nil

	case "UPDATE", "REVOKE", "EXPIRE":
		latestIndex, found, err := h.indexer.GetLatestIndexByDocUID(req.DocUID)
		if err != nil {
			return 0, fmt.Errorf("lookup latest entry for doc_uid %q: %w", req.DocUID, err)
		}
		if !found {
			return 0, fmt.Errorf("doc_uid %q has no existing chain", req.DocUID)
		}

		raw, err := h.readEntryByIndex(ctx, latestIndex)
		if err != nil {
			return 0, fmt.Errorf("read latest entry for doc_uid %q: %w", req.DocUID, err)
		}

		var head chainHeadEntry
		if err := json.Unmarshal(raw, &head); err != nil {
			return 0, fmt.Errorf("decode latest entry for doc_uid %q: %w", req.DocUID, err)
		}

		if strings.TrimSpace(head.DocUID) != req.DocUID {
			return 0, fmt.Errorf("doc_uid %q latest entry mismatch", req.DocUID)
		}
		if req.PrevEventID == nil {
			return 0, fmt.Errorf("%s requires prev_event_id", req.EventType)
		}
		if strings.TrimSpace(*req.PrevEventID) != strings.TrimSpace(head.EventID) {
			return 0, fmt.Errorf("prev_event_id must match latest event_id for doc_uid %q", req.DocUID)
		}
		return head.DocVersion + 1, nil

	default:
		return 0, fmt.Errorf("event_type invalid (got %q)", req.EventType)
	}
}
