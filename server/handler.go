package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/transparency-dev/tessera"
	"github.com/transparency-dev/tessera/api"
	"github.com/transparency-dev/tessera/api/layout"

	formatsLog "github.com/transparency-dev/formats/log"

	tclient "github.com/transparency-dev/tessera/client"
)

type NotaryHandler struct {
	appender *tessera.Appender
	indexer  *Indexer
	reader   tessera.LogReader
}

func NewNotaryHandler(a *tessera.Appender, i *Indexer, r tessera.LogReader) *NotaryHandler {
	return &NotaryHandler{appender: a, indexer: i, reader: r}
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

	// 1. Estrazione metadati dal JSON
	var event struct {
		DocUID      string `json:"doc_uid"`
		EventID     string `json:"event_id"`
		PayloadHash struct {
			Value string `json:"value"`
		} `json:"payload_hash"`
	}
	if err := json.Unmarshal(body, &event); err != nil {
		http.Error(w, "Invalid JSON structure", http.StatusBadRequest)
		return
	}

	// Validazione minima
	if event.DocUID == "" || event.EventID == "" {
		http.Error(w, "Missing required fields: doc_uid or event_id", http.StatusBadRequest)
		return
	}

	docHash, err := parsePayloadHash(event.PayloadHash.Value)
	if err != nil {
		http.Error(w, "Invalid payload_hash.value", http.StatusBadRequest)
		return
	}

	leafHash := hashBytes(body)

	// TODO: queste operazioni sono bloccanti per il server? Può gestire più richieste parallelamente?
	// 2. Append al Merkle Log (operazione asincrona che restituisce un future)
	future := h.appender.Add(r.Context(), tessera.NewEntry(body))
	idx, err := future() // Chiamiamo il future per attendere l'effettivo inserimento e ottenere l'indice
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 3. Indicizzazione asincrona (opzionale) o sincrona
	if err := h.indexer.AddEntry(event.DocUID, event.EventID, docHash, leafHash, idx.Index); err != nil {
		log.Printf("Indexing error: %v", err)
		// Non blocchiamo la risposta se il log è ok ma l'indice fallisce,
		// ma in produzione andrebbe gestito meglio
	}

	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintf(w, "%d\n", idx.Index)
}

// TODO: attenzione, ci potrebbero essere più eventi notarizzati con lo stesso doc hash. Lo stesso documento può essere notarizzato più volte in contesti diversi.
func (h *NotaryHandler) GetByDoc(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed. Only GET", http.StatusMethodNotAllowed)
		return
	}

	hash := r.URL.Query().Get("hash")

	if hash == "" {
		http.Error(w, "Parametro 'hash' mancante", http.StatusBadRequest)
		return
	}

	leafHash, logIndex, err := h.indexer.GetByDocHash(hash)
	if err != nil {
		http.Error(w, "Doc not found", http.StatusNotFound)
		return
	}

	jsonResponse(w, map[string]interface{}{
		"log_index": logIndex,
		"leaf_hash": leafHash,
	})
}

func (h *NotaryHandler) GetByLeaf(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed. Only GET", http.StatusMethodNotAllowed)
		return
	}

	hash := r.URL.Query().Get("hash")

	if hash == "" {
		http.Error(w, "Parametro 'hash' mancante", http.StatusBadRequest)
		return
	}

	logIndex, err := h.indexer.GetByLeafHash(hash)
	if err != nil {
		http.Error(w, "Leaf not found", http.StatusNotFound)
		return
	}

	jsonResponse(w, map[string]interface{}{"log_index": logIndex})
}

const EntryBundleWidth uint64 = 256

func (h *NotaryHandler) GetEntry(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed. Only GET", http.StatusMethodNotAllowed)
		return
	}

	idx, err := parseIndexFromPath(r.URL.Path, "/get-entry/")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	entry, err := h.readEntryByIndex(r.Context(), idx)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.Error(w, "Entry not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to read entry", http.StatusInternalServerError)
		return
	}

	// Se le tue entry sono JSON, ok. Altrimenti usa application/octet-stream.
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(entry)
}

func (h *NotaryHandler) GetProof(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed. Only GET", http.StatusMethodNotAllowed)
		return
	}

	idx, err := parseIndexFromPath(r.URL.Path, "/get-proof/")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Checkpoint pubblicato -> tree size “commit-tato”
	cpRaw, cp, err := h.readPublishedCheckpoint(r.Context())
	if err != nil {
		http.Error(w, "Checkpoint not available", http.StatusServiceUnavailable)
		return
	}

	if idx >= cp.Size {
		http.Error(w, "Index out of range", http.StatusNotFound)
		return
	}

	// Tile fetcher richiesto dal client.ProofBuilder:
	// - se p!=0 e il parziale non esiste, deve fare fallback al full.
	tileF := func(ctx context.Context, level, index uint64, p uint8) ([]byte, error) {
		b, err := h.reader.ReadTile(ctx, level, index, p)
		if err == nil {
			return b, nil
		}
		// Fallback: prova tile full se il parziale manca (o comunque se p!=0).
		if p != 0 {
			b2, err2 := h.reader.ReadTile(ctx, level, index, 0)
			if err2 == nil {
				return b2, nil
			}
			// Se vuoi rispettare “os.ErrNotExist”, prova a propagare quello.
			if errors.Is(err2, os.ErrNotExist) {
				return nil, os.ErrNotExist
			}
			return nil, err2
		}
		if errors.Is(err, os.ErrNotExist) {
			return nil, os.ErrNotExist
		}
		return nil, err
	}

	pb, err := tclient.NewProofBuilder(r.Context(), cp.Size, tileF)
	if err != nil {
		http.Error(w, "Failed to init proof builder", http.StatusInternalServerError)
		return
	}

	hashes, err := pb.InclusionProof(r.Context(), idx)
	if err != nil {
		http.Error(w, "Failed to build proof", http.StatusInternalServerError)
		return
	}

	proofHex := make([]string, len(hashes))
	for i := range hashes {
		proofHex[i] = hex.EncodeToString(hashes[i])
	}

	jsonResponse(w, map[string]any{
		"log_index":  idx,
		"tree_size":  cp.Size,
		"root_hash":  hex.EncodeToString(cp.Hash),
		"checkpoint": string(cpRaw),
		"proof":      proofHex,
	})
}

