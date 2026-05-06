# Seed Log (`/seed_log`)

CLI tool for generating sample datasets (PDFs + JSON events) and sending them to `/add`.

## What it does
- Creates synthetic PDF documents
- Computes the PDF `payload_hash` (`sha-256`)
- Sends `CREATE` and `UPDATE` events to the server
- Leaves `doc_version` assignment to the server
- Saves:
  - notarized events returned by the server (`event/*.json`)
  - generated PDFs (`pdf/*.pdf`)
  - summary (`summary.json`)

## Quick start
From the repository root:

```bash
go run ./seed_log -url http://localhost:2025/add -out ./seed_log/seed_data
```

## Main parameters
- `-url`: endpoint `POST /add`
- `-out`: dataset output directory
- `-seed`: random seed for reproducibility
- `-days`: spreads `issued_at` values across the last N days (`0` = current date)
- `-issuer-id`: `issuer.entity_id` of the generated events
- `-issuer-name`: `issuer.name` of the generated events
- `-doc-prefix`: base prefix for `doc_id`; the tool automatically adds a random alphanumeric segment for each run, for example `PROT/A2C3/10001`

## Expected output
In the `-out` folder:
- `pdf/`: source PDF files
- `event/`: raw notarized JSON files, exactly as appended to the log
- `summary.json`: compact list with `doc_id`, `doc_version`, `event_id`, `log_index`, and file paths
