# merkle-log

Progetto di notarizzazione eventi/documenti basato su transparency log Merkle.

## Struttura repository
- `server/`: API HTTP + integrazione Tessera + indice SQLite + anchoring su file JSONL
- `client/`: pagine HTML/JS per usare il sistema da browser
- `seed_log/`: CLI che genera PDF/eventi di esempio e li invia a `/add`
- `hammer/`: CLI per stress test di `POST /add`

## Quick start locale
1. Avvia il server

```bash
cd server
export LOG_PRIVATE_KEY='PRIVATE+KEY+example.com/log/testdata+33d7b496+AeymY/SZAX0jZcJ8enZ5FY1Dz+wTML2yWSkK+9DSF3eg'
go run . --storage_dir="./tessera-log" --listen=":2025"
```

2. Avvia il client

```bash
cd client
python3 -m http.server 8080
```

Apri `http://localhost:8080/index.html`.

## Note
- Il file indice SQLite (`notary_index.db`) dipende dalla directory di esecuzione del server.
- In condizioni di alto parallelismo possono emergere `SQLITE_BUSY` durante la scrittura dell'indice.
