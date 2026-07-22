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
| C1 | Happy path simple chat | Grok project, message ngắn (vd. `hello grok`) | `dispatch.ndjson`: `prepared` → `send_claimed` → `send_started` → `terminal_completed`; `run-*-turns.ndjson` có prompt + `grok_session` | State sequence + outcome `completed` | |
| C2 | Crash after send | Kill runner sau `send_started`, trước complete; restart | Record còn `send_started` hoặc recovery → `uncertain`/`settle`; **không** silent loss turn | Pre/post dispatch rows | |
| C3 | Stop before send | Stop ngay sau prepared / trước provider accept | `stopped_before_send` / `terminal_cancelled`; **không** double `session/prompt` | State + single prompt in turns log | |
| C4 | Stop during stream | Stop khi model đang stream | Terminal cancelled/stopped; UI idle; không orphan `turnInFlight` | UI + terminal evidence | |
| C5 | Uncertain surface | Force uncertain (crash mid-send) | Desktop **Dispatch attention** card; **Inspect** (state/revision/settlePhase, redacted evidence) → **Retry-as-new** or **Retry-load** (repair) actions available, cancel-biased UI | Card + CAS row | |
| C6 | Drive sync round-trip | Sync project chats → Drive; machine B restore same `project_id` | Session + **dispatch** log restore; no empty overwrite of newer local | Both machines files | |
| C7 | Provider allow-list (Gemini only) | Gemini opt-in only, via `FLOWPILOT_DISPATCH_V2_PROVIDERS=gemini`; Claude/Codex/Grok are **default ON** (no allow-list needed) | Without allow-list: Gemini stays V1, Claude/Codex/Grok run V2. With allow-list: Gemini also V2 | Log start path (`providerV2Enabled` decision) | |
| C8 | Kill-switch | `FLOWPILOT_DISPATCH_V2=0`, new run | New turns pure V1; existing V2 rows still readable | Log kill-switch line | |
| C9 | Concurrent two runs | Two projects / two runs parallel Grok | Separate dispatch files/rows; no cross project_id | Two `dispatch.ndjson` | |
| C10 | Second turn session reuse | After C1, second message same run | Reuses provider session when possible; new turn_id; new prepared→terminal; **faster** than cold start (no full cold MCP if process warm) | Turns log `grok_session` + wall time | |
| C11 | Envelope / hash | Inspect prepared envelope | `envelope_hash` stable; `prompt_sha256` matches prompt bytes; model/yolo recorded | Prepared row fields | |
| C12 | Recovery attach epoch | After recovery path | `recovery_attach_epoch` increments when attach/settle; no double terminal | Revision sequence | |
| C13 | FCP marker cross-run replay reject | Capture a `flowpilot-fcp:<id>:<mac>` marker minted for run A's flow-coding handoff; replay same marker into a spawn for a different run B | Provenance check rejects marker (run A's `FCPMarkerProvenanceRunID` ≠ run B); child spawn does **not** trust the injected FCP context as authentic | Reject log line + child prompt shows marker rejected, not silently trusted | |
| C14 | Inspect never leaks raw payload | Force an uncertain/open-repair record with a real provider payload; call **Inspect** in UI | Response/UI shows canonical **hash** for receipt/terminal evidence and repair metadata only — never the raw provider payload or raw quarantine blob | Network response body (`receiptEvidence`/`terminalEvidence`/`openRepair` fields) | |
| C15 | Retry-as-new superseded guidance | On an uncertain record, click **Retry-as-new** twice (second click after first already advanced state/revision) | Second call returns 409 `dispatch_retry_superseded`; UI surfaces abandon guidance instead of silently failing or double-sending | UI message + HTTP 409 in network log | |
| C16 | Boot recovery auto-redispatch (no user action) | Kill runner process while a turn sits in `prepared`/`send_claimed` (before `send_started`); restart runner binary (not just reopen desktop window); do **not** touch the run in UI | `ScanDispatchRecoveryOnBoot` finds the record via `ListRecoverable`, reconstructs the run, and redispatches automatically — record advances past `send_claimed` without any user click | Server boot log (`ensureLiveAndRedispatch`) + dispatch.ndjson state advancing unattended | |
| C17 | Crash after send, no provider proof → uncertain (not dangling) | Kill runner **after** `send_started`/`provider_accepted` (model may genuinely be mid-response); restart runner binary | Boot scan claims the record and transitions it to `uncertain` (never left at `send_started` forever, never silently marked `terminal_completed` without proof); Dispatch attention card surfaces it for operator Inspect/Retry-as-new/Retry-load | dispatch.ndjson shows explicit `state=uncertain` row + `recovery_uncertain` audit entry; attention card in UI | |

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
| | | Live B1–B5 | ☐ | |
| | | Live C2–C12 | ☐ | |
| | | Live C13–C17 (new, post CA-340..349) | ☐ | |
| | | Live X1–X4 | ☐ | |

---

## 6. Gaps known (không expect full green §10 từ unit hiện tại)

- CP-51 **full barrier matrix B0…B8e + model suite** chưa đủ (Task-255 phase 2).
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
