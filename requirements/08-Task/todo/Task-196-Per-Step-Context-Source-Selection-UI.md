# Task-196: Per-Step Context Source Selection UI (User-Authored Flows)

## Metadata

- Document ID: `Task-196`
- Title: `Per-Step Context Source Selection UI (User-Authored Flows)`
- Phase: `task`
- Status: `superseded`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-08`
- Last Updated: `2026-07-08`
- Parent Documents: [CP-44: Pluggable Context Source Registry](../../07-Coding-Plan/todo/CP-44-Pluggable-Context-Source-Registry.md), [SD-22: Pluggable Context Source Registry](../../06-System-Tech-Design/SD-22-Pluggable-Context-Source-Registry.md), [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md)
- Child Documents: `None`
- Related Documents: [Task-194: Per-Flow Context Source Binding](Task-194-Per-Flow-Context-Source-Binding.md), [Task-191: Context Source Interface And Registry](Task-191-Context-Source-Interface-And-Registry.md), [Task-195: MCP-Backed Context Source Adapter](Task-195-MCP-Backed-Context-Source-Adapter.md), [CP-45: Generic Artifact Types And User-Scoped Artifact Instances](../../07-Coding-Plan/todo/CP-45-Generic-Artifact-Types-And-Instances.md), [Task-200: Step Artifact Instance Binding UI](Task-200-Step-Artifact-Instance-Binding-UI.md), [Task-201: Context Artifact Migration From Context Sources](Task-201-Context-Artifact-Migration-From-Context-Sources.md), [Task-179: Settings Flow Pack Authoring UI](../done/Task-179-Settings-Flow-Pack-Authoring-UI.md), [Task-189: Custom Flow Graph Authoring](../done/Task-189-Custom-Flow-Graph-Authoring.md), [BUG-236: Builtin Flow Mirror Stores Node Definition On Workflow Steps](../../09-BugFix/done/BUG-236-Builtin-Flow-Mirror-Stores-Node-Definition-On-Workflow-Steps-Instead-Of-Step-Definitions.md)
- Replaces: `None`
- Tags: `context-regression-engine, flow-mode, context-source, settings-ui, custom-flow, step-definition`

## AI Quick View

### Summary

- **SUPERSEDED, 2026-07-08:** CP-45 generalizes this task into artifact-instance binding. Do not implement this raw `contextSources[]` UI as the final UX unless CP-45 is explicitly deferred. The replacement path is [Task-200](Task-200-Step-Artifact-Instance-Binding-UI.md) (step selects artifact instances) + [Task-201](Task-201-Context-Artifact-Migration-From-Context-Sources.md) (`context_artifact.v1` stores selected context sources in instance config).
- Task-194 cho **built-in / pack-YAML** flow khai báo context source qua `contexts.sources:`. Nhưng flow/step do **user tạo qua Settings UI** là DB-row (`step_definitions`/`workflows`), và đường lưu của UI (`saveWorkflow`/`saveStepDefinition`) **không** đụng tới binding đó — nên user không có cách nào tự chọn context source. Task-196 lấp đúng khoảng trống này.
- Cho user **chọn tập context source cho một step** ngay trong step-form của `WorkflowsSettings.tsx`, **tái dùng đúng lane multi-select của `required_mcps`** (đã có sẵn), thay cho ô text `contextRef` đơn lẻ.
- Lựa chọn được **giới hạn ở tập source app đang support** (các source đã đăng ký trên `ContextSourceRegistry` — Task-191), đúng ranh giới bảo mật CP-44 `P-7`/SD-22 `D-5`/SS-14 `AC-16`: không nhập id tùy ý.
- Persist vào `step_definitions.context_sources` (mirror `required_mcps`), **không** đụng `workflow_steps` (giữ contract BUG-236: node data chỉ ở step-definition). `behaviorContextProduce` đọc tập này và truyền vào `registry.Collect` (Task-194 `T-4`).
- Kết quả: flow user-tạo **không còn âm thầm rơi về default set** — user chủ động bật/tắt được nguồn (kể cả `mcp.driver` của Task-195) cho step Plan của họ.

### Current Ask

- Superseded by CP-45: preserve this task as the record of the original CP-44 gap, but route implementation to artifact instance selection instead of raw context-source selection.

### Key Decisions

- `T-0` **SUPERSEDED BY CP-45:** Step authoring should select a `context_artifact.v1` artifact instance, not raw `contextSources[]`, once CP-45 is active.
- `T-1` Binding ở tầng **step-definition** (không phải `workflow_steps`), mirror `required_mcps`: thêm `contextSources: string[]` vào `StepDefinition`, cột `context_sources` trên bảng `step_definitions`. Giữ contract BUG-236 (node data chỉ ở step-definition; `workflow_steps` là bảng quan hệ thuần).
- `T-2` Danh sách chọn được = **source đã đăng ký** trên registry (Task-191). UI chỉ render các id hợp lệ; registry là **authority validate** (fail-fast lúc load flow như Task-194 `T-2`). Không cho nhập tự do.
- `T-3` `behaviorContextProduce` resolve tập enabled theo thứ tự ưu tiên: **(a) `step_definitions.context_sources`** nếu có → **(b)** flow-level `contexts.sources` (Task-194) → **(c)** `defaultSourceIDs` (3 nguồn built-in). Cùng một `registry.Collect(ctx, enabledIDs, hints)`.
- `T-4` UI reuse chip multi-select pattern của `requiredMcps` (`WorkflowsSettings.tsx` ~1644-1664 + toggle ~1980-2012), không dựng widget mới; ô `contextRef` cũ giữ lại tương thích (không xoá trong scope này).
- `T-5` Không thêm **bảng** Supabase mới — chỉ **cột** trên `step_definitions` (kế thừa ràng buộc SD-22 "không thêm bảng"; mirror precedent migration `20260701090000_add_flow_engine_attrs_to_workflows.sql`).

### Constraints

- Do not implement this task as written while CP-45 is the active direction; implement [Task-200](Task-200-Step-Artifact-Instance-Binding-UI.md) and [Task-201](Task-201-Context-Artifact-Migration-From-Context-Sources.md) instead.
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

Original goal: cho user tạo/sửa một step qua Settings và **chọn được các context source mà app đang support** cho step đó, với lựa chọn thực sự chạy ở Plan-time. Superseded goal after CP-45: user chọn một `context_artifact.v1` artifact instance cho step; selected context sources live in the artifact instance config instead of raw step fields.

## 2. Parent Links

- coding plan: `CP-44`
- tech design: `SD-22`
- system spec: `SS-14` (`US-9`, `AC-16`)
- specific upstream ids: `CP-44 P-7`, `CP-44 DOD-9`

## 3. Trigger

CP-44 (Task-191..195) làm registry + per-flow YAML binding, nhưng chỉ pack/YAML flow tận dụng được. Flow do user tạo qua `WorkflowsSettings.tsx` là DB-row và đường lưu UI không ghi context-source binding — nên với người dùng cuối, "chọn loại context cho flow của tôi" coi như không tồn tại. Task này nối UI → persistence → runner để mục tiêu của CP-44 đúng cho cả flow user-tạo.

**Supersession trigger (2026-07-08):** CP-45 lifts the model from "step selects context source IDs" to "step binds artifact instances." The CP-44 gap remains real, but the fix should land through CP-45's artifact instance model so the same UI/DB pattern can support context, file, review, and future artifact types.

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

- [ ] `DOD-1` `StepDefinition.contextSources` + cột `step_definitions.context_sources` tồn tại; map/save round-trip.
- [ ] `DOD-2` Step-form cho chọn source giới hạn ở tập app-support; không nhập tự do.
- [ ] `DOD-3` `behaviorContextProduce` dùng tập của step (precedence step→flow→default) qua `Collect`; id lạ fail-fast.
- [ ] `DOD-4` Flow user-tạo không chọn source vẫn chạy như trước (default set); `workflow_steps` không nhận field mới.
- [ ] `DOD-5` Guard no-vector + `PackageID` không regression.

## 7. Out of Scope

- Final CP-45 artifact instance UX — owned by Task-200/Task-201.
- Nguồn `jira.ticket` / MCP mới (task riêng theo Task-195 follow-up).
- Per-project override tập nguồn (Task-194 `Q-2`).
- Flow-level source editor riêng trên UI (chỉ step-level ở task này — Q-2).
- Bỏ/deprecate ô `contextRef` cũ.
- Cache kết quả MCP.

## 8. Completion Notes

- result: `Superseded before implementation` — CP-45 replaces raw context source selection with artifact instance binding.
- follow-ups: implement [Task-200](Task-200-Step-Artifact-Instance-Binding-UI.md) and [Task-201](Task-201-Context-Artifact-Migration-From-Context-Sources.md).
- upstream docs updated: [CP-45](../../07-Coding-Plan/todo/CP-45-Generic-Artifact-Types-And-Instances.md)
