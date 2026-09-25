# Task-440: ModelContextWindow Catalog Fallback at emitLocked

- Document ID: `Task-440`
- Title: `Populate TokenUsageSnapshot.ModelContextWindow from the provider model catalog when the adapter does not self-report`
- Phase: `task`
- Status: `done`
- Owner: `dat.nguyen`
- Reviewers: ``
- Created: `2026-09-25`
- Last Updated: `2026-09-25`
- Parent Documents: `CP-86`, `SD-10`, `SS-22`
- Child Documents: ``
- Related Documents: `CP-23 Task-334`, `provider_event.go`, `interactive_service.go emitLocked`
- Replaces: ``
- Tags: `token-usage`, `provider-parity`, `claude`, `context-window`

## AI Quick View

### Summary

- Claude emits `EventTokenUsageUpdated` with `Last.TotalTokens` but never a
  `ModelContextWindow` (Claude CLI does not send one — the window is a model
  catalog constant). Grok/Devin/Opencode/Codex self-report.
- The runner already serves per-model `ContextWindowTokens` in its provider
  catalog (the same data the TUI resolves via `contextWindowForModel`) but
  never writes it into usage snapshots — so every consumer must re-implement
  the fallback.

### Current Ask

- Enrich once at the single event-ingest seam (`emitLocked`): when a
  `token_usage_updated` event arrives with `ModelContextWindow == nil`,
  resolve it from the runner's provider model catalog via
  `rs.providerKey` + `rs.modelName`. Self-reported values are never
  overridden; unknown → stays nil (deterministic degradation).

### Key Decisions

- `T-1` Enrich at ingest (`emitLocked`), not at each consumer — one seam,
  every downstream reader (TUI, drift, Task-443 pressure) sees a uniform
  snapshot.
- `T-2` Provider-reported window always wins over catalog — catalog is the
  fallback, never an override.
- `T-3` Lookup must be pure in-memory — `emitLocked` runs under `s.mu`; no
  I/O, no network, no process spawn inside the lock.

### Constraints

- Fake adapters + fake catalog only — no live Claude on the dev machine.
- Gemini excluded (no usage events at all — CP-86 documented exclusion).
- Do not change any adapter's emitted payload — enrichment happens in the
  service, after emission.

### Open Questions

- Exact catalog accessor for the lookup (registry vs catalog store) — pick
  whichever already serves `/client/providers` so the number matches what
  the TUI shows.

### Source Refs

- `CP-86 P-1`, `provider_event.go` (`TokenUsageSnapshot`),
  `interactive_service.go` (`emitLocked`, `interactiveRun.providerKey`,
  `interactiveRun.modelName`), `runner.go` (`ProviderModel.ContextWindowTokens`),
  `tui/app/helpers.go` (`contextWindowForModel` — same lookup semantics).

## 1. Goal

Every `token_usage_updated` event carries a `ModelContextWindow` whenever
the runner can resolve one — so pressure detection, TUI display, and drift
telemetry never see a missing window for Claude (or any future
non-self-reporting provider).

## 2. Parent Links

- coding plan: `CP-86` (P-1)
- tech design: `SD-10`
- system spec: `SS-22`
- specific upstream ids: `CP-23 Task-334`, `AGENTS §5 provider parity`

## 3. Trigger

Context-pressure work (Task-443) needs `ModelContextWindow` to compute
`usage/window`. Claude — a primary provider — leaves it nil today, so
pressure logic would silently never fire for Claude runs.

## 4. Exact Change

- `T-1` Add `contextWindowForRun(rs)` resolver: catalog lookup by
  `rs.providerKey` + `rs.modelName`, returns `int64` (0 = unknown).
- `T-2` In `emitLocked`, before `rs.events` append: if
  `ev.Type == EventTokenUsageUpdated && ev.TokenUsage != nil &&
  ev.TokenUsage.ModelContextWindow == nil` and resolver returns > 0, set it.
- `T-3` Log once per run on first successful enrichment
  (`[usage] context window resolved from catalog run=… model=… window=…`).

