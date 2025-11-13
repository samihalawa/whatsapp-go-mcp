# WhatsApp GO MCP

<a href="https://smithery.ai/server/@samihalawa/whatsapp-go-mcp"><img src="https://smithery.ai/badge/@samihalawa/whatsapp-go-mcp"></a>

WhatsApp GO MCP provides:
- A comprehensive HTTP REST API for WhatsApp multi-device interactions
- A Model Context Protocol (MCP) server for AI agent tooling integration

Only one mode runs at a time (REST or MCP).

---
### Key Features
- REST API endpoints for login, user info, messaging (text, media, polls, presence), chats, groups, newsletters
- MCP server with tool catalog (served at `/tools`)
- Webhook support with HMAC SHA256 signature (`X-Hub-Signature-256`)
- Image/video compression options
- Custom device name (`--os=MyApp`)
- Basic Auth (multi-credential)
- Subpath deployment (`--base-path=/gowa`)
- Auto-reply & auto mark-read toggles
- Chat storage (SQLite by default)
- Media handling via FFmpeg
- Persistent storage volume (`/app/storages`)

---
### Configuration Priority
1. Command-line flags
2. Environment variables
3. `.env` file (lowest)

Copy example:
```bash
cp src/.env.example src/.env
```

Essential environment variables:

| Variable | Purpose | Default |
|----------|---------|---------|
| APP_PORT | REST port | 3000 |
| APP_DEBUG | Debug logging | false |
| APP_OS | WhatsApp device label | Chrome |
| APP_BASIC_AUTH | Basic auth credentials | (none) |
| APP_BASE_PATH | Base subpath prefix | (none) |
| DB_URI | Main DB URI | file:storages/whatsapp.db?_foreign_keys=on |
| WHATSAPP_WEBHOOK | Webhook URLs (comma-separated) | (none) |
| WHATSAPP_WEBHOOK_SECRET | HMAC key | secret |
| WHATSAPP_AUTO_REPLY | Auto-reply text | (none) |
| WHATSAPP_AUTO_MARK_READ | Mark incoming read | false |
| WHATSAPP_ACCOUNT_VALIDATION | Account validation | true |
| WHATSAPP_CHAT_STORAGE | Enable chat storage | true |

Flags override all.

---
### Requirements (Local Build)
- Go 1.24+
- FFmpeg installed (outside Docker)
- SQLite (CGO) included via Go build

macOS fix (if needed):
```bash
export CGO_CFLAGS_ALLOW="-Xpreprocessor"
```

---
### REST Mode
```bash
git clone https://github.com/samihalawa/whatsapp-go-mcp.git
cd whatsapp-go-mcp/src
go run . rest
# or
go build -o whatsapp && ./whatsapp rest
```
Open: http://localhost:3000

### MCP Mode
```bash
cd whatsapp-go-mcp/src
go run . mcp
# or
go build -o whatsapp && ./whatsapp mcp
```
Defaults (flags: `--host`, `--port`):
- Port: 8081 (container & default)
- Endpoints:
  - `/mcp` (MCP HTTP stream)
  - `/health` (status)
  - `/tools` (tool catalog)

### Docker (REST)
Uses `docker/golang.Dockerfile`
```bash
docker-compose up -d --build
# REST at http://localhost:3000
```

### Docker (MCP / Smithery)
Root Dockerfile (multi-stage, Chromium + FFmpeg for WhatsApp web automation):
```bash
docker build -t whatsapp-go-mcp .
docker run -p 8081:8081 whatsapp-go-mcp
```

---
### Webhooks
See <a>docs/webhook-payload.md</a> for:
- Message events (text, reaction, edit, revoke)
- Receipt events (`message.ack`: delivered, read)
- Group participant events
- Media payload shapes
- View-once & forwarded flags
- Integration examples (Node.js, Python)
Configure:
```bash
./whatsapp rest --webhook="https://your.site/hook" --webhook-secret="customkey"
```

---
### API Specification
OpenAPI: <a>docs/openapi.yaml</a>

Base server (REST): `http://localhost:3000`

Endpoints categories: /app, /user, /send, /message, /group, /newsletter, /chats

Refer to the OpenAPI file for latest contract (keep updated when adding endpoints).

---
### MCP Tooling
MCP server publishes tool inventory at `/tools` (JSON).
Smithery integration config: <a>smithery.yaml</a>
Deployment helper: `deploy.sh` (builds binary, bundles statics + docs, runs `smithery deploy`).

---
### Persistent Data
Volume path: `/app/storages`
Includes session DB and optional chat history.

---
### Important
- Unofficial project (not affiliated with WhatsApp).
- REST and MCP modes are mutually exclusive (underlying library constraint).
- Keep OpenAPI and README synchronized with code whenever adding/modifying endpoints.