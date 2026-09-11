# CP-60 Test Steps — TUI Vibe Snake MVP (gate-sandbox)

## Metadata

- Document ID: `CP-60-TEST-STEPS`
- Title: `CP-60 TUI verification — vibe-ingest a Go snake MVP on gate-sandbox`
- Phase: `verification`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-08`
- Last Updated: `2026-09-11`
- Parent Documents: [CP-60: Vibe Working Mode](./CP-60-Vibe-Working-Mode.md)
- Child Documents: `None`
- Related Documents: [SS-18](../../05-System-Specs/SS-18-Vibe-Working-Mode.md), [SD-24](../../06-System-Tech-Design/SD-24-Vibe-Working-Mode.md), [Task-326](../../08-Task/inprogress/Task-326-Vibe-Working-Mode-Switch-And-Flow-Family-Gate.md), [KR-004](../../reviews/KR-004-cp60-vibe-working-mode-impl-rev2.md), [CP-61-Test-Steps](../done/CP-61-Test-Steps.md)
- Replaces: `None`
- Tags: `vibe-mode, verification, test-steps, tui, cp-60, gate-sandbox`
- Feature Keys: `vibe-mode`

## AI Quick View

### Summary

- TUI-only live bed: `just chat-dev D:/working/gate-sandbox`.
- One Branch V run on **CA-791 graph**: vague prompt → `ss_lock` → `cp_writer` → join `task_slicer` (skip `cp_lock`) → 3× `vibe-sprint` v2 → playable Go snake (`rắn ăn mồi`) under `D:/working/gate-sandbox/snake/`.
- Language: **Go** (sandbox already `go test ./...`; vibe `validate` is real).
- Desktop / Admin Web / Branch C happy path are out of this file except one reject probe (F1).

### Current Ask

- Unit §2 **done** 2026-09-09 (`TestCA793_*` + reconstruct lock green). M1–M3 done.
- Live **run-678326** (`vibe-ingest`, grok-4.5, Mac `/Users/tiendat/Desktop/BE/gate-sandbox`) **done** 2026-09-11: three sprints (`task 3/3`), `snake/` has `game.go` `input.go` `main.go` `ui.go` + tests, `go test ./snake` **44 PASS**, `tdd-signatures.md` on disk, `snake-mvp` in FEATURE-KEYS.md. V5 + G2 ticked from this run. V7/V8 still unexercised. F1 PASS (invalid_flow_ref). §11 R-SS-K PASS; R-SS-D demote PASS + **Task-327** closes O-6 auto-resume (unit). Re-prove R-SS-D live spawn after Task-327 binary.

### Key Decisions

- `V-1` One product: terminal snake, stdlib only, package `snake`. Do not touch `calc.go` / `calc-core`.
- `V-2` Three frozen Tasks after `task_slicer` (not `sprint_slicer`): (1) grid+snake+food+collision tests, (2) WASD tick loop, (3) score + game-over + `go run ./snake`.
- `V-3` Empty Continue on the SS lock card = lock current draft. Paste SS markdown = write-back then re-validate. Join from ingest **skips** `cp_lock`.
- `V-4` After lock, user cards are only `r-requirement` or Owner-cap. Dev `1/2/3` in vibe = FAIL.
- `R-1` CA-793: three durable stops **SS / CP / Task**. Step done commits checkpoint only if files exist (`stat` + size>0). User delete demotes: Task→CP→SS→empty.

### Constraints

- `feature_key: vibe-mode` (FlowPilot). Game code in sandbox may use `snake-mvp` if that repo has `FEATURE-KEYS.md`.
- TUI only. Project path: Windows `D:/working/gate-sandbox` **or** Mac `/Users/tiendat/Desktop/BE/gate-sandbox` (not the FlowPilot checkout).
- R1: do not edit FlowPilot old tests to green a live fail.
- Live demo is operator; unit green ≠ this file done.
- run-213752 is **historical** (old `sprint_slicer` graph). Re-tick V1–V4 on the new run.

### Open Questions

- None for this bed. Branch C CP-ingest happy path is a later Test-Steps add-on.

### Source Refs

- CP-60 §7 manual checks, §10 DoD Branch V. Task-326 `/vibe` `/vibe off` `/flow`. `vibe-ingest` `ss_lock=user.confirm` → `cp_writer`. CA-791 join `task_slicer`. `vibe-sprint` v2 `context → tdd → coder → validate → synthesis → audit`. CA-793 `vibe_checkpoint.go` exist-gate + demote.

## 1. Goal

Prove TUI Vibe can take a vague snake idea, lock an SS, write one CP, auto-slice three Tasks, and leave a playable Go MVP in gate-sandbox without Dev cards and without editing `calc-core`.

## 2. Automated — run first

Working dir: `C:/working/flowpilot/apps/local-runner`.

```powershell
go test ./internal/workingmode/ ./internal/flowgate/ ./internal/agentpack/ ./internal/runner/ ./internal/tui/app/ -count=1 -timeout 180s
```

| Step | Pass khi | Tick |
| --- | --- | --- |
| 2.1 | `TestLoadBuiltinPack` → **12 flows / 8 agents** | [x] 12 flows; `TestPack_InventoryUnchanged` 12/8 |
| 2.2 | `TestDefaultRules` → **18** ids, no `r-requirement` in the list | [x] 18 ids, no `r-requirement` |
| 2.3 | `TestFlowAllowed_*` / `TestHTTPStart_*` / `TestTUIVibe*` related patterns green | [x] `-run` filter green |
| 2.4 | `TestVibeCoderSpawnBlocked_*`, `TestHasVibeTddSignatures_*` green | [x] |
| 2.5 | `TestCA793_CommitRequiresExistingFile` — missing SS does not commit | [x] 2026-09-09 PASS |
| 2.6 | `TestCA793_EmptyFileDoesNotCommit` — 0-byte CP does not count | [x] 2026-09-09 PASS |
| 2.7 | `TestCA793_DeleteCPDemotesToSS` — delete CP, SS remains → `ss_lock` | [x] 2026-09-09 PASS |
| 2.8 | `TestCA793_DeleteAllClearsCheckpoint` — delete SS → empty checkpoint | [x] 2026-09-09 PASS |
| 2.9 | `TestCA793_ReconstructDemotesDeletedTask` — delete Task, CP remains → `cp_writer` | [x] 2026-09-09 PASS |
| 2.10 | `TestVibeSession_ReconstructAwaitingLockAndIdempotent` still green (do not clear `vibeLockedSS`) | [x] 2026-09-09 PASS |
| 2.11 | `TestCA793_AliasSprintSlicerDemotesToCP` | [x] 2026-09-09 PASS |
| 2.12 | `TestCA793_AliasCpLockDemotesToSS` | [x] 2026-09-09 PASS |
| 2.13 | `TestCA793_CommitStoresCanonicalLayer` | [x] 2026-09-09 PASS |

```powershell
go test ./internal/runner/ -count=1 -timeout 60s -run 'TestCA793_|TestVibeSession_ReconstructAwaitingLock'
```

Full §2 command: `workingmode`/`flowgate`/`agentpack` ok. `./internal/runner/` hung >180s (not fail) on 2026-09-08; filtered CA-793 run is green 2026-09-09 (9 tests, 0.215s). `./internal/tui/app/` red on unrelated chrome (`TestStatusSpinner_*`, You-box) — not vibe. Do not edit those old tests.

Stop here if **vibe probes** red. Chrome red ≠ block V5.

## 3. Prep — gate-sandbox

| # | Việc | Cách kiểm | Tick |
| --- | --- | --- | --- |
| P1 | Sandbox exists | `Test-Path D:\working\gate-sandbox` | [x] |
| P2 | It is a Go module | `D:\working\gate-sandbox\go.mod` present | [x] |
| P3 | `calc-core` stays clean | `git -C D:\working\gate-sandbox status --short` — note dirty files; **do not** start from a half-edited `calc.go` | [x] 2026-09-09 clean (no `calc.go` diff) |
| P4 | No leftover `snake/` from a failed run | If `snake/` exists and is junk, move/delete it **before** vibe | [x] 2026-09-09 empty `snake/` removed; no SS/CP/Task vibe leftovers |
| P5 | TUI on **this** project | From FlowPilot repo: `just chat-dev D:/working/gate-sandbox` | [x] prior session; reopen for §5 |
| P6 | Statusline shows a real provider (Claude or Grok or Codex) | Not the scripted demo adapter | [x] OpenCode muse-spark |

If `FEATURE-KEYS.md` exists in the sandbox, add:

```text
- snake-mvp — terminal snake MVP (CP-60 vibe live bed)
```

Added 2026-09-09 under `D:/working/gate-sandbox/change-audit/FEATURE-KEYS.md`.

## 4. Mode gate (TUI, no game yet)

| # | Làm | Pass | Tick |
| --- | --- | --- | --- |
| M1 | `/vibe` then type `/flow ` | Suggestions = **only** `vibe-ingest` (bare or pack-prefixed) | [x] |
| M2 | `/vibe off` then `/flow ` | Exactly the five harness ids; **no** `vibe-ingest` / `vibe-sprint` / `vibe-owner-debate` | [x] |
| M3 | `/vibe` again (chip shows vibe / next-start default) | Restart TUI (`/exit`, `just chat-dev …`) still opens in vibe unless you `/vibe off` | [x] |

M2 fail → stop. Family gate is broken; snake run would be invalid.

## 5. Branch V — snake MVP (the live DoD)

Stay in **vibe**. Paste **exactly** this prompt (English on purpose — model writes Go):

```text
Build a new terminal snake game (rắn ăn mồi) as package snake/ in this Go module.

