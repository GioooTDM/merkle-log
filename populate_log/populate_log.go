// go run populate_log.go -url http://localhost:2025/add -out ./seed_data

// TODO: il JSON inviato al server non è in formato canonico.

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	mathrand "math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
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
		issuerID  = flag.String("issuer-id", "IPA:COMUNE-XYZ", "Issuer entity_id")
		issuerNam = flag.String("issuer-name", "Comune di Esempio — Ufficio Protocollo", "Issuer name")
	)
	flag.Parse()

	mathrand.Seed(*seed)

	absOut, err := filepath.Abs(*outDir)
	must(err)

	must(os.MkdirAll(absOut, 0o755))
	must(os.MkdirAll(filepath.Join(absOut, "pdf"), 0o755))
	must(os.MkdirAll(filepath.Join(absOut, "event"), 0o755))

	client := &http.Client{Timeout: 20 * time.Second}

	ctx := context.Background()

	issuer := Issuer{EntityID: *issuerID, Name: *issuerNam}

	// 1) CREA 20 PDF + CREATE events
	const N = 20
	states := make([]DocState, 0, N)
	summary := make([]SummaryRow, 0, N+5)

	for i := 1; i <= N; i++ {
		docUID := fmt.Sprintf("PROT/2026/%05d", 10000+i) // esempio di doc_uid “PA style”
		version := 1

		pdfName := fmt.Sprintf("doc_%02d_v%d.pdf", i, version)
		pdfPath := filepath.Join(absOut, "pdf", pdfName)

		lines := buildPALines(docUID, version, i, false)
		pdfBytes := mustMakePDF(lines)

		must(os.WriteFile(pdfPath, pdfBytes, 0o644))

		pdfHash := sha256.Sum256(pdfBytes)
		pdfHashHex := hex.EncodeToString(pdfHash[:])

		ev := NotaryEvent{
			Schema:     "pa-notary-event@1",
			EventID:    uuidV4(),
			EventType:  "CREATE",
			DocUID:     docUID,
			DocVersion: 1,
			PayloadHash: PayloadHash{
				Alg:   "sha-256",
				Value: "hex:" + pdfHashHex,
			},
			Issuer:      issuer,
			IssuedAt:    time.Now().UTC().Format(time.RFC3339Nano),
			RecordedAt:  time.Now().UTC().Format(time.RFC3339Nano),
			Title:       "Atto amministrativo — Emissione",
			Description: "Registrazione di un nuovo atto amministrativo in formato digitale (versione iniziale).",
			Notes:       fmt.Sprintf("Documento di prova #%02d generato automaticamente.", i),
		}

		logIndex := postEvent(ctx, client, *baseURL, ev)

		// salva evento JSON raw (lo salviamo noi, non è il raw “wire”, ma va bene per test)
		evPath := filepath.Join(absOut, "event", fmt.Sprintf("event_%02d_v%d.json", i, version))
		must(writeJSON(evPath, ev))

		states = append(states, DocState{
			DocUID:        docUID,
			Version:       1,
			PrevEventID:   ev.EventID,
			PrevEventLeaf: logIndex,
			OriginalPDF:   pdfPath,
		})

		summary = append(summary, SummaryRow{
			DocUID:        docUID,
			Version:       1,
			EventID:       ev.EventID,
			EventType:     ev.EventType,
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
	updatesPerDoc := []int{1, 2, 2, 3, 4}

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

			ev := NotaryEvent{
				Schema:        "pa-notary-event@1",
				EventID:       uuidV4(),
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
				IssuedAt:    time.Now().UTC().Format(time.RFC3339Nano),
				RecordedAt:  time.Now().UTC().Format(time.RFC3339Nano),
				Title:       "Atto amministrativo — Aggiornamento",
				Description: fmt.Sprintf("Aggiornamento del documento con modifiche minori (update %d/%d).", u, numUpdates),
				Notes:       "Generato automaticamente per testare catena UPDATE/prev.",
			}

			logIndex := postEvent(ctx, client, *baseURL, ev)

			evPath := filepath.Join(absOut, "event", fmt.Sprintf("event_%02d_v%d_update_%d.json", idx+1, newVersion, u))
			must(writeJSON(evPath, ev))

			// aggiorna stato per permettere update successivi (catena)
			st = DocState{
				DocUID:        st.DocUID,
				Version:       newVersion,
				PrevEventID:   ev.EventID,
				PrevEventLeaf: logIndex,
				OriginalPDF:   st.OriginalPDF,
			}
			states[idx] = st

			summary = append(summary, SummaryRow{
				DocUID:        st.DocUID,
				Version:       newVersion,
				EventID:       ev.EventID,
				EventType:     ev.EventType,
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

func postEvent(ctx context.Context, client *http.Client, url string, ev NotaryEvent) uint64 {
	body := mustJSON(ev)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	must(err)
	req.Header.Set("Content-Type", "application/json")

	res, err := client.Do(req)
	must(err)
	defer res.Body.Close()

	respBody, _ := io.ReadAll(res.Body)

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		panic(fmt.Errorf("POST %s failed: status=%d body=%s", url, res.StatusCode, string(respBody)))
	}

	// server returns log_index as plain text (e.g. "123\n")
	s := strings.TrimSpace(string(respBody))
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		panic(fmt.Errorf("cannot parse log_index from response %q: %w", s, err))
	}
	return n
}

// ---- PDF generator (minimal, single page, Helvetica, multi-line text) ----
// This generates a simple valid PDF with one page containing your lines.

func mustMakePDF(lines []string) []byte {
	pdf, err := makePDF(lines)
	if err != nil {
		panic(err)
	}
	return pdf
}

func makePDF(lines []string) ([]byte, error) {
	// Content stream: write each line with Td down.
	var content bytes.Buffer
	content.WriteString("BT\n")
	content.WriteString("/F1 12 Tf\n")
	content.WriteString("72 760 Td\n") // start position
	lineStep := 14

	for i, ln := range lines {
		ln = escapePDFText(ln)
		content.WriteString("(")
		content.WriteString(ln)
		content.WriteString(") Tj\n")
		if i != len(lines)-1 {
			content.WriteString(fmt.Sprintf("0 -%d Td\n", lineStep))
		}
	}
	content.WriteString("ET\n")

	contentBytes := content.Bytes()

	// Build objects
	// 1: catalog, 2: pages, 3: page, 4: font, 5: content stream
	var out bytes.Buffer
	write := func(s string) { out.WriteString(s) }

	write("%PDF-1.4\n%\xE2\xE3\xCF\xD3\n")

	offsets := make([]int, 0, 6)
	offsets = append(offsets, 0) // object 0 placeholder

	// obj 1
	offsets = append(offsets, out.Len())
	write("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")

	// obj 2
	offsets = append(offsets, out.Len())
	write("2 0 obj\n<< /Type /Pages /Kids [3 0 R] /Count 1 >>\nendobj\n")

	// obj 3 (page)
	offsets = append(offsets, out.Len())
	write("3 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] ")
	write("/Resources << /Font << /F1 4 0 R >> >> ")
	write("/Contents 5 0 R >>\nendobj\n")

	// obj 4 (font)
	offsets = append(offsets, out.Len())
	write("4 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\nendobj\n")

	// obj 5 (content stream)
	offsets = append(offsets, out.Len())
	write("5 0 obj\n<< /Length ")
	write(strconv.Itoa(len(contentBytes)))
	write(" >>\nstream\n")
	out.Write(contentBytes)
	write("\nendstream\nendobj\n")

	// xref
	xrefPos := out.Len()
	write("xref\n0 6\n")
	write("0000000000 65535 f \n")
	for i := 1; i <= 5; i++ {
		write(fmt.Sprintf("%010d 00000 n \n", offsets[i]))
	}

	// trailer
	write("trailer\n<< /Size 6 /Root 1 0 R >>\n")
	write("startxref\n")
	write(strconv.Itoa(xrefPos))
	write("\n%%EOF\n")

	return out.Bytes(), nil
}

func escapePDFText(s string) string {
	// Escape backslash and parentheses for PDF literal strings.
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "(", "\\(")
	s = strings.ReplaceAll(s, ")", "\\)")
	// Keep it simple: replace CR/LF with space
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
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
		fmt.Sprintf("Premesso che il presente atto digitale n. %02d è stato redatto ai sensi delle disposizioni vigenti,", seq),
		"si dispone la registrazione e conservazione dell'atto nei sistemi informativi dell'Ente.",
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

func uuidV4() string {
	// RFC 4122 UUID v4
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4],
		b[4:6],
		b[6:8],
		b[8:10],
		b[10:16],
	)
}

// fmt with byte slices: helper via Sprintf will print %!x([]uint8=...).
// Convert segments manually:
// func (b []byte) String() string { return hex.EncodeToString(b) }

// Silence unused import if you later remove something:
var _ = errors.New
