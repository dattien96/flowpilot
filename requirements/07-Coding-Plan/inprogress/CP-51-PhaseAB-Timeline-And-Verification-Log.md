# CP-51 Companion: Phase A/B Timeline + Verification & E2E Test Log

## Metadata

- Document ID: `CP-51-VERIFY`
- Title: `Phase A/B Timeline And Verification / E2E Test Log`
- Phase: `coding_plan` companion / verification log (not a new CP number)
- Status: `inprogress` (automation suites runnable; live desktop E2E checklist for human)
- Owner: `FlowPilot`
- Created: `2026-07-17`
- Last Updated: `2026-07-22`
- Parent Documents: [CP-51](./CP-51-Durable-Turn-Dispatch-State-Machine-And-Recovery-Reconciliation.md), [Task-238](../../08-Task/done/Task-238-Flow-Mode-Node-Edge-Behavior-State-Machine-Hardening-Audit.md), [BUG-288](../../09-BugFix/inprogress/BUG-288-Flow-Mode-Three-Tier-Gate-And-Change-Contract-Reentry-Gaps.md)
- Child Documents: none
- Related: Task-239…242 (Phase A), CP-50 / Task-244…247 (Phase B), Task-248…258 (CP-51 + Drive dispatch)
- Tags: `verification, e2e, timeline, phase-a, phase-b, cp-51, bug-288`
- Feature Keys: `agent-flow-engine`

## Purpose

1. **Ghi timeline** (để không nhầm 238 graph SM với CP-51 turn SM).
2. **Checklist E2E / regression** cho bạn chạy khi verify full feature Phase A + Phase B + CP-51.
3. **Lệnh `go test` đã có** (unit/integration trong repo) + **kịch bản live desktop** (thủ công).

---

## 1. Timeline (canonical note)

```text
[Trước 238]
  Flow Mode E2E “chạy được” nhưng ~25+ bug rời:
  timeline · restore step · hang synthesis · cohort · gate r-ca trên child
        │
        ▼
[Phase A — Task-238 charter + 239…242]
  238  catalog bất biến I-1…I-17 + 4 wave
  239  restore / step-transition log (T-10)
  240  ordering + synthesis settle
  241  cohort join matrix + stall (T-11)
  242  three-tier gate (T-9)
  → “đồ thị flow settle đúng chưa?”
  Turn provider vẫn: startTurn → turnInFlight → runTurn → SendTurn → finishTurn
        │
        ▼
[Phase B — CP-50 / Task-244…247]
  context sources, change.contract inject, producers, …
        │
        ▼
[Codex review FAIL trên nhánh Phase A+B]
        │
        ▼
[BUG-288 multi-round epic]
  Vòng 1…~14  chủ yếu FLOW (gate lifecycle, contract, cohort, restore, stop…)
  Vòng ~15…20 vá TURN (idempotency, prep:/bare, launch-ack, marker…)
  Post-Vòng-20: root cause TURN = turnInFlight + prep/bare
  → không còn vá điểm → CP-51
        │
        ▼
[CP-51 Task-248…257]
  DispatchRecord + CAS + recovery/settle/marker/retention/operator
  + Task-258 per-project log + Drive sync (không Supabase dispatch)
```

### Hai lớp state machine (đừng gộp)

| Lớp | Câu hỏi | Docs |
|-----|---------|------|
| **Flow graph** | Step/node/cohort/gate timeline đúng chưa? | 238–242, BUG-288 early rounds |
| **Provider turn** | Turn AI durable / crash-safe / Stop linearize? | BUG-288 late rounds → CP-51 |

---

## 2. Automated test inventory (chạy từ `apps/local-runner`)

Working directory:

```bash
cd apps/local-runner
```

### 2.1 Full package smoke (rộng — có thể có test flaky pre-existing ngoài scope)

```bash
go test ./internal/runner/ ./internal/flowgate/ ./internal/agentpack/ -count=1 -timeout 10m
```

### 2.2 Phase A — Flow graph (Task-238…242)

| Wave | Chủ đề | Lệnh / pattern |
|------|--------|----------------|
| **239 restore** | reconstruct / resume gate / parent stop gen | `go test ./internal/runner/ -count=1 -timeout 5m -run 'TestResumePendingFlowGate\|TestReconstruct\|TestRehydrate\|TestNormalizeResumed\|TestSeedTranscriptFallsBack'` |
| **240 ordering/synthesis** | one decision/turn, escalate, inline skip terminal | `go test ./internal/runner/ -count=1 -timeout 5m -run 'TestApplyFlowControl\|TestFlowInline\|TestTryAdvanceFlow\|TestEscalate\|TestFinishTurn'` |
| **241 cohort/stall** | cohort matrix | `go test ./internal/runner/ -count=1 -timeout 5m -run 'TestCohort\|TestStopAgentLoopCancelsCohort\|TestBuildCohortNote\|TestMemberAction'` |
| **242 three-tier gate** | gate tier + flowgate rules | `go test ./internal/runner/ -count=1 -timeout 5m -run 'TestChildGate\|TestRunTurnGate\|TestGate\|Tier'` <br> `go test ./internal/flowgate/ -count=1 -timeout 5m` |
| **Pack / review-loop** | built-in flows | `go test ./internal/agentpack/ -count=1 -timeout 2m` <br> `go test ./internal/runner/ -count=1 -run 'TestBuiltinOrchestration\|TestReviewLoopFlowConfig\|TestValidateChatOrchestration'` |

**One-shot Phase A bundle:**

```bash
go test ./internal/runner/ ./internal/flowgate/ ./internal/agentpack/ -count=1 -timeout 10m \
  -run 'TestResumePendingFlowGate|TestReconstruct|TestRehydrate|TestApplyFlowControl|TestFlowInline|TestTryAdvanceFlow|TestCohort|TestStopAgentLoopCancelsCohort|TestChildGate|TestRunTurnGate|TestGate|TestBuiltinOrchestration|TestReviewLoopFlowConfig|TestValidateChatOrchestration|TestFinishTurnStall|TestMemberAction'
```

### 2.3 Phase B — CP-50 / context + change contract → **MOVED**

> **Đã tách sang file catalog riêng:** [CP-43-Context-Source-Catalog-And-Test-Log.md](./CP-43-Context-Source-Catalog-And-Test-Log.md) §4 (per-source automated test inventory), §5 (GitNexus impact-analysis gap), §6 (live E2E B1–B12).
>
> Lý do: Phase B = context sources — bản chất thuộc CP-43/CP-44/CP-50 (registry + canonical.head/change.contract/source.excerpt), không thuộc CP-51 (durable turn dispatch). File catalog cũng là nơi test **tách biệt từng source** (cũ + mới + `source.dependence` sắp tới). File CP-51 companion này giờ chỉ giữ Phase A (flow-graph) + CP-51 (turn dispatch).
>
> Smoke nhanh khi cần một dòng: `go test ./internal/runner/ -run 'TestContext|TestChangeContract|TestCanonicalHead|TestSourceExcerpt|TestRenderFlowContext' -count=1 -timeout 5m` — chi tiết & per-source ở file catalog.

### 2.4 BUG-288 regression rounds (flow **và** early turn patches)

```bash
go test ./internal/runner/ -count=1 -timeout 10m \
  -run 'Test.*[Bb]ug288|TestDurable|TestGateSettle|TestWithGateEpoch|TestMarkPending|TestInjectFeatureHistory|TestIsFlowContext|TestObserveTurnScoped|TestRunTurnGatePass|TestRunTurnPostGate'
```

Hoặc theo file:

```bash
go test ./internal/runner/ -count=1 -timeout 10m \
  bug288_r11_test.go bug288_round11_test.go bug288_round12_test.go \
  bug288_round13_test.go bug288_round15_test.go bug288_round16_test.go \
  bug288_round17_test.go bug288_round18_test.go bug288_round19_test.go \
  bug288_round20_test.go
```

(Lưu ý: `go test` với list file cần prefix package path:)

```bash
go test ./internal/runner/ -count=1 -timeout 10m \
  -run 'Test' \
  # better: all round files via pattern
go test $(ls internal/runner/bug288*.go | sed 's|^|./|') -count=1 -timeout 10m
# portable:
go test ./internal/runner/ -count=1 -timeout 10m -run 'Test'  # too wide

# Recommended:
go test ./internal/runner/ -count=1 -timeout 10m \
  -run 'TestDurable|TestGateSettle|TestWithGateEpoch|TestMarkPending|TestInjectFeatureHistory|TestIsFlowContextHandoff|TestObserveTurnScoped|TestRunTurnGate|TestRunTurnPostGate|TestResumePendingFlowGate|TestFlowContextTrusted|TestChangeContractTrusted|TestMemberActionRetry|TestFinishTurnPreservesFailed|TestFinishTurnStall|TestParseDurable|TestSessionRuntimeBlob|TestServiceMarker|TestIdempotencyKeys|TestMarkerSecret|TestMarkerInit|TestBehaviorContextRender|TestComposeRetryPrompt'
```

### 2.5 CP-51 — durable dispatch (Task-248…258)

**Env optional for V2 live path in process tests:** most unit tests use memory/local store directly (no env).

```bash
# Core dispatch contract + crash foundation + CP51 tasks
go test ./internal/runner/ -count=1 -timeout 5m \
  -run 'TestDispatch|TestCrashMatrix|TestStopCAS|TestSessionIDIsNotAReceipt|TestProviderV2|TestFCPMarker|TestApplySessionRuntime|TestSnapshot_|TestRecoveryScanner|TestSettleDriver|TestOperatorAttention|TestCapabilityEvidence|TestLocalStore_Disk|TestCreatePrepared|TestActivation|TestRetryAsNew|TestRecordEffect|TestReleaseManifest|TestOpenRepair|TestEventTurnStarted|TestPreSend|TestReceipt|TestTerminalCommit|TestOwnRunStop|TestChildSendStarted|TestResolveUncertain|TestRecoveryAttach|TestEverySent|TestRunStopState|TestLocalLog_Startup|TestDispatchInspect|TestRepairResolutionHandler|TestRecoveryCommitGuards|TestDispatchAttentionHandlers|TestDispatchAttention_RebuildsAfterRestart|TestResumePendingFlowGate_CompletionEventKeyedByTurnID'

# Race (subset — longer)
go test -race ./internal/runner/ -count=1 -timeout 5m \
  -run 'TestDispatchCAS|TestDispatchStore_Contract|TestCrashMatrix|TestStopCAS|TestRecoveryScanner|TestSettleDriver|TestOperatorAttention'
```

**Enable V2 when testing full server path (manual / e2e with binary):**

```bash
export FLOWPILOT_DISPATCH_V2=1
# optional experimental providers:
# export FLOWPILOT_DISPATCH_V2_PROVIDERS=claude,gemini
```

### 2.6 V9 matrix / interactive e2e (nếu có trong tree)

```bash
go test ./internal/runner/ -count=1 -timeout 10m -run 'TestV9|TestInteractiveServiceE2E|Test.*E2E'
```

Provider live e2e (cần binary + account — **optional**, skip khi thiếu):

```bash
go test ./internal/runner/ -count=1 -timeout 15m -run 'TestCodexE2E|TestClaudeE2E|TestGrok' 
# often guarded by env; expect skip without credentials
```

---

## 3. Live desktop E2E checklist (thủ công)

Ghi ☐ khi pass. Chạy với desktop + `flowpilot serve`, project git thật.

**Preconditions (mọi scenario):**

| Item | Default / note |
|------|----------------|
| V2 dispatch | **default ON** (kill-switch: `FLOWPILOT_DISPATCH_V2=0`) |
| Project | Git repo open in desktop; note `project_id` (UUID under `.flowpilot/chats/`) |
| Evidence roots | `.flowpilot/chats/<project_id>/dispatch.ndjson`, `run-<id>-turns.ndjson`, runner stdout |
| Grok account | Home có auth (`~/.grok` hoặc account-home override); YOLO theo project setting |

**Cách ghi evidence (copy vào §5 notes):**

1. `run_id` / `turn_id` / wall-clock start→end  
2. Tail `dispatch.ndjson` cho turn đó (states + revision)  
3. 3–5 dòng server log quan trọng (`[turn-params]`, `[grok-acp]`, `[dispatch]` nếu có)  
4. Screenshot UI nếu attention card / gate card

### 3.1 Phase A — Flow graph

