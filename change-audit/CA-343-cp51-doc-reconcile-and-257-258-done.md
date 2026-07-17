# CA-343: CP-51 doc reconcile (Codex review) + move Task-257/258 to done

## Scope

Apply the corrections from the Codex review of the 2026-07-17 CP-51 status audit, and move the two genuinely-complete tasks to `done/`. Documentation only — no `apps/` code changed in this entry.

## Changes

- Moved `Task-257-Capability-Evidence-Spike.md` and `Task-258-Per-Project-Dispatch-Log-And-Drive-Sync.md` from `08-Task/inprogress/` to `08-Task/done/` (git mv).
- Fixed stale parent links in all 11 CP-51 tasks: `07-Coding-Plan/todo/CP-51…` → `07-Coding-Plan/inprogress/CP-51…` (CP-51 lives in `inprogress/`).
- Task-249: metadata status line reconciled with §8 (was "live Stop path NOT wired", contradicting the Completion Notes); recorded the **Codex-found fail-closed gap** — `requestRunStopV2` returns void and swallows store errors (`dispatch_live.go:295`), so a durable-store failure still lets the Stop API RAM-cancel and report success (INV-3 holds only on happy path).
- Task-250: recorded the two **Codex-found deeper gaps** — (1) `ScanAllRecoverable` enumerates via `ListAttention` which only surfaces `uncertain` records, so prepared/send_claimed/send_started are invisible to boot recovery even with a boot caller; (2) prepared/send_claimed branch claims then `return nil` with no redispatch. "Just add a boot caller" is insufficient.
- Task-255: corrected count 10/12 → 9/12 named suites absent (`stop_race_barrier_test.go` now exists from P1).

## Codex corrections accepted (verified against code)

- `requestRunStopV2` void/error-swallowing — confirmed (`dispatch_live.go:295`).
- `ScanAllRecoverable`→`ListAttention` only-uncertain — confirmed (`dispatch_recovery.go:85`, `dispatch_store_memory.go:1291`); prepared/claimed `return nil` no-redispatch (`dispatch_recovery.go:63`).
- `multiProjectDispatchStore` has NO `Close()` while `localDispatchStore.Close()` exists — confirmed (`dispatch_store_open.go:59` holds `byProject map[string]*localDispatchStore`); this is the real root cause of the Windows `dispatch.lock` TempDir-cleanup failures (not "flock never closed").
- CP-51/Task-249/250/255 still require Supabase/real-PG while Task-258 dropped the Supabase DispatchStore — contract reconcile (SD-24 → CP-51 → tasks) is a prerequisite before P2.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CP-51
change_type: docs
summary: Reconcile CP-51 task docs per Codex review (fail-closed Stop gap, recovery enumeration gap, 255 count, parent links); move Task-257/258 to done
# --->8---
