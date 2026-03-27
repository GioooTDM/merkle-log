// go run main.go -url http://localhost:2025/add -out ./seed_data
package main

import (
	"bytes"
	"context"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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
	DocID         string      `json:"doc_id"`
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
	DocID         string      `json:"doc_id"`
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
	DocID         string
	Version       int
	PrevEventID   string
	PrevEventLeaf uint64
	OriginalPDF   string
}

type SummaryRow struct {
	DocID         string `json:"doc_id"`
	Version       int    `json:"doc_version"`
	EventID       string `json:"event_id"`
	EventType     string `json:"event_type"`
	LogIndex      uint64 `json:"log_index"`
	PDFPath       string `json:"pdf_path"`
	EventJSONPath string `json:"event_json_path"`
	PDFHashHex    string `json:"payload_hash_hex"`
}

const (
	numCreateDocs = 20 // numero di documenti CREATE generati
	numChosenDocs = 5  // documenti che ricevono UPDATE
	eventSchema   = "pa-notary-event@1"
	docIDBaseNum  = 10000
)

// updatesPerChosenDoc definisce quanti UPDATE riceve ciascuno dei numChosenDocs documenti.
// len(updatesPerChosenDoc) deve essere == numChosenDocs.
var updatesPerChosenDoc = []int{1, 2, 2, 3, 4}

