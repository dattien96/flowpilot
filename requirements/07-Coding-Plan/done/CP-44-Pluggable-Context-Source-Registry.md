# CP-44: Pluggable Context Source Registry (Dynamic Context Types)

## Metadata

- Document ID: `CP-44`
- Title: `Pluggable Context Source Registry (Dynamic Context Types)`
- Phase: `coding_plan`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-08`
- Last Updated: `2026-07-09`
- Parent Documents: [SD-22: Pluggable Context Source Registry](../../06-System-Tech-Design/SD-22-Pluggable-Context-Source-Registry.md), [SD-17: Context And Regression Engine](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md), [SS-13: AI-Followable Document Contract](../../05-System-Specs/SS-13-AI-Followable-Document-Contract.md)
- Child Documents: [Task-191: Context Source Interface And Registry](../../08-Task/done/Task-191-Context-Source-Interface-And-Registry.md) (P-1, done), [Task-192: Migrate Built-in Context Sources](../../08-Task/done/Task-192-Migrate-Builtin-Context-Sources.md) (P-2, done), [Task-193: Context Package Sections And Compatibility Projection](../../08-Task/done/Task-193-Context-Package-Sections-And-Compat-Projection.md) (P-3, done), [Task-194: Per-Flow Context Source Binding](../../08-Task/done/Task-194-Per-Flow-Context-Source-Binding.md) (P-4, done), [Task-195: MCP-Backed Context Source Adapter](../../08-Task/done/Task-195-MCP-Backed-Context-Source-Adapter.md) (P-5, done — fake adapter per its own DOD), [Task-196: Per-Step Context Source Selection UI](../../08-Task/done/Task-196-Per-Step-Context-Source-Selection-UI.md) (P-7, done)
- Related Documents: [Task-204: Wire Production MCP Driver Adapter](../../08-Task/todo/Task-204-Wire-Production-MCP-Driver-Adapter.md) (follow-up, outside CP-44's own DOD-5 scope — production Google Drive backing for mcp.driver), [BUG-266: Changeledger Feature History Order Non-Deterministic](../../09-BugFix/done/BUG-266-Changeledger-Feature-History-Order-Nondeterministic-On-Equal-CommittedAt.md) (side-fix discovered while writing Task-192's golden test), [CP-41: RAG Harness Flow Mode](../inprogress/CP-41-RAG-Harness-Flow-Mode.md), [CP-42: Flow Pack And Generic Node Behavior Refactor](../done/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md) (mẫu registry gốc; **tiếp quản CP-42 `P-4`** "context packages become typed artifacts with declared bindings" — trước đây parked ở CP-43 §11, nay là charter của CP-44), [CP-43: Change Contract And Canonical Intent Signature](./CP-43-Change-Contract-And-Canonical-Intent-Signature.md) (consumer — Canonical Head là 1 source cắm vào CP-44), [Task-168: Flow Mode Context Package Contract](../../08-Task/done/Task-168-Flow-Mode-Context-Package-Contract.md), [Task-176: Node-Behavior Registry And Dispatch](../../08-Task/done/Task-176-Node-Behavior-Registry-And-Dispatch.md), [BUG-243: Flow Mode Validate And Audit Behaviors Disconnected](../../09-BugFix/done/BUG-243-Flow-Mode-Validate-And-Audit-Behaviors-Disconnected-From-Task-170-171.md)
- Replaces: `None`
- Tags: `context-regression-engine`, `flow-mode`, `context-source`, `registry`, `extensibility`, `mcp`, `deterministic-retrieval`

## AI Quick View

### Summary

- CP-44 biến việc thu thập context ở Plan step (`context.produce`) từ **4 nguồn hardcode** thành một **registry các Context Source cắm-được (pluggable)**, đúng theo mẫu `BehaviorRegistry` mà CP-42/Task-176 đã dựng cho node behavior.
- Mục tiêu cốt lõi: built-in code **hoặc** user/pack có thể **thêm bất kỳ loại context nào** (MCP driver file, Jira ticket id, Crashlytics, upstream doc ref, ...) mà **không sửa struct `FlowContextPackage`** và không đụng luồng executor.
- Đây là **substrate cho CP-43**: Canonical Head + Change Contract của CP-43 sẽ là các source đăng ký (`canonical.head`, `change.contract`) trên registry này, **không phải** thêm nhánh hardcode mới. Vì vậy CP-44 phải làm **trước** CP-43.
- Giữ nguyên bất biến của CP-41: **deterministic-only, không vector DB / embedding / similarity search**. Mỗi source truy xuất bằng lookup tường minh và phải mang `SourceRef`.
- Refactor phải **behavior-preserving**: output package cho 4 source hiện có phải giống hệt (golden test) để không phá Task-169 (renderer) và Task-171 (audit draft).

### Current Ask

- Thiết kế và lên kế hoạch triển khai một `ContextSourceRegistry` + interface `ContextSource` để mở rộng loại context ở Plan step một cách động, cắm được cả nguồn Go built-in lẫn nguồn MCP-backed, mà không đổi contract của `FlowContextPackage`.

- Runner-side substrate của CP-44 (`Task-191` → `Task-195`) đã land. Task UI của `Task-196` đã được làm nhưng sẽ được update bởi [CP-45](./CP-45-Generic-Artifact-Types-And-Instances.md): step sẽ chọn `context_artifact.v1` artifact instance thay vì chọn source-id trực tiếp.

### Key Decisions

- `P-1` Thêm interface `ContextSource` + `ContextSourceRegistry`, mirror `BehaviorRegistry` (CP-42 `P-1`, Task-176 `T-2/T-3`): pack/config chỉ **chọn source theo ID**; Go sở hữu implementation cho built-in; nguồn ngoài (MCP) cắm qua một adapter được khai báo trong pack.
- `P-2` Refactor 4 nguồn đang hardcode trong [`BuildFlowContextPackage`](../../../apps/local-runner/internal/runner/flow_context_package.go) — feature-history (CA + commits), chat-summary, source-excerpt — thành các registered source; builder **lặp qua registry theo priority** thay vì gọi cứng. Feature resolver vẫn là bước resolve đứng trước, cấp `feature_key` vào hints cho các source.
- `P-3` `FlowContextPackage` bổ sung `Sections []FlowContextSection` (mỗi section mang `SourceType` + `SourceRef`); các field top-level cũ (`HistoryBlock`, `DiscussionBlock`, `SourceExcerpts`) giữ lại dưới dạng **compatibility projection** để renderer (Task-169) và audit (Task-171) không đổi.
- `P-4` Việc bật/tắt/thêm source là **khai báo per-flow** qua binding `contexts:` trong pack YAML và per-project — thêm loại context = đăng ký source + khai báo, **không sửa struct**.
- `P-5` Nguồn MCP-backed (driver file — đã chạy ở Chat mode; sau này Jira ticket) cắm qua source adapter tái dùng đúng đường MCP của Chat mode; vẫn "deterministic" theo nghĩa lookup tường minh theo id, **không** similarity search.
- `P-6` CP-44 là substrate; Canonical Head (CP-43 `P-3`) và Change Contract (CP-43 `P-1`) là **future source cắm vào**, không phải nhánh hardcode — giữ interface ổn định để CP-43 land không phá vỡ.
- `P-7` (chốt `Q-2`, 2026-07-08) Nguồn ngoài chỉ được bind tới **các MCP mà hệ thống đã support/connect** — không cho phép lệnh shell hay đường dẫn tùy ý. Danh sách nguồn khai báo được giới hạn theo tập MCP đang hỗ trợ.
- `P-8` (chốt `Q-4`, 2026-07-08) **Tính đầy đủ quan trọng hơn thứ tự**: yêu cầu duy nhất là mọi context source đã enable đều được import vào package; thứ tự pack không phải ràng buộc cứng (giữ default theo `Priority` cho ổn định, nhưng không thêm cơ chế order per-flow).
- `P-9` (chốt `Q-5`, 2026-07-08) v1 các source **độc lập**, không dependency graph (không source nào đọc output source khác).
- `P-10` (chốt `Q-3`, 2026-07-08) Nguồn MCP gọi **đồng bộ tại Plan-time + timeout cứng** (degrade-mềm khi hết giờ/MCP down); cache là tối ưu về sau, không làm ở v1.
- `P-11` (mới, 2026-07-08, `Q-6`) Tính pluggable phải với tới **flow/step do user tạo qua Settings UI**, không chỉ pack YAML. User cần chọn được context payload cho step Plan của họ, giới hạn ở tập source đã đăng ký/app-support (kế thừa `P-7`) và giữ contract BUG-236. Thiết kế raw `context_sources` của [Task-196](../../08-Task/todo/Task-196-Per-Step-Context-Source-Selection-UI.md) đã được supersede bởi CP-45: triển khai cuối nên đi qua `context_artifact.v1` artifact instance ([Task-200](../../08-Task/done/Task-200-Step-Artifact-Instance-Binding-UI.md) + [Task-201](../../08-Task/done/Task-201-Context-Artifact-Migration-From-Context-Sources.md)).

### Constraints

- Không được phá contract output của Task-168: phải có regression test chứng minh package của 4 source hiện có là **byte-identical** sau refactor.
- Kế thừa ràng buộc CP-41: không vector DB, embedding index, hay similarity-search dependency ở bất kỳ source nào.
- Giữ đúng thứ tự packing deterministic của Task-168 `T-4` (source refs + confidence trước, newest truth, issue, excerpts, discussion, warnings/omitted).
- Mọi source (kể cả MCP) phải **bounded**, mang `SourceRef`, và **degrade graceful** (thiếu → warning, không phải error) như Task-168 `T-4`.
- Tái dùng ranh giới persistence hiện có (`WorkflowStore`, artifact/event/log); không tạo store context song song.
- Nguồn ngoài không được đọc dữ liệu tùy tiện: chỉ trong phạm vi pack-declared MCP hoặc path an toàn trong workspace (kế thừa `readSourceExcerpts` — chặn `outside_workspace`).

### Open Questions

- `Q-1` **(RESOLVED → [SD-22](../../06-System-Tech-Design/SD-22-Pluggable-Context-Source-Registry.md) created, 2026-07-08)** Design-of-record đã có: thay vì một addendum `SD-17 D-13`, tạo **SD-22** standalone (đối xứng SD-21↔CP-43). SD-22 phủ context-source registry; intent-signature vẫn ở SD-21. SD-22 nêu `Q-1` mở về việc có cần `SS-14 US-9` cho extensibility hay không.
- `Q-2` **(RESOLVED → `P-7`)** Chỉ cho phép bind tới MCP đã được support/connect; không lệnh/đường dẫn tùy ý.
- `Q-3` **(RESOLVED → `P-10`)** Nguồn MCP: live + timeout cứng trước, cache sau. Chỉ áp dụng khi làm Task-195.
- `Q-4` **(RESOLVED → `P-8`)** Thứ tự không quan trọng; yêu cầu là import đầy đủ mọi source enabled.
- `Q-5` **(RESOLVED → `P-9`)** v1 các source độc lập, không dependency graph.
- `Q-6` **(RESOLVED → `P-7` + `P-11`, 2026-07-08 → [Task-196](../../08-Task/todo/Task-196-Per-Step-Context-Source-Selection-UI.md), superseded by CP-45)** Flow/step do user tạo qua UI có chọn được context source không? Có, nhưng không nên expose raw source-id là abstraction cuối. CP-45 thay bằng step bind tới `context_artifact.v1` artifact instance; instance config giữ `sources`. Task-194 chỉ phủ pack-YAML binding; [Task-200](../../08-Task/done/Task-200-Step-Artifact-Instance-Binding-UI.md)/[Task-201](../../08-Task/done/Task-201-Context-Artifact-Migration-From-Context-Sources.md) phủ mặt UI/DB cho user-authored.

### Source Refs

- `SD-17` D-3 (ordered history, newest = truth), D-4, §3.2 (chat_summary), §6.1 (prompt-assembly seam).
- `SS-13` change-ledger + phase-document contract.
- `CP-41` `P-1/P-2/P-3` (Plan step sở hữu context, deterministic-only, no-vector).
- `CP-42` `P-1` + `Task-176` `T-2/T-3` — mẫu `BehaviorRegistry.Register/Resolve/Dispatch` để nhân bản cho context source.
- `Task-168` `T-1..T-6` — contract `FlowContextPackage` hiện tại và các nguồn hardcode cần migrate.
- `CP-43` `P-1/P-3` — consumer tương lai (Canonical Head, Change Contract) cắm vào registry này.

## 1. Goal

Cho Plan step (behavior `context.produce`) khả năng thu thập context từ **một tập source mở rộng động**, thay vì 4 nguồn cố định hiện đang được gọi cứng trong `BuildFlowContextPackage`. Sau CP-44:

- Thêm một loại context mới (MCP driver file, Jira ticket id, Crashlytics report, upstream doc ref, Canonical Head của CP-43, ...) = **đăng ký một `ContextSource` + khai báo trong pack**, không sửa `FlowContextPackage`, không sửa executor, không sửa renderer.
- 4 nguồn hiện có (feature-history, chat-summary, source-excerpt, cộng bước feature-resolve) trở thành các source chuẩn hóa trên cùng registry, với output **không đổi**.
- Registry là nền để `CP-43` cắm Canonical Head + Change Contract vào như source, tránh phải hardcode rồi refactor lần hai.

Bất biến kế thừa từ CP-41: toàn bộ retrieval là **deterministic, tường minh, auditable** — không vector/embedding/similarity search ở bất kỳ source nào.

## 2. Input Documents

- `requirements/06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md`
- `requirements/05-System-Specs/SS-13-AI-Followable-Document-Contract.md`
- `requirements/07-Coding-Plan/inprogress/CP-41-RAG-Harness-Flow-Mode.md`
- `requirements/07-Coding-Plan/done/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md`
- `requirements/07-Coding-Plan/todo/CP-43-Change-Contract-And-Canonical-Intent-Signature.md`
- `requirements/08-Task/done/Task-168-Flow-Mode-Context-Package-Contract.md`
- `requirements/08-Task/done/Task-176-Node-Behavior-Registry-And-Dispatch.md`
- `apps/local-runner/internal/runner/flow_context_package.go`
- `apps/local-runner/internal/runner/behavior_registry.go`
- `apps/local-runner/internal/runner/behavior_registry_builtin.go`
- `apps/local-runner/internal/agentpack/flow-pack/flows/rag-harness.yaml`
- `apps/local-runner/internal/agentpack/flow-pack/contexts/flow-context-package.yaml`

## 3. Implementation Strategy

- overall approach:
  - Dựng `ContextSourceRegistry` như bản sao kiến trúc của `BehaviorRegistry` (CP-42): `Register(spec)` (chặn trùng ID), `Resolve(id)` (fail-fast nếu unknown), và một `Collect(ctx, hints)` chạy các source đã enable theo priority.
  - `BuildFlowContextPackage` chuyển từ "gọi cứng 4 bước" sang "resolve feature → lấy danh sách source enabled → collect theo priority → gộp thành `Sections` → projection ra field cũ".
  - Nguồn Go built-in đăng ký lúc khởi tạo (giống `NewDefaultBehaviorRegistry`). Nguồn MCP-backed cắm qua adapter được khai báo trong pack, gọi lại đường MCP Chat-mode đã có.
  - Giữ output không đổi cho 4 source hiện tại bằng golden/regression test trước khi mở rộng.
- sequencing logic:
  - `P-1` (interface + registry) → `P-2` (migrate 4 source, giữ output) → `P-3` (mở rộng package = `Sections` + projection) → `P-4` (binding per-flow trong pack) → `P-5` (adapter MCP source, demo driver file) → `P-6` (test regression + extensibility).
  - `P-1/P-2` có thể merge trước như một refactor behavior-preserving thuần túy, chưa lộ tính năng mới.
- dependencies:
  - Yêu cầu `CP-42`/`Task-176` `BehaviorRegistry` đã merge (đã có) để nhân bản mẫu.
  - `P-5` MCP source phụ thuộc đường MCP Chat-mode hiện có.
  - `BUG-243` **không** phụ thuộc CP-44 (khác node); nên fix trước để CP-41 chạy live, độc lập với CP-44.
  - `CP-43` phụ thuộc CP-44 (cắm Canonical Head/Change Contract như source).

## 4. Work Breakdown

- `P-1` Định nghĩa contract source + registry — `internal/runner/context_source_registry.go` (hoặc package mới `internal/contextsource/`).
  - Interface đề xuất:
    ```go
    type ContextSource interface {
        ID() string            // "feature.history", "chat.summary", "source.excerpt",
                               // "mcp.driver", "jira.ticket", "canonical.head" (CP-43)
        Priority() int          // vị trí packing theo Task-168 T-4
        Deterministic() bool    // registry từ chối source vi phạm bất biến no-vector
        Fetch(ctx context.Context, hints FlowContextHints) (FlowContextSection, error)
    }
    ```
  - `ContextSourceRegistry` với `Register`/`Resolve`/`Collect`; `Register` chặn trùng ID và từ chối `Deterministic()==false` trong v1.
  - `Collect` gọi từng source enabled, thu `FlowContextSection`, dừng-mềm (source lỗi → warning trong package, không fail run).

- `P-2` Migrate 4 nguồn hardcode thành registered source.
  - `feature.history` bọc `HistorySlot`/`composeFeatureBlocks` (CA + commits).
  - `chat.summary` bọc `ChatSummarySlot`.
  - `source.excerpt` bọc `readSourceExcerpts` (đọc `ChangedPaths` + `ExplicitSourcePaths`, giữ nguyên omitted-reasons).
  - Bước feature-resolve giữ nguyên là tiền xử lý cấp `feature_key` + confidence vào `hints`.
  - `BuildFlowContextPackage` refactor để iterate registry; **output phải không đổi**.

- `P-3` Mở rộng model package không phá contract.
  - Thêm `Sections []FlowContextSection` (mỗi cái có `SourceType`, `SourceRef`, `Body`, `Omitted`, `Confidence`).
  - Giữ `HistoryBlock`/`DiscussionBlock`/`SourceExcerpts`/`Warnings`/`Omitted` như projection suy ra từ `Sections`, để `RenderFlowContextPackage` (Task-169) và `BuildAuditDraft` (Task-171) chạy nguyên trạng.
  - `RenderFlowContextPackage` có thể render thêm section mới generic mà vẫn giữ dòng `No vector retrieval used`.

- `P-4` Binding source per-flow.
  - Mở rộng khai báo `contexts:` trong pack YAML (`rag-harness.yaml`, `contexts/flow-context-package.yaml`) để liệt kê source-id enabled + thứ tự.
  - Mặc định = 4 source hiện có, giữ tương thích flow cũ (flow không khai báo → dùng default set).
  - Per-project override tùy chọn (đặt sau nếu cần).

- `P-5` Adapter cho nguồn ngoài (MCP-backed).
  - `mcp.driver` source: nhận driver ref từ hints/pack, gọi đường MCP Chat-mode, trả về `FlowContextSection` bounded + `SourceRef`.
  - Timeout + degrade khi MCP down (thành warning, không fail run).
  - Đặt nền cho `jira.ticket` (sau khi có MCP Jira) dùng cùng adapter contract.

- `P-6` Test regression + extensibility (xem §7).

- `P-7` UI + persistence cho **user-authored step** chọn context payload — original raw-source plan là [Task-196](../../08-Task/todo/Task-196-Per-Step-Context-Source-Selection-UI.md), nay superseded bởi CP-45.
  - Không triển khai final UX bằng `StepDefinition.contextSources`/`step_definitions.context_sources` nếu CP-45 đang active.
  - Step-form `WorkflowsSettings.tsx` nên bind tới artifact instance qua [Task-200](../../08-Task/done/Task-200-Step-Artifact-Instance-Binding-UI.md).
  - `context_artifact.v1` instance config giữ danh sách source (`sources`) và migrate/fallback từ CP-44 qua [Task-201](../../08-Task/done/Task-201-Context-Artifact-Migration-From-Context-Sources.md).
  - Giữ invariant cũ: options giới hạn ở source đã đăng ký/app-support, runner fail-fast id lạ, và `workflow_steps` không nhận metadata mới (BUG-236).

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/runner/flow_context_package.go` (refactor builder + model)
  - `apps/local-runner/internal/runner/behavior_registry_builtin.go` (`behaviorContextProduce` gọi registry)
  - mới: `apps/local-runner/internal/runner/context_source_registry.go` (hoặc package `internal/contextsource/`)
  - mới: `apps/local-runner/internal/runner/context_source_registry_test.go`
  - `apps/local-runner/internal/featurecatalog/slots.go`, `chat_summary_slots.go` (bọc, không sửa hành vi)
  - `apps/local-runner/internal/agentpack/flow-pack/flows/rag-harness.yaml`
  - `apps/local-runner/internal/agentpack/flow-pack/contexts/flow-context-package.yaml`
  - (P-7 / CP-45 replacement) `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx`, `packages/flowpilot-client-core/src/domain/adminModels.ts`, `packages/flowpilot-client-core/src/data/supabaseAdminRepository.ts`, `apps/local-runner/internal/runner/flow_definition_resolver.go` + Supabase tables `artifact_types`, `artifact_instances`, `step_artifact_bindings`