| # | Scenario | Steps | Expect | Evidence | ☐ |
|---|----------|-------|--------|----------|---|
| A1 | Review Loop spawn | Chat Mode → bug sub-mode → pick **Review Loop** → start | Agents board: hub + coder + reviewer slots; không chỉ 1 agent mồ côi | UI agents + timeline steps | ☑ done (2026-07-22) — residual DOD from §3.8 (hub must not write while flow children run) closed by **CA-367** (commit `038b3d7`, 2026-07-20): `shouldParkHubWriteTurn`/`scheduleRootGateRepromptOrPark` + durable continue-delegate suppress. Re-verified 2026-07-22: `run9437_hub_park_active_child_test.go`, `run9437_hub_park_restart_suppress_test.go`, `run9437_hub_park_turn_scoped_suppress_test.go` (8 tests incl. restart/idle-flush + turn-scoped N+1 non-suppression) all green, no skips. Spawn/hang/gate/stop/model/escalate cluster (CA-354…360) unit-covered per §3.8; general Review Loop happy path confirmed OK in ongoing operator testing. |
| A2 | Cohort join → synthesis | Để coder + cả 2 reviewers complete | Cohort join; hub **synthesis** turn chạy; không hang `RUNNING` mãi | ☑ `TestRun1264*`: coder + 2 reviewers done, synthesis submitted `approved`, terminal `done`; replay spawns restored before synthesis | ☑ done (2026-07-20) |
| A3 | YOLO off approval | Project YOLO=off; child tool/permission | Approval / gate card surface; stall policy Stop/Skip/Retry nếu stall | ☑ `TestRun2383*` + `TestChildGate*` / `TestGateSettle*`; expanded normal-chat restart `run-2334` / CA-363 parity for Grok, Codex, Claude. Flow Mode's YOLO-on lock is recorded in CA-378. | ☑ done (chat-mode YOLO-off) + **waived (flow-mode primary)** — decided 2026-07-22 |
| A4 | Stop mid-flow | Hub running; bấm Stop | Loop stopped; steps không stuck `RUNNING` giả; children cancel/settle | ☑ `TestStopAgentLoopCancelsCohortMemberAndJoins`, `TestStopAgentLoopCancelsParentTurn`, and `TestInterruptParentCancelsRunningChildAgents`: terminal settle with no ghost running | ☑ done (2026-07-20) |
| A5 | Restart mid-flow | Kill serve mid-flow; restart; reopen project | Timeline/step restore (239); không fake-cancel synthesis `DONE` | ☑ Live run-17987/run-18371 (Review Loop, `fix bug 1+1 != 2`): step-transitions + agent cards + synthesis restore correctly, but the hub's original first prompt bubble was silently dropped after restart — root-caused and fixed as **BUG-300** (`prependMissingPromptOnlyEvents` missing from the shared Claude/Codex `seedTranscriptFromDisk` path when the hub's turn 1 never itself calls the provider); regression test `TestClaudeRestartRestoresFirstPromptWhenHubNeverCalledProviderOnTurnOne`; see CA-380 | ☑ done (2026-07-21) |
| A6 | Tier-1 code gate | Coding child edit code ngoài/scope doc | Tier-1 doc/scope gate / reprompt **về đúng child**; không gate sau Completed mù | ☑ Live run-18997 (Review Loop, Chat→Bug, no Bug ID declared): gate correctly surfaced `⚠ Flow gate: declared bug mode but no bugfix document found` on the coder's own reprompt path (single card, no dual UI, no hang, hub reinvoked coder not itself); run reached Completed cleanly. Side finding while inspecting this exact run: its dispatch record never settled past `settle_pending` — root-caused and fixed as **BUG-301** (live gate-block/reprompt branch never called `scheduleSettleAfterGateBlock`, unlike the boot/resume path and the sibling gate-pass branch); regression test `TestLiveGateBlockSchedulesSettleDisposition`; see CA-381 | ☑ done (2026-07-21) |
| A7 | Validate retry (optional) | Flow có validate node (rag-harness…) | Retry lifecycle reinvoke implement (279-class) | Step retry count | ☑ automated coverage confirmed (2026-07-22 audit; pre-existing, previously undocumented here) — `apps/local-runner/internal/agentpack/flow-pack/flows/rag-harness.yaml` has a real `validate` node (`command.validate`, back-edge to `implement`); `flow_validate_audit_dispatch.go` (BUG-278/279) wires bounded retry. Tests: `TestTryAdvanceFlowFromNodeValidateFailingCommandRetriesCoder`, `…MaxRetriesEscalates` (cap → escalate, not silent pass), `…RetryReinvokesLifecycleReinvokeTarget` (same-child reinvoke), `…PassingCommandAdvancesToAudit`, `…RunsValidateSkippedNoCommandEscalates`; `phase_a_dod_test.go:TestParentFlowHasValidateNode`. No skips. Only the live desktop click-through (optional per this row's own title) remains open. |
| A8 | Parent stop gen | Stop parent khi child còn in-flight | Child không re-enter với gen cũ; stop generation advances | Log stop gen + child cancel | ☑ Live run-19500 (Review Loop, `fix bug 1+1 != 2`): coder done, 2 reviewers (run-19554 correctness/sonnet, run-19562 security/opus) in-flight → Stop main. Stop-fence advanced & rejected BOTH children's terminal commits (`[dispatch] terminal commit failed … dispatch record revision is stale`), both reviewers `cancelled`, parent stayed `cancelled` with NO synthesis/reinvoke after stop (loop did not revive). Boot recovery surfaced durable `cancel_required`+`repair_required` markers (BUG-289 A2/F-7) — operator Retry/Abandon cannot revive parent (ParentStopFence). Unit guard: `TestLiveStop_ParentStopFencesChildSend`. Transcript live-vs-disk content delta after restart (cancelled child shows provider output on reopen) accepted as cosmetic (Option A). Follow-ons found in this same session while continuing the stopped run — a post-stop follow-up hangs (**BUG-307**, stranded cohort note) and/or is 409'd with the prompt vanishing on restart (**BUG-308**, stopped-seal reversal) — are verified under A11. ☑ done (2026-07-21) |
| A9 | Inline skip terminal | Flow edge skip/terminal | Không advance sai node; synthesis settle một lần | ☑ Live **run-38462** (Workflow **tele-flow** `4a309d32-…`, Grok `grok-4.5`, prompt `fix bug 1+1 != 2`): multi-round continue (2×) then final hub decision `done` once → edge walk `synthesis --done--> tele-step (hub.notify) --done--> done`; `synthesis` DONE ×1 and `tele-step` DONE ×1 only; `flow_control_received(status=done)` ×1, `flow_run_complete_done` ×1, `flow_control_done` (loop `done`) ×1; no wrong-node advance / no second terminal row. Evidence: `.flowpilot/logs/features/agent-flow-engine/run-38462.ndjson`, `run-38462-step-transitions.ndjson`, turn-44703 tele-step prompt. Unit wave 240 still covers `TestTryAdvanceFlowThroughInlineSkipsTerminalRun` / `TestFlowInline*`. | ☑ done (2026-07-22) |
| A10 | Second turn same flow | Sau A2, user message thêm trên hub | Không re-resolve workflow→flowRef mù (chỉ turn 0); timeline append đúng | ☑ Live run-18997 (Chat mode, `chat_flow_ref`) and run-18371 (Workflow mode) both accepted a follow-up turn after loop `done`, previously blocked by `409 flow_stopped` — root-caused and fixed as **BUG-302** (admission gate in `startTurn` treated `done` as sealed for all run kinds); regression tests `TestChatFollowUpAllowedAfterFlowLoopDone` / `TestWorkflowRunAlsoAllowsNewTurnAfterFlowLoopDone`; see CA-382. Side findings during the same verification pass fixed as BUG-303 (transcript merge duplicate) and BUG-304 (test-only race). Follow-on UI staleness bug found during this same live test (post-done follow-up's TurnCompleted deferred behind a gate that force-blocks a "done" loop → desktop turn stream hangs until reopen) fixed as **BUG-305**; regression tests `TestChatFollowUpAfterFlowDoneBroadcastsLiveCompletion` / `TestTurnStartedAfterLoopDoneFlagReflectsAdmissionLoopState`; see CA-385. Second follow-on found on run-18371 restart (post-flow follow-up transcript misordered — original prompt sank to bottom because the agent-context-wrapped follow-up slot was skipped and the suppressed hub turn-1 shifted the positional overlay) fixed as **BUG-306**; regression tests `TestPostFlowFollowUpTranscriptKeepsPromptOrderOnRestart` (+ `…Grok` cross-provider variant, `TestIsOverlayableUserTurn` discriminator unit); see CA-386. **Residual run-24377 (2026-07-21):** multi-round Grok hub+coder shared one `provider_session_id`; hub seed loaded polluted session → follow-up mid-timeline + agents bottom-dumped. Fixed **CA-390** (flow hub with durable assistants rebuilds from per-run turn log; Claude/Codex same prefer gate). Tests `TestRun24377FlowHubPrefersTurnLogWhenSharedGrokSessionPolluted`, `TestBuildFlowHubTranscriptEventsFromTurnLogSkipsSystemPromptsKeepsSynthesis`, `TestPreferFlowHubTurnLogTranscriptGates`. Live re-open run-24377 ☐. | ☑ unit residual done (2026-07-21); live re-open ☐ |
| A11 | Post-stop follow-up (continue a stopped run) | Sau A8 (Stop khi cohort in-flight), chat tiếp một câu bình thường trên cùng run — cả live lẫn sau restart | Follow-up được admit + trả lời bình thường, được persist (không mất prompt sau restart); KHÔNG 409 flow_stopped, KHÔNG hang, KHÔNG "hub has made no progress" card | ☑ Two live-found bugs on this exact scenario, fixed together: (1) run-19500 — after Stop+restart the follow-up was admitted but **hung 2m0s** (hub-stalled card) because a cohort-join synthesis note was queued to `pendingAgentContext` unconditionally even though the loop had sealed by Stop; no reinvoke drains it, so it rode into the next prompt via BUG-122's `composeAgentContextBlock` → **BUG-307** (tests `TestStopAgentLoopCancelsCohortMemberDoesNotPoisonPendingContext` + `…Grok`, `TestLoopSealedForReinvoke`; CA-387). (2) run-19845 — the follow-up got **`409 flow_stopped`** (live and after restart) and the optimistic prompt **vanished** on reopen (a rejected turn is never persisted); the `"stopped"` root admission seal (BUG-302 kept it) is the cause → **BUG-308**, a deliberate reversal making a Stop-ped run continuable like a `"done"` one (tests `TestChatRunAllowsNewTurnAfterStopped`, `TestChatFollowUpAllowedAfterFlowLoopStoppedResumedFromDisk`, `…Grok`, `TestTurnStartedAfterLoopDoneFlagTrueForStoppedFollowUp`, `TestChildTurnStillRejectedWhenParentStopped`; CA-388). BUG-307 is a prerequisite for BUG-308 (an admitted follow-up must not inherit the stale note). `"blocked"` still seals; child-under-stopped-parent still 409s. | ☑ done (2026-07-21) |
| A12 | Child multi gate-reprompt (durable gen + pending-path docs) | Review Loop coding child hits **≥2** post-turn gate reprompts on the **same child run** (e.g. Missing Change Contract then remediation turn; or any second durable reprompt). Live found: hub `run-23820` + coder `run-23825` (Grok) — coder looked "done", hub never advanced reviewers | After reprompt #1 delivery, gen is **high-water** (not reset to 0); reprompt #2 uses a **new** durable idempotency key (`…0002`), starts a **new** provider turn (does **not** silent-replay completed turn-1); `r-ca`/`r-bug` on empty-GitDiff pending-path re-check still honor CA/BUG held in `WrittenPaths`; child can settle DONE → hub/cohort continue. Provider-agnostic (Case 1) | ☑ Live **run-23820** / child **run-23825** (Grok Review Loop, `fix bug 1 + 1 != 2`): turn-1 gate `r-contract` (mid-line `[Change Contract]` failed parse) → durable reprompt turn-24501; turn-2 declared contract (saved under parent) then second gate queue reused key `durable-run-23825-reprompt-000…001` → `startTurn` replayed completed turn, cleared intent, **no turn 3**, coder stuck `running`, step only `coder RUNNING`, hub wait-note forever. Fixed: `clearIntentFieldsLocked` keeps `pendingGateRepromptGen`/`pendingResumeGen` high-water; flowgate `HasChangeAuditNoteInPaths`/`HasBugFixDocInPaths` for pending re-check. **New tests only** (additive): `TestRun23820SecondDurableRepromptUsesNextGenNotFirstKeyReplay`, `TestRun23820ZeroingGenWouldReplayCompletedRepromptKey`, `TestRun23820CodeChangedSatisfiedByPendingCAPathWithoutGitDiff`, `TestRun23820CodeChangedStillFiresWithoutCAInDiffOrPaths`, `TestRun23820BugFixedSatisfiedByPendingBugPathWithoutGitDiff`, `TestHasChangeAuditNoteInPathsAbsoluteAndRelative`. CA-389. Unit verify 2026-07-21: new tests **pass**; related old patterns `TestRun9437*`, `TestDurable*`, `TestBug288*`, `TestBug289*`, full `./internal/flowgate` **green** (no old-suite edits). Live re-run Review Loop multi-reprompt still recommended as operator close-out. | ☑ unit done (2026-07-21); live re-run ☐ |
| A13 | Workflow mode post-done / post-Stop chat (+ restart) | **Workflow mode** (not only Chat→Bug): after loop **done** *or* after **Stop**, send a new ordinary prompt on the same run — live, then again after **server restart** | Same behavior as Chat→Bug (A10 done path + A11 stop path): follow-up admitted + answers; no `409 flow_stopped`; no Thinking hang; transcript survives restart; when follow-up completes UI shows **Completed** (not stuck Cancelled from prior Stop). Shared Case-1 paths — not a separate Workflow SM | ☑ Live operator verify **2026-07-21** (tiendat): Workflow mode new-prompt-after-done, new-prompt-after-Stop, and post-restart variants all **work OK**, parity with Chat→Bug. Shared code already covers both run kinds: **BUG-302**/305/306 (post-`done`, incl. `TestWorkflowRunAlsoAllowsNewTurnAfterFlowLoopDone`); **BUG-307**/308 + **CA-391** / run-33289 (post-Stop: release hub `run_stop` fence keep Generation, revive cancelled→running, TurnFailed if fence cancels); chat-ui status map prefers run `completed` over residual loop `stopped` after follow-up. | ☑ done (2026-07-21) |

#### A3 operator guide — YOLO off approval

**A3 primary (flow mode):**

1. Start desktop + runner on the target project; set project YOLO **off**.
2. Use **Workflow** mode with `grok-flow` / Review Loop. Use a prompt that causes the coder child to need a tool, write, or gate decision, for example `fix bug 1+1 != 2`.
3. Keep the run open until a child tool/permission/gate point appears.
4. Expect: approval/gate card surfaces in the UI while the flow remains coherent; no silent YOLO execution; no fake terminal state; if the run stalls, the operator policy surface offers Stop/Skip/Retry-style recovery instead of hanging forever.
5. Evidence to capture: screenshot of the approval/gate card, run id, child run id, relevant feature log lines, and dispatch/turn rows around the approval.

**CP-51 bonus smoke (normal chat restart):**

1. Switch to normal **Chat** mode, set YOLO **off**, and send a simple prompt that would normally need a tool or permission.
2. Quit/restart the desktop app and runner, reopen the same project/session, and verify the UI still reflects YOLO **off**.
3. Send the next normal chat prompt.
4. Expect: the new turn still respects YOLO off and surfaces approval instead of silently running tools; prior chat/run state restores without losing the setting.
5. Evidence to capture: before/after restart screenshots, run id/session id, and dispatch/turn rows for the post-restart prompt.

> **Expanded normal-chat restart check: ☑ done (2026-07-19).** CA-363 regression coverage verifies Grok, Codex, and Claude restore raw user prompts and place durable approval/question cards beside their owning turn even without provider timestamps.
>
> **A3 flow-mode primary approval: ☑ waived (2026-07-22, operator decision).** This precondition — a flow-mode child hitting a real OS tool/permission approval while the project's YOLO is **off** — was never live-verified, and **CA-378** (same day as the original "done" stamp, 2026-07-20) subsequently locked every Flow/Workflow-mode run to YOLO **on** product-wide (Settings checkbox forced checked+disabled, DB default flipped). The scenario this row asks for is therefore no longer reachable going forward; waived rather than marked done. Re-open only if a future change reintroduces a YOLO-off path inside Flow/Workflow mode.

#### A4–A10 operator guide — remaining Phase A flow checks

**A4 Stop mid-flow**

1. Start a Review Loop run and wait until the hub or at least one child is visibly active.
2. Press **Stop** on the main run, not only on a child card.
3. Expect: parent loop becomes terminal stopped/cancelled; active children stop or settle; step rail has no ghost `RUNNING`; composer returns to idle.
4. Evidence: before/after screenshots, run id, child ids, `run-<id>-step-transitions.ndjson`, and feature log lines around stop generation/cancel.

**A5 Restart mid-flow**

1. Start a Review Loop run and wait until a child or synthesis step is in-flight.
2. Kill/restart the runner process, then reopen/reload the desktop project.
3. Expect: timeline and step rail restore from durable state; running/waiting/terminal states match the last durable rows; synthesis is not fake-cancelled or fake-done.
4. Evidence: screenshot before kill, screenshot after reload, `run-<id>-step-transitions.ndjson`, root/child `run-*-turns.ndjson`, and server boot recovery logs.

**A6 Tier-1 code gate**

1. Run Review Loop on a repo state where coder can edit production code but omit or violate the expected change contract / audit / tests.
2. Let the coder complete or reach the post-turn gate.
3. Expect: Tier-1 gate surfaces on the **child/coder ownership path**; any reprompt goes to the correct child; the hub must not blindly gate after the child is already completed.
4. Evidence: gate card screenshot, child run id, gate reason, feature log rows for gate routing, and child dispatch/turn rows.

**A7 Validate retry (optional)**

1. Use a flow that has a validate node, such as a rag-harness-style validation node when available.
2. Force one validation failure, then allow the retry path to run.
3. Expect: retry lifecycle increments the step retry count; reinvoke targets the expected node; cap/terminal behavior remains bounded.
4. Evidence: step transition rows showing retry count, validation failure row, reinvoke row, and final terminal state.

**A8 Parent stop generation**

1. Start a Review Loop run, wait until a child is still in-flight, then stop the parent.
2. If possible, attempt a stale child completion/reentry after parent stop.
3. Expect: stop generation advances; stale child results cannot re-enter the parent with the old generation; UI remains terminal instead of reviving the loop.
4. Evidence: feature log lines for stop generation, child cancel/interrupted rows, and absence of later accepted old-generation reinvoke.

**A9 Inline skip terminal**

1. Use or configure a flow edge that can skip/terminal from an inline hub decision.
2. Trigger the skip/terminal branch once.
3. Expect: edge walk advances only to the intended terminal/skip target; synthesis/settle happens once; no duplicate decision card or second terminal row.
4. Evidence: step transitions around the inline decision, flow-control output, and final terminal row count.

> **Live closed 2026-07-22 (tiendat):** **run-38462** tele-flow — synthesis `done` → `tele-step` (`hub.notify`) → flow complete; single terminal settle. See §3.1 A9 Evidence column.

**A10 Second turn same flow**

1. After an A2-style flow completes, send one more user message in the same run/session.
2. Expect: workflow reference is not blindly re-resolved as a new turn-0 flow; timeline appends the new turn cleanly; prior flow steps stay terminal.
3. Evidence: log line like `[flow-ref-resolve] bailing, turnCount=...`, timeline screenshot, and turn rows showing only the new user turn appended.

**A11 Post-stop follow-up (continue a stopped run)**

1. Run an A8-style scenario: start a Review Loop, wait until at least one reviewer child is still in-flight, then press Stop on the main run.
2. Send one ordinary follow-up chat message on the same run (no special content — e.g. "what did we fix last time"), **live** (before any restart).
3. Restart the server (kill/relaunch `flowpilot serve`), reopen the same chat, and send another ordinary follow-up.
4. Expect (both step 2 and step 3): the follow-up is admitted and answers normally within a few seconds — no `409 flow_stopped`; no `Thinking...`/Stop-button hang; no "hub has made no progress" watchdog card. After a further restart, previously-sent follow-ups are still present in the transcript (not dropped).
5. Evidence: server log shows the follow-up's `[prompt]`/`[turn-params]` and normal completion (no 409 at admission, no ~2-minute gap before a `hub_stalled`/"no progress" line); the outgoing prompt does NOT contain a "[flow-engine joined result note]" / "call the flow's control tool" block — only the user's own message. Note: a **child** turn under a stopped/done parent, and a follow-up on a **blocked** loop, are still correctly rejected.

**A12 Child multi gate-reprompt (durable gen + pending-path docs)**

1. Start Review Loop (Chat→Bug or Workflow) with a coding task that will trip a **tier-1 gate** on the coder (e.g. code edit without a well-formed own-line `[Change Contract]` block, or omit CA/BUG doc first pass). Grok is a strong trigger; Claude/Codex must behave the same if they also hit ≥2 reprompts.
2. Let the first gate **reprompt** the **coder child** (not the hub) and complete that remediation turn.
3. If the gate queues a **second** durable reprompt on the same child (or force a second fail then remediation), watch the next turn actually **start** (new `[prompt]` / `turn-params` for a **new** `turn_id`, not silence after `[gate] child gate reprompt`).
4. Expect: no permanent `coder` `RUNNING` with only step `coder RUNNING` and hub stuck on "wait for agent"; after gate allow, step DONE and reviewers/cohort can spawn. Session must **not** only keep `durable-<child>-reprompt-000…001` after a second queue (should show `…0002` or a second live turn).
5. Unit close-out (no desktop):  
   `go test ./internal/runner/ -count=1 -timeout 2m -run 'TestRun23820'`  
   `go test ./internal/flowgate/ -count=1 -timeout 1m -run 'TestRun23820|TestHasChangeAuditNoteInPaths'`  
   Related old guards (must stay green, **untouched**):  
   `go test ./internal/runner/ -count=1 -timeout 3m -run 'TestRun9437|TestDurable|TestBug288|TestBug289'`
6. Evidence: parent/child run ids, `sessions.ndjson` last coder snapshot (`idempotency_keys`, `reprompt_attempts`, `pending_gate_code_paths`), runner.log lines for both gate reprompts + second `[prompt]`, step-transitions showing progress past `coder` RUNNING.

**A13 Workflow mode post-done / post-Stop chat (+ restart)**

> Parity checklist vs Chat→Bug (A10 + A11). Workflow uses the same hub admission / stop-fence / transcript paths — this row closes the **mode surface**, not a second code path.

1. Start a **Workflow mode** Review Loop (or equivalent workflow that reaches hub `done` or can be Stopped mid-cohort). Complete one happy path to **done**, *or* Stop mid-flow (A8-style).
2. **Post-done (or post-Stop) live:** send one ordinary new prompt on the same run (e.g. "tóm tắt lại" / "done hay bị cancel rồi?").
3. **Restart server:** kill/relaunch `flowpilot serve`, reopen the same project/run, send another ordinary prompt.
4. Expect (live + after restart): admit + normal answer; no `409 flow_stopped`; no multi-minute Thinking hang; prior follow-ups still in transcript; badge **Completed** when the follow-up finishes (not stuck Cancelled solely because an earlier Stop sealed the loop).
5. Evidence: mode = Workflow, run id(s), server log `[prompt]`/`turn-params` + terminal (not silent `terminal_cancelled` from `ErrRunStopFence`), UI before/after restart. Cross-check Chat→Bug on the same build if regression suspected — should match.

### 3.2 Phase B — Context / change contract → **MOVED**

> Live desktop checklist context (B1–B12, gồm cả `source.dependence` mới) đã chuyển sang [CP-43-Context-Source-Catalog-And-Test-Log.md](./CP-43-Context-Source-Catalog-And-Test-Log.md) §6. Chạy chung session desktop với Phase A/§3.3 nếu muốn verify full — chỉ khác file ghi checklist.

### 3.3 CP-51 — Durable turn (+ Task-258 Drive)

| # | Scenario | Steps | Expect | Evidence | ☐ |
|---|----------|-------|--------|----------|---|
| C1 | Happy path simple chat | Grok project, message ngắn (vd. `hello grok`) | `dispatch.ndjson`: `prepared` → `send_claimed` → `send_started` → `terminal_completed`; `run-*-turns.ndjson` có prompt + `grok_session` | State sequence + outcome `completed` | ☑ done (2026-07-22) — from C9's BEMplan `run-33208`/`turn-33210` (Claude, not Grok — dispatch layer is provider-agnostic per C9's own finding). Full clean sequence confirmed: seq1 `prepared` → seq2 `send_claimed` → seq3 `send_started` → seq7 `completion_event(outcome=terminal_completed)` → seq9 `graph_signal` → seq11 `dependents_release` → seq13 `finalizer`, `envelope_hash` identical across all three pre-terminal states. `run-33208-turns.ndjson` has the prompt. Cross-verified `prompt_sha256` byte-for-byte: `sha256("hello from project B test. app này lam gì")` = `390dd0e6a4818a09…` matches the envelope exactly. No `grok_session` line — expected, that turn-log kind is Grok-specific, not applicable to Claude. |
| C2 | Crash after send | Kill runner sau `send_started`, trước complete; restart | Record còn `send_started` hoặc recovery → `uncertain`/`settle`; **không** silent loss turn | Pre/post dispatch rows | ☑ done (2026-07-22) — same mechanism and evidence as **C17** (a stricter superset of this scenario): 3 live kills at/after `send_started` (0s/3s/25s delay) all recovered to `uncertain` cleanly, zero silent turn loss, zero dangling `send_started`. See §3.3 C17 for full detail — not re-run separately since it's the identical trigger point and code path. |
| C3 | Stop before send | Stop ngay sau prepared / trước provider accept | `stopped_before_send` / `terminal_cancelled`; **không** double `session/prompt` | State + single prompt in turns log | ☑ done (2026-07-22) — automated watcher called `POST /client/workflow-runs/{runId}/interrupt` directly (precise timing vs racing a human Stop-button click) the instant a fresh turn (`run-35063`/`turn-35065`, gate-sandbox, Claude) hit `state:"prepared"` — even earlier than "before send" strictly requires. Full sequence: `prepared`→`send_claimed`→`send_started` (the send itself still raced ahead ~112ms before the cancel took effect — `interrupt()` sets a cancel signal, it doesn't block an already-progressing CAS chain) →`transport_error` (payload decodes to `"claude start: context canceled"`, i.e. the interrupt's context-cancellation reached the live Claude call) → `completion_event`/`graph_signal`/`finalizer` all `outcome:"terminal_cancelled"` — matches one of the doc's two accepted outcomes exactly. `run-35063-turns.ndjson` has exactly one prompt entry, no duplicate. |
| C4 | Stop during stream | Stop khi model đang stream | Terminal cancelled/stopped; UI idle; không orphan `turnInFlight` | UI + terminal evidence | ☑ done (2026-07-22) — automated watcher waited for genuine `send_started` on a fresh long-prompt turn (`run-35068`/`turn-35070`, "explain Go's GC in detail", gate-sandbox, Claude), then deliberately delayed 5s so the model was actually mid-stream (visible answer text: "Go uses a concurrent, tri-color mark-and-sweep..."), then called `POST .../interrupt`. Clean resolution: `transport_error` ("context canceled") → `completion_event`/`graph_signal`/`finalizer` all `outcome:"terminal_cancelled"`. Single prompt in turns log, no duplicate. **UI idle confirmed live**: composer accepted new text input and Send button was active immediately after — no orphan `turnInFlight` blocking a follow-up. Same minor side-note as C16: top-bar badge read plain "Failed" for a user-initiated Stop, same precision gap already logged there. |
| C5 | Uncertain surface | Force uncertain (crash mid-send) | Desktop **Dispatch attention** card; **Inspect** (state/revision/settlePhase, redacted evidence) → **Retry-as-new** or **Retry-load** (repair) actions available, cancel-biased UI | Card + CAS row | ☑ done (2026-07-22) — using the `run-33581`/`turn-33583` uncertain record from C16. Card auto-surfaced (no manual refresh): "Dispatch attention · 1 item(s) · uncertain · run run-33581 · turn turn-33583" with actions **Inspect, Confirm cancelled, Mark completed, Mark failed, Retry as new, Abandon** (more operator actions than the doc's literal Retry-as-new/Retry-load pair — a superset, fine). Clicked Inspect → `GET /client/workflow-runs/run-33581/dispatches/turn-33583` 200, raw response: `{cancelRequested:false, envelopeHash:"e060f3f2…", intentOwnerRunID, outerIntentGen:0, outerIntentKey:"", revision:5, runId, settlePhase:"", state:"uncertain", stopOutcome:"", turnId}` — `revision` present and correct (matches dispatch.ndjson seq3310) but **not rendered on the compact attention card** (card only shows state/settlePhase/cancelRequested) — a real UI/data gap, minor. No raw prompt/response content in any field; `envelopeHash` is a hash as expected. **Caveat:** this record died pre-send (killed at `send_claimed`), so it never captured any real provider evidence — the redaction claim (no raw payload leak) is *consistent* here but not *exercised*, since there was nothing sensitive to leak. A proper C14 pass needs an uncertain/repair record that captured real provider bytes first (planned as part of C17's retest, killing after a genuine `send_started`). |
| C6 | Drive sync round-trip | Sync project chats → Drive; machine B restore same `project_id` | Session + **dispatch** log restore; no empty overwrite of newer local | Both machines files | ☒ **partial — perf bug found AND fixed same-day; restore-side still untested (2026-07-22)**. Sync-up mechanically works (real uploads landed: 9× `[chat-sync] dispatch log uploaded project=db51ec26-… bytes=3522204`, correct byte counts, no errors) — but batching gate-sandbox's 17 chats took 6-8 min, root-caused to `syncChatRunToDrive` (`chat_session_sync.go:537`, once per run in a batch) unconditionally re-exporting + re-uploading the **entire** project-wide `dispatch.ndjson` on every call. **Fixed this session (CA-397):** `syncDispatchLogToDrive` now caches the last-uploaded content hash per project and skips the round-trip when unchanged; 3 new tests pass (unchanged-skip, changed-reupload, per-project scoping — the last one deliberately using identical content across two projects to catch a wrongly-global cache); 42-test chat-sync regression sweep clean (1 pre-existing unrelated failure confirmed via `git stash` baseline, untouched by this fix); ~35-pattern CP-51 dispatch sweep clean. The desktop unsynced-count regression ("~12/17" → "15") observed live was **not** separately root-caused — may simply have been this redundant I/O making the batch look stalled; re-open only if it recurs post-fix. **Method A (direct Drive read via `flowpilot_drive` MCP) did not pan out**: `search`/`listFolder` from Drive root surfaced neither `chat-sessions/` nor `dispatch.ndjson`, most likely `drive.file` scope (visible only to files the calling app itself created) — the MCP tool and the desktop's own chat-sync are very likely separate OAuth client identities on the same Google account, so the MCP structurally can't see what the desktop uploaded. Method B (simulate machine B) not attempted. **Restore-side property ("no empty overwrite of newer local") still has zero live evidence.** |
| C7 | Provider allow-list (Gemini only) | Gemini opt-in only, via `FLOWPILOT_DISPATCH_V2_PROVIDERS=gemini`; Claude/Codex/Grok are **default ON** (no allow-list needed) | Without allow-list: Gemini stays V1, Claude/Codex/Grok run V2. With allow-list: Gemini also V2 | Log start path (`providerV2Enabled` decision) | ☑ done via existing unit tests (2026-07-22, re-run, all pass) — `TestProviderV2GeminiDisabled_ClaudeEnabled`, `TestCapabilityEvidence_GeminiAllowListOptIn` (allow-list flips Gemini to experimental V2, three-outcome only, still no `Accepted` seam), `TestCapabilityEvidence_CodexGrokClaudeNoAcceptedSeam` (Codex/Grok/Claude V2-enabled by default; corroborates the C14 Task-257 finding). No live desktop click-through — logic is env-var-gated and unit-covered directly; a live pass would only re-confirm the same `providerV2Enabled` decision already exercised. |
| C8 | Kill-switch | `FLOWPILOT_DISPATCH_V2=0`, new run | New turns pure V1; existing V2 rows still readable | Log kill-switch line | ☑ done via existing unit test (2026-07-22, re-run, pass) — `TestDispatchV2DefaultOn`: empty env defaults on; `"0"`/`"false"` kill-switch off; `"1"` keeps on. Live pass would need running the whole stack under `FLOWPILOT_DISPATCH_V2=0` and confirming V1 fallback end-to-end — not attempted (the env-parsing decision itself, which is what could regress, is unit-covered; the V1 code path it falls back to is the pre-CP-51 baseline, exercised implicitly by every existing V1-era test). |
| C9 | Concurrent two runs | Two projects / two runs parallel Grok | Separate dispatch files/rows; no cross project_id | Two `dispatch.ndjson` | ☑ done (2026-07-22) — live BEMplan `run-33208` (plain chat) concurrent with gate-sandbox `run-33182` (2-round Review Loop, coder+2 reviewers+synthesis), both Claude. `run-33208`'s full lifecycle (03:22:14–35) fell entirely inside gate-sandbox's coder-child RUNNING window (03:22:02.9–51.1) — genuine overlap, not just close timing. Both dispatch.ndjson state machines terminal_completed cleanly. Zero cross-content: BEMplan's file has 0 hits for `calc.go`/`gate-sandbox`/`fix bug 1`; gate-sandbox's file has 0 hits for `BEMplan`/`MPlan`/"project B" (case-insensitive full-file scan, not just run_id matching). Provider was Claude not Grok — fine, isolation keys on project_id/run_id, not provider. |
| C10 | Second turn session reuse | After C1, second message same run | Reuses provider session when possible; new turn_id; new prepared→terminal; **faster** than cold start (no full cold MCP if process warm) | Turns log `grok_session` + wall time | ☑ done (2026-07-22), timing caveat noted. Live: fresh gate-sandbox run `run-35045` (Claude), 2 plain-chat messages in a row (`hello, what's 2+2` then `and what's 3+3?`). **Session reuse definitively confirmed:** both turns' envelopes carry the identical `provider_session_id_at_prepare: "thread-35046"` — turn 2 did not spin up a new Claude session. New `turn_id` (`turn-35047` → `turn-35055`) and a full independent `prepared→send_claimed→send_started→terminal_completed` sequence for turn 2, both confirmed. **"Faster than cold start" not demonstrated here** (turn 1: 6.044s prepared→completion; turn 2: 6.180s — essentially identical): this specific runner process had already been extensively warmed by the day's C16/C17/C14 testing before this check even started, so "turn 1" of this test was never a genuine cold start with a real MCP-init cost to shed. Operator-accepted the session-reuse evidence as sufficient without forcing a fresh cold-boot re-measurement — the architectural property (reuse, not the raw wall-clock number) is what C10 primarily cares about. |
| C11 | Envelope / hash | Inspect prepared envelope | `envelope_hash` stable; `prompt_sha256` matches prompt bytes; model/yolo recorded | Prepared row fields | ☑ done (2026-07-22) — from C16's `turn-33583` prepared envelope: `envelope_hash` identical across `prepared`/`send_claimed` records; `model:"claude-sonnet"`, `yolo:true` recorded. `prompt_sha256` cross-verified byte-for-byte: `sha256("hello, what's 2+2")` = `06a1b105c9fede2d9fd48875c0dad863f2b41fe46393a61eed50d4cf83702ea8`, exact match. |
| C12 | Recovery attach epoch | After recovery path | `recovery_attach_epoch` increments when attach/settle; no double terminal | Revision sequence | ☑ done (2026-07-22) — demonstrated identically 3× (C16, C17, C14 retries): every recovered record showed `recovery_attach_epoch: 0` pre-crash → `1` post-boot-attach, exactly once per record; no case produced a second/competing terminal-ish row after the attach. |
| C13 | FCP marker cross-run replay reject | Capture a `flowpilot-fcp:<id>:<mac>` marker minted for run A's flow-coding handoff; replay same marker into a spawn for a different run B | Provenance check rejects marker (run A's `FCPMarkerProvenanceRunID` ≠ run B); child spawn does **not** trust the injected FCP context as authentic | Reject log line + child prompt shows marker rejected, not silently trusted | ☑ done via existing unit tests (2026-07-22, re-run, all pass) — `fcp_marker_replay_test.go`: `TestFCPMarker_SiblingWithoutProvenance_DoesNotSuppress`, `TestFCPMarker_RecordedProvenance_Suppresses`, `TestSpawnChildRun_StampsFCPMarkerProvenance`, `TestProvenance_RoundTripsAcrossRestart`. No live desktop repro attempted — this is an internal provenance-record check (no UI surface), the doc's own §6 Gaps already flagged it as unit-only. |
| C14 | Inspect never leaks raw payload | Force an uncertain/open-repair record with a real provider payload; call **Inspect** in UI | Response/UI shows canonical **hash** for receipt/terminal evidence and repair metadata only — never the raw provider payload or raw quarantine blob | Network response body (`receiptEvidence`/`terminalEvidence`/`openRepair` fields) | ☑ done via existing unit test (2026-07-22) — **live desktop repro is not currently possible, by design, not by test gap.** Attempted live 3× (gate-sandbox, Claude): killed a real turn at `send_claimed` (0 delay), at `send_started`+3s (9-process tree reaped, confirming genuine mid-flight work), and at `send_started`+25s — all three restarts landed on `state=uncertain` with **no** `receiptEvidence`/`terminalEvidence` field in any Inspect response. Root cause (code, not timing): `dispatch_live.go`'s `turnBridge.Accepted` — the sole caller of `CommitReceiptAndClearIntent` — carries the comment *"No current Codex/Grok/Claude adapter calls this (Task-257 unprovable)"*; `ReceiptEvidence` cannot be populated live by any provider today regardless of wait time. `TerminalEvidence` *is* set live on normal completion (`interactive_service.go:6644`, `bridge.Terminal(...)`), but its `PayloadCanonicalJSON` envelope there is `{type, error, final:"", source}` — `final` deliberately left empty — so even a captured live terminal record carries nothing sensitive to leak via that path. Verification instead rests on the existing, adversarial `TestDispatchInspect_RedactsCanonicalReceiptEvidence` (re-run 2026-07-22, **pass**): injects a record with `PayloadCanonicalJSON: {"secret":"do-not-leak-me"}`, asserts the Inspect HTTP response body never contains that string and the `receiptEvidence` object has no `payloadCanonicalJSON` key. This is the correct and currently-only-available way to verify C14; re-open the live path once Task-257 lands a real `Accepted()` caller. |
| C15 | Retry-as-new superseded guidance | On an uncertain record, click **Retry-as-new** twice (second click after first already advanced state/revision) | Second call returns 409 `dispatch_retry_superseded`; UI surfaces abandon guidance instead of silently failing or double-sending | UI message + HTTP 409 in network log | ☑ done (2026-07-22), with a doc-wording clarification. Live: called `POST .../retry-as-new` directly 3× against real uncertain records (run-33581/run-34067, gate-sandbox) to control exact CAS timing instead of racing UI clicks. (1) Valid `expectedRev` → 200 success, old record → `terminal_cancelled`, new turn minted. (2) Same now-stale `expectedRev` replayed → **409, but code was `dispatch_conflict`/"dispatch record revision is stale" (`ErrStaleDispatch`), not `dispatch_retry_superseded`**. (3) Valid rev + deliberately wrong `expectedEnvelopeHash`, no queued newer intent → 200 success (not rejected). Root cause read from code (`dispatch_store_memory.go:922-938`): the entire supersede-guard block (`ErrSuperseded`/`dispatch_retry_superseded`) is gated behind `r.OuterIntentKey != ""` — it only fires when the run has a **genuinely queued newer intent**, not merely a stale/mismatched revision or hash claimed by the caller. Neither live record had one queued, so calls (2) and (3) legitimately couldn't reach that branch — this is correct, narrowly-scoped behavior, not a bug: **the doc's own scenario description ("second click after already advanced") actually describes the `dispatch_conflict` path (which I did reproduce live), while the specifically-named `dispatch_retry_superseded` code needs a materially different precondition** (a real second live user prompt queued on the run while the first turn is still unresolved — not attempted live, since it may hit a further admission block per the attention card's own "Automated dispatch stays blocked until resolved" text). That exact path is verified instead via the existing, passing `TestRetryAsNewHandler_SupersededSurfacesAbandonGuidance` (re-run 2026-07-22, pass). Net: the double-submit safety property C15 cares about is confirmed live (a stale/replayed retry is always rejected, never silently double-executes); the specific `dispatch_retry_superseded` code path is unit-verified only, by operator decision (see run log). |
| C16 | Boot recovery auto-redispatch (no user action) | Kill runner process while a turn sits in `prepared`/`send_claimed` (before `send_started`); restart runner binary (not just reopen desktop window); do **not** touch the run in UI | `ScanDispatchRecoveryOnBoot` finds the record via `ListRecoverable`, reconstructs the run, and redispatches automatically — record advances past `send_claimed` without any user click | Server boot log (`ensureLiveAndRedispatch`) + dispatch.ndjson state advancing unattended | ☑ done (2026-07-22) — **"no user action" confirmed; "redispatches" clarified.** Live: automated watcher (byte-diff on `dispatch.ndjson`, no polling delay) hard-killed `apps/local-runner`'s standalone process the instant `run-33581`/`turn-33583` hit `prepared`→`send_claimed` (same microsecond, no `send_started` yet). Restarted the runner binary only; desktop window (separate process) was never touched. Within the same boot second, `boot-recovery-scanner` claimed the record unattended (`state=send_started, claim_owner=boot-recovery-scanner`) then transitioned it to `state=uncertain, recovery_attach_epoch=1` 469µs later — **no user click needed**, but the outcome is `uncertain`, not an automatic fresh resend/completion. This is correct per the already-documented Task-250 T-4 waiver (§6): once a record reaches `send_claimed` (the CAS immediately preceding the provider call), the runner cannot prove whether the killed process reached the adapter first, so it deliberately does not guess-resend — it surfaces `uncertain` instead (same safe branch as C17). The `prepared`-only-before-`send_claimed` window (the doc's literal "redispatch, not uncertain" case) measured **same-microsecond** in this run — likely unhittable by an external kill in practice. Desktop confirmed unattended: `Dispatch attention` card appeared with `uncertain · run run-33581 · turn turn-33583` and Inspect/Retry-as-new/Abandon actions, with zero manual refresh/interaction. **Side finding (not blocking):** the top-bar run-status badge read plain "**Failed**" for this same run, while the attention card correctly said "uncertain — needs an operator decision"; the blunter top badge risks reading as terminally dead to an operator glancing only at it. Also confirmed (boot-recovery sweep, not narrowly scoped to our one record): the same boot cycle also swept ~18 other long-stale non-terminal records from earlier CP-51 live-test sessions in this project (e.g. `run-19554`/`run-19562` from the A8 evidence) to `cancel required`, corroborating `ScanDispatchRecoveryOnBoot`/`ListRecoverable` work project-wide, not just for a hand-picked record. |
| C17 | Crash after send, no provider proof → uncertain (not dangling) | Kill runner **after** `send_started`/`provider_accepted` (model may genuinely be mid-response); restart runner binary | Boot scan claims the record and transitions it to `uncertain` (never left at `send_started` forever, never silently marked `terminal_completed` without proof); Dispatch attention card surfaces it for operator Inspect/Retry-as-new/Retry-load | dispatch.ndjson shows explicit `state=uncertain` row + `recovery_uncertain` audit entry; attention card in UI | ☑ done (2026-07-22) — clean match, zero doc-vs-code discrepancy (unlike C16). Live: automated watcher waited for a genuine `send_started` on a fresh gate-sandbox turn (`run-34067`/`turn-34069`, Claude), then deliberately delayed 3s before hard-killing the runner — the kill reaped a 9-process tree (real Claude CLI + MCP subprocess chain, `taskkill /F /T` correctly walked all descendants), confirming this was a genuine mid-flight crash, not a pre-send one. On restart, `boot-recovery-scanner` claimed it unattended and transitioned `send_started → uncertain` in 0.5ms (`recovery_attach_epoch` 0→1) — never dangling, never a fake `terminal_completed`. Dispatch attention card surfaced automatically. |

