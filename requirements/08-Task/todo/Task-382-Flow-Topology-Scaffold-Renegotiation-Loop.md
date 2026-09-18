# Task-382: Flow Topology Scaffold TDD & Renegotiation Loop

## Metadata

- Document ID: `Task-382`
- Title: `Flow Topology Scaffold TDD & Renegotiation Loop`
- Phase: `task`
- Status: `todo`
- Owner: `Claude Sonnet MAX`
- Reviewers: `Claude agent review`
- Created: `2026-09-18`
- Last Updated: `2026-09-18`
- Parent Documents: [CP-67 P-5](../../07-Coding-Plan/todo/CP-67-Contract-First-Scaffold-TDD-And-Signature-Lock.md)
- Child Documents: `None`
- Related Documents: [CP-64 P-3](../../07-Coding-Plan/done/CP-64-Reproduce-First-TDD-Gate.md), [CP-62](../../07-Coding-Plan/done/CP-62-Zcode-Harness-Parity.md), [Task-378](./Task-378-Scaffold-Coder-Declared-Face-Tool-Schemas.md), [Task-379](./Task-379-Signature-Lock-Rule-AST-Extractor.md), [Task-380](./Task-380-Scaffold-Architect-Prompt-Agent-Persona.md), [Task-381](./Task-381-Coder-Scaffold-Body-Prompt-Batch-Contract.md)
- Replaces: `None`
- Tags: `contract-first-tdd, flow-topology, task-harness, vibe-sprint, renegotiation-loop, hub-mediation`
- Feature Keys: `contract-first-tdd`

## AI Quick View

### Summary

- Slice cuối cùng (P-5) của CP-67: wire toàn bộ hạ tầng (tool schemas P-1, gate P-2, TDD prompt P-3, Coder prompt P-4) vào topology thực tế của `task-harness.yaml` và `vibe-sprint.yaml`.
- Thay thế node `test_signatures` (task-harness) và `tdd` (vibe-sprint) thành node `scaffold_tdd` mới với behavior `agent.scaffold`, agent `agents/scaffold-architect.md`, promptTemplate `prompts/scaffold-contract-tdd.md`.
- Cập nhật node `implement`/`coder` dùng prompt mới `prompts/implement-scaffold-body.md` và tool `submit-coder-outcome.yaml`.
- Thiết lập vòng lặp đàm phán signature qua Main Agent Hub: `implement/coder` → `synthesis` (hub mediation, status `continue` từ `renegotiate_signatures`) → `scaffold_tdd` (back-edge) → `implement/coder` (forward). Ngân sách: `cap: 5`.
- Runner khóa test files (`ReadOnlyPaths`) và snapshot `SignatureHash` vào `FrozenContractRecord` khi scaffold_tdd pass gate.
- Cờ fallback: `FLOWPILOT_ENABLE_CONTRACT_FIRST_TDD=false` → flow fallback về `test_signatures` + `implement-complete-tests.md` cũ.

### Current Ask

- Implement P-5 theo CP-67 §4: sửa 2 flow YAML, cập nhật runner wiring cho test lock + signature hash snapshot, thêm tools vào flow, 3 test signatures phải xanh.

### Key Decisions

