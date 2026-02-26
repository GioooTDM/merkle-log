# Seed Log (`/seed_log`)

Tool CLI per generare dataset di esempio (PDF + eventi JSON) e inviarli a `/add`.

## Cosa fa
- Crea documenti PDF sintetici
- Calcola `payload_hash` (`sha-256`) dei PDF
- Invia eventi `CREATE` e `UPDATE` al server
- Salva:
  - eventi notarizzati restituiti dal server (`event/*.json`)
  - PDF generati (`pdf/*.pdf`)
  - riepilogo (`summary.json`)

## Avvio rapido
Da root repository:

```bash
go run ./seed_log -url http://localhost:2025/add -out ./seed_log/seed_data
```

## Parametri principali
- `-url`: endpoint `POST /add`
- `-out`: directory output dataset
- `-seed`: seed random per riproducibilita
- `-issuer-id`: `issuer.entity_id` degli eventi generati
- `-issuer-name`: `issuer.name` degli eventi generati

## Output atteso
Nella cartella `-out`:
- `pdf/`: file PDF sorgente
- `event/`: JSON notarizzati raw (quelli effettivamente appesi nel log)
- `summary.json`: elenco sintetico con `doc_uid`, `doc_version`, `event_id`, `log_index`, path file