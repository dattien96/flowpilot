# BUG-409: `ResolveBuiltin` propagates Supabase store errors instead of embedded-pack fallback — built-in flowRefs silently degrade to plain chat

## Metadata

- Document ID: `BUG-409`
- Title: `Configured-but-unreachable Supabase flow store takes down ALL built-in flowRef resolution — silent fallback to ungated chat turns`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-21`
- Last Updated: `2026-09-21`
- Parent Documents: [CP-51-Durable-Turn-Dispatch-State-Machine-And-Recovery-Reconciliation](../../07-Coding-Plan/done/CP-51-Durable-Turn-Dispatch-State-Machine-And-Recovery-Reconciliation.md), [CP-54-Test-Steps](../../07-Coding-Plan/done/CP-54-Test-Steps.md), [CP-49-Test-Steps](../../07-Coding-Plan/done/CP-49-Test-Steps.md)
- Feature Keys: `flow-resolver`, `flow-engine`, `supabase-mirror`

## AI Quick View

### Summary

- `ResolveBuiltin` (`flow_definition_resolver.go` ~L162) propagates `store.GetByPackFlow` **errors** instead of falling back to the embedded pack — contradicting its own doc comment ("falling back to the embedded pack when the store has no row yet **or is unavailable**").
- With a stale `~/.flowpilot/settings/supabase-config.json` pointing at unresolvable `demo-ref.supabase.co`, `FlowDefinitionStoreFor` returns a non-nil store whose every call errors → **every** built-in flow start silently degrades to a plain chat turn (`[chat-flow-ref] flowRef … failed to resolve; falling back to a normal chat turn`). Confirmed 3× live: CP51-004 (`bug-harness` run-13), CP54-2 (`context-coding-review-synthesis` run-9/turn-11), CP49-1 (`vibe-cp-ingest` run-4 — devin wrote `textutil/*.go` ungated, bed contamination captured + reverted).

### Current Ask

- Captured during cp_live_test live-verification wave; awaiting prioritization.

## Bug report

- **Symptom:** `POST /client/workflow-runs` with a built-in `flowRef` logs `[chat-flow-ref] flowRef "…" for run "…" failed to resolve; falling back to a normal chat turn`; the flow engine never engages; the model emulates the flow inline or just chats — no error surfaced to the caller.
- **Expected:** per the function's own contract, a store error ("unavailable") falls back to the embedded pack — built-in flows keep working offline/degraded.
- **Actual:** store errors abort resolution; every built-in flowRef (harnesses, vibe flows, review loops) becomes an ungated plain chat turn — no SS-lock, no contract, no gate discipline.
- **Impact:** a single stale/broken Supabase config silently disables the entire flow layer; agents then write production code outside any contract/gate (run-4 wrote `textutil/*.go`, `*_test.go`, Task/CA/req docs with zero gate evaluation — captured in `L49-2-run4-contamination.diff`). Silent degradation makes the failure invisible to users.

## Reproduction

1. Configure `~/.flowpilot/settings/supabase-config.json` with an unreachable ref (e.g. `demo-ref.supabase.co` — DNS fails).
2. `POST /client/workflow-runs` with `flowRef:"bug-harness"` (bare or canonical `flowpilot-core-flow-pack/…`).
3. Observe the `[chat-flow-ref] … failed to resolve; falling back to a normal chat turn` log line and a plain turn result.
4. Workaround used live (env-only): write `{}` to `<worktree>/.flowpilot/settings/supabase-config.json` → nil store → embedded pack resolves (bug-harness then spawned real children run-860→run-865 etc.).

## Root cause

- `apps/local-runner/internal/runner/flow_definition_resolver.go` `ResolveBuiltin` (~L162): `if record, ok, err := r.store.GetByPackFlow(ctx, packID, flowID); err != nil { return …, err }` — the error path returns instead of falling through to the embedded-pack lookup, contradicting the doc comment at ~L158-160 ("…or is unavailable"). With a configured-but-dead Supabase mirror, `FlowDefinitionStoreFor` yields a non-nil store and every built-in resolution errors out.

## Evidence

- `~/fp-beds/lt-evidence/cp51/RESULT.md` (BUG-LIVE-CP51-004) + `runner.log` (`[chat-flow-ref]` lines for run-13).
- `~/fp-beds/lt-evidence/cp54/RESULT.md` (BUG-LIVE-2, CONFIRMED) — startup `[flow-mirror-sync]` dial failures, `flowRef "context-coding-review-synthesis" for run "run-9" failed to resolve` (old `runner.log` ~1037–1038, 1561); run-9 → plain `turn-11`.
- `~/fp-beds/lt-evidence/cp49/RESULT.md` (BUG-LIVE-1) — `vibe-cp-ingest` run-4 fell back to chat; `L49-2-run4-contamination.diff` / `L49-2-run4-contamination-status.txt` (bed contamination, reverted).

## Severity

`high` — one bad config file silently disables all built-in flows and lets agents write ungated production code; confirmed in three independent live sessions.

## Completion Notes (implemented 2026-09-23, CA-922b)

- Fix: `ResolveBuiltin` logs + falls through to the embedded pack on `GetByPackFlow` error (its documented contract); `ResolveFlowRef` remembers the `GetByRef` error and still attempts builtin resolution, re-surfacing the store error only for refs that aren't built-in (fail closed, never silent chat fallback).
- Files: `internal/runner/flow_definition_resolver.go`.
- Tests: `TestBug409_ResolveBuiltinFallsBackOnStoreError`, `TestBug409_ResolveBareRefFallsBackOnStoreError`, `TestBug409_NonBuiltinRefStillErrorsOnDeadStore` (converse). Live-verified: embedded pack resolved `vibe-cp-ingest` during the BUG-399 live session.
