# merkle-log

Event/document notarization project based on a Merkle transparency log.

## Repository structure
- `server/`: HTTP API + Tessera integration + SQLite index + append-only text-file anchoring
- `client/`: HTML/JS pages for using the system from a browser
- `seed_log/`: CLI that generates sample PDFs/events and sends them to `/add`
- `hammer/`: CLI for stress-testing `POST /add`

## Local quick start
1. Start the server

```bash
cd server
export LOG_PRIVATE_KEY='PRIVATE+KEY+example.com/log/testdata+33d7b496+AeymY/SZAX0jZcJ8enZ5FY1Dz+wTML2yWSkK+9DSF3eg'
go run . --storage_dir="./tessera-log" --listen=":2025"
```

2. Start the client

```bash
cd client
python3 -m http.server 8080
```

Open `http://localhost:8080/index.html`.

## Notes
- The SQLite index file (`notary_index.db`) depends on the server process working directory.
- Under high parallelism, `SQLITE_BUSY` errors may appear while writing the index.
