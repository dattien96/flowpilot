# Live-test defects captured — run-2062497 (CP-03 vibe-sprint × 7, PrivateVault)

Captured during live operation of the vibe-tasks flow. Each entry has evidence,
severity, and a suggested fix direction. For the post-run fix session.

**Status: all addressed** (fix session CA-1165 … CA-1173). D11 verified
non-defect. Per-item resolution marked inline.

## P0 — correctness

### D1. Premature sprint seal: hub_done seals sprint with implementation nodes SKIPPED
- Sprint 3 (Task-036): after `debate_synthesis` resolved a drift escalate, hub
  posted `done`; engine stamped `coder/validate/spec_align/reviewer` SKIPPED and
  `flow_run_complete_done` — while the oracle suite was RED and no CA existed.
- Evidence: diag 04:49:48 mass-settle; no CA file; task doc moved to done/.
- The `hub_done` path does not verify mid-nodes actually ran or that the
  workspace/oracle is green. Fail-closed required: a sprint must not seal with
  mandatory nodes never executed.
- **FIXED (CA-1165)**: engine-originated `done` now refuses to seal when
  `done`-edge spine nodes upstream were never dispatched (PENDING/WAITING step
  rows) — escalates to waiting instead of stamping SKIPPED.

### D2. Scope-drift gate cannot handle git submodules (unwinnable park)
- `FrozenContractScopeDrift` lists a submodule as ONE path (gitlink). Its
  `BaselineWorktree` fingerprint is `""` (directories un-fingerprintable) while
  the live fingerprint is a deterministic dir-hash → subtract never matches →
  drift fires on EVERY eval, forever ("no progress since last continue").
