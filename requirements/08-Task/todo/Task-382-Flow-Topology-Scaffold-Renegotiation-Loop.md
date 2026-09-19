# Task-382: Flow Topology Scaffold TDD & Renegotiation Loop

## Metadata

- Document ID: `Task-382`
- Title: `Flow Topology Scaffold TDD & Renegotiation Loop`
- Phase: `task`
- Status: `todo`
- Owner: `Claude Sonnet MAX`
- Reviewers: `Claude agent review`
- Created: `2026-09-18`
- Last Updated: `2026-09-19`
- Parent Documents: [CP-67 P-5](../../07-Coding-Plan/todo/CP-67-Contract-First-Scaffold-TDD-And-Signature-Lock.md)
- Child Documents: `None`
- Related Documents: [CP-64 P-3](../../07-Coding-Plan/done/CP-64-Reproduce-First-TDD-Gate.md), [CP-62](../../07-Coding-Plan/done/CP-62-Zcode-Harness-Parity.md), [Task-378](./Task-378-Scaffold-Coder-Declared-Face-Tool-Schemas.md), [Task-379](./Task-379-Signature-Lock-Rule-AST-Extractor.md), [Task-380](./Task-380-Scaffold-Architect-Prompt-Agent-Persona.md), [Task-381](./Task-381-Coder-Scaffold-Body-Prompt-Batch-Contract.md)
- Replaces: `None`
- Tags: `contract-first-tdd, flow-topology, task-harness, vibe-sprint, renegotiation-loop, hub-mediation`
- Feature Keys: `contract-first-tdd`

## AI Quick View

### Summary

- Slice cuối cùng (P-5) của CP-67: wire toàn bộ hạ tầng (tool schemas P-1, gate P-2, TDD prompt P-3, Coder prompt P-4) vào topology thực tế của `task-harness.yaml` và `vibe-sprint.yaml`.
- **Post-review B-7**: Giữ nguyên node id trong cả 2 flow (`test_signatures`, `tdd`, `implement`, `coder`) - không đổi id để tránh vỡ `step_definitions` rows và `vibe_sprint.go`/`vibe_cp.go` hardcode.
- **Post-review B-6**: Thêm node trung gian `synthesis_negotiation` (inline hub, `hub.inline`, `agents/synthesizer.md`) để tránh trùng back-edge `continue` với `synthesis -> coder` trong vibe-sprint.
- Cập nhật node `test_signatures` (task-harness) / `tdd` (vibe-sprint): behavior `agent.code` -> `agent.scaffold`, agent `agents/tester.md` -> `agents/scaffold-architect.md`, promptTemplate `prompts/test-signatures.md` -> `prompts/scaffold-contract-tdd.md`, lifecycle `reinvoke`, thêm `model: claude-sonnet-4-5`.
- Cập nhật node `implement`/`coder`: promptTemplate -> `prompts/implement-scaffold-body.md`; tools list thêm `submit-scaffold-outcome.yaml` + `submit-coder-outcome.yaml`.
- **Post-review B-10**: Thiết lập vòng lặp đàm phán signature qua `synthesis_negotiation` Hub: `implement/coder` -> `synthesis_negotiation` (hub mediation, status `continue` từ `renegotiate_signatures`) -> `test_signatures/tdd` (back-edge) -> `implement/coder` (forward). Ngân sách: `policy.negotiationCap: 5` (phase-scoped, không dùng chung flow cap).
- Runner khóa test files (`ReadOnlyPaths`) và snapshot `SignatureHash` + `LockedSignatures` vào `FrozenContractRecord` (single-write `LockScaffoldArtifacts`) khi scaffold pass gate.
- **Post-review B-9**: Không có cờ fallback `FLOWPILOT_ENABLE_CONTRACT_FIRST_TDD` - CP-67 luôn bật khi merge; CP-64 `FLOWPILOT_ENABLE_REPRODUCE_GATE` cũng retire (always-on); legacy prompt giữ lại chỉ để rollback revert commit.

### Current Ask

- Implement P-5 theo CP-67 §4: sửa 2 flow YAML, cập nhật runner wiring cho test lock + signature hash snapshot, thêm tools vào flow, 6 test signatures phải xanh.

### Key Decisions

