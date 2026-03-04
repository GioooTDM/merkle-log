// go run populate_log.go -url http://localhost:2025/add -out ./seed_data

// TODO: estrarre magic strings

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	mathrand "math/rand"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/go-pdf/fpdf"
)

type PayloadHash struct {
	Alg   string `json:"alg"`
	Value string `json:"value"` // "hex:<...>"
}

type Issuer struct {
	EntityID string `json:"entity_id"`
	Name     string `json:"name,omitempty"`
}

type NotaryEvent struct {
	Schema        string      `json:"schema"`
	EventID       string      `json:"event_id"`
	EventType     string      `json:"event_type"` // CREATE, UPDATE
	DocUID        string      `json:"doc_uid"`
	DocVersion    int         `json:"doc_version"`
	PrevEventID   *string     `json:"prev_event_id,omitempty"`
	PrevEventLeaf *uint64     `json:"prev_event_leaf,omitempty"`
	PayloadHash   PayloadHash `json:"payload_hash"`
	Issuer        Issuer      `json:"issuer"`
	IssuedAt      string      `json:"issued_at"`
	RecordedAt    string      `json:"recorded_at"`
	Title         string      `json:"title,omitempty"`
	Description   string      `json:"description,omitempty"`
	Notes         string      `json:"notes,omitempty"`
}

type AddEventRequest struct {
	Schema        string      `json:"schema"`
	EventType     string      `json:"event_type"` // CREATE, UPDATE
	DocUID        string      `json:"doc_uid"`
	DocVersion    int         `json:"doc_version"`
	PrevEventID   *string     `json:"prev_event_id,omitempty"`
	PrevEventLeaf *uint64     `json:"prev_event_leaf,omitempty"`
	PayloadHash   PayloadHash `json:"payload_hash"`
	Issuer        Issuer      `json:"issuer"`
	IssuedAt      string      `json:"issued_at"`
	Title         string      `json:"title,omitempty"`
	Description   string      `json:"description,omitempty"`
	Notes         string      `json:"notes,omitempty"`
}

type DocState struct {
	DocUID        string
	Version       int
	PrevEventID   string
	PrevEventLeaf uint64
	OriginalPDF   string
}

type SummaryRow struct {
	DocUID        string `json:"doc_uid"`
	Version       int    `json:"doc_version"`
	EventID       string `json:"event_id"`
	EventType     string `json:"event_type"`
	LogIndex      uint64 `json:"log_index"`
	PDFPath       string `json:"pdf_path"`
	EventJSONPath string `json:"event_json_path"`
	PDFHashHex    string `json:"payload_hash_hex"`
}

