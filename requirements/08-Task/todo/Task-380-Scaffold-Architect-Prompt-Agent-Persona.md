# Task-380: Scaffold Architect Prompt, Agent Persona, And Behavior

## Metadata

- Document ID: `Task-380`
- Title: `Scaffold Architect Prompt, Agent Persona, And Behavior`
- Phase: `task`
- Status: `todo`
- Owner: `Claude Sonnet MAX`
- Reviewers: `Claude agent review`
- Created: `2026-09-18`
- Last Updated: `2026-09-19`
- Parent Documents: [CP-67 P-3](../../07-Coding-Plan/todo/CP-67-Contract-First-Scaffold-TDD-And-Signature-Lock.md)
- Child Documents: `None`
- Related Documents: [SP-06](../../04-System-Principle/SP-06-Oracle-Rule-And-Schema-First-Gate.md), [CP-64](../../07-Coding-Plan/done/CP-64-Reproduce-First-TDD-Gate.md), [Task-378](./Task-378-Scaffold-Coder-Declared-Face-Tool-Schemas.md), [Task-379](./Task-379-Signature-Lock-Rule-AST-Extractor.md)
- Replaces: `None`
- Tags: `contract-first-tdd, prompt-template, agent-pack, behavior-registry, scaffold-architect`
- Feature Keys: `contract-first-tdd`

## AI Quick View

### Summary

- Slice 3 của CP-67: tạo prompt template `prompts/scaffold-contract-tdd.md` thay thế tư duy "chỉ viết khung hàm test rỗng" của `test-signatures.md` trong task/vibe flow — TDD Agent đóng vai trò API Contract Architect, phải sinh cả Production Stubs (hàm có signature, body `TODO()/return nil`) VÀ test suite hoàn chỉnh executable (Compile XANH, Test ĐỎ).
- Tạo agent persona `agents/scaffold-architect.md` chuyên thiết kế API contract, và đăng ký behavior mới `agent.scaffold` vào runtime + reference registry + manifest.
- Prompt yêu cầu TDD Agent gọi tool `submit_scaffold_outcome` (Task-378) để bàn giao structured result.
- Slice này chưa wire vào flow nào (P-5 lo); chỉ bảo đảm pack load/validate/render trọn vẹn.

### Current Ask

- Implement P-3 theo CP-67 §4: 2 file mới (prompt + persona), đăng ký behavior `agent.scaffold`, cập nhật manifest, 3 test signatures phải xanh.

### Key Decisions

- `T-1` Prompt `scaffold-contract-tdd.md` hướng dẫn TDD Agent:
  - (1) Đọc Task Plan / Specs từ frozen contract.
  - (2) Tạo Production Stubs: hàm có signature đầy đủ (tên, params, return types), body bắt buộc nằm trong whitelist stub theo ngôn ngữ — Kotlin `TODO("not implemented")`, Go `return nil, errors.New("not implemented")`, TS/React `throw new Error("not implemented")`, C `return` zero-sentinel / `assert(0 && "not implemented")`, C++ `throw std::runtime_error("not implemented")` (B-11: whitelist được Task-383 enforce static bằng AST).
  - (3) Viết test assertions hoàn chỉnh gọi stubs.
  - (4) Chạy test xác nhận Compile OK + Test ĐỎ (RED). **Post-review B-4**: test phải thực sự RED (assertion failure hoặc not-implemented panic), không phải compile error, không phải all-green — enforced bởi `r-scaffold-red` gate.
  - (5) Gọi `submit_scaffold_outcome` với status `scaffold_ready`.
- `T-2` Behavior `agent.scaffold` scope `delegate`, runtime handler reuse `behaviorAgentDelegate` (giống `agent.reproduce` CP-64 T-2). Write-scope: file stub production MỚI + file test MỚI theo DeclaredPaths.
- `T-3` Persona `scaffold-architect.md`: API Contract Architect — tư duy thiết kế interface trước implementation, cam kết "mọi hàm phải có test ĐỎ chứng minh contract trước khi Coder chạm vào".
- `T-4` Prompt override rõ: nếu Agent mang persona tester cũ, prompt này GHI ĐÈ — KHÔNG viết empty signatures, PHẢI viết full stubs + full test bodies.