- `T-1` **`internal/agentpack/flow-pack/flows/task-harness.yaml`** (modified):
  - Node `test_signatures` (**id giữ nguyên** - B-7): behavior `agent.code` → `agent.scaffold`; agent `agents/tester.md` → `agents/scaffold-architect.md`; promptTemplate `prompts/test-signatures.md` → `prompts/scaffold-contract-tdd.md`; lifecycle `reinvoke`; thêm `model: claude-sonnet-4-5` (B-5).
  - Node `implement`: promptTemplate → `prompts/implement-scaffold-body.md`.
  - tools list: thêm `tools/submit-scaffold-outcome.yaml`, `tools/submit-coder-outcome.yaml`.
  - Node mới `synthesis_negotiation` (B-6): inline hub, `hub.inline`, `agents/synthesizer.md` - nhận renegotiation batch từ coder step, thẩm định, forward cho `test_signatures` → `implement`.
  - Edges (B-6): `synthesis → synthesis_negotiation` (when: continue, payload batch), `synthesis_negotiation → test_signatures` (when: continue, kind: back), `synthesis_negotiation → synthesis` (when: done, kind: forward).
  - `policy.negotiationCap: 5` (B-10).
- `T-2` **`internal/agentpack/flow-pack/flows/vibe-sprint.yaml`** (modified):
  - Node `tdd` (**id giữ nguyên** - B-7): behavior `agent.code` -> `agent.scaffold`; agent `agents/tester.md` -> `agents/scaffold-architect.md`; promptTemplate `prompts/test-signatures.md` -> `prompts/scaffold-contract-tdd.md`; lifecycle `reinvoke`; `model: claude-sonnet-4-5`; contextProfile `tdd` giữ nguyên.
  - Node `coder`: promptTemplate -> `prompts/implement-scaffold-body.md`.
  - tools list: thêm `tools/submit-scaffold-outcome.yaml`, `tools/submit-coder-outcome.yaml`.
  - Node mới `synthesis_negotiation` (B-6): inline hub, `hub.inline`, `agents/synthesizer.md`.
  - Edges (B-6): `coder -> synthesis_negotiation` (when: continue, payload batch), `synthesis_negotiation -> tdd` (when: continue, kind: back), `synthesis_negotiation -> synthesis` (when: done, kind: forward).
  - **Không đổi edge `synthesis -> coder (when:continue, kind:back)`** (cũ) - vẫn dùng cho review loop; renegotiation route qua `synthesis_negotiation`.
  - `policy.negotiationCap: 5` (B-10).
- `T-3` Node `implement`/`coder` cập nhật: promptTemplate đổi sang `prompts/implement-scaffold-body.md`; thêm tool `tools/submit-coder-outcome.yaml` vào danh sách tools của flow.
- `T-4` Renegotiation route (sync B-6): renegotiation KHÔNG đi qua node `synthesis` cũ — đi qua node mới `synthesis_negotiation` (inline hub). Hub nhận batch `renegotiate_signatures` từ `submit_coder_outcome` (buffer `pendingBatchSignatureByStep`) → thẩm định → back-edge `synthesis_negotiation → test_signatures/tdd` (when: continue) → forward `synthesis_negotiation → synthesis` (when: done). Edge cũ `synthesis → coder/test_signatures (when:continue, kind:back)` giữ nguyên cho review loop.
- `T-5` Runner wiring (gate_hook.go): sau khi scaffold node (`test_signatures`/`tdd`) pass gate (test compile OK + test ĐỎ) → gọi `LockScaffoldArtifacts` (Task-379, B-8.3) — single-write 1 version bump: (a) khóa test files vào `ReadOnlyPaths`; (b) extract signatures và snapshot `SignatureHash` + `LockedSignatures` vào `FrozenContractRecord`.
- `T-6` (sync B-9) **Không có env flag**: CP-67 luôn bật khi merge — không có `FLOWPILOT_ENABLE_CONTRACT_FIRST_TDD`; CP-64 `FLOWPILOT_ENABLE_REPRODUCE_GATE` cũng retire (always-on). Fallback = revert commit; legacy prompt files (`test-signatures.md`, `implement-complete-tests.md`) giữ lại như artifact.
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
- `internal/runner/reproduce_gate.go` (lock mechanism reference, B-9 retire flag).
- `internal/runner/gate_hook.go` (SignatureHash snapshot wiring).

## 1. Goal