- `T-1` `task-harness.yaml` node `test_signatures` (line 146-151) đổi thành `scaffold_tdd`: behavior `agent.scaffold`, agent `agents/scaffold-architect.md`, promptTemplate `prompts/scaffold-contract-tdd.md`, lifecycle `reinvoke`. Edge topology: `preflight_contract_freeze → scaffold_tdd → implement` giữ nguyên forward flow.
- `T-2` `vibe-sprint.yaml` node `tdd` (line 52-68) đổi thành `scaffold_tdd` tương tự.
- `T-3` Node `implement`/`coder` cập nhật: promptTemplate đổi sang `prompts/implement-scaffold-body.md`; thêm tool `tools/submit-coder-outcome.yaml` vào danh sách tools của flow.
- `T-4` Renegotiation back-edge: `synthesis → scaffold_tdd` (when: `continue`, kind: `back`). Hub synthesis nhận `renegotiate_signatures` payload từ `submit_coder_outcome` → forward batch cho TDD Agent ở turn tiếp theo. Đây là back-edge thứ 3 hợp pháp (task-harness đã có 2: plan_synthesis→plan_writer, validate→implement; thêm 1 synthesis→scaffold_tdd — CP-58 Task-304 keyed validator cho phép duplicate back-edges khi from khác nhau).
- `T-5` Runner wiring (gate_hook.go): sau khi scaffold_tdd pass gate (test compile OK + test ĐỎ) → (a) khóa test files vào `ReadOnlyPaths` (reuse `LockReproduceTestPaths` mechanism từ CP-64); (b) extract signatures và snapshot `SignatureHash` + `LockedSignatures` vào `FrozenContractRecord`.
- `T-6` Fallback: env `FLOWPILOT_ENABLE_CONTRACT_FIRST_TDD` unset → runtime resolver degrade node `scaffold_tdd` về behavior `agent.code` + prompt `test-signatures.md` (giống hệt legacy), gate `r-signature-lock` không append. Zero-behavior-change.
- `T-7` Cả 2 flow thêm tool: `tools/submit-scaffold-outcome.yaml` + `tools/submit-coder-outcome.yaml` vào `tools:` list.

### Constraints

- Không sửa `bug-harness.yaml` / `bug-plan-harness.yaml` — CP-64 reproduce flow giữ nguyên.
- Không sửa `rag-harness.yaml` — chatBaseline flow giữ nguyên.
- Topology changes phải pass `ValidateFlowSafetyTopology` (CP-55 P-1): mọi `agent.code`/`agent.scaffold` writer bị dominated bởi `contract.freeze` node.
- `cap: 5` — policy.cap giữ nguyên 5 cho task-harness (dual-loop: plan review + code review + renegotiation chia sẻ budget).
- Additive tests only.
- GitNexus impact analysis trước khi sửa flow YAML.

### Open Questions

- None.

### Source Refs

- CP-67 §3.1, §4 P-5.
- `internal/agentpack/flow-pack/flows/task-harness.yaml` (current topology).
- `internal/agentpack/flow-pack/flows/vibe-sprint.yaml` (current topology).
- `internal/agentpack/flow-pack/flows/bug-harness.yaml` (CP-64 reproduce wiring reference).
- `internal/runner/reproduce_gate.go` (`LockReproduceTestPaths` reuse).
- `internal/runner/gate_hook.go` (SignatureHash snapshot wiring).

## 1. Goal

`task-harness` và `vibe-sprint` chạy End-to-End với quy trình Contract-First TDD: TDD Agent tạo stubs + red tests → khóa test + snapshot hash → Coder fill body → nếu cần đổi signature: batch qua hub → TDD sửa → Coder tiếp. Cờ fallback degrade về empty signatures.

## 2. Parent Links

- coding plan: `CP-67-Contract-First-Scaffold-TDD-And-Signature-Lock.md` P-5
- tech design: `SD-20-Flow-Gate-Rule-Semantics.md`
- system spec: `SS-14-Code-Context-And-Regression-Safety.md`, `SP-06-Oracle-Rule-And-Schema-First-Gate.md`

## 3. Trigger

P-1→P-4 (Task-378→381) đã hoàn thành toàn bộ hạ tầng: tool schemas, gate rule, TDD prompt, Coder prompt. Slice này wire mọi thứ vào flow YAML thực tế và runner hooks — bước cuối cùng để Contract-First TDD hoạt động End-to-End.

## 4. Exact Change

