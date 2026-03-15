# Hammer (`/hammer`)

Tool CLI per fare load test sull'endpoint `POST /add` del server.

## Avvio rapido
Da root repository:

```bash
go run ./hammer -url http://localhost:2025/add -requests 1000 -concurrency 40
```

Esempio più pesante:

```bash
go run ./hammer -url http://localhost:2025/add -requests 20000 -concurrency 200 -timeout 15s
```

## Parametri principali
- `-url`: endpoint `/add`
- `-requests`: numero totale richieste
- `-concurrency`: numero di worker concorrenti
- `-timeout`: timeout per singola richiesta
- `-issuer-id`, `-issuer-name`, `-doc-prefix`: metadati usati nei payload generati
  - il tool aggiunge automaticamente un segmento alfanumerico casuale dopo `doc-prefix`, per esempio `HAMMER/A2C3/00000001`
- `-error-print-limit`: quanti errori stampare a video

## Output report

- durata totale (`Elapsed`)
- richieste riuscite/fallite
- RPS totale e RPS success
- latenze min/p50/p90/p95/p99/max

## Nota importante (stato attuale)
Con concorrenza medio-alta è facile vedere errori lato server tipo `database is locked (SQLITE_BUSY)`.

L'indice SQLite non è ancora ottimizzato per scritture concorrenti (WAL/busy timeout/retry/backoff da migliorare).

Il tool `hammer` non è il problema. Il limite attuale e nella gestione write dell'indice DB.
