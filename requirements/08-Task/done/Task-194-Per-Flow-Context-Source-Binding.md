# Task-194: Per-Flow Context Source Binding

## Metadata

- Document ID: `Task-194`
- Title: `Per-Flow Context Source Binding`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-08`
- Last Updated: `2026-07-09`
- Parent Documents: [CP-44: Pluggable Context Source Registry](../../07-Coding-Plan/done/CP-44-Pluggable-Context-Source-Registry.md), [SD-22: Pluggable Context Source Registry](../../06-System-Tech-Design/SD-22-Pluggable-Context-Source-Registry.md), [SD-17: Context And Regression Engine](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md)
- Child Documents: `None`
- Related Documents: [Task-193: Context Package Sections And Compatibility Projection](Task-193-Context-Package-Sections-And-Compat-Projection.md), [Task-173: AgentPack Schema And Parser](../done/Task-173-AgentPack-Schema-And-Parser.md), [CP-42: Flow Pack And Generic Node Behavior Refactor](../../07-Coding-Plan/done/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md)
- Replaces: `None`
- Tags: `context-regression-engine, flow-mode, context-source, agentpack, flow-definition`

## AI Quick View

### Summary

- Cho flow khai báo **danh sách context source enabled + thứ tự** trong pack YAML (mở rộng khối `contexts:` đã có trong `rag-harness.yaml`).
- Flow không khai báo → dùng **default set** (3 nguồn built-in của Task-192) — tương thích ngược tuyệt đối.
- Source-id lạ trong pack → **fail-fast lúc load flow** (như `BehaviorRegistry.Resolve`), không silent no-op.

### Current Ask

- Thêm binding nguồn per-flow qua pack schema + validate lúc load, và truyền danh sách enabled vào `Collect`.

### Key Decisions

- `T-1` Nguồn enabled đọc từ pack `contexts:`; thiếu khai báo → default set built-in.
- `T-2` Validate flow lúc load: mọi source-id khai báo phải `Resolve` được, nếu không → lỗi load rõ ràng (không chạy flow lỗi).
- `T-3` Thứ tự pack theo `Priority` của source (v1); thứ tự khai báo trong YAML chỉ là gợi ý — chốt ở `Q-1`.
- `T-4` `behaviorContextProduce` đọc danh sách enabled của flow đang chạy và truyền vào `Collect`.

### Constraints

- Depends on Task-193.
- Không phá `rag-harness.yaml` hiện tại: khối `contexts:` đang trỏ `flow-context-package.yaml` phải tiếp tục hợp lệ.
- Không phá flow cũ không có khai báo nguồn.
- Reuse parser AgentPack (Task-173), không tạo schema loader song song.

### Open Questions

- `Q-1` **(RESOLVED → CP-44 `P-8`)** Thứ tự không phải ràng buộc cứng — yêu cầu là import **đầy đủ** mọi source enabled. Giữ default theo `Priority`; KHÔNG thêm field `order` per-flow.
- `Q-2` Per-project override (bật/tắt nguồn ngoài phạm vi pack) — để Task sau nếu cần.

### Source Refs

- `CP-44 P-4`, `DOD-4`
- `Task-173` (AgentPack schema/parser), `CP-42`
- current code: `apps/local-runner/internal/agentpack/*` (schema `contexts:`), `flow-pack/flows/rag-harness.yaml`, `flow-pack/contexts/flow-context-package.yaml`, `behavior_registry_builtin.go` (`behaviorContextProduce`)

## 1. Goal

Cho phép mỗi flow chọn tập context source một cách khai báo, với default an toàn và validate fail-fast, để thêm/bớt loại context là việc sửa pack chứ không phải sửa Go.

## 2. Parent Links

- coding plan: `CP-44`
- tech design: `SD-17`
- system spec: `SS-13`
- specific upstream ids: `CP-44 P-4`, `CP-44 DOD-4`

## 3. Trigger

Sau Task-193 package đã chứa Sections động, nhưng `behaviorContextProduce` vẫn ngầm chạy toàn bộ default set. Để flow (và sau này user) bật nguồn MCP/Canonical Head có chọn lọc, cần binding khai báo per-flow.

## 4. Exact Change

- `T-1` Mở rộng schema AgentPack `contexts:` để một context có thể liệt kê `sources: [feature.history, chat.summary, source.excerpt, ...]` (giữ `ref` cũ hợp lệ).
- `T-2` Loader flow: sau parse, validate mọi source-id trong `sources` qua `ContextSourceRegistry.Resolve`; unknown → error load (thông điệp nêu flow id + source id).
- `T-3` Khi flow không khai báo `sources`, dùng hằng `defaultSourceIDs` (3 nguồn Task-192).
- `T-4` `behaviorContextProduce` lấy danh sách enabled của flow đang chạy (qua record/definition đã resolve) và truyền vào `registry.Collect(ctx, enabledIDs, hints)`.
- `T-5` Cập nhật `rag-harness.yaml`/`flow-context-package.yaml` khai báo tường minh 3 nguồn mặc định (documentation-as-config), giữ output không đổi.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/agentpack/pack.go` (schema `contexts.sources`)
  - `apps/local-runner/internal/agentpack/pack_test.go`
  - `apps/local-runner/internal/runner/behavior_registry_builtin.go` (`behaviorContextProduce`)
  - `apps/local-runner/internal/runner/flow_executor.go` hoặc nơi resolve flow (validate lúc load)
  - `apps/local-runner/internal/agentpack/flow-pack/flows/rag-harness.yaml`
  - `apps/local-runner/internal/agentpack/flow-pack/contexts/flow-context-package.yaml`
- modules:
  - agentpack schema/parser, flow resolve/validate, runner context assembly
- routes: none
- tables: none

## 6. Acceptance Check

- Flow không khai báo `sources` → chạy đúng default set, output như CP-41 Scenario 1.
- Flow khai báo tập con → package chỉ chứa các section đó.
- Source-id lạ trong pack → load flow trả lỗi rõ (không chạy).
- `rag-harness.yaml` sau cập nhật vẫn cho output không đổi (golden Task-192/193).

### 6.1 Test Items

- `TestFlowWithoutSourcesUsesDefaultSet`
- `TestFlowWithExplicitSourceSubset`
- `TestUnknownSourceIDFailsFlowLoad`
- `TestRagHarnessDeclaredSourcesMatchDefaultOutput`

### 6.2 Definition of Done

- [x] `DOD-1` Schema `contexts.sources` parse được; default khi vắng.
- [x] `DOD-2` Validate fail-fast cho source-id lạ.
- [x] `DOD-3` `behaviorContextProduce` truyền enabled set vào `Collect`.
- [x] `DOD-4` Không regression rag-harness output (golden).

## 7. Out of Scope

- Nguồn MCP thực tế (Task-195) — task này chỉ cho phép *khai báo* nguồn.
- UI Settings để user sửa nguồn (có thể là Task sau, tham chiếu Task-179 authoring UI).
- Per-project override.

## 8. Completion Notes

- result: `done` — 2026-07-08: `agentpack.FlowContextBinding.Sources` + `ValidateFlowContextSources` + `resolveEnabledContextSourceIDs` + wiring vào `flow_definition_resolver.go`/`flow_executor.go`/`behaviorContextProduce`; `rag-harness.yaml` khai báo tường minh; 6 test item pass. Precedence sau đó mở rộng ở Task-196 (step-level ưu tiên hơn flow-level).
- follow-ups: Task-195 đăng ký nguồn `mcp.driver` để flow khai báo được (xong); Task-196 thêm tầng step-definition-level (xong).
- upstream docs updated: none required.
