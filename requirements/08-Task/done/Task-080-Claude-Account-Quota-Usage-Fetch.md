
# Task-080: Claude Account Quota Usage Fetch

## Metadata

- Document ID: `Task-080`
- Title: `Claude Account Quota Usage Fetch`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-19`
- Last Updated: `2026-06-19`
- Parent Documents: [SS-05: Workflow AI Provider](../../05-System-Specs/SS-05-Workflow-Ai-Provider.md), [SD-06: AI Provider Integration](../../06-System-Tech-Design/SD-06-AI-Provider-Integration.md)
- Child Documents: `—`
- Related Documents: [Task-018: Auto Switch Account On Provider Usage Limit](../done/Task-018-Auto-Switch-Account.md), [Task-015: UI Show Account Execution Limit Usage As Percent](../done/Task-015-UI-Acc-Limit-Percent.md), [Task-036: Desktop Provider Accounts Sidebar](../done/Task-036-Desktop-Provider-Accounts-Sidebar.md), [BUG-090: Cross-Provider Chat Parity Gaps](../../09-BugFix/done/BUG-090-Cross-Provider-Chat-Parity-Gaps.md), [CA-105: Claude Account Quota Usage Fetch](../../../change-audit/CA-105-claude-account-quota-usage-fetch.md)
- Replaces: `—`
- Tags: `local-runner, claude, quota, provider-accounts, usage-limit`

## AI Quick View

### Summary

- `loadClaudeAccountMetadata` never made an HTTP call to fetch quota data — it only read static fields from `.claude.json`, so `remaining5hPercent` and `remaining7dPercent` were always `nil` for Claude accounts.
- The Anthropic inference API returns `anthropic-ratelimit-unified-5h-utilization` and `7d-utilization` headers on every successful call; these carry the used-fraction (0.0–1.0) that converts to remaining% via `100 - int(utilization*100)`.
- The OAuth Bearer token lives in `~/.claude/.credentials.json` under `claudeAiOauth.accessToken`; a minimal Haiku call (1 input word, max_tokens=1) is the cheapest viable way to obtain these headers (~9 tokens total).
- This unblocks Q-1 from Task-018 (trustworthy Claude quota telemetry) and allows the auto-switch candidate ranking to use real quota data in a future slice.

### Current Ask

- Implement `loadClaudeQuota` in `provider_account_terminal.go` to read the access token, call the Anthropic API, parse the rate-limit headers, and populate `remaining5hPercent`, `remaining7dPercent`, reset timestamps, and `usageDetailLines` on the Claude account metadata.

### Key Decisions

- `T-1` Read credentials from `~/.claude/.credentials.json` (`claudeAiOauth.accessToken`), not from `.claude.json` (which has no token).
- `T-2` Use a single POST to `https://api.anthropic.com/v1/messages` with `claude-haiku-4-5-20251001`, `max_tokens=1`, and a one-character message — the cheapest viable call that returns the unified rate-limit headers.
- `T-3` Remaining percent = `100 - int(utilization * 100)`; utilization headers express **used** fraction, not remaining.
- `T-4` Skip the API call when the stored token is already expired (`expiresAt` milliseconds check); return `nil` percentages silently without logging.
- `T-5` Do not cache quota results — Codex follows the same pattern (no caching per refresh); consistency and simplicity take priority over token savings at this call volume.
- `T-6` Do not touch Codex quota logic; the Codex path (`loadCodexAccountMetadata`) already works via `https://chatgpt.com/backend-api/wham/usage`.

### Constraints

- Do not modify Codex account metadata loading.
- Do not make an inference call if the token is expired.
- Do not block account list loading on quota fetch failures — all errors silently return `nil` percentages.
- `clampInt` must guard the computed remaining percent to handle edge cases where utilization > 1.0.

### Open Questions

- `Q-1` Should a future slice use the now-available `remaining5hPercent` and `remaining7dPercent` for Claude to enable ranked best-account selection in the auto-switch flow (currently using slot-order fallback only)?
- `Q-2` Should quota results be cached short-term (e.g. 5 min file cache) if the API call latency becomes noticeable at account refresh time?

### Source Refs

- `apps/local-runner/internal/cli/provider_account_terminal.go` — `loadClaudeAccountMetadata`, new `loadClaudeQuota`, new `claudeCredentialsFile`
- `anthropic-ratelimit-unified-5h-utilization` / `7d-utilization` response headers from `POST https://api.anthropic.com/v1/messages`
- `Task-018` Q-1: "Should Claude later expose a runner-normalized candidate score once better quota telemetry exists?"

