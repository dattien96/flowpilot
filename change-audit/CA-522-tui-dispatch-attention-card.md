---
id: CA-522
feature_key: cli-tui
title: TUI dispatch operator attention surface (CP-51 Task-256) — chips above composer
date: 2026-08-15
status: COMPLETE
---

## Problem

CP-51 Task-256 shipped the runner operator-resolution surface (uncertain turns /
open repairs needing an operator decision) with REST routes
(`dispatch_operator.go`) and a Desktop `DispatchAttentionCard`, but the TUI had
**zero** handling: a run with unresolved dispatch attention offered no way to
inspect, resolve, or repair it, and a new turn could be sent against a flow the
runner was keeping blocked on an operator decision.

## Root cause (TUI-only, not a runner regression)

The runner routes and durable dispatch store already exist (Task-256). The gap
was purely on the TUI client + app surface:

- No TUI client methods for the 5 Task-256 endpoints.
- No app state / rendering / click handling to surface `uncertain` /
  `repair_required` attention (Desktop `DispatchAttentionCard` parity).
- `sendBlocked()` / `turnIsActive()` had no awareness of unresolved attention,
  so the TUI could arm `[stop]` / send a new turn against a dispatch-blocked run.
- No hydrate-on-open, so reopening a flow with pending attention never surfaced it.

## Fix (TUI-only)

**A. Client (`tui/client/client.go`)**
- DTOs `DispatchAttentionItem`, `DispatchSettlement`, `DispatchInspectResult`,
  `ReceiptEvidenceSummary`, `TerminalEvidenceSummary`, `OpenRepairSummary`,
  `RetryAsNewInput` (mirror `contract.ts` + `HttpWsRunnerClient.ts`).
- `ListDispatchAttention(ctx, runID)` → `GET .../dispatch-attention`.
- `InspectDispatch(ctx, runID, turnID)` → `GET .../dispatches/{turnId}`.
- `ResolveDispatchUncertain(ctx, runID, turnID, rev, resolutionID, action, detail)`
  → `POST .../dispatches/{turnId}/resolve` (OR ledger row: response carries the
  already-committed settlement disposition, no follow-up read).
- `RetryDispatchAsNew(ctx, runID, turnID, input)` → `POST .../retry-as-new`.
- `ResolveDispatchRepair(ctx, runID, expectedRepairRev, resolutionID, action)`
  → `POST .../repair-resolution`.

**B. App (`app/attention.go` new, `app/app.go`, `app/mouse.go`, `app/model.go`)**
- `attention []DispatchAttentionItem` + `attentionInspect` cache +
  `attentionInFlight` + `attentionErr` + `attentionRetryConfirm` (cancel-bias).
- `renderAttentionBar()` — Desktop `DispatchAttentionCard` parity rendered above
  the composer in the input bar. `settle_pending` → passive `[details]` only;
  `uncertain`/`cancel_required` → `[inspect] [confirm-cancelled] [mark-completed]
  [mark-failed] [retry-as-new] [abandon]`; `repair_required` → `[inspect]
  [retry-load] [abandon-repair]`.
- `hitAttentionChip()` — x/y-aware click hit against the actual rendered input
  row (mirrors `hitApprovalChrome`), wired into `clickTargetAt`.
- `dispatchMouseClick` handles attention chips → `cmdInspectAttention`,
  `cmdResolveAttention`, `cmdRetryAttention` (T-5 cancel-bias: first click arms
  `[confirm-retry]`, second click actually retries), `cmdResolveRepair`.
- `hasUnresolvedAttention()` — true only for `uncertain`/`repair_required`/
  `cancel_required` (NOT passive `settle_pending`). Drives `turnIsActive()` false
  (no `[stop]`) and `sendBlocked()` true (no new turn) → the run is parked, not
  live-running.
- `applyAttention()` / `showAttentionBanner()` — store + warn banner
  "Dispatch attention: N item(s) need an operator decision".
- `cmdHydrateDispatchAttention()` — one-shot fetch on `/open` of a flow run
  (Desktop poll parity), so reopening surfaces pending attention.
- `AttentionLoadedMsg` / `AttentionInspectedMsg` / `AttentionResolvedMsg`
  handlers with a stale-run guard; resolve/retry/repair success schedules an
  attention refresh to drop the resolved item (committed-settlement parity).

## Provider impact

Provider-agnostic. Attention is run/record-level, not provider-adapter logic.
Every decision test parameterizes Claude / Codex / Grok (cross-provider-parity
Case 1 + parameterized guard).

## Tests

New additive file `app/tui_dispatch_attention_test.go` (mirrors the 7 runner
`dispatch_operator_test.go` §4.2 tests at the TUI surface):

- Client wire: list → resolve → attention clears (replay-safe), and resolve
  response carries the committed settlement disposition.
- `InspectDispatch` surfaces only canonical hash, never raw payload (redaction).
- Uncertain item → chips + parked (no `[stop]`, send blocked) × 3 providers.
- `settle_pending` → `[details]` only, NOT parked, NOT send-blocked.
- Resolve (mark_completed) drops the item after refresh × 3 providers.
- Retry-as-new superseded → 409 `dispatch_retry_superseded` error surfaces
  (never clears), with cancel-bias double-confirm.
- Repair abandon + retry_load resolve.
- Hydrate-on-open rebuilds attention after "restart" × 3 providers.
- Stale-run attention ignored (different run).

## Contract matrix

| Surface | Coverage |
|---|---|
| Uncertain resolution (Desktop card actions) | `TestAttentionResolve_DropsItem`, `TestAttentionBanner_ChipsAndNoStop` |
| Repair-required (retry-load / abandon) | `TestAttentionRepair_AbandonAndRetryLoad` |
| settle_pending passive (no decision) | `TestAttentionSettlePending_OnlyDetails` |
| Retry-as-new superseded guidance | `TestAttentionRetry_Superseded409` |
| Attention survives restart (hydrate on open) | `TestAttentionHydrate_OnOpen` |
| Stale-run guard | `TestAttention_NonAttentionRunIgnored` |
| Evidence redaction | `TestDispatchInspect_RedactsEvidence` |
| Providers | claude/codex/grok parameterized |

## Verification

- `go build ./...` clean; `go vet ./internal/tui/... ./internal/cli/...` clean.
- `go test ./internal/tui/... ./internal/cli/... -count=1` all `ok` (legacy
  untouched and green).
- New tests + app/client race-clean: `go test ./internal/tui/app ./internal/tui/client -race`.

## Out of scope / residual

- Runner routes / durable dispatch store unchanged (Task-256 already shipped).
- No new slash commands — the surface is clickable chips in the input bar
  (Desktop `DispatchAttentionCard` parity), matching how the TUI already
  surfaces approval/question chips.
- Audit GET route (`/dispatches/{turnId}/audit`) exists on the runner but is not
  wired to a TUI command (no dedicated audit view); `[inspect]` covers the
  decision-critical fields.
- Pre-existing unrelated suites (`internal/runner` flaky, `internal/structure`,
  `internal/changecontract` platform path-separator on darwin) unchanged.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: feature
summary: The TUI now surfaces the CP-51 Task-256 dispatch operator attention surface as clickable chips above the composer (Desktop DispatchAttentionCard parity): uncertain turns offer inspect/confirm-cancelled/mark-completed/mark-failed/retry-as-new/abandon, repair_required offers retry-load/abandon-repair, settle_pending is passive [details]-only; unresolved attention parks the run (no [stop], send blocked) and hydrate-on-open rebuilds it after restart, with committed-settlement refresh and a stale-run guard (Task-256)
# --->8---
