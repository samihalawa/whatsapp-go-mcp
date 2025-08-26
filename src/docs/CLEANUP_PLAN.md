# Cleanup Plan for WhatsApp MCP

## 🗑️ Files to Remove

### Binary Files (78MB total)
- `whatsapp` - 39MB compiled binary
- `whatsapp-mcp` - 39MB old compiled binary

### Duplicate/Obsolete MCP Handlers
Since we're using optimized_v2.go, we can remove:
- `ui/mcp/app.go` - Old individual handler (replaced by optimized)
- `ui/mcp/send.go` - Old individual handler
- `ui/mcp/user.go` - Old individual handler
- `ui/mcp/message.go` - Old individual handler
- `ui/mcp/group.go` - Old individual handler
- `ui/mcp/chat.go` - Old individual handler
- Keep `ui/mcp/optimized.go` for reference (v1)
- Keep `ui/mcp/optimized_v2.go` as main implementation

### Documentation to Move
- `MISSING_FEATURES.md` - Move to docs/
- `IMPLEMENTATION_COMPARISON.md` - Move to docs/
- `CLEANUP_PLAN.md` - Move to docs/

## 🧹 Code Cleanup

### Unused Imports
Need to check and clean:
- All Go files for unused imports
- Especially in cmd/mcp.go where we commented out old handlers

### Dead Code
- Commented out handler registrations in cmd/mcp.go
- Any unused helper functions

### .gitignore Updates
Add to .gitignore:
- `whatsapp` (binary)
- `whatsapp-mcp` (binary)
- `*.exe`
- `*.log`
- `*.tmp`
- `.DS_Store`

## 📁 Structure Improvements

### Create docs/ directory
```
docs/
├── MISSING_FEATURES.md
├── IMPLEMENTATION_COMPARISON.md
├── CLEANUP_PLAN.md
└── V2_MIGRATION.md
```

## 🔒 Safe Cleanup Strategy

1. **Phase 1: Remove binaries** (Safe, 78MB saved)
2. **Phase 2: Move documentation** (Safe, better organization)
3. **Phase 3: Remove old MCP handlers** (Safe, using v2 now)
4. **Phase 4: Clean imports and dead code** (Safe with validation)
5. **Phase 5: Update .gitignore** (Safe, prevent future issues)

## 📊 Expected Results

- **Disk space saved**: ~78MB
- **Files removed**: 8 files
- **Files reorganized**: 3 files
- **Code quality**: Improved with no dead code
- **Repository size**: Significantly reduced