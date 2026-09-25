# CA-981 — Task-449: Flow/Vibe quota gate & bounded auto-rotation

## Summary

The quota gate is now live: one resolver fronts provider admission and
limit-driven re-entry for Flow and Vibe alike. Manual mode (the default)
parks the run on a durable `quota_route_required` decision card — no
provider call happens without a committed choice. Auto mode rotates only
exact/high-confidence candidates; unknown, stale, or ambiguous quota
evidence falls back to the same gate. Every committed rotation produces
exactly one new leg (same-provider via the Task-447 claim ledger,
cross-provider via `switchChatLeg` handoff), never a live-session or
global active-account mutation, and never mid-turn.

## What changed

- `runner/quota_gate.go` (new) — `QuotaPreflightOutcome`
  (`proceed|gate|rotate|blocked`), `QuotaResolution`, and the three spec
  methods `ResolveQuotaPreflight`, `CommitQuotaResolution`,
  `ResumeQuotaGate`. Resolution order: resolve `ExecutionDemand` →
  same-provider candidates → cross-provider candidates → outcome.
  Commit path enforces candidate freshness, claims same-provider pins,
  mints new legs for provider switches, and records structured
  `quota_route_committed` / `quota_route_stopped` events. Gate options
  are `use_once` / `use_for_run` / `stop` (run-scope claims are durably
  scoped to the run ID).
- `runner/quota_preflight.go` — `ExecutionDemand.ObservedLimit` carries
  the just-observed `ProviderLimitKind` on gate entry; it hard-rejects
  the current binding for *this* decision without writing a durable
  block (`quota_exhausted` self-heals on window reset). `accountBlocked`
  → `accountBlockReason` so the true ledger reason surfaces in candidate
  rejection reasons.
- `runner/interactive_service.go` — `usageRouter = s` (activates the
  CP-86 `usage_budget_exceeded` rotate option and context-pressure
  escalation via `RotateUsageBudgetRun`); `AnswerQuestion` routes
  `quota_route_required` cards (kind recovered from the persisted prompt
  prefix so rehydrated cards still route correctly after restart);
  `finishTurn`'s provider-limit branch feeds the live limit kind into
  the gate.
- `runner/chat_switch.go` — `chatSwitchRequest.ProviderAccountID` lets a
  cross-provider rotation pin the target account on the new leg.
- `runner/provider_event.go` — `ProviderEvent.QuotaRoute` payload field
  for the structured route-commit/stop notices.
- `runner/task449_quota_gate_test.go` (new) — all thirteen §7 tests.

## Decisions

- `extend` on `usage_budget_exceeded` is filtered out of automatic
  selection unconditionally — auto mode may rotate, never extend the
  user's budget.
- Flow-child rotation reuses the close/respawn seam rather than a
  mid-turn provider swap; the new child leg carries the pinned triple.
- Gate-card kind is recovered from the persisted prompt prefix on
  rehydration — durable questions don't store `kind`, and this keeps
  restart-restore honest without schema churn.
- `already_tried` ledger marks apply to switch *targets*, so a replayed
  gate pass after a committed rotation produces no duplicate rotation
  (the now-current account is rejected via `ObservedLimit`, not via
  already-tried).

## Verification

- `go test -count=1 -run 'TestTask449_' ./internal/runner/` — 13/13
  green (manual Flow+Vibe gate parity, auto exact-candidate rotation,
  unknown-quota gating, new-leg creation, no-selection/no-provider-call,
  restart restores pending gate, restart does not replay committed
  rotation, structured stop, pressure headroom-fail escalation,
  post-compaction rotate option, extend-never-auto).
- Adjacent slice (`TestTask44[3-9]`, switch/pressure/budget/answer/chat)
  — green except one TempDir `RemoveAll` cleanup race
  (`TestSwitchInitializesTranscriptWriterBeforeHandoff`) that passes
  standalone; documented env flake, assertions passed.
- **Cross-provider parity** (per `.agents/skills/cross-provider-parity`):
  Case 1 — provider-agnostic. `quota_gate.go`/`quota_preflight.go`/
  `quota_candidates.go` take `providerKey`/`accountID` as data and never
  branch on a provider constant; candidate enumeration is
  registry-driven. Per-adapter pin honoring was verified at the
  Task-447 layer (`newAdapterForAccount` seam across all six live
  factories). Task-449 tests exercise claude→codex/grok cross-provider
  fixtures.
- **Durable replay** (per `.agents/skills/durable-replay-contracts`):
  interactive-state row — quota cards are durable question records,
  restart rehydration proven by `TestTask449_RestartRestoresPendingGate`;
  terminal/settlement row — committed rotations are not replayed
  (`TestTask449_RestartDoesNotReplayCommittedRotation`); handoff
  transcript seeding reuses the existing `switchChatLeg` machinery
  (unchanged contract). No claim of exactly-once; commit is CAS/ledger
  ordered before any provider call.
- detect_changes: GitNexus CLI exposes no `detect_changes` (MCP-only,
  not configured here); equivalent staged-diff scope review performed.

## Follow-ups

- Task-450: Desktop/TUI quota settings screen, candidate table UI
  (rejection reasons are already machine-readable), audit surface.
