# Server (`/server`)

Backend di notarizzazione basato su Tessera (log append-only Merkle) con indice SQLite di supporto per query applicative.

## Avvio rapido
Esegui da `server/`:

```bash
export LOG_PRIVATE_KEY='PRIVATE+KEY+example.com/log/testdata+33d7b496+AeymY/SZAX0jZcJ8enZ5FY1Dz+wTML2yWSkK+9DSF3eg'
export LOG_PUBLIC_KEY='example.com/log/testdata+33d7b496+AeHTu4Q3hEIMHNqc6fASMsq3rKNx280NI+oO5xCFkkSx'
export LOG_DIR="$HOME/tessera-log"
mkdir -p "$LOG_DIR"

go run . --storage_dir="$LOG_DIR" --listen=":2025"
```

## Parametri principali
- `--storage_dir` (obbligatorio): directory dei dati Tessera (checkpoint, tile, entries)
- `--listen` (default `:2025`): porta HTTP
- `--private_key` (opzionale): path file chiave privata; se assente usa `LOG_PRIVATE_KEY`
- `--anchor_file` (opzionale): abilita anchoring periodico su "blockchain fake" (file di testo JSONL)
- `--anchor_interval` (default `1h`): intervallo di pubblicazione checkpoint

## Anchoring periodico (fake blockchain)

```bash
go run . \
  --storage_dir="$LOG_DIR" \
  --listen=":2025" \
  --anchor_file="./anchors/fake-chain.txt" \
  --anchor_interval="1h"
```

Comportamento:
- ad ogni intervallo legge il checkpoint corrente
- pubblica solo checkpoint nuovi
- scrive un record JSON per riga (JSONL)
- puoi forzare una pubblicazione immediata con `POST /anchor/force`

## Note su indice SQLite
- File: `notary_index.db` nella directory di esecuzione del processo (cwd).
- È un indice applicativo, non la fonte di verità del log.
- In startup viene verificato allineamento DB/log; mismatch blocca l'avvio.
- Se punti a un log diverso, conviene ripartire con un DB indice coerente.

## Troubleshooting
- Sotto carico può comparire `database is locked (SQLITE_BUSY)`.
- Causa nota: concorrenza scritture SQLite non ancora ottimizzata (WAL/retry/backoff).
