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
- **Dispatch durability (CP-51)**: `prepared → send_claimed → send_started →
  terminal_completed` chain with envelope hashes observed on every provider
  turn across run-1/run-94 records.

Evidence convention: every row gets runId + log line / artifact path. UI-only rows are
marked `UI` — backend evidence still required where noted.

---

## A. Harness & Vibe core (nhánh 1)

### A-1 CP-58 — bug/task/cp harness review loops — `feature_key: agent-flow-engine`

| ID | Case | Steps → Pass criteria | Auto cover | Status |
|----|------|-----------------------|------------|--------|
| A-58-1 | task-harness happy path | `/flow task-harness` + GCD-style prompt (calc-core) → scout→context→plan_writer→plan_reviewer→plan_synthesis→freeze→test_signatures→implement→validate→reviewer→synthesis→audit→done. Writer/reviewer prompts contain "Templated file outputs" / "Bound input artifacts"; reviewer calls `submit_review_outcome` | e2e in runner suite | ◑ run-94: all nodes DONE through `implement`; `validate` dead-parked → BUG-455 (fixed, CA-952); reviewer→audit legs pending re-run |
| A-58-2 | Plan loop reject→re-entry same session | Force `changes_requested` (ask plan to include benchmark) → `flow_control_hub_done_continue_on_review_verdict`, writer re-enters **same** child; context/freeze stay DONE | `TestCP61HubDone/plan_synthesis_changes_requested_continues` | ☑ run-94: reviewer `changes_requested` → `plan_writer` re-entered on same child run-235 (rounds 2–4), `context`/freeze untouched |
| A-58-3 | Code loop isolated from plan loop | Force code reject → implement re-entry; `plan_*` + freeze remain DONE | unit matrix | ☐ |
| A-58-4 | Round cap 3 → escalate | Force 3 rejects → `blocked`/escalate card, no 4th round (was never forced live) | `agent_orchestrator_test.go` cap | ☐ DEFERRED-LIVE |
| A-58-5 | cp-harness slice-only | `/flow cp-harness` → `cp_plan_writer→cp_reviewer→cp_synthesis→task_splitter→audit→done`; exactly N Task files, additive, no implement nodes | `bug356_slice_audit_test.go` | ☐ |
| A-58-6 | cp-harness-smoke (clone) | Clone → run: continues through implement chain; `acceptance_nodes` preserved | — | ☐ NEVER-LIVE |
| A-58-7 | Regression canary | `rag-harness` + `review-loop` behave as pre-CP-58 | F3 test cmds §F | ☐ |

### A-2 CP-61 — done-verdict gate (machine PASS, not self-grade)

