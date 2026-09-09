# CA-798 — tdd done advances coder; resume does not regen ingest

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: CP-60
change_type: bugfix
summary: Coder spawn accepts harness-style empty TestX files; missing frozen contract parks; resume after tdd advances coder not ingest
# --->8---

## Why

Live run-220036: tdd wrote signature-only `snake/snake_test.go` (correct). No `tdd-signatures.md`. `vibeCoderSpawnBlocked` required the md file, and `GetFrozenForStep(coder)` miss `continue`d without parking — hub hung ~8m with coder still `[ ]`. Resume must continue from tdd, not regen ingest.

## Change

- `hasVibeTddOutput`: non-empty md file OR git-new (untracked/added) signature-only `*_test.go`. Comments stripped before `t.Fatal` scan. Empty md, tracked placeholders, and full-body tests do not count (CA-769).
- No frozen contract, frozen store open failure, or writer spawn failure → park `requirement`.
- `maybeResumeVibeCoderAfterTdd`: in-flight claim; tdd DONE without artifact parks (not a silent spinner); skip when memory or persisted live/completed coder exists (no duplicate spawn); `tryAdvance(tdd)` only with sprint graph; missing graph parks (never ingest).
- Production `countChildRunsWithLabelLocked` (not the `_test.go` helper).
- `markForwardDonePredecessorsSkipped` leaves non-PENDING rows (DONE/RUNNING) intact.
- Checkpoint tdd layer can record git-new signature test rels.

## Tests

`ca798_tdd_output_advances_coder_test.go` (git-new signatures, comment t.Fatal, full-body, resume park/no-advance). `ca797` preserve-DONE. Old CA-769 tests untouched.

## Providers

Agnostic: disk walk + spawn gate take no `providerKey`.

## Will not undo

CA-769 md artifact still counts; repo full-body tests still do not. CA-795 tdd=`agent.code`. CA-793 demote. CA-797 skipped predecessors.