- `T-1` **`internal/agentpack/flow-pack/flows/task-harness.yaml`** (modified): (a) node `test_signatures` (id rename to `scaffold_tdd`) → behavior `agent.scaffold`, agent `agents/scaffold-architect.md`, promptTemplate `prompts/scaffold-contract-tdd.md`, lifecycle `reinvoke`; (b) node `implement` → promptTemplate `prompts/implement-scaffold-body.md`; (c) tools list: thêm `tools/submit-scaffold-outcome.yaml`, `tools/submit-coder-outcome.yaml`; (d) back-edge mới: `from: synthesis, to: scaffold_tdd, when: continue, kind: back` (renegotiation loop).
- `T-2` **`internal/agentpack/flow-pack/flows/vibe-sprint.yaml`** (modified): tương tự task-harness — node `tdd` → `scaffold_tdd`, `coder` → prompt mới, tools mới, back-edge `synthesis → scaffold_tdd`.
- `T-3` **`internal/runner/gate_hook.go`** (modified): sau scaffold_tdd turn pass gate — (a) reuse `LockReproduceTestPaths` logic từ `reproduce_gate.go` để khóa test files vào `ReadOnlyPaths`; (b) gọi `ExtractCanonicalSignatures` + `CanonicalSignatureHash` (Task-379) trên declared stub paths → lưu `SignatureHash` + `LockedSignatures` vào `FrozenContractRecord`.
- `T-4` **`internal/runner/scaffold_fallback.go`** (new hoặc wiring trong `gate_hook.go`): khi `FLOWPILOT_ENABLE_CONTRACT_FIRST_TDD` unset → resolve `scaffold_tdd` node về legacy behavior: behavior `agent.code`, prompt `test-signatures.md`, gate `r-signature-lock` không append.
- `T-5` **Test files** (new): 3 test signatures.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/agentpack/flow-pack/flows/task-harness.yaml` (modified)
  - `apps/local-runner/internal/agentpack/flow-pack/flows/vibe-sprint.yaml` (modified)
  - `apps/local-runner/internal/runner/gate_hook.go` (modified — test lock + signature snapshot)
  - `apps/local-runner/internal/runner/scaffold_fallback.go` (new — fallback resolver)
  - `apps/local-runner/internal/runner/scaffold_topology_test.go` (new)
  - `apps/local-runner/internal/agentpack/scaffold_flow_topology_test.go` (new)
- modules: `agentpack`, `runner`
- routes: none
- tables: none

## 6. Acceptance Check

- [ ] AC-1: `task-harness.yaml` topology: `preflight_contract_freeze → scaffold_tdd → implement → validate → reviewer → synthesis → audit` với back-edge `synthesis → scaffold_tdd` (renegotiation).
- [ ] AC-2: `vibe-sprint.yaml` topology: `context → scaffold_tdd → coder → validate → synthesis → audit` với back-edge `synthesis → scaffold_tdd`.
- [ ] AC-3: Pack validation pass cho cả 2 flow (ValidateFlowSafetyTopology: scaffold_tdd dominated by contract.freeze).
- [ ] AC-4: Runner khóa test files vào `ReadOnlyPaths` sau scaffold_tdd pass gate.
- [ ] AC-5: Runner snapshot `SignatureHash` vào `FrozenContractRecord` sau scaffold_tdd pass gate.
- [ ] AC-6: `FLOWPILOT_ENABLE_CONTRACT_FIRST_TDD=false` → fallback về empty-signature legacy behavior, test-identical.
- [ ] AC-7: `bug-harness.yaml`, `bug-plan-harness.yaml`, `rag-harness.yaml` không bị ảnh hưởng.
- [ ] AC-8: 3 test signatures green:
  - `TestTaskHarnessTopologyScaffoldRenegotiationLoop`
  - `TestMainAgentMediationDispatchesBatchToTDD`
  - `TestRenegotiationCapEnforcedAtFive`

## 7. Out of Scope

- Tool face schemas (P-1 / Task-378).
- Gate rule `r-signature-lock` core logic (P-2 / Task-379).
- TDD prompt content (P-3 / Task-380).
- Coder prompt content (P-4 / Task-381).
- LSP documentSymbol integration cho Kotlin/TS (follow-up).

## 8. Completion Notes

- result: `todo`
- follow-ups: E2E verification, Kotlin/TS LSP extractor.
- upstream docs updated: `todo`
