# Task-217: Grok SuperGrok Weekly Included-Usage Fetch

## Metadata

- Document ID: `Task-217`
- Title: `Grok SuperGrok Weekly Included-Usage Fetch`
- Phase: `task`
- Status: `todo`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-10`
- Last Updated: `2026-07-10`
- Parent Documents: [CP-46: Grok Build Controlled Adapter Over ACP Transport](../../07-Coding-Plan/inprogress/CP-46-Grok-Build-Controlled-Adapter-Over-ACP.md), [Task-216: Grok Real Usage/Quota Fetch](../../08-Task/done/Task-216-Grok-Real-Usage-Quota-Fetch.md)
- Child Documents: `None`
- Related Documents: [Task-210: Grok Account Model — Detect, Connect, Switch, Quota](../../08-Task/inprogress/Task-210-Grok-Account-Model-Detect-Connect-Switch-Quota.md)
- Replaces: `None`
- Tags: `grok, grok-build, xai, supergrok, quota, usage, billing, provider-accounts, local-runner, desktop-chat`

## AI Quick View

### Summary

- `Task-216` shipped a real Grok team-credit usage fetch (`GET https://cli-chat-proxy.grok.com/v1/billing`), correctly labeled `"Team Credits (Monthly)"`/`"Team Credits (Weekly)"`.
- Live comparison against the actual `grok` interactive TUI on the same account (subscription tier `SuperGrok`) showed a **second, distinct metric**: the CLI prints `"Weekly limit: 1%"` / `"Next reset: July 16, 00:31 PT"` after a turn — a personal, subscription-included weekly allowance, separate from the team credit pool.
- **The exact source was found** by running a real interactive `grok` session (via a FIFO-fed `--no-alt-screen --minimal` process, no true TTY needed) under `--debug --debug-file` and reading the captured trace directly — no MITM proxy was needed after all. The response, captured live:
  ```json
  {"config":{"creditUsagePercent":1.0,
    "currentPeriod":{"type":"USAGE_PERIOD_TYPE_WEEKLY",
      "start":"2026-07-09T07:31:03.807421+00:00","end":"2026-07-16T07:31:03.807421+00:00"},
    "onDemandCap":{"val":0},"onDemandUsed":{"val":0},"prepaidBalance":{"val":0},
    "isUnifiedBillingUser":true,"billingPeriodStart":"...","billingPeriodEnd":"..."},
   "subscription_tier":"SuperGrok"}
  ```
  `end: 2026-07-16T07:31:03+00:00` = July 16, 00:31 PT — exact match to the CLI's own display. `creditUsagePercent` is already a 0–100 percentage (1.0 = "1%"), confirmed by comparing against the CLI's own rendered value at the same moment.
