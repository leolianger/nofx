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

### 2026-03-23 10:07 — Context Propagation, Component Extraction, Security & Reliability
- [DONE] Propagate `c.Request.Context()` in all kline API handlers (was `context.Background()`)
  — client disconnects now cancel upstream calls to CoinAnk, Alpaca, TwelveData, Hyperliquid
- [DONE] Extract `MessageRenderer.tsx` from `AgentChatPage.tsx` (1009 → 825 lines)
  — renderMessageContent + renderInline moved to `components/agent/MessageRenderer.tsx`
- [DONE] Remove `/api/crypto/decrypt` from public routes — was a security hole allowing
  anyone to decrypt ciphertext without authentication (internal callers use service directly)
- [DONE] Add `safe.Go` / `safe.GoNamed` panic recovery wrapper (`safe/go.go`)
  — 31 goroutines had zero `recover()` calls; a single panic would crash the entire process
- [DONE] Apply `safe.GoNamed` to all trader launch goroutines (StartAll, RestoreRunning,
  LoadSingleTrader auto-start, API start/restart handlers)

## Pending

### 2026-03-23 10:22 — Complete Panic Recovery Coverage
- [DONE] Apply `safe.GoNamed` to all 27 remaining bare goroutines across 21 files:
  — 9 exchange order_sync (OKX, Hyperliquid, Aster, Bybit, KuCoin, Gate, Bitget, Lighter, Binance×2)
  — Drawdown monitor, Brain news+briefs, Sentinel, Scheduler
  — x402 + MCP stream idle watchdogs, Rate limiter cleanup
  — 3 telemetry sends, CoinAnk WS handler, API server goroutine
  — Manual defer/recover for Telegram bot (sends error to user) and trader data fetch (prevents deadlock)
- [DONE] Zero bare `go func()` remaining outside safe.go itself
- Build verified ✅, pushed to feat/nofxi

## Pending

### Security
- [DONE] Upgrade go-ethereum v1.16.7→v1.16.8 (fixes 3 vulns: GO-2026-4508, GO-2026-4314, GO-2026-4315)
- [DONE] Upgrade golang-jwt/jwt v5.2.0→v5.2.2 (fixes GO-2025-3553 memory alloc DoS)
- [DONE] Upgrade quic-go v0.54.0→v0.57.0 (fixes GO-2025-4233 HTTP/3 QPACK DoS)
- [PENDING] 3 stdlib vulns remain (GO-2026-4599/4600/4601) — need Go 1.26.1 upgrade (currently on 1.26.0)
- [PENDING] GitHub Dependabot still reports 21 vulns — some may be transitive/test-only, needs further triage

### Code Quality
- [DONE] Extract WelcomeScreen, ChatMessages, ChatInput from AgentChatPage (825→480 lines)
- [DONE] Sanitize 3 more error message leaks in API responses (handler_trader.go ×2, handler_ai_cost.go ×1)
- [DONE] Consistent safe type helpers in ALL auto_trader files — zero raw `pos["key"].(type)` remaining:
  — auto_trader_decision.go: 2 leverage lookups → posFloat64
  — auto_trader_grid.go: emergencyExit + getDecisionContext → posFloat64/posString
  — auto_trader_grid_orders.go: position value calc + state sync + close → posFloat64/posString
  — auto_trader_grid_regime.go: GetGridRiskInfo position lookup → posFloat64/posString
  — auto_trader_loop.go: leverage + createdTime → posFloat64/posInt64
  — auto_trader_orders.go: duplicate position check + close fallback → posString/posFloat64
  — auto_trader_risk.go: drawdown monitor position data → posFloat64/posString
  — Added `posInt64` helper for int64 extraction (createdTime, timestamps)
- [DONE] Fix emergencyExit: log CloseLong/CloseShort errors + GetPositions failure (was silently dropping)
- [DONE] Upgrade closeAllPositions log severity: Infof → Warnf for close failures
- [PENDING] `context.Background()` used in ~69 exchange/trader calls — should propagate request context for proper cancellation (partially done: kline handlers fixed, trader/exchange calls remain)

