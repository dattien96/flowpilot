# CA-931 — Realtime multi-lane run updates: mux SSE + decision payloads (CP-84 Task-429/430)

## Summary

Runs across all projects previously surfaced waiting state only via the 30s
history poll of the selected project. This adds:

- **Runner `GET /client/events/stream`** — a level-triggered multiplexed SSE
  lane stream. Frames: `snapshot` (chunked, staged under `snapshotId`,
  reconciled only on `complete`), `upsert`, `remove`, `resync`. Per-subscriber
  dirty-set drain coalesces bursts; overflow → `resync` (drop-and-refetch,
  never silent loss). Terminal runs carrying actionable worktree-merge
  decisions stay in the lane set.
- **`DecisionPayload` (Task-430)** — durable, bounded, redacted decision
  projection per lane: `id` (stable across restarts), `revision` (opaque
  resubmit token → 409 on stale), `kind` ∈ approval/question/gate/ss_lock/
  worktree_merge/dispatch_attention/r_requirement/quota, plus per-kind payload
  blocks. Projected from durable records (approval/question shards, gate-block
  events, ss-lock gates, worktree binding, dispatch store) — not from
  resequenced event seqs.
- **Desktop `streamRunUpdates`** — app-lifetime consumer with backoff+jitter
  reconnect; mux lanes live in a dedicated map inside `attentionQueue` so a
  reconnect snapshot drops only mux-owned lanes while poll history stays.
  Mux status freshens stale polled rows; synthetic history rows surface lanes
  not yet polled.
- **Telemetry**: connect/disconnect/subscriber-count only — never payload.

## Verified

- 15 new runner tests green (`decision_payload_test.go`), `-race` clean.
- 7 new desktop tests green (`runUpdates.test.ts`).
- Baseline diff on clean HEAD worktree: all 27 Go-suite failures and all
  phase1 failures reproduce identically at baseline → zero regressions.

## Files

- `apps/local-runner/internal/runner/decision_payload.go` (+ `_test.go`),
  `interactive_handlers.go`, `dispatch_live.go`, `dispatch_record.go`,
  `dispatch_store_memory.go`, `flow_step_runtime.go`, `gate_hook.go`,
  `run_worktree_merge.go`, `ss_lock_gate.go`, `interactive_service.go`
- `apps/desktop-flowpilot/src/types/contract.ts`,
  `client/HttpWsRunnerClient.ts`, `client/MockRunnerClient.ts`,
  `state/attentionQueue.ts`, `state/store.ts`, `state/runUpdates.test.ts`

# ---8<--- flowpilot:change-ledger
feature_key: event-plane
source_doc_id: CP-84
change_type: feature
summary: level-triggered multiplexed run-events SSE stream with durable DecisionPayload projection and desktop mux-lane reconciliation
# --->8---
