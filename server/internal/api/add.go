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

	parsed, canonicalBody, err := event.PrepareAddEventForNotarizationWithMode(body, time.Now().UTC(), h.useIssuedAtAsRecordedAt)
	if err != nil {
		http.Error(w, "Invalid add payload: "+err.Error(), http.StatusBadRequest)
		return
	}

	// TODO: questo controllo non è atomico con append+indexing. Due POST /add quasi simultanee
	// sullo stesso doc_uid possono ancora leggere la stessa head e creare un fork.
	if err := h.validateLatestDocumentChain(r.Context(), parsed); err != nil {
		http.Error(w, "Invalid document chain: "+err.Error(), http.StatusBadRequest)
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
		DocUID:         parsed.DocUID,
		EventID:        parsed.EventID,
		DocHash:        parsed.DocHash,
		LeafHash:       leafHash,
		IssuerEntityID: parsed.IssuerEntityID,
		RecordedAt:     parsed.RecordedAt,
	}); err != nil {
		log.Printf("indexing error for event_id=%s doc_uid=%s index=%d: %v", parsed.EventID, parsed.DocUID, idx.Index, err)
		// Non blocchiamo la risposta se il log è ok ma l'indice fallisce,
		// ma in produzione andrebbe gestito meglio
	}

	jsonResponse(w, map[string]any{
		"log_index":      idx.Index,
		"notarized_json": json.RawMessage(canonicalBody),
	})
}

type chainHeadEntry struct {
	EventID    string `json:"event_id"`
	DocUID     string `json:"doc_uid"`
	DocVersion int    `json:"doc_version"`
}

func (h *NotaryHandler) validateLatestDocumentChain(ctx context.Context, parsed event.PreparedAddEvent) error {
	if parsed.EventType != "CREATE" && parsed.EventType != "UPDATE" {
		return nil
	}

	latestIndex, found, err := h.indexer.GetLatestIndexByDocUID(parsed.DocUID)
	if err != nil {
		return fmt.Errorf("lookup latest entry for doc_uid %q: %w", parsed.DocUID, err)
	}

	if !found {
		if parsed.EventType == "CREATE" {
			return nil
		}
		return fmt.Errorf("doc_uid %q has no existing chain", parsed.DocUID)
	}

	if parsed.EventType == "CREATE" {
		return fmt.Errorf("doc_uid %q already exists", parsed.DocUID)
	}

	raw, err := h.readEntryByIndex(ctx, latestIndex)
	if err != nil {
		return fmt.Errorf("read latest entry for doc_uid %q: %w", parsed.DocUID, err)
	}

	var head chainHeadEntry
	if err := json.Unmarshal(raw, &head); err != nil {
		return fmt.Errorf("decode latest entry for doc_uid %q: %w", parsed.DocUID, err)
	}

	if strings.TrimSpace(head.DocUID) != parsed.DocUID {
		return fmt.Errorf("doc_uid %q latest entry mismatch", parsed.DocUID)
	}
	if parsed.PrevEventID == nil {
		return fmt.Errorf("%s requires prev_event_id", parsed.EventType)
	}
	if strings.TrimSpace(*parsed.PrevEventID) != strings.TrimSpace(head.EventID) {
		return fmt.Errorf("prev_event_id must match latest event_id for doc_uid %q", parsed.DocUID)
	}
	// TODO: forse DocVersion è un campo che può essere gestito dal server.
	if parsed.DocVersion != head.DocVersion+1 {
		return fmt.Errorf("doc_version must be %d for doc_uid %q", head.DocVersion+1, parsed.DocUID)
	}

	return nil
}
