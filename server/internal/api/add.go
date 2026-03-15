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

	prepared, canonicalBody, err := event.PrepareAddEventForNotarizationWithMode(body, time.Now().UTC(), h.useIssuedAtAsRecordedAt)
	if err != nil {
		http.Error(w, "Invalid add payload: "+err.Error(), http.StatusBadRequest)
		return
	}

	// TODO: questo controllo non è atomico con append+indexing. Due POST /add quasi simultanee
	// sullo stesso doc_uid possono ancora leggere la stessa head e creare un fork.
	if err := h.validateLatestDocumentChain(r.Context(), prepared); err != nil {
		http.Error(w, "Invalid document chain: "+err.Error(), http.StatusBadRequest)
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

type chainHeadEntry struct {
	EventID    string `json:"event_id"`
	DocUID     string `json:"doc_uid"`
	DocVersion int    `json:"doc_version"`
}

func (h *NotaryHandler) validateLatestDocumentChain(ctx context.Context, prepared event.PreparedEvent) error {
	if prepared.EventType != "CREATE" && prepared.EventType != "UPDATE" {
		return nil
	}

	latestIndex, found, err := h.indexer.GetLatestIndexByDocUID(prepared.DocUID)
	if err != nil {
		return fmt.Errorf("lookup latest entry for doc_uid %q: %w", prepared.DocUID, err)
	}

	if !found {
		if prepared.EventType == "CREATE" {
			return nil
		}
		return fmt.Errorf("doc_uid %q has no existing chain", prepared.DocUID)
	}

	if prepared.EventType == "CREATE" {
		return fmt.Errorf("doc_uid %q already exists", prepared.DocUID)
	}

	raw, err := h.readEntryByIndex(ctx, latestIndex)
	if err != nil {
		return fmt.Errorf("read latest entry for doc_uid %q: %w", prepared.DocUID, err)
	}

	var head chainHeadEntry
	if err := json.Unmarshal(raw, &head); err != nil {
		return fmt.Errorf("decode latest entry for doc_uid %q: %w", prepared.DocUID, err)
	}

	if strings.TrimSpace(head.DocUID) != prepared.DocUID {
		return fmt.Errorf("doc_uid %q latest entry mismatch", prepared.DocUID)
	}
	if prepared.PrevEventID == nil {
		return fmt.Errorf("%s requires prev_event_id", prepared.EventType)
	}
	if strings.TrimSpace(*prepared.PrevEventID) != strings.TrimSpace(head.EventID) {
		return fmt.Errorf("prev_event_id must match latest event_id for doc_uid %q", prepared.DocUID)
	}
	// TODO: forse DocVersion è un campo che può essere gestito dal server.
	if prepared.DocVersion != head.DocVersion+1 {
		return fmt.Errorf("doc_version must be %d for doc_uid %q", head.DocVersion+1, prepared.DocUID)
	}

	return nil
}
