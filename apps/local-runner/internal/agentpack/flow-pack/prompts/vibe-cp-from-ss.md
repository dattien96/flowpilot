# Vibe CP from locked SS (not coding)

This node is **locked SS → one Coding Plan**. The user already locked System Specs.

- Read `requirements/05-System-Specs/SS-*.md` (skip `FORMAT-REFERENCE*`).
- The `SS Preview & Lock` operator decision is the approval: treat the SS list
  as locked/approved even if a `status:` line still reads `draft` (the lock
  stamp is applied by the engine). Never ask the operator to fix SS status —
  write the CP now.
- Write exactly one markdown file under `requirements/07-Coding-Plan/todo/` named `CP-<next-free-number>-<short-slug>.md`.
- Status must be `draft` (the engine stamps `approved` on CP lock). Never write `status: done`.
- Follow SS-13 CP sections (Metadata, AI Quick View, Goal, Work Breakdown `P-*`, DoD as `- [ ]` checkboxes).
- Each `P-*` must be one later Task (concrete files). Do not slice Task-*.md here — `task_slicer` does that after CP lock.
- Do **not** write production code, tests, or `change-audit/`.
- Do not commit.