- `agent-loop/amend` rejects directory/submodule paths ("not a concrete code
  target") → no sanctioned way to widen scope → hard block.
- Evidence: repeated drift parks on `third_party/boringssl` (08:04, 08:20, 08:21,
  08:44) — resolved only by unstaging the gitlink + gitignoring the tree.
- Fix: exempt gitlink/submodule paths the same way `IsBinaryOrBuildArtifact`
  exempts build dirs, OR fingerprint gitlinks by recorded commit SHA.
- **FIXED (CA-1166)**: dirs fingerprint via embedded `git rev-parse HEAD`;
  baseline + gate now share one fingerprint function/format (baseline subtraction
  was dead code for ALL path types — 64-hex vs 32-hex); trailing-slash keys
  normalized. Dir amend → see D14.

### D3. Provider child dispatch fails on a `done` loop
- `spawn-agent` child runs created while parent loop status=`done` fail
  immediately: `leg_closed_reason: dispatch_failed`, no turns file, no provider
  work dir. Reproduced for BOTH devin and grok (run-2081893, run-2081899,
  run-2081905). Same call on a `running` loop dispatches fine (run-2081913).
- Fix: either allow dispatch on done loops for remediation legs, or fail fast
  with a typed reason instead of creating-then-failing the child.
- **FIXED (CA-1167)**: `spawnChildRun` refuses up-front on `done`/`stopped`
  loops (typed error naming the sealed loop + pointing at boundary Continue),
  before any child record exists.

## P1 — orchestration

### D4. Sprint boundary auto-advance never fires after audit/hub-done terminal
- After every task audit: `flow_run_complete_done` + `flow_control_done`, then
  silence — `maybeAutoAdvanceVibeSprintBoundary` never mounts the next sprint.
- Workaround verified ×3: `POST /workflow-runs/{id}/resume` re-derives the
  pending boundary gate (`maybeReparkVibeSprintBoundary` runs on resume/open
  paths only) → `gate-decision ok` mounts next sprint.
- Fix: invoke boundary re-park/auto-advance on the audit-settle path itself.
- **FIXED (CA-1167)**: root cause was subtler — auto-advance DID run at settle
  time but `hasRunningSprintStep` vetoed on `synthesis`, which is the settle
  ancestor `markFlowRunComplete` itself stamps DONE. The settle-edge direct
  predecessors are now excluded; non-ancestor RUNNING steps still veto.

### D5. Provider cancel does not propagate → zombie leg writes 23 min after cancel
- `run-2062497-tournament` marked `cancelled` at 02:56; its provider session kept
  running until ~20:19, writing files (incl. out-of-scope Task-037/038 files and
  a fabricated done-state) into the live workspace.
- Fix: linearize provider-session stop on the durable CAS cancel path — a
  cancelled leg must not keep writing.
- **FIXED (CA-1168)**: `abortDevinRemoteSession` sends ACP `session/cancel`
  (bounded 30s, off the Stop path) when a Devin leg terminalizes — from
  `stopAgentLoop` AND from reconstruct when a persisted-running leg resolves
  terminal (runner died mid-turn, remote backend session still alive).

### D6. Cohort verdict map not updated by human adjudication
- After a cohort split verdict is human-adjudicated, `hub_done` still sees the
  member's stale `blocked` verdict → "reviewer machine verdict missing or not
  approved" → repark. Recovery requires a member redrive
  (`verdict_deficient_member_redrive`) which DID work — but adjudication alone
  should record an override, or the card should say so.
- **FIXED (CA-1169)**: the adjudication park reason ("Cohort split needs human
  adjudication — REVIEWER: approved, SPEC_ALIGN: blocked", hub-authored prose)
  did not match `isReviewVerdictGateReason`, so Continue re-invoked the hub
  identically. Continue now also redrives when the park reason names a
  deficient cohort member; reasons naming no member keep the generic resume.

### D7. `renegotiate_signatures` outcome never reaches synthesis_negotiation
- Coder submitted a correct batched renegotiation (unsatisfiable locked-test
  defect, Task-037). The turn was retried instead of the batch being consumed by
  the `synthesis_negotiation` hub — deferred ~14 min, then applied as a generic
  `continue` back-edge. The batch rows never got an adjudication pass.
- Related: escalate→park "cancelled in-flight turns" discards mid-work turns
  (3 times for the Task-037 coder), re-dispatch starts cold each time.
- **FIXED (CA-1169)**: two root causes. (1) `signature_hash` froze as "" when
  the scaffold turn wrote only test files (prod stubs pre-existed) →
  `isSignatureLockedCoderChild` false → coder was never offered
  `submit_review_outcome` and could not file the typed outcome; a prod-signature-
  empty fallback now pins a hash over written+declared paths. (2) the signature
  batch was keyed by `parent.stepID`, which re-stamps per turn — read-side now
  flattens all buffered keys.

### D8. `zero_delta_progress` punishes completed work
- Coder's turn produced zero file delta *because the work was already done* →
  detector scored 100 → pause_for_human → reprompt loop. Completion state
  should exempt zero-delta (check outcome/green-suite first).
- **FIXED (CA-1170)**: `TurnSummary.TestsGreen` (suite ran, ≥1 pass, no
  failures/regressions) exempts zero-delta; no-green-suite still fires.

## P2 — planner / contract quality

### D9. Frozen contracts systematically omit CMakeLists.txt for new native tests
- Every sprint contract declares new `*_test.cpp` files but never declares
  `CMakeLists.txt` → test files can never be wired into the suite the oracle
  runs → `r-scaffold-red` cannot see red (4/4 sprints needed operator wiring +
  amend). Planner must include the build-graph file when it declares a new
  test target. (Task-031 planner DID declare it — regression since.)
- **FIXED (CA-1171)**: contract-planner pack prompt now requires declaring the
  build-graph file alongside every new test file.

### D10. Post-freeze operator writes count as leg drift
- Operator CMake wiring between freeze and leg-settle is attributed to the leg
  → drift park. Mitigated by `amend`, but the gate should distinguish
  writer-identity or accept an "operator-authored" mark.
- **PARTIALLY FIXED (CA-1170)**: true writer-identity is not computable from
  the diff (a leg bash redirect and an operator edit look identical), so the
  gate still fails closed. The reason now names drifted paths that bypassed
  the leg's tool calls and points at amend — the operator-visible symptom
  (hunting for a rogue leg write) is resolved.

### D11. Diag ndjson stops flushing while legs still run
- run-2062497.ndjson last entry 09:22:43 while `run-2095397` reviewer kept
  running and new cohort pairs spawned — agents API shows events the diag
  never recorded. Event sink gap.
- **NON-DEFECT (verified)**: the diag file is complete — 09:22:43→09:32:30 was
  a single 10-minute reviewer turn; diag records orchestration events, not
  turn chatter, by design. No gap.

## P3 — UX

### D12. Sub-agent chat view loads entire history — heavy lag on long runs
- Opening a coder/reviewer leg renders the full turn history (Task-037 coder:
  6+ turns, each with a full transcript) → UI freezes.
- Expected: lazy-load latest ~10–15 responses initially + "load previous"
  button to page older turns (same pattern as the main chat).
- Evidence: user report — opening coder sub-agent near-unusable on this run.
- **FIXED (CA-1172)**: focus now replays the tail + cursor like the main chat
  with a load-earlier affordance, and `chatRowsSig` hashes only the rendered
  window instead of every message every frame.

### D13. Drift escalate card buttons unresponsive in UI
- User clicked the drift decision card options and nothing happened (had to be
  resolved via `agent-loop/continue` + `amend` API directly).
- **FIXED (CA-1172)**: root cause was [Allow] → batch 422 on the unamendable
  dir entry (silent dead click). Amend now partitions — amends the amendable,
  reports `unamendable_paths` — and failures surface as transcript warnings
  instead of flipping the connection status. See also D14: dirs are now
  amendable when they exist on disk.

### D14. Amend rejects directory paths entirely
- `agent-loop/amend` needs a directory-tree widening mode (or explicit
  submodule support) — see D2.
- **FIXED (CA-1166)**: extension-less paths naming an existing directory
  (submodule gitlink, untracked-dir row) are amendable via the shared
  `IsAmendableDriftPath` predicate → `AllowedExtraPaths`, exact-matching the
  diff row the gate reports. Files without extensions (Makefile) and
  nonexistent buckets stay rejected per CA-427 Finding 5.

## Confirmed-good behaviors observed (do not regress)

- `agent-loop/amend` widen+auto-resume path works (used 4×).
- `orphaned_wait_healed`: bare WAITING without live card self-heals to running.
- Drift self-correction ladder: leg self-amended `.clang-format` into scope in
  47 s (designed behavior — document it).
- `verdict_deficient_member_redrive` recovers missing cohort verdicts.
- Review loop is genuinely rigorous (5 rounds on Task-037, found real test
  weaknesses, AEAD/mutex ordering issues).
- Bounded reprompt on missing machine verdicts works.
