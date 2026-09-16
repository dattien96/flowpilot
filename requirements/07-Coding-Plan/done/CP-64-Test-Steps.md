# CP-64 Test Steps — Reproduce-First TDD Gate

## Metadata

- Document ID: `CP-64-TEST-STEPS`
- Title: `CP-64 Verification Steps (Automated + gate-sandbox Manual)`
- Phase: `verification`
- Status: `ready`
- Owner: `FlowPilot`
- Reviewers: `Operator`
- Created: `2026-09-16`
- Last Updated: `2026-09-16`
- Parent Documents: [CP-64: Reproduce-First TDD Gate](./CP-64-Reproduce-First-TDD-Gate.md)
- Child Documents: `None`
- Related Documents: [Task-364](../../08-Task/done/Task-364-Reproduce-Gate-Rule-FlowGate.md), [Task-365](../../08-Task/done/Task-365-Reproducer-Prompt-Agent-Behavior.md), [Task-366](../../08-Task/done/Task-366-Bug-Flows-Reproduce-Node-Test-Lock.md), [Task-367](../../08-Task/done/Task-367-Bug-Fix-Lifecycle-E2E-Reproduce-Gate.md), [safe-fix-contract](../../../.agents/skills/safe-fix-contract/SKILL.md)
- Replaces: `None`
- Tags: `reproduce-first-gate, tdd, oracle-rule, test-steps, verification, cp-64`
- Feature Keys: `reproduce-first-gate`

## AI Quick View

### Summary

- Danh mục kiểm thử tự động hóa và thủ công trên `gate-sandbox` cho toàn bộ CP-64 (P-1→P-4).
- Xác thực:
  1. Rule `r-reproduce`: chặn khi test xanh, chặn khi compile error, pass khi assertion failure (`P-1`).
  2. Prompt/agent/behavior `agent.reproduce` (`P-2`).
  3. Node `reproduce_test` trong `bug-harness`/`bug-plan-harness` + khóa read-only file test ở node coder (`P-3`).
  4. E2E đỏ→xanh fail-closed (`P-4`).
- Giường thử thủ công (Manual Bed): `gate-sandbox` (`/Users/tiendat/Desktop/BE/gate-sandbox`).

### Current Ask

- Chạy kiểm thử tự động xác nhận rule engine, topology, behavior và E2E lifecycle đều PASS.
- Dogfood thủ công trên 1 bug thật: quan sát RED bắt buộc → lock → GREEN.

### Key Decisions

- `V-1` **Compile error ≠ reproduce**: chỉ assertion failure mới pass gate (oracle classifier).
- `V-2` **Provider-agnostic**: grep `providerKey|ProviderKey` trên rule/oracle/lock/gate files = 0 hit (CA-872).
- `V-3` **Flag-off legacy**: `FLOWPILOT_ENABLE_REPRODUCE_GATE` tắt → về empty-signatures cũ, không gate không lock.

### Constraints

- Không sửa test cũ.
- Bed kiểm thử thủ công: `gate-sandbox`.

---

## 1. Goal

Chứng minh 100% bug fix có bằng chứng vật lý RED→GREEN: không test đỏ assertion thì không được chạm production code, và coder không được viết lại test để né lỗi.

---

## 2. Automated — run first

Thư mục làm việc: `apps/local-runner`.

```bash
# 1. Rule engine + oracle classifier (P-1)
go test ./internal/flowgate/ -run 'TestRuleReproduce|TestClassifySuite|TestReproduceRule' -count=1 -v

# 2. Topology + behavior + lock (P-2, P-3)
go test ./internal/agentpack/ -count=1
go test ./internal/changecontract/ -count=1
go test ./internal/runner/ -run 'TestBugHarnessTopology|TestBugPlanHarnessTopology|TestCoderNodeHasTestFileAsReadOnly|TestReproducer|TestReproduceBehavior' -count=1 -v

# 3. Lifecycle E2E red-green (P-4, mock provider + REAL go toolchain)
go test ./internal/runner/ -run 'TestBugFixLifecycleEndToEndWithReproduceGate|TestBugFixFailsClosedWhenBugNotReproduced|TestTaskHarnessSignatureTurnPassesWithReproduceGateOn|TestBugHarnessLegacyPathWithReproduceGateOff' -count=1 -v
```

