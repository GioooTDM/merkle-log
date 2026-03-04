package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"merkle-log/server/internal/anchor"
	"merkle-log/server/internal/api"
	"merkle-log/server/internal/index"

	"github.com/transparency-dev/tessera"
	"github.com/transparency-dev/tessera/storage/posix"
	"golang.org/x/mod/sumdb/note"
)

func main() {
	storageDir := flag.String("storage_dir", "", "Root directory for log files")
	listenAddr := flag.String("listen", ":2025", "Listen address")
	privKeyArg := flag.String("private_key", "", "Private file key path. Se vuoto usa LOG_PRIVATE_KEY env var.")
	anchorFile := flag.String("anchor_file", "", "Fake blockchain anchor output file (JSONL). Empty disables anchoring.")
	anchorInterval := flag.Duration("anchor_interval", time.Hour, "Checkpoint anchor interval (e.g. 1h, 10m)")
	devMode := flag.Bool("dev-mode", false, "DEV ONLY: usa issued_at come recorded_at. Non usare in produzione.")
	flag.Parse()

	if *storageDir == "" {
		log.Fatal("--storage_dir is required")
	}

	// 1. Inizializza componenti Core
	signer := initSigner(*privKeyArg)
	ctx := context.Background()

	driver, err := posix.New(ctx, posix.Config{Path: *storageDir})
	if err != nil {
		log.Fatalf("Failed to init storage: %v", err)
	}

	appender, shutdown, logReader, err := tessera.NewAppender(ctx, driver,
		tessera.NewAppendOptions().
			WithCheckpointSigner(signer).
			WithCheckpointInterval(1*time.Second).
			WithCheckpointRepublishInterval(30*time.Second).
			WithBatching(256, 1*time.Second),
	)
	if err != nil {
		log.Fatalf("Failed to init appender: %v", err)
	}
	defer shutdown(ctx)

	indexer, err := index.New("notary_index.db")
	if err != nil {
		log.Fatalf("Failed to init indexer: %v", err)
	}
	defer indexer.Close()

	if err := indexer.ValidateAlignedWithLog(ctx, logReader); err != nil {
		log.Fatalf("DB/log alignment check failed: %v", err)
	}
	log.Printf("DB/log alignment check passed")

	if *devMode {
		log.Printf("!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!")
		log.Printf("WARNING: --dev-mode ENABLED. recorded_at will be copied from issued_at.")
		log.Printf("WARNING: DEV MODE IS FOR DEMO/TEST ONLY. DO NOT USE IN PRODUCTION.")
		log.Printf("!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!")
	}

	anchorWorker := initAnchorWorker(ctx, logReader, *anchorFile, *anchorInterval)

	// 2. Setup Handlers
	h := api.NewNotaryHandler(appender, indexer, logReader)
	h.SetDevMode(*devMode)
	mux := http.NewServeMux()

	// Registro Log API
	api.RegisterRoutes(mux, h, anchorWorker)

	// Tessera File Server: espone i file generati in storage_dir
	fs := http.FileServer(http.Dir(*storageDir))
	mux.Handle("/checkpoint", noCache(fs))
	mux.Handle("/tile/", longCache(fs))
	mux.Handle("/entries/", fs) // TODO: endpoint sbagliato, non vengono rispettate le convenzioni di tessera

	// 3. Start Server
	log.Printf("PA Notary Server starting on %s", *listenAddr)
	srv := &http.Server{
		Addr:    *listenAddr,
		Handler: enableCORS(mux),
	}
	log.Fatal(srv.ListenAndServe())
}

// initAnchorWorker creates and starts the anchor worker if anchorFile is set.
// Returns nil if anchoring is disabled.
func initAnchorWorker(ctx context.Context, logReader tessera.LogReader, anchorFile string, interval time.Duration) *anchor.Worker {
	if anchorFile == "" {
		return nil
	}

	publisher, err := anchor.NewFilePublisher(anchorFile)
	if err != nil {
		log.Fatalf("Failed to init anchor publisher: %v", err)
	}

	worker, err := anchor.NewWorker(logReader, publisher, interval)
	if err != nil {
		log.Fatalf("Failed to init anchor worker: %v", err)
	}

	go worker.Run(ctx)
	log.Printf("Checkpoint anchoring enabled: every %s -> %s", interval, anchorFile)

	return worker
}

func initSigner(path string) note.Signer {
	key := os.Getenv("LOG_PRIVATE_KEY")
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			log.Fatalf("Failed to read private key file %q: %v", path, err)
		}
		key = string(b)
	}
	key = strings.TrimSpace(key)
	if key == "" {
		log.Fatal("Private key missing: use --private_key=... or LOG_PRIVATE_KEY env var")
	}
	s, err := note.NewSigner(key)
	if err != nil {
		log.Fatalf("Invalid key: %v", err)
	}
	return s
}
