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
| A-58-3 | Code loop isolated from plan loop | Force code reject → implement re-entry; `plan_*` + freeze remain DONE | unit matrix | ☐ |
| A-58-4 | Round cap 3 → escalate | Force 3 rejects → `blocked`/escalate card, no 4th round (was never forced live) | `agent_orchestrator_test.go` cap | ☐ DEFERRED-LIVE |
| A-58-5 | cp-harness slice-only | `/flow cp-harness` → `cp_plan_writer→cp_reviewer→cp_synthesis→task_splitter→audit→done`; exactly N Task files, additive, no implement nodes | `bug356_slice_audit_test.go` | ☐ |
| A-58-6 | cp-harness-smoke (clone) | Clone → run: continues through implement chain; `acceptance_nodes` preserved | — | ☐ NEVER-LIVE |
| A-58-7 | Regression canary | `rag-harness` + `review-loop` behave as pre-CP-58 | F3 test cmds §F | ☐ |

### A-2 CP-61 — done-verdict gate (machine PASS, not self-grade)

| ID | Case | Pass criteria | Auto | Status |
|----|------|---------------|------|--------|
| A-61-1 | All 3 hubs gate on cohort verdict | task-harness live run: `plan_synthesis` needs `plan_reviewer` PASS; `synthesis` needs `reviewer` PASS → audit; wrong-cohort PASS doesn't unlock | `TestCP61HubDone` 13×3 providers | ☑ run-6893: `synthesis` consumed grok reviewer approved-verdict → audit; run-94: plan_synthesis consumed devin plan_reviewer arc |
| A-61-2 | Missing verdict → escalate | `flow_control_rejected_missing_review_verdict`; never freeze/audit | unit | ☐ (live optional — rare) |
| A-61-3 | cp-harness reject path live | cp_reviewer `changes_requested` → writer re-entry (deferred from M-wave) | unit 2.4 | ☐ DEFERRED-LIVE |
| A-61-4 | Non-harness chat unaffected | plain chat → no hub events | `normal_chat_unaffected` | ☐ |

### A-3 CP-62 — ZCode parity (verdict schema, AC coverage, escalation card)

| ID | Case | Pass criteria | Auto | Status |
|----|------|---------------|------|--------|
| A-62-1 | Verdict schema + node isolation | reviewer `submit_review_outcome` with per-AC verdicts+evidence; schema enforced | task-harness e2e | ☑ run-94: plan_reviewer (run-3414/5007) submitted verdict with "9/9 AC verdicts pass" per-AC rows; run-5007 explicitly verified plan doc against repo state |
| A-62-2 | AC coverage at bridge | reviewer omits an AC → rejected at `turnBridge.SubmitFlowControl` (HTTP face too — BUG-392) | `TestReviewACCoverage_*` 12 | ☐ |
| A-62-3 | Escalation/or-explained schema | `dod_explanation` schema pass | `TestRDodComplete_*` | ☐ |
| A-62-4 | Sprint handoff enrichment | handoff carries card choice + consequence; prose fallback → recommended | `TestHandoffEnrichment_*` 8 | ☐ |
| A-62-5 | Decision card UI | render + option_id submit + prose fallback — Desktop+TUI | DecisionCard TUI tests | ☐ UI |
| A-62-6 | Drift pause card UI | dev-mode drift ≥80 card (shared w/ CP-23) | — | ☐ UI |

### A-4 CP-64 — reproduce-first TDD gate

