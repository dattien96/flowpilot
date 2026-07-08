# CP-45: Generic Artifact Types And User-Scoped Artifact Instances

## Metadata

- Document ID: `CP-45`
- Title: `Generic Artifact Types And User-Scoped Artifact Instances`
- Phase: `coding_plan`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-08`
- Last Updated: `2026-07-08`
- Parent Documents: [SD-22: Pluggable Context Source Registry](../../06-System-Tech-Design/SD-22-Pluggable-Context-Source-Registry.md), [SD-17: Context And Regression Engine](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md), [SS-13: AI-Followable Document Contract](../../05-System-Specs/SS-13-AI-Followable-Document-Contract.md), [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md)
- Child Documents: [Task-197: Artifact Type Catalog And Schema](../../08-Task/todo/Task-197-Artifact-Type-Catalog-And-Schema.md), [Task-198: Artifact Instance Model And Resolver](../../08-Task/todo/Task-198-Artifact-Instance-Model-And-Resolver.md), [Task-199: Artifact Instance Settings Page](../../08-Task/todo/Task-199-Artifact-Instance-Settings-Page.md), [Task-200: Step Artifact Instance Binding UI](../../08-Task/todo/Task-200-Step-Artifact-Instance-Binding-UI.md), [Task-201: Context Artifact Migration From Context Sources](../../08-Task/todo/Task-201-Context-Artifact-Migration-From-Context-Sources.md), [Task-202: File Artifact Type Proof Of Generality](../../08-Task/todo/Task-202-File-Artifact-Type-Proof-Of-Generality.md), [Task-203: Artifact Framework Validation And Fallback](../../08-Task/todo/Task-203-Artifact-Framework-Validation-And-Fallback.md)
- Related Documents: [CP-44: Pluggable Context Source Registry](./CP-44-Pluggable-Context-Source-Registry.md), [Task-196: Per-Step Context Source Selection UI](../../08-Task/todo/Task-196-Per-Step-Context-Source-Selection-UI.md), [Task-168: Flow Mode Context Package Contract](../../08-Task/done/Task-168-Flow-Mode-Context-Package-Contract.md), [Task-176: Node-Behavior Registry And Dispatch](../../08-Task/done/Task-176-Node-Behavior-Registry-And-Dispatch.md), [BUG-236: Builtin Flow Mirror Stores Node Definition On Workflow Steps Instead Of Step Definitions](../../09-BugFix/done/BUG-236-Builtin-Flow-Mirror-Stores-Node-Definition-On-Workflow-Steps-Instead-Of-Step-Definitions.md)
- Replaces: `None`
- Tags: `artifact, artifact-type, artifact-instance, flow-mode, typed-contract, settings-ui, supabase`

## AI Quick View

### Summary

- CP-45 tổng quát hóa ý tưởng của CP-44: thay vì chỉ có **context source → context package**, hệ thống có một **artifact framework chung** nơi node output/input các artifact có type rõ ràng.
- `ArtifactType` là **system-owned contract**; user **không** tự tạo type ở v1. User chỉ tạo **artifact instance** dựa trên type có sẵn, rồi bind instance đó vào step làm input/output.
- Cùng một type có thể có **nhiều instance** với config khác nhau. Ví dụ `context_artifact.v1` có instance `main_context` dùng `feature.history + chat.summary`, và instance `review_context` dùng `source.excerpt + mcp.driver`.
- Cần một **page mới** để author artifact instances từ type có sẵn, và khi tạo/sửa step phải cho chọn artifact instance cho từng input/output slot.
- Khác CP-44: ở đây **cần Supabase tables thật** để lưu artifact type catalog, artifact instances, và step-to-instance bindings; không còn đủ với pack YAML/context column đơn lẻ.

### Current Ask

- Thiết kế một framework artifact tổng quát hơn CP-44: type hệ thống + instance do user tạo + step binding theo instance, đủ để sau này context/file/review artifact dùng chung một cơ chế.

### Key Decisions

- `P-1` V1 chỉ cho **system-owned artifact types**; user **không** được tạo custom type tự do. User chỉ tạo **artifact instances** dựa trên type đã định nghĩa sẵn.
- `P-2` `ArtifactInstance` giữ **producer config theo instance** (`sources`, `paths`, filters, options...), không nhét config cố định vào `ArtifactType`.
- `P-3` Khi tạo/sửa step, user phải chọn **artifact instance** cho input/output slot; không chọn raw source/path trực tiếp ở step nếu type đã che được abstraction đó.
- `P-4` Cần **page authoring artifact instances** riêng trong Settings để tạo/sửa instance dựa trên type có sẵn, thay vì nhồi vào step form.
- `P-5` Cần Supabase tables cho `artifact_types`, `artifact_instances`, và `step_artifact_bindings`; binding nằm ở **step-definition layer**, không phải `workflow_steps` (giữ contract BUG-236).
- `P-6` CP-44 trở thành **vertical slice đầu tiên** của `context_artifact.v1`; `contextSources` step-level của Task-196 là cơ chế chuyển tiếp, không phải mô hình cuối cùng.

### Constraints

- Không phá runner path hiện có của CP-44/Task-168/169/171; artifact framework mới phải có đường migrate từ `flow_context_package.v1`.
- Không mở custom artifact type DSL ở v1; nếu mở quá sớm sẽ thành plugin platform lớn hơn scope.
- Không cho step binding ghi dữ liệu cấu hình riêng xuống `workflow_steps`; metadata node/step tiếp tục nằm ở `step_definitions` hoặc bảng liên kết riêng trỏ về `step_definitions`.
- Phải hỗ trợ **nhiều instance cùng một type** trong một flow/project.
- Phải có validation rõ ràng: step input/output chỉ bind được instance có `artifact_type` tương thích.

### Open Questions

- `Q-1` Có cần tạo `SD-23` riêng cho artifact framework tổng quát này không, hay CP-45 đủ làm thiết kế tạm thời trước khi code? Đề xuất: nếu owner approve scope này thì nên có SD-level record sau.
- `Q-2` Step binding có cần bảng riêng `step_artifact_bindings`, hay đủ với 2 cột JSON trên `step_definitions` (`input_artifact_instance_ids` / `output_artifact_instance_ids`)? Đề xuất: bảng riêng để giữ `direction`, `slot_name`, `required`, `position`.
- `Q-3` Artifact instance có project-scoped hoàn toàn, hay cần workspace-global/library scope? Đề xuất: v1 project-scoped trước, thêm shared scope sau.

### Source Refs

- `CP-44` `P-3/P-4/P-7` — typed context artifact + flow/step binding, nhưng còn hẹp ở context.
- `SD-22` `D-3/D-4/D-5` — typed context sections, per-flow declared binding, deterministic external boundary.
- `Task-196` — hiện mới cho step chọn `contextSources`, là lớp hẹp cần được supersede bởi artifact instance selection.
- current code: `agentpack/pack.go` (`ContextArtifact`, `FlowContextBinding.Ref`), `flow_context_package.go`, `behavior_registry_builtin.go`, `WorkflowsSettings.tsx`, `supabaseAdminRepository.ts`.

## 1. Goal

Tổng quát hóa mô hình "typed context package" thành một **artifact framework chung** cho flow engine: node A có thể output một artifact instance có type rõ ràng, node B bind artifact instance đó làm input, và UI/DB cho phép user author/manage các artifact instance dựa trên các type hệ thống đã định nghĩa sẵn.

## 2. Input Documents

- `requirements/06-System-Tech-Design/SD-22-Pluggable-Context-Source-Registry.md`
- `requirements/06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md`
- `requirements/05-System-Specs/SS-13-AI-Followable-Document-Contract.md`
- `requirements/05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md`
- `requirements/07-Coding-Plan/todo/CP-44-Pluggable-Context-Source-Registry.md`
- `requirements/08-Task/todo/Task-196-Per-Step-Context-Source-Selection-UI.md`
- `apps/local-runner/internal/agentpack/flow-pack/contexts/flow-context-package.yaml`
- `apps/local-runner/internal/agentpack/flow-pack/flows/rag-harness.yaml`

## 3. Implementation Strategy

- overall approach:
  - Tách rõ 3 lớp: **ArtifactType** (system-owned contract), **ArtifactInstance** (user-authored/configured object), **StepArtifactBinding** (input/output binding giữa step và instance).
  - Giữ CP-44 sống như implementation đầu tiên của `context_artifact.v1`, rồi dần migrate step selection từ `contextSources[]` sang artifact instance selection.
  - Dùng Supabase làm source of truth cho type catalog, instance authoring, và step bindings để UI authoring không còn phụ thuộc pack file cứng.
- sequencing logic:
  - `P-1` chốt model/type-system + schema.
  - `P-2` dựng type catalog + runner resolver.
  - `P-3` dựng artifact instance authoring page.
  - `P-4` cho step chọn input/output artifact instances.
  - `P-5` migrate `contextSources` của CP-44 thành `context_artifact` instances.
  - `P-6` mở type thứ hai (`file_artifact.v1`) để chứng minh framework tổng quát.
- dependencies:
  - phụ thuộc CP-44 đã xong runner-side (`Task-191..195`) vì `context_artifact.v1` tái dùng `ContextSourceRegistry`.
  - phần UI/step authoring phụ thuộc `Task-179`/`Task-189` surfaces hiện có trong Settings.

## 4. Work Breakdown

- `P-1` Artifact type system and contracts.
  - Child task: [Task-197](../../08-Task/todo/Task-197-Artifact-Type-Catalog-And-Schema.md).
  - Định nghĩa `ArtifactType` metadata: `id`, `version`, `category`, `producerBehavior`, `consumerHints`, `configSchema`, `renderTemplate`, `systemOwned`.
  - Chốt rule v1: type là built-in only; user không tự định nghĩa type mới.
  - Định nghĩa `ArtifactInstance`: `id`, `project_id`, `artifact_type_id`, `name`, `description`, `config_json`, `status`.

- `P-2` Supabase schema + resolver.
  - Child task: [Task-198](../../08-Task/todo/Task-198-Artifact-Instance-Model-And-Resolver.md).
  - Thêm bảng `artifact_types` (seeded/system-owned catalog).
  - Thêm bảng `artifact_instances` (project-scoped, user-authored instances from built-in types).
  - Thêm bảng `step_artifact_bindings`:
    - `step_definition_id`
    - `direction` (`input` | `output`)
    - `slot_name`
    - `artifact_instance_id`
    - `required`
    - `position`
  - Runner/client-core resolve step bindings qua các bảng này.

- `P-3` Artifact instance authoring UI.
  - Child task: [Task-199](../../08-Task/todo/Task-199-Artifact-Instance-Settings-Page.md).
  - Tạo page/surface mới trong Settings để list/create/edit/delete artifact instances.
  - User flow:
    1. chọn built-in `ArtifactType`
    2. nhập tên instance
    3. điền config hợp lệ theo type
    4. save instance vào Supabase
  - V1 page này là nơi author chính; step form chỉ bind instance, không author instance inline.

- `P-4` Step editor binds artifact instances.
  - Child task: [Task-200](../../08-Task/todo/Task-200-Step-Artifact-Instance-Binding-UI.md).
  - Khi tạo/sửa step, UI phải cho chọn artifact instance cho input/output slot.
  - Step không tự giữ raw config của artifact; nó chỉ giữ binding tới instance.
  - Validation:
    - chỉ hiện instance có type tương thích
    - chặn bind sai `artifact_type`
    - support nhiều input/output slots

- `P-5` Context artifact migration layer.
  - Child task: [Task-201](../../08-Task/todo/Task-201-Context-Artifact-Migration-From-Context-Sources.md).
  - Định nghĩa built-in type `context_artifact.v1`.
  - Migrate logic `contextSources[]` thành `ArtifactInstance.config_json.sources`.
  - `Task-196` trở thành lớp chuyển tiếp:
    - ngắn hạn: vẫn support `contextSources[]`
    - dài hạn: step Plan chọn `context_artifact` instance thay vì raw source list

- `P-6` Add second built-in type to prove generality.
  - Child task: [Task-202](../../08-Task/todo/Task-202-File-Artifact-Type-Proof-Of-Generality.md).
  - Thêm built-in `file_artifact.v1` với config kiểu `paths: []`.
  - Consumer side có thể render/prompt-inject theo type contract, ví dụ step B đọc file list được bind sẵn.

- `P-7` Validation, migration, and fallback.
  - Child task: [Task-203](../../08-Task/todo/Task-203-Artifact-Framework-Validation-And-Fallback.md).
  - Backward compat cho flow chưa có artifact instance binding.
  - Nếu step không bind instance:
    - path cũ của CP-44 vẫn chạy cho `context.produce`
  - Migrate dần chứ không hard-cut.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/agentpack/pack.go`
  - `apps/local-runner/internal/runner/behavior_registry_builtin.go`
  - `apps/local-runner/internal/runner/flow_definition_resolver.go`
  - `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx`
  - mới: artifact instance management page/component(s) trong `apps/desktop-flowpilot/src/components/settings/`
  - `packages/flowpilot-client-core/src/domain/adminModels.ts`
  - `packages/flowpilot-client-core/src/data/supabaseAdminRepository.ts`
