# CA-791 — ingest cp_writer joins vibe-cp-ingest at task_slicer

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: CP-60
change_type: feature
summary: After ingest writes CP, overlay vibe-cp-ingest and spawn task_slicer (skip cp_reader/cp_lock); wire cp_validator to vibe-cp-validate.md
# --->8---

## Why

Two vibe entries, one implement tail. Ingest already locked SS and wrote the CP — re-running `cp_reader` + `cp_lock` was a second confirm. User-start `vibe-cp-ingest` still reads and reviews the CP.

## Change

- `maybeStartVibeCpIngest`: `vibeAwaitingLock=false`; `startResolvedFlowFromNode(..., task_slicer)`
- `startResolvedFlowFromNode`: optional start node; empty = existing entry spawn
- `cp_validator` `promptTemplate: prompts/vibe-cp-validate.md` (manifest-declared)
- User-start `vibe-cp-ingest` still no-ops on `cp_writer` so `cp_lock` runs

## Tests

New `ca791_ingest_joins_slicer_test.go`, `ca791_vibe_cp_validate_prompt_test.go`.
Operator-approved edit of `TestOnVibeCpWriterDone_StartsVibeCpIngest`: overlay `vibe-cp-ingest` still required; `vibeAwaitingLock` must stay false; no `cp_reader` child. `TestOnVibeCpWriterDone_IdempotentWhenAlreadyCpIngest` untouched.

## Providers

Agnostic: `maybeStartVibeCpIngest` / `startResolvedFlowFromNode` take no `providerKey` (`vibe_cp.go` has zero `providerKey` refs). `newTestServer` only implements Codex createRun; join tests use Codex as the representative.


## Will not undo

CA-786 overlay onto vibe-cp-ingest graph. CA-788/789 writer terminal. CA-790 lock-once. CA-783 slicer → vibe-sprint.