| ID | Case | Pass criteria | Auto | Status |
|----|------|---------------|------|--------|
| A-64-1 | RED→lock→GREEN on real bug | `bug-harness`: reproducer writes failing test → `r-reproduce` passes → test file locked read-only (abs+rel paths, BUG-388) → implement can't touch it → GREEN → done; outer run settles terminal (was stuck-running) | `internal/flowgate` + e2e | ☑ run-14071: RED `strutil_flag_test.go` confirmed FAIL → implement fixed RI clause (test file untouched) → validate green → audit DONE; run-13080: same RED leg on combining marks + child-authored CA note |
| A-64-2 | False alarm → fail-closed | green-on-arrival → reprompt "suite passed…not reproduced"; implement PENDING; cap reachable (BUG-391) | oracle suite | ☐ RE-VERIFY |
| A-64-3 | Compile-error wording | reprompt says "failed to compile" not "suite passed" (`[setup failed]` signature) | `classify_probe_test.go` | ☐ |
| A-64-4 | Gate gaming | fabricated RED (doesn't call target) rejected (BUG-389); tampered test dropped (BUG-387); lock bypass attempts denied (BUG-388/396/397) | `bug386..398` files | ☐ |

### A-5 CP-67 — contract-first scaffold + signature lock (still `todo/`)

| ID | Case | Pass criteria | Auto | Status |
|----|------|---------------|------|--------|
| A-67-1 | Happy path on devin + one more provider | scaffold stubs + RED → contract v1 hash-pinned → body-only fill → signature lock holds → validate green → audit → done | P-1..P-4 unit battery | ☑ run-6893 devin leg: stubs+RED → body-only fill → validate green → audit done. Second-provider leg still open |
| A-67-2 | **r-signature-lock arms** (was BUG-LIVE-CP67-1 CRITICAL) | frozen contract record must be the hash-bearing one; violation (rename/additive fn) → reprompt, not silent ship | `bug386_*` + `TestRuleSignatureLock*` | ☐ RE-VERIFY CRITICAL |
| A-67-3 | Gate rejections | real-logic scaffold → r-scaffold-red; all-green → reject; compile-broken → compile wording; locked-test edit denied | `TestRuleScaffoldRed*` ×15 | ☐ |
| A-67-4 | Renegotiation | `renegotiate_signatures` offered to coder → record-only → `synthesis_negotiation` → round++ → cap 5 escalate; owner-debate resolves (BUG-411) | unit | ☐ RE-VERIFY |
| A-67-5 | Restart mid-negotiation | run survives restart; no false-done; parked batch restored or surfaced (BUG-410/404) | unit | ☐ RE-VERIFY |

### A-6 CP-65 — tournament

| ID | Case | Pass criteria | Auto | Status |
|----|------|---------------|------|--------|
| A-65-1 | Standalone 2-provider cohort | devin + opencode candidates → isolated worktrees → arbiter → winner merges clean via `ApplyPatch` (never reached arbiter live before) | `tournament_*` suites | ☑ run-25153: grok+devin cohort, 3 rounds of isolated worktrees, human card pick `candidate-a` → merge DONE, patch landed (`strutil.go` GB3-5 fix + `strutil_crlf_test.go` + CA-004), `go test`/`go vet` green. run-20041: auto-pick topology live but empty winner exposed BUG-459 (fixed CA-956); run-21364 exposed BUG-460 worktree autoindex pollution (fixed CA-957); run-23455 verified empty-winner escalate |
| A-65-2 | Cap→auto-escalate | review cap → `tournament_escalation` child runs to completion (resumable, BUG-412); dedup — no `-2/-3` dup children (BUG-413/446) | `bug446_453_*`, cluster-h | ☐ |
| A-65-3 | Tie → decision card | card `[]any` payload; choice routes: candidate→merge / retry→fresh cohort / ask→park (BUG-414) | `TestTournamentTie*` | ☑ run-25153: round-3 tie 1.0000 → escalate card parked `WAITING_USER_APPROVAL` → `continue` feedback `candidate-a` captured via `captureDecisionChoice` → `resumeTournamentChoice` routed to `merge_and_audit` → stored snapshot patch applied, DONE. run-19067: same card path → `candidate-b` choice → escalate "no mergeable diff" (BUG-453 path live) |
| A-65-4 | Retry ≤2 → parent resume | back-edge bounded; parent resumes after tournament | `TestTournamentEscalation*`, `TestResumeParentAfterTournament` | ☑ run-25153: arbiter `retry` ×2 (tie 1.0000 each) → `parallel_rollout` back-edge re-spawned fresh cohorts with distilled failure brief in prompts (`Tournament attempt N failed: tie …` observed verbatim in round-2/3 candidate prompts) → bounded by `max_attempts` → round-3 escalate card → post-card `merge_and_audit` DONE = flow settled terminal. Parent-resume leg still unproven (standalone run, no parent) |
| A-65-5 | `.flowpilot` exclusion | candidate diff/patch excludes runner metadata dir (manager.go union — verify post-rebase) | worktree tests | ☑ run-25153: merged winner patch = `strutil.go`+`strutil_crlf_test.go`+`CA-004.md` only; candidate worktree `.flowpilot/contracts`/`canonical-pending` writes excluded by `:(exclude).flowpilot` pathspec (manager.go:258/270). AGENTS/CLAUDE autoindex stamps additionally blocked by BUG-460 guard |

### A-7 CP-41 — RAG harness flow mode

| ID | Case | Pass criteria | Auto | Status |
|----|------|---------------|------|--------|
| A-41-1 | Happy path | `flowRef:"rag-harness"` → `flow_context_package` → sentinel in implement prompt → `flow_validation_result` exit 0 → `flow_audit_draft` ready; no auto-commit | runner e2e | ☐ |
| A-41-2 | Validation retry + max | fail-once → `Retry 1/3`; always-fail → `failed_validation_max_retries`, no 4th | unit | ☐ |
| A-41-3 | Env error | missing binary → `skipped_env_error`, retryAttempt stays 0 | unit | ☐ |
| A-41-4 | Audit blocked | missing feature key → `blocked_missing_feature_key` | unit | ☐ |
| A-41-5 | flowRef respects working mode | turn-level `flowRef` denied in wrong mode (BUG-400) | `bug400_*` | ☑ live run-46461/46463: vibe run + `task-harness` → `working_mode_flow_forbidden`; dev run + `vibe-ingest` → same; also vibe + `vibe-sprint` (system-only) rejected |

### A-8 CP-43 + CP-55 — change contract & preflight/canonical

| ID | Case | Pass criteria | Auto | Status |
|----|------|---------------|------|--------|
| A-43-1 | Declared contract | `contracts.ndjson` `confidence:"declared"` + declared_paths | contract tests | ☐ |
| A-43-2 | Inferred contract | no contract → exactly one `r-contract` reprompt → `inferred` entry; reprompt turn carries gate-carried paths (BUG-425/439) | `bug425_*`, `bug439_440_*` | ☐ |
| A-43-3 | Scope drift → amend v2 | write outside declared_paths → WAITING_USER_APPROVAL → `agent-loop/amend` → v2 supersedes → resume | unit + live | ◑ run-6893: drift detect + park + re-gate verified live (undeclared `livebed` binary blocked; resolved by artifact removal + Continue-with-feedback). amend→v2 leg still open |
| A-43-4 | Pending→final canonical exactly-once + restart durability | SIGKILL between stage/finalize → resume → single Head write | `internal/changecontract` | ☐ |
| A-43-5 | Symlinked workspace | `/var`,`/tmp`-symlinked bed → freeze not falsely blocked (BUG-396) | `paths.go` tests | ☐ |
| A-55-1 | Freeze v1 pins writers | `frozen_contracts.ndjson` v1 + base_sha; writer constrained | e2e | ☑ run-6893: freeze DONE first pass; scaffold writer constrained to `strutil.go`/`strutil_test.go` — undeclared `livebed` write blocked by gate |
| A-55-2 | Amend→v2→resume | live on grok previously; re-run | live | ☐ |
| A-55-3 | Idempotent freeze post-SIGKILL | no planner re-fire | unit | ☐ |
| A-55-4 | Resume retry prompt carries contract scope (BUG-424) | reprompt text contains change.contract block | `bug424` ref'd tests | ☐ |

### A-9 CP-60 — vibe working mode ⚠ (most open debt lives here)

| ID | Case | Pass criteria | Auto | Status |
|----|------|---------------|------|--------|
| A-60-1 | Mode gate | `/vibe` on clean bed → only vibe flows armable; dev `1/2/3` cards absent in vibe | workingmode tests | ☑ live run-46461/46463: vibe↔harness and dev↔vibe-ingest both rejected `working_mode_flow_forbidden`; system-only `vibe-sprint` also rejected for user start |
| A-60-2 | Snake MVP end-to-end | vibe run → SS → CP → tasks → sprint → playable `snake` (build+run green). **Was PARTIAL: owner-debate parked forever (BUG-411, fixed) — re-run required** | unit | ☑ run-37268+run-41626: Task-3 sprint FULL engine chain DONE; Task-4/5 sprints via agent-orchestrated `vibe-sprint` children (run-42033/run-43155) — `go test ./...` green, `go vet` clean, `snake/cmd` renders+quits. BUG-462/463 found+fixed live. Caveat: CP-01 interactive-playthrough DoD awaits human sign-off (documented) |
| A-60-3 | Fail-closed probes | `rm -rf`-class → refused + parked | unit | ☐ |
| A-60-4 | Resume checkpoint matrix §11 | R-SS/R-CP/R-TK keep + delete demotion (Task→CP→SS→empty); N-CP/N-SS/N-TK/N-DEL new-flow skip — **all unchecked live** | — | ☐ DEFERRED-LIVE |
| A-60-5 | Sprint reuse freeze (BUG-368, fixed CA-823) | 2nd vibe-sprint on same run gets fresh contract (sprint-1 paths not reused) | `bug368_*` | ☑ run-37268: sprint-2 minted NEW contract v4 for coder step with Task-4 paths (loop/input/render) — Task-3's v2 (model.go sigs) untouched |
| A-60-6 | SS lock stamp (BUG-365, fixed CA-820) | ss_lock writes `status: approved` on disk; requirement park surfaces blocked card | `bug365_*` | ☑ run-34947: ss_lock confirm → all 3 SS docs carry `Status: approved` on disk; card surfaced as `flow_parked_awaiting_user` + pendingGate ok/cancel |
| A-60-7 | vibe-cp-ingest admission | non-CP input → 422 `invalid_cp_source` (BUG-399 fixed — re-verify) | `bug399` tests | ☑ run-37268: turn without sourceDocId rejected `invalid_cp_source`; with `sourceDocId` armed cp_reader |
| A-60-8 | Waiting-approval orphan reconcile (BUG-432) | flow done while child waits → child terminalized, no orphan | unit | ☐ |

---

## B. State-durable (nhánh 2)

### B-1 CP-51 — durable turn dispatch + recovery

| ID | Case | Pass criteria | Auto | Status |
|----|------|---------------|------|--------|
| B-51-1 | Crash mid-turn | `kill -9` during live turn → restart → `POST resume` → reconcile-not-retry; `dispatch.ndjson` seqs unique+contiguous (BUG-406) | `bug447_449_*`, dispatch tests | ☑ live run-46465: SIGKILL mid-devin-stream → restart → run `cancelled` (no phantom retry), partial file `docs/durability-drill.md` survived, dispatch.ndjson seqs 1..1754 contiguous no dupes; boot settle finalized leftover turn-44931 |
| B-51-2 | Stop mid-flow | children cancelled, parent terminal, stop gen advanced; no ghost RUNNING | unit | ☑ live run-48587 (BUG-464 build): interrupt mid-`ss_converter` child (run-48832 → cancelled, "interrupted by user"), hub escalated to WAITING card, then `agent-loop/stop` → run `cancelled`, converter keeps real FAILED, hub swept WAITING→CANCELED (BUG-464 fix live-verified), never-started nodes stay PENDING |
| B-51-3 | Post-stop + post-done follow-up | admitted, answered, persisted across restart (BUG-302/305/306/307/308) | unit | ☑ live run-46465: new turn admitted on cancelled run (turn-46477), answered, r-task gate reprompt (turn-46868) ran and settled — agent recovered deleted Task-1/2 into `done/` from git history |
| B-51-4 | Gate reprompt idempotency | ×2 reprompts → durable keys `…0001`→`…0002`, no turn replay | unit | ☑ live run-46465: r-task gate reprompted twice — `durable-run-46465-reprompt-…0001`→turn-46868, `…0002`→turn-49288 (distinct turns, no replay); `pending_gate_reprompt_gen` 1→2 monotonic; sessions.ndjson durable rows carry both keys |
| B-51-5 | Repair/uncertain surfaces | forced repair → listed + resolved atomically; `repair-resolution` replay → recorded outcome 200 not 502 (BUG-407); cancel_required resolvable (BUG-408) | `bug405..409` refs | ◑ **uncertain leg ☑ live ×2**: run-46465 turn-46467 (SIGKILL mid-stream) + run-49161 turn-49163 (SIGKILL mid-send) → both surfaced `uncertain` in dispatch-attention post-restart; resolve `abandon` → `terminal_cancelled` atomic; replay same resolutionId → HTTP 200 idempotent; stale rev → 409 dispatch_conflict; attention cleared. **repair_resolution leg ◑**: cancel_required window (SetCancelRequested fsync → terminal commit) completes in <12ms — 3 kill races all landed before/after it; corrupt-runtime path only exists in Supabase loader (bed uses local store). BUG-407 replay-200 + BUG-408 covered by unit tests |
| B-51-6 | Restart mid-flow restore | kill during child/synthesis → resume → steps+agent cards+timeline from durable rows; hub's unsent first prompt reappears; `run-*-turns.ndjson` identical pre/post | restart tests | ☑ live run-47170: SIGKILL mid-`ss_converter` child turn (run-47631) → restart → resume rehydrated run cancelled + steps from durable rows (converter CANCELED, hub ss_validator stamped RUNNING by cohort-join) → `resumeFrom: ingest_reader` gate → `ok` re-drove killed node: new child run-47655 spawned + completed, old child correctly shows cancelled; dispatch.ndjson 1..1819 contiguous no dupes. Found+fixed BUG-464: Stop after join left hub RUNNING ghost (sweep to CANCELED added) |
| B-51-7 | Quiet-flow self-recover | post-SIGKILL silent flow re-drives hub (BUG-404); stale terminal commits retry bounded (BUG-409) | unit | ◑ run-47170 (operator-resume re-drive) + run-49191: **autonomous boot re-drive verified** — `orphaned_work work=1` → turn-49193 re-dispatched with no operator input; provider cancel landed via durable stop fence (4s); terminal commit then failed closed `dispatch record revision is stale` — no double-commit, no retry storm. **Open observation**: the re-dispatched turn's record was already `terminal_cancelled` — a phantom provider send that the stop fence had to cancel (possible BUG-467: boot settle should skip records already terminal in dispatch.ndjson) |

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
| C-23-1 | 23 | Budget packer | wide-read turn → `[prompt-pack]` truncation fields | promptpacker tests | ☐ |
| C-23-2 | 23 | Drift ladder + single pause | failing turns → score events → `drift_pause_required` once at ≥80 → continue resumes | driftdetect + `bug430_*` | ☐ |
| C-23-3 | 23 | Vibe non-pause | same ladder in vibe → no pause | unit | ☐ |
| C-23-4 | 23 | Skillpack install | `.agents/skills` + `.claude/skills` populated, `version:` markers (BUG-415) | skillpack tests | ☑ bed: 15 skills in `.agents/skills` + mirrored `.claude/skills`, all carry `version:` frontmatter (e.g. flow-harness-contract v6) |
| C-35-1 | 35 | Feature resolve + history | "improve calc-core" → verified confidence, newest-last prior work | featurecatalog | ◑ run-6893: feature `str-utils` resolved; canonical.head/history sections empty (first-run feature — correct degrade), change.contract section carried declared scope |
| C-35-2 | 35 | r-ca gate | no CA note → reprompt→block; CA written → pass | runner gate tests | ☑ run-6893: audit tier-3 blocked done on missing CA note → operator wrote CA-001 → re-observe → audit DONE |
| C-35-3 | 35 | Oracle regression block | break pre-existing test → `regression_test_broke` | flowgate | ☐ |
| C-37-1 | 37 | History + CA inject on **all** providers incl. devin | prompt artifact contains feature-history block (BUG-376 allowlist) | unit | ◑ run-6893: flow_context_package events persisted per node (fcp-071af78c context, fcp-557a1ad5 test_signatures); change.contract + source.dependence sections observed in prompt artifacts |
| C-37-2 | 37 | Unknown feature key degrade | reprompt once, no crash | unit | ☐ |
| C-37-3 | 37 | Sticky/pivot + no cross-feature mixing | calc-core vs calc-format isolation | unit D/G | ☐ |
| C-54-1 | 54 | changed_paths in ledger + locus builder (frozen/diff/empty) + ranked history + `[context-rank]` tiers + chat.summary recency | runner.log 3-tier rank; locus correct | contextsync | ☐ |
| C-63-1 | 63 | gopls diagnostics live | `[lsp] lsp.start` + **sev1 diagnostics actually surfaced** (BUG-380 initialized-notify fix — re-verify end-to-end) | lsp tests | ☐ RE-VERIFY |
| C-63-2 | 63 | Degrade + doctor | missing binary → warn-once + `flowpilot doctor` MISSING exit 1 | cli/lsp | ☐ |
| C-63-3 | 63 | Crash budget | repeated crashes → session-wide disable, no respawn | unit | ☐ |
| C-66-1 | 66 | GitNexus bootstrap → knowledge artifacts | bootstrap ok; 0-process degrade graceful | knowledge tests | ◑ bed `.flowpilot/knowledge/` populated: system-overview.md, data-models.md, execution-flows.md, index.json (bootstrap ran); 0-process degrade leg not drilled |
| C-66-2 | 66 | Locus routing | planner gets `knowledge.flow`; coder does NOT; `candidateSources` wired (BUG-421) | unit | ☐ RE-VERIFY |
| C-66-3 | 66 | Audit hook differential non-blocking | incremental update touches only changed artifacts | unit | ☐ |

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