- modules:
  - runner artifact resolution
  - settings UI authoring
  - client-core admin repo/domain
  - flow definition storage/binding
- database:
  - `artifact_types`
  - `artifact_instances`
  - `step_artifact_bindings`
- external systems:
  - none beyond existing Supabase usage

## 6. Data or Migration Steps

- schema:
  - add `artifact_types` table (seeded built-ins, system-owned)
  - add `artifact_instances` table
  - add `step_artifact_bindings` table
- data backfill:
  - seed built-in types:
    - `context_artifact.v1`
    - `file_artifact.v1` (if landed in same slice)
  - optional migration helper to convert existing `step_definitions.context_sources` into one default `context_artifact` instance per step/workflow
- config updates:
  - step editor stops treating `contextRef`/`contextSources` as final abstraction
  - artifact instance page added to Settings nav

## 7. Validation Plan

- tests to add:
  - type catalog load/seed tests
  - instance CRUD repo tests
  - `step_artifact_bindings` round-trip tests
  - runner resolve precedence + type compatibility tests
  - backward-compat tests for steps still using CP-44-style context source selection
- manual checks:
  - create a `context_artifact` instance from built-in type, bind it to Plan step, run flow
  - create two instances of the same type with different configs and bind them to different steps
  - create a `file_artifact` instance and confirm consumer step receives the expected prompt/render behavior
