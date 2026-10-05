# CA-1183 — r-task false positive on task-referencing legs (BUG-1183)

## Defect

`r-task` (`task_referenced`) required the referenced task document to appear
inside the firing turn's `GitDiff`. Reviewer, spec-aligner and synthesis legs
routinely mention `Task-NNN` in the final message without authoring the doc —
the doc was written by an earlier coding leg or a human — so the gate emitted
a false-positive reprompt, and repeated mounts burned the owner-debate budget
(BUG-1182 amplifier).

## Fix (domain-free)

The rule keeps its original contract — "a task reference must be backed by a
task document" — but satisfaction now has three layers, matching the r-bug /
r-ca parity already established by BUG-288 #8:

1. `HasTaskDoc(tr.GitDiff)` — unchanged, doc authored this turn.
2. `HasTaskDocInPaths(tr.WrittenPaths)` — held paths on pending re-checks
   (empty-GitDiff remediation turns).
3. `TaskDocExistsOnDisk(tr.WorkspaceCwd, taskRefIDs(tr))` — the referenced
   `Task-NNN` doc already exists under `requirements/08-Task/**`. When ids are
   extractable from `SourceDocID`/`FinalMessage` the match is id-scoped
   (`Task-039*.md`); with no ids it falls back to any `Task-*.md` under the
   tree — the same looseness `HasTaskDoc` already applies to diff paths.

`WorkspaceCwd` empty → no disk fallback → identical behavior to before
(fail-closed). No role/phase conditionals added — the coordinator stays
domain-free per `AGENTS.md` §1.

## Evidence

Live CP-03 (`run-204891` sprint): reviewer/spec-align legs emitted
`Task-039` references and hit `r-task` reprompts even though
`requirements/08-Task/todo/Task-039-*.md` existed, feeding owner-debate mounts.

## Regression tests

`bug1183_task_doc_on_disk_test.go` — 6 cases: on-disk doc satisfies (both
implicit reference and declared task mode), missing doc still violates,
wrong-id doc still violates, held `WrittenPaths` doc satisfies, empty
`WorkspaceCwd` keeps prior fail behavior.