func main() {
	var (
		baseURL   = flag.String("url", "http://localhost:2025/add", "Notary endpoint URL (POST)")
		outDir    = flag.String("out", "notary_seed", "Output directory")
		seed      = flag.Int64("seed", time.Now().UnixNano(), "Random seed")
		days      = flag.Int("days", 0, "Distribuisce gli eventi sugli ultimi N giorni. 0 = data corrente")
		issuerID  = flag.String("issuer-id", "IPA:COMUNE-XYZ", "Issuer entity_id")
		issuerNam = flag.String("issuer-name", "Comune di Esempio — Ufficio Protocollo", "Issuer name")
	)
	flag.Parse()
	if *days < 0 {
		panic(fmt.Errorf("invalid -days=%d: must be >= 0", *days))
	}

	mathrand.Seed(*seed)

	absOut, err := filepath.Abs(*outDir)
	must(err)

	must(os.MkdirAll(absOut, 0o755))
	must(os.MkdirAll(filepath.Join(absOut, "pdf"), 0o755))
	must(os.MkdirAll(filepath.Join(absOut, "event"), 0o755))

	client := &http.Client{Timeout: 20 * time.Second}

	ctx := context.Background()

	issuer := Issuer{EntityID: *issuerID, Name: *issuerNam}

	// Pianifica la distribuzione di issued_at:
	// - days=0  -> usa time.Now() per ogni evento
	// - days>0  -> distribuisce in ordine sugli ultimi N giorni
	const N = 20
	updatesPerDoc := []int{1, 2, 2, 3, 4}
	totalEvents := N
	for _, n := range updatesPerDoc {
		totalEvents += n
	}
	issuedAtPlan := newIssuedAtPlanner(*days, totalEvents)
	if issuedAtPlan.distributed {
		fmt.Printf("[seed] issued_at distribuiti negli ultimi %d giorni (%s -> %s)\n",
			*days,
			issuedAtPlan.start.Format(time.RFC3339),
			issuedAtPlan.end.Format(time.RFC3339),
		)
	} else {
		fmt.Printf("[seed] issued_at in data corrente (days=0)\n")
	}

	// 1) CREA 20 PDF + CREATE events
	states := make([]DocState, 0, N)
	summary := make([]SummaryRow, 0, N+5)

	for i := 1; i <= N; i++ {
		docUID := fmt.Sprintf("PROT/2026/%05d", 10000+i)
		version := 1

		pdfName := fmt.Sprintf("doc_%02d_v%d.pdf", i, version)
		pdfPath := filepath.Join(absOut, "pdf", pdfName)

		lines := buildPALines(docUID, version, i, false)
		pdfBytes := mustMakePDF(lines)

		must(os.WriteFile(pdfPath, pdfBytes, 0o644))

		pdfHash := sha256.Sum256(pdfBytes)
		pdfHashHex := hex.EncodeToString(pdfHash[:])

		reqEv := AddEventRequest{
			Schema:     "pa-notary-event@1",
			EventType:  "CREATE",
			DocUID:     docUID,
			DocVersion: 1,
			PayloadHash: PayloadHash{
				Alg:   "sha-256",
				Value: "hex:" + pdfHashHex,
			},
			Issuer:      issuer,
			IssuedAt:    issuedAtPlan.Next().Format(time.RFC3339Nano),
			Title:       "Atto amministrativo — Emissione",
			Description: "Registrazione di un nuovo atto amministrativo in formato digitale (versione iniziale).",
			Notes:       fmt.Sprintf("Documento di prova #%02d generato automaticamente.", i),
		}

		logIndex, storedEv := postEvent(ctx, client, *baseURL, reqEv)

		// Salva l'evento effettivamente notarizzato dal server.
		evPath := filepath.Join(absOut, "event", fmt.Sprintf("event_%02d_v%d.json", i, version))
		must(writeJSON(evPath, storedEv))

		states = append(states, DocState{
			DocUID:        docUID,
			Version:       1,
			PrevEventID:   storedEv.EventID,
			PrevEventLeaf: logIndex,
			OriginalPDF:   pdfPath,
		})

		summary = append(summary, SummaryRow{
			DocUID:        docUID,
			Version:       1,
			EventID:       storedEv.EventID,
			EventType:     storedEv.EventType,
			LogIndex:      logIndex,
			PDFPath:       pdfPath,
			EventJSONPath: evPath,
			PDFHashHex:    pdfHashHex,
		})

		fmt.Printf("[CREATE] %s -> log_index=%d\n", docUID, logIndex)
	}

	// 2) SCEGLI 5 DOC e crea UPDATE (catene da 1,2,3 update)
	perm := mathrand.Perm(len(states))
	chosen := perm[:5]

	// Distribuzione catena update sui 5 documenti scelti.
	// Esempi possibili:
	// []int{1,2,2,3,4}  -> più update totali
	// []int{1,1,2,2,3}  -> bilanciato (tot 9 update)

	updateSerial := 0

	for j, idx := range chosen {
		st := states[idx]
		numUpdates := updatesPerDoc[j]

		for u := 1; u <= numUpdates; u++ {
			updateSerial++
			newVersion := st.Version + 1

			pdfName := fmt.Sprintf("doc_%02d_v%d_update_%d.pdf", idx+1, newVersion, u)
			pdfPath := filepath.Join(absOut, "pdf", pdfName)

			lines := buildPALines(st.DocUID, newVersion, idx+1, true)
			lines = append(lines,
				"",
				"— AGGIORNAMENTO —",
				fmt.Sprintf("Annotazione: rettifica ref. interno n. %d/%d.", 200+updateSerial, 2026),
				fmt.Sprintf("Motivazione: correzione refuso e integrazione metadati (update %d/%d).", u, numUpdates),
			)

			pdfBytes := mustMakePDF(lines)
			must(os.WriteFile(pdfPath, pdfBytes, 0o644))

			pdfHash := sha256.Sum256(pdfBytes)
			pdfHashHex := hex.EncodeToString(pdfHash[:])

			prevID := st.PrevEventID
			prevLeaf := st.PrevEventLeaf

			reqEv := AddEventRequest{
				Schema:        "pa-notary-event@1",
				EventType:     "UPDATE",
				DocUID:        st.DocUID,
				DocVersion:    newVersion,
				PrevEventID:   &prevID,
				PrevEventLeaf: &prevLeaf,
				PayloadHash: PayloadHash{
					Alg:   "sha-256",
					Value: "hex:" + pdfHashHex,
				},
				Issuer:      issuer,
				IssuedAt:    issuedAtPlan.Next().Format(time.RFC3339Nano),
				Title:       "Atto amministrativo — Aggiornamento",
				Description: fmt.Sprintf("Aggiornamento del documento con modifiche minori (update %d/%d).", u, numUpdates),
				Notes:       "Generato automaticamente per testare catena UPDATE/prev.",
			}

			logIndex, storedEv := postEvent(ctx, client, *baseURL, reqEv)

			evPath := filepath.Join(absOut, "event", fmt.Sprintf("event_%02d_v%d_update_%d.json", idx+1, newVersion, u))
			must(writeJSON(evPath, storedEv))

			// aggiorna stato per permettere update successivi (catena)
			st = DocState{
				DocUID:        st.DocUID,
				Version:       newVersion,
				PrevEventID:   storedEv.EventID,
				PrevEventLeaf: logIndex,
				OriginalPDF:   st.OriginalPDF,
			}
			states[idx] = st

			summary = append(summary, SummaryRow{
				DocUID:        st.DocUID,
				Version:       newVersion,
				EventID:       storedEv.EventID,
				EventType:     storedEv.EventType,
				LogIndex:      logIndex,
				PDFPath:       pdfPath,
				EventJSONPath: evPath,
				PDFHashHex:    pdfHashHex,
			})

			fmt.Printf("[UPDATE] %s v%d -> log_index=%d (prev=%s) [%d/%d]\n",
				st.DocUID, newVersion, logIndex, prevID, u, numUpdates)

			// piccola pausa: differenzia timestamp nei file
			time.Sleep(10 * time.Millisecond)
		}
	}

	// 3) salva summary
	sumPath := filepath.Join(absOut, "summary.json")
	must(os.WriteFile(sumPath, mustJSON(summary), 0o644))
	fmt.Printf("\nOK. Output in: %s\n- PDF: %s\n- Event JSON: %s\n- Summary: %s\n",
		absOut, filepath.Join(absOut, "pdf"), filepath.Join(absOut, "event"), sumPath)
}

