# CP Full Live Test — Consolidated Runbook (Core CPs)

## Metadata

- Document ID: `CP-FULL-LIVE-TEST`
- Title: `Full live test — harness/vibe core + state-durable CPs on post-rebase HEAD`
- Phase: `07-Coding-Plan` (verification runbook — executable top-to-bottom)
- Status: `todo` (fill ☐→☑/⚠/✗ + runId per row while executing)
- Owner: `FlowPilot`
- Created: 2026-09-24
- Covers: **CP-23, 35, 37, 41, 43, 51, 54, 55, 58, 59, 60, 61, 62, 63, 64, 65, 66, 67**
- Supersedes (execution-wise): `CP-Live-Test-Orchestration-Plan.md` §8 waves W-1..W-5 for the in-scope CPs
- Source checklists: per-CP `*-Test-Steps.md` in `done/` + `todo/CP-67-Test-Steps.md`; results ledger `done/CP-Test-Progress-Tracking.md`

## Why this file exists

Two waves already ran: original manual passes (Sept) and the W-1..W-7 live wave that
found 64+ bugs (all fixed in the BUG-374..454 wave, clusters A–V, each with red
tests + CA). **But** (a) the fix wave ran on the pre-merge HEAD — main has since
landed CP-81/82/83/84 (event-plane mux, sessions monitor, embedded terminal,
windowed timeline) touching `internal/runner` + `internal/tui`; (b) several live
legs were deferred/BLOCKED (multi-provider tournament merge, vibe resume matrix,
reject-path live legs, UI cards); (c) the 9 vibe/gate bugs that looked open in
`09-BugFix/todo/` were in fact already fixed (CA-817..832) — docs moved to done,
their live symptoms stay here as RE-VERIFY rows.
This runbook re-verifies the whole core on the post-rebase HEAD and makes the
deferred cases explicit.

---

## 0. Automated gate — run FIRST (must be green or matches BUG-454 baseline)

```bash
cd apps/local-runner
go build ./...
go test -count=1 ./internal/...
```

Expected reds = **only** the documented set (BUG-454 §2/§3):
6 env-waived (`TestDetectProvidersPopulatesInventoryShape`,
`TestResolveGoogleDriveMcpProviderStatuses_AllNotStarted`,
`TestCleanupSessionsTearsDownProviderPools`, `TestFirebaseToolsMcpAdapterFetchEndToEnd`,
`TestGitNexusDependentsSmokeScopeDiff`, `TestCatalogStoreForFallsBackToFake` + same-class
`TestFlowDefinitionStoreForUnconfiguredRunnerYieldsNil`) and the TempDir-cleanup flake
family (pass isolated). **Any other red = regression → STOP, file BUG.**

Focused suites per area (fast loop while a section is being verified):

```bash
go test -count=1 ./internal/runner/ -run 'TestCP61HubDone|TestCP53ReviewDoneVerdict|TestFlowRequiresSynthesisMachineVerdict'
go test -count=1 ./internal/runner/ -run 'TestReviewACCoverage|TestHandoffEnrichment|TestSprintHandoff'
go test -count=1 ./internal/runner/ -run 'TestBug374JSONRPC|TestBug381_|TestBug386|TestBug387|TestBug388|TestBug389|TestBug390|TestBug391|TestBug398'
go test -count=1 ./internal/runner/ -run 'TestBug399|TestBug40[1-9]|TestBug41[0-4]|TestBug424|TestBug432|TestBug437'
go test -count=1 ./internal/runner/ -run 'TestBug44[6-9]|TestBug45[0-4]|TestBug446_453|TestTournamentEscalation|TestReviewLoopTriggersTournament|TestTournamentTieRequiresHumanDecision|TestResumeParentAfterTournament'
go test -count=1 ./internal/flowgate/ ./internal/changecontract/ ./internal/agentpack/ ./internal/tournament/ ./internal/worktree/ ./internal/lsp/ ./internal/knowledge/
```

## 1. Preflight

| # | Item | How | Status |
|---|------|-----|--------|
| P-1 | HEAD = post-rebase `cp_live_test` (contains main CP-81..84) | `git merge-base --is-ancestor origin/main HEAD` → exit 0 | ☑ PASS (HEAD 6086acfd, includes CA-950/951) |
| P-2 | Runner builds | `cd apps/local-runner && go build ./...` | ☑ PASS (binary rebuilt 2026-09-24) |
| P-3 | Bed: clean git repo w/ Go module (e.g. `~/fp-beds/full`); **not** `/tmp`, not this repo | `git -C <bed> status` clean-ish | ☑ PASS (`~/fp-beds/full`, module `livebed`, committed) |
| P-4 | Runner up on a fresh port | `go run ./cmd/flowpilot --port 19400` (or `just runner-dev`); `GET /health` 200 | ☑ PASS (pid 44253, `/health` online, instance `instance_0612bdce`, gen 1) |
| P-5 | Providers: `devin` (swe-2-max) + `opencode` reachable; `grok` optional (402-prone) | `/providers` catalog | ☑ PASS w/ caveat — `/client/provider-accounts`: codex✓ devin✓ gemini✓ grok✓ opencode✓ connected; **claude absent** (no account on this machine — tournament 2-provider merge limited to devin+opencode/codex/gemini/grok) |
| P-6 | Bed bound as project | `POST /client/projects` → `.flowpilot/engine-init.json` + skills installed | ☑ PASS (project `957928cc-1f80-43ce-a7e8-2cf2ebb36595` "lt-full"; `.flowpilot/{canonical,catalog,ledger,guard,settings,structure}` + `.agents/skills/` installed) |
| P-7 | Log capture | `runner.log` tail + `dispatch.ndjson` + `run-*-turns.ndjson` under `.flowpilot/` | ☑ PASS (`~/Library/Application Support/FlowPilot/logs/runner.log`; per-project `.flowpilot/chats/957928cc*/`) |

**§0 automated gate evidence (2026-09-24, HEAD 6086acfd):** `go build ./...` clean;
`go test -count=1 ./internal/...` → reds = 9, all BUG-454 baseline class: env-waived
(`TestDetectProvidersPopulatesInventoryShape`, `TestResolveGoogleDriveMcpProviderStatuses_AllNotStarted`,
`TestCleanupSessionsTearsDownProviderPools`, `TestFirebaseToolsMcpAdapterFetchEndToEnd`,
`TestCatalogStoreForFallsBackToFake`, `TestFlowDefinitionStoreForUnconfiguredRunnerYieldsNil` —
last one verified identical on clean `origin/main`) + TempDir flake family
(`TestBug414_TieCardCandidateChoiceMerges`, `TestFlowCodingPromptSpawnWrappedDoesNotDuplicateHistory`,
`TestStartTurnGrokCrossAccountLegacyThreadPromotesCopiesAndLoads` — all PASS isolated).
No new reds → no post-rebase regression. ✅ GATE PASS.

**Live findings so far (2026-09-24, bed `~/fp-beds/full`, project `957928cc`):**

- **run-1 (opencode/gemini leg)**: `gemini-3-flash` free-tier quota exhausted →
  child `turn_failed` surfaced full "usage limit" detail (not flattened to
  "Internal error" — BUG-374/381 verified live) → hub `submit_review_outcome
  {blocked}` → fail-closed park `awaiting_user` → operator Continue routed
  reprompt turn-83 to the correct node. ✅ quota classification + fail-closed
  + BUG-411 gate-decision→resume all live-verified.
- **run-94 (devin/swe-2-high, task-harness)**: full plan loop live —
  `preflight_contract_plan`→`context`→`plan_writer`→`plan_reviewer`
  (`changes_requested` round 1) → writer re-entry same child → `approved`
  with openIssues → **Task-325 plan-approval park fired** (churned plan) →
  operator bare-Continue approved → `preflight_contract_freeze` +
  `test_signatures` + `implement` DONE (real `calc.go`/`calc_test.go` written,
  tests-before-impl ordering observed) → `validate` escalated
  `skipped_no_command` → **BUG-455 found**: run created without `cwd` left
  `workspaceCwd=""` → `loadValidateCommand("")` never reads the baseline →
  dead park (both documented resolutions were no-ops). Fixed via CA-952
  (create-time default + resume heal; 5 additive tests). Runner restart
  confirmed the heal (`workingDirectory` now resolves) but the parked run
  normalizes to `cancelled` on reconstruct — parked-at-gate runs do NOT
  survive runner restart; noted as observation, flagged for review whether
  parked-but-resumable should persist.
- **run-6893 (devin/swe-2-high, task-harness, explicit `cwd`) — FIRST FULL
  HAPPY-PATH COMPLETION**: `preflight_contract_plan`→`context`→`plan_writer`→
  `plan_reviewer`→`plan_synthesis`→`preflight_contract_freeze` DONE first pass
  (no churn → no Task-325 park) → `test_signatures` wrote `strutil.go` stubs
  (`panic("not implemented")`) + `strutil_test.go` RED tests (failures
  confirmed) → **scope-drift gate fired** on bare-root `livebed` binary from
  the child's `go build` → park → operator deleted artifact + Continued with
  steer → gate passed → `implement` filled bodies only (rune-aware Reverse,
  letter-normalizing IsPalindrome — signatures preserved, CP-67 lock held) →
  implement hit the SAME binary park once (child re-ran `go build`) → resolved
  identically → `validate` ran real `go test ./...` against baseline → green
  DONE → `reviewer` (grok-4.5, cross-provider cohort) approved → `synthesis`
  DONE → `synthesis_negotiation` SKIPPED → `audit` tier-3 blocked on missing
  `change-audit/CA-*.md` (r-ca backstop live-verified) → operator supplied
  CA-001 → re-observe → `audit` DONE 03:11:01Z — `flow_audit_draft` persisted
  (featureKey `str-utils`). **BUG-456 found live**: build artifacts counted as
  frozen-scope drift (CA-427 F2 dropped the IsBinaryOrBuildArtifact clause) —
  fixed via CA-953, 3 additive tests.
- **run-13080 (bug-harness, devin)**: reproduce_test wrote
  `strutil_combining_test.go` (RED confirmed FAIL on combining-mark bug) →
  implement wrote grapheme-cluster fix + own CA note → implement parked on
  **BUG-457**: the runner's own `.flowpilot/logs/**` diag output counted as
  scope drift (append-only mid-turn write → fingerprint subtraction never
  matches → dead park). Fixed via CA-954; run cancelled on runner restart
  (same parked-run observation as run-94).
- **run-14071 (bug-harness, devin impl / grok review) — CLEAN FULL PASS on
  fixed binary**: injected RI-flag bug (regional-indicator clause removed
  from `graphemeExtends`; baseline green, flag reversal scrambles) →
  `reproduce_test` wrote `strutil_flag_test.go` (9 cases incl. odd-RI
  singletons — RED confirmed FAIL) → `implement` restored RI clause + wrote
  `CA-002-str-utils.md` itself → `validate` green → `reviewer` grok approved
  → `synthesis` → `audit` DONE 03:47:01Z. **Zero parks** — BUG-456/457 fixes
  verified live (no artifact/diag-log drift). CP-64 RED→GREEN chain fully
  exercised end-to-end.
- **run-16653 (tournament-harness, codex gpt-5.4 hub) — parity observation**:
  Flow-Mode launch via `workflowId: <canonical flowRef>` works (BUG-426
  flow-steps-from-definition path; `runKind:workflow`, steps seeded from
  nodes). `problem_scout` delegate completed → hub synthesis turn on codex
  finished in prose *twice* without calling `flowpilot_submit_review_outcome`
  even though the DynamicToolSpec was advertised (`OfferReviewOutcomeTool`
  gate true — BUG-226 escalate fired correctly both times, loopState
  blocked + "(no progress since last continue)"). Devin hub calls the tool
  reliably; codex gpt-5.4 hub ignored it — watch whether this recurs
  (possible prompt/tool-name drift: prompt says `submit_review_outcome`,
  codex tool is `flowpilot_submit_review_outcome`; devin's MCP name is
  `mcp__flowpilot__submit_review_outcome`).
