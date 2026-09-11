# CA-822 — vibe Task/CP never `done`; DoD ticks; task x/y chip

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: BUG-367
change_type: bugfix
summary: Vibe engine stamps Task in_progress and ticks DoD boxes (never status done); CP lock is approved; TUI shows task x/y from LoopState
# --->8---

## Why

Live run-654339: snake vibe-sprint finished with only Task-910 core model
coded (`go test ./snake` green, empty `main()`). Task-911/912 unstarted.
CP-01 and Tasks stayed `draft` with every DoD box open. TUI chrome was
`Flow: vibe-sprint` with no 1/3 cue, so the operator could not see which
Task was running or that two sprints remained.

## Change

- `vibe_task_stamp.go` (new): `stampVibeTaskInProgress`,
  `stampVibeTaskDoDChecked`, `stampVibeCPApproved`, `stampVibeCPDoDChecked`,
  `stampCompletedVibeTask`. Status rewrite never emits `done`.
- `resumeVibeLock`: CP lock stamps `approved` (SS stamp unchanged).
- `maybeStartNextVibeSprint`: Task → `in_progress` when a sprint starts.
- `maybeParkVibeSprintBoundary`: tick completed Task DoD on park and on
  last-sprint no-park; last sprint also ticks CP DoD.
- `AgentLoopState` + TUI/desktop contract: `vibeTaskIndex/Total/Name`.
- TUI composer `Flow: vibe-sprint · task 1/3`; steps header
  `steps  task 1/3 Task-….md  round 0/3`.
- Prompts: task-splitter requires Acceptance Check checkboxes; implement
  step must not set Task/CP `status: done`.

## Tests

New files only. `TestBUG367_*` cover stamp never-done, DoD tick vs Out of
Scope, CP lock, last-sprint CP DoD, boundary park tick, graph snapshot
progress, TUI chip matrix (Claude/Codex/Grok). Old
`TestStepsHeader_ShowsRoundChip` still exact (empty vibe fields hide chip).

## Providers

Agnostic Case 1 for stamps/LoopState. TUI chips parameterized over the
three providers.

## Will not undo

BUG-365 SS approved stamp. CA-817/818 boundary Continue gate. Task-322
round chip. FEATURE-KEYS Allow (BUG-366 / CA-821).
