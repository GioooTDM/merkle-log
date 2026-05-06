# Client UI (`/client`)

Web interface for using and verifying the transparency log.

## Quick start
From `client/`:

```bash
python3 -m http.server 8080
```

Open:
- `http://localhost:8080/index.html`

Prerequisite: backend server running at `http://localhost:2025` (default).

## Operational notes
- Blockchain anchoring is still under development.
- The client does not sign events: it sends JSON payloads to the server.
- Checkpoint signature verification has not been implemented yet.
