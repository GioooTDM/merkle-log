// go run ./hammer -url http://localhost:2025/add -requests 1000 -concurrency 20
// go run ./hammer -url http://localhost:2025/add -requests 10000 -concurrency 200 -timeout 15s

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"merkle-log/internal/notaryapi"
)

var (
	url         = flag.String("url", "http://localhost:2025/add", "Endpoint /add")
	requests    = flag.Int("requests", 1000, "Numero totale richieste")
	concurrency = flag.Int("concurrency", 20, "Numero worker concorrenti")
	timeout     = flag.Duration("timeout", 10*time.Second, "Timeout per singola richiesta")
	issuerID    = flag.String("issuer-id", "IPA:HAMMER", "issuer.entity_id")
	issuerName  = flag.String("issuer-name", "Load Test", "issuer.name")
	docPrefix   = flag.String("doc-prefix", "HAMMER", "Prefisso base doc_id")
	errLimit    = flag.Int("error-print-limit", 10, "Max errori stampati")
)

type result struct {
	Latency time.Duration
	Err     error
}

func main() {

	flag.Parse()

	if *requests <= 0 {
		panic("requests must be > 0")
	}
	if *concurrency <= 0 {
		panic("concurrency must be > 0")
	}

	runPrefix := mustMakeRunPrefix(*docPrefix)

	client := &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:        *concurrency * 2,
			MaxIdleConnsPerHost: *concurrency * 2,
			MaxConnsPerHost:     *concurrency * 2,
			IdleConnTimeout:     30 * time.Second,
		},
	}

	jobs := make(chan int)
	results := make(chan result, *requests)

	var wg sync.WaitGroup

	start := time.Now()

	for w := 0; w < *concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				payload := makePayload(i, runPrefix, *issuerID, *issuerName)
				results <- doOne(client, *url, *timeout, payload)
			}
		}()
	}

	go func() {
		for i := 0; i < *requests; i++ {
			jobs <- i
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()

	latencies := make([]time.Duration, 0, *requests)
	errs, success, printedErr := 0, 0, 0

	for r := range results {
		if r.Err != nil {
			errs++
			if printedErr < *errLimit {
				fmt.Printf("ERR: %v\n", r.Err)
				printedErr++
			}
			continue
		}
		success++
		latencies = append(latencies, r.Latency)
	}

	elapsed := time.Since(start)

	// Stampa Report
	printReport(runPrefix, elapsed, success, errs, latencies)
}

func doOne(client *http.Client, url string, timeout time.Duration, payload notaryapi.AddEventRequest) result {
	body, err := json.Marshal(payload)
	if err != nil {
		return result{Err: fmt.Errorf("marshal payload: %w", err)}
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return result{Err: fmt.Errorf("build request: %w", err)}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	start := time.Now()
	resp, err := client.Do(req)
	lat := time.Since(start)
	if err != nil {
		return result{Latency: lat, Err: fmt.Errorf("http post: %w", err)}
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		msg := strings.TrimSpace(string(raw))
		if len(msg) > 240 {
			msg = msg[:240] + "..."
		}
		return result{Latency: lat, Err: fmt.Errorf("status=%d body=%q", resp.StatusCode, msg)}
	}

	var out notaryapi.AddEventResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return result{Latency: lat, Err: fmt.Errorf("decode response: %w", err)}
	}
	if len(out.NotarizedRaw) == 0 {
		return result{Latency: lat, Err: fmt.Errorf("empty notarized_json in response")}
	}
	return result{Latency: lat}
}

func makePayload(i int, runPrefix, issuerID, issuerName string) notaryapi.AddEventRequest {
	return notaryapi.AddEventRequest{
		Schema:    "pa-notary-event@1",
		EventType: "CREATE",
		DocID:     fmt.Sprintf("%s/%08d", runPrefix, i+1),
		PayloadHash: &notaryapi.PayloadHash{
			Alg:   "sha-256",
			Value: "hex:" + randomSHA256Hex(),
		},
		Issuer: notaryapi.Issuer{
			EntityID: issuerID,
			Name:     issuerName,
		},
		IssuedAt:    time.Now().UTC().Format(time.RFC3339Nano),
		Title:       fmt.Sprintf("Load test document %d", i+1),
		Description: "Synthetic load-test entry",
	}
}

func randomSHA256Hex() string {
	b := make([]byte, 64)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
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

	buf := make([]byte, n)
	raw := make([]byte, n)
	if _, err := rand.Read(raw); err != nil {
		panic(err)
	}
	for i, b := range raw {
		buf[i] = alphabet[int(b)%len(alphabet)]
	}
	return string(buf)
}

func printReport(runPrefix string, elapsed time.Duration, success, errs int, latencies []time.Duration) {
	fmt.Println("\n=== HAMMER REPORT ===")
	fmt.Printf("URL: %s\n", *url)
	fmt.Printf("Run prefix: %s\n", runPrefix)
	fmt.Printf("Requests: %d\n", *requests)
	fmt.Printf("Concurrency: %d\n", *concurrency)
	fmt.Printf("Elapsed: %s\n", elapsed)
	fmt.Printf("Success: %d\n", success)
	fmt.Printf("Errors: %d\n", errs)
	fmt.Printf("RPS (total): %.2f\n", float64(*requests)/elapsed.Seconds())
	fmt.Printf("RPS (success): %.2f\n", float64(success)/elapsed.Seconds())

	if len(latencies) == 0 {
		fmt.Println("No successful requests; no latency percentiles available")
		return
	}

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })

	fmt.Printf("Latency min: %s\n", latencies[0])
	fmt.Printf("Latency p50: %s\n", percentile(latencies, 50))
	fmt.Printf("Latency p90: %s\n", percentile(latencies, 90))
	fmt.Printf("Latency p95: %s\n", percentile(latencies, 95))
	fmt.Printf("Latency p99: %s\n", percentile(latencies, 99))
	fmt.Printf("Latency max: %s\n", latencies[len(latencies)-1])
}

func percentile(sorted []time.Duration, p int) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 100 {
		return sorted[len(sorted)-1]
	}
	idx := (p * (len(sorted) - 1)) / 100
	return sorted[idx]
}