- modules:
  - runner context assembly (Plan step)
  - feature catalog / change ledger / chat-summary ledger (qua bọc)
  - agentpack (context binding schema)
  - MCP integration (adapter source)
  - (P-7) desktop settings authoring UI + client-core admin repo/domain (per-step source selection)
- database:
  - không thêm bảng; dùng artifact/event/log hiện có cho package.
- external systems:
  - không thêm vector DB/embedding; nguồn MCP dùng đường MCP hiện có (driver, sau này Jira).

## 6. Data or Migration Steps

- schema:
  - Không migration DB. `Sections` nằm trong payload artifact/event của package.
- data backfill:
  - Không cần; package được build lại mỗi Plan run.
- config updates:
  - Thêm khai báo `contexts:` (danh sách source-id + order) trong pack YAML; mặc định giữ 4 source hiện có.
  - Tùy chọn: config per-project để enable source MCP (driver/Jira) và timeout.

## 7. Validation Plan

- tests to add:
  - `TestContextSourceRegistryRegisterRejectsDuplicate` / `...ResolveUnknownFails` — mirror `behavior_registry_test.go`.
  - `TestContextSourceRegistryRejectsNonDeterministicSource` — bảo vệ bất biến no-vector.
  - `TestBuildFlowContextPackageOutputUnchangedAfterRegistryRefactor` — golden test byte-identical cho 4 source hiện có (regression bảo vệ Task-168/169/171).
  - `TestRegisterCustomSourceAppearsInPackageWithoutStructChange` — đăng ký fake source → xuất hiện trong `Sections` + render, không sửa struct.
  - `TestContextSourceCollectDegradesOnSourceError` — 1 source lỗi → warning, không fail run.
  - `TestMcpDriverSourceProducesBoundedSectionWithSourceRef` — nguồn MCP-backed bounded + có SourceRef.
  - `TestFlowContextPackageStillHasNoVectorDependency` — giữ guard CP-41.
