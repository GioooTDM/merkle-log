# Hammer (`/hammer`)

CLI tool for load-testing the server `POST /add` endpoint.

## Quick start
From the repository root:

```bash
go run ./hammer -url http://localhost:2025/add -requests 1000 -concurrency 40
```

Heavier example:

```bash
go run ./hammer -url http://localhost:2025/add -requests 20000 -concurrency 200 -timeout 15s
```

## Main parameters
- `-url`: endpoint `/add`
- `-requests`: total number of requests
- `-concurrency`: number of concurrent workers
- `-timeout`: timeout for each request
- `-issuer-id`, `-issuer-name`, `-doc-prefix`: metadata used in generated payloads
  - the tool automatically adds a random alphanumeric segment after `doc-prefix`, for example `HAMMER/A2C3/00000001`
- `-error-print-limit`: how many errors to print

## Report output

- total duration (`Elapsed`)
- successful/failed requests
- total RPS and success RPS
- min/p50/p90/p95/p99/max latencies

## Important note (current state)
With medium-high concurrency, server-side errors such as `database is locked (SQLITE_BUSY)` are easy to observe.

The SQLite index is not optimized for concurrent writes yet (WAL/busy timeout/retry/backoff still need improvement).

The `hammer` tool is not the problem. The current bottleneck is DB index write handling.