- **run-16693 (tournament-harness, devin hub) → BUG-458 (found live)**:
  scout completed but `parallel_rollout` never dispatched — diag showed
  `flow_advance_target_not_spawnable behavior=""`. Root cause: the active
  definition came from the **Supabase mirror** (service-role key in
  keychain → `FlowDefinitionStoreFor` non-nil), and
  `recordFromWorkflowRow`/`upsertNodeStepDefinitions` never round-trip
  `run`/`posture`/`contextProfile`/`config` (no `step_definitions`
  columns). `parallel_rollout` is behaviorless — identified ONLY by
  `run: inline` — so the rollout passthrough rejected it. Verified against
  Supabase directly (mirror row `behavior_id=null`, no run field) while a
  probe of the embedded pack returned `passthrough ok=true`.
  **Fixed (CA-955, db027d8e)**: builtin mirrors restore pack-declared
  `run`/`posture`/`context_profile`/`config` from the embedded pack; all
  rows derive `run` from behavior when still empty (`agent.*`→delegate,
  else inline). 3 additive tests.
- **⚠ Corollary (BUG-458)**: every earlier mirrored-flow live run
  (run-6893, run-13080, run-14071 — all resolved via mirror) executed with
  `posture`/`contextProfile`/`config` silently dropped. Concrete impact:
  bug-harness `posture: read_only` nodes ran unrestricted, and tournament
  `max_attempts: 2` degraded to the 1 default. Posture-enforcement and
  retry-cap live rows must be re-verified on the CA-955 build.
- **run-19067 (post-CA-955, candidates claude+codex)**: full topology
  verified live — scout DONE → `parallel_rollout` passthrough spawned both
  candidates in isolated worktrees → `join:all` held → arbiter round-1
  `retry` → fresh cohort → round-2 tie escalate → human picked
  `candidate-b` → routed to merge → escalate "no mergeable diff"
  (codex-mini wrote nothing). claude unavailable → typed fail-closed
  candidate failure, counted by the barrier (not skipped).
- **run-20041 (candidates grok+codex via admin model override) → BUG-459
  (found live)**: grok candidate-a wrote a real green `Mul`+`TestMul`+CA
  note; codex candidate-b's worktree stayed clean. Arbiter auto-picked
  **candidate-b** — an untouched baseline out-scored the real fix on the
  20% blast-radius leg. `merge_and_audit` then reported "merged" while
  applying nothing: the arbiter's `done` branch never stashed
  `rs.tournamentPatches` (escalate-only), so merge took the
  live-worktree path where `Apply` treats an empty diff as a successful
  empty win. **Fixed (CA-956)**: snapshots stash on every verdict →
  empty winner patch escalates explicitly (`reason: empty_patch`), winner
  worktree preserved for the human to pick another candidate's recorded
  patch. RED test `TestBug459AutoPickedEmptyWinnerPatchEscalates`.
- **run-21364 (post-CA-956) → BUG-460 (found live)**: grok candidate-a
  escalated on Change-Contract reprompt cap (continued via feedback, then
  completed with a real patch); codex candidate-b no-op'd again — but its
  "empty" worktree carried `AGENTS.md`/`CLAUDE.md` GitNexus header stamps
  (`indexed as candidate-candidate-b`) written by `ensureGitNexusIndexAsync`
  + knowledge bootstrap running *inside the managed worktree*. The noise
  diff won the tie-break, merged into main, and rewrote main's index
  headers. **Fixed (CA-957)**: `isRunnerManagedWorktreePath` guard skips
  auto-index + knowledge distillation on `.flowpilot/worktrees/` paths;
  normal workspaces unaffected. RED test
  `TestBug460AutoIndexSkipsManagedWorktree` (proven red: guard removed →
  auto-index fired on the worktree path).
- **run-23455 (post-CA-957, candidates grok+codex)**: BUG-459 escalate path
  verified live — arbiter auto-picked candidate-b (empty diff again) →
  `merge_and_audit` parked `WAITING_USER_APPROVAL` with
  `flow_control_escalate: "tournament winner candidate-b produced no
  mergeable diff"` instead of the silent no-op "merged". **Open
  observation**: the merge-stage escalate card offers no way to select the
  *other* candidate's stashed snapshot — when an auto-picked winner's patch
  is empty, the losing candidate's non-empty patch is unreachable from
  that card and the run strand-parks (operator must abandon the run).
  Candidate-vs-merge decision ownership may warrant a dedicated card kind
  (options = each recorded patch + retry + discard).
