# CA-1041 — Non-code doc/data files no longer count as production code (BUG-532)

## Summary

A normal_chat turn that wrote `notes.txt` triggered the r-tests reprompt
("you changed production code… ADD a new test file"), forcing the agent to
invent nonsense coverage. `IsDocOrAuditFile` exempted only
requirements/change-audit/.flowpilot/*.md — plain doc/data suffixes counted
as code across every consumer (r-tests, r-scope, gate-blind, drift,
planner-mutation).

## Changes

- `internal/flowgate/observe.go`: `IsDocOrAuditFile` now treats
  `.txt/.csv/.tsv/.log/.rst/.adoc/.pdf` as non-code. `.json`/`.yaml`
  deliberately remain code-adjacent (config is legitimately gateable).
- `internal/runner/bug532_doc_files_not_code_test.go`: additive regression
  test (doc set + code-boundary guard + diff-level HasCodeChanges).
- `requirements/09-BugFix/todo/BUG-532-*.md`: capture doc.

## Red test

`TestBug532_NonCodeDocFilesDoNotCountAsCodeChanges` failed on `notes.txt`
before the fix.

## Live evidence

- run-4429 (devin/swe-2-high, /tmp/fp-live2): reprompt turn-4513 fired on a
  `.txt`-only write; after the reprompt the agent wrote `notes_test.go`
  asserting the haiku exists — the false-positive cost in one shot.
- Same run also re-verified mid-turn durability: kill -9 during a turn with
  a pending `permission_required` card → restart → card surfaces and the
  run resumes to `completed` on approve.

## Provider parity

Pure flowgate classification — provider-agnostic.