- manual checks:
  - Chạy RAG Harness với default set → package giống hành vi CP-41 Scenario 1.
  - Bật `mcp.driver` source trong pack → context có block driver, degrade sạch khi MCP tắt.
- failure cases:
  - source-id unknown trong pack → fail-fast lúc validate flow (không silent no-op).
  - MCP source timeout/down → warning + package vẫn dựng từ source còn lại.
  - source ngoài trả path ngoài workspace → omitted `outside_workspace`.
  - hai source cùng priority → thứ tự ổn định, tie-break theo ID.

## 8. Rollout and Fallback

- rollout order:
  - Ship `P-1/P-2/P-3` như refactor behavior-preserving (default set = 4 source; không lộ tính năng mới).
  - Bật `P-4` binding per-flow, vẫn default tương thích.
  - Bật `P-5` `mcp.driver` source (internal/flag trước).
  - `CP-43` sau đó land Canonical Head + Change Contract như source mới.
- fallback path:
  - Nếu registry/binding lỗi, degrade về default set 4 source (hành vi CP-41 hiện tại).
  - Nếu một source lỗi, bỏ qua source đó với warning; không chặn Plan step.
- monitoring:
  - Log: danh sách source enabled, priority, thời gian fetch mỗi source, source degrade/omit, kích thước package, `feature_key`/confidence.