// ---- HTTP /add ----

func postEvent(ctx context.Context, client *http.Client, url string, ev AddEventRequest) (uint64, NotaryEvent) {
	body, err := json.Marshal(ev)
	must(err)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	must(err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	res, err := client.Do(req)
	must(err)
	defer res.Body.Close()

	respBody, _ := io.ReadAll(res.Body)

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		panic(fmt.Errorf("POST %s failed: status=%d body=%s", url, res.StatusCode, string(respBody)))
	}

	var addResp struct {
		LogIndex     uint64          `json:"log_index"`
		NotarizedRaw json.RawMessage `json:"notarized_json"`
	}
	if err := json.Unmarshal(respBody, &addResp); err != nil {
		panic(fmt.Errorf("cannot parse /add JSON response %q: %w", string(respBody), err))
	}
	if len(addResp.NotarizedRaw) == 0 {
		panic(fmt.Errorf("invalid /add response: empty notarized_json"))
	}

	var storedEv NotaryEvent
	if err := json.Unmarshal(addResp.NotarizedRaw, &storedEv); err != nil {
		panic(fmt.Errorf("cannot decode notarized_json %q: %w", string(addResp.NotarizedRaw), err))
	}

	return addResp.LogIndex, storedEv
}

// ---- PDF generator ----

