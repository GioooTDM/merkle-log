# Server (`/server`)

Notarization backend based on Tessera, an append-only Merkle log, with a supporting SQLite index for application-level queries.

## Quick start
Run from `server/`:

```bash
export LOG_PRIVATE_KEY='PRIVATE+KEY+example.com/log/testdata+33d7b496+AeymY/SZAX0jZcJ8enZ5FY1Dz+wTML2yWSkK+9DSF3eg'
export LOG_PUBLIC_KEY='example.com/log/testdata+33d7b496+AeHTu4Q3hEIMHNqc6fASMsq3rKNx280NI+oO5xCFkkSx'

go run . --storage_dir="./tessera-log" --listen=":2025"
```

## Main parameters
- `--storage_dir` (required): Tessera data directory (checkpoints, tiles, entries)
- `--listen` (default `:2025`): HTTP port
- `--private_key` (optional): private-key file path; if omitted, the server uses `LOG_PRIVATE_KEY`
- `--anchor_file` (optional): enables periodic anchoring to a fake blockchain represented by a `.txt` file
- `--anchor_interval` (default `1h`): checkpoint publication interval
- `--dev-mode` (default `false`): DEV ONLY, uses `issued_at` as `recorded_at` for seed/demo data

## Periodic anchoring (fake blockchain)

```bash
go run . \
  --storage_dir="$LOG_DIR" \
  --listen=":2025" \
  --anchor_file="./anchors/fake-chain.txt" \
  --anchor_interval="1h"
```

Behavior:
- reads the current checkpoint at each interval
- publishes only new checkpoints
- writes one text line per record, with space-separated fields in this order:
  - `published_at_utc domain_separator version tree_size root_hash_hex checkpoint_hash`
- you can force immediate publication with `POST /anchor/force`
- you can read the latest anchored checkpoint from the fake blockchain with `GET /anchor/latest`

## Proof endpoints
- Inclusion proof: `GET /get-inclusion/{log_index}`
- Consistency proof: `GET /get-consistency?from={tree_size_a}&to={tree_size_b}`

## SQLite index notes
- File: `notary_index.db` in the process working directory (`cwd`).
- It is an application index, not the log source of truth.
- At startup, DB/log alignment is checked; a mismatch blocks startup.
- If you point the server to a different log, start with a matching index DB.

## Troubleshooting
- Under load, `database is locked (SQLITE_BUSY)` may appear. Known cause: SQLite write concurrency is not optimized yet (WAL/retry/backoff).
- If you change `cwd`, the path of `notary_index.db` changes too.
- If you point the server to a different Tessera log while keeping an old index, startup fails because of the alignment check.