## 9. Risks

- `R-1` Refactor gây regression output package → phá Task-169 renderer/Task-171 audit. Mitigation: golden test byte-identical trước khi mở rộng (P-6 chạy song song P-2).
- `R-2` Nguồn MCP tại Plan-time gây latency/treo. Mitigation: timeout cứng + degrade-thành-warning; cân nhắc cache (Q-3).
- `R-3` Bảo mật: source do user khai báo đọc dữ liệu tùy tiện. Mitigation: chỉ cho phép MCP pack-declared + path an toàn trong workspace; chốt ở Q-2 trước khi mở cho user.
- `R-4` Xói mòn determinism nếu một source lén dùng similarity search. Mitigation: interface bắt buộc `Deterministic()`, registry từ chối source vi phạm; giữ guard no-vector của CP-41.
- `R-5` Coupling với CP-43. Mitigation: khóa interface `ContextSource` ổn định để Canonical Head/Change Contract cắm vào mà không sửa struct; CP-43 chỉ thêm source + gate rules.
- `R-6` Pack YAML tham chiếu source-id sai. Mitigation: validate flow lúc load, fail-fast như `BehaviorRegistry.Resolve`.

## 10. Definition of Done

- [x] `DOD-1` `ContextSource` interface + `ContextSourceRegistry` tồn tại, có test (Register chặn trùng, Resolve fail-fast, từ chối non-deterministic).
- [x] `DOD-2` 4 nguồn hiện có (feature-history, chat-summary, source-excerpt + bước feature-resolve) đã migrate thành registered source; golden test chứng minh output package **không đổi**.
- [x] `DOD-3` `FlowContextPackage` hỗ trợ N source qua `Sections` + projection tương thích; thêm một source = đăng ký + khai báo, **không sửa struct/contract**.
- [x] `DOD-4` Có binding source per-flow trong pack YAML; flow không khai báo vẫn dùng default set (tương thích ngược).
- [x] `DOD-5` Ít nhất một nguồn MCP-backed (`mcp.driver`) hoạt động end-to-end, bounded, có SourceRef, degrade sạch khi MCP down.
- [x] `DOD-6` Toàn bộ đường CP-44 không có vector DB/embedding/similarity-search dependency (guard test giữ nguyên từ CP-41).
- [x] `DOD-7` Chứng minh (bằng test/PoC) `CP-43` có thể đăng ký Canonical Head như một source mà **không** sửa struct `FlowContextPackage`.
- [x] `DOD-8` Package/source state gắn với `workflow_run_id` + Plan `workflow_step_run_id` hiện có, không tạo store context song song.
- [x] `DOD-9` User tạo/sửa một step qua Settings UI chọn được tập context source (giới hạn ở source app-support); lựa chọn persist vào `step_definitions` và `behaviorContextProduce` dùng đúng tập đó ở Plan-time; flow user-tạo không còn âm thầm rơi về default set (Task-196). — done.

