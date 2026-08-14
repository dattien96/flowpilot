---
id: CA-445
feature_key: cli-tui
title: Repair incomplete TUI/Grok bugfixes (shutdown, auth, accounts, skills, retries)
date: 2026-08-13
status: COMPLETE
---

## flowpilot:change-ledger

```yaml
feature_key: cli-tui
prior_ca: CA-444-fix-just-chat-dev-project-flag-parsing
will_not_undo: CA-443 TUI client; CA-444 unique auth-session tmp files and tui-session persistence
```

## Summary

Gemini's handoff claimed 10 issues were fixed. Several were incomplete or wrong against live APIs. This pass repairs the real TUI/desktop contracts without changing the Grok runner that Desktop already uses.

## Root causes (Gemini leftovers)

1. `/exit` always called `POST /system/shutdown` + port kill, including when TUI **reused** Desktop's runner.
2. Gemini patched Grok `session/new` to send `authMethodId`. Live Grok 1.0.3 (logged-in account) accepts the Desktop-stable shape (`cwd` + `mcpServers` only). TUI must not change that runner.
3. `POST /provider-accounts/activate` returns `{"account":...}` but TUI unmarshaled the wrapper as the account.
4. Skills client called `/client/skills`; runner serves `GET /client/provider-skills`.
5. `IsRetryableAPIError` treated any error string containing `"409"` as retryable (e.g. port 4090).
6. Windows `netstat` used `HasSuffix(":"+port)`, matching `14317` for port `4317`.
7. `SessionDefaultsMsg` reprinted "Ready" and refused to refresh the account label on `/provider account` reload.
8. High-reasoning Grok turns stayed on "streaming events…" because thought chunks are not client-facing; no thinking status.

## Changes

- Shutdown only when `ChatConfig.OwnsRunner` (TUI spawned the process).
- **Reverted** Grok runner `authMethodId` on `session/new`/`session/load`. Desktop-stable ACP is unchanged.
- Unwrap activate JSON; map `display_name` → `DisplayLabel`.
- List skills from `/client/provider-skills`.
- Retry HTTP 409 / gate codes only; do not substring-match `"409"`.
- Exact TCP port match for Windows listen PID.
- First-load-only Ready banner; refresh account label on later `SessionDefaultsMsg`.
- Status `thinking…` while reasoning=high until first `message_delta`.
- Clear `runHandle` on turn-stream errors; keep prefixed run IDs (`run-105935`).
- Windows auth-session rename: replace dest on EEXIST/EPERM.

## Tests added (new files only)

- `internal/runner/grok_live_session_new_compat_test.go` (opt-in `FLOWPILOT_LIVE_GROK=1`; proves `session/new` without `authMethodId`)
- `internal/tui/client/client_activate_skills_retry_test.go`
- `internal/tui/app/shutdown_port_test.go`
- `internal/tui/app/bugfix_regression_test.go`

## Verification

`go test ./internal/tui/... ./internal/cli/...` passed.

Live Grok 1.0.3 after runner revert (`FLOWPILOT_LIVE_GROK=1`):
- `TestLiveGrokSessionNewWithoutAuthMethodID` PASS (`session/new` without `authMethodId`, then `session/prompt`)
- `TestLiveRealGrokChatStreamAndResume` PASS (adapter `session/new` → PINEAPPLE, `session/load` resume → PINEAPPLE)

## Provider impact

- Grok runner ACP: **unchanged** (Case 3 path left alone; live-verified Desktop `session/new` still works). Claude/Codex adapters were never touched.
- TUI client paths (shutdown, accounts, skills, 409 retry, statusline) are provider-agnostic.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: Repair TUI shutdown/auth/account/skills/retry contracts that Gemini left incomplete
# --->8---
