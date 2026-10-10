# BUG-662 — Task collector only parses `Parent Documents`, misses `Parent Links` tasks

## Metadata

- Document ID: `BUG-662`
- Title: `Task collector only parses Parent Documents, misses Parent Links tasks`
- Phase: `bugfix`
- Status: `done`
- Fixed By: `CA-657` (collector honors `Parent Links` dialect; commit 781d44a5)
- Owner: `flowpilot runner`
- Reviewers: `dev`
- Created: `2026-10-09`
- Last Updated: `2026-10-09`
- Related Documents: `run-640182` (PrivateVault vibe run, CP-12, cancelled by user)
- Tags: `watchdog, production-watch, vibe-tasks, task-scoping`

## AI Quick View

### Summary

- Live PrivateVault run for CP-12 started Task-0312 (parented to CP-03) instead of the CP-12 tasks 121–124. User cancelled the run.
- `collectVibeTaskPlanForCP` / `collectVibeDoneTasksForCP` (`apps/local-runner/internal/runner/vibe_cp.go:920`) scope tasks to a CP by parsing ONLY the `Parent Documents:` metadata line (`vibeTaskParentDocsLine`).
- Tasks 121–124 declare parentage as `- Parent Links: CP-12 (...)` — no `Parent Documents:` line — so the CP-12 plan resolves empty for them, while Task-0312 (`- Parent Documents: CP-03, ...`) is collected for any run whose scoping falls back to the legacy unscoped glob (empty cpID) or a mismatched pin.
- Two task-parentage dialects exist in the wild (`Parent Documents` vs `Parent Links`); the collector honors one. BUG-468 scoped the glob to stop foreign-task pickup, but the scope key misses the second dialect, reproducing the same user-visible symptom (wrong task sprinted) through a different hole.

### Evidence (watchdog snapshot)

```
run: run-640182
status=cancelled (user-cancelled after wrong-task start)
Task-0312-crypto-ndk-container-compaction-vacuum.md parentage: `- Parent Documents: CP-03, Task-0311, SD-01, SS-01`
Task-121..124 parentage: `- Parent Links: CP-12 (Air-Gap Offline Vault Migration), ...` (no `Parent Documents:` line)
collector regex: `(?im)^\s*-?\s*Parent Documents\s*:(.*)$` (vibe_cp.go:920)
CP-12 doc present: requirements/07-Coding-Plan/todo/CP-12-airgap-offline-vault-migration-hotspot-socket.md
```

### Deep check (empirical, on the real repo — worktree + main, identical)

Replicated `collectVibeTaskPlan` + `collectVibeTaskPlanForCP` semantics in
Python over `requirements/08-Task/todo/` (21 files):

- Unscoped sorted-first = `Task-0312-...` exactly (`Task-0312` < `Task-058`
  < `Task-1xx` in byte order) — the observed pick, to the file.
- Scoped `CP-12` via the tool regex = `[]` (121–124 invisible: `Parent
  Links` dialect, no `Parent Documents:` line).
- Scoped `CP-03` = 3 files, first = `Task-0312-...`.

Mechanism constraint: had `vibeCpDocID` been `CP-12` through the
reader arm, `onVibeCpNodeDone` (vibe_cp.go:1467) would have PARKED with
"found no Task files parented to CP-12" instead of sprinting — a sprint
started, so either (a) `cpID` was empty (unscoped glob → 0312 first) or
(b) the pin was a CP whose scope contains 0312 (e.g. CP-03), or (c) the
plan entered via a non-reader path. The pin itself is not recoverable
post-hoc here (thin timeline, sessions in Supabase, runner now down) —
next live start must record the picked flow + CP doc BEFORE pressing
Start so the watchdog can bind the pin to the outcome.

### Update: the run executed SPRINT content, not tasks

Node-set proof (`steps-runtime` for run-640182 vs pack YAML):

- Run steps: `preflight_contract_plan/freeze, context, tdd, coder,
  validate, spec_align, reviewer, synthesis, synthesis_negotiation,
  audit` — byte-identical to `vibe-sprint.yaml`, zero overlap with
  `vibe-tasks.yaml` (`cp_reader, cp_validator, cp_lock,
  task_plan_reader`).
- The user's clone ("Vibe Tasks- Opencode") is therefore a copy OF
  VIBE-SPRINT wearing a tasks-like name. No `task_plan_reader` ever
  ran, so the CP-12 scoping/park arm never engaged — and the picked
  CP-12 doc could not pin anything: `SourceDocID → vibeCpDocID`
  pinning lives in `validateVibeCpIngestSource`, whose entry check
  (`isVibeCpSourcedFlowID`, vibe_cp.go:596 + runFirstTurnFences) keys
  off canonical ids only and skips clone UUIDs. Sprint path +
  empty `vibeCpDocID` → unscoped plan → byte-first Task-0312 (proven
  above). The picked CP-12 doc was silently inert for this run.
- Unblock without a tool fix: add a `- Parent Documents: CP-12` line
  to Task-121..124 and start via vibe-tasks (reader path), not the
  sprint clone.

### Full chain: why no run could reach Task-121 (verified per-link)

Two runs, three stacked defects — each link evidenced, no guessing:

1. **Wrong content (run-640182).** The "Vibe Tasks- Opencode" clone is
   SPRINT content (steps-runtime node set byte-identical to
   `vibe-sprint.yaml`, zero overlap with `vibe-tasks.yaml`). No reader
   node exists in it, so CP-12 scoping/park machinery never runs, and
   the picked CP-12 doc is inert (`SourceDocID → vibeCpDocID` pinning
   lives in `validateVibeCpIngestSource`, canonical-ids only — clone
   UUIDs skip it). Plan unscoped → byte-first Task-0312 (proven by
   replication). User-cancelled.
2. **Invisible tasks (both runs).** Even with CP-12 pinned, `collect*`
   returns `[]`: 121–124 use the `Parent Links` dialect the regex
   misses. (Fixed at data level by the metadata commit; tool fix
   deferred per mission.)
3. **Debate block (run-641691, tasks-family run).** Reader completed
   without parking (hence its pin was empty or non-CP-12 — a CP-12 pin
   through the reader arm would have parked, vibe_cp.go:1467), then the
   `suite_regressed` gate mounted the owner-debate overlay and never
   released: 7 owner legs, same card, parent timeline frozen (see
   BUG-663). No sprint could mount while the overlay holds the run.
   Cancel → resume left it idle with a pre-fix-era/empty plan; no HTTP
   endpoint rebuilds a plan for a live run, and nudges (`resume`,
   `continue`) move nothing. Terminal state for this run: start fresh.

### Current Ask

- Triage: confirm which pin/selection path picked Task-0312 (admission cpID vs fallback glob), then extend the collector to honor the `Parent Links` dialect (and audit writers emitting it). The watchdog only observes; it never repairs code.