## 11. E2E Test Matrix

> **Scope note.** Các use case dưới đây chỉ verify phần đã ship của CP-44 (`Task-191` → `Task-195`) ở runner/pack/runtime. **Không** bao phủ user-authored step selection; phần đó đã chuyển sang CP-45 artifact instance UX thay vì `step_definitions.context_sources` raw UI.

### 11.1 - (PASSED) Default Built-In Flow Works Unchanged

- Mục tiêu: chứng minh refactor registry không làm đổi hành vi `rag-harness` mặc định.
- Cách test:
  1. Chạy flow `rag-harness` với prompt map rõ vào một feature đã có history/chat-summary.
  2. Quan sát package render/prompt handoff của node `context`.
  3. Xác nhận có đủ các block quen thuộc: feature history, chat summary, source excerpts, và dòng `No vector retrieval used`.
- Kỳ vọng:
  - Output tương đương hành vi cũ trước CP-44.
  - Không có lỗi unknown source hay missing section.

**VERIFIED 2026-07-09 (manual E2E, live app, run `db51ec26-1a0f-4b92-8ceb-b03dc8e9b363` / `run-13065`)**: chạy `rag-harness` qua "Run Flow" trên feature `calc-core` (`D:\working\gate-sandbox\calc.go`). `.flowpilot/logs/features/agent-flow-engine/run-13065.ndjson` cho thấy `context` chuyển `DONE` trước khi `implement` được spawn (`flow_start_inline_entry`), và prompt handoff (`run-13070/prompt-turn-13075.txt`) có đủ block `Flow Context Package` — feature `calc-core (confidence: verified)`, Change History, Prior Discussion, dòng `No vector retrieval used` — trước task chính. Flow chạy hết `context → implement → validate → audit` sạch, không lỗi. Lần chạy trước đó (`run-12804`, cùng phiên debug) từng lộ ra bug thật ở `entryDelegateNodes`/`entryNodesNoDeps` (edge-unaware entry detection, xem `flow_executor.go` doc comment) — đã fix trước lần verify này.
- **RE-VERIFIED 2026-07-09 sau khi fix duplicate-history bug** (`run-4352` → child `run-4357`, `flow_ref: flowpilot-core-flow-pack/rag-harness`, feature `calc-core`): `run-4357/prompt-turn-4362.txt` có đúng 1 lần `### Change History` và đúng 1 lần `### Prior Discussion` — không còn preamble `## Prior work on`/`## Prior discussion on` bị chèn lặp lại phía trước `[FlowPilot sub-agent — ...]` như các lần chạy trước fix (`run-13065`, `run-13592`). Xem [BUG-268](../../09-BugFix/done/BUG-268-Flow-Coding-Prompt-Duplicates-Feature-History.md) cho root cause đầy đủ.

