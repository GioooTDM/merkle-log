# Server (`/server`)

Backend di notarizzazione basato su Tessera (transparency log append-only) con indice SQLite di supporto per query applicative.

## Avvio rapido
Da dentro `server/`:

```bash
export LOG_PRIVATE_KEY='PRIVATE+KEY+example.com/log/testdata+33d7b496+AeymY/SZAX0jZcJ8enZ5FY1Dz+wTML2yWSkK+9DSF3eg'
export LOG_PUBLIC_KEY='example.com/log/testdata+33d7b496+AeHTu4Q3hEIMHNqc6fASMsq3rKNx280NI+oO5xCFkkSx'
export LOG_DIR="$HOME/tessera-log"
mkdir -p "$LOG_DIR"

go run . --storage_dir="$LOG_DIR" --listen=":2025"
```

## Parametri principali
- `--storage_dir` (obbligatorio): directory dei dati Tessera (checkpoint, tile, entries)
- `--listen` (default `:2025`): bind HTTP
- `--private_key` (opzionale): path file chiave privata; se assente usa `LOG_PRIVATE_KEY`
- `--anchor_file` (opzionale): abilita anchoring periodico su "blockchain fake" (file di testo JSONL)
- `--anchor_interval` (default `1h`): intervallo di pubblicazione checkpoint

## Anchoring periodico (fake blockchain)
Esempio:

```bash
go run . \
  --storage_dir="$LOG_DIR" \
  --listen=":2025" \
  --anchor_file="./anchors/fake-chain.txt" \
  --anchor_interval="1h"
```

Comportamento:
- ogni intervallo legge il checkpoint corrente
- se il checkpoint e nuovo lo pubblica nel file fake chain
- formato file: una riga JSON per record (JSONL)
- puoi forzare una pubblicazione immediata con `POST /anchor/force`

Campi salvati:
- `published_at_utc`
- `tree_size`
- `root_hash_hex`
- `checkpoint_hash`
- `checkpoint_raw`

## Endpoint principali
- `POST /add`
  - body JSON evento (senza `event_id` e `recorded_at`)
  - response:
    - `log_index`
    - `notarized_json` (raw canonical JSON effettivamente notarizzato)

- `GET /get-entry/{index}`
  - restituisce l'entry raw del log a quell'indice

- `GET /get-proof/{index}`
  - restituisce inclusion proof + root hash + tree size + checkpoint

- `GET /get-indexes?doc_uid=...`
  - restituisce gli indici log associati al `doc_uid`

- `GET /get-entries-by-docuid?doc_uid=...`
  - restituisce entry e indici associati al `doc_uid`

- `GET /checkpoint`

- `POST /anchor/force`
  - forza pubblicazione immediata del checkpoint corrente (se anchoring abilitato)

## Note su DB indice
- File: `server/notary_index.db`
- Serve solo come indice applicativo, non è la fonte di verità del log.
- Se cambi log storage o avvii su un log diverso, conviene rigenerare/resettare il DB indice per evitare mismatch.

## Troubleshooting
- `SQLITE_BUSY` sotto carico:
  - riduci concorrenza o abilita tuning SQLite (WAL/busy timeout/retry) come da TODO.