#### C2–C17 operator guide — durable dispatch checks

**C2 Crash after send**

1. Start a normal chat turn and watch `dispatch.ndjson` until `send_started` appears.
2. Kill the runner before a terminal row is written.
3. Restart runner and reopen the run.
4. Expect: record is recovered to `uncertain`/settle or another explicit recovery state; no silent turn loss and no fake success without provider proof.
5. Evidence: pre-kill row, post-restart rows, recovery log, and any attention card.

**C3 Stop before send**

1. Start a normal chat turn and stop it immediately, ideally while row is `prepared` or `send_claimed`.
2. Expect: terminal becomes `stopped_before_send` or `terminal_cancelled`; provider session/prompt is not duplicated.
3. Evidence: dispatch row sequence and `run-<id>-turns.ndjson` showing a single user prompt.

**C4 Stop during stream**

1. Start a longer normal chat prompt so model output streams.
2. Press **Stop** while streaming.
3. Expect: terminal cancelled/stopped, UI idle, no orphan `turnInFlight`, and no later ghost completion.
4. Evidence: UI screenshot after stop, dispatch terminal row, turn log, and server stop/interrupt log.

**C5 Uncertain surface**

1. Force or reuse an uncertain dispatch record, usually via C2/C17.
2. Reopen the desktop run.
3. Click **Inspect** on the Dispatch attention card.
4. Expect: card shows state/revision/settle phase and redacted evidence; Retry-as-new / Retry-load / cancel-biased actions are available.
5. Evidence: attention card screenshot, Inspect network response, and CAS revision rows.