### 11.2 - (PASSED) Flow With Explicit Source Subset

- Mục tiêu: chứng minh per-flow binding thật sự giới hạn nguồn được dùng.
- Cách test:
  1. Clone một flow test từ `rag-harness` nhưng để `contexts.main_context.sources: [feature.history]`.
  2. Chạy flow với prompt map vào feature có đủ history + chat-summary + excerpts.
  3. Quan sát artifact/package của node `context`.
- Kỳ vọng:
  - `pkg.Sections` chỉ có `feature.history`.
  - `HistoryBlock` có dữ liệu.
  - `DiscussionBlock` và `SourceExcerpts` rỗng.

**VERIFIED 2026-07-09 (manual E2E, live app, run `db51ec26-1a0f-4b92-8ceb-b03dc8e9b363` / `run-13587` → `run-13592`)**: dùng CP-45 artifact-binding thay cho YAML `contexts.main_context.sources` (tương đương về hiệu lực — cùng đi qua `resolveArtifactBoundContextSources`, tier ưu tiên cao nhất trong `resolveEnabledContextSourceIDs`). Clone `rag-harness`, tạo `context_artifact.v1` instance "History Only Context" (`config_json.sources: ["feature.history"]`), bind làm **output** trên step `context` của bản clone. Chạy trên feature `calc-core` (đủ cả history + chat-summary). Prompt handoff (`run-13592/prompt-turn-13597.txt`) chỉ còn section `### Change History` — không còn `### Prior Discussion` (chat.summary) — `prompt_len` giảm từ 4041 (default 3 nguồn, §11.1) xuống 1733. Đúng kỳ vọng.
- Lần thử đầu (`run-13326`/`run-13331`) bị leak `chat.summary` do 2 bug thật phát hiện giữa lúc test, cả hai đã fix trước lần verify cuối:
  1. Binding artifact bị lưu nhầm vào step của flow **built-in gốc** (`step_definition_id` không namespace) thay vì step của clone — do `WorkflowsSettings.tsx`'s `refresh()` validate `stepType` carry-over theo catalog `step_definitions` toàn cục thay vì theo `workflow_steps` của đúng workflow đang chọn ([WorkflowsSettings.tsx:731](../../../apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx:731)) — đã fix, đồng thời đã xoá binding contaminate khỏi built-in gốc.
  2. `saveArtifactInstance` gửi `id: ""` cho instance mới → Postgres `invalid input syntax for type uuid` ([supabaseAdminRepository.ts:1055](../../../packages/flowpilot-client-core/src/data/supabaseAdminRepository.ts:1055)) — đã fix.
