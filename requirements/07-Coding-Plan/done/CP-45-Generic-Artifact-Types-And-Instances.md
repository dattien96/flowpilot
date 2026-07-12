# CP-45: Generic Artifact Types And User-Scoped Artifact Instances

## Metadata

- Document ID: `CP-45`
- Title: `Generic Artifact Types And User-Scoped Artifact Instances`
- Phase: `coding_plan`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-08`
- Last Updated: `2026-07-12`
- Parent Documents: [SD-23: Generic Artifact Framework](../../06-System-Tech-Design/SD-23-Generic-Artifact-Framework.md), [SD-22: Pluggable Context Source Registry](../../06-System-Tech-Design/SD-22-Pluggable-Context-Source-Registry.md), [SD-17: Context And Regression Engine](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md), [SS-13: AI-Followable Document Contract](../../05-System-Specs/SS-13-AI-Followable-Document-Contract.md), [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md)
- Child Documents: [Task-197: Artifact Type Catalog And Schema](../../08-Task/done/Task-197-Artifact-Type-Catalog-And-Schema.md), [Task-198: Artifact Instance Model And Resolver](../../08-Task/done/Task-198-Artifact-Instance-Model-And-Resolver.md), [Task-199: Artifact Instance Settings Page](../../08-Task/done/Task-199-Artifact-Instance-Settings-Page.md), [Task-200: Step Artifact Instance Binding UI](../../08-Task/done/Task-200-Step-Artifact-Instance-Binding-UI.md), [Task-201: Context Artifact Migration From Context Sources](../../08-Task/done/Task-201-Context-Artifact-Migration-From-Context-Sources.md), [Task-202: File Artifact Type Proof Of Generality](../../08-Task/done/Task-202-File-Artifact-Type-Proof-Of-Generality.md), [Task-203: Artifact Framework Validation And Fallback](../../08-Task/done/Task-203-Artifact-Framework-Validation-And-Fallback.md), [Task-205: Built-in Artifact Flow (Context → Coding → Review → Synthesis)](../../08-Task/done/Task-205-Builtin-Artifact-Flow-Context-Coding-Review-Synthesis.md), [Task-222: Artifact-Only Step UX](../../08-Task/done/Task-222-Artifact-Only-Step-UX-And-File-Artifact-Semantics-Copy.md), [Task-223: File Artifact Output Contract](../../08-Task/done/Task-223-File-Artifact-Output-Contract-And-Review-Input-Chain.md), [Task-224: Flow Prompt Scoping And Coder Why Template](../../08-Task/done/Task-224-Flow-Prompt-Scoping-And-Coder-Output-Why-Template.md)
- Related Documents: [CP-44: Pluggable Context Source Registry](./CP-44-Pluggable-Context-Source-Registry.md), [Task-196: Per-Step Context Source Selection UI](../../08-Task/todo/Task-196-Per-Step-Context-Source-Selection-UI.md), [Task-168: Flow Mode Context Package Contract](../../08-Task/done/Task-168-Flow-Mode-Context-Package-Contract.md), [Task-176: Node-Behavior Registry And Dispatch](../../08-Task/done/Task-176-Node-Behavior-Registry-And-Dispatch.md), [BUG-236: Builtin Flow Mirror Stores Node Definition On Workflow Steps Instead Of Step Definitions](../../09-BugFix/done/BUG-236-Builtin-Flow-Mirror-Stores-Node-Definition-On-Workflow-Steps-Instead-Of-Step-Definitions.md), [Task-225: File Artifact Instance Output Structure](../../08-Task/done/Task-225-File-Artifact-Instance-Output-Structure.md) (Phase-2 polish, not blocking CP-45)
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

- **Closed (2026-07-12).** Implementation + live E2E verified (gate-sandbox). Residual polish (per-instance file output structure + optional section gate) tracked as Task-225, not blocking this CP.

### Key Decisions

- `P-1` V1 chỉ hardcode **system-owned artifact types**; user **không** tạo custom type. **Code chỉ hardcode lớp `ArtifactType`.**
- `P-2` `ArtifactInstance` có `name` + giữ **producer config theo instance** (`sources`, `paths`, filters...), validate app-level theo `configSchema`; `ArtifactType` không giữ config. Cùng type → nhiều instance (vd built-in `context_artifact` default đủ source vs user instance chỉ `mcp.driver`).
- `P-2b` **Instance có 2 nguồn**: built-in instance **seeded/force-migrate** (`is_builtin=true`) giống built-in Flow — read-only, RLS chặn user ghi, hiện tag "built-in"; user instance (`is_builtin=false`) do user tạo và đặt tên. Mirror `EnsureBuiltinFlowMirrorsWithStore` + RLS `is_builtin=false`.
- `P-3` Khi tạo/sửa step, user attach **danh sách artifact** cho **cả input lẫn output** (nhiều artifact mỗi chiều); step chỉ giữ binding tới instance, không giữ raw config. Artifact là **I/O chéo-step**: output của step A = input của step B qua cùng instance.
- `P-4` Trang authoring artifact instance là **tab thứ 3 (`artifacts`)** trong Flow settings (cạnh `workflows`/`steps`), không phải nav riêng; author instance ở đây, step form chỉ bind.
- `P-5` Cần Supabase tables `artifact_types`, `artifact_instances` (có `is_builtin`), `step_artifact_bindings`; binding ở **step-definition layer**, không phải `workflow_steps` (BUG-236).
- `P-6` CP-44 là **vertical slice đầu tiên** của `context_artifact.v1`; `contextSources` step-level của Task-196 là chuyển tiếp, không phải mô hình cuối.
- `P-7` **Dispatch qua `ArtifactTypeRegistry` nhỏ** (mirror `ContextSourceRegistry`), per-type resolver gọi ở prompt-assembly seam — không thêm behavior node. `file_artifact` resolver inject file path vào prompt consumer.
- `P-8` Ship **built-in flow chuẩn** `Context → Coding → Review → Synthesis` (giống built-in review-loop) làm nơi wiring artifact sẵn + E2E vehicle; user flow tự tạo tự quyết dùng Context.

### Constraints

- Không phá runner path hiện có của CP-44/Task-168/169/171; artifact framework mới phải có đường migrate từ `flow_context_package.v1`.
- Không mở custom artifact type DSL ở v1; nếu mở quá sớm sẽ thành plugin platform lớn hơn scope.
- Không cho step binding ghi dữ liệu cấu hình riêng xuống `workflow_steps`; metadata node/step tiếp tục nằm ở `step_definitions` hoặc bảng liên kết riêng trỏ về `step_definitions`.
- Phải hỗ trợ **nhiều instance cùng một type** trong một flow/project.
- Phải có validation rõ ràng: step input/output chỉ bind được instance có `artifact_type` tương thích.

### Open Questions

- `Q-1` **(RESOLVED → [SD-23](../../06-System-Tech-Design/SD-23-Generic-Artifact-Framework.md) created + `SS-14 US-10`/`AC-17` added, 2026-07-09)** Owner approve scope → dựng SD-23 làm design-of-record cho artifact framework tổng quát (đối xứng SD-22↔CP-44), và thêm `SS-14 US-10`/`AC-17` làm spec-authority cho "typed artifacts as first-class flow I/O". CP-45 giờ trace đủ chuỗi SS-14 US-10/AC-17 → SD-23 → CP-45.
- `Q-2` **(RESOLVED → SD-23 `D-4`)** Dùng **bảng riêng** `step_artifact_bindings` (giữ `direction`/`slot_name`/`required`/`position`), không nhồi JSON cột.
- `Q-3` **(RESOLVED, 2026-07-09)** Instance **project-scoped** v1; shared/library scope để sau.
- `Q-4` **(RESOLVED, 2026-07-09 — batch verify)** config validate **app-level**; built-in instance seed qua migration/mirror-sync; file artifact **read live** + inject path; binding-failure dùng **Settings validation + runtime error** (không flow-gate modal); trang Artifacts là **tab thứ 3**. Chi tiết ở SD-23 `Q-2`..`Q-6`.

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
  - `P-5` migrate `contextSources` của CP-44 thành `context_artifact` instances (+ seed built-in default instance).
  - `P-6` mở type thứ hai (`file_artifact.v1`) + `ArtifactTypeRegistry` để chứng minh framework tổng quát.
  - `P-7` validation/type-compat/fallback + backward-compat CP-44 (Task-203).
  - `P-8` ship built-in flow `Context → Coding → Review → Synthesis` (Task-205) — E2E vehicle, phụ thuộc P-1..P-7.
- dependencies:
  - phụ thuộc CP-44 đã xong runner-side (`Task-191..195`) vì `context_artifact.v1` tái dùng `ContextSourceRegistry`.
  - phần UI/step authoring phụ thuộc `Task-179`/`Task-189` surfaces hiện có trong Settings.

## 4. Work Breakdown

- `P-1` Artifact type system and contracts.
  - Child task: [Task-197](../../08-Task/done/Task-197-Artifact-Type-Catalog-And-Schema.md).
  - Định nghĩa `ArtifactType` metadata: `id`, `version`, `category`, `producerBehavior`, `consumerHints`, `configSchema`, `renderTemplate`, `systemOwned`.
  - Chốt rule v1: type là built-in only; user không tự định nghĩa type mới.
  - Định nghĩa `ArtifactInstance`: `id`, `project_id`, `artifact_type_id`, `name`, `description`, `config_json`, `is_builtin`, `status`.

- `P-2` Supabase schema + resolver + built-in instance mirror.
  - Child task: [Task-198](../../08-Task/done/Task-198-Artifact-Instance-Model-And-Resolver.md).
  - Thêm bảng `artifact_types` (seeded/system-owned catalog).
  - Thêm bảng `artifact_instances` (project-scoped; cột `is_builtin`; user instance `is_builtin=false`).
  - Thêm bảng `step_artifact_bindings`: `step_definition_id`, `direction` (`input`|`output`), `slot_name`, `artifact_instance_id`, `required`, `position` (một step nhiều binding mỗi chiều).
  - RLS: `authenticated` chỉ ghi `artifact_instances` `is_builtin=false` (mirror workflows RLS); built-in seeded qua **service-role mirror-sync** (cạnh `EnsureBuiltinFlowMirrorsWithStore`).
  - Runner/client-core resolve step bindings qua các bảng này; runner-side resolver structs mang binding.

- `P-3` Artifact tab (authoring UI).
  - Child task: [Task-199](../../08-Task/done/Task-199-Artifact-Instance-Settings-Page.md).
  - Thêm **tab thứ 3 `artifacts`** trong Flow settings (`WorkflowsSettings.tsx`, `type Tab = "workflows"|"steps"|"artifacts"`).
  - Built-in instance hiện read-only với tag "built-in" (như built-in workflow `editable===false`); user list/create/edit/delete instance `is_builtin=false`.
  - User flow tạo: chọn built-in `ArtifactType` (catalog read-only) → nhập tên → điền config hợp lệ → save.
  - Page này là nơi author chính; step form chỉ bind, không author inline.

- `P-4` Step editor binds artifact list (input + output).
  - Child task: [Task-200](../../08-Task/done/Task-200-Step-Artifact-Instance-Binding-UI.md).
  - Step attach **danh sách** artifact cho **cả input lẫn output**; step chỉ giữ binding, không raw config.
  - Artifact là I/O chéo-step: bind cùng instance vào output step A + input step B.
  - Validation: chỉ hiện instance type tương thích; chặn bind sai `artifact_type`; nhiều binding mỗi direction.

- `P-5` Context artifact migration + built-in default.
  - Child task: [Task-201](../../08-Task/done/Task-201-Context-Artifact-Migration-From-Context-Sources.md).
  - Định nghĩa built-in type `context_artifact.v1`; seed **built-in instance default = đủ mọi source**.
  - `config_json.sources`; user có thể tạo instance khác (vd chỉ `mcp.driver`).
  - Migrate mềm: existing user flow **lazy** (tạo default instance khi edit); flow cũ chưa đụng vẫn chạy fallback CP-44.
  - `Task-196` là lớp chuyển tiếp (ngắn hạn support `contextSources[]`; dài hạn bind `context_artifact` instance).

- `P-6` Add second built-in type + registry seam.
  - Child task: [Task-202](../../08-Task/done/Task-202-File-Artifact-Type-Proof-Of-Generality.md).
  - Thêm `ArtifactTypeRegistry` nhỏ + built-in `file_artifact.v1` (config `paths: []`).
  - Luồng: step A output `file_artifact` (mang path) → step B bind làm input → resolver **inject file path** (workspace-safe) vào prompt step B.

- `P-8` Built-in standard flow (wiring vehicle).
  - Child task: [Task-205](../../08-Task/done/Task-205-Builtin-Artifact-Flow-Context-Coding-Review-Synthesis.md).
  - Ship built-in flow `Context → Coding → Review → Synthesis` (giống built-in review-loop), seed sẵn binding artifact chéo-step; đóng vai E2E vehicle.

- `P-7` Validation, migration, and fallback.
  - Child task: [Task-203](../../08-Task/done/Task-203-Artifact-Framework-Validation-And-Fallback.md).
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

- [x] `DOD-1` Có model rõ cho `ArtifactType`, `ArtifactInstance` (`name`, `is_builtin`), `StepArtifactBinding`; type system-owned (code chỉ hardcode type); instance có built-in seeded + user-authored. — Task-197 (type) + Task-198 (instance/binding)
- [x] `DOD-2` Có Supabase tables `artifact_types`, `artifact_instances` (`is_builtin` + RLS), `step_artifact_bindings` cùng repo/client-core mapping; built-in instance seeded qua migration upsert (idempotent — xem Task-198 `DOD-5` ghi chú lý do không phải Go mirror-sync). — Task-198
- [x] `DOD-3` Có **tab thứ 3 `artifacts`** trong Flow settings để create/edit artifact instance từ built-in type; built-in instance hiện read-only với tag "built-in". — Task-199
- [x] `DOD-4` Khi tạo/sửa step, user attach được **danh sách** artifact vào cả input lẫn output; artifact là I/O chéo-step. — Task-200
- [x] `DOD-5` CP-44 path (`context_artifact`) chạy được như một built-in type đầu tiên, có đường migrate từ `contextSources[]`. — Task-201
- [x] `DOD-6` Ít nhất một built-in type thứ hai ngoài context chứng minh framework không bị hardcode cho context. — Task-202
- [x] `DOD-7` Type-compatibility được enforce: slot chỉ bind instance khớp `artifact_type` + `direction`; bind sai / instance thiếu → fail-fast ở authoring **và** flow-load (SD-23 `D-7`). — Task-200 (UI) + Task-203 (runner validation)
- [x] `DOD-8` Backward-compat: flow CP-44 cũ (default set + flow-level `contexts.sources` + transition step `context_sources`) vẫn chạy qua fallback precedence (SD-23 `D-6`); required binding thiếu → lỗi rõ, optional thiếu → degrade-mềm. — Task-203
- [x] `DOD-9` Guard bất biến giữ nguyên xuyên suốt: no-vector/deterministic (CP-41), `PackageID` không đổi, `workflow_steps` không nhận artifact metadata (BUG-236). — Task-198/201/203
- [x] `DOD-10` Dispatch qua `ArtifactTypeRegistry` (mirror `ContextSourceRegistry`), per-type resolver ở prompt-assembly seam, không thêm behavior node; `file_artifact` inject path vào prompt consumer. — Task-202
- [x] `DOD-11` Có built-in flow chuẩn `Context → Coding → Review → Synthesis` chạy được, artifact binding chéo-step thật trong DB; xem Task-205 `DOD-3` ghi chú scope (prompt injection thật chỉ ở 1 điểm dispatch hiện có, chưa mở rộng ra mọi consumer node). — Task-205

## 11. E2E Test Matrix

> **Scope note.** Matrix này phủ **artifact framework end-to-end** (type catalog → instance authoring → step binding → runtime resolve → consume), gồm cả đường fallback về CP-44. Các use case runner-level thuần của CP-44 (`§11.1`–`§11.7` của CP-44) được **kế thừa nguyên** làm substrate — xem ledger `§11.1` bên dưới; chỉ đường **user-authored raw `step_definitions.context_sources`** (Task-196) bị CP-45 override và **mark NOT TEST**.

### 11.1 CP-44 E2E Reuse Ledger

| CP-44 case | Nội dung | Verdict cho CP-45 |
|-----------|----------|-------------------|
| `11.1` Default Built-In Flow Works Unchanged | rag-harness default set không đổi hành vi | **REUSE** — artifact framework không được đổi default; là baseline của `§11.8` backward-compat. |
| `11.2` Flow With Explicit Source Subset | per-flow `contexts.sources` (pack YAML) | **REUSE** — chính là nhánh fallback flow-level của `D-6`; vẫn phải xanh. |
| `11.3` Unknown Source ID Fails Fast | source-id lạ trong pack → fail load | **REUSE + EXTEND** — mở rộng sang `ArtifactInstance.config_json.sources` lạ (`§11.5`). |
| `11.4` Generic Section Rendering For New Source | đăng ký source mới không sửa struct | **REUSE** — registry extensibility (SD-22) không đổi; `context_artifact` producer dựa lên nó. |
| `11.5` MCP Source Happy Path | `mcp.driver` bounded + SourceRef | **REUSE** — producer của `context_artifact` gọi cùng registry. |
| `11.6` MCP Timeout/Down Degrades | MCP lỗi → warning, không fail | **REUSE** — kế thừa degrade-mềm (`F-7`). |
| `11.7` Safety And Determinism Guards | outside-workspace omit, no-vector, thứ tự ổn định | **REUSE** — áp cho cả `file_artifact` (`§11.4`). |
| Task-196 raw `step_definitions.context_sources` UI selection (`TestStepDefinitionContextSourcesRoundTripsThroughRepo`, step-form context-source chips) | user chọn raw source-id trực tiếp trên step | **NOT TEST (superseded)** — CP-45 thay bằng bind `context_artifact` instance (Task-200/201). Chỉ giữ **runner precedence resolve** (`behaviorContextProduce` step→flow→default) như một mắt trong `D-6`, không test lại UI raw-source. |

### 11.2 Context Artifact Instance Drives Package

- Mục tiêu: chứng minh step bound tới `context_artifact` instance build package từ `config_json.sources` của instance.
- Cách test:
  1. Seed built-in type `context_artifact.v1`.
  2. Tạo instance `main_context` với `sources: [feature.history, chat.summary]`.
  3. Bind instance vào input slot của step Plan; run flow.
- Kỳ vọng:
  - `pkg.Sections` đúng bằng `feature.history` + `chat.summary`; `source.excerpt` vắng.
  - `PackageID` giữ công thức cũ; dòng `No vector retrieval used` còn.

### 11.3 Two Instances Same Type, Different Config

- Mục tiêu: chứng minh nhiều instance cùng type với config khác nhau hoạt động độc lập.
- Cách test:
  1. Tạo `main_context` (`sources: [feature.history, chat.summary]`) và `review_context` (`sources: [source.excerpt, mcp.driver]`).
  2. Bind `main_context` vào step A, `review_context` vào step B; run flow.
- Kỳ vọng:
  - Package của step A và step B chứa đúng section của instance tương ứng, không lẫn.

### 11.4 File Artifact Instance Consumed By Step

- Mục tiêu: chứng minh type ngoài context (`file_artifact.v1`) chạy trên cùng framework.
- Cách test:
  1. Seed `file_artifact.v1`; tạo instance với `paths: [<workspace-file>]`.
  2. Bind as **OUTPUT** on producer step and **INPUT** on consumer step; run flow.
- Kỳ vọng (aligned BUG-276 / SD-23 `D-8` / Task-223–225):
  - **OUTPUT:** producer prompt lists required path(s) as write contract; existence may be gated (`r-artifact-output`). Optional **per-instance** structure (Task-225) for template + section gate when configured.
  - **INPUT:** consumer step receives **path mention only** and must read with tools; **no** full body/excerpt inject into the consumer prompt (unlike `context_artifact` package content).
  - Path ngoài workspace/symlink escape → omit or fail existence, not silent success.

### 11.5 Type-Mismatch And Unknown-Source Fail Fast

- Mục tiêu: chứng minh ranh giới type-compat + validation (`D-7`).
- Cách test:
  1. Bind một `file_artifact` instance vào slot mong đợi `context_artifact` → load flow.
  2. Tạo `context_artifact` instance với `sources: [totally.unknown]` → bind → load flow.
- Kỳ vọng:
  - Cả hai **fail-fast** lúc flow-load với lỗi nêu rõ step/slot/instance/type (hoặc source-id lạ), không silent no-op.
  - UI chặn bind sai type từ đầu (không hiện instance sai type trong picker).

### 11.6 Missing / Stale Binding Behavior

- Mục tiêu: chứng minh required vs optional binding degrade đúng (`F-1`).
- Cách test:
  1. Bind required instance rồi xóa instance đó khỏi DB (stale) → load flow.
  2. Bind optional instance rồi xóa → run flow.
  3. Thử xóa instance đang bound qua Settings UI.
- Kỳ vọng:
  - Required stale → lỗi flow-load rõ ràng.
  - Optional stale → warning + step vẫn hoàn tất.
  - UI chặn/guard xóa instance đang bound (Task-199 `T-4`).

### 11.7 BUG-236 And Persistence Boundary

- Mục tiêu: chứng minh binding không rò xuống `workflow_steps`.
- Cách test:
  1. Tạo/sửa step có artifact binding; save workflow.
  2. Kiểm `workflow_steps` rows + `step_artifact_bindings`.
- Kỳ vọng:
  - Binding chỉ nằm ở `step_artifact_bindings` (gắn `step_definition_id`); `workflow_steps` không có cột/field artifact (BUG-236).
  - Binding round-trip đúng qua reload.

### 11.8 Backward-Compat With CP-44 Flows

- Mục tiêu: chứng minh flow CP-44 cũ chạy nguyên qua fallback precedence (`D-6`).
- Cách test:
  1. Flow chỉ có default set (không binding, không `contexts.sources`) → run.
  2. Flow có flow-level `contexts.sources` (CP-44 `§11.2`) → run.
  3. Flow có transition step `context_sources` (Task-196) nhưng chưa migrate → run.
- Kỳ vọng:
  - Cả ba chạy như trước CP-45; không âm thầm rơi về default sai.
  - Ưu tiên: artifact binding → step `context_sources` → flow `contexts.sources` → default.

### 11.9 Suggested Automated Commands

- `go test ./apps/local-runner/internal/runner/... ./apps/local-runner/internal/agentpack/...`
- `go test ./apps/local-runner/internal/runner/... -run 'Test(ResolveEnabledContextSourceIDs|ResolveArtifactBoundContextSources|ArtifactTypeRegistry|DefaultArtifactTypeRegistry|FileArtifactResolver|ResolveInputArtifactPrompt|ValidateFlowArtifactBindings|FlowDefinitionResolver.*ArtifactBinding|SeedBuiltinContextArtifactBindings|EnsureBuiltinArtifactBindingsWithStore)'`
- `apps/desktop-flowpilot`: `npm run typecheck` (no automated test harness for `flowpilot-client-core`/`WorkflowsSettings.tsx` — pre-existing gap, not opened by CP-45).

## 12. Completion Notes (2026-07-09)

- result: `done` — all 8 child tasks (197–203, 205) landed in one session. `go test ./...` (local-runner, 13 packages): 1411 passed, 15 pre-existing unrelated failures (codex CLI/session-path/skills-merge — same count before and after this change), 0 regressions. `tsc --noEmit` (desktop-flowpilot) clean.
- known scope gaps (see individual task completion notes for detail):
  - Built-in artifact *instances* are seeded via an idempotent migration upsert, not a Go-side mirror-sync service (Task-198 `DOD-5`) — a deliberate simplification, not a gap in coverage.
  - Task-205's built-in flow proves cross-step binding *data* end to end, but live prompt injection is proven only at the one existing dispatch seam (`startInlineEntryChain`'s hop to `coder`); widening injection to every bound consumer node is a follow-up, not required by this CP's own DOD wording.
  - Full interactive UI verification (Task-199/200) needs a live local-runner + Supabase backend with these migrations applied — not available in this session's sandbox; verified via `tsc --noEmit` + a clean dev-server boot instead.
- upstream docs updated: SD-23 (unchanged — implementation matched design as written, including the output/input direction fix caught during implementation, which was already correct in SD-23's own prose); CP-44/Task-195/Task-196 (updated earlier this session when Task-198 was renumbered to Task-204).

## 13. Live E2E ledger (gate-sandbox, 2026-07-11 → 2026-07-12)

Target: `/Users/tiendat/Desktop/BE/gate-sandbox`.

### 13.1 Wave status

| Wave | Status | Notes |
|------|--------|-------|
| **A1–A9** Artifact UI authoring | **PASS** | Types, instances, type-filter pickers; legacy Step Context Sources hidden (Task-222) |
| **B** Bind flows | **PASS** | Binding OK; edges-only Save dirty fixed (BUG-274) |
| **C** Fail-fast | **PASS** | Unknown source + type filter confirmed |
| **D** Context package | **PASS** | Coder receives Flow Context Package (`calc-core` verified); history inside package |
| **E** File write→read | **PASS** | OUTPUT write contract + gate (Task-223); INPUT path-only (BUG-276); What/Why/Baseline template (Task-224) |
| **Hub synthesis** | **PASS** | Join note once; no feature self-resolve on flow-engine (BUG-275); no full Prior work dump on synthesis (Task-224) |
| **Reviewer prompt scoping** | **PASS** | No ledger inject + no full coder final body when file INPUT (BUG-277 / Task-224); live `run-1024` |

### 13.2 Follow-up tasks / bugs closed this verification

| ID | Status | CA |
|----|--------|-----|
| Task-222 Artifact-only Step UX | **done** | CA-286 |
| Task-223 File artifact write→read chain | **done** | CA-287 |
| BUG-274 Edge dirty snapshot | **done** | CA-288 |
| BUG-275 Hub join dedupe + feature resolve | **done** | CA-288 |
| BUG-276 File INPUT path-only | **done** | CA-288 |
| Task-224 Prompt scoping + Why template | **done** | CA-289 |
| BUG-277 Ledger over-inject on review | **done** | CA-289 |

### 13.3 Residual / not CP-45 blockers

| Item | Status | Notes |
|------|--------|-------|
| Chat_summary ledger **noise** in package Prior discussion | **Open (sandbox data)** | Old E2E agent text in gate-sandbox `chat_summary.ndjson`; not an inject bug |
| Per-instance file OUTPUT structure (+ optional section gate) | **Done (Task-225 / CA-290)** | Phase-2; structure-only config; paths-only = existence |
| Optional `prompt-index.jsonl` | **Deferred** | P4 discoverability only; `last-prompt` overwrite expected |
| SD-23 file_artifact INPUT/OUTPUT semantics | **Aligned 2026-07-12** | Path mention + tools for INPUT; write contract for OUTPUT (BUG-276 / Task-223) |

### 13.4 Live evidence

- Pre-Task-224: `run-41047` hub, `run-41052` coder, `run-41359` reviewer (and later `run-309`/`314`/`658`).
- Post-Task-224: `run-723` coder, `run-1024` reviewer, `run-718` hub — package + Why template; path-first review; no Prior work on review/hub synthesis.

### 13.5 Closeout

**CP-45 is done** — implementation (Task-197–205), live E2E (A–E + residuals Task-222–224 / BUG-274–277), and authority docs (this §13 + SD-23 D-8 alignment). Follow-up: [Task-225](../../08-Task/done/Task-225-File-Artifact-Instance-Output-Structure.md) for per-instance file OUTPUT structure (template + optional section gate; default paths-only = existence only).