`task-harness` và `vibe-sprint` chạy End-to-End với quy trình Contract-First TDD: TDD Agent tạo stubs + red tests → khóa test + snapshot hash → Coder fill body → nếu cần đổi signature: batch qua hub → TDD sửa → Coder tiếp. Không env flag (B-9) — always-on; legacy prompt giữ lại như artifact rollback.

## 2. Parent Links

- coding plan: `CP-67-Contract-First-Scaffold-TDD-And-Signature-Lock.md` P-5
- tech design: `SD-20-Flow-Gate-Rule-Semantics.md`
- system spec: `SS-14-Code-Context-And-Regression-Safety.md`, `SP-06-Oracle-Rule-And-Schema-First-Gate.md`

## 3. Trigger

P-1→P-4 (Task-378→381) đã hoàn thành toàn bộ hạ tầng: tool schemas, gate rule, TDD prompt, Coder prompt. Slice này wire mọi thứ vào flow YAML thực tế và runner hooks — bước cuối cùng để Contract-First TDD hoạt động End-to-End.

## 4. Exact Change

- `T-1` **`internal/agentpack/flow-pack/flows/task-harness.yaml`** (modified, sync B-6/B-7): (a) node `test_signatures` (**id giữ nguyên** — B-7) → behavior `agent.scaffold`, agent `agents/scaffold-architect.md`, promptTemplate `prompts/scaffold-contract-tdd.md`, lifecycle `reinvoke`, thêm `model: claude-sonnet-4-5`; (b) node `implement` → promptTemplate `prompts/implement-scaffold-body.md`; (c) tools list: thêm `tools/submit-scaffold-outcome.yaml`, `tools/submit-coder-outcome.yaml`; (d) node `synthesis_negotiation` (inline hub) + edges: `synthesis → synthesis_negotiation` (when: continue, payload batch), `synthesis_negotiation → test_signatures` (when: continue, kind: back), `synthesis_negotiation → synthesis` (when: done, kind: forward); (e) `policy.negotiationCap: 5`.
- `T-2` **`internal/agentpack/flow-pack/flows/vibe-sprint.yaml`** (modified, sync B-6/B-7): tương tự task-harness — node `tdd` (**id giữ nguyên** — B-7) → behavior `agent.scaffold` + `model: claude-sonnet-4-5`, node `coder` → prompt mới, tools mới, node `synthesis_negotiation` (inline hub) + edges (`coder → synthesis_negotiation` continue, `synthesis_negotiation → tdd` back, `synthesis_negotiation → synthesis` done/forward), `policy.negotiationCap: 5`. **Không đổi edge `synthesis → coder (when:continue, kind:back)`** — review loop giữ nguyên.
- `T-3` **`internal/runner/gate_hook.go`** (modified): sau scaffold node (`test_signatures`/`tdd`) turn pass gate — gọi `LockScaffoldArtifacts` (Task-379, B-8.3) single-write 1 version bump: (a) khóa test files vào `ReadOnlyPaths`; (b) snapshot `SignatureHash` + `LockedSignatures` vào `FrozenContractRecord` trên declared stub paths.
- `T-4` **Không có `scaffold_fallback.go`** (post-review B-9): CP-67 luôn bật, không env flag. CP-64 `FLOWPILOT_ENABLE_REPRODUCE_GATE` cũng retire (always-on). Legacy prompt files (`test-signatures.md`, `implement-complete-tests.md`) giữ lại như artifact (rollback revert commit, hoặc cho bug-harness legacy).
- `T-5` **`internal/agentpack/pack.go`** (modified): parse `policy.negotiationCap` (default 5); validate range 1..20.
- `T-6` **`internal/runner/agent_orchestrator.go` + `internal/runner/interactive_service.go`** (modified, B-10): `AgentLoopState` thêm `NegotiationRound int`; `effectiveNegotiationCap` = `policy.negotiationCap` (trong flow policy) else 5; mỗi back-edge negotiation: increment `NegotiationRound`; nếu `>= negotiationCap` -> escalate (không extend); reset khi `synthesis_negotiation -> synthesis (done)`.
- `T-7` **Test files** (new): **6 test signatures**:
  - `TestTaskHarnessTopologyScaffoldNegotiationLoop`
  - `TestVibeSprintTopologyScaffoldNegotiationLoop`
  - `TestScaffoldNegotiationCapEnforcedAtFive`
  - `TestSynthesisNegotiationNodeRouting`
  - `TestScaffoldTestLockAndSignatureSnapshotSingleVersion`
  - `TestLegacyTestSignaturesFlowUnaffected` (impact check)

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/agentpack/flow-pack/flows/task-harness.yaml` (modified)
  - `apps/local-runner/internal/agentpack/flow-pack/flows/vibe-sprint.yaml` (modified)
  - `apps/local-runner/internal/agentpack/pack.go` (modified - policy.negotiationCap)
  - `apps/local-runner/internal/runner/gate_hook.go` (modified - test lock + signature snapshot + retire flag)
  - `apps/local-runner/internal/runner/agent_orchestrator.go` (modified - NegotiationRound, negotiationCap)
  - `apps/local-runner/internal/runner/interactive_service.go` (modified - routing synthesis_negotiation)
  - `apps/local-runner/internal/runner/reproduce_gate.go` (modified - retire flag, legacy degrade xóa)
  - `apps/local-runner/internal/runner/scaffold_topology_test.go` (new)
  - `apps/local-runner/internal/agentpack/scaffold_flow_topology_test.go` (new)
- modules: `agentpack`, `runner`
- routes: none (dùng existing `POST /client/workflow-runs/{run}/flow-control`)
- tables: none

## 6. Acceptance Check

- [ ] AC-1: `task-harness.yaml` topology (post-review B-6/B-7): giữ node id (`test_signatures`, `implement`, `validate`, `reviewer`, `synthesis`, `audit`), thêm `synthesis_negotiation`; edges: `preflight_contract_freeze → test_signatures → implement → validate → reviewer → synthesis → synthesis_negotiation → test_signatures (back) / synthesis (forward) → audit`.
- [ ] AC-2: `vibe-sprint.yaml` topology (post-review B-6/B-7): giữ node id (`context`, `tdd`, `coder`, `validate`, `synthesis`, `audit`), thêm `synthesis_negotiation`; edges: `context → tdd → coder → validate → synthesis → synthesis_negotiation → tdd (back) / synthesis (forward) → audit`. **Không đổi edge `synthesis → coder (when:continue, kind:back)`** (cũ) - vẫn dùng cho review loop.
- [ ] AC-3: Pack validation pass cho cả 2 flow (ValidateFlowSafetyTopology: `test_signatures`/`tdd` là writer, dominated by contract.freeze).
- [ ] AC-4: Runner khóa test files vào `ReadOnlyPaths` sau scaffold pass gate (single-write `LockScaffoldArtifacts` - B-8.3).
- [ ] AC-5: Runner snapshot `SignatureHash` + `LockedSignatures` vào `FrozenContractRecord` sau scaffold pass gate.
- [ ] AC-6 (post-review B-9): Không có `FLOWPILOT_ENABLE_CONTRACT_FIRST_TDD` flag - CP-67 luôn bật; legacy prompt giữ lại như artifact.
- [ ] AC-7: `bug-harness.yaml`, `bug-plan-harness.yaml`, `rag-harness.yaml` không bị ảnh hưởng (node id giữ nguyên, legacy prompt vẫn dùng).
- [ ] AC-8: `policy.negotiationCap: 5` được parse và enforce; `NegotiationRound` increment/reset đúng.
- [ ] AC-9: 6 test signatures green:
  - `TestTaskHarnessTopologyScaffoldNegotiationLoop`
  - `TestVibeSprintTopologyScaffoldNegotiationLoop`
  - `TestScaffoldNegotiationCapEnforcedAtFive`
  - `TestSynthesisNegotiationNodeRouting`
  - `TestScaffoldTestLockAndSignatureSnapshotSingleVersion`
  - `TestLegacyTestSignaturesFlowUnaffected`

## 7. Out of Scope

- Tool face schemas (P-1 / Task-378).
- Gate rule `r-signature-lock` core logic (P-2 / Task-379).
- TDD prompt content (P-3 / Task-380).
- Coder prompt content (P-4 / Task-381).
- Static stub-body validation adapters cho React/Kotlin/C/C++ — B-11, Task-383 (P-2b).

## 8. Completion Notes

- result: `todo`
- follow-ups: E2E verification, Kotlin/TS LSP extractor (nếu cần nâng cấp từ regex).
- upstream docs updated: `todo`
- **Post-review notes (B-6, B-7, B-9, B-10)**: giữ node id, thêm `synthesis_negotiation` node, không flag runtime, `negotiationCap: 5` phase-scoped.