### Constraints

- Không sửa `test-signatures.md` hay `agents/tester.md` — các flow cũ (rag-harness, bug-harness trước CP-64, flow dùng legacy empty signatures) vẫn giữ nguyên.
- Không sửa `reproduce-failing-test.md` hay `agents/reproducer.md` — CP-64 BugFix flow vẫn hoạt động độc lập.
- Additive tests only.
- Provider parity: behavior `agent.scaffold` provider-agnostic.
- `manifest.yaml` phải khai báo file mới.

### Open Questions

- None.

### Source Refs

- CP-67 §4 P-3.
- `internal/agentpack/flow-pack/prompts/test-signatures.md` (đối trọng — legacy).
- `internal/agentpack/flow-pack/prompts/reproduce-failing-test.md` (mẫu prompt viết test thật).
- `internal/agentpack/flow-pack/agents/reproducer.md` (mẫu agent persona).
- `internal/runner/behavior_registry_builtin.go` (runtime registry).

## 1. Goal

Pack flow có đầy đủ 3 mảnh của node scaffold_tdd: prompt template hướng dẫn TDD Agent tạo stubs + red tests executable, persona scaffold-architect chuyên biệt, behavior `agent.scaffold` đăng ký runtime — pack load/validate/render thành công.

## 2. Parent Links

- coding plan: `CP-67-Contract-First-Scaffold-TDD-And-Signature-Lock.md` P-3
- tech design: `SD-20-Flow-Gate-Rule-Semantics.md`
- system spec: `SP-06-Oracle-Rule-And-Schema-First-Gate.md`

## 3. Trigger

P-1 (Task-378) đã có tool schema `submit_scaffold_outcome`; P-2 (Task-379) đã có gate `r-signature-lock`. Slice này tạo prompt + persona để TDD Agent biết cách sử dụng cả hai.

## 4. Exact Change

