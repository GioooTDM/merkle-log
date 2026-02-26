# Client UI (`/client`)

Interfaccia web per usare il transparency log.

## Avvio rapido
Da dentro `client/`:

```bash
python3 -m http.server 8080
```

Poi apri:
- `http://localhost:8080/index.html`

## Configurazione endpoint API
Le pagine usano `API_BASE = "http://localhost:2025"` direttamente negli script.
Se cambi host/porta backend, aggiorna quel valore nei file HTML/JS interessati.

## Note operative
- La parte di ancoraggio blockchain non è ancora implementata.
- Il client non firma eventi: invia payload JSON al server.
- Non ho ancora implementato il controllo sulle firme dei checkpoint.