func main() {
	var (
		baseURL   = flag.String("url", "http://localhost:2025/add", "Notary endpoint URL (POST)")
		outDir    = flag.String("out", "notary_seed", "Output directory")
		seed      = flag.Int64("seed", time.Now().UnixNano(), "Random seed")
		days      = flag.Int("days", 0, "Distribuisce gli eventi sugli ultimi N giorni. 0 = data corrente")
		issuerID  = flag.String("issuer-id", "IPA:COMUNE-XYZ", "Issuer entity_id")
		issuerNam = flag.String("issuer-name", "Comune di Esempio — Ufficio Protocollo", "Issuer name")
		docPrefix = flag.String("doc-prefix", "PROT", "Prefisso base doc_id")
	)
	flag.Parse()
	if *days < 0 {
		panic(fmt.Errorf("invalid -days=%d: must be >= 0", *days))
	}

	rng := rand.New(rand.NewSource(*seed))

	absOut, err := filepath.Abs(*outDir)
	must(err)

	must(os.MkdirAll(absOut, 0o755))
	must(os.MkdirAll(filepath.Join(absOut, "pdf"), 0o755))
	must(os.MkdirAll(filepath.Join(absOut, "event"), 0o755))

	client := &http.Client{Timeout: 20 * time.Second}

	ctx := context.Background()

	issuer := Issuer{EntityID: *issuerID, Name: *issuerNam}
	runPrefix := mustMakeRunPrefix(*docPrefix)

	// Pianifica la distribuzione di issued_at:
	// - days=0  -> usa time.Now() per ogni evento
	// - days>0  -> distribuisce in ordine sugli ultimi N giorni
	totalEvents := numCreateDocs
	for _, n := range updatesPerChosenDoc {
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
	fmt.Printf("[seed] doc_id prefix per questa run: %s\n", runPrefix)

	states, summary := runCreates(ctx, client, *baseURL, absOut, runPrefix, issuer, &issuedAtPlan)
	summary = append(summary, runUpdates(ctx, client, *baseURL, absOut, issuer, &issuedAtPlan, states, rng)...)

	sumPath := filepath.Join(absOut, "summary.json")
	must(os.WriteFile(sumPath, mustJSON(summary), 0o644))
	fmt.Printf("\nOK. Output in: %s\n- PDF: %s\n- Event JSON: %s\n- Summary: %s\n",
		absOut, filepath.Join(absOut, "pdf"), filepath.Join(absOut, "event"), sumPath)
}

func runCreates(ctx context.Context, client *http.Client, baseURL, absOut, runPrefix string, issuer Issuer, plan *issuedAtPlanner) ([]DocState, []SummaryRow) {
	states := make([]DocState, 0, numCreateDocs)
	summary := make([]SummaryRow, 0, numCreateDocs)

	for i := 1; i <= numCreateDocs; i++ {
		docID := fmt.Sprintf("%s/%05d", runPrefix, docIDBaseNum+i)
		issuedAt := plan.Next()

		pdfName := fmt.Sprintf("doc_%02d_v1.pdf", i)
		pdfPath := filepath.Join(absOut, "pdf", pdfName)
		pdfBytes := mustMakePDF(buildPALines(docID, 1, i, false, issuedAt))
		must(os.WriteFile(pdfPath, pdfBytes, 0o644))

		pdfHash := sha256.Sum256(pdfBytes)
		pdfHashHex := hex.EncodeToString(pdfHash[:])

		reqEv := AddEventRequest{
			Schema:      eventSchema,
			EventType:   "CREATE",
			DocID:       docID,
			PayloadHash: PayloadHash{Alg: "sha-256", Value: "hex:" + pdfHashHex},
			Issuer:      issuer,
			IssuedAt:    issuedAt.Format(time.RFC3339Nano),
			Title:       "Atto amministrativo — Emissione",
			Description: "Registrazione di un nuovo atto amministrativo in formato digitale (versione iniziale).",
			Notes:       fmt.Sprintf("Documento di prova #%02d generato automaticamente.", i),
		}

		logIndex, storedEv := postEvent(ctx, client, baseURL, reqEv)

		evPath := filepath.Join(absOut, "event", fmt.Sprintf("event_%02d_v1.json", i))
		must(writeJSON(evPath, storedEv))

		states = append(states, DocState{
			DocID:         docID,
			Version:       1,
			PrevEventID:   storedEv.EventID,
			PrevEventLeaf: logIndex,
			OriginalPDF:   pdfPath,
		})

		summary = append(summary, SummaryRow{
			DocID:         docID,
			Version:       1,
			EventID:       storedEv.EventID,
			EventType:     storedEv.EventType,
			LogIndex:      logIndex,
			PDFPath:       pdfPath,
			EventJSONPath: evPath,
			PDFHashHex:    pdfHashHex,
		})

		fmt.Printf("[CREATE] %s -> log_index=%d\n", docID, logIndex)
	}
	return states, summary
}

func runUpdates(ctx context.Context, client *http.Client, baseURL, absOut string, issuer Issuer, plan *issuedAtPlanner, states []DocState, rng *rand.Rand) []SummaryRow {
	chosen := rng.Perm(len(states))[:numChosenDocs]
	summary := make([]SummaryRow, 0, numChosenDocs)
	updateSerial := 0

	for j, idx := range chosen {
		st := states[idx]
		numUpdates := updatesPerChosenDoc[j]

		for u := 1; u <= numUpdates; u++ {
			updateSerial++
			newVersion := st.Version + 1
			issuedAt := plan.Next()

			pdfName := fmt.Sprintf("doc_%02d_v%d_update_%d.pdf", idx+1, newVersion, u)
			pdfPath := filepath.Join(absOut, "pdf", pdfName)

			lines := append(
				buildPALines(st.DocID, newVersion, idx+1, true, issuedAt),
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
				Schema:        eventSchema,
				EventType:     "UPDATE",
				DocID:         st.DocID,
				PrevEventID:   &prevID,
				PrevEventLeaf: &prevLeaf,
				PayloadHash:   PayloadHash{Alg: "sha-256", Value: "hex:" + pdfHashHex},
				Issuer:        issuer,
				IssuedAt:      issuedAt.Format(time.RFC3339Nano),
				Title:         "Atto amministrativo — Aggiornamento",
				Description:   fmt.Sprintf("Aggiornamento del documento con modifiche minori (update %d/%d).", u, numUpdates),
				Notes:         "Generato automaticamente per testare catena UPDATE/prev.",
			}

			logIndex, storedEv := postEvent(ctx, client, baseURL, reqEv)

			evPath := filepath.Join(absOut, "event", fmt.Sprintf("event_%02d_v%d_update_%d.json", idx+1, newVersion, u))
			must(writeJSON(evPath, storedEv))

			st = DocState{
				DocID:         st.DocID,
				Version:       newVersion,
				PrevEventID:   storedEv.EventID,
				PrevEventLeaf: logIndex,
				OriginalPDF:   st.OriginalPDF,
			}
			states[idx] = st

			summary = append(summary, SummaryRow{
				DocID:         st.DocID,
				Version:       newVersion,
				EventID:       storedEv.EventID,
				EventType:     storedEv.EventType,
				LogIndex:      logIndex,
				PDFPath:       pdfPath,
				EventJSONPath: evPath,
				PDFHashHex:    pdfHashHex,
			})
			fmt.Printf("[UPDATE] %s v%d -> log_index=%d (prev=%s) [%d/%d]\n",
				st.DocID, newVersion, logIndex, prevID, u, numUpdates)

			time.Sleep(10 * time.Millisecond)
		}
	}
	return summary
}

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

func mustMakeRunPrefix(base string) string {
	base = strings.TrimSpace(strings.TrimSuffix(base, "/"))
	if base == "" {
		panic("doc-prefix must not be empty")
	}
	return fmt.Sprintf("%s/%s", base, randomAlphaNum(4))
}

func randomAlphaNum(n int) string {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

	if n <= 0 {
		return ""
	}

	raw := make([]byte, n)
	if _, err := cryptorand.Read(raw); err != nil {
		panic(err)
	}

	buf := make([]byte, n)
	for i, b := range raw {
		buf[i] = alphabet[int(b)%len(alphabet)]
	}
	return string(buf)
}

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

func buildPALines(docID string, version int, seq int, isUpdate bool, issuedAt time.Time) []string {
	prot := docID
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
		fmt.Sprintf("Data/ora emissione (UTC): %s", issuedAt.UTC().Format(time.RFC3339)),
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