func (h *NotaryHandler) GetIndexesByDocUID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed. Only GET", http.StatusMethodNotAllowed)
		return
	}

	docUID := strings.TrimSpace(r.URL.Query().Get("doc_uid"))
	if docUID == "" {
		http.Error(w, "Missing doc_uid", http.StatusBadRequest)
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

func (h *NotaryHandler) publishedSize(ctx context.Context) (uint64, error) {
	_, cp, err := h.readPublishedCheckpoint(ctx)
	if err != nil {
		return 0, err
	}
	return cp.Size, nil
}

func (h *NotaryHandler) readEntryByIndex(ctx context.Context, idx uint64) ([]byte, error) {
	// 1) size pubblicato
	size, err := h.publishedSize(ctx)
	if err != nil {
		return nil, err
	}
	if idx >= size {
		return nil, os.ErrNotExist
	}

	// 2) coordinate bundle + offset
	bundleIdx := idx / EntryBundleWidth
	offset := idx % EntryBundleWidth

	// p = partial size (0 se bundle pieno, 1..255 se parziale)
	partial := layout.PartialTileSize(0 /*level*/, bundleIdx, size)

	raw, err := h.reader.ReadEntryBundle(ctx, bundleIdx, partial)
	if err != nil && partial != 0 {
		// Fallback al bundle pieno: alcuni store non servono i bundle parziali.
		raw, err = h.reader.ReadEntryBundle(ctx, bundleIdx, 0)
	}
	if err != nil {
		return nil, err
	}

	var eb api.EntryBundle
	if err := eb.UnmarshalText(raw); err != nil {
		return nil, err
	}
	if int(offset) >= len(eb.Entries) {
		return nil, os.ErrNotExist
	}
	return eb.Entries[offset], nil
}

func (h *NotaryHandler) GetEntriesByDocUID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed. Only GET", http.StatusMethodNotAllowed)
		return
	}

	docUID := strings.TrimSpace(r.URL.Query().Get("doc_uid"))
	if docUID == "" {
		http.Error(w, "Missing doc_uid", http.StatusBadRequest)
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

	// Ordine stabile: crescente (poi il client può reverse per “latest first”)
	sort.Slice(indexes, func(i, j int) bool { return indexes[i] < indexes[j] })

	// 2) recupera entry dal log
	entries := make([]json.RawMessage, 0, len(indexes))
	okIndexes := make([]uint64, 0, len(indexes))

	for _, idx := range indexes {
		b, err := h.readEntryByIndex(r.Context(), idx)
		if err != nil {
			// Se vuoi essere “strict”, puoi fallire subito:
			// http.Error(w, "Failed to read entry from log", http.StatusInternalServerError); return
			// Per ora: salta entry non leggibile
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

// Helpers
func parsePayloadHash(v string) (string, error) {
	s := strings.ToLower(strings.TrimSpace(v))
	s = strings.TrimPrefix(s, "hex:")
	if s == "" {
		return "", nil
	}
	raw, err := hex.DecodeString(s)
	if err != nil || len(raw) != sha256.Size {
		return "", fmt.Errorf("invalid sha-256 hex")
	}
	return s, nil
}

func parseIndexFromPath(path, prefix string) (uint64, error) {
	idxStr := strings.Trim(strings.TrimPrefix(path, prefix), "/")
	if idxStr == "" {
		return 0, fmt.Errorf("missing index")
	}
	idx, err := strconv.ParseUint(idxStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid index")
	}
	return idx, nil
}

func (h *NotaryHandler) readPublishedCheckpoint(ctx context.Context) ([]byte, formatsLog.Checkpoint, error) {
	cpRaw, err := h.reader.ReadCheckpoint(ctx)
	if err != nil {
		return nil, formatsLog.Checkpoint{}, err
	}
	var cp formatsLog.Checkpoint
	if _, err := cp.Unmarshal(cpRaw); err != nil {
		return nil, formatsLog.Checkpoint{}, err
	}
	return cpRaw, cp, nil
}

func hashBytes(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func jsonResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("jsonResponse encode error: %v", err)
	}
}
