# NOFXi Self-Drive Tasks

## Completed

### 2026-03-23 09:37 — Security Hardening Batch
- [DONE] Fix health endpoint returning `"time": null` → now returns proper RFC3339 timestamp
- [DONE] Add login rate limiting (5 attempts / 15min, 5min block) to prevent brute-force
- [DONE] Move `/api/reset-password` behind auth middleware (was unauthenticated — critical vuln)
- [DONE] Add response body size limits on proxy/external API calls (prevent memory exhaustion)
- [DONE] Sanitize error messages in agent chat endpoint (was leaking internal error details)
- [DONE] Record login success/failure for rate limiter tracking
- [DONE] Sanitize markdown link URLs in frontend (prevent javascript: XSS)

## Pending

### Security
- [PENDING] Investigate GitHub Dependabot's 21 reported vulnerabilities (13 high, 7 moderate, 1 low)
- [PENDING] Add CSRF protection or tighten CORS from wildcard `*` to specific origins
- [PENDING] Agent chat endpoint (`/api/agent/chat`) is not behind auth middleware — should require authentication

### Code Quality
- [PENDING] The `resolveStockCodeDynamic` function at line 210 of `agent/stock.go` also has unlimited io.ReadAll (line 136)
- [PENDING] `context.Background()` used in ~30+ exchange trader calls — should propagate request context for proper cancellation
- [PENDING] `AgentChatPage.tsx` is 1001 lines — could be split into smaller components

### Performance
- [PENDING] `gatherContext` in agent.go iterates all traders and positions on every message — consider caching
- [PENDING] News scan `seen` map grows unbounded, only reset at 1000 entries — use TTL-based expiry

### Features
- [PENDING] Agent chat has fake streaming (word-by-word setTimeout) — implement real SSE streaming
- [PENDING] Add WebSocket support for real-time position/balance updates instead of polling