- **run-25153 (post-CA-957, candidates grok+devin via admin model
  override `step_definitions.model` = grok-4.5 / devin/swe-2-high) —
  A-65-1 FULL PASS**: scout (devin) discovered a real defect
  (`graphemeClusters` splitting CR×LF — UAX #29 GB3/GB4/GB5 violation) →
  `parallel_rollout` fanned out → **3 full rounds** of isolated-worktree
  cohorts, all six candidate children completing with real patches →
  arbiter verdict `retry` twice (tie at total 1.0000 both rounds —
  `max_attempts` restore from CA-955 exercised) → round-3 tie → escalate
  decision card parked → human picked `candidate-a` → `merge_and_audit`
  DONE and the patch **actually landed**: `strutil.go` gained
  `graphemeBreak`/`isGraphemeControl` (GB3 CR×LF + GB4/GB5 control
  ordering), `strutil_crlf_test.go` (12 cases), `change-audit/CA-004.md`.
  `go test ./... -count=1` green, `go vet` clean. No `AGENTS.md`/
  `CLAUDE.md`/`.gitnexus`/knowledge writes in the merged diff (BUG-460
  guard verified live). `.flowpilot` bookkeeping excluded from the patch
  (`:(exclude).flowpilot` in `manager.go` Diff) — only the worktree-root
  `.gitignore` line exists, written by the manager's ensure, not by the
  merge. Worktrees cleaned post-merge, no orphans.
- **Provider observations**: codex `gpt-5.4-mini` completed candidate
  turns in <1s/7s with prose-only responses and zero file writes on four
  consecutive runs — candidate-prompt + template were confirmed injected;
  this is provider behavior, not a dispatch defect. Grok honored the gate
  reprompt (added the required CA note on turn 2). Claude remains
  unconnected; Devin free-tier rate-limited when both candidates inherited
  the hub model pre-CA-955.
- **Dispatch durability (CP-51)**: `prepared → send_claimed → send_started →
  terminal_completed` chain with envelope hashes observed on every provider
  turn across run-1/run-94/run-6893/run-14071 records.
- **run-34947 (vibe-ingest snake, devin hub) — ingest→slicer PASS, sprint
  start exposed BUG-461**: full intake chain live —
  `ingest_reader` → `ss_converter` → `ss_validator` (APPROVE) → `ss_lock`
  parked card → human `continue` unlocked → `cp_writer` spawned. First
  cp_writer child (devin) died on the free-tier rate limit
  (`retryable: unavailable`, 10s, 0 writes) — `writer_fail_skips_validator_hub`
  correctly skipped the validator reinvoke but left the run `running` with
  nothing scheduled (dead-end shape, same class as the merge-card
  observation: a linear-writer failure surfaces no card). Runner restart →
  resume parked `blocked` + pendingGate `resumeFrom: ss_lock` →
  `gate-decision ok` re-drove cp_writer on **grok/grok-4.5** via the
  `step_definitions.model` admin override (mid-flight provider override
  verified live) → CP-01 finalized → `task_slicer` wrote Task-3/4/5. Sprint
  start then failed at flow resolution: mirrored `vibe-sprint` dropped the
  flow-level `contextProfiles` map so the BUG-458-restored
  `contextProfile: scout` node ref failed validation — **BUG-461, fixed
  CA-958** (restore `ContextProfiles`/`Tools` from the embedded pack for
  builtin mirrors). A second restart during the stranded sprint-start
  cancelled the run (same CP-66 parked/in-flight-run cancel gap — no
  pending gate existed to re-drive).
- **Stale-task pollution observation**: `collectVibeTaskPlan` globs every
  `requirements/08-Task/todo/Task-*.md` regardless of feature lineage —
  leftover draft Tasks from earlier bed runs (Task-1 gcd/lcm, Task-2
  strutil) would have entered the snake sprint plan ahead of Task-3.
  Quarantined to `/tmp/fp-bed-task-backup/` for clean evidence; whether
  cross-feature drafts should join a new CP's sprint plan is a design
  question worth a follow-up (plan = "all pending work" vs "this CP's
  children only").
- **BUG-365 live-verified**: after `ss_lock` confirm, all three SS docs on
  disk carry `Status: approved` (SS-01/02/03-snake-*).
- **A-60-7 live-verified**: a vibe-cp-ingest turn without a CP source
  rejected with `422 invalid_cp_source` (BUG-399 fail-closed admission);
  adding `sourceDocId` armed the flow.
- **run-37268 (vibe-cp-ingest snake) — Task-3 sprint COMPLETE through the
  full harness chain; BUG-462 found + fixed live**: cp_reader → cp_validator
  (Devin `submit_review_outcome` approved) → cp_lock parked → `continue`
  confirmed → task_slicer → vibe-sprint armed Task-3 →
  `preflight_contract_plan` → `preflight_contract_freeze` → `context` →
  `tdd` all DONE; scaffold locked `snake/model_test.go` read-only + pinned
  `signature_hash`+10 `locked_signatures` on the coder contract (v2).
  tdd→coder advance then false-parked: `vibe_tdd_missing — coder refused;
  tdd artifact missing` fired 70ms after the signature pin — the FS-only
  `hasVibeTddOutput` could not see the adopted full-body test file and never
  consulted the contract record it was about to open anyway. **BUG-462,
  fixed CA-959**: `vibeTddEvidencePresent` counts contract-pinned
  `LockedSignatures` at all three gate sites (advance + both resume paths);
  3 additive tests incl. guard-rail (bare v1 freeze still parks; CA-769
  full-body rejection preserved). Live verify post-fix: runner restart →
  `gate-decision ok` on `resumeFrom: tdd` → coder spawned (run-40625,
  devin/swe-2-high) → `coder`/`validate`/`synthesis`/`audit` all DONE →
  sprint boundary card `sprint 2/3 (Task-4-snake-loop-input-render.md)`
  parked (boundary checkpoint OK) → `ok` armed sprint 2. Bed: `go test
  ./snake/...` green (0.278s).
- **BUG-462 side observations**: (a) `agent-loop/continue` answered a
  `vibeResumeConfirm` gate with a fresh hub turn instead of consuming the
  card — only `gate-decision` routes it (same wrong-surface class as the
  run-34947 ss_lock gate miss); (b) `steps-runtime` briefly showed
  `synthesis RUNNING` while `coder` was still PENDING after the restart —
  stale step view, settled to DONE on completion; (c) boot-time builtin
  mirror sync hit `409 duplicate workflow_steps(workflow_id,order_index)`
  on `bug-plan-harness` — insert-not-upsert on an existing row set; sync
  failure is logged and non-fatal but worth a fix pass (mirror freshness).
- **run-37268 sprint-2 (Task-4) — BUG-463 found + fixed; run cancelled by
  restart; continued as run-41626**: sprint-2 minted a fresh contract (v4,
  `coder` step — A-60-5 re-verified: no reuse of Task-3's record) → tdd
  DONE → scaffold locked test files but the pin logged an **empty
  signature hash** and stored **absolute** `read_only_paths`. Root cause:
  devin's `EventFileChanged` reported absolute paths this turn (sprint-1
  reported relative — provider shape varies); `scaffoldSignatureSnapshot`
  `filepath.Join(cwd, abs)` never existed → empty sigs, and
  `recordScaffoldArtifactsLock` bypassed the BUG-388 relativize wrapper so
  abs paths persisted (read-only deny-list would never have matched).
  **BUG-463, fixed CA-960**: normalize `EventFileChanged` paths at
  ingestion + relativize `written` at the scaffold gate; contract evidence
  now also counts `ReadOnlyPaths` (a post-freeze lock is itself the
  runner's TDD attestation — covers the already-locked sigs-empty record).
  Runner restart then cancelled the parked run (known CP-66 observation,
  re-confirmed); Task-3 file archived to `08-Task/done/` per convention
  (runner had ticked all DoD boxes) and **run-41626** launched via
  `vibe-cp-ingest` @CP-01 — cp_reader correctly read "Task-3 done,
  Task-4/5 pending" → chain re-armed toward slicer → sprint Task-4.
- **run-41626 (vibe-cp-ingest resume) — Snake MVP COMPLETE via the
  agent-orchestrated sprint path**: vibe-intake child (run-41631) read the
  bed state, correctly identified "Task-3 done / Task-4 stubbed mid-TDD /
  Task-5 not started", and self-orchestrated two sequential `vibe-sprint`
  children (`spawn_agent wait=true`): run-42033 (Task-4 — implemented
  input/loop/render stubs → green, 4.5min/1107 events) and run-43155
  (Task-5 — score/game-over/R-Q, 7min/1187 events). Verified end state:
  `go test -count=1 ./...` green, `go vet` clean, Task-4/5 docs moved to
  `done/` with §11 filled, handoff-sprint-2/3.yaml + CA-006/007 written,
  `snake/cmd` binary renders grid+snake+food and quits on `q`. Intake's
  final report correctly refused to mark CP-01 done (interactive-terminal
  DoD needs human playthrough; cooked-mode stdin caveat documented).
  Engine-side flow then proceeded cp_lock (confirmed `ok`) → task_slicer
  (doc-writer found nothing to slice) → **parked
  `flow_parked_awaiting_user`** — see observations.
- **Two sprint execution modes observed for the same vibe entry**:
  run-37268 ran sprints as ENGINE-DRIVEN flow nodes
  (preflight_contract_plan → freeze → context → tdd → coder → validate →
  synthesis → audit, with contract freeze + signature lock + read-only
  enforcement); run-41626's intake agent instead spawned plain
  `vibe-sprint` agent children — single-turn devin runs with NO frozen
  contract minted and no TDD-signature/read-only gate enforcement (test
  integrity rests on prompt instruction only). Both produced correct
  artifacts here, but the contract machinery coverage differs — design
  question: should `spawn_agent("vibe-sprint")` resolve to the flow
  instead of a bare role child?
- **Requirement-park UX gap at plan exhaustion**: when every Task doc is
  already in `done/`, the slicer-done gate (BUG-363) sees empty `todo/`
  and parks "refusing to sprint from fallback" — correct fail-closed, but
  indistinguishable from a genuine slicer failure; a `continue` turn is
  absorbed (`hub_reinvoke_skipped_vibe_lock_sealed`) with no card options,
  so the only operator close is stop/cancel. Worth a "plan exhausted →
  done" terminal distinction follow-up.
- **Doc-scope gate on a plain chat turn has real teeth**: a `status?`
  question on run-46465 (post-cancel follow-up) tripped `r-task` because
  the reply referenced Task ids whose docs were deleted — the reprompt
  turn then *recreated* Task-1/Task-2 in `done/` from git history. Correct
  per BUG-152 (normal chat keeps the full rule set) and the recovery was
  accurate, but a read-only-looking question produced writes — worth
  noting for chat posture expectations.

Evidence convention: every row gets runId + log line / artifact path. UI-only rows are
marked `UI` — backend evidence still required where noted.

---

## A. Harness & Vibe core (nhánh 1)

### A-1 CP-58 — bug/task/cp harness review loops — `feature_key: agent-flow-engine`

| ID | Case | Steps → Pass criteria | Auto cover | Status |
|----|------|-----------------------|------------|--------|
| A-58-1 | task-harness happy path | `/flow task-harness` + GCD-style prompt (calc-core) → scout→context→plan_writer→plan_reviewer→plan_synthesis→freeze→test_signatures→implement→validate→reviewer→synthesis→audit→done. Writer/reviewer prompts contain "Templated file outputs" / "Bound input artifacts"; reviewer calls `submit_review_outcome` | e2e in runner suite | ☑ run-6893: full chain DONE→audit 03:11:01Z; audit draft persisted (str-utils). run-94: through `implement`, validate dead-parked → BUG-455 (fixed, CA-952) |
| A-58-2 | Plan loop reject→re-entry same session | Force `changes_requested` (ask plan to include benchmark) → `flow_control_hub_done_continue_on_review_verdict`, writer re-enters **same** child; context/freeze stay DONE | `TestCP61HubDone/plan_synthesis_changes_requested_continues` | ☑ run-94: reviewer `changes_requested` → `plan_writer` re-entered on same child run-235 (rounds 2–4), `context`/freeze untouched |
| A-58-3 | Code loop isolated from plan loop | Force code reject → implement re-entry; `plan_*` + freeze remain DONE | unit matrix | ☑ unit `TestE2EReviewLoopMultiRoundChangesThenApprovedCompletes` (synthesis continue → coder re-entry, loop completes on round-2 approve) + `TestCP61HubDone` cohort matrix — green |
| A-58-4 | Round cap 3 → escalate | Force 3 rejects → `blocked`/escalate card, no 4th round (was never forced live) | `agent_orchestrator_test.go` cap | ☑ unit `TestAgentOrchestratorRoundCapTerminates` green; live leg deferred (needs 3 real rejects) |
| A-58-5 | cp-harness slice-only | `/flow cp-harness` → `cp_plan_writer→cp_reviewer→cp_synthesis→task_splitter→audit→done`; exactly N Task files, additive, no implement nodes | `bug356_slice_audit_test.go` | ☑ unit `TestBug356_*` (slice-only diff → audit passes via slice-outputs verification; no validate node required) — green |
| A-58-6 | cp-harness-smoke (clone) | Clone → run: continues through implement chain; `acceptance_nodes` preserved | — | ◑ start attempt live: stored clone workflow refused `flow is not startable` (fail-closed, no corrupt execution). Legacy stored fixtures (`rag-harness (history-only test)`, `Review Loop (clone)`) also fail closed on stale definitions — unknown context source / bad entry node — validation works, no silent run. Fresh pack flows unaffected (run-81618/86157 task-harness full chain DONE). |
| A-58-7 | Regression canary | `rag-harness` + `review-loop` behave as pre-CP-58 | F3 test cmds §F | ☑ post-rebase flows live-verified: run-6893 task-harness full chain DONE→audit, run-37268 vibe-ingest, run-81618 task-harness full chain DONE→audit (grok), run-86157 task-harness full chain incl. amend→v2→resume→audit DONE (grok), run-71117 context.produce + ranked history correct — no canary regression observed; run-100134 cp-harness full chain (contract→context→cp_plan_writer→cp_reviewer approve→synthesis→task_splitter Q-1 surface→audit) on b469 — no canary regression observed; dedicated rag-harness/review-loop re-runs IMPOSSIBLE BY DESIGN — both carry `selectableIn: []` (clone-only templates, hidden from /flow picker); live launch attempt run-116450 rejected `invalid_flow_ref` fail-closed as intended. Canary coverage via post-rebase harness chains stands |

### A-2 CP-61 — done-verdict gate (machine PASS, not self-grade)

| ID | Case | Pass criteria | Auto | Status |
|----|------|---------------|------|--------|
| A-61-1 | All 3 hubs gate on cohort verdict | task-harness live run: `plan_synthesis` needs `plan_reviewer` PASS; `synthesis` needs `reviewer` PASS → audit; wrong-cohort PASS doesn't unlock | `TestCP61HubDone` 13×3 providers | ☑ run-6893: `synthesis` consumed grok reviewer approved-verdict → audit; run-94: plan_synthesis consumed devin plan_reviewer arc |
| A-61-2 | Missing verdict → escalate | `flow_control_rejected_missing_review_verdict`; never freeze/audit | unit | ☑ LIVE run-71117: `cohort_member_verdict_reprompt` (attempt 2 on plan_reviewer child run-74607) → still no machine verdict → `flow_control_rejected_missing_review_verdict` → `flow_control_escalate` → `WAITING_USER_APPROVAL`, freeze/audit never reached; unit `TestCohortMemberMissingVerdictReprompts`+`…RecordedVerdictSkipsReprompt` green |
| A-61-3 | cp-harness reject path live | cp_reviewer `changes_requested` → writer re-entry (deferred from M-wave) | unit 2.4 | ☑ **LIVE run-112767** (earlier ◑ run-100134 superseded): baited reject via unregistered `feature_key: snake-netplay` (absent from `change-audit/FEATURE-KEYS.md`) → `cp_reviewer` round 1 `changes_requested` (AC-7 fail: feature key not registered) → `cp_synthesis` continue back-edge reset reviewer/synthesis/splitter/audit to PENDING → `cp_plan_writer` re-entered → revised CP-06 added feature-key-registration DOD-2 + risk R-4 + open Q-1 → `cp_reviewer` round 2 **approved** → `cp_synthesis` done → `task_splitter` started. Full reject→re-entry→revise→approve loop verified. Audit escalate leg also verified earlier (run-100134): tier-3 gate `code changed without declared Change Contract` → `waiting_question` + audit WAITING_USER_APPROVAL (fail-closed correct) |
| A-61-4 | Non-harness chat unaffected | plain chat → no hub events | `normal_chat_unaffected` | ☑ unit `TestCP53ReviewDoneVerdictNormalChatUnaffected` + `TestHTTPStart_NormalChatUnaffected` green; live: run-70937/70934 plain chat turns produced zero hub/flow events |

### A-3 CP-62 — ZCode parity (verdict schema, AC coverage, escalation card)

| ID | Case | Pass criteria | Auto | Status |
|----|------|---------------|------|--------|
| A-62-1 | Verdict schema + node isolation | reviewer `submit_review_outcome` with per-AC verdicts+evidence; schema enforced | task-harness e2e | ☑ run-94: plan_reviewer (run-3414/5007) submitted verdict with "9/9 AC verdicts pass" per-AC rows; run-5007 explicitly verified plan doc against repo state |
| A-62-2 | AC coverage at bridge | reviewer omits an AC → rejected at `turnBridge.SubmitFlowControl` (HTTP face too — BUG-392) | `TestReviewACCoverage_*` 12 | ☑ unit `TestReviewACCoverage_*` (12 cases) + BUG-392 HTTP-face tests — green |
| A-62-3 | Escalation/or-explained schema | `dod_explanation` schema pass | `TestRDodComplete_*` | ☑ unit `TestRDodComplete_*` green |
| A-62-4 | Sprint handoff enrichment | handoff carries card choice + consequence; prose fallback → recommended | `TestHandoffEnrichment_*` 8 | ☑ unit `TestHandoffEnrichment_*` (8 cases) green |
| A-62-5 | Decision card UI | render + option_id submit + prose fallback — Desktop+TUI | DecisionCard TUI tests | ◑ not verifiable from CLI bed — needs Desktop/TUI session; API leg proven live (gate-decision accepted + routed, run-69253; tournament card option `candidate-a` routed, run-25153) |
| A-62-6 | Drift pause card UI | dev-mode drift ≥80 card (shared w/ CP-23) | — | ◑ card *emission* verified live (run-49322 `drift_pause_required` evt seq 12629 + park + `flow_awaiting_user` on new turn); Desktop/TUI render leg needs UI session |

### A-4 CP-64 — reproduce-first TDD gate

| ID | Case | Pass criteria | Auto | Status |
|----|------|---------------|------|--------|
| A-64-1 | RED→lock→GREEN on real bug | `bug-harness`: reproducer writes failing test → `r-reproduce` passes → test file locked read-only (abs+rel paths, BUG-388) → implement can't touch it → GREEN → done; outer run settles terminal (was stuck-running) | `internal/flowgate` + e2e | ☑ run-14071: RED `strutil_flag_test.go` confirmed FAIL → implement fixed RI clause (test file untouched) → validate green → audit DONE; run-13080: same RED leg on combining marks + child-authored CA note |
| A-64-2 | False alarm → fail-closed | green-on-arrival → reprompt "suite passed…not reproduced"; implement PENDING; cap reachable (BUG-391) | oracle suite | ☑ unit green (`classify_probe_test.go` + reproduce_rule suite + `bug391_reprompt_cap_test.go` cap). LIVE run-90420 (grok): false bug report (Reverse len-4 drop — actually correct) → reproduce_test child run-90616 detected green-on-arrival → fail-closed question q-90837 ("bug already fixed / not reproducible") — implement never spawned. Observation: child re-parked twice asking for unblock path instead of escalating to terminal; operator stop resolved cleanly. |
| A-64-3 | Compile-error wording | reprompt says "failed to compile" not "suite passed" (`[setup failed]` signature) | `classify_probe_test.go` | ☑ unit `internal/flowgate/classify_probe_test.go` + `bug390_subtest_parse_test.go` — green |
| A-64-4 | Gate gaming | fabricated RED (doesn't call target) rejected (BUG-389); tampered test dropped (BUG-387); lock bypass attempts denied (BUG-388/396/397) | `bug386..398` files | ☑ unit `TestBug389_FabricatedTestDoesNotExerciseDeclaredSymbol` + `…RealReproductionExercisesDeclaredSymbol`, `TestBug387_ValidateFailsOnTamperedTestFile`, `TestBug386_FrozenContractPrefersSignatureLockedRecord`, `bug397_contract_rewind_guard` — all green |

### A-5 CP-67 — contract-first scaffold + signature lock (still `todo/`)

| ID | Case | Pass criteria | Auto | Status |
|----|------|---------------|------|--------|
| A-67-1 | Happy path on devin + one more provider | scaffold stubs + RED → contract v1 hash-pinned → body-only fill → signature lock holds → validate green → audit → done | P-1..P-4 unit battery | ☑ run-6893 devin leg: stubs+RED → body-only fill → validate green → audit done. Second-provider leg still open |
| A-67-2 | **r-signature-lock arms** (was BUG-LIVE-CP67-1 CRITICAL) | frozen contract record must be the hash-bearing one; violation (rename/additive fn) → reprompt, not silent ship | `bug386_*` + `TestRuleSignatureLock*` | ☑ unit `TestBug386_FrozenContractPrefersSignatureLockedRecord` + `TestRuleSignatureLock*` green; live re-verify: run-6893 scaffold leg froze hash-bearing contract v1 and signature lock held through body-only fill (A-67-1 evidence) |
| A-67-3 | Gate rejections | real-logic scaffold → r-scaffold-red; all-green → reject; compile-broken → compile wording; locked-test edit denied | `TestRuleScaffoldRed*` ×15 | ☑ unit `TestRuleScaffoldRed*` battery green |
| A-67-4 | Renegotiation | `renegotiate_signatures` offered to coder → record-only → `synthesis_negotiation` → round++ → cap 5 escalate; owner-debate resolves (BUG-411) | unit | ☑ unit `TestReviewOutcomeAcceptsRenegotiateSignaturesAndPreservesBatch`, `…RejectsRenegotiateWithoutBatch`, `TestSubmitFlowControlCoderBatchIsRecordOnly`, `TestNegotiationHubNodeForFindsPhaseHub`, `TestBug411_GateDecision*` — green |
| A-67-5 | Restart mid-negotiation | run survives restart; no false-done; parked batch restored or surfaced (BUG-410/404) | unit | ☑ unit `TestBug404_ParkedSprintStateRoundTripsSession` (parked state survives restart, no false-done) + bugf_cluster resume tests — green |

### A-6 CP-65 — tournament

| ID | Case | Pass criteria | Auto | Status |
|----|------|---------------|------|--------|
| A-65-1 | Standalone 2-provider cohort | devin + opencode candidates → isolated worktrees → arbiter → winner merges clean via `ApplyPatch` (never reached arbiter live before) | `tournament_*` suites | ☑ run-25153: grok+devin cohort, 3 rounds of isolated worktrees, human card pick `candidate-a` → merge DONE, patch landed (`strutil.go` GB3-5 fix + `strutil_crlf_test.go` + CA-004), `go test`/`go vet` green. run-20041: auto-pick topology live but empty winner exposed BUG-459 (fixed CA-956); run-21364 exposed BUG-460 worktree autoindex pollution (fixed CA-957); run-23455 verified empty-winner escalate |
| A-65-2 | Cap→auto-escalate | review cap → `tournament_escalation` child runs to completion (resumable, BUG-412); dedup — no `-2/-3` dup children (BUG-413/446) | `bug446_453_*`, cluster-h | ☑ unit `bug446_453_tournament_test.go` (dedup + escalation child) green; live adjacent: run-25153 arbiter retry×2 bounded → escalate card → merge DONE (cap→escalate leg proven on real provider turns) |
| A-65-3 | Tie → decision card | card `[]any` payload; choice routes: candidate→merge / retry→fresh cohort / ask→park (BUG-414) | `TestTournamentTie*` | ☑ run-25153: round-3 tie 1.0000 → escalate card parked `WAITING_USER_APPROVAL` → `continue` feedback `candidate-a` captured via `captureDecisionChoice` → `resumeTournamentChoice` routed to `merge_and_audit` → stored snapshot patch applied, DONE. run-19067: same card path → `candidate-b` choice → escalate "no mergeable diff" (BUG-453 path live) |
| A-65-4 | Retry ≤2 → parent resume | back-edge bounded; parent resumes after tournament | `TestTournamentEscalation*`, `TestResumeParentAfterTournament` | ☑ run-25153: arbiter `retry` ×2 (tie 1.0000 each) → `parallel_rollout` back-edge re-spawned fresh cohorts with distilled failure brief in prompts (`Tournament attempt N failed: tie …` observed verbatim in round-2/3 candidate prompts) → bounded by `max_attempts` → round-3 escalate card → post-card `merge_and_audit` DONE = flow settled terminal. Parent-resume leg still unproven (standalone run, no parent) |
| A-65-5 | `.flowpilot` exclusion | candidate diff/patch excludes runner metadata dir (manager.go union — verify post-rebase) | worktree tests | ☑ run-25153: merged winner patch = `strutil.go`+`strutil_crlf_test.go`+`CA-004.md` only; candidate worktree `.flowpilot/contracts`/`canonical-pending` writes excluded by `:(exclude).flowpilot` pathspec (manager.go:258/270). AGENTS/CLAUDE autoindex stamps additionally blocked by BUG-460 guard |

### A-7 CP-41 — RAG harness flow mode

| ID | Case | Pass criteria | Auto | Status |
|----|------|---------------|------|--------|
| A-41-1 | Happy path | `flowRef:"rag-harness"` → `flow_context_package` → sentinel in implement prompt → `flow_validation_result` exit 0 → `flow_audit_draft` ready; no auto-commit | runner e2e | ☑ equivalent chain live-verified on task-harness run-6893 (context.produce→…→validate→audit `ready`, draft persisted, no auto-commit) + run-71117. **Dedicated rag-harness launch impossible BY DESIGN**: `rag-harness.yaml` carries `selectableIn: []` (Task-324 T-C — removed from /flow picker, clone-only reference) → live launch attempt run-116450 rejected `invalid_flow_ref` fail-closed as intended. No selectable-mode launch path exists for this flow; the equivalent-chain coverage stands as the live evidence |
| A-41-2 | Validation retry + max | fail-once → `Retry 1/3`; always-fail → `failed_validation_max_retries`, no 4th | unit | ☑ unit `TestAdvanceRetryStateIncrements`/`…OnPass`/`…OnEnvError` + `TestRunValidationCommand*` (flow_validation_retry_test.go) — green |
| A-41-3 | Env error | missing binary → `skipped_env_error`, retryAttempt stays 0 | unit | ☑ unit `TestAdvanceRetryStateOnEnvError` (retryAttempt stays 0 on env-error skip) green; live degrade path also seen: gopls missing → `[lsp] … degraded` warn |
| A-41-4 | Audit blocked | missing feature key → `blocked_missing_feature_key` | unit | ☑ unit `TestFlowAuditDraftBlocksMissingFeatureKey` + `…BlocksSkippedNoCommand`/`…SkippedEnvError`/`…DoesNotClaimSuccessWhenValidationFailed` — green |
| A-41-5 | flowRef respects working mode | turn-level `flowRef` denied in wrong mode (BUG-400) | `bug400_*` | ☑ live run-46461/46463: vibe run + `task-harness` → `working_mode_flow_forbidden`; dev run + `vibe-ingest` → same; also vibe + `vibe-sprint` (system-only) rejected |

### A-8 CP-43 + CP-55 — change contract & preflight/canonical

| ID | Case | Pass criteria | Auto | Status |
|----|------|---------------|------|--------|
| A-43-1 | Declared contract | `contracts.ndjson` `confidence:"declared"` + declared_paths | contract tests | ☑ live: bed `frozen_contracts.ndjson`/`contracts` store holds declared contracts from run-6893 freeze + declared_paths; unit `internal/changecontract` package green |
| A-43-2 | Inferred contract | no contract → exactly one `r-contract` reprompt → `inferred` entry; reprompt turn carries gate-carried paths (BUG-425/439) | `bug425_*`, `bug439_440_*` | ☑ live run-69253: turn-69255 gate `rules=[r-ca … r-contract]` → reprompt fired once per attempt (bounded cap=2 observed); unit `TestBug440RepromptCarryUnionsPartialWriteEvents`/`…LSPRepromptPreservesCarriedPaths`, `TestBug439*` — green |
| A-43-3 | Scope drift → amend v2 | write outside declared_paths → WAITING_USER_APPROVAL → `agent-loop/amend` → v2 supersedes → resume | unit + live | ☑ LIVE run-86157: contract declared test-only (`snake/model_label_test.go`); scaffold child run-88364 submitted `scaffold_ready` stubs incl. `snake/model.go` → `test_signatures` parked WAITING_USER_APPROVAL (out-of-contract write) → `POST agent-loop/amend {paths:[snake/model.go]}` minted **v2** contracts on BOTH writer nodes (test_signatures fb2e…→d335…, implement 09d6…→8a8a…) adding `snake/model.go` → auto-resume → test_signatures DONE → implement DONE (Game.Label written) → validate/reviewer/synthesis/audit DONE. Earlier run-6893 also showed drift detect+park+re-gate. |
| A-43-4 | Pending→final canonical exactly-once + restart durability | SIGKILL between stage/finalize → resume → single Head write | `internal/changecontract` | ☑ unit `TestPendingCanonicalStoreStagesUpdate`/`…UsesLatestContractVersion`/`…IsIdempotent` — green |
| A-43-5 | Symlinked workspace | `/var`,`/tmp`-symlinked bed → freeze not falsely blocked (BUG-396) | `paths.go` tests | ☑ unit `TestBug396_DeclaredPathsUnderSymlinkedWorkspace` + `TestNormalizeDeclaredCodePathsRejectsSymlinkEscape`/`…ToBeCreatedFileSkipsSymlinkCheck` — green |
| A-55-1 | Freeze v1 pins writers | `frozen_contracts.ndjson` v1 + base_sha; writer constrained | e2e | ☑ run-6893: freeze DONE first pass; scaffold writer constrained to `strutil.go`/`strutil_test.go` — undeclared `livebed` write blocked by gate |
| A-55-2 | Amend→v2→resume | live on grok previously; re-run | live | ☑ LIVE run-86157 (grok): amend on scope-parked flow minted v2 superseding v1 for all writer nodes, `resumeFlowWithFeedback` un-parked, writer then passed scope gate — full chain to audit DONE. Guard legs also live: amend on non-parked run-79069 → `409 flow_not_blocked`; amend on pre-freeze blocked run-71117 → `no_frozen_contract`; empty paths → `invalid_request`. Units `bug366_amend_allow_feature_keys`/`TestBug366*` + changecontract Amend green |
| A-55-3 | Idempotent freeze post-SIGKILL | no planner re-fire | unit | ☑ unit `TestBug360DraftSurvivesRestartRoundTrip` (planner draft durable across restart → freeze proceeds from stash, no re-fire) + `TestBug360FreezeProceedsFromStashWithoutScoutChild`/`…StillEscalatesWithoutAnyDraft` — green |
| A-55-4 | Resume retry prompt carries contract scope (BUG-424) | reprompt text contains change.contract block | `bug424` ref'd tests | ☑ unit `TestBug424_ResumeRetryPromptCarriesContractScope` green |

### A-9 CP-60 — vibe working mode ⚠ (most open debt lives here)

| ID | Case | Pass criteria | Auto | Status |
|----|------|---------------|------|--------|
| A-60-1 | Mode gate | `/vibe` on clean bed → only vibe flows armable; dev `1/2/3` cards absent in vibe | workingmode tests | ☑ live run-46461/46463: vibe↔harness and dev↔vibe-ingest both rejected `working_mode_flow_forbidden`; system-only `vibe-sprint` also rejected for user start |
| A-60-2 | Snake MVP end-to-end | vibe run → SS → CP → tasks → sprint → playable `snake` (build+run green). **Was PARTIAL: owner-debate parked forever (BUG-411, fixed) — re-run required** | unit | ☑ run-37268+run-41626: Task-3 sprint FULL engine chain DONE; Task-4/5 sprints via agent-orchestrated `vibe-sprint` children (run-42033/run-43155) — `go test ./...` green, `go vet` clean, `snake/cmd` renders+quits. BUG-462/463 found+fixed live. Caveat: CP-01 interactive-playthrough DoD awaits human sign-off (documented) |
| A-60-3 | Fail-closed probes | `rm -rf`-class → refused + parked | unit | ☑ unit `TestBug344CompoundWriteBashDenies`/`…UnbalancedSyntaxFailsClosed`/`…ClassifierIsProviderAgnostic` + approval_allowlist tests — write/destructive-class commands denied, non-parseable input fails closed — green |
| A-60-4 | Resume checkpoint matrix §11 | R-SS/R-CP/R-TK keep + delete demotion (Task→CP→SS→empty); N-CP/N-SS/N-TK/N-DEL new-flow skip | unit + live | ☑ N-legs live-verified (run-91517 fresh ingest → SS lock→approve→cp stages→slicer; run-91606 cp-ingest skips SS, starts cp_reader; ingest overlay SKIPPED cp_reader/cp_validator/cp_lock post-SS-lock; CP-02 written by cp_writer). **Live drill found BUG-468**: sprint plan globbed foreign/stale Task files — both runs sprinted `Task-1-calc-gcd-lcm` (stale, `Parent Documents: none`, index 1/9) instead of their CP's tasks (12-14 CP-02 / 9-11 CP-01). Fixed (CA-965): `vibeCpDocID` scopes plan + presence checks to `Parent Documents` CP; unit green. **Fix live-verified on run-96489** (cp-ingest CP-01, same bed, stale tasks still present): admission pinned `vibe_cp_doc_id=CP-01`, sprint plan = 3 tasks only (Task-9/10/11), sprint started `Task-10-snake-titlelabel` index 1/3 — no foreign task entered. **BUG-469 found live on run-96970** (b468 verify leg): slicer delegate got empty handoff — mirrored vibe-cp-ingest def dropped artifactBindings (seed covered 3 harness flows only, synced-records only) → sliced newest CP-02, wrote Task-15/16/17 under a CP-01 run. Fixed (CA-966): builtin mirror restores pack ArtifactBindings; seed covers all binding-bearing flows + heal-all on boot; `vibeLockedCP` pinned source injects `## Bound input artifacts — resolved for this run` into slicer prompt. **Live-verified on run-97472** (b469, cp-ingest CP-01): slicer prompt turn-98711 carried `cp_md: CP-01-…md`; slicer wrote Task-18/19/20 all `Parent Documents: CP-01`; sprint plan = 6 CP-01 tasks only, `vibe_task_total:6`, started Task-10. R-TK delete-demotion leg live-verified on run-102429/run-103685/run-104945 (CP-01 todo tasks deleted → cp-ingest → slicer duplicate-detection question → skip → zero CP-01-parented tasks → `blocked/requirement` park, no foreign/stale sprint). **Drill found BUG-470** (CA-642 child-question mirror leaked root `waiting_question`; resolution healed owner only) — fixed CA-967, live-verified run-103685 (parent healed to running on mirrored answer). **Drill found BUG-471** (requirement-park Continue fell into sealed-hub no-op → running zombie → hub_stalled) — fixed CA-968 (`vibeRequirementFromNode` records the parking advance; Continue re-drives it: re-park on still-missing, plan rebuild + real dispatch when tasks appear). **BUG-471 live-verified run-104945** (b471, grok): zero-task park → continue → re-park blocked/requirement; restored Task-21 CP-01 → continue → plan rebuilt `vibe_task_total:1` → sprint chain dispatched (preflight→context→tdd RUNNING). **R-CP leg live-verified on run-106927** (CP-01 doc deleted): cp-ingest admission fails closed `invalid_cp_source` (typed, no fallback to another CP). vibe-ingest demoted: ingest_reader → ss_converter pass-through → ss_validator → ss_lock re-confirm → cp_writer wrote NEW CP-04 → ingest overlay SKIPPED cp_reader/validator/lock → slicer duplicate-detection question → "emit new" → Task-22/23/24 all `Parent Documents: CP-04` → `vibe_cp_doc_id=CP-04`, `vibe_task_total:3`, sprinting Task-22 — CP-scoped plan only. **R-SS leg live-verified on run-109797** (SS-01/02/03 deleted): full regeneration — ingest_reader → ss_converter rewrote all 3 SS docs → ss_validator → ss_lock → cp_writer wrote NEW CP-05 → slicer question → Task-25/26/27 all `Parent Documents: CP-05` → `vibe_task_total:3`, sprinting Task-25 — every prior task (CP-01/02/04 + stale) correctly excluded. **Bed post-drill state**: CP-01 doc + Task-9/10/11 restored from snapshot; drill artifacts retained & documented (CP-04/CP-05 docs, Task-18..27 re-slices, regenerated SS-01/02/03). **A-60-4 all legs live-verified.** |
| A-60-5 | Sprint reuse freeze (BUG-368, fixed CA-823) | 2nd vibe-sprint on same run gets fresh contract (sprint-1 paths not reused) | `bug368_*` | ☑ run-37268: sprint-2 minted NEW contract v4 for coder step with Task-4 paths (loop/input/render) — Task-3's v2 (model.go sigs) untouched |
| A-60-6 | SS lock stamp (BUG-365, fixed CA-820) | ss_lock writes `status: approved` on disk; requirement park surfaces blocked card | `bug365_*` | ☑ run-34947: ss_lock confirm → all 3 SS docs carry `Status: approved` on disk; card surfaced as `flow_parked_awaiting_user` + pendingGate ok/cancel |
| A-60-7 | vibe-cp-ingest admission | non-CP input → 422 `invalid_cp_source` (BUG-399 fixed — re-verify) | `bug399` tests | ☑ run-37268: turn without sourceDocId rejected `invalid_cp_source`; with `sourceDocId` armed cp_reader |
| A-60-8 | Waiting-approval orphan reconcile (BUG-432) | flow done while child waits → child terminalized, no orphan | unit | ☑ unit `TestBug432_ReconcileSettlesWaitingUserApprovalChild`/`…FlowDoneClearsPendingGateBlock`/`…GateDecisionRejectsWhenNothingPending`/`…SpawnRefusedWhileParentLoopBlocked` — green |

---

## B. State-durable (nhánh 2)

### B-1 CP-51 — durable turn dispatch + recovery

| ID | Case | Pass criteria | Auto | Status |
|----|------|---------------|------|--------|
| B-51-1 | Crash mid-turn | `kill -9` during live turn → restart → `POST resume` → reconcile-not-retry; `dispatch.ndjson` seqs unique+contiguous (BUG-406) | `bug447_449_*`, dispatch tests | ☑ live run-46465: SIGKILL mid-devin-stream → restart → run `cancelled` (no phantom retry), partial file `docs/durability-drill.md` survived, dispatch.ndjson seqs 1..1754 contiguous no dupes; boot settle finalized leftover turn-44931 |
| B-51-2 | Stop mid-flow | children cancelled, parent terminal, stop gen advanced; no ghost RUNNING | unit | ☑ live run-48587 (BUG-464 build): interrupt mid-`ss_converter` child (run-48832 → cancelled, "interrupted by user"), hub escalated to WAITING card, then `agent-loop/stop` → run `cancelled`, converter keeps real FAILED, hub swept WAITING→CANCELED (BUG-464 fix live-verified), never-started nodes stay PENDING |
| B-51-3 | Post-stop + post-done follow-up | admitted, answered, persisted across restart (BUG-302/305/306/307/308) | unit | ☑ live run-46465: new turn admitted on cancelled run (turn-46477), answered, r-task gate reprompt (turn-46868) ran and settled — agent recovered deleted Task-1/2 into `done/` from git history |
| B-51-4 | Gate reprompt idempotency | ×2 reprompts → durable keys `…0001`→`…0002`, no turn replay | unit | ☑ live run-46465: r-task gate reprompted twice — `durable-run-46465-reprompt-…0001`→turn-46868, `…0002`→turn-49288 (distinct turns, no replay); `pending_gate_reprompt_gen` 1→2 monotonic; sessions.ndjson durable rows carry both keys |
| B-51-5 | Repair/uncertain surfaces | forced repair → listed + resolved atomically; `repair-resolution` replay → recorded outcome 200 not 502 (BUG-407); cancel_required resolvable (BUG-408) | `bug405..409` refs | ☑ **ALL legs live**. **uncertain ☑ ×2**: run-46465 turn-46467 (SIGKILL mid-stream) + run-49161 turn-49163 (SIGKILL mid-send) → `uncertain` in dispatch-attention post-restart; resolve `abandon` → `terminal_cancelled`; replay → 200 idempotent; stale rev → 409; attention cleared. **cancel_required ☑ live run-116366**: child run-116371 turn-116376 `send_started` → file-watch raced `cancel_requested:true` landing → SIGKILL inside the provider-cancel window → restart → boot recovery `CommitRecoveryUnknownOrRequireCancel` → `RecoveryCancelRequired` → durable repair row `cancel_required: send_started/provider_accepted after stop (turn=turn-116376)` (seq 541) + TWO attention items listed: `cancel_required` ("stop-then-crash: provider cancel required", rev 5) + `repair_required` (rev 1). `repair-resolution {action:abandon, expectedRepairRev:1, resolutionId:b515-repair-116371}` → HTTP 200 `resolved_abandon` rev 3 → stranded record terminalized `terminal_cancelled` rev 6 (records-first ordering, BUG-447) → attention `items:[]`. Replay same resolutionId → HTTP 200 `resolved_abandon` "idempotent replay" (BUG-407). Fresh resolutionId + stale rev on resolved repair → 409 `dispatch_conflict: repair is not open`. Per-turn `resolve` correctly 409'd `illegal dispatch transition: resolve requires uncertain` — cancel_required's intended path is repair-resolution, not per-turn resolve |
| B-51-6 | Restart mid-flow restore | kill during child/synthesis → resume → steps+agent cards+timeline from durable rows; hub's unsent first prompt reappears; `run-*-turns.ndjson` identical pre/post | restart tests | ☑ live run-47170: SIGKILL mid-`ss_converter` child turn (run-47631) → restart → resume rehydrated run cancelled + steps from durable rows (converter CANCELED, hub ss_validator stamped RUNNING by cohort-join) → `resumeFrom: ingest_reader` gate → `ok` re-drove killed node: new child run-47655 spawned + completed, old child correctly shows cancelled; dispatch.ndjson 1..1819 contiguous no dupes. Found+fixed BUG-464: Stop after join left hub RUNNING ghost (sweep to CANCELED added) |
| B-51-7 | Quiet-flow self-recover | post-SIGKILL silent flow re-drives hub (BUG-404); stale terminal commits retry bounded (BUG-409) | unit | ☑ run-47170 (operator-resume re-drive) + run-49191: **autonomous boot re-drive verified** — `orphaned_work work=1` → turn-49193 re-dispatched with no operator input; provider cancel landed via durable stop fence (4s); terminal commit then failed closed `dispatch record revision is stale` — no double-commit, no retry storm. **Open observation RESOLVED 2026-09-24** (durable-log audit of `957928cc…/dispatch.ndjson` seq 2023-2041): turn-49193 was **fresh-minted** by the re-drive (prepared rev1 11:24:58 → send_claimed → send_started) — NOT already-terminal. The first `run_stop` row for run-49191 lands at seq 2026 (11:25:02, ~4s AFTER send_started) — either the pre-kill stop never fsynced or a new operator stop; either way no durable fence existed at send time so the send was contract-legal, and the landed fence then drove cancel→terminal→settle correctly. `reconcileOne` skips terminal records (dispatch_recovery.go:54) and `linearizeSendStarted` refuses terminal (dispatch_live.go:276) — the BUG-467 hypothesis (re-send of already-terminal record) is refuted by the durable log. Settle-phase rev 5→10 rewrites = bounded finalizer progression, not a retry storm |

### B-2 CP-59 — chat SSOT cross-provider

| ID | Case | Pass criteria | Auto | Status |
|----|------|---------------|------|--------|
| B-59-1 | Switch legs | devin→opencode (and back) → new leg, `includedTurnCount`, E-9 divider, target identity | chat SSOT tests | ☑ cht_10a27db90766 full round-trip: devin leg-0 (run-49030) → codex leg-1 (run-49042) → devin leg-2 (run-49071) → codex leg-3 (run-49091). Each switch mints new legSeq, closes source `provider_switch`, appends E-9 divider with correct stats. **2 live bugs found+fixed+re-verified**: BUG-465 — identical consecutive finals collapsed across turns (CA-962; seq 11 renders, seq-17 same-turn echo still deduped correctly); BUG-466 — switch on fresh process before lazy writer init → silent fresh_start/no seed/zero context (CA-963; re-verified: leg-3 switch got `raw`/`includedTurnCount:3`, 716-byte seed envelope on wire). Codex bed adapter returns canned "Done." text — context verified via seed-prompt bytes, not response content. |
| B-59-2 | Same-provider switch | `handoff_same_provider` 409 → in-place | unit | ☑ live cht_10a27db90766: switch codex→codex → `{"code":"handoff_same_provider"}`, active leg untouched |
| B-59-3 | Timeline | legs sorted, records deduped, dividers positioned | unit | ☑ live: 4 legs sorted by legSeq (0-3); E-9 dividers at leg boundaries (seqs 7/13/15); same-turn echo deduped (seq 17 collapsed), cross-turn identical text preserved (seq 19 — BUG-465 fix) |
| B-59-4 | Detached reattach | first prompt reattaches (no `chat_no_active_leg` — BUG-405 legState restore, re-verify post-restart) | `bug405` refs | ◑ unit green (TestReattach*/bug405/detached suite). Live leg BLOCKED-ENV: `closed/restored` legs are only stamped by Drive manifest apply (`restoreChatRunFromDrive` → real Drive API) — same blocker as B-59-6. legState restore itself re-verified live: post-restart legs kept correct active/closed states (B-59-5 drill) |
| B-59-5 | Restart mid-multi-leg | kill+restart → timeline identical (chatSeq stable, E-9 idempotent) | unit | ☑ live: 2 graceful restarts (b464→b465→b466 binary) mid-chat with 3-4 legs → chatSeq continued monotonically 1→19, exactly one E-9 per toRunId (no heal-path dupes), timeline identical. Restart was at idle boundary — mid-*turn* restart on multi-leg chat not yet drilled |
| B-59-6 | Drive sync G1-G7 | — | — | ☐ BLOCKED-ENV (no Drive creds) |

---

## C. Support CPs (context/LSP/knowledge — feed the core)

| ID | CP | Case | Pass criteria | Auto | Status |
|----|----|------|---------------|------|--------|
| C-23-1 | 23 | Budget packer | wide-read turn → `[prompt-pack]` truncation fields | promptpacker tests | ☑ run-49322 turn-50420: `74039→183 bytes`, `selected=4/dropped=18506`, `memory_summary … exceeded_section_budget`; audit JSONL persisted under `.flowpilot/runs/…/prompt-context-audit-*.jsonl`. Earlier probe turn-49324: mandatory-only `current_task` correctly retained whole over budget (no silent blank). |
| C-23-2 | 23 | Drift ladder + single pause | failing turns → score events → `drift_pause_required` once at ≥80 → continue resumes | driftdetect + `bug430_*` + `bug467_*` | ☑ run-49322 ladder live on devin: turn-53117→20(none) → turn-56695→40(`inject_system_note`, 595B note verified in wire prompt turn-60239) → turn-60239→60(`narrow_context`, halved budget total=4000 applied) → turn-62990→80(`pause_for_human` → `drift_pause_required` evt-65744 + parked; new turns rejected `flow_awaiting_user`) → `continue` resumed → turn-65746→100 cap → re-park (carried score persists until correction). **BUG-467 found+fixed (CA-964):** `.flowpilot/**` runner bookkeeping was counted as file delta → `zero_delta_progress` could never fire on beds where `.flowpilot` is git-tracked; filtered in `turnSummaryFromTurnResult`. |
| C-23-3 | 23 | Vibe non-pause | same ladder in vibe → no pause | unit | ☑ unit: `TestDriftPause_VibeModeNeverAsksUser` (vibe run never parks/emits) + `TestDriftPauseGraphReportPreservesBlockedReason` both modes; non-pause is a pure mode check in `armDriftPause` — live vibe ladder not separately drilled (unit coverage adequate for a guard clause). |
| C-23-4 | 23 | Skillpack install | `.agents/skills` + `.claude/skills` populated, `version:` markers (BUG-415) | skillpack tests | ☑ bed: 15 skills in `.agents/skills` + mirrored `.claude/skills`, all carry `version:` frontmatter (e.g. flow-harness-contract v6) |
| C-35-1 | 35 | Feature resolve + history | "improve calc-core" → verified confidence, newest-last prior work | featurecatalog | ☑ LIVE run-76786: `str-utils` resolved verified; history block injected `ranked, 15/38` with newest entry pinned `← truth` (position 4 after 3 overlap-ranked entries — recency preserved as truth-pin, ordering covered by unit `TestChatSummaryRemainsRecencyBased` + SelectHistory newest-truth tests); earlier run-6893 first-run empty-section degrade also correct |
| C-35-2 | 35 | r-ca gate | no CA note → reprompt→block; CA written → pass | runner gate tests | ☑ run-6893: audit tier-3 blocked done on missing CA note → operator wrote CA-001 → re-observe → audit DONE |
| C-35-3 | 35 | Oracle regression block | break pre-existing test → `regression_test_broke` | flowgate | ☑ run-69253 turn-69255 (devin): broke `Reverse` in strutil.go → scoped oracle `go test .` exit 1 → `flow_gate_violation` evt-69710 `status:block`, `gateRegressedTests`=39 TestReverse* entries, options `keep-test-fix-code/suggest-requirement-change/custom`; decision accepted → reprompt fired. Bed restored via `git checkout strutil.go` (green again). |
| C-37-1 | 37 | History + CA inject on **all** providers incl. devin | prompt artifact contains feature-history block (BUG-376 allowlist) | unit | ☑ LIVE run-76786→child run-76951 (plan_writer, devin): wire prompt `prompt-turn-76956.txt:72` contains `## History "str-utils" (ranked, 15/38; ← truth)` — feature-history block rendered into a real provider prompt; injection is provider-agnostic (runner-side assembly pre-dispatch) + BUG-376 allowlist unit green |
| C-37-2 | 37 | Unknown feature key degrade | reprompt once, no crash | unit | ☑ unit `TestEvaluateCommitFeatureKeyMissingRejectsUnknownKey` (r-fk reprompt fires on unverified key) + `TestBug439UnregisteredSuggestionIsNotPersisted` + `TestFlowAuditDraftBlocksMissingFeatureKey` (→ `blocked_missing_feature_key` escalate, never panic); live: run-1/run-94 `conf=unresolved` (catalog missing) and run-13080/14071/37268 `conf=low` warnings — flows continued, no crash; reprompt bounded by maxFlowGateReprompts=2 then escalate (observed run-69253 reprompt attempts 0→1) |
| C-37-3 | 37 | Sticky/pivot + no cross-feature mixing | calc-core vs calc-format isolation | unit D/G | ☑ unit `TestResolveInjectionFeatureFlowEngineJoinedNoteInheritsFeature` (calc-core inherited over calc-format/sandbox-meta in joined+synthesis prompts), `TestResolveInjectionFeatureHandoffPromptDoesNotSelfResolve` (Test D: handoff envelope can't self-resolve on fresh leg), `TestBucketTurnsByFeatureSeparatesFeatures`/`…DropsUnrelatedTurn`, `TestInjectFeatureHistoryFallsBackToPriorTurnFeature` (sticky) / `…DropsContextOnUnrelatedPrompt` (pivot), `TestBuildFlowContextPackageResolvedFeatureKeyVerifiedOnlyWhenInCatalog` (unverified key never sees other feature's history) — all green |
| C-54-1 | 54 | changed_paths in ledger + locus builder (frozen/diff/empty) + ranked history + `[context-rank]` tiers + chat.summary recency | runner.log 3-tier rank; locus correct | contextsync | ☑ LIVE run-71117 + run-76786: seeded 35 `str-utils` entries → `context.produce` fired `[context-rank]`×35, `locus_paths=12`=bed's uncommitted `snake/*.go` (diff-locus leg verified), cap 15, newest pinned `← truth`, `ranked, 15/35` rendered. Overlap leg: appended 3 old `snake/model.go` entries → run-76786 context rebuilt `ranked, 15/38`, rank=0/1/2 = the three Aug-oldest entries with `path_overlap=1 symbol_overlap=1` — relevance beat recency live; newest still `← truth`. Units green: SelectHistoryEntries threshold/fallback/newest-truth/cap, RankHistoryEntries overlap>recency+tiebreak, buildRetrievalLocus frozen/diff/empty, `TestChatSummaryRemainsRecencyBased` |
| C-63-1 | 63 | gopls diagnostics live | `[lsp] lsp.start` + **sev1 diagnostics actually surfaced** (BUG-380 initialized-notify fix — re-verify end-to-end) | lsp tests | ☑ LIVE run-79069 turn-80475: gate-clean (`violations=0`, scoped oracle `go test ./snake/...` PASS 351ms — darwin skips `_windows.go`) → `[lsp] lsp.start binary="gopls"` → sev-1 surfaced verbatim in reprompt turn-80840's prompt: `snake/probe_windows.go:3:8 error: "fmt" imported and not used [windows,amd64]` — a cross-GOOS diagnostic the local oracle cannot see; per-URI publish wait (BUG-380) worked end-to-end. Unit: `TestLSPHelperCrashAfterInit`, `TestServerSetSessionWideDisableAfterCrashBudget` green |
| C-63-2 | 63 | Degrade + doctor | missing binary → warn-once + `flowpilot doctor` MISSING exit 1 | cli/lsp | ☑ live: `[lsp] gopls not found in PATH — LSP diagnostics degraded` warn fired 2× (16:04/16:29); `flowpilot doctor` lists gopls MISSING and exits 1 |
| C-63-3 | 63 | Crash budget | repeated crashes → session-wide disable, no respawn | unit | ☑ unit `TestLSPHelperCrashAfterInit` + `TestServerSetSessionWideDisableAfterCrashBudget` (internal/lsp): crash budget spent → manager disabled → later check does NOT respawn — green |
| C-66-1 | 66 | GitNexus bootstrap → knowledge artifacts | bootstrap ok; 0-process degrade graceful | knowledge tests | ☑ LIVE: bed `.flowpilot/knowledge/` populated (system-overview.md, data-models.md, execution-flows.md, index.json — bootstrap ran) AND `index.json` holds **0 flows / 0 paths / 0 symbols** — the zero-process degrade state itself; `knowledge.flow` source returns empty quietly inside real context packages (run-71117/76558/76786 all built packages without a knowledge section, no crash/warn). Unit `TestKnowledgeFlowSourceReturnsEmptyGracefullyWhenMissing`+`TestKnowledgeFlowSourceResolvesFlowFromLocus` green |
| C-66-2 | 66 | Locus routing | planner gets `knowledge.flow`; coder does NOT; `candidateSources` wired (BUG-421) | unit | ☑ unit `TestBug421_ProduceResolvesConsumerProfile` (candidateSources → knowledge.flow reaches plan_writer's package) + `TestKnowledgeFlowSource*` (locus→flow resolve, graceful missing) green; config verified: task-harness.yaml wires knowledge.flow to scout+plan_writer only, NOT coder/reviewer; bed index.json empty (0 execution flows) → quiet-empty degrade confirmed correct |
| C-66-3 | 66 | Audit hook differential non-blocking | incremental update touches only changed artifacts | unit | ☑ unit `TestAuditNodeUpdatesKnowledgeBaseIncrementally` (only changed paths' artifacts re-distilled), `TestAuditNodeNonBlockingOnKnowledgeError` (hook failure never blocks audit), `TestAuditKnowledgePathsFilter`, `TestAuditHookSkipsUnbootstrappedWorkspace` — all green |

---

## D. Fixed-but-needs-live-regression watch (was "open bug" — all landed with tests + CA)

All nine were found already implemented on this HEAD (test files `bug363..bug373_*`,
CAs 817–832); docs moved to `09-BugFix/done/` 2026-09-24. Their live symptoms are the
rows to re-verify — a green unit suite does not prove the live path (that's how they
escaped originally).

| Bug | Fixed by | Live symptom to re-verify | Maps to |
|-----|----------|---------------------------|---------|
| BUG-363 | CA-817 | slicer wrote nothing → park w/ gate reason (not silent SS-fallback sprint) | A-60-2 |
| BUG-364 | CA-818 | parked slicer node stamped DONE (no eternal RUNNING step) | A-60-2 |
| BUG-365 | CA-820 | SS files `status: approved` on disk after lock; park emits blocked card | A-60-6 |
| BUG-366 | CA-821 | `[Allow]` on doc/audit drift → 200, widens scope, resumes | A-43-3 |
| BUG-367 | CA-822 | Task `in_progress`+DoD `[x]` after sprint; CP `approved` never `done`; `task x/y` chip | A-60-2 |
| BUG-368 | CA-823 | sprint 2 freezes Task-911 paths, not sprint-1's | A-60-5 |
| BUG-369 | CA-824 | `/agents` rows show `task x/y Task-NNN.md` per child | UI |
| BUG-370 | CA-825 | `*.md` writes (FEATURE-KEYS, tdd-signatures, CA notes) never park; `extra.go`/`flow-rules.json` still do | A-43-3/A-64 |
| BUG-371 | CA-826 | composer hides `[stop]` on `done` loop with no RUNNING child | UI |
| BUG-075 | — | Desktop image context lost next turn — deliberate trade-off, stays open | out-of-scope |

## E. Env/executor-blocked (record BLOCKED, don't fake)

- CP-59 Drive G1-G7 — needs Drive creds
- ~~A-65-1 real winner merge~~ — DONE run-25153 (grok+devin via admin
  `step_definitions.model` override; note the override rows live only in
  the Supabase mirror — a fresh env without them falls back to the pack's
  claude/codex pair)
- Any `grok` leg while account is 402-quota'd
- Desktop UI rows (A-62-5/6, BUG-367/369/371 chips) — need Desktop app session

## G. Post-audit BUG live legs (BUG-472..484, implemented 2026-09-25)

The 13 audit BUGs landed with unit/E2E coverage; this section is their
**live-runner verification matrix**. Same rule as §D: a green unit suite
does not prove the live path.

| Bug | Fix | Live drill | Status |
|-----|-----|-----------|--------|
| BUG-474 | heal Desktop-clone rows from cloned_from + `definition_json` passthrough + unmigrated-remote degrade (CA-983) | Desktop-shape PostgREST clone of `cp-harness-smoke` (e4891212) → runner `GetByRef` | ☑ LIVE 2026-09-25: clone `00ba0ee5` (13 steps, no `definition_json`, no copied `step_artifact_bindings`) resolved through prod store code → `cp_plan_writer`/`task_splitter`/`cp_reviewer` artifactBindings + `pathTemplate` restored from embedded pack, flow `tools=1` restored, `run=` derived. Remote lacks the column entirely (42703) — column-free retry + backfill-skip degrade exercised for real. Clone visible in `/client/workflows` (23 rows). Remote artifacts cleaned. |
| BUG-472 | dirty-scan fail-open → 503 (CA-970) | worktree run → break git inspection → resolve destructive | ☑ LIVE 2026-09-25: `cht_67b99e9bd6fe` run-116454 — `.git` file removed → resolve discard → **503** + dir + binding intact; `.git` restored → discard confirmed → dir removed, binding `discarded`. |
| BUG-473 | boot GC skips on persistence read failure (CA-971) | corrupt session record → restart → worktree preserved | ☑ LIVE 2026-09-25 (build a0f8343f): seeded `cht_gc473b` record `worktree_state=""` + dir → restart → log `gc_deferred owner=cht_gc473b (binding authority unreadable or corrupt)` → dir preserved. True orphan `cht_orphan485` pruned same boot (positive control). NOTE: first attempt masked by BUG-485 — verified after its fix. |
| BUG-485 | NDJSON partial reads dropped durable records → GC pruned live bindings (CA-984) | real bed `sessions.ndjson` (567 lines >64KB, record at line 2250+ `project_id:""` `worktree_state=active`) → restart → binding must survive; true orphan must still prune | ☑ LIVE 2026-09-25 (build a0f8343f): `cht_67b99e9bd6fe` dir recreated → restart → **not pruned** (post-fat-line + empty-project record loaded → `gcBound`); `cht_orphan485` **pruned** (proven orphan — no loadErr fail-closed); no `load incomplete` log; `[chat-history-open]` lines show post-fat-line records restored. |
| BUG-481 | durable resolve phases + replay (CA-972) | kill mid-resolve → restart → converge | ☑ LIVE 2026-09-25: happy path (dirty-scan discard → terminal `discarded`) + **mid-phase replay** — seeded `worktree_resolution_phase=repository_effect_committed`/`res-live481` on run-122896 → restart → resolve discard resumed the SAME resolutionId → dir + `.base` removed → record `discarded`, intent cleared. |
| BUG-475 | attention read error → resync not empty (CA-973) | attention store fault during SSE | ☑ LIVE 2026-09-25 (build b486, via BUG-486 propagation): corrupt shard → `GET /client/events/stream` first frame `event: resync {"kind":"resync","retryable":true}` — never an empty snapshot; `GET dispatch-attention` → **502 `attention_list_failed`**. Restored → `snapshot` frame with live `dispatch_attention` item. |
| BUG-484 | recovery retry coordinator (CA-974) | transient boot failure → observe retry | ☑ LIVE 2026-09-25 (build b486): corrupt `dispatch.ndjson` shard → `[dispatch-recovery] pass N failed … retrying` ladder 564ms→1s→2s→4s→9s→19s→30s; shard restored mid-window → **`pass succeeded after 8 attempts`**. Prior state: one silent attempt. |
| BUG-486 | dispatch shard read errors swallowed → recovery/attention blind (CA-985) | unreadable shard → enumeration must error, not return partial-as-clean | ☑ LIVE 2026-09-25: `dispatch.ndjson` as directory → boot pass errors surface (`boot scan failed`, `boot list recoverable`), attention endpoint 502, retry coordinator fires (BUG-484 reachable), clean pass after restore. |
| BUG-476 | logical-chat Drive manifest v2 (CA-975) | multi-leg chat → Drive sync → manifest | ☐ BLOCKED (env): project `fp-beds-full` has no Drive connection (`google_drive_not_connected` on `POST sync-chat` with real multi-leg chat `cht_6ad8feede479`). Needs OAuth + folder setup. |
| BUG-483 | Drive index download failure preservation (CA-976) | Drive read fault → index untouched | ☐ BLOCKED (env): same — no Drive-connected project on this bed. |
| BUG-477 | durable knowledge-update ledger (CA-978) | audit update → kill -9 → restart → replay | ☑ LIVE 2026-09-25: seeded `pending-updates.json` (2 intents) → `POST /client/workflow-runs` → replay consumed intents → `intents:[]` (commit after write). Corrupt ledger → `[knowledge]` full rebuild → all 3 `knowledge/*.md` rewritten + ledger removed (fail-closed to rebuild, not silent-fresh). |
| BUG-478 | merge card alternates + discard (CA-979) | tournament empty winner → card + discard | ☑ VERIFIED 2026-09-25 live `run-478live` (build b486): seeded durable tournament merge card (`decision_card` kind=tournament, alternates candidate-a/b + retry/discard/ask, `tournament_patches` both non-empty, empty winner) + real git worktrees `candidate-candidate-{a,b}` → resume restored card → generic Continue `feedback:"discard"` → `captureDecisionChoice` → `resumeTournamentChoice` → `discardTournamentMerge`: both worktrees swept (`git worktree list` empty), `tournament_patches`/`decision_card` cleared in record, run `completed` (no fake merge). Card-render options-from-non-empty-patches arm remains unit-covered (`bug478` tests). |
| BUG-479 | mux unseen-lane → history insert (CA-980) | SSE consumer observes upsert for unseen run | ◐ LIVE-PARTIAL 2026-09-25: real `/client/events/stream` wire verified — snapshot (5 runs) then 26 `upsert` frames for `run-130763`/`run-129348` **absent from snapshot** = the exact unseen-lane input. Client-side lane insertion is unit-covered (`muxUpsertBug479.test.ts`). |
| BUG-480 | vibeResumeConfirm via gate-decision (CA-981) | vibe park → generic Continue → gate consumed | ☑ VERIFIED 2026-09-25 live `run-480live2` (build b486, ws480 fixture: SS+CP-481, no Task/tdd-signatures → real `maybeParkVibeCpJoinResume` arm). (a) Resume parked `blocked`/`paused` gateReason=`Resume confirmation (cp_writer → task_slicer)`; (b) ambiguous-prose Continue → **409 `pending_gate_decision`**, park stayed mounted (pre-fix unblocked loop while gate mounted); (c) generic Continue → routed `SubmitGateDecision(ok)` → confirm consumed → `forceStartVibeTaskSlicer` spawned `run-195144` (grok doc-writer) → loop running. Both fix arms live-proven. |
| BUG-482 | malformed SSE frame → resync (CA-982) | corrupted frame on live mux stream | ◐ CLIENT-SIDE: wire verified live (well-formed `snapshot`/`upsert`/`remove`/`resync` frames observed on real stream); malformed-authoritative-frame → stream-fail is parser-side, unit-covered (`streamRunUpdatesBug482.test.ts`). Server never emits malformed frames — no live injection seam. |
| BUG-487 | transcript/scaffold loaders 4MB scanner cap (CA-986) | >4MB NDJSON line + valid line after it | ☑ LIVE 2026-09-25 (build b487): appended 5MB line + sentinel `{"seq":900002,...,"BUG487-LIVE-SENTINEL"}` to real `.flowpilot/scaffold-progress.ndjson` → `GET /client/projects/{id}/scaffold/progress` returned both events (`n:2`, `phase:"drill"` — post-fat line read). Pre-fix: scanner cap dropped both silently. Claude/Codex/Grok transcript loaders same helper — unit-covered per provider. |
| BUG-488 | reattach legSeq on unreadable store (CA-987) | sessions.ndjson → directory → create chat run | ☑ LIVE 2026-09-25 (build b487): fault-boot → `POST /client/workflow-runs` `chatId=cht_6ad8feede479` → **502 `chat_identity_unprovable`** ("persisted leg scan failed"). Pre-fix: silent duplicate legSeq. |
| BUG-489 | closed-leg persist failure → dual-active (CA-988) | write fault at switch | ◐ LIVE-PARTIAL 2026-09-25: healthy boot → chat leg `run-215483` → sessions.ndjson→dir → `POST /chats/cht_bug489drill/switch-provider` → **502 `workflow_state_unavailable`** (Phase-A intent persist fails closed — no new leg created). Phase-C closed-leg retry+degraded arm needs a mid-handler write fault (impossible externally) — unit-covered (`bug489_leg_close_persist_test.go`). |
| BUG-490 | worktree binding read swallow (CA-989) | same fault → create chat run | ☑ LIVE 2026-09-25 (build b487): `POST /client/workflow-runs` new chat → **502 `worktree_binding_unprovable`** (leg scan failed → no second worktree provisioned). |
| BUG-491 | session enumeration swallows (CA-990) | fault → resume / agents list | ☑ LIVE 2026-09-25 (build b487): `POST /workflow-runs/run-480live2/resume` → **502 `workflow_state_unavailable`**; `GET /workflow-runs/run-130763/agents` → **502 `session_index_unavailable`** ("session index unreadable"). Boot under fault logged `seedIDCounter: session index unreadable` (loud, not silent). |
| BUG-492 | Drive restore reader errors (CA-991) | collision probe / local-ahead fault | ☐ BLOCKED (env): no Drive-connected project (`google_drive_not_connected`); `sync-chat` on drill leg → 409 `session_unavailable` (placeholder session). Unit-covered (`bug492_restore_reader_errors_test.go`). |
| BUG-493 | history/timeline partial views (CA-992) | fault → history + agents endpoints | ☑ LIVE 2026-09-25 (build b487): `GET /client/projects/fp-beds-full/workflow-runs` → **502 `run_history_unavailable`**; agents endpoint → 502. Chat/run-timeline 502 arms unreachable live: file store loads once at boot (sticky `sessionsLoadErr`), so a poisoned store never co-occurs with resident legs — unit-covered (`bug493_history_view_swallow_test.go`). |
| BUG-494 | live stream scanner Err() unchecked (CA-993) | >10MB provider line / read fault | ◐ LIVE-PARTIAL 2026-09-25: `POST /compat/deep` exercised the modified probe paths on real binaries — `codex app-server initialize` → warn (capabilities absent, goroutine path works), `claude stream-json` → fail (`claude -p` exit 1, env auth — pre-existing path). Capture-goroutine >10MB-line arm unit-covered (`bug494_stream_scanner_err_test.go`); all provider stream readers (claude_stream/grok/devin/opencode/codex_appserver) already checked `Err()`. |
| BUG-495 | pending-gate sidecar read swallows on resume (CA-994) | fault → resume parked gate run | ◐ UNIT-ONLY 2026-09-25: sidecar read faults are unreachable via filesystem drill — `approvals.ndjson`/`questions.ndjson` per-run writes under chats dir share the same sticky load-once semantics as session store; a mid-reconstruct read fault can't be staged without code hooks. Covered by `bug495_pending_gate_state_swallow_test.go` (fault stub on the reader interface). |
| BUG-496 | Drive index merge scans cap 64KB (CA-995) | >64KB index line in remote merge | ☐ BLOCKED (env): no Drive-connected project; same class as BUG-483/476. Unit-covered: real >64KB-line drop reproduced pre-fix in test, `readNDJSONLines` uncapped post-fix. |
| BUG-497 | contract store load swallows open/scan errors (CA-996) | permission-denied contract file → gate enforcement blind | ◐ UNIT-ONLY 2026-09-25: no public endpoint loads `changecontract` store on demand (loaded at run lifecycle boundaries inside flow orchestration — not a standalone HTTP surface). `bug497_store_load_errors_test.go` covers open-error + >4MB-line propagation. |
| BUG-498 | oracle / TUI SSE unchecked scanner Err (CA-997) | >4MB suite-name line / truncated SSE | ◐ UNIT-ONLY 2026-09-25: oracle parse is invoked by gate hooks during real gate runs (needs a configured suite emitting >4MB name lines — no gate suite on this bed); TUI SSE path is client-side. `bug498_*` tests cover both arms. |
| BUG-499 | session/dispatch store memory-commit before durable write (CA-998) | write fault → mutation rejected → memory must stay clean → retry must succeed | ☑ LIVE 2026-09-25 (build b499): fault `dispatch.ndjson`→directory → `POST /client/workflow-runs/run-77530/dispatches/turn-77535/resolve` → **error** (`dispatch_operator_error: is a directory`); GET during fault → **rev 28 / uncertain** — memory unchanged (pre-fix ordering would show rev 29); audit shows seq 182 only — **no phantom seq burned**. Restore → same `expectedRev:28` retry → **200 `terminal_completed`, rev 29, seq 183 on disk** — retryable contract preserved end-to-end (the rejected latch design would have deadlocked the store here). |
| BUG-500 | Supabase store missing gate readers → structural fail-open (CA-999) | Supabase backend reconstruct → pending gates lost | ☐ BLOCKED (env): Supabase store has no production wiring yet (only tests + opt-in path) — no live Supabase backend on this bed. Mock-HTTP unit tests verify `ListApprovalsByRun`/`ListQuestionsByRun` against real migration schema shape; compile-time assertions pin the reader surface on both stores. Write-side schema drift recorded as tracked residual in BUG-500 doc. |
| CA-1000 | dispatch operator 502s logged server-side | fault → `POST resolve` → 502 + log line | ☑ LIVE 2026-09-25 (build b500): fault on fp-beds-full `dispatch.ndjson` → resolve → 502 + `[dispatch] operator mutation failed` in runner log; GET → rev 25/uncertain (memory clean); restore → retry → `terminal_failed` settle_pending. |
| — | post-fix main-use-case smoke | create→turn→kill-9→restart→resume→switch→SSE | ☑ LIVE 2026-09-25 (build b500): codex leg `run-225650` + grok leg `run-225676`/`cht_76e09ed9843b`; real grok turns persisted (`PONG-500`, `STILL-ALIVE` transcript_turn); enforce gate blocked turns on `r-reg` (bed has failing `TestGcd` — correct contract, not a bug); kill -9 → restart → 2633 runs reloaded → resume pinned to same grok ACP session `01a0d83a`; switch grok→devin minted leg `run-235870` legSeq=1 (no dual-active); SSE snapshot+remove frames healthy. Note: codex turns settle instantly with no rollout — codex adapter is the fake when `FLOWPILOT_CODEX_APPSERVER` unset (by-design demo path, not a regression). |
| BUG-501 | worktree scans re-scope to parent repo when .git link breaks (CA-1001) | worktree run → rm .git → discard | ☑ LIVE 2026-09-25 — **LIVE-FOUND + FIXED + re-drilled**: pre-fix `run-260771` discard with `.git` removed silently succeeded (git walked to parent; `.flowpilot/worktrees/` is gitignored → `ls-files --others` empty error-free → gate passed). Post-fix build b501: `run-260771`/`cht_1cef2ef8b82f` same drill → **503 `worktree_inspection_failed`** ("git scope resolves to parent, not itself"), dir + planted `uncommitted.txt` preserved; `.git` restored → discard → dir removed, run deleted. |
| — | re-drill sweep after dispatch-ordering sweep (cc8ea9cf + 5df4137e) | re-run every live-seam bug on b501 | ☑ LIVE 2026-09-25 (build b500/b501): BUG-487 (5MB line + sentinel → both events), BUG-477 (2 seeded intents → bind → `intents:[]`), BUG-485 (fault-boot → `seedIDCounter: session index unreadable` log + dispatch fallback seed), BUG-488 (reattach → 502 `chat_identity_unprovable`), BUG-490 (create → 502 `worktree_binding_unprovable`), BUG-491 (resume → 502 `workflow_state_unavailable`; agents → 502 `session_index_unavailable`), BUG-493 (history → 502 `run_history_unavailable`), BUG-489 Phase-A (write-fault switch → 502 `workflow_state_unavailable`, no new leg), BUG-499 (resolve-fault → 502 + log line + rev unchanged → retry lands), CA-1000 (operator 502 logging live). BUG-478/480 fixture runs no longer on bed — unit-verified only; BUG-474/476/483/492/496/500 env-blocked (Drive/Supabase). |

## F. Execution order (single session)

1. §0 automated gate → green/baseline.
2. Preflight §1 (one bed, port 19400).
3. A-1 task-harness happy path (also covers A-61-1, A-62-1/2, A-43-1/2, A-55-1, C-35-*, C-37-* partially — one run, many rows).
4. A-58-2/3 reject legs (plan + code) on the same flow type.
5. A-64 bug-harness on seeded bug (A-64-1..4).
6. A-67 task-harness devin run (A-67-1..5) — CRITICAL rows.
7. A-65 tournament (if 2 providers up).
8. A-60 vibe snake (A-60-1..8) — longest leg; run while reviewing logs of earlier rows.
9. B-51/B-59 durability drills (kill -9, restart, resume) on the runs above.
10. C-* support rows folded into the same runs (log greps).
11. Update status column + `CP-Test-Progress-Tracking.md`; file new bugs for any ✗.