- **This is delivered via an ACP extension method, `x.ai/billing`** (a raw JSON-RPC call `{"jsonrpc":"2.0","id":N,"method":"x.ai/billing","params":{}}`), sent immediately after `initialize` — no `authenticate`/`session/new` round trip needed first.
- **Attempted implementation and reverted (2026-07-10):** wired a Go fetch (`grok_weekly_quota.go`) that spawned a short-lived `grok agent stdio` process and sent exactly this `initialize` → `x.ai/billing` sequence. It failed live: `grok agent stdio` (the plain external ACP surface FlowPilot's own `grokAdapter`/`grokDispatcher` already speak) returned `{"error":{"code":-32601,"message":"Method not found"}}` for `x.ai/billing`. **`x.ai/billing` only exists inside the full interactive TUI's own internal client↔agent split (the "grok-pager" frontend talking to its own `mvp_agent` backend in-process) — it is not registered on the external `agent stdio` surface at all.** This is a materially different, more sobering finding than the original "just need a MITM capture" framing: there is no known supported way to reach this data from outside the full TUI process. All added code (`grok_weekly_quota.go`, its test, and the `loadGrokAccountMetadata` wiring) was removed; `go test ./internal/cli/...` confirmed back to the clean Task-216 baseline (10 tests) before/after.
- Reaching this data would require either driving the actual full-TUI binary (fragile: parsing raw ANSI-rendered terminal output, no stable contract, breaks on any TUI update) or xAI exposing `x.ai/billing` (or an equivalent) on the external ACP surface in a future Grok Build release. Neither is something to build against today.

### Current Ask

- None actionable right now. This task documents a confirmed dead end for the *mechanism*, not just an unknown value — do not re-attempt the `grok agent stdio` + `x.ai/billing` approach; it has been proven not to work, not just "not yet tried."
- If a future `grok` release adds `x.ai/billing` (or an equivalent) to the documented external ACP method set, or xAI publishes a REST endpoint for it, revisit then.

### Key Decisions

- `T-1` Confirmed: `x.ai/billing` exists and returns the exact right data, but only inside the full interactive TUI process — the external `agent stdio` ACP surface (the only one FlowPilot is designed to drive) returns `-32601 Method not found` for it.
- `T-2` Do not attempt to drive the full interactive TUI (ANSI-parsing its rendered screen) to scrape this value — no stable contract, would break on any Grok Build UI change, and is a fundamentally different (and much more fragile) integration shape than every other provider adapter in this codebase.
- `T-3` Keep `Task-216`'s `"Team Credits (Monthly/Weekly)"` fetch and labels untouched — this task's non-outcome does not affect it.

### Constraints

- Do not reintroduce a `grok agent stdio` call to `x.ai/billing` — it is confirmed non-functional (`-32601`), not merely unverified.
- Do not implement TUI screen-scraping as a data source for account metadata.

### Open Questions

- `Q-1` Does xAI plan to expose `x.ai/billing` (or an equivalent weekly-quota method) on the documented external ACP surface in a future Grok Build release? No public roadmap known; nothing to track proactively beyond re-checking on future `grok` version bumps (`CompatTestedGrokVersion` in `compat.go` already tracks the tested baseline — a version bump review could re-probe this).

### Source Refs

- `Task-216` (prior art: team-credit fetch, endpoint-discovery method); `CP-46 Q-5`.

## 1. Goal

Determine whether the SuperGrok personal weekly included-usage allowance (the metric the native `grok` CLI prints as `"Weekly limit: X%"` / `"Next reset: ..."`) can be surfaced on FlowPilot's Grok account card alongside the team-credit line `Task-216` already ships.

## 2. Parent Links

- coding plan: [CP-46: Grok Build Controlled Adapter Over ACP Transport](../../07-Coding-Plan/inprogress/CP-46-Grok-Build-Controlled-Adapter-Over-ACP.md)
- tech design: [SD-06: AI Provider Integration](../../06-System-Tech-Design/SD-06-AI-Provider-Integration.md)
- system spec: [SS-05: Workflow AI Provider](../../05-System-Specs/SS-05-Workflow-Ai-Provider.md)
- specific upstream ids: `Task-216` (all)

## 3. Trigger

After `Task-216` shipped the team-credit fetch, the user compared FlowPilot's card against the real `grok` CLI's own status line and found a second, unaccounted-for metric — a weekly limit resetting on a different date than the credit pool.

## 4. Exact Change

- `T-1` Ran a real interactive `grok` session (FIFO-fed stdin, `--no-alt-screen --minimal`, no true TTY required) under `--debug --debug-file` and located the exact `x.ai/billing` ACP extension method call and its weekly-cycle response shape in the captured trace.
- `T-2` Implemented `loadGrokWeeklyQuota`/`grokWeeklyUsageLine` in a new `apps/local-runner/internal/cli/grok_weekly_quota.go`: spawned `grok agent stdio`, sent `initialize` then `x.ai/billing`, mapped the response. Added unit tests for the pure mapping function (`grok_weekly_quota_test.go`) and wired it into `loadGrokAccountMetadata`.
- `T-3` Verified end-to-end against the real connected account and got `{"error":{"code":-32601,"message":"Method not found"}}` — `agent stdio` does not implement this extension method.
- `T-4` Reverted all of `T-2`'s code (deleted `grok_weekly_quota.go` and its test, removed the `loadGrokAccountMetadata` wiring) rather than ship dead code that silently always returns nil in production. Re-ran `go build ./internal/cli/...`, `go vet`, and `go test ./internal/cli/...` — confirmed clean return to the `Task-216` baseline (10 tests passing, no Grok weekly-related tests remaining).

## 5. Touched Areas

- files: none remaining (all changes in this task were added then reverted in the same session — see `T-4`)
- modules: provider-accounts metadata enrichment (Go CLI) — investigated only, not changed
- routes: none
- tables: none

## 6. Acceptance Check

- `go build ./internal/cli/...`, `go vet ./internal/cli/...`, `go test ./internal/cli/...` all clean, matching the pre-`Task-217` (`Task-216`-only) baseline exactly.
- The `x.ai/billing` mechanism and its `-32601` external-surface rejection are documented precisely enough that no future task re-attempts the same dead end without new information (e.g., a `grok` version bump that changes this).

## 7. Out of Scope

- Any TUI screen-scraping approach to reach this value.
- Contacting xAI or filing an external feature request for a public quota API (not something FlowPilot's codebase can act on).
- Changing `Task-216`'s team-credit fetch, labels, or tests.

## 8. Completion Notes

- result: Investigated and confirmed non-implementable via the supported integration surface. The `x.ai/billing` ACP extension method was located and its response shape fully captured and verified against the live account, but it returns `Method not found` on the external `grok agent stdio` process FlowPilot uses — it is internal to the full interactive TUI only. A working implementation was built, live-tested, found non-functional, and cleanly reverted in the same session (no dead/half-shipped code left behind).
- follow-ups: Re-check on a future `grok` CLI version bump (tracked via `compat.go`'s `CompatTestedGrokVersion`) in case xAI adds this method to the external ACP surface. No other actionable path currently exists.
- upstream docs updated: none — `Task-216`'s shipped team-credit fetch is unaffected and remains the only real Grok quota data FlowPilot surfaces.
