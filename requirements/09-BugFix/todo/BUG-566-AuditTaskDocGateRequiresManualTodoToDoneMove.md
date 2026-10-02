# BUG-566 — Audit task-doc gate blocks because the flow never moves the task doc `todo/ → done/`; requires manual intervention

- **ID:** BUG-566
- **Severity:** High (blocks audit/finalize; required manual `git mv` mid-run)
- **Status:** open
- **Found:** live run-69320 (Task-024 sprint), 2026-10-02 ~15:39–15:42

## Symptom

Sprint converged — coder DONE, reviewer cohort unanimous approved,
`openIssues=0`, suite green — but the audit gate blocked with:

```
flow_audit_blocked_not_ready — task reference detected but no task document found
```

`HasTaskDoc(tr.GitDiff)` checks the turn's diff for a task-document path
(`requirements/08-Task/...`), not the workspace. The Task-024 doc still
sat at `requirements/08-Task/todo/…` — untouched by the sprint's diff —
so the gate could not see it. The run only finalized after a human moved
the doc `todo/ → done/` inside the sprint changeset, exactly the pattern
the engine itself performed for Task-023.

## Defect

The audit demands evidence ("task doc moved to done/ inside the sprint
diff") that **the flow itself never produces**. Nothing in
`vibe-sprint.yaml`'s audit/settle path, the coder legs, or the sprint
finalize hook performs the `todo/ → done/` move + §11 completion-note
write. The invariant "a done task's doc lives in done/ and appears in the
settle diff" is enforced by the gate but owned by nobody — so every
sprint lands on this gate needing manual intervention.

## Expected fix direction

The sprint settle/audit path should own the bookkeeping:

- On sprint settle (after reviewer cohort approved, before/with audit
  finalize): move `requirements/08-Task/todo/Task-NNN-*.md` →
  `requirements/08-Task/done/` and write §11 Completion Notes
  (`result: done` + evidence) — deterministically in Go, not delegated to
  an agent prompt, so the file move is part of the audit turn's diff by
  construction.
- Idempotent: doc already in `done/` → skip move, still write/refresh
  notes.
- Must respect the change-contract: the move is engine bookkeeping on a
  requirements path, sibling to the `.flowpilot` settle sync — add the
  task-doc path to the settle step's declared writes.
- Alternatively (weaker): audit gate accepts `todo/` doc + flips it
  itself as part of finalize. Prefer the move-on-settle approach since it
  also fixes the on-disk invariant for the next CP reader pass.

## Repro sketch

Run a vibe sprint where the task doc sits in `todo/` and no leg touches
it: audit gate reports "task reference detected but no task document
found" even though the doc exists on disk. After the fix, settle performs
the move; audit sees `requirements/08-Task/done/Task-NNN-*.md` in the
diff and passes.