**C6 Drive sync round-trip**

1. On machine A, sync project chats/runs to Drive.
2. On machine B or a clean local state, restore the same `project_id`.
3. Reopen the restored project and run history.
4. Expect: sessions plus dispatch logs restore; newer local data is not overwritten by older empty remote data.
5. Evidence: Drive sync logs, restored `.flowpilot/chats` files, and before/after project id.

**C7 Provider allow-list**

1. Run one turn without `FLOWPILOT_DISPATCH_V2_PROVIDERS=gemini`.
2. Run one Gemini turn with `FLOWPILOT_DISPATCH_V2_PROVIDERS=gemini`.
3. Expect: Claude/Codex/Grok use V2 by default; Gemini uses V1 unless explicitly allow-listed, then uses V2.
4. Evidence: provider start-path logs and dispatch presence/absence for each provider.

**C8 Kill-switch**

1. Start runner with `FLOWPILOT_DISPATCH_V2=0`.
2. Create a new normal chat run.
3. Expect: new turns use pure V1; old V2 rows remain readable when reopening prior runs.
4. Evidence: kill-switch log line, absence of new V2 dispatch rows, and successful old-run restore.

**C9 Concurrent two runs**

1. Start two projects or two runs in parallel, preferably both Grok.
2. Send a turn in each without waiting for the other to finish.
3. Expect: each run writes separate dispatch/turn rows; no cross-project session, prompt, or terminal pollution.
4. Evidence: two run ids, two dispatch files/row groups, and screenshots of both runs.