### Security — Response Body Limits
- [DONE] Created `safe/io.go` with `ReadAllLimited` (default 10MB limit) to prevent OOM from malicious responses
- [DONE] Replaced 62 unbounded `io.ReadAll(resp.Body)` calls across 32 files (all exchange traders, providers, MCP, market, wallet, telegram, kernel)

### Robustness — HTTP Status Code Checks
- [DONE] Add HTTP status code checks in market/api_client.go (exchangeInfo, klines, price)
- [DONE] Add HTTP status code checks in market/data.go (openInterest, premiumIndex)
- [DONE] Add HTTP status code checks in provider/coinank (GET + POST)
- [DONE] Add HTTP status code checks in provider/twelvedata (timeSeries + quote)
- [DONE] Add `truncateBody` helper for safe error messages

### Performance
- [DONE] Reuse shared HTTP client in Hyperliquid trader (was creating new client per API call, preventing TCP/TLS connection reuse)
- [DONE] Reuse shared HTTP client in Bybit trader — replaced `http.Get` (no timeout!) and `http.DefaultClient` with `bybitHTTPClient` (30s timeout, connection pooling)
- [PENDING] `gatherContext` in agent.go iterates all traders and positions on every message — consider caching (low priority: only triggered per user message)

### Frontend Auth Bugs (2026-03-23 11:22)
- [DONE] Fix `resetPassword` in AuthContext.tsx — was calling `/api/reset-password` without auth token (endpoint moved behind auth middleware, so it always returned 401)
- [DONE] Fix SettingsPage.tsx password change — was using `localStorage.getItem('token')` but auth system stores as `auth_token` (always sent empty Bearer token)

### Code Consistency
- [DONE] Migrate remaining `io.ReadAll(io.LimitReader())` in agent/brain.go and agent/sentinel.go to `safe.ReadAllLimited` — consistent usage across codebase, removed unused `io` imports

### 2026-03-23 11:52 — Panic Prevention in Trading Code + API Body Limits
- [DONE] Add `requestBodyLimitMiddleware` (1MB) — all API endpoints now reject oversized payloads (prevents OOM)
- [DONE] Fix `defer resp.Body.Close()` inside loop in `getPublicIPFromAPI` — was leaking connections
- [DONE] Add `posFloat64`/`posString` safe helpers in `trader/helpers.go`
- [DONE] Convert 30+ unsafe type assertions (`pos["key"].(type)`) to safe comma-ok pattern across ALL exchange traders:
  — OKX, Hyperliquid, Aster, Bybit, KuCoin, Gate, Bitget, Binance
  — auto_trader_risk.go (drawdown monitor could panic → silently stop protecting positions)
  — auto_trader_decision.go (trading decisions)
  — auto_trader_loop.go (core trading loop)
- [DONE] Zero unsafe type assertions remaining in `trader/` package
- [DONE] Fix frontend `config.ts`: rejected promise cached forever on network error (never retried)

### 2026-03-23 12:37 — Agent Intelligence + Resource Leak Fixes
- [DONE] Expand gatherContext crypto detection: 10 hardcoded → 38+ known symbols + dynamic XXXUSDT pattern extraction
  — Users asking about PEPE, SUI, TRUMP, etc. now get real-time data enrichment
  — Caps at 5 symbols per message to prevent slow context gathering
- [DONE] Add `get_trade_history` tool to agent LLM toolset
  — Returns recent closed trades with PnL per trade, win rate, total PnL summary
  — Users can now ask "how have my trades been doing?" and get structured data
- [DONE] Fix ticker leak in ALL 9 exchange order_sync goroutines (missing `defer ticker.Stop()`)
  — OKX, Hyperliquid, Aster, Bybit, KuCoin, Gate, Bitget, Lighter, Binance
  — Each stopped trader was leaking a goroutine + ticker forever

### Features
- [PENDING] Agent chat has fake streaming (word-by-word setTimeout) — implement real SSE streaming
- [PENDING] Add WebSocket support for real-time position/balance updates instead of polling