- **RE-VERIFIED 2026-07-09 sau khi fix duplicate-history bug** (`run-4286` → child `run-4291`, flow clone, `config_json.sources: ["feature.history"]`): `run-4291/prompt-turn-4296.txt` chỉ có `### Change History`, không có `### Prior Discussion`, và không còn preamble lặp lại — đúng cả 2 kỳ vọng (giới hạn nguồn đúng, và không duplicate). Xem [BUG-268](../../09-BugFix/done/BUG-268-Flow-Coding-Prompt-Duplicates-Feature-History.md).

### 11.3 Unknown Source ID Fails Fast

- Mục tiêu: chứng minh pack sai không bị silent no-op.
- Cách test:
  1. Tạo flow test với `sources: [feature.history, totally.unknown.source]`.
  2. Load/resolve flow qua runner.
- Kỳ vọng:
  - Resolve/load flow fail ngay.
  - Lỗi nêu rõ flow id, context binding, và source id lạ.

### 11.4 Generic Section Rendering For New Source

- Mục tiêu: chứng minh thêm source mới không cần sửa `FlowContextPackage`.
- Cách test:
  1. Đăng ký một fake/custom source trong test harness hoặc local branch.
  2. Enable source đó cùng default set.
  3. Build + render package.
- Kỳ vọng:
  - Section mới xuất hiện trong `pkg.Sections`.
  - Renderer có heading generic `### <sourceType>`.
  - SourceRef và body của section mới được render.
  - Không cần thêm field mới vào struct package.

