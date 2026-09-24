# BUG-399: vibe-cp-ingest accepts non-CP input — README.md produces SS drafts and parks at lock

## Metadata

- Document ID: `BUG-399`
- Title: `vibe-cp-ingest runs a README through CP ingestion — no CP-shape validation before SS drafting`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-21`
- Last Updated: `2026-09-21`
- Parent Documents: [CP-60-Test-Steps](../../07-Coding-Plan/done/CP-60-Test-Steps.md)
- Feature Keys: `vibe-mode`

## AI Quick View

### Summary

- run-3439 fed `README.md` into `vibe-cp-ingest`; `cp_reader` (vibe-intake agent) treated it as a raw requirement, drafted `SS-108`/`SS-109` + `SPRINT-PLAN-readme`, and parked at `SS Preview & Lock` (`user_question_required` "drafts are ready").
- The flow contract says "read a CP-*.md" — there is no CP-shape validation; a README passes straight through to SS drafting.
- Parked safely (nothing destructive; lock checkpoint held), so impact is a quality/fail-closed gap rather than data loss — but the drafts/lock park create a plausible-looking plan from the wrong input.

### Current Ask

- Captured during cp_live_test live-verification wave; awaiting prioritization.

## Bug report

- **Symptom:** `vibe-cp-ingest` accepts and processes a non-CP document end-to-end to the SS-lock checkpoint instead of rejecting it.
- **Expected:** The ingest validates the input is a CP-shaped document (metadata/sections) and rejects/parks with an explicit "not a CP" error before drafting SS artifacts.
- **Actual:** Arbitrary markdown is drafted into SS docs + sprint plan; the only safeguard is the human at the SS Preview & Lock gate.
- **Impact:** Wrong-input ingestion silently produces plausible artifacts; if the operator confirms the lock, downstream cp_writer/slicer/vibe-sprint machinery runs on a non-CP source. Fail-closed posture of the vibe pipeline weakened.

## Reproduction

1. `POST /client/workflow-runs` + turn with `flowRef:"vibe-cp-ingest"`, `workingMode:"vibe"`, pointing the intake at a non-CP file (e.g. `README.md`) — run-3439.
2. Observe `cp_reader` draft `SS-108`/`SS-109` + `SPRINTPLAN-readme` and park at SS Preview & Lock.
- runIds: `run-3439`.

## Root cause

- The `vibe-cp-ingest` flow's intake (`cp_reader` node) has no input-shape validation step — nothing checks the source document is a `CP-*` coding plan (metadata block, required §1–§8 sections) before SS drafting proceeds. The intake agent defaults to treating input as a raw requirement.

## Evidence

- `~/fp-beds/lt-evidence/cp60/RESULT.md` (L-60-3 notes; Bugs BUG-LIVE-2)
- `~/fp-beds/lt-evidence/cp60/run-3439-events.ndjson` — `user_question_required` "SS Preview & Lock — drafts are ready"
- `~/fp-beds/lt-evidence/cp60/l60-3-probe-outcomes.txt` — probe detail

## Severity

- `medium` — fail-closed/quality gap; non-destructive in the observed run (parked at lock awaiting human), but no defense before that point.

## Completion Notes (implemented 2026-09-23, CA-921b)

- Root cause: the `vibe-cp-ingest` flow-start turn had no deterministic input check — `DetectVibeEntry`/`RejectNonCP` only run in the TUI picker and the cp_lock write-back; an API-pinned `flowRef` sent `README.md` straight to `cp_reader`.
- Fix: `startTurn` admission validates the flow-starting turn (turnCount==0, non-restored) — source must resolve to `requirements/07-Coding-Plan/**/CP-*.md` (SourceDocID or first CP-shaped prompt token) whose file exists and carries `Document ID: CP-*`; fail-closed `invalid_cp_source` 422 otherwise.
- Files: `internal/runner/vibe_cp.go` (`validateVibeCpIngestSource`), `interactive_service.go` (admission hook).
- Tests: `bug399_cp_ingest_validation_test.go` — 5 tests (README prompt / missing Document ID / missing file rejected; real CP admitted; follow-up not revalidated). Baseline-red verified.
- Live: local runner, `flowRef=vibe-cp-ingest` + `README.md` → `422 invalid_cp_source`; real CP → admitted, `cp_reader` spawned.