## 1. Goal

Populate `remaining5hPercent`, `remaining7dPercent`, their reset timestamps, and `usageDetailLines` for Claude accounts in the account panel sidebar — matching the Codex and Gemini quota bar display already in place.

## 2. Parent Links

- coding plan: no dedicated CP; this task directly resolves `Q-1` from Task-018 and fills a parity gap noted in BUG-090
- tech design: [SD-06: AI Provider Integration](../../06-System-Tech-Design/SD-06-AI-Provider-Integration.md)
- system spec: [SS-05: Workflow AI Provider](../../05-System-Specs/SS-05-Workflow-Ai-Provider.md)
- specific upstream ids: Task-018 `T-5`, Task-018 `Q-1`

## 3. Trigger

Task-018 (`T-5`) explicitly deferred numeric Claude quota ranking because "trustworthy telemetry" did not yet exist. Investigation in this session confirmed:

1. `claude auth status --json` does not expose quota percentages.
2. `.claude.json` stores no OAuth token.
3. `~/.claude/.credentials.json` stores `claudeAiOauth.accessToken`.
4. The Anthropic inference API returns unified rate-limit utilization headers (`anthropic-ratelimit-unified-5h-utilization`, `7d-utilization`) on every successful 200 response.
5. A minimal Haiku call reliably returns those headers and costs ~9 tokens.

This constitutes trustworthy telemetry and unblocks the Claude quota display.

## 4. Exact Change

- `T-1` Add `claudeCredentialsFile` struct to `provider_account_terminal.go`:
  ```go
  type claudeCredentialsFile struct {
      ClaudeAiOauth struct {
          AccessToken string `json:"accessToken"`
          ExpiresAt   int64  `json:"expiresAt"`
      } `json:"claudeAiOauth"`
  }
  ```
- `T-2` Add `loadClaudeQuota(homePath string) (remaining5h *int, reset5h string, remaining7d *int, reset7d string)`:
  - Reads `~/<homePath>/.claude/.credentials.json`
  - Skips if token is empty or `expiresAt` is past
  - POSTs `{"model":"claude-haiku-4-5-20251001","max_tokens":1,"messages":[{"role":"user","content":"x"}]}` with Bearer auth and `anthropic-version: 2023-06-01`
  - Parses `anthropic-ratelimit-unified-5h-utilization` → `remaining5h = 100 - int(f*100)` (clamped)
  - Parses `anthropic-ratelimit-unified-5h-reset` unix timestamp → ISO 8601 string
  - Same for 7d
- `T-3` Wire `loadClaudeQuota` into `loadClaudeAccountMetadata`: call after `buildClaudeUsageSummary`, populate `metadata.remaining5hPercent`, `remaining5hResetAt`, `remaining7dPercent`, `remaining7dResetAt`, and append to `metadata.usageDetailLines` when non-nil.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/cli/provider_account_terminal.go`
- modules:
  - `cli` package — Claude account metadata loading
- routes: none
- tables: none

## 6. Acceptance Check

- `go build ./internal/cli/...` passes with no errors.
- `go test ./internal/cli/...` — all 5 existing tests pass.
- In the running app, the Claude account in the sidebar shows "Remaining 5h" and "Remaining 7d" progress bars (matching the Codex account display style).
- Values match what the official Claude.ai app reports for the same account (utilization → remaining conversion verified: utilization=0.36 → 64% remaining).
- When the token is missing or expired, the account panel degrades gracefully (no bars, no crash).

## 7. Out of Scope

- Upgrading the auto-switch candidate ranking for Claude to use numeric quota (still slot-order only; tracked as `Q-1` above).
- Token refresh — if the stored token is expired, quota fetch is skipped silently; the Claude CLI handles refresh on next actual run.
- Caching quota results between refreshes (tracked as `Q-2`).
- Codex quota logic — untouched.
- Gemini quota logic — untouched.
- Model-specific sub-bucket headers (e.g. Sonnet-only percentage) — not exposed by the current API response.

## 8. Completion Notes

- result: implemented and verified — `go build` and `go test ./internal/cli/...` pass; sidebar expected to show quota bars on next account refresh.
- follow-ups:
  - `Q-1`: Use `remaining5hPercent`/`remaining7dPercent` for ranked Claude auto-switch candidate selection in a follow-up slice of Task-018.
  - `Q-2`: Add a short-lived file cache for quota results if account-refresh latency becomes noticeable.
- upstream docs updated:
  - Task-018 `Q-1` is now answered (telemetry exists); no structural change required to Task-018 or SD-06.