## 5. Touched Areas

- files: `internal/runner/interactive_service.go` (emitLocked + resolver),
  possibly `internal/runner/context_usage.go` (new home for resolver)
- modules: `runner`
- routes: none
- tables: none

## 6. Code Guide Signatures

```go
// internal/runner/context_usage.go (new file — usage accounting helpers)
// contextWindowForRun resolves the model's context window from the runner's
// provider catalog. Returns 0 when unknown (T-1).
func (s *InteractiveService) contextWindowForRun(rs *interactiveRun) int64

// enrichTokenUsage fills snap.ModelContextWindow from the catalog when nil.
// Never overrides a self-reported value. Pure in-memory (T-2/T-3).
func (s *InteractiveService) enrichTokenUsage(rs *interactiveRun, snap *TokenUsageSnapshot)
```

```go
// internal/runner/interactive_service.go — inside emitLocked, before rs.events append:
if ev.Type == EventTokenUsageUpdated && ev.TokenUsage != nil {
    s.enrichTokenUsage(rs, ev.TokenUsage) // T-2 — unchanged signature of emitLocked
}
```

## 7. Test Signatures

- `TestTask440_UsageEventWithoutWindow_EnrichedFromCatalog` — fake adapter
  emits usage without window; catalog knows model → recorded event carries
  catalog window (covers AC: enrichment)
- `TestTask440_SelfReportedWindow_NeverOverridden` — event arrives with
  window=500000, catalog says 200000 → stays 500000 (covers AC: no override)
- `TestTask440_UnknownModel_WindowStaysNil` — modelName absent from catalog
  → window remains nil, no error (covers AC: degrade-soft)
- `TestTask440_EnrichmentIsPureInMemory` — catalog fake that panics on I/O
  paths → enrichment still succeeds (covers T-3)
- `TestTask440_ClaudeCodexGrok_Parity` — table-driven: claude (no self-report
  → catalog), codex (self-report), grok (self-report) → all events carry
  window post-emit (covers AGENTS §5)

## 8. Acceptance Check

- A Claude run's recorded `token_usage_updated` events show
  `modelContextWindow` populated in the persisted ndjson and on the SSE
  stream — without any adapter change.
- TUI `formatContextLimits` shows the same number the event carries
  (single source).

## 9. Out of Scope

- Gemini adapter usage emission (none today).
- Pressure thresholds / compaction detection (Task-443).
- Changing what adapters emit or how mappers parse provider payloads.

## 10. Definition of Done

- [x] All §6 signatures implemented exactly (or deviation documented in §11)
- [x] All §7 tests exist, green, additive-only (no pre-existing test edited)
- [x] Related pre-existing tests still green — any old failure → STOP and report (safe-fix-contract R1)
- [x] Provider parity proven or evidenced where the change touches shared/provider paths (R2)
- [x] `feature_key` set; CA ledger entry written; FEATURE-KEYS.md already contains the key
- [x] §8 acceptance checks verified by hand or test
- [x] GitNexus `detect_changes` shows only expected symbols before commit

## 11. Completion Notes

- result: token_usage_updated events get ModelContextWindow filled from the provider catalog (modelContextWindowFor) when the adapter did not self-report; pure in-memory inside emitLocked, self-reported values never overridden, unknown models stay nil. Task tests green incl. Claude/Codex/Grok parity. Two pre-existing suite failures (provider count 4-vs-6, env-dependent catalog fallback) reproduce on clean HEAD worktree — unrelated, recorded.
- live-found bug (fixed, CA-975): Devin reports the window on a mid-turn usage event but omits it on the turn-terminal event → added legContextWindows[providerSessionID] carry-forward (self-report → catalog → same-leg remembered; new leg starts empty). Regression: TestTask440_LegWindowCarryForward. Verified live: terminal token_usage_updated now carries modelContextWindow=262000.
- follow-ups: none — feeds Task-443 ratios and Task-444 labels
- upstream docs updated: CP-86, CP-86-Test-Steps
