# BUG-363 — Vibe ingest writer outputs unverified (wrong-path CP + silent slicer fallback)

## Metadata

- Document ID: `BUG-363`
- Title: `Vibe ingest writer outputs unverified — cp_writer wrong path + slicer silent fallback (run-635006)`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-09`
- Last Updated: `2026-09-09`
- Parent Documents: [CP-60: Vibe Working Mode](../../07-Coding-Plan/done/CP-60-Vibe-Working-Mode.md), [CP-60 Test Steps](../../07-Coding-Plan/done/CP-60-Test-Steps.md)
- Child Documents: `None`
- Related Documents: [SS-18](../../05-System-Specs/SS-18-Vibe-Working-Mode.md), [SD-24](../../06-System-Tech-Design/SD-24-Vibe-Working-Mode.md)
- Replaces: `None`
- Tags: `vibe-mode, ingest, fail-closed, live-bed, regression`

## AI Quick View

### Summary

- Live run-635006 (new Branch V snake run, clean sandbox): `ss_lock` → `cp_writer` → `task_slicer` → 1 sprint → `done`, but **no `CP-*.md`** and **no `Task-*.md`** were ever written.
- `cp_writer` wrote `05-System-Specs/SP-snake-mvp.md` (invented SP layer) instead of `07-Coding-Plan/todo/CP-*.md` per its `pathTemplate`. Templated OUTPUT bindings are prompt-level only by design (`artifact_type_registry.go:615-622` — flowgate "must NOT reprompt on it"), so the deviation sailed through with zero signal.
- Two silent fallbacks compounded it: `maybeStartVibeCpIngest` starts the slicer on a directory prompt when no CP exists (`vibe_cp.go:218-221`); the slicer-done handler falls back to the SS glob (`vibe_cp.go:259-262`) → 1 sprint from SS → flow done looking healthy.
- NOT in scope: missing `tdd-signatures.md` file is allowed by design (CA-798 — git-new signature Test frames count; coder ran, so the gate saw frames).

### Current Ask

- Fail closed at both writer boundaries (verify on disk, park with a clear gate reason instead of silent fallback) without changing legacy no-workspace shapes or the CA-783-pinned SS fallback.

### Key Decisions

- `V-1` Verification is active only when `workspaceCwd != ""` (disk visible). Empty-cwd unit shapes keep legacy behavior — this is what keeps CA-786/CA-791/CA-783 green untouched (R1).
- `V-2` Keep the SS-glob fallback in `collectVibeSprintPlan` (pinned by CA-783 `TestCollectVibeSprintPlan_UsesSSWhenNoTasks`); fail closed one layer up, in `onVibeCpNodeDone`.
- `V-3` Stale pre-existing CP (from an older run) still counts as "CP present" — standalone `vibe-cp-ingest` legitimately consumes pre-existing CPs, so presence (not freshness) is the check. Documented residual.

### Constraints

- `feature_key: vibe-mode`.
- R1: no edits to CA-786/CA-791/CA-783 or any old tests.
- R2: agnostic path (disk + loop state, no `providerKey`); Codex-representative tests per CA-796/797/798 precedent.
- TUI-only live bed; unit green ≠ live DoD (CP-60-Test-Steps still needs a re-run after fix).

### Open Questions

- None blocking. Follow-up candidate (not this bug): provenance check (CP newer than lock) for the stale-CP residual.

### Source Refs

- Live: run-635006 (`vibe-ingest`, opencode muse-spark), sandbox `/Users/tiendat/Desktop/BE/gate-sandbox` (was `git status` clean before run).
- Disk after run: `SS-snake-mvp.md` (7.6KB), `SP-snake-mvp.md` (1.9KB), `snake/{main,snake,snake_test}.go`, `CA-922-snake-mvp.md`; `07-Coding-Plan/todo/CP-*.md` absent; `08-Task/todo/Task-*.md` absent; `tdd-signatures.md` absent anywhere.
- `go test ./snake` green (G1) despite the above — game code exists, plan artifacts don't.

## 1. Issue Summary

On a clean sandbox, a fresh vibe-ingest run locked SS, ran `cp_writer`, ran `task_slicer`, ran one sprint (`tdd → coder → validate → synthesis → audit`), and finished `done` — while writing no CP file and no Task files. The CP slot was filled with a mislabeled `SP-snake-mvp.md` under `05-System-Specs/`. The run looked healthy in the timeline; only the disk (and V4/V5 ticks) showed the failure.

## 2. Parent Links

- impacted coding plan: `CP-60-Vibe-Working-Mode.md` §7 manual checks, §10 DoD Branch V; `CP-60-Test-Steps.md` V4/V5/G1.
- impacted tech design: `SD-24-Vibe-Working-Mode.md` (ingest → CP → slice → sprint chain).
- impacted system spec: `SS-18-Vibe-Working-Mode.md` (TDD-first per-sprint loop, artifact stops).

## 3. Environment and Reproduction

- environment: TUI `just chat-dev /Users/tiendat/Desktop/BE/gate-sandbox` (Mac), provider opencode/muse-spark, FlowPilot branch `cp60-vibe` at CA-816.
- reproduction steps: clean sandbox (no SS/CP/Task/snake) → new `/vibe` run with the §5 snake prompt → lock SS at `ss_lock` → Continue → observe `cp_writer` then `task_slicer` complete with no `CP-*.md` / `Task-*.md` on disk.
- frequency: observed once live (run-635006); deterministic at unit level (fallback chain has no randomness).

## 4. Expected vs Actual

- expected: `cp_writer` done ⇒ one `07-Coding-Plan/todo/CP-*.md` exists; `task_slicer` done ⇒ N `08-Task/todo/Task-*.md` exist; missing writer output parks with a gate reason (fail-closed, CA-793 precedent).
- actual: writer outputs unverified; directory-prompt fallback + SS-glob fallback chain silently produced a 1-sprint run that finished `done`.

## 5. Impact

- users affected: operator live bed (CP-60 Branch V DoD blocked: V4/V5 untickable on affected runs).
- workflows affected: `vibe-ingest` → `vibe-cp-ingest` join; any run where a doc-writer deviates from its `pathTemplate`.
- severity: high for vibe-mode correctness (silent plan loss), low blast radius (vibe-only path, no harness flows touched).

## 6. Root Cause

- hypothesis: "có SS/SP rồi nên cp_writer skip" — REJECTED. Sandbox was `git status` clean; edge `ss_lock --done--> cp_writer` always spawns (pinned by CA-789).
- confirmed cause (2 layers):
  1. Templated OUTPUT bindings are prompt-level only by design (`artifact_type_registry.go:615-622`); the agent resolved `CP-{{cpID}}-{{slug}}.md` as `SP-snake-mvp.md` under the wrong folder and nothing checked.
  2. `maybeStartVibeCpIngest` (`vibe_cp.go:218-221`) and the slicer-done plan collect (`vibe_cp.go:259-262`, `360-364`) fall back silently (directory prompt → empty slicer → SS glob → `["sprint-0"]`), so a missing writer output degrades into a smaller run instead of parking.
- evidence: timeline run-635006 (`cp_writer` RUNNING → `task_slicer` RUNNING → single sprint → done); disk listing (no CP/Task globs match; SS+SP+snake present); `TestCollectVibeSprintPlan_UsesSSWhenNoTasks` pins the SS fallback as intended — hence the fix goes one layer up, not into the fallback.

## 7. Fix Strategy

- `F-1` (REJECTED during implementation) `maybeStartVibeCpIngest` park-on-missing-CP contradicts `TestCA791_MissingCPFileStillJoinsSlicer` (TempDir workspace, no CP, asserts the overlay still joins). Same shape as the live failure, opposite pinned verdict — no code discriminator separates them without editing the old test (R1 forbids). Fail-closed point moved to `F-2`.
- `F-2` `onVibeCpNodeDone` slicer branch: if `workspaceCwd != ""` and the Task glob is empty → `parkVibeRequirement` ("task_slicer produced no Task files…") instead of starting a sprint from the SS fallback. `collectVibeSprintPlan` itself unchanged (CA-783 green); CA-791 pass-through (Tasks present) and all empty-cwd shapes unchanged.
- `F-3` Additive matrix tests only (`bug363_…_test.go`, 4 tests): missing-Task parks, sprint_slicer alias parks, park→Tasks recovery sprints from Tasks (guards gate-before-mutation ordering), empty-cwd legacy carve-out pinned. Present-Task pass-through already guarded by CA-791 `SlicerAfterIngestJoinStartsSprint` (cited, not duplicated).

## 8. Validation

- `V-1` New `bug363_*` tests green; CA-786/CA-791/CA-783/CA-793 suites green untouched.
- `V-2` `go test ./internal/runner/ -run 'TestBUG363_|TestCA791_|TestCA783_|TestCA786|TestOnVibeCpWriterDone_|TestAdvanceHubDone_SprintSlicer|TestCollectLatestVibeCP|TestCollectVibeSprintPlan|Task321|Task326'` green (verified 0 FAIL; broader 59-PASS sweep in §9).
- `V-3` Live re-run of CP-60-Test-Steps §5 on a clean sandbox must now either produce CP+Tasks or park with the new gate reason (operator tick, not claimed here).

## 9. Regression Guard

- tests: `bug363_vibe_writer_output_gates_test.go` (4 tests: slicer-missing-Task parks, sprint_slicer alias parks, park→Tasks recovery sprints from Tasks, empty-cwd legacy preserved). Present-Task pass-through covered by existing CA-791 `SlicerAfterIngestJoinStartsSprint` (cited, not duplicated).
- alerts: any future `TestCollectVibeSprintPlan_UsesSSWhenNoTasks` / `TestOnVibeCpWriterDone_*` red ⇒ STOP (R1).
- audit checks: CA note referencing BUG-363 with Providers Agnostic Case 1 + will-not-undo list (§V-1/V-2 above).

## 10. Follow-Up Document Updates

- upstream docs that must change: `change-audit/CA-NNN` (new note for the fix); CP-60-Test-Steps §7 evidence row after live re-run.
- notes left unchanged on purpose: CA-783 (SS fallback stays), CA-786 (overlay stays), CA-798 (Test-frames gate stays), `task-splitter.md` prompt (not the defect).
