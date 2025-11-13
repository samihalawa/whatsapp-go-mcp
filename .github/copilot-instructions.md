# Copilot Onboarding – whatsapp-go-mcp

Trust these instructions. Only search if information is missing or demonstrably outdated.

## 1. Summary & High-Level Details
Project: Go-based WhatsApp Web Multi-Device server with two mutually exclusive modes:
- REST API (port 3000 default) defined in `docs/openapi.yaml`
- MCP HTTP stream server (port 8081 default) for AI tooling (Smithery-compatible)

Languages (approximate): Go (~64%), JavaScript (~32%), plus HTML/CSS assets in `src/views` / `src/statics`.

Primary external components:
- whatsmeow (WhatsApp Web protocol)
- FFmpeg (media processing)
- SQLite (default persistence)
- Chromium (inside container for WhatsApp Web automation)

## 2. Build & Validation Workflow
Always run from `src/` (module root).

Bootstrap (ALWAYS do before build):
```bash
cd src
go mod download
go mod tidy
```

Build:
```bash
go build -o whatsapp    # Linux/macOS
go build -o whatsapp.exe # Windows
```

Run REST:
```bash
./whatsapp rest
# or
go run . rest
```
Run MCP:
```bash
./whatsapp mcp
# or
go run . mcp
```

REST verification:
- Open http://localhost:3000
- Hit a lightweight endpoint (e.g. GET /app/devices after session established)
MCP verification:
- GET http://localhost:8081/health returns JSON status ok
- GET http://localhost:8081/tools returns tool catalog JSON

Testing:
```bash
cd src
go test ./...
go test -cover ./...
```

Lint / sanity:
```bash
go fmt ./...
go vet ./...
```

Docker (REST via docker/golang.Dockerfile):
```bash
docker-compose up -d --build
```
Docker (MCP via root Dockerfile):
```bash
docker build -t whatsapp-go-mcp .
docker run -p 8081:8081 whatsapp-go-mcp
curl http://localhost:8081/health
```

Environment notes:
- FFmpeg required for media if not using Docker.
- macOS possible CGO flag: `export CGO_CFLAGS_ALLOW="-Xpreprocessor"` (only if build errors reference it).
- Only one mode active at a time.

ALWAYS:
- Run `go mod tidy` after adding imports.
- Rebuild before tests if flags changed.
- Validate `/health` and `/tools` when modifying MCP code.

## 3. Architecture & Layout
Root:
- `Dockerfile` (MCP image)
- `docker/golang.Dockerfile` (REST build)
- `docker-compose.yml` (REST dev)
- `deploy.sh` (Smithery packaging)
- `smithery.yaml`, `SMITHERY_DEPLOYMENT.md`
- `docs/` (openapi.yaml, webhook-payload.md)
- `readme.md`

Source (`src/`):
- `cmd/` (Cobra commands: rest, mcp)
- `ui/rest/` (HTTP handlers)
- `ui/mcp/` (MCP tool registration)
- `domains/` (business logic per area)
- `usecase/` (orchestration)
- `infrastructure/` (WhatsApp client, DB)
- `validations/`
- `pkg/` (utilities)
- `statics/` (media, assets)
- `views/` (templates)

MCP server (`src/cmd/mcp.go`): endpoints `/mcp`, `/health`, `/tools`; tool groups: app, send, user, message, group, chat, newsletter.
REST server (`src/cmd/rest.go`): Fiber app, static assets, optional basic auth, domain route initialization.

Configuration order: flags > env vars > `.env`.

OpenAPI alignment: modify `docs/openapi.yaml` when you add or change REST endpoints.

## 4. Validation Checklist Before PR
1. `cd src && go build`
2. `go test ./...`
3. If REST endpoints changed: openapi.yaml updated.
4. If MCP tools changed: `/tools` lists them.
5. `go vet ./...` passes.
6. Docker builds succeed for affected mode.

Optional confidence:
- Remove `storages/` contents and re-run to confirm auto recreation.
- Test a media send after FFmpeg availability.
- Simulate webhook: run with `--webhook=` pointing at a local mock and observe post.

## 5. Quick Index
- `docs/openapi.yaml`: definitive REST contract.
- `docs/webhook-payload.md`: payload schemas & security header.
- `deploy.sh`: Smithery binary packaging.
- MCP: served at port 8081 by default, `/mcp` stream endpoint.

## 6. Missing vs Legacy
Ignore obsolete references to SSE endpoints not in current `mcp.go`. Use only documented endpoints above unless adding new ones intentionally.

## 7. Guidance
Trust these instructions. Search only when something here is missing or verifiably wrong.
