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

### 2026-03-23 09:52 — CORS & Response Limits
- [DONE] Add configurable CORS origins (`CORS_ALLOWED_ORIGINS` env var) — replaces wildcard `*`
- [DONE] Add `io.LimitReader` to sentinel.go (was unbounded, now 256KB)
- [DONE] Fix news `seen` map eviction: keep half instead of clearing all (was losing dedup state)
- [DONE] Agent chat endpoint already behind auth middleware (verified, was falsely flagged as pending)

## Pending

### Security
- [PENDING] Investigate GitHub Dependabot's 21 reported vulnerabilities (13 high, 7 moderate, 1 low)

### Code Quality
- [PENDING] `context.Background()` used in ~69 exchange/trader calls — should propagate request context for proper cancellation
- [PENDING] `AgentChatPage.tsx` is 1001 lines — could be split into smaller components

### Performance
- [PENDING] `gatherContext` in agent.go iterates all traders and positions on every message — consider caching (low priority: only triggered per user message)

### Features
- [PENDING] Agent chat has fake streaming (word-by-word setTimeout) — implement real SSE streaming
- [PENDING] Add WebSocket support for real-time position/balance updates instead of polling