Do not modify calc.go, calc_test.go, or anything under the existing calc-core feature.

Stdlib only. No extra modules.

MVP:
- 20x20 grid
- snake starts length 3, moving right
- food spawns on a random empty cell
- WASD (and arrow keys if easy) move one cell per tick
- eat food: length+1, score+1, new food
- hit wall or self: game over, print score, offer restart (press R) or quit (Q)
- go test ./snake must be green (use/edge/error: wrap, empty spawn, self-hit, eat)
- go run ./snake is playable in this terminal

Slice exactly 3 sprints:
1) grid + snake + food + collision as testable funcs (no UI loop yet)
2) tick loop + WASD input
3) score, game-over, restart, go run ./snake

feature_key: snake-mvp
```

New run only. Do **not** resume run-213752.

| # | Làm | Pass | Tick |
| --- | --- | --- | --- |
| V1 | Send the prompt | Run starts `working_mode=vibe`, flow `vibe-ingest`. **Not** `task-harness`. | [x] run-223416 `vibe-ingest` VIBE, not `task-harness` |
| V2 | `ss_lock` card **SS Preview & Lock** | Timeline parks `WAITING_USER_APPROVAL`. Draft has `AC-*` for grid/food/WASD/game-over. | [x] run-223416 parked `ss_lock WAITING`, then DONE |
| V3 | Edit if AC missing (paste into card), then empty **Continue** / Lock | SS written under sandbox `requirements/05-System-Specs/` (or path on the card). Sprint does **not** start before lock. | [x] `SS-13-snake-mvp.md` (9184B) on disk; no sprint before lock |
| V4 | `cp_writer` then `task_slicer` (CA-791 join; **no** `cp_lock`, **no** `sprint_slicer`) | Exactly 3 `Task-*.md` under `requirements/08-Task/todo/`. Timeline shows task plan. No extra lock cards. | [x] `CP-snake-mvp.md` + `Task-904/905/906` (exactly 3); no `cp_lock` card |
| V5 | Each sprint | Order `context → tdd → coder → validate → synthesis → audit`. File `requirements/.flowpilot/vibe/tdd-signatures.md` exists **before** coder on that sprint. `validate` green. | [x] run-678326 `task 3/3` all steps DONE (plan/freeze/context/tdd/coder/validate/synthesis/audit); `requirements/.flowpilot/vibe/tdd-signatures.md` on disk 2026-09-11 (2.2K). Residual: one TDD turn first appended signatures into `game_test.go` (operator asked TDD re-run into the vibe file) |
| V6 | User cards after lock | Only `r-requirement` (plain AC↔test language) or Owner-cap. **Zero** Dev `1/2/3`. | [x] run-223416 operator-observed 2026-09-10 (2 TUI screenshots): `ss_lock` vibe_lock card + audit escalate `Retry` card only; no Dev `1/2/3` modal rendered |
| V7 | If `r-requirement` fires | Do **not** Approve a weaken-test. Fix SS or tests per the card, Continue. Never `done` while drifted. | [ ] N/A — not exercised (no `r-requirement` fired on sprint1); re-prove on a run where it fires |
| V8 | If generic `r-*` (not requirement) | `vibe-owner-debate` auto (2 owners). No Dev modal. | [ ] N/A — not exercised (audit missing-CA took the writer-retry path, not owner-debate); re-prove on a run where a generic gate fires |

### Game acceptance (after V5)

From **another** terminal (leave TUI running or `/exit`):

```powershell
cd D:\working\gate-sandbox
go test ./snake -count=1
go run ./snake
```

| # | Làm | Pass | Tick |
| --- | --- | --- | --- |
| G1 | `go test ./snake` | Green. Covers eat, wall, self-hit. | [x] 2026-09-11 `go test ./snake -count=1` → **44 PASS**, `ok gatesandbox/snake` (Mac sandbox). Historical 2026-09-10 9 PASS was sprint1-only |
| G2 | `go run ./snake` | Grid renders; WASD moves; eat grows + score; wall/self ends; R restarts or Q quits. | [x] run-678326 `snake/main.go` + `ui.go` (`func main` / `runCLI`: WASD+Enter, score, game-over, R/Q). Line-input fallback (SS-101). Not raw TTY |
| G3 | `git -C D:\working\gate-sandbox diff --stat` | `snake/` (+ tests) present. **`calc.go` / `calc_test.go` not modified.** | [x] `snake/{game,input,main,ui}.go` + tests untracked; `calc.go`/`calc_test.go` untouched |

V5 + G1 + G2 + G3 = live Branch V pass for this bed.

## 6. Fail-closed probes (same TUI session or a new one)

| # | Làm | Pass | Tick |
| --- | --- | --- | --- |
| F1 | `/flow vibe-cp-ingest README.md` (non-CP; `/vibe-cp` removed CA-782) | Deterministic reject. No CP lock card. | [x] |
| F2 | `/vibe off`, start a harness (`/flow` + `task-harness` or chat as usual) | Dev `1/2/3` path if a gate fires. **No** `vibe-owner-debate`. | [ ] |
| F3 | Optional: `/exit` mid SS-lock, `just chat-dev D:/working/gate-sandbox`, reopen the run | Lock card still there; mode still `vibe`. | [ ] |

## 7. Evidence to paste when ticking

- TUI run id / flow id (`vibe-ingest`): **run-678326 done** 2026-09-11 (Mac sandbox; `task 3/3`; synthesis `submit_review_outcome → approved`). Also historical **run-223416 done** 2026-09-10 (CA-791 graph: `ingest_reader` → `ss_converter` → `ss_validator` → `ss_lock` WAITING→DONE → `cp_writer` → `task_slicer` → sprint1). Gate inventory 2026-09-11: md scope-drift parks (`FEATURE-KEYS.md`, `tdd-signatures.md`) then ask_user "FlowPilot failure" — fixed BUG-370. Composer leftover `[stop]` after done — fixed BUG-371. No Dev `1/2/3` modal. Historical: run-213752 (old `sprint_slicer`), run-211980, run-213333. **Do not resume run-678326.**
- SS lock (historical): `ss_lock` `WAITING_USER_APPROVAL` then DONE; drafts `SS-14` / `SS-15` / `SS-16` — **removed 2026-09-09** so the next ingest starts clean. Only `FORMAT-REFERENCE-SS.md` remains under `05-System-Specs/`.
- `sprint_plan` (historical): `SPRINT-PLAN-snake-mvp.md` on run-213752. New binary expects 3 `Task-*.md` after `task_slicer`, not that file.
- Path of `tdd-signatures.md`: **present** 2026-09-11 at `requirements/.flowpilot/vibe/tdd-signatures.md` (2.2K) on Mac sandbox. 2026-09-10 was missing (sprint1-only).
- `go test ./snake` output: 2026-09-11 `go test ./snake -count=1` → **44 PASS**. Historical 2026-09-10 9 PASS (sprint1).
- `go run ./snake`: 2026-09-11 `snake/main.go` provides `func main` + `runCLI` (WASD line input, score, R/Q). Historical 2026-09-10 FAIL `not a main package`.
- `git diff --stat` proving calc untouched: Mac sandbox `?? snake/{game,input,main,ui}.go` + tests; `calc.go`/`calc_test.go` not modified.
- §2.1–2.4 (2026-09-08 Mac): `TestLoadBuiltinPack` PASS (12 flows); `TestPack_InventoryUnchanged` 12/8; `TestDefaultRules` PASS (18 ids, no `r-requirement`); vibe `-run` filter PASS. Full `./internal/runner/` hung >180s. `./internal/tui/app/` red on CA-537 spinner / You-box (not vibe; not edited).
- §2.5–2.13 (2026-09-09 Windows `C:/working/flowpilot/apps/local-runner`): `go test ./internal/runner/ -count=1 -timeout 60s -run 'TestCA793_|TestVibeSession_ReconstructAwaitingLock'` → 9 PASS, 0.215s.
- Sandbox clean 2026-09-09: empty `snake/` removed; no vibe SS/CP/Task files; `FEATURE-KEYS.md` has `snake-mvp`.

## 8. Stop / fail rules

- M1/M2 fail → family gate bug (`feature_key: vibe-mode`), do not continue to snake.
- Sprint starts before V3 lock → R-4 regression, new BUG.
- `cp_lock` card appears on Branch V after SS lock → CA-791 regression, new BUG.
- Coder runs with no `tdd-signatures.md` → TDD bypass, new BUG.
- `calc.go` edited → scope fail, revert sandbox, rerun.
- Live red while unit green → live residual (KR-004 O-1). Do not weaken `TestDefaultRules` / `TestLoadBuiltinPack`.

## 9. Out of scope

| ID | Concern |
| --- | --- |
| O-1 | Desktop chrome toggle (Task-326 §10.6) — not this file |
| O-2 | Admin Web 403 — unit already |
| O-3 | Branch C `/vibe-cp CP-*.md` N× sprint — later add-on |
| O-4 | Provider matrix 3× same snake (Claude+Grok+Codex) — optional rerun of §5 |
| O-5 | Pretty graphics / audio / high score file |
| O-6 | Auto `startResolvedFlowFromNode` at demoted checkpoint — **SS-delete @ ss_lock closed by Task-327 / CA-827** (reconstruct + Continue). Other layers (missing CP/Task auto-spawn) still residual. |

## 10. Definition of Done (this bed)

- [x] §2.1–2.4 vibe probes green (full `tui/app` chrome red, out of vibe)
- [x] §2.5–2.13 `TestCA793_*` + reconstruct lock green (2026-09-09)
- [x] M1–M3 ticked
- [x] V1–V4 ticked on run-223416 (CA-791; historical run-213752 does not count)
- [x] V6 ticked (zero Dev observed on run-223416 and run-678326 screenshots); V7/V8 OPEN (unexercised — no `r-requirement` / owner-debate card)
- [x] V5 ticked on run-678326 (`task 3/3`, `tdd-signatures.md` on disk, validate/synthesis/audit DONE)
- [x] G1 + G2 + G3 ticked (44 PASS; `main.go` playable CLI; calc clean)
- [x] F1 ticked; [ ] F2 ticked if a Dev gate was observed
- [x] §11 R-SS-K / R-SS-D (demote + Task-327 unit spawn); [ ] remaining live R-CP-* / R-TK-* / N-*
- [x] Evidence §7 attached for run-678326 (snake MVP files + `go test` 44 PASS) and historical run-223416

## 11. Resume checkpoint — SS / CP / Task (CA-793)

Three durable stops. History updates **only** if the artifact exists (`os.Stat`, size > 0). Empty file = missing.

Sandbox (this machine): `D:/working/gate-sandbox`. Alt: `/Users/tiendat/Desktop/BE/gate-sandbox`.

Inspect checkpoint after reconstruct: `vibe_checkpoint_node` / `vibe_checkpoint_artifacts` on the run (sessions.ndjson). Do not trust `vibeLockedSS` alone.

### 11.1 Same-run resume (reopen the old chat)

Stop with `/exit` (or kill TUI). Reopen `just chat-dev <sandbox>`. Open **the same run**.

| ID | Stop after | Disk after stop | Pass (demote) | Spawn next (residual) | Tick |
| --- | --- | --- | --- | --- | --- |
| R-SS-K | `ss_lock` | SS remains | checkpoint `ss_lock` | `cp_writer` | [x] run-225468 reopen kept `ss_lock` WAITING + lock card |
| R-SS-D | `ss_lock` | **delete** `requirements/05-System-Specs/SS-*.md` (not FORMAT) | checkpoint **empty** | `ingest_reader` | [x] demote empty (run-225468 sessions); spawn `ingest_reader` = **Task-327 / CA-827** (unit `TestTask327_*`); re-tick live on next binary |
| R-CP-K | `cp_writer` | CP + SS remain | checkpoint `cp_writer` | `task_slicer` | [ ] |
| R-CP-D1 | `cp_writer` | **delete CP**, SS remains | demote `ss_lock` | `cp_writer` | [ ] |
| R-CP-D2 | `cp_writer` | **delete CP + SS** | empty | ingest | [ ] |
| R-TK-K | `task_slicer` | Task + CP + SS remain | checkpoint `task_slicer` | `vibe-sprint` | [ ] |
| R-TK-D1 | `task_slicer` | **delete Task-*.md**, CP remains | demote `cp_writer` | `task_slicer` | [ ] |
| R-TK-D2 | `task_slicer` | **delete Task + CP**, SS remains | demote `ss_lock` | `cp_writer` | [ ] |
| R-TK-D3 | `task_slicer` | **delete Task + CP + SS** | empty | ingest | [ ] |
| R-NF | node done, artifact **never written** | no file | checkpoint **unchanged** (no commit) | previous layer | [x] unit 2.5 |
| R-EF | write 0-byte CP then done | empty file | no commit | previous layer | [x] unit 2.6 |
| R-AL | stored alias `sprint_slicer` / `cp_lock` | delete Task or CP as above | same demote as canonical layer | same | [x] unit 2.11–2.13 |

Delete demo: `rm` only the named glob under sandbox. Do **not** delete `calc.go`. Reopen the **same** run, then check checkpoint node.

### 11.2 New flow on changed codebase (new chat)

| ID | Disk | Start | Pass | Tick |
| --- | --- | --- | --- | --- |
| N-CP | `CP-*.md` exists | `/flow vibe-cp-ingest` + that path | flow `vibe-cp-ingest`, not `vibe-ingest` | [ ] |
| N-SS | SS exists, no CP | `/vibe` + idea prompt | **today** may re-ingest SS (residual; should skip to `cp_writer`) | [ ] |
| N-TK | Task files exist | new vibe run | **today** slicer may add Tasks (residual; should skip to sprint) | [ ] |
| N-DEL | delete SS+CP+Task then new `/vibe` + idea | ingest from idea | no stale checkpoint from the old run | [ ] |
| N-REJ | `/flow vibe-cp-ingest README.md` | reject, no lock card | same as F1 | [x] same as F1 |

Old-run checkpoint **must not** attach to a new `createRun`.

### 11.3 Fail rules for §11

- Commit checkpoint when the file is missing or 0-byte → BUG (exist-gate).
- Delete CP, SS remains, checkpoint stays `cp_writer` → BUG (demote).
- Delete all, checkpoint still `task_slicer` → BUG.
- `TestVibeSession_ReconstructAwaitingLockAndIdempotent` red because reconstruct wiped `vibeLockedSS` → BUG (do not edit that test).
