package api

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"

	"merkle-log/server/internal/event"

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

	parsed, canonicalBody, err := event.PrepareAddEventForNotarization(body, time.Now().UTC())
	if err != nil {
		http.Error(w, "Invalid add payload: "+err.Error(), http.StatusBadRequest)
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
	if err := h.indexer.AddEntry(parsed.DocUID, parsed.EventID, parsed.DocHash, leafHash, parsed.RecordedAt, idx.Index); err != nil {
		log.Printf("indexing error for event_id=%s doc_uid=%s index=%d: %v", parsed.EventID, parsed.DocUID, idx.Index, err)
		// Non blocchiamo la risposta se il log è ok ma l'indice fallisce,
		// ma in produzione andrebbe gestito meglio
	}

	jsonResponse(w, map[string]any{
		"log_index":      idx.Index,
		"notarized_json": json.RawMessage(canonicalBody),
	})
}