- failure cases:
  - bind instance of wrong type to slot
  - delete instance still bound to step
  - stale step binding points to missing instance
  - unsupported type config payload saved from stale UI

## 8. Rollout and Fallback

- rollout order:
  - ship schema + type catalog
  - ship instance authoring page
  - ship step binding UI
  - then migrate context step authoring from raw source list to instance binding
- fallback path:
  - steps without artifact instance bindings continue using CP-44 context path
  - old `contextSources[]` remains readable during migration
- monitoring:
  - count artifact instances by type
  - count step bindings by direction/type
  - runner warnings for missing or mismatched artifact instances

## 9. Risks

- `R-1` Scope explosion: artifact framework có thể trượt thành plugin/platform system. Mitigation: v1 chỉ built-in type + user-created instance.
- `R-2` Overlap/conflict với CP-44 `Task-196`. Mitigation: xem `contextSources[]` là transition layer; CP-45 supersede abstraction sau khi instance binding sẵn sàng.
- `R-3` Schema complexity tăng nhanh nếu step bindings nhét vào JSON cột. Mitigation: dùng bảng `step_artifact_bindings` thay vì nhồi vào `step_definitions`.
- `R-4` Consumer semantics chưa đủ rõ cho type mới. Mitigation: mỗi built-in type phải có contract/producer/consumer hints rõ ràng trước khi expose cho UI.
- `R-5` User confusion giữa type và instance. Mitigation: authoring UI tách làm 2 bề mặt rõ:
  - Artifact Types = read-only catalog
  - Artifact Instances = thứ user thực sự tạo và bind

## 10. Definition of Done

- [ ] `DOD-1` Có model rõ cho `ArtifactType`, `ArtifactInstance`, `StepArtifactBinding`; type là system-owned, instance là user-authored.
- [ ] `DOD-2` Có Supabase tables `artifact_types`, `artifact_instances`, `step_artifact_bindings` cùng repo/client-core mapping đầy đủ.
- [ ] `DOD-3` Có page mới trong Settings để create/edit artifact instance từ built-in type.
- [ ] `DOD-4` Khi tạo/sửa step, user bind được artifact instance vào input/output slots.
- [ ] `DOD-5` CP-44 path (`context_artifact`) chạy được như một built-in type đầu tiên, có đường migrate từ `contextSources[]`.
- [ ] `DOD-6` Ít nhất một built-in type thứ hai ngoài context chứng minh framework không bị hardcode cho context.
