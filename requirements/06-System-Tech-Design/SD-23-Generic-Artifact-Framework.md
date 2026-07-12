# SD-23: Generic Artifact Framework (Types, Instances, Step Bindings)

## Metadata

- Document ID: `SD-23`
- Title: `Generic Artifact Framework (Types, Instances, Step Bindings)`
- Phase: `tech_design`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-09`
- Last Updated: `2026-07-12`
- Parent Documents: [SS-13: AI-Followable Document Contract](../05-System-Specs/SS-13-AI-Followable-Document-Contract.md), [SS-14: Code Context And Regression Safety](../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md) (**US-10**, **AC-17** — typed artifacts as first-class flow I/O; also US-9, AC-16, US-1, US-2, US-5, US-7, AC-9, AC-13, AC-15)
- Child Documents: [CP-45: Generic Artifact Types And User-Scoped Artifact Instances](../07-Coding-Plan/done/CP-45-Generic-Artifact-Types-And-Instances.md)
- Related Documents: [SD-22: Pluggable Context Source Registry](./SD-22-Pluggable-Context-Source-Registry.md) (substrate — `context_artifact.v1` producer tái dùng `ContextSourceRegistry`), [SD-17: Context And Regression Engine](./SD-17-Context-And-Regression-Engine.md) (D-4 context package), [SD-21: Change Contract And Canonical Intent Signature](./SD-21-Change-Contract-And-Canonical-Intent-Signature.md), [CP-44: Pluggable Context Source Registry](../07-Coding-Plan/done/CP-44-Pluggable-Context-Source-Registry.md) (vertical slice đầu tiên), [CP-42: Flow Pack And Generic Node Behavior Refactor](../07-Coding-Plan/done/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md) (mẫu registry gốc), [BUG-236: Builtin Flow Mirror Stores Node Definition On Workflow Steps Instead Of Step Definitions](../09-BugFix/done/BUG-236-Builtin-Flow-Mirror-Stores-Node-Definition-On-Workflow-Steps-Instead-Of-Step-Definitions.md), [BUG-276: File Artifact Input Path-Only](../09-BugFix/done/BUG-276-File-Artifact-Input-Should-Mention-Paths-Not-Paste-Content.md), [Task-223: File Artifact Output Contract](../08-Task/done/Task-223-File-Artifact-Output-Contract-And-Review-Input-Chain.md), [Task-224: Flow Prompt Scoping](../08-Task/done/Task-224-Flow-Prompt-Scoping-And-Coder-Output-Why-Template.md), [Task-225: File Artifact Instance Output Structure](../08-Task/done/Task-225-File-Artifact-Instance-Output-Structure.md)
- Replaces: `None`
- Tags: `artifact, artifact-type, artifact-instance, step-binding, flow-mode, typed-contract, supabase, extensibility`

## AI Quick View

### Summary

- SD-23 là design-of-record cho một **artifact framework tổng quát** ở flow engine: node output/input các **artifact có type rõ ràng**, thay vì mỗi loại grounding (context, file, review...) là một nhánh code riêng.
- Tách rõ **3 lớp**: `ArtifactType` (contract hệ thống sở hữu, **code chỉ hardcode lớp này**), `ArtifactInstance` (object cụ thể có `name` + `config_json`), `StepArtifactBinding` (buộc artifact vào step). Một step attach một **danh sách artifact** cho cả **input** lẫn **output** (nhiều artifact mỗi chiều).
- **Instance có 2 nguồn gốc**: (a) **built-in instance** — seeded/force-migrate lên Supabase giống cơ chế built-in Flow (`is_builtin=true`, RLS chặn user sửa, hiện read-only với tag "built-in", tên do hệ thống đặt trước); (b) **user instance** — user tạo trên trang Artifacts từ một built-in type, tự đặt `name`. Ví dụ: built-in `context_artifact` default phải đủ mọi source; user có thể tạo instance khác chỉ dùng `mcp.driver`.
- Artifact là **I/O giữa các step**: step A khai một artifact là **output**, step B bind chính artifact instance đó làm **input** → tạo luồng dữ liệu chéo-step (Context produce → Coder/Review consume).
- Đây là **tổng quát hóa của SD-22**: SD-22 mới cho *context source* cắm-được ở Plan step; SD-23 nâng thành "typed artifact" dùng chung. `context_artifact.v1` là **vertical slice đầu tiên**, producer **tái dùng nguyên `ContextSourceRegistry`** của SD-22/CP-44 — SD-23 **compose lên trên** SD-22, không thay thế.
- Khác SD-22 ở persistence: SD-23 **cần Supabase tables thật** (`artifact_types`, `artifact_instances`, `step_artifact_bindings`) vì instance là state (built-in seeded + user-authored) được bind lại nhiều step; không còn đủ với pack YAML + một cột đơn.
- v1 giữ scope hẹp: **chỉ system-owned type** (không mở custom-type DSL), user chỉ author **instance**; giữ bất biến deterministic/no-vector của CP-41/SD-17; giữ contract BUG-236 (binding ở `step_definitions`, không phải `workflow_steps`).

### Current Ask

- **Implemented via CP-45** (live-verified 2026-07-12). Design remains authority for type/instance/binding; consumer prompt rules for `file_artifact` refined by BUG-276 / Task-223 / Task-224 (see `D-8` amendment below).

### Key Decisions

- `D-1` Artifact framework gồm **3 lớp tách biệt**: `ArtifactType` (contract, system-owned), `ArtifactInstance` (object có `name` + `config_json`, project-scoped), `StepArtifactBinding` (binding step↔instance). Một step attach **danh sách** binding cho **cả input lẫn output** (nhiều artifact mỗi `direction`); binding cùng một instance vào output của step A và input của step B tạo **luồng I/O chéo-step**. Đây là tổng quát hóa `ContextSource`/`ContextSourceRegistry` của SD-22 `D-1`.
- `D-2` v1 **chỉ built-in `ArtifactType`** (code chỉ hardcode type); registry **từ chối** type user tự định nghĩa (không mở DSL). Nhưng **instance có cả built-in lẫn user**: built-in instance được **seeded/force-migrate** lên Supabase (`is_builtin=true`) giống built-in Flow — read-only, RLS chặn user ghi, hiện cùng màn Flow settings với tag "built-in"; user chỉ tạo/sửa/xóa instance `is_builtin=false`. Đối xứng cơ chế built-in workflow (`EnsureBuiltinFlowMirrorsWithStore` + RLS `is_builtin=false`).
- `D-3` **Config theo instance, không theo type**: `ArtifactInstance.config_json` giữ producer config (`sources`, `paths`, filters, options...), validate theo `ArtifactType.configSchema`. `ArtifactType` **không** chứa config cụ thể. Cùng một type có **nhiều instance** cấu hình khác nhau — ví dụ built-in `context_artifact` default = **đủ mọi source**, còn user tạo instance chỉ có `sources: [mcp.driver]`. Instance có `name` (built-in: hệ thống đặt sẵn; user: tự đặt).
- `D-4` **Cần Supabase tables thật** (khác SD-22 `§Constraints` "không thêm bảng"): `artifact_types` (seeded/system-owned), `artifact_instances` (project-scoped), `step_artifact_bindings`. Binding gắn ở **step-definition layer** (`step_definition_id`), **không** ghi xuống `workflow_steps` (giữ BUG-236). Dùng **bảng riêng** cho binding, không nhồi JSON, để giữ `direction`/`slot_name`/`required`/`position` truy vấn được.
- `D-5` **`context_artifact.v1` là vertical slice đầu tiên** và producer của nó **tái dùng `ContextSourceRegistry`** (SD-22): `config_json.sources` = danh sách context-source-id đã đăng ký. Artifact framework **compose lên** SD-22; không viết lại thu-thập-context.
- `D-6` **Resolver precedence có fallback**, migrate mềm không hard-cut: step **artifact binding** → (transition) step `context_sources` (CP-44/Task-196) → flow-level `contexts.sources` (SD-22 `D-4`) → default set. Flow chưa có binding vẫn chạy nguyên đường CP-44.
- `D-7` **Type-compatibility là ranh giới cứng**: một slot chỉ bind được instance có `artifact_type` khớp type mong đợi của slot + đúng `direction`. Bind sai type → **fail-fast** ở authoring (UI) và ở flow-load (runner), không silent.
- `D-8` **Bắt buộc ≥1 type ngoài context** (`file_artifact.v1`, config `{ paths: string[] }`) để chứng minh framework **không hardcode cho context**. **Direction semantics (amended 2026-07-12, BUG-276 / Task-223 / Task-224):**
  - **OUTPUT** binding: **write contract** — agent must create/update the designated path(s); flow-gate may enforce existence (`r-artifact-output`). Optional **per-instance** `structure` on `config_json` (section title list only — no separate `format` field) drives prompt template + structural gate when set (Task-225); paths-only instances get existence only — do not force coding What/Why/Baseline on every file artifact.
  - **INPUT** binding: **path reference only** — prompt **mentions** workspace-relative path(s) and instructs the agent to open them with tools. **Do not paste full file body/excerpts** into the consumer prompt (unlike `context_artifact` / Flow Context Package, which must push content because it has no durable path handle).
  - Workspace-safety for existence/path resolution reuses `source.excerpt` protections (SD-22 `F-6`) where paths are validated; outside-workspace / symlink escape → omit or treat as missing, not silent success.
- `D-9` Bất biến **deterministic/no-vector** của CP-41/SD-17 `D-3` áp cho **mọi** artifact producer (không riêng context): producer là explicit lookup theo config, không similarity search; degrade-mềm khi backing unavailable (`AC-9`/`AC-13`).
- `D-10` **Ship một built-in flow chuẩn** làm nơi wiring artifact chain sẵn (giống built-in review-loop): `Context → Coding → Review → Synthesis`, trong đó `context_artifact` là output của Context step và input của các step sau. User tạo flow riêng sau này **tự quyết** có dùng Context/artifact nào. Đây cũng là E2E vehicle chứng minh cross-step I/O. Triển khai: [Task-205](../08-Task/done/Task-205-Builtin-Artifact-Flow-Context-Coding-Review-Synthesis.md).
- `D-11` **Dispatch seam** cho producer/consumer: thêm một `ArtifactTypeRegistry` nhỏ (mirror `ContextSourceRegistry`) với per-type helpers ở **prompt-assembly seam** — **không** thêm behavior node mới. `context_artifact` → `ContextSourceRegistry.Collect` (full package content). `file_artifact` **INPUT** → path-list mention only (BUG-276); **OUTPUT** → required-path write contract + template guidance (Task-223/224). Optional registry resolver that reads excerpts may exist for tooling/tests but **must not** be the default consumer prompt path for INPUT.

### Constraints

- Không phá runner path hiện có của CP-44/Task-168/169/171; framework mới phải có đường migrate từ `flow_context_package.v1` và giữ `PackageID` behavior.
- Không mở custom `ArtifactType` DSL ở v1 (nếu mở sớm → thành plugin platform ngoài scope).
- Không cho step binding ghi config riêng xuống `workflow_steps`; metadata node/step vẫn ở `step_definitions` hoặc bảng liên kết trỏ về `step_definitions` (BUG-236).
- Phải hỗ trợ **nhiều instance cùng một type** trong một flow/project.
- Kế thừa ranh giới bảo mật SD-22 `D-5`: nguồn ngoài chỉ tới MCP đã support/connect + path workspace-safe; không lệnh/đường dẫn tùy ý.
- Local-first + Drive sync theo cơ chế SD-17 `§5.1`; artifact state là project-scoped Supabase, không tạo store song song cho package.

### Open Questions

- `Q-1` **(RESOLVED → `SS-14 US-10`/`AC-17` added, 2026-07-09)** SS-authority cho artifact framework tổng quát đã có: `US-10` ("typed artifacts as first-class flow I/O") + `AC-17` (3 lớp type/instance/binding, type-compat fail-fast vs degrade, deterministic, ≥1 type ngoài context, compatibility fallback). SD-23 giờ trace thẳng về US-10/AC-17, không còn nợ spec-authority.
- `Q-2` **(RESOLVED, 2026-07-09)** `config_json` validate **app-level** theo `ArtifactType.configSchema`; không ràng buộc ở Supabase.
- `Q-3` **(RESOLVED, 2026-07-09)** Instance **project-scoped** v1; shared/library scope để sau.
- `Q-4` **(RESOLVED, 2026-07-09)** Output slot bind một **instance cấu hình sẵn** (built-in seeded hoặc user-created), không tạo ephemeral lúc run. Instance là handle chia sẻ giữa output của producer step và input của consumer step (`D-1`).
- `Q-5` **(RESOLVED, 2026-07-09)** Type catalog + built-in instance **seed qua migration/mirror-sync** (service role, `is_builtin=true`) là source-of-truth, đối xứng built-in Flow; startup chỉ verify khớp.
- `Q-6` **(RESOLVED, 2026-07-09)** UI: trang Artifacts là **tab thứ 3** trong Flow settings (cạnh `workflows`/`steps`), không phải nav item riêng (CP-45/Task-199).

### Source Refs

- `SS-14` US-10, AC-17 (typed artifacts); US-9, AC-16 (context extensibility); US-1, US-2, US-5, US-7; AC-9, AC-13, AC-15.
- `SD-22` `D-1` (registry pattern), `D-2` (deterministic-only), `D-3` (typed sections + projection), `D-4` (per-flow declared binding), `D-5` (external boundary + degrade).
- `SD-17` `D-3` (deterministic retrieval), `D-4` (feature resolution), `§5.1` (local + Drive sync).
- `CP-44` `P-3/P-4/P-7`, `DOD-9` — context artifact + flow/step binding (hẹp ở context).
- `BUG-236` — binding phải ở `step_definitions`, không `workflow_steps`.
- current code: `agentpack/pack.go` (`ContextArtifact`, `FlowContextBinding.Ref`), `flow_context_package.go`, `context_source_registry.go`, `context_sources_builtin.go`, `behavior_registry_builtin.go` (`behaviorContextProduce`), `flow_definition_resolver.go`, `WorkflowsSettings.tsx`, `adminModels.ts`, `supabaseAdminRepository.ts`.

## 1. Goal

Tổng quát hóa mô hình "typed context package" (SD-22) thành một **artifact framework chung** cho flow engine: node A output một `ArtifactInstance` có `ArtifactType` rõ ràng, node B bind instance đó làm input, và UI/DB cho phép user author/manage các instance dựa trên các type hệ thống đã định nghĩa sẵn — mà **không** phá contract package hiện tại, **không** mở custom-type DSL, và giữ bất biến deterministic/no-vector.

## 2. Input Documents

- `SS-14` (US-9, AC-16; US-1/2/5/7, AC-9/13/15) — spec context & regression safety + extensibility.
- `SS-13` — document/context contract.
- `SD-22` (`D-1`..`D-5`) — context-source registry mà SD-23 tổng quát hóa.
- `SD-17` (`D-3`, `D-4`, `§5.1`) — context engine nền.
- `CP-44` (`P-3/P-4/P-7`, `DOD-9`) — vertical slice context artifact hiện có.
- `BUG-236` — ranh giới step-definition vs workflow-step.
- current code: `flow_context_package.go`, `context_source_registry.go`, `flow_definition_resolver.go`, `supabaseAdminRepository.ts`, `adminModels.ts`, `WorkflowsSettings.tsx`.

## 3. Architecture Decision

### 3.1 `D-1` Three-layer artifact model

- `D-1` Thay vì mỗi loại grounding là một nhánh code, framework có 3 lớp:
  - **`ArtifactType`** — contract system-owned: `id` (versioned, vd `context_artifact.v1`), `category`, `producerBehavior`, `consumerHints`, `configSchema`, `renderTemplate`, `systemOwned`, `status`.
  - **`ArtifactInstance`** — object do user tạo: `id`, `project_id`, `artifact_type_id`, `name`, `description`, `config_json`, `status`.
  - **`StepArtifactBinding`** — buộc slot: `step_definition_id`, `direction` (`input`|`output`), `slot_name`, `artifact_instance_id`, `required`, `position`.
- Alternatives considered:
  - Tiếp tục thêm registry riêng cho mỗi loại (context registry, file registry, ...) → nhân bản pattern, không có typed I/O chung giữa node.
  - Một plugin/DSL cho type do user định nghĩa → over-scope thành platform, khó giữ deterministic + khó review (xem `R-1`).
- Why: đối xứng `BehaviorRegistry` (CP-42) và `ContextSourceRegistry` (SD-22) đã chứng minh; tách type/instance/binding cho phép reuse instance qua nhiều step và giữ config ra khỏi step form.

### 3.2 `D-2` System-owned types only (v1)

- `D-2` `ArtifactType` là built-in; catalog seeded, registry từ chối type user-defined. User authoring bắt đầu ở instance.
- Why: giữ v1 nhỏ, an toàn review; mirror SD-22 "Go sở hữu implementation, pack/config chỉ chọn ID". Custom type là future scope sau khi có SS-authority (`Q-1`).

### 3.3 `D-3` Config lives on instance, validated by type

- `D-3` `ArtifactInstance.config_json` giữ config cụ thể; validate theo `ArtifactType.configSchema` (app-level, `Q-2`). Type không giữ config instance.
- Why: cùng một type có nhiều instance khác cấu hình (`main_context` vs `review_context`); tránh copy config vào từng step.

### 3.4 `D-4` Real Supabase persistence at step-definition layer

- `D-4` Thêm 3 bảng: `artifact_types`, `artifact_instances`, `step_artifact_bindings`. Binding gắn `step_definition_id`; **không** ghi xuống `workflow_steps` (BUG-236). Dùng bảng riêng cho binding (không JSON blob) để `direction`/`slot_name`/`required`/`position` truy vấn/validate được.
- Alternatives: 2 cột JSON (`input_artifact_instance_ids`/`output_artifact_instance_ids`) trên `step_definitions` → mất `direction`/`slot`/`required`/`position` có cấu trúc (CP-45 `Q-2`).
- Why: instance là durable user state được nhiều step tham chiếu; pack YAML/1-cột (đủ cho SD-22) không đủ. Bảng riêng giữ contract BUG-236 và cho validation type-compat.

### 3.5 `D-5` `context_artifact.v1` composes over SD-22

- `D-5` Type built-in đầu tiên `context_artifact.v1`; producer **tái dùng `ContextSourceRegistry`** của SD-22. `config_json.sources` = danh sách context-source-id. Không viết lại thu-thập-context.
- Why: SD-22 đã ship và test byte-identical; SD-23 chỉ đổi *nơi khai báo* selection (từ pack/1-cột → instance), không đổi *cách build* package. Giữ `PackageID` và no-vector guard.

### 3.6 `D-6` Resolver precedence with soft migration

- `D-6` `behaviorContextProduce`/resolver chọn source theo thứ tự: **artifact binding** (`context_artifact` instance) → step `context_sources` (transition, CP-44/Task-196) → flow-level `contexts.sources` (SD-22 `D-4`) → default built-in set.
- Why: migrate dần, flow cũ không vỡ; đóng CP-44 fallback lại chỉ khi adoption đủ (Task-203).

### 3.7 `D-7` Type-compatibility as a hard boundary

- `D-7` Slot khai báo type mong đợi + `direction`; UI chỉ hiện instance khớp type; runner fail-fast khi binding trỏ instance sai type / mất instance.
- Why: ngăn bind nhầm (vd `file_artifact` vào slot context); fail rõ lúc authoring/load thay vì lỗi mờ trong provider turn (Task-203 `T-2`).

### 3.8 `D-8` Non-context type proves generality

- `D-8` `file_artifact.v1` (`config { paths: string[] }`) là type thứ hai.
- **OUTPUT:** write contract on designated paths (existence gate always; optional per-instance structure template/gate = Task-225).
- **INPUT:** path mention + tools-read only — **not** full-body excerpt inject into the consumer prompt (BUG-276). Contrast: `context_artifact` still pushes package content.
- Workspace-safe path checks reuse SD-22 `source.excerpt` / `F-6` style protections where applicable.
- Why: DOD của CP-45 yêu cầu framework không âm thầm chỉ phục vụ context; live product further separates path-based I/O from content-package I/O.

### 3.9 `D-9` Determinism/no-vector across all producers

- `D-9` Mọi producer explicit-lookup theo config; không similarity search; degrade-mềm khi backing unavailable. Guard no-vector giữ nguyên từ CP-41.

### 3.10 `D-10` Built-in standard flow as wiring vehicle

- `D-10` Ship built-in flow `Context → Coding → Review → Synthesis` (giống built-in review-loop): `context_artifact` là output của Context step, input của Coding/Review/Synthesis. User flow tự tạo sau này tự quyết có dùng Context không.
- Why: cho user một flow chuẩn chạy ngay + là E2E vehicle chứng minh cross-step artifact I/O; không ép migrate mọi flow cũ.

### 3.11 `D-11` Artifact type registry + built-in instance mirror

- `D-11` **Producer/consumer dispatch**: `ArtifactTypeRegistry` / prompt-assembly helpers — không thêm behavior node. `context_artifact` → package content via `ContextSourceRegistry.Collect`; `file_artifact` INPUT → path mentions only; OUTPUT → write-contract path list (+ template).
- `D-11b` **Built-in instance seeding**: mirror cơ chế built-in Flow — `is_builtin=true` rows seeded qua service-role mirror-sync (đối xứng `EnsureBuiltinFlowMirrorsWithStore`), RLS chặn `authenticated` ghi row `is_builtin=true`. Không hardcode instance trong Go runtime path; chỉ hardcode **type**.
- Why: giữ executor bất biến, tái dùng đúng pattern registry + built-in-mirror đã chứng minh ở CP-42/built-in workflows.

## 4. Component Impact

- Impacted modules:
  - runner artifact resolution (`flow_definition_resolver.go`, `behavior_registry_builtin.go`, `flow_context_package.go`).
  - runner built-in mirror-sync (`supabase_workflow_flow_store.go` — thêm built-in artifact instance mirror cạnh `EnsureBuiltinFlowMirrorsWithStore`).
  - client-core admin domain/repo (`adminModels.ts`, `supabaseAdminRepository.ts`).
  - desktop Settings UI (`WorkflowsSettings.tsx` — thêm **tab thứ 3 `artifacts`** cạnh `workflows`/`steps`; `type Tab = "workflows" | "steps" | "artifacts"`).
- New modules:
  - `ArtifactTypeRegistry` + per-type resolver (`context_artifact`, `file_artifact`) ở runner (`D-11`).
  - artifact type catalog + instance domain model (client-core).
  - artifact tab component(s) (built-in read-only + user CRUD).
  - runner artifact-binding resolver structs.
- Reused unchanged / composed:
  - `ContextSourceRegistry` + built-in sources (SD-22) as `context_artifact.v1` resolver backing.
  - Workspace-safe path validation patterns from `source.excerpt` / `readSourceExcerpts` (existence/outside-workspace), without using full excerpt dump as the default INPUT prompt contract.
  - built-in mirror pattern (`is_builtin` + RLS `is_builtin=false` + service-role sync) từ built-in workflows.
  - `WorkflowStore` persistence for package; `PackageID` formula.
- New DB surface: `artifact_types`, `artifact_instances` (có `is_builtin`), `step_artifact_bindings` (project-scoped, Supabase; local-first + Drive sync per SD-17 `§5.1`).

## 5. Data Model

- `ArtifactType`: `id` (pk, versioned string), `version`, `category`, `producer_behavior`, `consumer_hints`, `config_schema` (json), `render_template`, `system_owned` (bool, always true v1), `status`. **Chỉ lớp này hardcode trong code.**
- `ArtifactInstance`: `id` (pk), `project_id` (fk), `artifact_type_id` (fk → `artifact_types`), `name`, `description`, `config_json`, `is_builtin` (bool — built-in seeded read-only vs user-created), `status`, timestamps.
- `StepArtifactBinding`: `id` (pk), `step_definition_id` (fk → `step_definitions`), `direction` (`input`|`output`), `slot_name`, `artifact_instance_id` (fk → `artifact_instances`), `required` (bool), `position` (int). Một step có **nhiều** binding mỗi `direction` (danh sách).
- Runner-side resolver structs carry bindings alongside resolved step definition (Task-198 `T-4`).
- Invariants:
  - `artifact_instances.artifact_type_id` phải trỏ type tồn tại trong catalog.
  - binding phải trỏ instance cùng project + type khớp slot (`D-7`).
  - RLS: `authenticated` chỉ insert/update/delete `artifact_instances` có `is_builtin=false` (mirror workflows RLS); built-in seeded qua service role.
  - không có artifact config trên `workflow_steps` (BUG-236).

## 6. Interfaces and Contracts

- Type catalog: read-only ở UI; seeded qua migration (`Q-5`).
- Built-in instances: seeded/mirror-synced (`is_builtin=true`), hiện read-only với tag "built-in" (đối xứng built-in workflows `editable===false`); user không sửa/xóa.
- Instance CRUD (user, `is_builtin=false`): client-core repo methods (list/create/update/delete) qua `supabaseAdminRepository.ts`; delete instance đang bound → chặn/guard (Task-199 `T-4`).
- Binding: round-trip qua `step_artifact_bindings` (danh sách per direction); resolver load kèm step definition.
- `context_artifact.v1` resolver contract: nhận `config_json.sources` → gọi `registry.Collect(enabledIDs, hints)` (SD-22 `§5`) → `FlowContextPackage` (Sections + projection, `PackageID` không đổi).
- `file_artifact.v1` prompt contract:
  - **OUTPUT:** list required path(s); agent must write them; gate may enforce existence; if instance declares `structure.sections`, prompt + gate use those titles only (Task-225) — not a global coding memo for all file outputs.
  - **INPUT:** list path(s) + “read with tools”; **do not** paste file body into the prompt (BUG-276).
  - outside-workspace / invalid paths → omit or fail existence checks, not silent success.
- Fallback contract (`D-6`): resolver deterministic theo precedence; unknown source-id vẫn fail-fast qua registry (SD-22 `F-2`).

## 7. Execution Flow

1. **Seed (migration/mirror-sync)**: `artifact_types` + built-in `artifact_instances` (`is_builtin=true`, tên hệ thống đặt, vd `context_artifact` default đủ source) được seed qua service role; built-in flow `Context→Coding→Review→Synthesis` mirror sẵn binding.
2. **Authoring (Settings → tab Artifacts)**: user chọn built-in `ArtifactType` (catalog read-only) → nhập `name` + `config_json` hợp lệ theo `configSchema` → save user instance (`is_builtin=false`, project-scoped). Built-in instance hiện read-only.
3. **Binding (step editor)**: user attach **danh sách** artifact cho input/output của step; UI lọc theo type-compat (`D-7`) → save `step_artifact_bindings` (gắn `step_definition_id`).
4. **Flow load (runner)**: resolve step definitions kèm artifact bindings; validate type-compat + instance tồn tại → fail-fast nếu sai/mất (required) hoặc warn (optional).
5. **Produce/consume (cross-step)**: producer step binds artifact as **OUTPUT** (`context_artifact` → build package content; `file_artifact` → write-contract paths). Consumer binds same instance as **INPUT** (`context_artifact` package content already assembled on the context hop; `file_artifact` → path mentions only in prompt). Keep no-vector invariant for context packages.

## 8. Failure and Edge Handling

- `F-1` Binding trỏ instance đã bị xóa → required: **lỗi flow-load** rõ ràng; optional: warning + degrade.
- `F-2` Binding sai type (instance type ≠ slot type) → **fail-fast** lúc authoring và flow-load (`D-7`).
- `F-3` `config_json` không hợp lệ theo `configSchema` (vd UI cũ lưu payload lạ) → validate reject lúc save; nếu lọt xuống DB → resolver báo lỗi cấu hình, không chạy mù.
- `F-4` Flow chưa có artifact binding → chạy nguyên đường CP-44 (`D-6` fallback), không âm thầm rơi về default sai.
- `F-5` `context_artifact` với source-id lạ trong `config_json.sources` → fail-fast qua registry (kế thừa SD-22 `F-2`).
- `F-6` `file_artifact` path ngoài workspace/symlink escape → omitted với lý do (`outside_workspace`, ...), giữ SD-22 `F-6`.
- `F-7` Producer backing (MCP) timeout/down → section rỗng + warning; step vẫn hoàn tất (`AC-9`, SD-22 `F-3`).
- `F-8` Xóa instance đang bound → chặn ở UI; nếu xóa qua đường khác → binding stale rơi vào `F-1`.
- `F-9` User cố sửa/xóa built-in instance (`is_builtin=true`) → chặn ở UI (read-only) **và** ở RLS (chỉ `is_builtin=false` mới ghi được), đối xứng built-in workflow.

## 9. Security and Operational Concerns

- auth/boundary: nguồn ngoài chỉ MCP đã support/connect + path workspace-safe (kế thừa SD-22 `D-5`); type/instance/binding là project-scoped, không cross-project.
- secrets: producer không log payload thô vào event; chỉ metadata + `SourceRef` (Task-170/168 bounded-logging).
- audit: package/artifact state gắn `workflow_run_id` + `workflow_step_run_id`; giữ dòng `No vector retrieval used`.
- data: `artifact_types` seeded/system-owned (read-only cho user); built-in `artifact_instances` (`is_builtin=true`) chỉ ghi được qua service-role mirror-sync, `authenticated` bị RLS chặn (đối xứng built-in workflows RLS); user instance + `step_artifact_bindings` project-scoped, local-first + Drive sync (SD-17 `§5.1`).
- rollback: bỏ binding → về precedence thấp hơn (`D-6`); disable một type → instance của type đó ẩn/khóa, flow degrade thay vì vỡ.

## 10. Risks and Trade-Offs

- `R-1` Scope explosion → trượt thành plugin/platform. Mitigation: v1 chỉ built-in type + user-created instance (`D-2`); custom type chờ SS-authority (`Q-1`).
- `R-2` Overlap/conflict với SD-22/CP-44 `Task-196`. Mitigation: xem `contextSources[]` là transition layer; artifact binding supersede sau khi sẵn sàng (`D-6`, Task-201).
- `R-3` Schema complexity nếu binding nhét vào JSON cột. Mitigation: bảng `step_artifact_bindings` riêng (`D-4`).
- `R-4` Consumer semantics chưa rõ cho type mới. Mitigation: mỗi built-in type có producer/consumer hints + render contract rõ trước khi expose (`D-1`).
- `R-5` User nhầm type vs instance. Mitigation: UI tách 2 bề mặt — Types = read-only catalog, Instances = thứ user tạo/bind (Task-199 `T-2`).
- `R-6` Migrate phá flow cũ. Mitigation: fallback precedence `D-6` + không hard-cut; CP-44 path còn test (Task-203).
- `R-7` Xói mòn determinism nếu producer mới lén similarity search. Mitigation: `D-9` guard, kế thừa registry reject non-deterministic của SD-22 `D-2`.

## 11. Validation Strategy

- unit:
  - type catalog seed/load; reject user-owned type creation; `context_artifact` type biểu diễn được mà không lưu instance sources trên type.
  - instance CRUD round-trip; binding round-trip; save không ghi artifact config vào `workflow_steps`.
  - resolver precedence (`D-6`); type-compat filter/validate (`D-7`); config-schema validate (`F-3`).
- integration:
  - step bound `context_artifact` build package từ instance `sources`; flow chỉ có CP-44 `contexts.sources` vẫn chạy; step không binding về default; unknown source fail-fast.
  - `file_artifact` instance đọc bounded workspace-safe; outside-workspace omitted.
- backward-compat:
  - old CP-44 default + flow-level binding vẫn chạy; missing/mismatched required binding fail rõ; optional missing degrade.
- built-in instance:
  - built-in instance seeded (`is_builtin=true`) xuất hiện read-only ở tab Artifacts; user không sửa/xóa (UI + RLS); mirror-sync idempotent (đối xứng built-in workflow tests).
- manual/E2E:
  - built-in flow `Context→Coding→Review→Synthesis` chạy: `context_artifact` (output của Context) được Coding/Review/Synthesis consume làm input.
  - tạo 2 instance cùng type khác config (default-đủ-source vs mcp-only), bind 2 step khác nhau, run flow; `file_artifact` OUTPUT on step A + INPUT on step B → step B prompt **mentions path only** (agent reads with tools; no full body paste).
- observability: count instance theo type/`is_builtin`; count binding theo direction/type; runner warning cho binding thiếu/mismatch.

## 12. Traceability to Spec

- `US-10` (typed artifacts as first-class flow I/O — nhiều loại grounding, reusable, bind theo type) → `D-1` three-layer model + `D-3` instance config + `D-8` non-context type.
- `AC-17` (3 lớp type/instance/binding; type-compat fail-fast vs degrade; deterministic; ≥1 type ngoài context; compatibility fallback) → `D-1`/`D-2`/`D-3`/`D-7`/`D-8`/`D-9` + `D-6` fallback + `F-1`/`F-2`/`F-3`.
- `US-9` (thêm loại context/grounding mới không đổi cấu trúc package) → `D-1` three-layer model + `D-5` context artifact + `D-8` file artifact.
- `AC-16` (declarative, deterministic, degrade-mềm, nguồn ngoài chỉ integration đã support) → `D-2`/`D-3`/`D-6`/`D-9` + `F-5`/`F-6`/`F-7`.
- `AC-9` (non-fatal, retryable, never blocks) → `D-9` degrade-mềm; producer lỗi không fail step (`F-7`).
- `AC-13` (tooling degrade tier khi thiếu) → producer MCP/backing degrade (`F-7`).
- `AC-15` (local + Drive sync) → artifact tables project-scoped, sync theo SD-17 `§5.1`.
- `US-1`/`US-2`/`US-5`/`US-7` → grounding giàu hơn, typed, reuse qua nhiều step; mở rộng sang nguồn/loại mới không refactor.
- `SS-13` change-ledger/document contract → binding/instance là state có thể audit; không phá contract package.
