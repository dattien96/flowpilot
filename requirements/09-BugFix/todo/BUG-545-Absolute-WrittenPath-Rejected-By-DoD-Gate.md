# BUG-545 — absolute WrittenPaths inside workspace rejected by the DoD gate

## Status
FIXED — unit-verified (red→green), live leg verified 2026-09-28
(run-34296/run-34322 on :4322 fp-live5, rebuilt binary).

## Live-found during
`run-34296` (tournament-harness on :4322), candidate-b `run-34322`,
2026-09-28.

## Live sequence
1. Candidate-b's turn completed having written
   `requirements/08-Task/done/Task-06.md` with a valid
   `## Definition of Done` checklist — verified on disk.
2. The gate's `tr.GitDiff`/`WrittenPaths` carried the provider's file-change
   path **absolute**:
   `/private/tmp/fp-live5/.flowpilot/worktrees/candidate-candidate-b/requirements/08-Task/done/Task-06.md`.
3. `MissingDodDocs` (`internal/flowgate/dod.go`) rejected every
   `filepath.IsAbs` candidate via the escape guard → the document was
   reported missing on EVERY evaluation.
4. Reprompt budget drained against a violation no agent action could clear
   → repeated `flow_control_escalate` (max reprompts exceeded) — a
   permanent wedge disguised as gate debt.

## Root cause
The absolute-path escape guard in `MissingDodDocs` treated in-root absolute
paths identically to out-of-root traversal. Provider event paths (devin
ACP `file_change` notifications) are absolute; sibling consumers
(`HasChangeAuditNoteInPaths`, `HasCodeChangesInList`) tolerate them via
`strings.Contains` matching, so only the DoD doc check wedged.

## Fix (CA-638)
`MissingDodDocs` resolves an absolute candidate with
`filepath.EvalSymlinks` and accepts it only when the resolved path stays
inside the resolved `WorkspaceCwd` root; out-of-root and `..` escapes
remain rejected. In-root paths are then read and validated normally.

## Tests
`internal/flowgate/bug545_abs_writtenpath_dod_test.go`:
- abs path inside root + valid DoD → not missing;
- abs path outside root → missing;
- abs path inside root, no DoD → missing.

## Live verification
Post-fix binary: the same absolute `Task-06.md` path no longer appeared in
the violation list; candidate-b advanced `WAITING_USER_APPROVAL → DONE`
(12:42:48Z).
