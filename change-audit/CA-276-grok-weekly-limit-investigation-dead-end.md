# CA-276 — Grok SuperGrok Weekly Limit: Mechanism Found, Confirmed Unreachable

## Scope

Investigated [Task-217](../requirements/08-Task/todo/Task-217-Grok-SuperGrok-Weekly-Included-Usage-Fetch.md), the follow-up to [CA-275](./CA-275-grok-real-usage-quota-fetch.md)/[Task-216](../requirements/08-Task/done/Task-216-Grok-Real-Usage-Quota-Fetch.md): the user compared the shipped "Team Credits (Monthly)" line against the real `grok` CLI's own status output and found a second, distinct metric — a SuperGrok subscription weekly allowance ("Weekly limit: 1%", resets weekly) — not covered by `Task-216`'s endpoint.

## Changes

- None retained. Ran a real interactive `grok` session (fed via a FIFO to stdin with `--no-alt-screen --minimal`, no true TTY required) under `--debug --debug-file` and located the exact mechanism: an ACP extension method `x.ai/billing` (raw JSON-RPC `{"jsonrpc":"2.0","id":N,"method":"x.ai/billing","params":{}}`, sent right after `initialize`, no `authenticate`/`session/new` needed), returning `{"config":{"creditUsagePercent":1.0,"currentPeriod":{"type":"USAGE_PERIOD_TYPE_WEEKLY","end":"2026-07-16T07:31:03+00:00",...}},"subscription_tier":"SuperGrok"}` — an exact match to the CLI's displayed value and reset time.
- Implemented a Go fetch (`grok_weekly_quota.go`: spawn `grok agent stdio`, send the same two-frame sequence, map the response) and unit tests, wired into `loadGrokAccountMetadata`. Live end-to-end verification against the real connected account returned `{"error":{"code":-32601,"message":"Method not found"}}` — **`x.ai/billing` is not registered on the external `agent stdio` ACP surface** that FlowPilot's `grokAdapter`/`grokDispatcher` (and every other provider adapter in this codebase) is built to speak; it exists only inside the full interactive TUI's own internal client↔agent split.
- All added code was reverted in the same session rather than ship a fetch that would always silently return nil in production. `go build`/`go vet`/`go test ./internal/cli/...` confirmed a clean return to the `Task-216`-only baseline (10 tests).

## Verification

- Live-verified the exact `x.ai/billing` request/response shape from a real interactive session.
- Live-verified the same call fails with `-32601 Method not found` on `grok agent stdio` (the only Grok transport FlowPilot uses).
- `go build ./internal/cli/...`, `go vet ./internal/cli/...`, `go test ./internal/cli/...` — clean, 10 passed, identical to the pre-investigation state.
- No leftover code, temp files, or test artifacts from this investigation remain in the tree.

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: Task-217
change_type: other
summary: investigated surfacing Grok's SuperGrok weekly usage allowance; found the exact ACP extension method (x.ai/billing) but confirmed it is unreachable from the external agent-stdio surface FlowPilot uses, so the implementation was reverted and the dead end documented rather than shipped
# --->8---
