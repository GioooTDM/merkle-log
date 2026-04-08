# Server (`/server`)

Backend di notarizzazione basato su Tessera (log append-only Merkle) con indice SQLite di supporto per query applicative.

## Avvio rapido
Esegui da `server/`:

```bash
export LOG_PRIVATE_KEY='PRIVATE+KEY+example.com/log/testdata+33d7b496+AeymY/SZAX0jZcJ8enZ5FY1Dz+wTML2yWSkK+9DSF3eg'
export LOG_PUBLIC_KEY='example.com/log/testdata+33d7b496+AeHTu4Q3hEIMHNqc6fASMsq3rKNx280NI+oO5xCFkkSx'

go run . --storage_dir="./tessera-log" --listen=":2025"
```

## Parametri principali
- `--storage_dir` (obbligatorio): directory dei dati Tessera (checkpoint, tile, entries)
- `--listen` (default `:2025`): porta HTTP
- `--private_key` (opzionale): path file chiave privata; se assente usa `LOG_PRIVATE_KEY`
- `--anchor_file` (opzionale): abilita anchoring periodico su "blockchain fake" (file `.txt`)
- `--anchor_interval` (default `1h`): intervallo di pubblicazione checkpoint
- `--dev-mode` (default `false`): DEV ONLY, usa `issued_at` come `recorded_at` per seed/demo

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
- scrive una riga testuale per record, con campi separati da spazio in questo ordine:
  - `published_at_utc domain_separator version tree_size root_hash_hex checkpoint_hash`
- puoi forzare una pubblicazione immediata con `POST /anchor/force`
- puoi leggere l'ultimo checkpoint notarizzato (fake blockchain) con `GET /anchor/latest`

## Endpoint proof
- Inclusion proof: `GET /get-inclusion/{log_index}`
- Consistency proof: `GET /get-consistency?from={tree_size_a}&to={tree_size_b}`

## Note su indice SQLite
- File: `notary_index.db` nella directory di esecuzione del processo (cwd).
- È un indice applicativo, non la fonte di verità del log.
- In startup viene verificato allineamento DB/log; mismatch blocca l'avvio.
- Se punti a un log diverso, conviene ripartire con un DB indice coerente.

## Troubleshooting
- Sotto carico può comparire `database is locked (SQLITE_BUSY)`. Causa nota: concorrenza scritture SQLite non ancora ottimizzata (WAL/retry/backoff).
- Se cambi `cwd`, cambia anche il path del file `notary_index.db`.
- Se punti il server a un log Tessera diverso mantenendo un indice vecchio, l'avvio fallisce per disallineamento.