func mustMakePDF(lines []string) []byte {
	pdf, err := makePDF(lines)
	if err != nil {
		panic(err)
	}
	return pdf
}

func makePDF(lines []string) ([]byte, error) {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(20, 20, 20)
	pdf.SetAutoPageBreak(true, 20)
	pdf.AddPage()
	pdf.SetFont("Helvetica", "", 12)

	translate := pdf.UnicodeTranslatorFromDescriptor("")
	for _, line := range lines {
		pdf.MultiCell(0, 6, translate(line), "", "L", false)
	}

	if pdf.Err() {
		return nil, fmt.Errorf("pdf build error: %v", pdf.Error())
	}

	var out bytes.Buffer
	if err := pdf.Output(&out); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// ---- content builders ----

func buildPALines(docUID string, version int, seq int, isUpdate bool) []string {
	now := time.Now().UTC()
	prot := docUID
	tipo := "DETERMINA DIRIGENZIALE"
	if isUpdate {
		tipo = "DETERMINA DIRIGENZIALE (AGGIORNAMENTO)"
	}

	return []string{
		"REPUBBLICA ITALIANA",
		"ENTE PUBBLICO TERRITORIALE — UFFICIO PROTOCOLLO",
		"",
		fmt.Sprintf("OGGETTO: %s", tipo),
		fmt.Sprintf("Protocollo: %s", prot),
		fmt.Sprintf("Versione documento: %d", version),
		fmt.Sprintf("Data/ora emissione (UTC): %s", now.Format(time.RFC3339)),
		"",
		fmt.Sprintf("Premesso che il presente atto digitale n. %02d è stato redatto ai sensi delle disposizioni vigenti, si dispone la registrazione e conservazione dell'atto nei sistemi informativi dell'Ente.", seq),
		"",
		"Il documento è prodotto in formato elettronico e sottoposto a procedura di notarizzazione su registro di trasparenza.",
		"",
		"Firmato digitalmente (simulazione):",
		"Il Responsabile del Procedimento",
		"Dott. Mario Rossi",
	}
}

// ---- utils ----

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func mustJSON(v any) []byte {
	b, err := json.MarshalIndent(v, "", "  ")
	must(err)
	return b
}

func writeJSON(path string, v any) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, mustJSON(v), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

type issuedAtPlanner struct {
	distributed bool
	start       time.Time
	end         time.Time
	step        time.Duration
	seq         int
}

func newIssuedAtPlanner(days, totalEvents int) issuedAtPlanner {
	if days <= 0 || totalEvents <= 0 {
		return issuedAtPlanner{distributed: false}
	}

	now := time.Now().UTC()
	span := time.Duration(days) * 24 * time.Hour
	start := now.Add(-span)

	step := time.Duration(0)
	if totalEvents > 1 {
		step = span / time.Duration(totalEvents-1)
	}

	return issuedAtPlanner{
		distributed: true,
		start:       start,
		end:         now,
		step:        step,
	}
}

func (p *issuedAtPlanner) Next() time.Time {
	if !p.distributed {
		return time.Now().UTC()
	}
	t := p.start.Add(time.Duration(p.seq) * p.step).UTC()
	p.seq++
	return t
}
