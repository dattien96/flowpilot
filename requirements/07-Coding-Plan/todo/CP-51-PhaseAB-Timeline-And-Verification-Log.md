# CP-51 Companion: Phase A/B Timeline + Verification & E2E Test Log

## Metadata

- Document ID: `CP-51-VERIFY`
- Title: `Phase A/B Timeline And Verification / E2E Test Log`
- Phase: `coding_plan` companion / verification log (not a new CP number)
- Status: `inprogress` (automation suites runnable; live desktop E2E checklist for human)
- Owner: `FlowPilot`
- Created: `2026-07-17`
- Last Updated: `2026-07-17`
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

### 2.3 Phase B — CP-50 / context + change contract

```bash
go test ./internal/runner/ -count=1 -timeout 5m \
  -run 'TestContext|TestChangeContract|TestRenderFlowContext|TestContract|TestSourceExcerpt|TestArtifactBinding|TestResolveEnabledContext'
```

Related files (search): `context_source_*_test.go`, `context_source_change_contract_test.go`, `context_package_sections_test.go`, `gate_tier_test.go` (contract/gate interplay).

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
  -run 'TestDispatch|TestCrashMatrix|TestStopCAS|TestSessionIDIsNotAReceipt|TestProviderV2|TestFCPMarker|TestApplySessionRuntime|TestSnapshot_|TestRecoveryScanner|TestSettleDriver|TestOperatorAttention|TestCapabilityEvidence|TestLocalStore_Disk|TestCreatePrepared|TestActivation|TestRetryAsNew|TestRecordEffect|TestReleaseManifest|TestOpenRepair|TestEventTurnStarted|TestPreSend|TestReceipt|TestTerminalCommit|TestOwnRunStop|TestChildSendStarted|TestResolveUncertain|TestRecoveryAttach|TestEverySent|TestRunStopState|TestLocalLog_Startup'

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

### 3.1 Phase A — Flow graph

| # | Scenario | Expect | ☐ |
|---|----------|--------|---|
| A1 | Chat Mode bug sub-mode → pick **Review Loop** | Agents board spawn coder → reviewers cohort | |
| A2 | Coder complete, both reviewers complete | Cohort join → hub synthesis turn (không hang RUNNING mãi) | |
| A3 | YOLO off: child needs approval | Card surface; Stop/Skip/Retry stall policy nếu stall | |
| A4 | Stop mid-flow from hub | Loop stopped; steps không stuck RUNNING giả | |
| A5 | Restart server mid-flow (local) | Timeline/step restore hợp lý (239); không cancel synthesis DONE giả | |
| A6 | Coding child + code change | Tier-1 doc/scope gate / reprompt về đúng child (242); không gate sau Completed mù | |
| A7 | Flow có validate node (rag-harness nếu dùng) | Retry lifecycle reinvoke implement (279 class) | |

### 3.2 Phase B — Context / change contract

| # | Scenario | Expect | ☐ |
|---|----------|--------|---|
| B1 | Coding child declares scope / change contract | Downstream re-entry (Continue/retry) có contract inject | |
| B2 | Reviewer turn | Không overwrite contract của coder (dirty tree) | |
| B3 | Context package / head | Feature history không double-inject sai (FCP / handoff) | |

### 3.3 CP-51 — Durable turn (+ Task-258 Drive)

| # | Scenario | Expect | ☐ |
|---|----------|--------|---|
| C1 | `FLOWPILOT_DISPATCH_V2=1`, one chat turn | File `.flowpilot/chats/<project_id>/dispatch.ndjson` có dòng prepared→… | |
| C2 | Kill runner sau turn started, trước complete | Restart: record send_started hoặc uncertain; không silent loss | |
| C3 | Stop ngay trước send | `stopped_before_send` / terminal_cancelled; không double prompt provider | |
| C4 | Uncertain surface | Desktop **Dispatch attention** card; abandon/mark works | |
| C5 | Chat sync project to Drive + restore on 2nd machine | Session + **dispatch** log restored under same project_id | |
| C6 | Claude/Gemini default | V2 start rejected / disabled trừ allow-list env | |

### 3.4 Cross-cut smoke

| # | Scenario | Expect | ☐ |
|---|----------|--------|---|
| X1 | Full review-loop happy path + V2 on | Flow complete + dispatch terminal rows | |
| X2 | `go test` §2.2 + §2.4 + §2.5 green | No new red in those patterns | |

---

## 4. Copy-paste “one shot” for CI-ish local verify

```bash
cd apps/local-runner

echo "=== Phase A-ish + pack + flowgate ==="
go test ./internal/agentpack/ ./internal/flowgate/ -count=1 -timeout 5m

echo "=== Phase A/B runner patterns ==="
go test ./internal/runner/ -count=1 -timeout 10m \
  -run 'TestCohort|TestApplyFlowControl|TestResumePendingFlowGate|TestChildGate|TestBuiltinOrchestration|TestContext|TestChangeContract|TestRenderFlowContext'

echo "=== BUG-288 rounds (durable/gate) ==="
go test ./internal/runner/ -count=1 -timeout 10m \
  -run 'TestDurable|TestGateSettle|TestWithGateEpoch|TestMarkPending|TestInjectFeatureHistory|TestParseDurable|TestSessionRuntime|TestIdempotencyKeys|TestMarkerSecret|TestRunTurnGate|TestRunTurnPostGate|TestFinishTurnStall|TestMemberActionRetry'

echo "=== CP-51 dispatch ==="
go test ./internal/runner/ -count=1 -timeout 5m \
  -run 'TestDispatch|TestCrashMatrix|TestStopCAS|TestRecoveryScanner|TestSettleDriver|TestOperatorAttention|TestCapabilityEvidence|TestFCPMarker|TestApplySessionRuntime|TestSnapshot_|TestProviderV2|TestSessionIDIsNot'
```

Ghi kết quả vào §5.

---

## 5. Run log (điền khi verify)

| Date | Who | Command block | Result | Notes |
|------|-----|---------------|--------|-------|
| | | §4 one-shot | ☐ pass / ☐ fail | |
| | | Live A1–A7 | ☐ | |
| | | Live B1–B3 | ☐ | |
| | | Live C1–C6 | ☐ | |

---

## 6. Gaps known (không expect full green §10 từ unit hiện tại)

- CP-51 **full barrier matrix B0…B8e + model suite** chưa đủ (Task-255 phase 2).
- Live provider e2e (Codex/Claude/Grok) phụ thuộc account/env.
- BUG-288 formal status vẫn `inprogress` đến re-review sạch + CP-51 closure.
- Desktop DispatchAttentionCard cần runner V2 + store wired (`FLOWPILOT_DISPATCH_V2=1`).

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