| ID | Case | Pass criteria | Auto | Status |
|----|------|---------------|------|--------|
| A-61-1 | All 3 hubs gate on cohort verdict | task-harness live run: `plan_synthesis` needs `plan_reviewer` PASS; `synthesis` needs `reviewer` PASS → audit; wrong-cohort PASS doesn't unlock | `TestCP61HubDone` 13×3 providers | ◑ run-94: plan_synthesis consumed plan_reviewer verdicts (changes_requested→blocked→approved arc observed); code-loop `synthesis` leg pending |
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
| A-64-1 | RED→lock→GREEN on real bug | `bug-harness`: reproducer writes failing test → `r-reproduce` passes → test file locked read-only (abs+rel paths, BUG-388) → implement can't touch it → GREEN → done; outer run settles terminal (was stuck-running) | `internal/flowgate` + e2e | ☐ RE-VERIFY |
| A-64-2 | False alarm → fail-closed | green-on-arrival → reprompt "suite passed…not reproduced"; implement PENDING; cap reachable (BUG-391) | oracle suite | ☐ RE-VERIFY |
| A-64-3 | Compile-error wording | reprompt says "failed to compile" not "suite passed" (`[setup failed]` signature) | `classify_probe_test.go` | ☐ |
| A-64-4 | Gate gaming | fabricated RED (doesn't call target) rejected (BUG-389); tampered test dropped (BUG-387); lock bypass attempts denied (BUG-388/396/397) | `bug386..398` files | ☐ |

### A-5 CP-67 — contract-first scaffold + signature lock (still `todo/`)

| ID | Case | Pass criteria | Auto | Status |
|----|------|---------------|------|--------|
| A-67-1 | Happy path on devin + one more provider | scaffold stubs + RED → contract v1 hash-pinned → body-only fill → signature lock holds → validate green → audit → done | P-1..P-4 unit battery | ☐ |
| A-67-2 | **r-signature-lock arms** (was BUG-LIVE-CP67-1 CRITICAL) | frozen contract record must be the hash-bearing one; violation (rename/additive fn) → reprompt, not silent ship | `bug386_*` + `TestRuleSignatureLock*` | ☐ RE-VERIFY CRITICAL |
| A-67-3 | Gate rejections | real-logic scaffold → r-scaffold-red; all-green → reject; compile-broken → compile wording; locked-test edit denied | `TestRuleScaffoldRed*` ×15 | ☐ |
| A-67-4 | Renegotiation | `renegotiate_signatures` offered to coder → record-only → `synthesis_negotiation` → round++ → cap 5 escalate; owner-debate resolves (BUG-411) | unit | ☐ RE-VERIFY |
| A-67-5 | Restart mid-negotiation | run survives restart; no false-done; parked batch restored or surfaced (BUG-410/404) | unit | ☐ RE-VERIFY |

### A-6 CP-65 — tournament

| ID | Case | Pass criteria | Auto | Status |
|----|------|---------------|------|--------|
| A-65-1 | Standalone 2-provider cohort | devin + opencode candidates → isolated worktrees → arbiter → winner merges clean via `ApplyPatch` (never reached arbiter live before) | `tournament_*` suites | ☐ DEFERRED-LIVE (needs 2 providers) |
| A-65-2 | Cap→auto-escalate | review cap → `tournament_escalation` child runs to completion (resumable, BUG-412); dedup — no `-2/-3` dup children (BUG-413/446) | `bug446_453_*`, cluster-h | ☐ |
| A-65-3 | Tie → decision card | card `[]any` payload; choice routes: candidate→merge / retry→fresh cohort / ask→park (BUG-414) | `TestTournamentTie*` | ☐ |
| A-65-4 | Retry ≤2 → parent resume | back-edge bounded; parent resumes after tournament | `TestTournamentEscalation*`, `TestResumeParentAfterTournament` | ☐ DEFERRED-LIVE |
| A-65-5 | `.flowpilot` exclusion | candidate diff/patch excludes runner metadata dir (manager.go union — verify post-rebase) | worktree tests | ☐ |

### A-7 CP-41 — RAG harness flow mode

| ID | Case | Pass criteria | Auto | Status |
|----|------|---------------|------|--------|
| A-41-1 | Happy path | `flowRef:"rag-harness"` → `flow_context_package` → sentinel in implement prompt → `flow_validation_result` exit 0 → `flow_audit_draft` ready; no auto-commit | runner e2e | ☐ |
| A-41-2 | Validation retry + max | fail-once → `Retry 1/3`; always-fail → `failed_validation_max_retries`, no 4th | unit | ☐ |
| A-41-3 | Env error | missing binary → `skipped_env_error`, retryAttempt stays 0 | unit | ☐ |
| A-41-4 | Audit blocked | missing feature key → `blocked_missing_feature_key` | unit | ☐ |
| A-41-5 | flowRef respects working mode | turn-level `flowRef` denied in wrong mode (BUG-400) | `bug400_*` | ☐ |

### A-8 CP-43 + CP-55 — change contract & preflight/canonical

| ID | Case | Pass criteria | Auto | Status |
|----|------|---------------|------|--------|
| A-43-1 | Declared contract | `contracts.ndjson` `confidence:"declared"` + declared_paths | contract tests | ☐ |
| A-43-2 | Inferred contract | no contract → exactly one `r-contract` reprompt → `inferred` entry; reprompt turn carries gate-carried paths (BUG-425/439) | `bug425_*`, `bug439_440_*` | ☐ |
| A-43-3 | Scope drift → amend v2 | write outside declared_paths → WAITING_USER_APPROVAL → `agent-loop/amend` → v2 supersedes → resume | unit + live | ☐ |
| A-43-4 | Pending→final canonical exactly-once + restart durability | SIGKILL between stage/finalize → resume → single Head write | `internal/changecontract` | ☐ |
| A-43-5 | Symlinked workspace | `/var`,`/tmp`-symlinked bed → freeze not falsely blocked (BUG-396) | `paths.go` tests | ☐ |
| A-55-1 | Freeze v1 pins writers | `frozen_contracts.ndjson` v1 + base_sha; writer constrained | e2e | ☐ |
| A-55-2 | Amend→v2→resume | live on grok previously; re-run | live | ☐ |
| A-55-3 | Idempotent freeze post-SIGKILL | no planner re-fire | unit | ☐ |
| A-55-4 | Resume retry prompt carries contract scope (BUG-424) | reprompt text contains change.contract block | `bug424` ref'd tests | ☐ |

### A-9 CP-60 — vibe working mode ⚠ (most open debt lives here)

| ID | Case | Pass criteria | Auto | Status |
|----|------|---------------|------|--------|
| A-60-1 | Mode gate | `/vibe` on clean bed → only vibe flows armable; dev `1/2/3` cards absent in vibe | workingmode tests | ☐ |
| A-60-2 | Snake MVP end-to-end | vibe run → SS → CP → tasks → sprint → playable `snake` (build+run green). **Was PARTIAL: owner-debate parked forever (BUG-411, fixed) — re-run required** | unit | ☐ RE-VERIFY |
| A-60-3 | Fail-closed probes | `rm -rf`-class → refused + parked | unit | ☐ |
| A-60-4 | Resume checkpoint matrix §11 | R-SS/R-CP/R-TK keep + delete demotion (Task→CP→SS→empty); N-CP/N-SS/N-TK/N-DEL new-flow skip — **all unchecked live** | — | ☐ DEFERRED-LIVE |
| A-60-5 | Sprint reuse freeze (BUG-368, fixed CA-823) | 2nd vibe-sprint on same run gets fresh contract (sprint-1 paths not reused) | `bug368_*` | ☐ RE-VERIFY |
| A-60-6 | SS lock stamp (BUG-365, fixed CA-820) | ss_lock writes `status: approved` on disk; requirement park surfaces blocked card | `bug365_*` | ☐ RE-VERIFY |
| A-60-7 | vibe-cp-ingest admission | non-CP input → 422 `invalid_cp_source` (BUG-399 fixed — re-verify) | `bug399` tests | ☐ |
| A-60-8 | Waiting-approval orphan reconcile (BUG-432) | flow done while child waits → child terminalized, no orphan | unit | ☐ |

---

## B. State-durable (nhánh 2)

### B-1 CP-51 — durable turn dispatch + recovery

| ID | Case | Pass criteria | Auto | Status |
|----|------|---------------|------|--------|
| B-51-1 | Crash mid-turn | `kill -9` during live turn → restart → `POST resume` → reconcile-not-retry; `dispatch.ndjson` seqs unique+contiguous (BUG-406) | `bug447_449_*`, dispatch tests | ☐ |
| B-51-2 | Stop mid-flow | children cancelled, parent terminal, stop gen advanced; no ghost RUNNING | unit | ☐ |
| B-51-3 | Post-stop + post-done follow-up | admitted, answered, persisted across restart (BUG-302/305/306/307/308) | unit | ☐ |
| B-51-4 | Gate reprompt idempotency | ×2 reprompts → durable keys `…0001`→`…0002`, no turn replay | unit | ☐ |
| B-51-5 | Repair/uncertain surfaces | forced repair → listed + resolved atomically; `repair-resolution` replay → recorded outcome 200 not 502 (BUG-407); cancel_required resolvable (BUG-408) | `bug405..409` refs | ☐ |
| B-51-6 | Restart mid-flow restore | kill during child/synthesis → resume → steps+agent cards+timeline from durable rows; hub's unsent first prompt reappears; `run-*-turns.ndjson` identical pre/post | restart tests | ☐ (agent-graph restore was PARTIAL — verify) |
| B-51-7 | Quiet-flow self-recover | post-SIGKILL silent flow re-drives hub (BUG-404); stale terminal commits retry bounded (BUG-409) | unit | ☐ |

### B-2 CP-59 — chat SSOT cross-provider

| ID | Case | Pass criteria | Auto | Status |
|----|------|---------------|------|--------|
| B-59-1 | Switch legs | devin→opencode (and back) → new leg, `includedTurnCount`, E-9 divider, target identity | chat SSOT tests | ☐ |
| B-59-2 | Same-provider switch | `handoff_same_provider` 409 → in-place | unit | ☐ |
| B-59-3 | Timeline | legs sorted, records deduped, dividers positioned | unit | ☐ |
| B-59-4 | Detached reattach | first prompt reattaches (no `chat_no_active_leg` — BUG-405 legState restore, re-verify post-restart) | `bug405` refs | ☐ |
| B-59-5 | Restart mid-multi-leg | kill+restart → timeline identical (chatSeq stable, E-9 idempotent) | unit | ☐ |
| B-59-6 | Drive sync G1-G7 | — | — | ☐ BLOCKED-ENV (no Drive creds) |

---

## C. Support CPs (context/LSP/knowledge — feed the core)

| ID | CP | Case | Pass criteria | Auto | Status |
|----|----|------|---------------|------|--------|
| C-23-1 | 23 | Budget packer | wide-read turn → `[prompt-pack]` truncation fields | promptpacker tests | ☐ |
| C-23-2 | 23 | Drift ladder + single pause | failing turns → score events → `drift_pause_required` once at ≥80 → continue resumes | driftdetect + `bug430_*` | ☐ |
| C-23-3 | 23 | Vibe non-pause | same ladder in vibe → no pause | unit | ☐ |
| C-23-4 | 23 | Skillpack install | `.agents/skills` + `.claude/skills` populated, `version:` markers (BUG-415) | skillpack tests | ☐ |
| C-35-1 | 35 | Feature resolve + history | "improve calc-core" → verified confidence, newest-last prior work | featurecatalog | ☐ |
| C-35-2 | 35 | r-ca gate | no CA note → reprompt→block; CA written → pass | runner gate tests | ☐ |
| C-35-3 | 35 | Oracle regression block | break pre-existing test → `regression_test_broke` | flowgate | ☐ |
| C-37-1 | 37 | History + CA inject on **all** providers incl. devin | prompt artifact contains feature-history block (BUG-376 allowlist) | unit | ☐ |
| C-37-2 | 37 | Unknown feature key degrade | reprompt once, no crash | unit | ☐ |
| C-37-3 | 37 | Sticky/pivot + no cross-feature mixing | calc-core vs calc-format isolation | unit D/G | ☐ |
| C-54-1 | 54 | changed_paths in ledger + locus builder (frozen/diff/empty) + ranked history + `[context-rank]` tiers + chat.summary recency | runner.log 3-tier rank; locus correct | contextsync | ☐ |
| C-63-1 | 63 | gopls diagnostics live | `[lsp] lsp.start` + **sev1 diagnostics actually surfaced** (BUG-380 initialized-notify fix — re-verify end-to-end) | lsp tests | ☐ RE-VERIFY |
| C-63-2 | 63 | Degrade + doctor | missing binary → warn-once + `flowpilot doctor` MISSING exit 1 | cli/lsp | ☐ |
| C-63-3 | 63 | Crash budget | repeated crashes → session-wide disable, no respawn | unit | ☐ |
| C-66-1 | 66 | GitNexus bootstrap → knowledge artifacts | bootstrap ok; 0-process degrade graceful | knowledge tests | ☐ |
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
- A-65-1 real winner merge — needs ≥2 working providers (claude unconnected historically)
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
