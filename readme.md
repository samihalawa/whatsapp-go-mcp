# WhatsApp GO MCP

<a href="https://smithery.ai/server/@samihalawa/whatsapp-go-mcp"><img src="https://smithery.ai/badge/@samihalawa/whatsapp-go-mcp"></a>

WhatsApp MCP server for AI agent integration via Model Context Protocol.

### Key Features
- 50+ MCP tools for WhatsApp interactions (send, receive, groups, contacts)
- HTTP Streamable transport (Smithery.ai compatible)
- Stateless mode for scalability
- Session management with SQLite
- Media handling (images, videos, audio, files)
- Group and contact management
- Message reactions, replies, and editing
- Persistent storage volume (`/app/storages`)

---
### Quick Start (MCP Mode)

**Deploy on Smithery:**
```bash
npx @smithery/cli deploy
```

**Local Development:**
```bash
git clone https://github.com/samihalawa/whatsapp-go-mcp.git
cd whatsapp-go-mcp/src
go run . mcp
```

**Docker:**
```bash
docker build -t whatsapp-go-mcp .
docker run -p 8081:8081 whatsapp-go-mcp
```

MCP server runs on port 8081 by default.

---
### MCP Endpoints
- `/mcp` - MCP HTTP stream (main endpoint)
- `/health` - Health check
- `/tools` - Available tools catalog (JSON)

### Available Tools
The MCP server provides 50+ tools organized in categories:
- **App**: QR login, device management, logout
- **Send**: Text, images, videos, audio, files, contacts, locations, polls
- **User**: Info, avatar, contacts, groups, privacy settings
- **Message**: React, delete, edit, star, mark as read
- **Group**: Create, manage, participants, settings
- **Chat**: List, archive, delete conversations
- **Newsletter**: Manage newsletter subscriptions

See `/tools` endpoint for complete list.

---
### Configuration
Environment variables (optional):
- `PORT` - Server port (default: 8081)
- `DB_URI` - Database path (default: file:storages/whatsapp.db)
- `APP_OS` - WhatsApp device label (default: Chrome)

Command-line flags:
```bash
./whatsapp mcp --port 8081 --host 0.0.0.0
```

---
### Storage
WhatsApp session data is persisted in `/app/storages` (Docker volume).
This includes authentication state and message history.

---
### Development
```bash
# Build
cd src && go build -o whatsapp

# Test
go test ./...

# Run locally
./whatsapp mcp
```

Requirements: Go 1.24+, SQLite (CGO enabled)

---
### Notes
- Unofficial project (not affiliated with WhatsApp)
- Built on whatsmeow library for WhatsApp Web Multi-Device
- Smithery.ai compatible with HTTP Streamable transport