### 11.5 MCP Source Happy Path

- Mục tiêu: chứng minh `mcp.driver` là source ngoài đầu tiên hoạt động end-to-end.
- Cách test:
  1. Tạo flow test bật `mcp.driver`.
  2. Cấp `MCPDriverRef` hợp lệ qua hints/path test.
  3. Dùng fake adapter hoặc adapter integration tương đương để trả nội dung driver.
- Kỳ vọng:
  - Package có section `mcp.driver`.
  - Section có `SourceRef` dạng `mcp:<driver-ref>`.
  - Nội dung bị cap trong giới hạn bounded.
  - Flow vẫn render bình thường cùng các source khác.

### 11.6 MCP Timeout Or Down Degrades Gracefully

- Mục tiêu: chứng minh MCP lỗi không làm fail Plan step.
- Cách test:
  1. Chạy flow có bật `mcp.driver`.
  2. Làm adapter timeout hoặc trả lỗi.
- Kỳ vọng:
  - Plan/context step vẫn hoàn tất.
  - Package có warning về `mcp.driver`.
  - Các source còn lại vẫn xuất hiện bình thường.

### 11.7 Safety And Determinism Guards

- Mục tiêu: verify các guard cốt lõi của CP-41/CP-44 vẫn còn load-bearing.
- Cách test:
  1. Chạy case source excerpt có path ngoài workspace hoặc symlink escape.
  2. Chạy render/package với source set có `mcp.driver`.
  3. Chạy lặp lại cùng một input nhiều lần.
- Kỳ vọng:
  - Path không an toàn bị omitted với lý do rõ (`outside_workspace` hoặc tương đương).
  - Output vẫn có dòng `No vector retrieval used`.
  - Thứ tự section ổn định theo `Priority`, tie-break theo `SourceType`.

### 11.8 Suggested Automated Commands

- `rtk go test ./apps/local-runner/internal/runner/...`
- `rtk go test ./apps/local-runner/internal/changeledger/...`
- `rtk go test ./apps/local-runner/internal/runner/... -run 'Test(BuildFlowContextPackageOutputUnchangedAfterRegistryRefactor|RegisterCustomSourceAppearsInPackageAndRender|FlowWithoutSourcesUsesDefaultSet|FlowWithExplicitSourceSubset|UnknownSourceIDFailsFlowLoad|McpDriverSource)'`
