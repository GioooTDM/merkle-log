package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/transparency-dev/tessera"
	"github.com/transparency-dev/tessera/storage/posix"
	"golang.org/x/mod/sumdb/note"
)

func main() {
	storageDir := flag.String("storage_dir", "", "Root directory for log files")
	listenAddr := flag.String("listen", ":2025", "Listen address")
	privKeyArg := flag.String("private_key", "", "Private file key path.  Se vuoto usa LOG_PRIVATE_KEY env var.")
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

	indexer, err := NewIndexer("notary_index.db")
	if err != nil {
		log.Fatalf("Failed to init indexer: %v", err)
	}
	defer indexer.Close()

	// 2. Setup Handlers
	h := NewNotaryHandler(appender, indexer, logReader)
	mux := http.NewServeMux()

	// Registro Log API
	mux.HandleFunc("/add", h.AddEvent)
	mux.HandleFunc("/get-by-doc", h.GetByDoc)
	mux.HandleFunc("/get-by-leaf", h.GetByLeaf)
	mux.HandleFunc("/get-entry/", h.GetEntry)
	mux.HandleFunc("/get-proof/", h.GetProof)
	mux.HandleFunc("/get-indexes", h.GetIndexesByDocUID)
	mux.HandleFunc("/get-entries-by-docuid", h.GetEntriesByDocUID)

	// Tessera File Server: espone i file generati in storage_dir
	fs := http.FileServer(http.Dir(*storageDir))
	mux.Handle("/checkpoint", noCache(fs))
	mux.Handle("/tile/", longCache(fs))
	mux.Handle("/entries/", fs)

	// 3. Start Server
	log.Printf("PA Notary Server starting on %s", *listenAddr)
	srv := &http.Server{
		Addr:    *listenAddr,
		Handler: enableCORS(mux),
	}
	log.Fatal(srv.ListenAndServe())
}

func initSigner(path string) note.Signer {
	key := os.Getenv("LOG_PRIVATE_KEY")
	if path != "" {
		b, _ := os.ReadFile(path)
		key = string(b)
	}
	if key == "" {
		log.Fatal("Private key missing: use --private_key=... or LOG_PRIVATE_KEY env var")
	}
	s, err := note.NewSigner(key)
	if err != nil {
		log.Fatalf("Invalid key: %v", err)
	}
	return s
}