**C10 Second turn session reuse**

1. Complete C1 or any successful normal chat run.
2. Send a second message in the same run.
3. Expect: provider session is reused when possible; new turn id and new dispatch sequence are written; second turn should avoid the full cold-start cost when process is warm.
4. Evidence: `grok_session`/provider session ids in turn logs, wall-clock comparison, and second turn dispatch rows.

**C11 Envelope / hash**

1. Inspect a `prepared` dispatch row before terminal.
2. Recompute or compare stored hashes where tooling is available.
3. Expect: `envelope_hash` is stable, `prompt_sha256` matches prompt bytes, and model/yolo metadata is recorded.
4. Evidence: prepared row fields and any hash verification command output.

**C12 Recovery attach epoch**

1. Run a recovery scenario such as C2, C16, or C17.
2. Inspect dispatch row revisions before and after recovery attaches.
3. Expect: `recovery_attach_epoch` increments on attach/settle and only one terminal row wins.
4. Evidence: revision sequence, attach epoch fields, and terminal row count.

**C13 FCP marker cross-run replay reject**

1. Capture a `flowpilot-fcp:<id>:<mac>` marker minted for run A.
2. Inject/replay that marker into a child spawn or handoff under different run B.
3. Expect: provenance rejects the marker because the recorded run id does not match; child prompt must not trust the foreign FCP as authentic.
4. Evidence: reject log, child prompt/context excerpt, and run A/run B ids.

**C14 Inspect never leaks raw payload**

1. Force an uncertain/open-repair record with a real provider receipt or terminal payload.
2. Open Dispatch attention and click **Inspect**.
3. Expect: UI/network response shows hashes and repair metadata only; raw provider payload and raw quarantine blob are absent.
4. Evidence: Inspect response body and screenshot, with secrets/payloads confirmed absent.

**C15 Retry-as-new superseded guidance**

1. Open an uncertain record.
2. Click **Retry-as-new** once and wait for revision/state to advance.
3. Click **Retry-as-new** again on the stale card/request.
4. Expect: second request returns 409 `dispatch_retry_superseded`; UI gives abandon/superseded guidance, not a duplicate send.
5. Evidence: network 409, UI message, and dispatch revision rows.

**C16 Boot recovery auto-redispatch**

1. Start a turn and kill runner while state is `prepared` or `send_claimed`, before `send_started`.
2. Restart the runner binary only; do not click anything in desktop.
3. Expect: boot scan finds recoverable rows and redispatches automatically; state advances unattended past `send_claimed`.
4. Evidence: server boot log with `ensureLiveAndRedispatch`/recoverable scan and dispatch rows advancing after restart.

**C17 Crash after send, no provider proof**

1. Kill runner after `send_started` or provider accepted, while the model may still be working.
2. Restart runner.
3. Expect: boot recovery claims the row and marks it `uncertain`; it must not dangle forever at `send_started` and must not mark `terminal_completed` without proof.
4. Evidence: `state=uncertain`, `recovery_uncertain` audit/log entry, and Dispatch attention card with Inspect/Retry actions.

### 3.4 Cross-cut smoke

| # | Scenario | Expect | ☐ |
|---|----------|--------|---|
| X1 | Full review-loop happy path + V2 on | Flow complete + dispatch terminal rows per AI turn | |
| X2 | `go test` §2.2 + §2.4 + §2.5 green | No new red in those patterns | |
| X3 | Simple chat latency sanity | First Grok turn cold: expect **~15–40s** wall (session/new + MCP + model). Not a CP-51 bug if dispatch already terminal_completed | |
| X4 | Log noise sanity | Server may dump full `[grok-acp]` frames (initialize/session/new/tools). User prompt 10 bytes ≠ log size. See §3.5 | |

### 3.5 Worked example — run-1510 (`hello grok`) — 2026-07-17

**User prompt:** `hello grok` (10 bytes). **Run:** `run-1510` / **turn:** `turn-1512`. **Project:** `db51ec26-1a0f-4b92-8ceb-b03dc8e9b363`. **cwd:** `…/BE/gate-sandbox`. **Provider:** Grok 4.5, YOLO=true, reasoning=medium.

#### Timeline (wall clock from server log)

| t | Event | Ý nghĩa |
|---|--------|---------|
| 05:43:22 | `[flow-ref-resolve] … bailing, no workflowID set on this run` | **Chat Mode thuần**, không Flow Mode — **bình thường**, không lỗi |
| 05:43:22 | `[grok-acp] send initialize` | Spawn/handshake process Grok ACP |
| 05:43:23 | `initialize` result + MCP servers_updated (drive/jira/telegram) | Agent process up; large capability payload |
| 05:43:23 | `[prompt] … bytes=10` + `[turn-params] provider=grok model=grok-4.5 yolo=true` | User text thật sự chỉ 10 bytes |
| 05:43:23 | V2 dispatch: prepared → send_claimed → send_started (~ms) | **CP-51 path OK** — overhead gần như 0 so với Grok |
| 05:43:23–25 | `session/new` + announcements + settings + **huge** available_commands (skills local+user) | Session cold start; log dài vì dump full JSON |
| 05:43:25 | `_x.ai/mcp/init_progress` total=4 | MCP init (permission MCP + jira + telegram + …) |
| 05:43:25–50 | Model work | `apiDurationMs≈19907` (~20s); `inputTokens≈13296` (system/skills/cwd context, **không** phải 10-byte user) |
| 05:43:50 | `turn_completed` / `session/prompt` result / notification | Done; output ~121 tokens |

**Dispatch evidence (local file, verified):**

```text
seq1 prepared  → seq2 send_claimed → seq3 send_started → seq4 terminal_completed
outcome=completed  recovery_attach_epoch=1 on terminal
turns: prompt "hello grok" + grok_session 019f6d19-5417-7fb3-8421-2fb5a8dfdc76
```

#### Vì sao log “quá nhiều” và “lâu” dù chat simple?

| Nguyên nhân | Có phải bug CP-51? | Ghi chú |
|-------------|-------------------|---------|
| Runner log **mọi frame ACP** qua `logGrokFrameDebug` (full JSON sau redact) | Không | `initialize`, `session/new`, `available_commands_update` (hàng chục skills) = kilobytes/log line |
| **Cold session**: process + `initialize` + `session/new` + MCP | Không | First turn on new session; second turn thường rẻ hơn |
| **MCP list** (jira HTTP, telegram stdio, permission MCP, …) | Không (config) | `mcp/init_progress total=4` — chờ connect làm trễ wall clock |
| **Skills / repo context** nạp vào prompt | Không | ~13k input tokens dù user chỉ “hello grok” |
| Model API ~20s | Không | `apiDurationMs: 19907` — latency Grok cloud + reasoning medium |
| Streaming thought/message chunks | Không | UI/log có thể spam chunk; server log truncated middle là expected |
| `flow-ref-resolve` bail | Không | Expected cho plain chat (no workflowID) |
| Dispatch V2 4 states | Không | ~27ms prepare→send_started; terminal sau model | |

**Kết luận operator:** run-1510 **thành công** (V2 terminal_completed + UI notification). Cảm giác “chậm + log dài” = **Grok ACP cold path + verbose frame logging + MCP/skills**, không phải dispatch SM hang.

**Gợi ý cải thiện log (product, OOS CP-51):** truncate `available_commands` / settings frames; log summary thay vì full JSON; debug flag cho raw ACP; warm process pool; optional skip MCP cho chat-only projects.

### 3.6 Live **A1** — run-10389 (Review Loop) — 2026-07-17 — **PARTIAL / FAIL**

**Setup:** project `db51ec26-…` / cwd `D:\working\gate-sandbox`. Chat Mode → bug → **Review Loop**. Prompt: `fix bug 1+1 != 2`. Hub `run-10389` (Grok), coder child `run-10394` (Claude Haiku). Screenshot: `D:\working\gate-sandbox\Screenshot 2026-07-17 152139.png`.

#### Timeline (feature log + step-transitions + dispatch)

| t (local +07) | Event | Ý nghĩa |
|---------------|--------|---------|
| 15:19:24 | `flow_start_resolved` review-loop 4 nodes / 7 edges | Resolve OK |
| 15:19:24 | spawn `coder` → `run-10394` | Entry node OK (A1 partial) |
| 15:19:25 | coder V2 `turn-10399` prepared→…→`terminal_completed` | CP-51 child turn OK |
| 15:19:49 | `flow_control_escalate` → synthesis `WAITING_USER_APPROVAL` | Gate block on coding step |
| 15:20:12 | Continue → `hub_reinvoke_start_failed` | `post-turn gate still running` |
| 15:20:58 | coder `turn-10437` (after “Fix the code”) terminal | Gate decision path ran |
| 15:21:27 | coder `pending_flow_gate_settle=true`; xin write `calc.go` | Permission + settle window |
| 15:21:28 | escalate **#2** (+ “no progress since last continue”) | Dual UI lần 2 |
| 15:22:30 | Continue lại → same `hub_reinvoke_start_failed` | Hang dead-end |

**Gate reason (bundled):** no change-audit · no bugfix doc · inferred Change Contract · `Tests failed: TestAdd`. Sandbox still intentional: `Add` returns `a + b - 156`.

#### A1 checklist score

| Expect A1 | Result |
|-----------|--------|
| Review Loop start | ☑ |
| Hub + coder child (not orphan only) | ☑ |
| Reviewer slots / spawn | ☐ (chưa tới `coder.done` — graph đúng nhưng board chưa full) |
| Flow healthy after spawn | ☒ dual UI + reinvoke fail + hang |

→ **Không tick ☐ A1** cho đến retest sau fix.

#### Ba lỗi operator quan sát + root cause

1. **Dual gate UI ×2** — `FlowAwaitingUserCard` (“Needs your decision”) + `GateBlockModal` (“Regression gate — tests broke”) cùng lúc.  
   Backend: child coding gate vừa set `pendingGateBlock`/options **vừa** `applyFlowControl(escalate)` lên hub. Desktop render cả hai.  
   **Fix (CA-354):** nếu child đã có r-reg `gateOptions` → **không** escalate hub; chỉ decision card. Desktop: ẩn FlowAwaitingUserCard khi `gateBlock` mở.

2. **Hang coder / Continue vô dụng** — log:  
   `hub_reinvoke_start_failed … "post-turn gate still running; wait for gate pass/block before a new turn"`  
   `startTurn` reject khi `pendingFlowGateSettle \|\| postTurnGateCancel` (hub còn stale settle từ entry turn trong khi gate thật ở child).  
   **Fix (CA-354):** `resumeFlowWithFeedback` clear hub stale `pendingFlowGateSettle` khi `postTurnGateCancel==nil` trước reinvoke.

3. **Stop main (hub) không “ăn”, Stop child được**  
   - Composer Stop chỉ khi `status∈{running,waiting_*}` — **thiếu `blocked`**.  
   - `stop()` chỉ gọi `stopAgentLoop` khi `hasActiveParentAgentLoop`; miss → chỉ `interrupt(hub)` (hub không có turn).  
   - `handleStopAgentLoop` discard snapshot khi durable fence fail → UI không flip cancelled.  
   - `gateBlock` modal không clear khi Stop.  
   **Fix (CA-354):** ChatInput include `blocked`; broaden stop() + interrupt all children; error body includes `snapshot`; clear `gateBlock` on stop; client `RunnerApiError.snapshot`.

#### Evidence paths

```text
.flowpilot/logs/features/agent-flow-engine/run-10389.ndjson
.flowpilot/chats/run-10389-step-transitions.ndjson
.flowpilot/chats/run-10389-turns.ndjson          # hub prompt only
.flowpilot/chats/run-10394-turns.ndjson          # coder prompts
.flowpilot/chats/db51ec26-…/dispatch.ndjson     # V2 child terminals OK
D:\working\gate-sandbox\Screenshot 2026-07-17 152139.png
```

#### Retest A1 (sau rebuild desktop + local-runner)

1. Rebuild/restart serve + desktop.  
2. New run Review Loop; prompt có thể giữ `fix bug 1+1 != 2` **hoặc** spawn-only prompt.  
3. Expect: **một** gate surface khi TestAdd breaks (regression modal), không chồng “Needs your decision”.  
4. Submit “Fix the code” → grant write → green path hoặc single escalate without dual.  
5. Mid-flight: Stop trên **main** → loop `stopped`, children cancelled, modal gone.  
6. Continue sau escalate (non-option block) không log `hub_reinvoke_start_failed` vì stale settle.

### 3.7 Live **A1** — run-1618 (entry fail → hub hang) — 2026-07-17 — **FAIL → fixed CA-355**

**Setup:** same project `db51ec26-…`, cwd `…/BE/gate-sandbox`. Chat → bug → **Review Loop**. Prompt: `fix bug 1 + 1 != 2`. Hub `run-1618` (Grok), coder `run-1623` (Claude haiku) **without** a connected Claude account on this machine.

#### Timeline

| t (UTC) | Event | Ý nghĩa |
|---------|--------|---------|
| 14:22:30.56 | hub `turn-1620` prepared → send_claimed | V2 claim OK |
| 14:22:30.58 | hub `pending_flow_gate_settle=true` | **flowStartOnly** synthetic `EventTurnCompleted` stamped gate settle (no real gate) |
| 14:22:31.17 | spawn coder `run-1623` Wait=false | Entry node OK (A1 partial) |
| 14:22:31.21–26 | child prepared→send_claimed→send_started + `transport_error` | `no connected local account found for provider "claude"` |
| 14:22:31.26 | sessions: coder **failed**; hub **running** + fail note in `pending_agent_context` | Child dead; hub never reinvoked |
| (stuck) | step `coder` stayed **RUNNING**; dispatch hub `send_claimed`, child `send_started` non-terminal | Hang |