| Step | Kiểm tra | Pass khi | Tick |
|---|---|---|---|
| 2.1 | Rule core (`P-1`) | `TestRuleReproduceFailsWhenAllTestsPass`, `TestRuleReproduceFailsOnCompileError`, `TestRuleReproducePassesOnAssertionFailure`, `TestRuleReproduceSkipsOnNonBugFlow` green | [x] PASS 2026-09-16 |
| 2.2 | Classifier + wiring (`P-1`) | `TestClassifySuiteOutputPerRunner`, `TestReproduceRuleOptInAndTestRuleSuppression` green | [x] PASS 2026-09-16 |
| 2.3 | Prompt/agent/behavior (`P-2`) | `TestReproducerAgentPromptRender`, `TestReproducerArtifactBinding`, `TestReproduceBehaviorRuntimeBinding` green | [x] PASS 2026-09-16 |
| 2.4 | Topology + lock (`P-3`) | `TestBugHarnessTopologyContainsReproduceGate`, `TestBugPlanHarnessTopologyContainsReproduceGate`, `TestCoderNodeHasTestFileAsReadOnly` green | [x] PASS 2026-09-16 |
| 2.5 | E2E red→green (`P-4`) | `TestBugFixLifecycleEndToEndWithReproduceGate` green (RED proof → lock → coder-deny → fix → GREEN, zero regression) | [x] PASS 2026-09-16 |
| 2.6 | E2E fail-closed (`P-4`) | `TestBugFixFailsClosedWhenBugNotReproduced` green (xanh-ngay/compile-error/bogus-binary đều reprompt, implement không dispatch, diff production rỗng) | [x] PASS 2026-09-16 |
| 2.7 | Compat (`P-4`) | `TestTaskHarnessSignatureTurnPassesWithReproduceGateOn`, `TestBugHarnessLegacyPathWithReproduceGateOff` green | [x] PASS 2026-09-16 |

---

## 3. Manual prep — project gate-sandbox

| # | Việc | Cách kiểm | Tick |
|---|---|---|---|
| P1 | Sandbox tồn tại | Thư mục `/Users/tiendat/Desktop/BE/gate-sandbox` | [ ] |
| P2 | Runner biên dịch | `cd apps/local-runner && go build ./...` thành công | [ ] |
| P3 | Bật gate | `FLOWPILOT_ENABLE_REPRODUCE_GATE=1` trong env chạy flow | [ ] |
| P4 | Seed 1 bug thật | 1 hàm sai logic kèm test đúng (chưa chạy) trong sandbox | [ ] |
| P5 | Provider | Mọi manual run có agent turn trên máy này: **Grok only** (không e2e Claude/Codex — operator constraint 2026-09-16) | [ ] |

---

## 4. Manual Verification Steps on gate-sandbox

### Kịch bản M-1: Bug thật đi trọn RED → lock → GREEN

1. Chạy flow `bug-harness` với bug đã seed (P4).
2. **Quan sát node `reproduce_test`**: agent viết test mới, runner chạy → gate `r-reproduce` PASS chỉ khi test fail bằng assertion (log trigger `reproduce_not_demonstrated` biến mất).
3. **Quan sát node `implement`**: thử bảo coder sửa file test vừa tạo → request bị silent-deny (test nằm trong `ReadOnlyPaths`).
4. Coder sửa production → suite GREEN → gate pass → audit done.

### Kịch bản M-2: Fail-closed khi không tái hiện được

1. Seed bug mà test viết ra **pass ngay** (green-on-arrival).
2. **Quan sát**: gate reprompt "chưa tái hiện được bug", node `implement` không bao giờ dispatch, diff production rỗng.
3. Lặp với test **lỗi cú pháp** → reprompt "sửa cho compile được" (không tính là reproduce).

### Kịch bản M-3: Flag-off về legacy

1. Tắt `FLOWPILOT_ENABLE_REPRODUCE_GATE`, chạy lại `bug-harness`.
2. **Quan sát**: node reproduce degrade về empty-signatures cũ, không gate, không lock — flow chạy như trước CP-64.

---

## 5. Log Grep (Bằng chứng Kiểm toán)

```text
r-reproduce
reproduce_not_demonstrated
ReadOnlyPaths
reproduceLockCommandTargetsLockedPath
```

---

## 6. CP-64 Verification Complete When

- [x] §2 Automated chạy xanh 100% (2026-09-16: flowgate + agentpack + changecontract + runner E2E).
- [x] M-1: **LIVE 2026-09-16 (run-704076, grok-4.5, runner :18754 + `FLOWPILOT_ENABLE_REPRODUCE_GATE=1`)** — seed `ManualSum` a-b bug, `go test` RED assertion (`= -1, want 5`) trước run; flow đi `contract-planner → reproducer → coder → reviewer`: reproducer viết `TestManualSum_TwoPlusThreeEqualsFive` mới, coder fix `a+b` (không đụng test), reviewer verify locked-tests non-stub + viết `CA-924`; `go test` GREEN sau run; qua `reproduce_test` được = gate pass (fail-closed nếu không). Seed files đã dọn, bed build+test xanh.
- [ ] M-2: green-on-arrival + compile-error đều fail-closed đúng. *(blocked: cần thêm 2 flow run grok — hết budget turn này; automated `TestBugFixFailsClosedWhenBugNotReproduced` xanh)*
- [ ] M-3: flag-off về legacy sạch. *(blocked cùng lý do; vehicle sẵn: runner :18753 không có flag)*
- [ ] Toàn bộ test cũ nguyên vẹn, không chỉnh sửa.

(End of file - total 128 lines)
