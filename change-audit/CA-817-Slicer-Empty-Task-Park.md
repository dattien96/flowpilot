# CA-817 — slicer with zero Task output parks instead of sprinting from fallback

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: BUG-363
change_type: bugfix
summary: Slicer-done with a visible workspace and zero Task files parks for the operator instead of starting a sprint from the SS-glob fallback
# --->8---

## Why

Live run-635006 (clean sandbox, Branch V snake): `ss_lock` → `cp_writer`
→ `task_slicer` → 1 sprint → `done`, with no `CP-*.md` and no `Task-*.md`
ever written. `cp_writer` resolved its `CP-{{cpID}}-{{slug}}.md` template as
`SP-snake-mvp.md` under `05-System-Specs/`; templated OUTPUT bindings are
prompt-level only by design (`artifact_type_registry.go:615-622` — the
flowgate "must NOT reprompt on it"), so nothing checked. Then
`maybeStartVibeCpIngest` started the slicer on a directory fallback prompt
(`vibe_cp.go:218-221`) and the slicer-done handler fell back to the SS glob
(`vibe_cp.go:259-262`) — a silent chain ending in a healthy-looking `done`.

## Change

- `onVibeCpNodeDone` slicer branch (`vibe_cp.go`, covers `task_slicer` +
  legacy `sprint_slicer`): with `workspaceCwd != ""` and zero Task files via
  `collectVibeTaskPlan`, `parkVibeRequirement` with an explicit gate reason
  and return before artifact record / sprint start. Skipping the checkpoint
  commit for phantom progress is intentional (fail-closed, CA-793 spirit).
- `F-1` (park in `maybeStartVibeCpIngest` on missing CP) was REJECTED: it
  contradicts pinned `TestCA791_MissingCPFileStillJoinsSlicer` and no code
  discriminator separates that shape from the live failure without editing
  the old test (R1). Recorded in BUG-363 §7.
- `collectVibeSprintPlan` SS fallback untouched.

## Tests

- `bug363_vibe_writer_output_gates_test.go` (new, additive-only):
  - `SlicerDoneWithoutTasksParks` — exact live shape (SS present, no
    Tasks): blocked/requirement, GateReason names Task output, sprint
    index 0, no tdd/coder/preflight nodes. Verified red-before (FAIL on
    pre-fix code) / green-after.
  - `LegacySlicerAliasWithoutTasksParks` — `sprint_slicer` alias same.
  - `ParkThenTasksRecoverFromTasks` — park stores no stale plan; adding
    Tasks then re-running the slicer sprints from the Task files.
  - `EmptyCwdKeepsLegacyBehavior` — empty cwd never parks (legacy
    carve-out pinned).
- The gate runs before the plan-mutation block so a park leaves
  `rs.vibeTaskPlan` empty for a later retry (round-1 review finding).
- Present-Task pass-through covered by existing CA-791
  `SlicerAfterIngestJoinStartsSprint` (cited, not duplicated).
- Related suites green: CA-78x/79x/80x/81x, `TestOnVibe*`,
  `TestCollect*`, Task-321/326, `TestVibeSession_*` — 59 PASS, 0 FAIL.

## Providers

Agnostic Case 1: the new branch keys off `workspaceCwd` + Task-glob only;
zero `providerKey` references in `vibe_cp.go`. Tests use
`ProviderKeyCodex` via `newTestServer`, same precedent as CA-796/797/798.

## Will not undo

CA-783 SS fallback (function unchanged; `UsesSSWhenNoTasks` green).
CA-786/CA-791 overlay + join (empty-cwd and Tasks-present shapes green).
CA-793 checkpoint semantics (skipped commit here is the exist-gate applied,
not a bypass). CA-798 TDD gate (untouched). BUG-308 fence generation
(untouched). Residuals: stale pre-existing CP still counts as present
(standalone vibe-cp-ingest consumes it by design); writer-output freshness
(provenance vs lock time) left for follow-up. The Task check matches only
`08-Task/todo/Task-*.md` — a Task written elsewhere (e.g. `inprogress/`)
still parks by design (fail-closed); widen the glob only with a new CA.