#### Root cause (CA-355)

1. flowStartOnly left hub `pendingFlowGateSettle` → later hub `startTurn` would 409 `gate_in_progress`.
2. Non-cohort `EventTurnFailed` only appended a note — no step FAILED, no hub reinvoke.
3. `finishTurn` emitLocked failed without CP-51 `Terminal` (child dispatch orphan).

#### Fix

- Clear synthetic settle on flowStartOnly + terminalize synthetic turn.
- Non-cohort flow child fail → step FAILED, clear hub settle, `maybeAutoReinvokeHubWithNote`.
- `finishTurn` → `commitFinishTurnDispatchTerminal`.

#### Retest

1. Rebuild/restart serve.
2. Review Loop with **no** Claude account → expect coder **failed**, hub reinvokes / reports error (not perpetual main active).
3. Connect Claude → happy path A1 again.

Evidence: `run-1618-*.ndjson`, feature log, dispatch seq 5–10; CA-355.

### 3.8 Live **A1** rollup — 2026-07-17 — **DONE** (2026-07-22: residual DOD closed)

**Status for checklist §3.1 A1:** ☑ **done** — spawn + hang/gate/stop/model/escalate clusters fixed and unit-covered (CA-354…360); residual **hub does not write while coder/agent children run** engineered and unit-verified via **CA-367** (2026-07-20, commit `038b3d7`) — see DOD below, now closed. Re-verified green 2026-07-22 (`run9437_hub_park_*` ×8, no skips). Live run-9437-class concurrent-write repro is a narrow race best covered by the unit suite; general Review Loop happy path has been confirmed OK across ongoing operator testing.

#### Code changes tested / fixed under A1 (change-audit)

| CA | Title (short) | Symptom fixed | Tests (additive) |
|----|---------------|---------------|------------------|
| **CA-354** | Dual gate UI + Continue hang + Stop hub weak | run-10389 dual card; `hub_reinvoke_start_failed`; Stop main weak | (prior commit on branch) |
| **CA-355** (gate modal) | keep-test-fix-code modal loop | run-11262 re-open r-reg modal every turn without write | `gate_fix_code_loop_test.go` (**new file**) |
| **CA-355** (run-1618) | Entry fail → hub hang | coder no-account fail; hub forever active; step stuck RUNNING | `run1618_entry_fail_hub_hang_test.go` (**new file**) |
| **CA-356** | Hang residuals H-A / H-B / H-C | preflight startTurn fail; all-entry spawn fail; F-0 stale settle forever-busy | same + suite in CA-356 |
| **CA-357** | Awaiting-user freeze | run-1675 hub keeps tool turns while “Needs your decision” open | `run1675_awaiting_user_freeze_test.go` (**new file**) |
| **CA-358** | Flow-scoped step model lookup | Review Loop coder wrongly got Context Coding’s claude-haiku | `TestResolveFlowNodeModelScopesByFlowRef` (**new test** in `flow_executor_test.go`) |
| **CA-359** | escalate then `rejected` clobber | run-2047 form lost → fake hub_stalled | `run2047_escalate_reject_clobber_test.go` (**new file**) |
| **CA-360** | Skip prose escalate w/ open cohort | run-5296 stale BUG-226 cancel new reviewers | `run5296_skip_prose_escalate_test.go` (**new file**) |

Related live runs used as evidence (not exhaustive): `run-10389`, `run-1618`, `run-11262`, `run-1675`, `run-2047`, `run-5296`, `run-9437` (hub+coder concurrent write race — residual).

#### additive-tests-only audit (this A1 fix set)

| Check | Result |
|-------|--------|
| New regression tests only in **new files** | ☑ `gate_fix_code_loop_test.go`, `run1618_*`, `run1675_*`, `run2047_*`, `run5296_*` |
| New `Test*` in existing file | ☑ only `TestResolveFlowNodeModelScopesByFlowRef` (CA-358) — **added**, no assertion rewrite of older cases |
| Pre-existing test assertion / skip / rename edits | ☑ **none** |
| Pre-existing call-site compile fix | ☑ only 4 lines: `resolveFlowNodeModel(ctx, node)` → `resolveFlowNodeModel(ctx, "", node)` after API gained `parentRunID` (no expected-value changes) |
| Soft skill pack | ☑ `skillpack/.../additive-tests-only/SKILL.md` added; hard gate rule deferred → [Task-260](../../08-Task/todo/Task-260-R-Additive-Tests-Gate-Rule.md) |

#### Residual — **CLOSED 2026-07-22 (CA-367, commit `038b3d7`, 2026-07-20)** — DOD below kept as historical record

> **Closure note:** all four DOD bullets below were implemented as `shouldParkHubWriteTurn` (hub `startTurn` rejects with `hub_parked` while any flow child is `RUNNING`/waiting) + `scheduleRootGateRepromptOrPark`/`stampHubContinueDelegatedDurable` (continue-delegate durably suppresses a same-turn hub gate reprompt, turn-scoped so it does not also eat a legitimate N+1 reprompt, survives restart/idle-flush). Tests: `run9437_hub_park_active_child_test.go`, `run9437_hub_park_restart_suppress_test.go`, `run9437_hub_park_turn_scoped_suppress_test.go` — re-verified green 2026-07-22. Graph delegate fan-out and hub.inline `flow_control` waits were deliberately left unchanged, per the "do not do" column below.

**DOD later: hub must wait writers (not graph wait)**

**Problem (confirmed live, e.g. run-9437):** after synthesis `continue`, flow correctly **reinvokes coder** (`delegate` fires immediately). Independently, **gate reprompt on hub** can start a hub write turn (e.g. create `BUG-908`) **while coder R1 is still RUNNING** (e.g. editing `CA-916` / `BUG-278`). Soft prompt `flow-start-wait.md` is not a hard lock. Topology can still complete; workspace exclusivity is wrong.

**Not the same as UI “Wait for result”** (`SpawnAgentInput.wait` / `waitForResult`, BUG-133): that blocks parent for a **tool/UI spawn**. Residual is **hub session park during flow children**, not `dependsOn` and not “graph waits for agent before advance”.

**Design constraint (do not break):**

| Keep | Do **not** do |
|------|----------------|
| `run: delegate` nodes fire immediately when edge activates | Serialize graph so reviewers cannot fan-out |
| `run: inline` waits for hub `flow_control` | Rely only on soft “don’t duplicate work” text |
| `dependsOn` / cohort join as written | Confuse with UI wait toggle |

**Definition of Done (implement later — new task when scheduled):**

1. **Hub posture `parked`:** while any flow child of the parent is `RUNNING` / `waiting_*` (or while active writer cohort open), **do not** `startTurn` a hub write/tool turn (gate reprompt, user-driven implement, gate artifact cure on hub).
2. **Gate routing after `continue`:** if `flow_control(continue)` already advanced a delegate coder, **do not** re-enter hub for missing docs on that same transfer; queue gate for next legitimate hub **inline** turn **or** reprompt the **coder** child (prefer writer ownership).
3. **Optional ordering:** evaluate hub-turn gate **before** committing `continue`/`done` edge walk when the violation is hub-owned; never apply continue then immediately gate-reprompt hub concurrent with child.
4. **Evidence:** feature log must show no hub `turn_started` with write tools overlapping child `RUNNING`; unit tests for park + gate suppress; live retest Review Loop (run class 9437) with concurrent gate missing-doc scenario.
5. **Out of scope for that DOD:** changing Review Loop YAML topology; promoting UI wait as the fix; Task-260 `r-additive-tests` (orthogonal test-suite hard rule).

**Suggested future task title:** `Hub park while flow children write / no gate re-enter hub after continue` (agent-flow-engine). Not opened as Task-NNN in this commit — tracked here as A1 residual DOD only.

#### What “done” means for operators (updated 2026-07-22)

- Safe to rebuild and retest A1 hang/gate/stop/model paths covered by CA-354…360 — all unit-covered.
- Residual DOD (hub-park while children run) is now implemented and unit-verified (CA-367); §3.1 A1 is ticked done on that basis. A dedicated live run-9437-class repro (force a hub gate-reprompt while a coder/reviewer is still RUNNING) has not been separately re-run as its own scenario — flag if you want that specific race exercised live before treating A1 as fully closed end-to-end.
- Full Review Loop green still needs YOLO/write + correct models, both already covered elsewhere in this checklist (A3 YOLO note, CA-358 model scoping).

---

## 4. Copy-paste “one shot” for CI-ish local verify

```bash
cd apps/local-runner

echo "=== Phase A-ish + pack + flowgate ==="
go test ./internal/agentpack/ ./internal/flowgate/ -count=1 -timeout 5m

echo "=== Phase A/B runner patterns ==="
go test ./internal/runner/ -count=1 -timeout 10m \
  -run 'TestCohort|TestApplyFlowControl|TestResumePendingFlowGate|TestChildGate|TestBuiltinOrchestration|TestContext|TestChangeContract|TestRenderFlowContext'

echo "=== BUG-288 rounds (durable/gate) + A12 multi-reprompt ==="
go test ./internal/runner/ -count=1 -timeout 10m \
  -run 'TestDurable|TestGateSettle|TestWithGateEpoch|TestMarkPending|TestInjectFeatureHistory|TestParseDurable|TestSessionRuntime|TestIdempotencyKeys|TestMarkerSecret|TestRunTurnGate|TestRunTurnPostGate|TestFinishTurnStall|TestMemberActionRetry|TestRun23820|TestRun9437'
go test ./internal/flowgate/ -count=1 -timeout 5m \
  -run 'TestRun23820|TestHasChangeAuditNoteInPaths|TestCheckRuleCodeChanged'

echo "=== CP-51 dispatch ==="
go test ./internal/runner/ -count=1 -timeout 5m \
  -run 'TestDispatch|TestCrashMatrix|TestStopCAS|TestRecoveryScanner|TestSettleDriver|TestOperatorAttention|TestCapabilityEvidence|TestFCPMarker|TestApplySessionRuntime|TestSnapshot_|TestProviderV2|TestSessionIDIsNot'
```

Ghi kết quả vào §5.

### 4.1 Live evidence commands (sau mỗi desktop scenario)

```bash
# project_id from desktop / sessions
PROJECT=db51ec26-1a0f-4b92-8ceb-b03dc8e9b363
RUN=run-1510

# Dispatch state machine for latest turns
tail -n 20 .flowpilot/chats/$PROJECT/dispatch.ndjson | jq -c '{seq,state:.record.state,turn:.record.turn_id,outcome:.record.outcome}'

# Turn log (prompt + provider session)
cat .flowpilot/chats/${RUN}-turns.ndjson

# Grep runner log (if redirected)
# rg 'run-1510|turn-1512|grok-acp|flow-ref-resolve|dispatch' serve.log
```

---

## 5. Run log (điền khi verify)