- `T-1` **`internal/agentpack/flow-pack/prompts/scaffold-contract-tdd.md`** (new): Hướng dẫn — (1) Đọc Task Plan từ frozen contract; (2) Tạo file production stubs với hàm/struct/interface signatures đầy đủ, body trong whitelist stub theo ngôn ngữ — Go `return nil, errors.New("not implemented")`, Kotlin `TODO("not implemented")`, TS/React `throw new Error("not implemented")`, C `return` zero-sentinel/`assert(0 && "not implemented")`, C++ `throw std::runtime_error("not implemented")` (B-11: whitelist được Task-383 enforce static bằng AST); (3) Viết file test mới với assertion hoàn chỉnh gọi stubs; (4) Chạy test suite — phải Compile XANH + Test ĐỎ (assertion failure hoặc not-implemented panic); (5) Gọi `submit_scaffold_outcome` với `stubs`, `test_suite`, `status: scaffold_ready`. CẤM viết test rỗng, CẤM viết implementation logic thật (bị bắt bởi cả GREEN-test lẫn static whitelist).
- `T-2` **`internal/agentpack/flow-pack/agents/scaffold-architect.md`** (new): Persona — Senior API Contract Architect, tư duy "thiết kế interface contract trước, implementation sau", cam kết test ĐỎ là bằng chứng contract tồn tại.
- `T-3` **`internal/agentpack/flow-pack/manifest.yaml`**: thêm `agents/scaffold-architect.md` vào `agents`, `prompts/scaffold-contract-tdd.md` vào `prompts`.
- `T-3b` (post-review B-3/B-5): **`internal/agentpack/pack.go`** (modified): thêm `"agent.scaffold": "agent.scaffold"` + aliases (`scaffold`, `scaffold_tdd`) vào `behaviorAliases`; mở model-allowlist (line ~1020-1035) cho `agent.scaffold` và `agent.code` (B-5).
- `T-3c` (post-review B-3): **`internal/agentpack/flow_safety_topology.go`** (modified): classify `agent.scaffold` như writer (dominated-by-freeze + acceptance invariants).
- `T-3d` (post-review B-3): **`internal/runner/behavior_registry.go`** (modified): thêm `BehaviorAgentScaffold BehaviorID = "agent.scaffold"`, `IsScaffoldBehavior(id)`; scaffold node là frozen-contract writer (stub trong DeclaredPaths), file test mới miễn scope-drift (IsTestFile exemption).
- `T-3e` (post-review B-3): **`internal/runner/behavior_registry_builtin.go`** (modified): register `agent.scaffold` → `behaviorAgentDelegate`.
- `T-3f` (post-review B-3): **`internal/runner/flow_validate_audit_dispatch.go`** (modified): thêm `agent.scaffold` vào spawnable-target allowlist (line ~225) và `flowAgentCodeWriterNodes` (line ~2183).
- `T-4` **`internal/runner/behavior_registry_builtin.go`** + **`internal/agentpack/flow-pack/behaviors/registry.yaml`**: đăng ký behavior `agent.scaffold` (scope `delegate`, aliases `scaffold`/`scaffold_tdd`, purpose thiết kế API contract bằng stubs + red tests, runtimeHandler `behaviorAgentDelegate`, write-scope file stubs + file test theo DeclaredPaths).
- `T-5` **`internal/agentpack/scaffold_architect_test.go`** (new): **3 test signatures**:
  - `TestScaffoldArchitectPromptRendering`
  - `TestScaffoldBehaviorRegistration`
  - `TestScaffoldNodeAllowsModelField`

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/agentpack/flow-pack/prompts/scaffold-contract-tdd.md` (new)
  - `apps/local-runner/internal/agentpack/flow-pack/agents/scaffold-architect.md` (new)
  - `apps/local-runner/internal/agentpack/flow-pack/manifest.yaml` (modified)
  - `apps/local-runner/internal/agentpack/flow-pack/behaviors/registry.yaml` (modified — reference doc)
  - `apps/local-runner/internal/runner/behavior_registry_builtin.go` (modified — runtime registration)
  - `apps/local-runner/internal/agentpack/scaffold_architect_test.go` (new)
- modules: `agentpack`, `runner`
- routes: none
- tables: none

## 6. Acceptance Check

- [ ] AC-1: Prompt template render được khi pack load (không lỗi parse/template).
- [ ] AC-2: Pack validation pass — agent file + prompt file tồn tại trong manifest, behavior `agent.scaffold` resolve được ở runtime registry (`behavior_registry.go` + `behavior_registry_builtin.go`).
- [ ] AC-2b (post-review B-3): behavior `agent.scaffold` được đăng ký ở `agentpack/pack.go` (`behaviorAliases`) và `agentpack/flow_safety_topology.go` (writer classification).
- [ ] AC-3: Prompt bao gồm hướng dẫn: tạo stubs (4 nhóm ngôn ngữ: Go, Kotlin, TS/React, C/C++ — B-11), viết test assertions, chạy test (RED — B-4), gọi `submit_scaffold_outcome`.
- [ ] AC-4: Persona scaffold-architect.md khác biệt rõ với reproducer.md (tạo mới vs tái hiện bug) và tester.md (full stubs vs empty signatures).
- [ ] AC-5: `test-signatures.md`, `reproduce-failing-test.md`, `agents/tester.md`, `agents/reproducer.md` không đổi.
- [ ] AC-6 (post-review B-5): model-allowlist mở cho `agent.scaffold` — node scaffold có thể khai `model:` trong flow YAML, pack validation pass.
- [ ] AC-7: 3 test signatures green:
  - `TestScaffoldArchitectPromptRendering`
  - `TestScaffoldBehaviorRegistration`
  - `TestScaffoldNodeAllowsModelField`

## 7. Out of Scope

- Tool face schemas (P-1 / Task-378).
- Gate rule `r-signature-lock` (P-2 / Task-379).
- Coder prompt implement-scaffold-body (P-4 / Task-381).
- Flow topology wiring (P-5 / Task-382).

## 8. Completion Notes

- result: `todo`
- follow-ups: consumed by Task-381/382.
- upstream docs updated: `todo`
- **Post-review notes (B-3, B-5)**: behavior `agent.scaffold` đăng ký đầy đủ ở agentpack (behaviorAliases, safety topology) + runner (behavior_registry, spawnable target, writer classification); model-allowlist mở cho `agent.scaffold`; bỏ `context_profiles.go` reference.
