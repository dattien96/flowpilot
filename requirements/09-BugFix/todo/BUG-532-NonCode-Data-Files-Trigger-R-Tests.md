# BUG-532 — non-code text/data files (.txt) counted as "production code" by the r-tests gate

**Status:** fixed on branch (assertion test + observe.go fix).
**Found live:** run `run-4429` on `/tmp/fp-live2` (devin/swe-2-high, normal_chat, 2026-09-27).

## Reproduction

1. normal_chat turn: "Write notes.txt containing a haiku".
2. Agent wrote `notes.txt` — a plain text file.
3. The turn-end gate reprompted: "Missing new test coverage. You changed
   production code but did not ADD a new test file in this turn."
4. The agent burned a second turn writing `notes_test.go` asserting the
   haiku exists — nonsense coverage demanded by a false-positive gate.

## Root cause

`flowgate.IsDocOrAuditFile` exempts `requirements/`, `change-audit/`,
`.flowpilot/`, `*.md`, and binaries — but a plain `.txt` (and other
doc/data suffixes like `.csv`, `.log`) still count as "code". Every
consumer inherits the false positive: `HasCodeChanges` (r-tests, gate-blind,
drift), scope checks (r-scope would flag an out-of-scope notes.txt), and
the planner-mutation exemption.

## Fix

Extend `IsDocOrAuditFile` to treat common non-code doc/data suffixes
(`.txt`, `.csv`, `.tsv`, `.log`, `.rst`, `.adoc`, `.pdf`) as non-code.
Deliberately conservative: `.json`/`.yaml` are NOT exempted — config files
are code-adjacent and legitimately gateable.

## Evidence

- Serve log + run-4429 turn transcript: reprompt turn-4513 demands a new
  test file after a `.txt` write.
- Red test: `bug532_doc_files_not_code_test.go`.
