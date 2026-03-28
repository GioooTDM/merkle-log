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

func (h *Handler) AddEvent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed. Only POST", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Read error", http.StatusInternalServerError)
		return
	}

	req, err := event.DecodeAddEventRequest(body)
	if err != nil {
		http.Error(w, "Invalid add payload: "+err.Error(), http.StatusBadRequest)
		return
	}

	// TODO: questo controllo non è atomico con append+indexing. Due POST /add quasi simultanee
	// sullo stesso doc_id possono ancora leggere la stessa head e creare un fork.
	docVersion, err := h.resolveDocumentVersion(r.Context(), req)
	if err != nil {
		http.Error(w, "Invalid document chain: "+err.Error(), http.StatusBadRequest)
		return
	}

	prepared, canonicalBody, err := event.PrepareDecodedAddEventForNotarizationWithMode(req, time.Now().UTC(), docVersion, h.useIssuedAtAsRecordedAt)
	if err != nil {
		http.Error(w, "Invalid add payload: "+err.Error(), http.StatusBadRequest)
		return
	}

	docHash, err := prepared.DocHash()
	if err != nil {
		log.Printf("prepare/index metadata mismatch for event_id=%s doc_id=%s: %v", prepared.EventID, prepared.DocID, err)
		http.Error(w, "Internal error preparing index metadata", http.StatusInternalServerError)
		return
	}

	leafHash := hashBytes(canonicalBody)

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
		DocID:          prepared.DocID,
		EventID:        prepared.EventID,
		DocHash:        docHash,
		LeafHash:       leafHash,
		IssuerEntityID: prepared.Issuer.EntityID,
		RecordedAt:     prepared.RecordedAt,
	}); err != nil {
		log.Printf("indexing error for event_id=%s doc_id=%s index=%d: %v", prepared.EventID, prepared.DocID, idx.Index, err)
		// Non blocchiamo la risposta se il log è ok ma l'indice fallisce,
		// ma in produzione andrebbe gestito meglio
	}

	jsonResponse(w, map[string]any{
		"log_index":      idx.Index,
		"notarized_json": json.RawMessage(canonicalBody),
	})
}

func (h *Handler) resolveDocumentVersion(ctx context.Context, req event.AddEventRequest) (int, error) {
	if req.DocID == "" {
		return 0, fmt.Errorf("doc_id is required")
	}

	switch req.EventType {
	case "CREATE":
		_, found, err := h.indexer.GetLatestIndexByDocID(req.DocID)
		if err != nil {
			return 0, fmt.Errorf("lookup latest entry for doc_id %q: %w", req.DocID, err)
		}
		if found {
			return 0, fmt.Errorf("doc_id %q already exists", req.DocID)
		}
		return 1, nil

	case "UPDATE", "REVOKE", "EXPIRE":
		latestIndex, found, err := h.indexer.GetLatestIndexByDocID(req.DocID)
		if err != nil {
			return 0, fmt.Errorf("lookup latest entry for doc_id %q: %w", req.DocID, err)
		}
		if !found {
			return 0, fmt.Errorf("doc_id %q has no existing chain", req.DocID)
		}

		head, err := h.eventReader.ReadEventByIndex(ctx, latestIndex)
		if err != nil {
			return 0, fmt.Errorf("read latest entry for doc_id %q: %w", req.DocID, err)
		}

		if strings.TrimSpace(head.DocID) != req.DocID {
			return 0, fmt.Errorf("doc_id %q latest entry mismatch", req.DocID)
		}
		if req.PrevEventID == nil {
			return 0, fmt.Errorf("%s requires prev_event_id", req.EventType)
		}
		if strings.TrimSpace(*req.PrevEventID) != strings.TrimSpace(head.EventID) {
			return 0, fmt.Errorf("prev_event_id must match latest event_id for doc_id %q", req.DocID)
		}
		return head.DocVersion + 1, nil

	default:
		return 0, fmt.Errorf("event_type invalid (got %q)", req.EventType)
	}
}
