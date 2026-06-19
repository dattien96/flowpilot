# CA-105: Claude Account Quota Usage Fetch

## Scope

Local-runner `cli` package — Claude account metadata loading in `provider_account_terminal.go`.

Adds real-time 5h/7d quota data for Claude accounts so the account sidebar shows the same usage bars already present for Codex and Gemini.

## Completed

**Root cause confirmed**: `loadClaudeAccountMetadata` never fetched quota data. It read only static fields from `.claude.json` (plan type, extra-usage disabled reason). `claude auth status --json` returns no quota fields. As a result `remaining5hPercent` and `remaining7dPercent` were always `nil` for Claude, so no bars appeared in the UI and the auto-switch fallback could not rank Claude accounts by quota.

**Discovery**: The Anthropic inference API returns `anthropic-ratelimit-unified-5h-utilization` and `anthropic-ratelimit-unified-7d-utilization` headers on every successful 200 response. These express the **used** fraction (0.0–1.0); remaining percent = `100 - int(utilization*100)`. The OAuth Bearer token is stored in `~/.claude/.credentials.json` under `claudeAiOauth.accessToken` — a separate file from `.claude.json`.

**Changes in `provider_account_terminal.go`**:

1. Added `claudeCredentialsFile` struct to parse `claudeAiOauth.{accessToken, expiresAt}` from `.credentials.json`.

2. Added `loadClaudeQuota(homePath string)`:
   - Reads access token from `<homePath>/.claude/.credentials.json`; bails silently if missing or expired (`expiresAt` millisecond check)
   - POSTs `{"model":"claude-haiku-4-5-20251001","max_tokens":1,"messages":[{"role":"user","content":"x"}]}` with Bearer auth and `anthropic-version: 2023-06-01` header
   - Parses `anthropic-ratelimit-unified-5h-utilization` → `remaining5h = clampInt(100 - int(f*100))`
   - Parses `anthropic-ratelimit-unified-5h-reset` (unix epoch seconds) → ISO 8601 string via `time.Unix(secs, 0).UTC().Format(time.RFC3339)`
   - Same for the 7d variants
   - Returns `nil` percentages on any HTTP error or non-200 status (silent degradation)

3. Wired into `loadClaudeAccountMetadata`: calls `loadClaudeQuota` after `buildClaudeUsageSummary`, then populates `remaining5hPercent`, `remaining5hResetAt`, `remaining7dPercent`, `remaining7dResetAt`, and appends to `usageDetailLines`.

**Codex path untouched** — Codex quota already works via `https://chatgpt.com/backend-api/wham/usage`; no changes made there.

## Verification

- `go build ./internal/cli/...` — clean build, no errors.
- `go test ./internal/cli/...` — 5/5 existing tests pass.
- Live probe confirmed header format and utilization semantics:
  - `anthropic-ratelimit-unified-5h-utilization: 0.24` → remaining = 76%
  - `anthropic-ratelimit-unified-7d-utilization: 0.37` → remaining = 63%
  - Reset timestamps present as unix epoch integers, converted to ISO 8601.
- API call cost: ~8 input tokens + 1 output token per account refresh (Haiku pricing, essentially free).

## Residual Notes

- Token expiry handling is best-effort: if the stored token is stale and the Claude CLI has not refreshed it yet, quota fetch is silently skipped. The bars disappear but no crash or error surfaces to the user.
- Model-specific sub-bucket headers (e.g. "Sonnet only") are not returned by the current API; only unified 5h/7d headers are available.
- Claude auto-switch still uses slot-order fallback (not quota ranking) — `Q-1` from Task-018 is now unblocked but requires a separate implementation slice.
- No caching implemented; quota is re-fetched on every account list refresh. If latency becomes visible, a 5-minute file cache is the recommended follow-up (`Q-2` in Task-080).
