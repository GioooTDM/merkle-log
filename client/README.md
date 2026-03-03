# Client UI (`/client`)

Interfaccia web per usare e verificare il transparency log.

## Avvio rapido
Da `client/`:

```bash
python3 -m http.server 8080
```

Apri:
- `http://localhost:8080/index.html`

Prerequisito: backend server attivo su `http://localhost:2025` (default).

## Note operative
- La parte di ancoraggio blockchain è in fase di sviluppo.
- Il client non firma eventi: invia payload JSON al server.
- Non ho ancora implementato il controllo sulle firme dei checkpoint.
