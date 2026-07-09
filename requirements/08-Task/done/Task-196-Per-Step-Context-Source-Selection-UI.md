# Task-196: Per-Step Context Source Selection UI (User-Authored Flows)

## Metadata

- Document ID: `Task-196`
- Title: `Per-Step Context Source Selection UI (User-Authored Flows)`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-08`
- Last Updated: `2026-07-09`
- Parent Documents: [CP-44: Pluggable Context Source Registry](../../07-Coding-Plan/done/CP-44-Pluggable-Context-Source-Registry.md), [SD-22: Pluggable Context Source Registry](../../06-System-Tech-Design/SD-22-Pluggable-Context-Source-Registry.md), [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md)
- Child Documents: `None`
- Related Documents: [Task-194: Per-Flow Context Source Binding](Task-194-Per-Flow-Context-Source-Binding.md), [Task-191: Context Source Interface And Registry](Task-191-Context-Source-Interface-And-Registry.md), [Task-195: MCP-Backed Context Source Adapter](Task-195-MCP-Backed-Context-Source-Adapter.md), [Task-179: Settings Flow Pack Authoring UI](../done/Task-179-Settings-Flow-Pack-Authoring-UI.md), [Task-189: Custom Flow Graph Authoring](../done/Task-189-Custom-Flow-Graph-Authoring.md), [BUG-236: Builtin Flow Mirror Stores Node Definition On Workflow Steps](../../09-BugFix/done/BUG-236-Builtin-Flow-Mirror-Stores-Node-Definition-On-Workflow-Steps-Instead-Of-Step-Definitions.md)
- Replaces: `None`
- Tags: `context-regression-engine, flow-mode, context-source, settings-ui, custom-flow, step-definition`

## AI Quick View

### Summary

- Task-194 cho **built-in / pack-YAML** flow khai báo context source qua `contexts.sources:`. Nhưng flow/step do **user tạo qua Settings UI** là DB-row (`step_definitions`/`workflows`), và đường lưu của UI (`saveWorkflow`/`saveStepDefinition`) **không** đụng tới binding đó — nên user không có cách nào tự chọn context source. Task-196 lấp đúng khoảng trống này.
- Cho user **chọn tập context source cho một step** ngay trong step-form của `WorkflowsSettings.tsx`, **tái dùng đúng lane multi-select của `required_mcps`** (đã có sẵn), thay cho ô text `contextRef` đơn lẻ.
- Lựa chọn được **giới hạn ở tập source app đang support** (các source đã đăng ký trên `ContextSourceRegistry` — Task-191), đúng ranh giới bảo mật CP-44 `P-7`/SD-22 `D-5`/SS-14 `AC-16`: không nhập id tùy ý.
- Persist vào `step_definitions.context_sources` (mirror `required_mcps`), **không** đụng `workflow_steps` (giữ contract BUG-236: node data chỉ ở step-definition). `behaviorContextProduce` đọc tập này và truyền vào `registry.Collect` (Task-194 `T-4`).
- Kết quả: flow user-tạo **không còn âm thầm rơi về default set** — user chủ động bật/tắt được nguồn (kể cả `mcp.driver` của Task-195) cho step Plan của họ.

### Current Ask

- Thêm UI + persistence + resolve để một step do user tạo/sửa qua Settings có thể chọn context source từ danh sách app-support, và tập đó thực sự chạy ở Plan-time.

### Key Decisions

- `T-1` Binding ở tầng **step-definition** (không phải `workflow_steps`), mirror `required_mcps`: thêm `contextSources: string[]` vào `StepDefinition`, cột `context_sources` trên bảng `step_definitions`. Giữ contract BUG-236 (node data chỉ ở step-definition; `workflow_steps` là bảng quan hệ thuần).
- `T-2` Danh sách chọn được = **source đã đăng ký** trên registry (Task-191). UI chỉ render các id hợp lệ; registry là **authority validate** (fail-fast lúc load flow như Task-194 `T-2`). Không cho nhập tự do.
- `T-3` `behaviorContextProduce` resolve tập enabled theo thứ tự ưu tiên: **(a) `step_definitions.context_sources`** nếu có → **(b)** flow-level `contexts.sources` (Task-194) → **(c)** `defaultSourceIDs` (3 nguồn built-in). Cùng một `registry.Collect(ctx, enabledIDs, hints)`.
- `T-4` UI reuse chip multi-select pattern của `requiredMcps` (`WorkflowsSettings.tsx` ~1644-1664 + toggle ~1980-2012), không dựng widget mới; ô `contextRef` cũ giữ lại tương thích (không xoá trong scope này).
- `T-5` Không thêm **bảng** Supabase mới — chỉ **cột** trên `step_definitions` (kế thừa ràng buộc SD-22 "không thêm bảng"; mirror precedent migration `20260701090000_add_flow_engine_attrs_to_workflows.sql`).

### Constraints

- Depends on Task-194 (plumbing `enabledIDs` → `Collect`, registry-as-validation, fail-fast) và Task-191 (registry). Task-195 không bắt buộc, nhưng khi có thì `mcp.driver` xuất hiện như một option chọn được.
- Không phá flow/step hiện có: step không chọn source → resolve về flow-level rồi default (tương thích ngược tuyệt đối).
- Giữ bất biến no-vector: chỉ liệt kê source `Deterministic()==true` đã đăng ký; không mở đường cho input tùy ý (SS-14 `AC-16`, CP-44 `P-7`).
- Desktop UI nói thẳng Supabase qua `SupabaseAdminRepository` (không qua HTTP local-runner — các endpoint đó đã bị gỡ, xem Task-179 notes). Giữ nguyên đường này.
- Không đổi `PackageID` / contract output package (Task-193 projection giữ nguyên).

### Open Questions

- `Q-1` Nguồn danh-sách-source cho UI: **(a)** hằng descriptor dùng chung trong `flowpilot-client-core` (id + label + `requiresMcp`) đồng bộ thủ công với Go registry — MVP nhanh, có **drift risk**; **(b)** seed một reference-table/endpoint từ Go registry — chống drift nhưng nặng hơn. **Đề xuất (a) cho v1** + Go registry vẫn là authority validate lúc load (mọi id sai đều fail-fast), giống cách `required_mcps` hoạt động. Chốt khi vào code.
- `Q-2` Có expose luôn per-flow override (flow chọn khác step-definition) trên UI không, hay chỉ step-level ở task này? Đề xuất: chỉ step-level (khớp yêu cầu gốc); flow-level override để task sau nếu cần (liên quan Task-194 `Q-2` per-project override).
- `Q-3` Có cần badge "cần MCP X" cạnh source MCP để user biết nó degrade khi MCP chưa connect không? Đề xuất: có, hiển thị trạng thái connect (tái dùng tín hiệu integration của `required_mcps`).

### Source Refs

- `CP-44 P-4/P-5/P-7` + `DOD-4`; task mới `CP-44 P-7`/`DOD-9`.
- `SD-22 D-4` (per-flow declared binding), `D-5` (external-source boundary); `SS-14 US-9`/`AC-16` (declarative extensibility, external chỉ integration đã support).
- `Task-194 T-1..T-4` (enabled-set + `Collect` + fail-fast validate) — task này là mặt UI/DB cho cùng cơ chế.
- `Task-179`/`Task-189` (Settings authoring UI + graph) — nơi field này nằm vào.
- `BUG-236` — node data chỉ ở `step_definitions`.
- current code: `WorkflowsSettings.tsx` (step-form, `requiredMcps` chips ~1644-1664 + toggle ~1980-2012, `contextRef` input ~1824); `supabaseAdminRepository.ts` (`mapStepDefinition` ~200, `saveStepDefinition` ~838/`required_mcps` ~844/`context_ref` ~861, `saveWorkflow` ~543/`required_mcps` ~720); `adminModels.ts` (`StepDefinition` ~194, `requiredMcps` ~199, `contextRef` ~216); `behavior_registry_builtin.go` (`behaviorContextProduce`); `flow_definition_resolver.go` (map step-definition → node).

## 1. Goal

Cho user tạo/sửa một step qua Settings và **chọn được các context source mà app đang support** cho step đó, với lựa chọn thực sự chạy ở Plan-time. Sau task này, tính pluggable của CP-44 áp dụng cho **cả flow user-tạo**, không chỉ pack YAML; flow user-tạo không còn bị khoá cứng ở 3 nguồn default.

## 2. Parent Links

- coding plan: `CP-44`
- tech design: `SD-22`
- system spec: `SS-14` (`US-9`, `AC-16`)
- specific upstream ids: `CP-44 P-7`, `CP-44 DOD-9`

## 3. Trigger

CP-44 (Task-191..195) làm registry + per-flow YAML binding, nhưng chỉ pack/YAML flow tận dụng được. Flow do user tạo qua `WorkflowsSettings.tsx` là DB-row và đường lưu UI không ghi context-source binding — nên với người dùng cuối, "chọn loại context cho flow của tôi" coi như không tồn tại. Task này nối UI → persistence → runner để mục tiêu của CP-44 đúng cho cả flow user-tạo.

## 4. Exact Change

- `T-1` Domain + persistence (client-core):
  - `StepDefinition` (`adminModels.ts`): thêm `contextSources: string[]` (mirror `requiredMcps`).
  - `mapStepDefinition` (`supabaseAdminRepository.ts`): đọc `context_sources` → mảng string (mirror `required_mcps` ~206).
  - `saveStepDefinition`: ghi `context_sources: step.contextSources` (mirror `required_mcps` ~844).
  - `saveWorkflow` (definition rows ~720): ghi `context_sources` cùng chỗ đang ghi `required_mcps`/`context_ref`.
- `T-2` Migration Supabase: thêm cột `context_sources` (jsonb/text[]) trên `step_definitions`, default `[]`. Mirror migration đã thêm `required_mcps`/flow-engine attrs. **Không** thêm bảng.
- `T-3` UI (`WorkflowsSettings.tsx`): trong step-form, thêm khối "Context Sources" dạng chip multi-select — clone block `requiredMcps` (~1644-1664) + toggle picker (~1980-2012). Options = danh sách source app-support (Q-1). Giữ ô `contextRef` cũ.
- `T-4` Supported-source catalog: dựng descriptor list (id + nhãn + `requiresMcp`) theo Q-1(a); nguồn built-in = `feature.history`, `chat.summary`, `source.excerpt` (+ `mcp.driver` nếu Task-195 đã land). Go registry giữ vai validate.
- `T-5` Runner resolve (`flow_definition_resolver.go` + `behaviorContextProduce`):
  - Map `step_definitions.context_sources` vào node đã resolve.
  - `behaviorContextProduce` chọn `enabledIDs` theo thứ tự ưu tiên `T-3` (step → flow → default) và truyền vào `registry.Collect`.
  - Giữ fail-fast: id không `Resolve` được → lỗi load flow (Task-194 `T-2`).
- `T-6` Tests (xem §6.1).

## 5. Touched Areas

- files:
  - `packages/flowpilot-client-core/src/domain/adminModels.ts` (`StepDefinition.contextSources`)
  - `packages/flowpilot-client-core/src/data/supabaseAdminRepository.ts` (map/save)
  - `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx` (step-form multi-select)
  - mới: Supabase migration thêm cột `step_definitions.context_sources`
  - `apps/local-runner/internal/runner/behavior_registry_builtin.go` (`behaviorContextProduce` resolve enabled-set)
  - `apps/local-runner/internal/runner/flow_definition_resolver.go` (map cột → node)
  - client-core: supported-source descriptor catalog (Q-1)
- modules:
  - desktop settings UI, client-core admin repo/domain, runner context assembly + flow resolve, Supabase schema (cột)
- routes: none (đi thẳng Supabase như hiện tại)
- tables: `step_definitions` (thêm cột, không thêm bảng)

## 6. Acceptance Check

- User tạo một step mới, chọn 2/3 source → lưu → reload: lựa chọn còn nguyên (persist đúng `context_sources`).
- Chạy flow chứa step đó ở Flow Mode → package Plan chỉ chứa các section của source đã chọn; source không chọn không xuất hiện.
- Step không chọn source → resolve về flow-level (nếu có) rồi default set — output như trước Task-196 (tương thích ngược).
- UI chỉ cho chọn source app-support; không có cách nhập id tuỳ ý; id lạ (nếu lọt vào DB) → flow load fail-fast, không silent.
- Chọn `mcp.driver` (khi Task-195 có) + MCP tắt → degrade-mềm (warning), Plan step vẫn hoàn tất.
- Không đổi `PackageID`; guard no-vector vẫn xanh; `workflow_steps` không nhận field mới (BUG-236).

### 6.1 Test Items

- `TestStepDefinitionContextSourcesRoundTripsThroughRepo` (map + save, client-core)
- `TestSaveWorkflowDoesNotWriteContextSourcesToWorkflowSteps` (BUG-236 contract)
- `TestBehaviorContextProduceUsesStepDefinitionSourcesOverFlowDefault` (precedence `T-3`)
- `TestUnknownContextSourceIDFailsFlowLoad` (fail-fast, tái dùng Task-194)
- `TestUserFlowWithoutSelectionFallsBackToDefaultSet` (tương thích ngược)
- UI: step-form render + toggle context-source chips (mirror test `requiredMcps` nếu có)

### 6.2 Definition of Done

- [x] `DOD-1` `StepDefinition.contextSources` + cột `step_definitions.context_sources` tồn tại; map/save round-trip.
- [x] `DOD-2` Step-form cho chọn source giới hạn ở tập app-support; không nhập tự do.
- [x] `DOD-3` `behaviorContextProduce` dùng tập của step (precedence step→flow→default) qua `Collect`; id lạ fail-fast.
- [x] `DOD-4` Flow user-tạo không chọn source vẫn chạy như trước (default set); `workflow_steps` không nhận field mới.
- [x] `DOD-5` Guard no-vector + `PackageID` không regression.

## 7. Out of Scope

- Nguồn `jira.ticket` / MCP mới (task riêng theo Task-195 follow-up).
- Per-project override tập nguồn (Task-194 `Q-2`).
- Flow-level source editor riêng trên UI (chỉ step-level ở task này — Q-2).
- Bỏ/deprecate ô `contextRef` cũ.
- Cache kết quả MCP.

## 8. Completion Notes

- result: `done` — 2026-07-09. `Q-1` chốt phương án (a) (descriptor constant `contextSourceOptions` trong `WorkflowsSettings.tsx`, Go registry là authority). `Q-2` giữ chỉ step-level (đúng đề xuất). `Q-3` chưa làm badge connect-status (nice-to-have, để sau).
  - **Client-core**: `StepDefinition.contextSources: string[]` (`adminModels.ts`); `mapStepDefinition`/`saveStepDefinition`/`cloneWorkflow`'s definition-row build đều đọc/ghi `context_sources` (`supabaseAdminRepository.ts`).
  - **Migration**: `20260708170000_add_context_sources_step_definition.sql` — cột `jsonb not null default '[]'`, không thêm bảng.
  - **UI**: `WorkflowsSettings.tsx` — thêm `PickerModal` kind `context-source`, chip block "Context Sources" (mirror `requiredMcps`), picker modal render options từ `contextSourceOptions`; `createEmptyStepDraft` cập nhật field mới.
  - **Go**: `agentpack.FlowNode.ContextSources []string` (+ parse YAML `contextSources`); `dbStepDefinitionRow.ContextSources` + `workflowSelect` mở rộng cột + `recordFromWorkflowRow` set `node.ContextSources` (`supabase_workflow_flow_store.go`); `resolveEnabledContextSourceIDs` cập nhật precedence step→flow→default; `ValidateFlowContextSources` validate cả node-level.
  - **Verify**: `tsc --noEmit` (desktop-flowpilot) sạch; Go `go test ./internal/runner/... ./internal/agentpack/...` — 6 test mới pass (`context_source_step_precedence_test.go`), full suite chỉ 15 fail pre-existing không liên quan.
  - Client-core test items (`TestStepDefinitionContextSourcesRoundTripsThroughRepo`, `TestSaveWorkflowDoesNotWriteContextSourcesToWorkflowSteps`) **không viết được** — repo không có test harness cho `flowpilot-client-core` (0 file `*.test.ts` trong package này); verify bằng typecheck + đọc code thay vì test tự động.
  - `upsertNodeStepDefinitions` (Go-side mirror-writer, khác đường với UI save) **không** ghi `context_sources` — nhất quán vì nó cũng không ghi `context_ref` từ trước; không phải regression của task này.
- follow-ups: cân nhắc flow-level override UI; đồng bộ descriptor list ↔ Go registry chống drift (Q-1); CP-43 Canonical Head khi land cũng sẽ là một option chọn được ở đây; badge connect-status (Q-3); test harness cho `flowpilot-client-core` (gap có sẵn, không riêng task này).
- upstream docs updated: Task-195 (liên kết Task-204 follow-up).