| Date | Who | Command / scenario | Result | Notes |
|------|-----|--------------------|--------|-------|
| 2026-07-17 | tiendat | Live **C1** run-1510 `hello grok` | ☑ pass (V2 terminal_completed) | See §3.5; wall ~28s cold Grok; log verbose ACP expected |
| 2026-07-17 | tiendat+grok | Live **A1** run-10389 Review Loop (`fix bug 1+1 != 2`) | ☒ **partial / blocked** → code fixed | See **§3.6**; dual gate + Continue hang + Stop hub → **CA-354**. |
| 2026-07-17 | tiendat+grok | Live **A1** run-1618 Review Loop + fake Claude (no account) | ☒ **hub hang** → code fixed | See **§3.7**; **CA-355** entry fail reinvoke + flowStartOnly settle + finishTurn Terminal. |
| 2026-07-17 | grok | Hang residuals **H-A/H-B/H-C** unit | ☑ pass | **CA-356**. |
| 2026-07-17 | grok | A1 cluster CA-355(gate)…CA-360 + additive-tests audit | ☑ **DONE PARTIAL** | See **§3.8**; CAs listed; residual hub-park DOD open (run-9437 class). |
| 2026-07-17 | grok | Live concurrent hub write vs coder (run-9437) | ☒ residual open | Gate hub BUG-908 while coder R1 wrote CA-916/BUG-278; topology OK. DOD in §3.8. |
| 2026-07-18 | tiendat+codex | Live **A2** run-1264 Review Loop (`fix bug 1+1 != 2`) | ☑ pass | Coder + `grok-review` + `my-reviewer` completed; synthesis submitted `approved` and flow terminal `done`. Replay regression fixed in CA-362: child spawn cards restore at durable times before synthesis, not appended at bottom. `run-333` remains cancelled/replay evidence, not A2 happy path. |
| 2026-07-19 | tiendat+codex | CP-51 **A3 expanded** normal-chat YOLO-off restart run-2334 | ☑ pass | CA-363 restores raw prompts and anchors approval/question cards by durable `ProviderTurnID`; new timestamp-free regression fixtures pass for Grok, Codex, and Claude. |
| 2026-07-22 | tiendat+claude | CP-51 **A3 flow-mode-primary** resolution decision | ☑ waived | Operator decision: waive rather than pursue further, since CA-378 (2026-07-20) locked Flow/Workflow mode to YOLO-on product-wide, making the YOLO-off-in-flow-mode precondition unreachable. See updated §3.1 A3 row + operator-guide note. |
| 2026-07-22 | claude | Test hygiene fix: `TestRun24377RealDiskStoreResumeOrder` unconditional `t.Skip` outside author's machine | ☑ fixed | Operator-approved (asked first, per additive-tests-only). Added embedded-literal fallback (same data as `TestRun24377LiveFixtureResumeOrderIsCorrect`) so the test always executes instead of silently skipping; zero assertion changes. `go vet` clean; `TestRun24377*` 3/3 pass, 0 fail. See CA-396. |
| 2026-07-21 | tiendat+claude | CP-51 **A5** Restart mid-flow, run-17987/run-18371 (Review Loop, `fix bug 1+1 != 2`) | ☑ pass (after fix) | Step-transitions, agent cards, and synthesis text all restored correctly; the hub's original first prompt bubble was missing after restart. Root cause: hub turn 1 spawns its entry child directly and never itself calls the provider, so its Claude session file starts directly at the round-0 synthesis turn — `overlayRawTurnPrompts` had no `turn_started` slot to overlay the raw prompt onto. Fixed as **BUG-300**: added the same `prependMissingPromptOnlyEvents` safety net Grok's restore path already had to the shared Claude/Codex `seedTranscriptFromDisk`. See CA-380; regression test `TestClaudeRestartRestoresFirstPromptWhenHubNeverCalledProviderOnTurnOne`; 191 pre-existing restart/resume/replay tests unaffected. |
| 2026-07-21 | tiendat+claude | CP-51 **A6** Tier-1 code gate, run-18997 (Review Loop, Chat→Bug, no Bug ID) | ☑ pass | Gate surfaced once (`⚠ Flow gate: declared bug mode but no bugfix document found`), reprompt correctly targeted the coder child (not the hub), no dual-UI/hang, run reached Completed. While inspecting this run's dispatch record, found it never settled past `settle_pending` — root-caused and fixed as **BUG-301**: the live post-turn-gate blocked/reprompt branch never called `scheduleSettleAfterGateBlock`, unlike the boot/resume path (`resumePendingFlowGate`) and the sibling live gate-pass branch. See CA-381; regression test `TestLiveGateBlockSchedulesSettleDisposition`; 227 pre-existing gate/dispatch/settle tests unaffected. |
| 2026-07-21 | tiendat | CP-51 **A13** Workflow mode post-done / post-Stop chat + server restart | ☑ pass | Operator live: new prompt after loop **done**, after **Stop**, and again after **server restart** — all OK in **Workflow mode**, same as Chat→Bug (A10/A11). Shared paths BUG-302/305/306 + BUG-307/308 + CA-391 (run-33289 fence) + chat-ui completed-status map. No Workflow-only delta found. |
| 2026-07-22 | tiendat+grok | CP-51 **A9** Inline skip terminal, run-38462 (tele-flow, Grok) | ☑ pass | Workflow tele-flow `4a309d32-…` / `grok-4.5`: after 2× `continue` rounds, final hub `done` once advanced `synthesis → tele-step (hub.notify) → done`; synthesis DONE ×1, tele-step DONE ×1; `flow_run_complete_done` + `flow_control_done` once each; no wrong-node advance / no double terminal. Logs: `run-38462.ndjson`, `run-38462-step-transitions.ndjson`. |
| 2026-07-22 | claude | CP-51 Phase A **full audit + doc reconciliation**: re-ran §2.2 Phase A bundle (104 tests, 0 fail, 3 packages) + targeted `TestRun9437\|TestRun23820\|TestRun24377` (15 pass, 0 fail, 1 expected skip); Explore-agent cross-check of every A1–A13 test citation against current code | ☑ A1 promoted done (CA-367 residual DOD confirmed closed, doc was stale); A7 promoted from stale-open to done (pre-existing rag-harness/`flow_validate_audit_dispatch` coverage was never documented here); A2/A4-A6/A8/A9/A11/A13 citations all confirmed still accurate, no change | Found: (1) **A3 contradiction** — table said done, operator-guide text said flow-mode-primary approval "remains open"; code confirms open is correct, and CA-378 (same-day YOLO-lock) may make the scenario unreachable going forward — flagged to user, not resolved unilaterally. (2) `run24377_real_disk_verify_test.go` unconditionally `t.Skip`s outside the original author's machine (hardcoded path) — looks covered but never runs elsewhere; flagged, not edited (pre-existing test, additive-tests-only gate requires asking first). |
| | | §4 one-shot | ☑ pass (2026-07-22, 104/104 + 15/15 targeted) | |
| | | Live A1 full pass retest | ☑ unit-closed 2026-07-22; dedicated live run-9437-class repro still optional | See §3.8 |
| | | Live A7–A9 | ☑ A7 automated-covered (2026-07-22); **A9 ☑** | A9 closed run-38462 (2026-07-22); A7 live desktop click-through still optional; A10 residual live re-open run-24377 still ☐; A11/A13 live closed |
| 2026-07-22 | tiendat+claude | CP-51 **C9** concurrent two runs: BEMplan `run-33208` (plain chat) + gate-sandbox `run-33182` (Review Loop, 2 rounds) both Claude | ☑ pass | Genuine wall-clock overlap (BEMplan's whole lifecycle inside gate-sandbox coder-child's RUNNING window); each project's `dispatch.ndjson` terminal_completed cleanly; zero cross-project content in either file. See §3.3 C9. |
| 2026-07-22 | tiendat+claude | CP-51 **C16** boot recovery, decoupled `just runner-dev`+`just desktop-dev-runner` on gate-sandbox; automated byte-diff watcher hard-killed the runner at `prepared`→`send_claimed` (same µs, before `send_started`) | ☑ pass (no user action) with a clarified outcome | Boot-recovery-scanner attached unattended in <1ms and moved the record to `uncertain` (not a fresh auto-resend) — correct per the already-documented Task-250 T-4 waiver (recovery can't prove no-send happened once `send_claimed` is reached, so it doesn't guess). Desktop attention card appeared with zero manual refresh. Side finding: top-bar run badge read plain "Failed", card correctly said "uncertain — needs decision" (minor UX precision gap). See §3.3 C16. |
| 2026-07-22 | tiendat+claude | CP-51 **C5** Inspect the C16 uncertain record (`run-33581`/`turn-33583`) | ☑ pass | Raw API response has `revision:5` (matches dispatch.ndjson) though the compact attention card doesn't render it (data present, not surfaced — minor gap); no raw content in any field. See §3.3 C5. |
| 2026-07-22 | tiendat+claude | CP-51 **C17** crash after genuine `send_started` (+3s real work, 9-process tree reaped on kill) | ☑ pass, clean match to doc (unlike C16, no wording discrepancy) | `send_started → uncertain` in 0.5ms on restart; never dangling, never fake-`terminal_completed`. See §3.3 C17. |
| 2026-07-22 | claude | CP-51 **C14** attempted live 3× (0s/3s/25s post-`send_started` kills) — all landed `uncertain` with no `receiptEvidence`/`terminalEvidence` | ☑ done via existing unit test, live repro not currently possible (by design) | Root cause in code: `turnBridge.Accepted` (sole `CommitReceiptAndClearIntent` caller) is explicitly uncalled by any current provider adapter (Task-257 gap) — no wait time fixes this. `TerminalEvidence` sets live but its payload envelope's `final` field is deliberately empty. Verified instead via `TestDispatchInspect_RedactsCanonicalReceiptEvidence` (re-run, pass): injects a `"do-not-leak-me"` secret, asserts it never appears in the Inspect response. See §3.3 C14. |
| 2026-07-22 | tiendat+claude | CP-51 **C1, C2, C7, C8, C11, C12, C13** closed via existing session evidence + re-run unit tests (no fresh live click-through needed) | ☑ done | C1: BEMplan `run-33208` full `prepared→…→terminal_completed` sequence, `prompt_sha256` verified byte-exact via manual sha256. C11: same, `envelope_hash` stability + hash-exact verification on `turn-33583`. C12: `recovery_attach_epoch` 0→1 observed identically 3× (C16/C17/C14). C2: same mechanism/evidence as C17 (superset), not re-run separately. C7/C8/C13: `TestProviderV2GeminiDisabled_ClaudeEnabled`, `TestCapabilityEvidence_GeminiAllowListOptIn`, `TestCapabilityEvidence_CodexGrokClaudeNoAcceptedSeam`, `TestDispatchV2DefaultOn`, `fcp_marker_replay_test.go`'s 4 funcs — all re-run, all pass. See §3.3 rows. |
| 2026-07-22 | claude | CP-51 **C15** retry-as-new double-submit guard — 3 direct API calls against real uncertain records (bypassing UI click-timing for precise CAS control) | ☑ done, doc-wording clarified | Valid retry succeeds; stale-revision replay correctly 409s (code `dispatch_conflict`, not the doc-named `dispatch_retry_superseded` — traced to `dispatch_store_memory.go:922-938`: the supersede-guard block only activates when `OuterIntentKey != ""`, i.e. a genuinely queued newer intent, which neither live record had). The doc's literal scenario ("second click after already advanced revision") in fact maps to the `dispatch_conflict` path just reproduced; the specific `dispatch_retry_superseded` code needs a materially different precondition, verified instead via the existing `TestRetryAsNewHandler_SupersededSurfacesAbandonGuidance` (re-run, pass). Operator-accepted this level of evidence rather than orchestrating the fuller live precondition. See §3.3 C15. |
| 2026-07-22 | tiendat+claude | CP-51 **C10** second-turn session reuse, fresh gate-sandbox `run-35045` (Claude), 2 plain-chat messages in a row | ☑ done, timing caveat noted | Both turns' envelopes carry the identical `provider_session_id_at_prepare:"thread-35046"` — definitive session-reuse proof. New turn_id + full independent dispatch sequence for turn 2 confirmed. "Faster than cold start" not demonstrated (6.044s vs 6.180s, essentially equal) because this runner was already warmed by the day's earlier C16/C17/C14 restarts — no genuine cold baseline existed within this test. Operator accepted the session-reuse evidence as sufficient; did not force a fresh cold-boot re-measurement. See §3.3 C10. |
| 2026-07-22 | tiendat+claude | CP-51 **C3** Stop before send, fresh gate-sandbox `run-35063`/`turn-35065` (Claude) | ☑ pass | Automated watcher called `POST .../interrupt` directly the instant the turn hit `state:"prepared"` (precise timing vs a human Stop click). Send still raced ahead ~112ms to `send_started` before the cancel landed, then `transport_error` ("claude start: context canceled") → `terminal_cancelled` cleanly (matches one of the doc's two accepted outcomes). Single prompt in turns log, no duplicate. See §3.3 C3. |
| 2026-07-22 | tiendat+claude | CP-51 **C4** Stop mid-stream, fresh gate-sandbox `run-35068`/`turn-35070` (Claude, "explain Go's GC in detail") | ☑ pass | Watcher waited for genuine `send_started` + 5s real streaming (visible partial answer on screen) before calling `POST .../interrupt`. Cleanly resolved `transport_error`("context canceled")→`terminal_cancelled`; single prompt, no duplicate. Operator-confirmed UI idle live: composer accepted new text immediately, Send active — no orphan `turnInFlight`. See §3.3 C4. |
| 2026-07-22 | tiendat+claude | CP-51 **C6** Drive sync (Method A: sync gate-sandbox's 17 chats live, then attempt direct Drive read via `flowpilot_drive` MCP instead of a real machine B) | ☒ attempted, 2 real findings, not cleanly closed | (1) Root-caused a genuine perf bug: `syncChatRunToDrive` re-exports+re-uploads the entire 3.4MB per-project `dispatch.ndjson` on every individual run in a batch (9× confirmed in the runner log for one 17-chat batch) instead of once — flagged as background task `task_d80cf120` rather than fixed inline (production sync code, deserves its own plan/tests). (2) Operator also observed the unsynced-count badge regress ("~12/17 synced" → later "15") — not root-caused this session, folded into the same task to investigate whether it's the same cause or separate. (3) Method A's direct Drive read came up empty (`search`/`listFolder` from root found no `chat-sessions`/`dispatch.ndjson`) — most likely `drive.file` OAuth scope only shows files the calling app itself created, and the MCP tool + desktop's chat-sync are very likely separate OAuth client identities even on the same account. Sync-up is confirmed mechanically functional (real successful uploads logged), but the restore-side property (no empty-overwrite of newer local) was not exercised. See §3.3 C6. |
| 2026-07-22 | claude | CP-51 **C6 follow-up**: fixed the perf bug flagged above same-day (per user request), `task_d80cf120` dismissed as superseded | ☑ fixed | `syncDispatchLogToDrive` (`dispatch_drive_sync.go`) now skips the Drive round-trip when the exported dispatch log's sha256 matches the last successful upload for that project (new `InteractiveService.dispatchLogSyncHash` cache, own dedicated mutex). New file `dispatch_drive_sync_test.go` (4 tests, additive-tests-only): unchanged-content skip, changed-content re-upload, failed-upload-does-not-poison-cache, per-project cache scoping. Regression: chat-sync sweep (1 pre-existing unrelated failure confirmed via `git stash` baseline — `TestRestoreChatRunFromDriveMissingActiveAccountHome`, untouched by this change) + ~35-pattern CP-51 dispatch sweep, both clean. See CA-397. |
| 2026-07-22 | claude | **BUG-309**: root-caused and fixed the desktop unsynced-count-regression symptom left open by the C6 perf-bug fix above | ☑ fixed | `projectRunHistory`'s in-memory branch (`interactive_handlers.go`) never carried `SyncStatus`/`SourceMachineID`/`SourceRunID` for runs still resident in `s.runs` — confirmed live via `GET /client/projects/{id}/workflow-runs` against the running dev server (project "Gate-sandbox": 15/17 runs missing `syncStatus`, uncorrelated with run status). Fixed by looking up each in-memory run's persisted session record once per request and copying the 3 fields across; persisted-augment branch (BUG-060) untouched. New file `bug309_test.go`: `TestProjectHistoryReportsSyncStatusForInMemoryRun`, proven to fail on the pre-fix baseline via `git stash`, passes on the fix; `TestRunHistoryEmptiesAfterServiceRecreation` (BUG-060) still passes. See BUG-309, CA-398. |
| 2026-07-22 | claude | **BUG-310 + BUG-311**: root-caused and fixed the genuine (not just mis-reported) 9-run sync-failure remaining after the BUG-309 fix | ☑ fixed | Live re-test post-BUG-309-fix still showed 9/17 Gate-sandbox runs failing real `POST .../sync-chat` calls. **BUG-310**: `resolveChatSessionTranscript` called `os.ReadFile` on Grok's session path, but `LocateSessionFile` returns a *directory* for Grok (chat_history.jsonl + sidecars), not a file — every Grok chat had never synced on any machine (`"Incorrect function"` on Windows / would be `EISDIR` elsewhere). Fixed by reading `chat_history.jsonl` inside the directory for the Grok case only; Codex/Claude unchanged. **BUG-311**: a cancelled chat with no session file anywhere correctly returns `session_unavailable`, but this permanent fact was never persisted, so it was silently retried forever — fixed by persisting `SyncStatus:"unsyncable"` on first failure and excluding it in `isSyncableRun` (distinct from `"failed"`, which still retries). New files `bug310_test.go`/`bug311_test.go` + one additive case in `navigatorHistory.test.ts`, all proven to fail on pre-fix baseline via `git stash`. Live re-verification after restarting `just dev`: all 5 Grok runs now `synced`, all 4 no-session cancelled runs now `unsyncable` — 0/17 remain in the unsynced count. Regression: only 2 pre-existing unrelated failures (`TestRestoreChatRunFromDriveMissingActiveAccountHome` CA-397, `TestNextAccountHomePathGrokUsesGrokHomePrefix` — machine-local account-slot state), both confirmed on baseline. See BUG-310, BUG-311, CA-399, CA-400. |
| | | Live B1–B5 | ☐ | |
| | | Live C1–C12 | C1/C2/C3/C4/C5/C7/C8/C9/C10/C11/C12 ☑ (C1/C2/C7/C8 via existing evidence/unit re-run, not fresh live click-through; C10 session-reuse confirmed, cold-vs-warm timing not isolated); **C6 ☑ DONE (2026-07-23) — sync-up AND restore-down both closed live. Sync-up: perf bug (CA-397), unsynced-count reporting (BUG-309), Grok sync-never-worked (BUG-310), perpetual-retry (BUG-311). Restore-down: path fix (BUG-312/Grok), timeline-content fix (BUG-313/turn-log), agent-card fix (BUG-314), and no-re-run-flow-on-followup fix (BUG-315/TurnCount) — all found+fixed same-day, live byte-identical round-trips + follow-up-does-not-re-trigger verified on the running Gate-sandbox runner. Residual: "no empty-overwrite of newer local" (BUG-091 guard) still lacks a dedicated live two-writer test; Method A direct Drive-read remains OAuth-scope blocked (cosmetic — restore works via the desktop path).** | |
| | | Live C13–C17 | C13/C15/C16/C17 ☑ (C13 unit-only, no UI surface; C15 double-submit guard confirmed live via 3 direct API calls, exact `dispatch_retry_superseded` code path unit-only by operator decision); C14 ☑ (unit-only, live not currently reachable — Task-257) | |
| | | Live X1–X4 | ☐ | |

---

## 6. Gaps known (không expect full green §10 từ unit hiện tại)

- **C6 Drive sync perf bug — FIXED same-day (2026-07-22, CA-397):** batch chat-sync was re-uploading the entire per-project `dispatch.ndjson` once per run instead of once per batch — 9 redundant 3.4MB uploads confirmed live for one 17-chat batch. Fixed via a per-project content-hash cache on `syncDispatchLogToDrive` (skips the Drive round-trip when nothing changed since the last successful upload); 4 new tests (unchanged-skip, changed-reupload, failed-upload-does-not-poison-cache, per-project cache scoping) pass; no regressions (chat-sync + ~35-pattern CP-51 dispatch sweep both clean, modulo one confirmed-pre-existing unrelated failure).
- **C6 desktop unsynced-count regression — FIXED same-day (2026-07-22, BUG-309/CA-398):** root-caused separately from the perf bug above. `projectRunHistory`'s in-memory branch (for runs still resident in `s.runs`) never set `SyncStatus`/`SourceMachineID`/`SourceRunID` at all — those fields are written only to the persisted store, asynchronously, after a run's own lifecycle ends, and `interactiveRun` never carried them. Any run created in the current process's uptime therefore always reported `syncStatus=""` regardless of how many times it was actually synced, so the Navigator's unsynced-count badge never reached zero. Confirmed live against the running dev server (project "Gate-sandbox": 15 of 17 runs missing `syncStatus`, uncorrelated with run status). Fixed by looking up each in-memory run's persisted session record and copying the 3 fields across; new regression test (`TestProjectHistoryReportsSyncStatusForInMemoryRun`) proven to fail on the pre-fix baseline via `git stash` and pass on the fix.
- **C6 unsynced count still stuck at 9 after the BUG-309 fix — root-caused as two further real bugs, both FIXED same-day (2026-07-22, BUG-310/CA-399 + BUG-311/CA-400):** after restarting the runner with the BUG-309 fix live, the same Gate-sandbox project's badge genuinely still showed 9 (not a reporting artifact this time — confirmed via direct `POST .../sync-chat` calls returning real errors). Two distinct causes: **(a) BUG-310** — Grok Drive sync had *never worked at all*, on any machine: `resolveChatSessionTranscript` called `os.ReadFile` on Grok's session path, but `LocateSessionFile` returns a *directory* for Grok (not a file, unlike Codex/Claude) — fixed by reading `chat_history.jsonl` inside it; live-reverified, all 5 affected Grok runs now `syncStatus: "synced"`. **(b) BUG-311** — a cancelled chat with no session file anywhere returns `session_unavailable`, correctly, but this permanent fact was never persisted, so it was silently retried and re-failed on every future batch forever — fixed by persisting `SyncStatus: "unsyncable"` on first failure and excluding it in `isSyncableRun`; live-reverified, all 4 affected cancelled runs now `syncStatus: "unsyncable"`. Combined: all 17 Gate-sandbox runs now resolve to either `synced` or `unsyncable` — 0 remaining in the unsynced count.
- **C6 restore-side unverified:** only the sync-UP half was exercised live; "no empty overwrite of newer local on restore" (the doc's core anti-regression claim) has no live evidence yet — needs either a genuine second machine or a locally-simulated clean-state restore. ~~Separately, Grok restore specifically is a known, pre-existing, larger gap (`restoreTargetPath` only supports Codex/Claude; Task-210 itself already noted this as out of scope) — not addressed by BUG-310, which fixed only the sync-upload side.~~ **Cập nhật 2026-07-23 (đã sửa, BUG-312/CA-406):** operator hit đúng gap này khi bấm Restore 1 chat Grok thật trong REMOTE CHATS (`409 sync_integrity_failed`, live-reproduced trước khi sửa). Root cause: `restoreTargetPath` chỉ có `case` cho Codex/Claude, Grok rơi vào `default`. Fixed bằng cách thêm `case ProviderKeyGrok` — tính lại thư mục đích từ `cwd` của máy hiện tại (dùng lại `grokSessionDirPath`, cùng cách `relocationTargetPath` đã làm cho cross-account relocation nội bộ) thay vì tái sử dụng nguyên path đã percent-encode theo cwd của máy nguồn (sẽ ghi vào thư mục `LocateSessionFile` không bao giờ tìm lại được trên máy khác). 3 test mới (unit path-computation, round-trip cross-cwd, end-to-end sync→restore) đều fail trên baseline pre-fix với đúng lỗi live, pass sau fix. Live re-verify: restart runner, restore lại đúng `run-22023` từng 409 trước đó → `200 restored`, file nằm đúng thư mục mã hoá theo cwd hiện tại. "No empty overwrite on restore" (phần đầu bullet này) vẫn chưa có evidence riêng — BUG-312 chỉ đóng phần "Grok restore hoàn toàn không hoạt động", không phải property anti-regression đó. **Cập nhật cùng ngày (BUG-313/CA-407):** restore chạy được xong thì lộ tiếp lỗi *nội dung* — chat restore mở lên mất prompt, agent card dồn xuống đáy, prose hub sai. Root cause: manifest sync chưa từng mang theo turn-log sidecar (`<runID>-turns.ndjson`) — input duy nhất mà toàn bộ reconstruct timeline sau restart đọc (raw prompts, chuỗi session-id per-turn, transcript_turn frames, anchor card). Fixed: manifest thêm `turnLog`, restore ghi lại sidecar (skip khi local đã có). Red-first TDD 3 test (fail đúng lỗi trên HEAD chưa fix, cross-provider Codex+Claude+Grok), live round-trip thật trên `run-44695`: sync → xoá sidecar → restore → **byte-identical**; restore lần 2 không nhân đôi. Lưu ý: chat đã sync bằng manifest CŨ vẫn restore thiếu prompt cho tới khi re-sync từ máy còn sidecar gốc; restore-side vẫn tải lại full dispatch.ndjson (~3.4MB) mỗi lần (mirror CA-397, follow-up riêng). **Cập nhật 2026-07-23 (BUG-315/CA-409 — C6 restore-side coi như ĐÓNG):** lỗi cuối lộ ra khi restore đã chạy đúng — restore 1 flow chat rồi gửi follow-up bất kỳ thì flow **tự chạy lại toàn bộ** (spawn lại coder + reviewers + synthesis) thay vì follow-up vào hub. Root cause cùng lớp BUG-313: manifest sync mang đủ mọi field flow-runtime **trừ `TurnCount`** → hub restore về `turnCount=0` → cả `startTurn` lẫn `resolveWorkflowFlowRef` (đều gate trên `turnCount==0`) tưởng đây là turn đầu của chat mới. Restart cùng máy không dính vì đọc `TurnCount` từ local store (persist đúng). Fix: manifest mang `TurnCount` + guard `restoredFrom` ở cả 2 đường start-flow (heal luôn manifest cũ). Red-first cross-provider Codex/Claude/Grok, full-sweep 18=18 y hệt baseline (không regression), live: re-sync→delete→restore `run-46797` cho `turn_count=1` (không phải 0), follow-up KHÔNG spawn child mới (giữ 8), hub tự trả lời (`turn_count` 1→2). Với BUG-312/313/314/315 đóng, **C6 restore-down đã verified live end-to-end**; chỉ còn property "no empty-overwrite of newer local" (BUG-091 guard) là chưa có test live 2-writer riêng. **Cập nhật cùng ngày (BUG-314/CA-408, bug khác — restart cùng máy chứ không phải Drive restore):** operator test cùng 1 flow shape trên cả 3 provider (Codex `run-46797`, Claude `run-53157`, Grok `run-45881`) — riêng Claude sau restart mất hẳn card round 2 (coder + reviewer), dù live đã chạy round 2 thật (coder commit fix, reviewer request changes, coder commit lại). Root cause: flow Claude chỉ có 1 reviewer (không cohort) — số activation của 1 node reinvoke được suy ra từ đếm prompt turn-log, bị chặn trần bởi heuristic "peer start-time wave" (suy round 2 từ thời điểm start của các child KHÁC); không có child khác thì không có wave, nên trần luôn co về 1 bất kể chạy thật bao nhiêu lần. Codex/Grok thoát nạn chỉ vì flow của họ có ≥2 reviewer/round nên tình cờ có đủ tín hiệu wave. Fixed: `resumedParentAgentAnnotations` ưu tiên đọc sidecar step-transition (Task-239, ghi RUNNING/DONE thật của từng node) khi node đó chỉ có đúng 1 child claim label — chặn bằng `labelCounts` để không đụng tới case Codex (label dùng chung bởi nhiều child request-run mới mỗi round, vẫn đúng, để nguyên đường cũ). Red-first: repro fail đúng lỗi trên baseline (`git stash` cô lập), pass sau fix, cross-provider Codex+Claude+Grok cùng shape, guard test khoá case Codex không bị đếm nhân 3. Full sweep baseline 17 fail (16 pre-existing + 1 flake TempDir không liên quan) vs fix 16 fail — không fail mới. Live re-verify trên chính 3 run thật của operator: Claude hiện đủ 2 card mỗi bên (coder+reviewer), Codex/Grok không đổi.
- ~~CP-51 **full barrier matrix B0…B8e + model suite** chưa đủ (Task-255 phase 2).~~ **Cập nhật 2026-07-22 (lỗi thời, đã sửa):** dòng này viết trước khi audit §10.1 ledger của [CP-51-Durable-Turn-Dispatch-State-Machine-And-Recovery-Reconciliation.md](./CP-51-Durable-Turn-Dispatch-State-Machine-And-Recovery-Reconciliation.md) — audit đó (cùng ngày) đối chiếu từng dòng ledger với test thật, chạy + mutation-verify, kết quả: **56/56 dòng ✅** (`GR` verify bằng `MU` thay thế do không replay lại được trên `6ea5417`; `CE-GEM` là non-goal xác nhận, không phải gap). Chỉ còn thật sự treo: `NR` có 18 fail khác phát hiện trong `internal/runner` nhưng nằm ngoài scope dispatch (đáng điều tra riêng, không chặn ledger này).
- Live provider e2e (Codex/Claude/Grok) phụ thuộc account/env.
- BUG-288 formal status vẫn `inprogress` đến re-review sạch + CP-51 closure.
- Desktop DispatchAttentionCard cần runner V2 + store (V2 default-on; kill-switch `=0`).
- **Log volume:** full ACP frame dump làm first-turn log “nặng” — không phải failure signal (xem §3.5).
- Cold Grok first turn 15–40s với MCP+skills là baseline hiện tại, không dùng làm failure cho C1 nếu dispatch terminal + reply OK.
- **C13–C17 chưa chạy live** (chỉ có unit test tương ứng: `TestFCPMarkerCrossServiceReplayRejected`-family cho C13, `TestDispatchInspect_RedactsCanonicalReceiptEvidence` cho C14, `TestRetryAsNewHandler_SupersededSurfacesAbandonGuidance` cho C15, chưa có unit test riêng cho boot-auto-redispatch không cần user action ở C16 — `reconcileOne`/`ScanAllRecoverable` test check redispatch được gọi, nhưng chưa có test end-to-end "process thật restart, không ai bấm gì" ở mức desktop; C17 có unit test hard-assert `TestScanAllRecoverable_EnumeratesEveryNonTerminalState_NotJustUncertain` xác nhận CAS đúng, nhưng chưa verify qua UI attention card thật).
- **Task-250 T-4 (provider reconcile/cancel-on-required) chính thức waived 2026-07-17**: mọi adapter hiện có (Codex/Grok/Claude) đều không có API query-by-id/cancel-by-id (Task-257 evidence), nên recovery không thể hỏi lại provider — nó fallback về `uncertain` một cách an toàn (atomic CAS, không mất, không trùng, luôn surface cho operator). Re-open chỉ khi có adapter mới hỗ trợ capability này.
- **A1 live residual (run-10389, 2026-07-17):** dual gate UI + hub reinvoke `gate_in_progress` + Stop hub weak — mitigated in CA-354; hang/model/escalate cluster CA-355…360 landed — see **§3.8 DONE (2026-07-22)**.
- **A1 residual DOD — CLOSED (CA-367, 2026-07-20; doc updated 2026-07-22):** hub-park + continue-delegate suppress engineered and unit-verified (`run9437_hub_park_*`, 8 tests green). A dedicated live run-9437-class repro (hub gate-reprompt attempted while a coder/reviewer is still RUNNING) is still recommended as a belt-and-suspenders operator check but no longer blocks ticking A1.
- YOLO/write permission for `calc.go` still needed for happy-path green TestAdd. Non-option multi-rule blocks (audit+contract without r-reg) still escalate to hub only (single card) — intentional.
- **Task-260** (todo): hard gate `r-additive-tests` — orthogonal to hub-park; soft skill already in pack.

---

## 7. Links nhanh

| Doc | Role |
|-----|------|
| [Task-238](../../08-Task/done/Task-238-Flow-Mode-Node-Edge-Behavior-State-Machine-Hardening-Audit.md) | Phase A charter |
| [Task-239…242](../../08-Task/done/) | Phase A waves |
| [CP-50](../done/CP-50-Context-Source-Completion.md) | Phase B |
| [BUG-288](../../09-BugFix/inprogress/BUG-288-Flow-Mode-Three-Tier-Gate-And-Change-Contract-Reentry-Gaps.md) | Multi-round review epic |
| [CP-51](./CP-51-Durable-Turn-Dispatch-State-Machine-And-Recovery-Reconciliation.md) | Turn durable SM |
| [Task-258](../../08-Task/todo/Task-258-Per-Project-Dispatch-Log-And-Drive-Sync.md) | Local+Drive transport |
