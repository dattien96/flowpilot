# BUG-406: `dispatch.ndjson` seq collision + gap on every terminal commit

## Metadata

- Document ID: `BUG-406`
- Title: `commitTerminal writes record with Seq: s.seq before appendAuditLocked increments → duplicate seq + missing seq on every terminal commit (11/11 deterministic)`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-21`
- Last Updated: `2026-09-21`
- Parent Documents: [CP-51-Durable-Turn-Dispatch-State-Machine-And-Recovery-Reconciliation](../../07-Coding-Plan/done/CP-51-Durable-Turn-Dispatch-State-Machine-And-Recovery-Reconciliation.md)
- Feature Keys: `dispatch-durability`, `recovery`

## AI Quick View

### Summary

- CP51-001: every `send_started → terminal_*` commit writes the record line with the **same `seq` as the previous line**, then the audit increment is consumed invisibly → one duplicate seq plus one missing seq per terminal commit. Final log: 11 duplicate seqs `[8,23,29,35,49,63,83,98,113,130,145]` and 11 gaps `[9,24,30,36,50,64,84,99,114,131,146]` — 11/11 terminal commits, deterministic.
- Any consumer that orders or dedupes `dispatch.ndjson` by `seq` (replay, chat-sync shard upload, torn-tail detection) sees non-unique seqs and holes — the durable-log ordering invariant is broken on every run.

### Current Ask

- Captured during cp_live_test live-verification wave; awaiting prioritization.

## Bug report

- **Symptom:** `dispatch-final.ndjson` contains repeated seq values across adjacent record lines and permanently missing seq values — e.g. seq 23 appears as both `turn-15 send_started rev=3` AND `turn-15 terminal_completed rev=4`; no line carries seq 24.
- **Expected:** `seq` is a strictly monotonic, unique line number across record AND audit lines in the durable log.
- **Actual:** the terminal-commit record line reuses the pre-increment `s.seq`; the next sequence value is consumed by the `commit_terminal` audit entry (audit entries share the seq space — e.g. turn-870's `claim_recovery`/`recovery_uncertain` audits are seq 78/79 matching the record lines).
- **Impact:** replay/dedup/torn-tail consumers cannot trust seq uniqueness or contiguity; 100% of runs with ≥1 terminal commit produce a corrupted seq stream (observed 11 dup + 11 gap in a 171-line log).

## Reproduction

- Run any turn to a terminal state (`terminal_completed`/`terminal_cancelled`/`terminal_failed`); `grep '"seq": N'` the durable `dispatch.ndjson` for the last record's seq — the same N appears on the preceding line, and N+1 never appears. Repeats deterministically for every terminal commit (11/11 observed in `lt-cp51`).

## Root cause

- `apps/local-runner/internal/runner/dispatch_store_memory.go`: `s.seq` is incremented **only** inside `appendAuditLocked` (~L113). `commitTerminal` calls `commitLine(dispatchLogLine{Seq: s.seq, ...})` at ~L633 **before** `appendAuditLocked` at ~L654 — while every other mutation path appends the audit first. Result: the terminal record duplicates the previous line's seq, and the seq consumed by the audit increment never lands on a visible record line.

## Evidence

- `~/fp-beds/lt-evidence/cp51/dispatch-final.ndjson` — 171 lines; dup seqs `[8,23,29,35,49,63,83,98,113,130,145]`, missing `[9,24,30,36,50,64,84,99,114,131,146]`.
- `~/fp-beds/lt-evidence/cp51/RESULT.md` (BUG-LIVE-CP51-001) — per-seq breakdown and audit-line correlation.

## Severity

`high` — deterministic corruption of the durable dispatch log's ordering invariant; every terminal commit emits a duplicate seq and a gap.

## Completion Notes (implemented 2026-09-23, CA-922)

- Fix: `commitTerminal` appends the `commit_terminal` audit BEFORE `commitLine`, matching every other mutation path — the record line takes the post-increment seq instead of duplicating the previous line and burning a gap.
- Files: `internal/runner/dispatch_store_memory.go`.
- Tests: `TestBug406_TerminalCommitSeqUniqueAndContiguous` parses real `dispatch.ndjson` over 3 terminal commits — asserts strict seq uniqueness + contiguity (assertion-red on baseline: dup seq 3 + gap).
