# CA-830 — Resume from tdd starts vibe-sprint when no tdd output

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: Task-329
change_type: bugfix
summary: Resume OK from tdd with Tasks present but no tdd-signatures starts vibe-sprint at tdd instead of silent maybeResumeVibeCoderAfterTdd no-op
# --->8---

## Why

Live R-TK-K (run-225468): Tasks kept, reopen parked Resume from tdd, Continue → all steps PENDING, no agents (hang). `maybeResumeVibeCoderAfterTdd` only advances coder after tdd artifacts exist; it does not start tdd.

## Change

- `forceStartVibeSprintAtTdd` + `resumeVibeAfterTddGate` (coder path if signatures exist, else start tdd)
- Wire Resume OK `from=tdd`

## Tests

`task330_resume_tdd_starts_sprint_test.go`. CA-801/CA-804 untouched green.

## Providers

Case 1 agnostic.
