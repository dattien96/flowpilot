# SD-22: Pluggable Context Source Registry

## Metadata

- Document ID: `SD-22`
- Title: `Pluggable Context Source Registry`
- Phase: `tech_design`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-08`
- Last Updated: `2026-07-08`
- Parent Documents: [SS-13: AI-Followable Document Contract](../05-System-Specs/SS-13-AI-Followable-Document-Contract.md), [SS-14: Code Context And Regression Safety](../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md) (**US-9**, AC-16; also US-1, US-2, US-5, US-7, AC-3, AC-8, AC-9, AC-13, AC-15)
- Child Documents: [CP-44: Pluggable Context Source Registry](../07-Coding-Plan/done/CP-44-Pluggable-Context-Source-Registry.md)
- Related Documents: [SD-17: Context And Regression Engine](./SD-17-Context-And-Regression-Engine.md) (D-4 context package, D-11 deferred scope-drift), [SD-21: Change Contract And Canonical Intent Signature](./SD-21-Change-Contract-And-Canonical-Intent-Signature.md) (consumer — Canonical Head là một context source cắm vào registry này), [SD-20: Flow Gate Rule Semantics](./SD-20-Flow-Gate-Rule-Semantics.md), [SD-10: Context Resolver & RAG](./SD-10-Context-Resolver-RAG.md), [CP-41: RAG Harness Flow Mode](../07-Coding-Plan/done/CP-41-RAG-Harness-Flow-Mode.md), [CP-42: Flow Pack And Generic Node Behavior Refactor](../07-Coding-Plan/done/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md) (realizes CP-42 `P-4`)
- Replaces: `None`
- Tags: `context-source, registry, flow-mode, context-harness, extensibility, mcp, deterministic-retrieval, plan-step`

## AI Quick View

### Summary

- SD-22 là design-of-record cho việc **thu thập context ở Plan step** trở thành một **registry các nguồn cắm-được (pluggable Context Source)**, thay cho 4 nguồn hardcode trong `BuildFlowContextPackage` (Task-168).
- Kiến trúc nhân bản mẫu `BehaviorRegistry` của CP-42/Task-176 nhưng ở tầng nguồn: pack/config chỉ **chọn source theo ID**; Go sở hữu implementation built-in; nguồn ngoài (MCP driver, sau này Jira) cắm qua adapter — **không đổi struct `FlowContextPackage`**.
- Giữ nguyên bất biến deterministic của SD-17/CP-41: mọi source truy xuất bằng lookup tường minh, **không vector DB/embedding/similarity**; registry từ chối source non-deterministic.
- Là **substrate cho SD-21**: Canonical Head + Change Contract là các source đăng ký trên registry này, nên SD-22 land trước.
- Hiện thực hóa CP-42 `P-4` ("context packages become typed artifacts with declared bindings") mà CP-42 để lại chưa làm.

### Current Ask

- Chốt thiết kế kỹ thuật cho một context-source registry cho Plan step: interface, contract nguồn, mô hình package, cách khai báo per-flow, adapter MCP, và ranh giới bảo mật — đủ để CP-44 (Task-191..195) code theo mà không phát sinh edge-case về cấu trúc.

### Key Decisions

- `D-1` Context sourcing là một **`ContextSourceRegistry`** các `ContextSource` cắm-được, mirror `BehaviorRegistry` (CP-42 `P-1`/Task-176). Pack chọn source theo ID; Go sở hữu implementation; `Register` chặn trùng ID, `Resolve` fail-fast khi unknown.
- `D-2` Mọi source phải **deterministic** (`Deterministic()==true`, lookup tường minh theo id/ref); registry **từ chối** source non-deterministic ở v1 — giữ bất biến no-vector của CP-41/SD-17 `D-3`.
- `D-3` `FlowContextPackage` trở thành **composition of typed `FlowContextSection[]`** (mỗi section mang `SourceRef`), thay các block cố định; field cũ (`HistoryBlock`/`DiscussionBlock`/`SourceExcerpts`) giữ làm **backward-compat projection**. `PackageID` vẫn = `f(run_id, step_id, feature_key)`, **không** hash nội dung section (giữ ổn định cho audit/retry). Đây là CP-42 `P-4`.
- `D-4` Nguồn enabled **khai báo per-flow** trong pack YAML (`contexts.sources`); vắng → default built-in set; source-id lạ → **fail-fast lúc load flow**.
- `D-5` Nguồn ngoài chỉ bind tới **MCP đã được hệ thống support/connect** — không lệnh shell / đường dẫn tùy ý (ranh giới bảo mật). Nguồn MCP chạy **đồng bộ tại Plan-time + timeout cứng**; timeout/lỗi → **degrade-mềm** (warning), không fail Plan step (`AC-9`, `AC-13`).
- `D-6` Các source **độc lập** (không dependency graph) ở v1; **feature-resolve là tiền xử lý** (cấp `feature_key`+confidence vào hints), không phải source.
- `D-7` **Đầy đủ hơn thứ tự**: yêu cầu là mọi source enabled đều được import; thứ tự pack là default ổn định (theo `Priority`), không phải ràng buộc cứng.
- `D-8` Canonical Head + Change Contract (SD-21) là **future registered source** trên registry này; SD-22 là substrate, SD-21 build lên trên (đóng `SD-21 P-5` packing qua source `feature.history`/`canonical.head`, không dùng slot bespoke).

### Constraints

- Không phá contract output của Task-168: refactor phải **behavior-preserving** (golden test byte-identical cho tập nguồn mặc định) — bảo vệ Task-169 (renderer) và Task-171 (`BuildAuditDraft`).
- Kế thừa SD-17 `D-3` deterministic + CP-41 no-vector.
- Tái dùng seam sẵn có: `featurecatalog` (`HistorySlot`/`ChatSummarySlot`), `changeledger`, `readSourceExcerpts`, `WorkflowStore` (artifact/event/log). Không tạo store context song song.
- Nguồn ngoài giới hạn ở MCP pack-declared + path workspace-safe.
- Local-first + Drive sync theo cơ chế hiện có (SD-17 `§5.1`); không thêm bảng Supabase.

### Open Questions

- `Q-1` **(RESOLVED, 2026-07-08)** `SS-14 US-9` + `AC-16` đã được thêm để làm spec-authority cho extensibility (thêm loại context mới một cách declarative, deterministic, degrade-mềm, giới hạn nguồn ngoài ở integration đã support). SD-22 giờ trace thẳng về US-9/AC-16.
- `Q-2` Cache kết quả nguồn MCP (thay vì gọi live mỗi Plan run) — hoãn sau v1 (CP-44 `P-10`).
- `Q-3` Per-project override tập nguồn (ngoài phạm vi pack) — cân nhắc khi có nhu cầu (CP-44/Task-194 `Q-2`).

### Source Refs

- `SS-14` US-1, US-2, US-5, US-7; AC-3, AC-8, AC-9, AC-13, AC-15.
- `SD-17` `D-3` (deterministic retrieval), `D-4` (feature resolution), `§4.2.1` (history packing), `§5.1` (on-disk + Drive sync).
- `SD-21` `P-3`/`P-5` (Canonical Head as packed authority — consumer).
- `CP-42` `P-4` (typed context bindings — realized here), `Task-176` (BehaviorRegistry pattern).
- current code: `flow_context_package.go`, `behavior_registry.go`, `behavior_registry_builtin.go` (`behaviorContextProduce`/`behaviorContextRender`), `agentpack/pack.go`, `flow-pack/flows/rag-harness.yaml`, `flow-pack/contexts/flow-context-package.yaml`.

## 1. Goal

Cho Plan step (behavior `context.produce`) khả năng thu thập context từ một **tập nguồn mở rộng động**, để thêm một loại context mới = đăng ký một `ContextSource` + khai báo trong pack, **không** sửa struct `FlowContextPackage`, executor, hay renderer. Thiết kế phải giữ bất biến deterministic/no-vector và không phá bất kỳ consumer nào của package hiện tại.

## 2. Input Documents

- `SS-14` (US-1, US-2, US-5, US-7, AC-3, AC-8, AC-9, AC-13, AC-15) — spec context & regression safety.
- `SS-13` — document/context contract (change-ledger, source refs).
- `SD-17` (`D-3`, `D-4`, `§4.2.1`, `§5.1`) — thiết kế context engine mà SD-22 mở rộng.
- `CP-42 P-4`, `Task-176` — mẫu registry.

## 3. Architecture Decision

### 3.1 `D-1` Context Source Registry

- `D-1` Thay 4 nguồn gọi cứng trong `BuildFlowContextPackage` bằng một `ContextSourceRegistry` các `ContextSource` cắm-được.
- Alternatives considered:
  - Giữ 4 nguồn hardcode + thêm `if` cho từng loại mới → không mở rộng, mỗi loại mới sửa builder + struct.
  - Một DSL/config-engine cho retrieval → over-engineered, khó giữ deterministic + khó review.
- Why this option: đối xứng với `BehaviorRegistry` (CP-42) đã chứng minh; pack chọn ID, Go giữ implementation → an toàn + mở rộng không đụng contract.

### 3.2 `D-2` Deterministic-only contract

- `D-2` Interface bắt buộc `Deterministic() bool`; registry từ chối đăng ký source trả `false` ở v1.
- Why: giữ SD-17 `D-3` + CP-41 no-vector; ngăn một source lén dùng similarity search làm xói mòn tính giải thích được.

### 3.3 `D-3` Typed sections + backward-compat projection

- `D-3` Package = `Sections []FlowContextSection` (mỗi cái có `SourceType`, `Priority`, `SourceRef`, `Body`, `Omitted`, `Confidence`); field cũ là projection suy ra từ Sections. `PackageID` giữ công thức cũ.
- Alternatives: đổi hẳn sang Sections và bỏ field cũ → phá Task-169/171.
- Why: mở rộng mà không phá consumer; `PackageID` ổn định để audit `OriginalPackageID`/retry `OriginalPlanPackageID` không vỡ.

### 3.4 `D-4` Per-flow declared binding

- `D-4` Pack YAML khai báo `contexts.sources: [...]`; vắng → default set; id lạ → lỗi load.
- Why: bật/tắt/thêm nguồn là việc sửa pack, không sửa Go; fail-fast tránh flow chạy thiếu nguồn âm thầm.

### 3.5 `D-5` External (MCP) source boundary + degrade

- `D-5` Nguồn ngoài chỉ bind MCP đã support/connect; live + timeout cứng; degrade-mềm khi lỗi.
- Why: an toàn (không chạy input tùy ý), và không để Plan step treo/chết theo MCP (`AC-9`, `AC-13`).

### 3.6 `D-6`/`D-7` Independence & completeness

- `D-6` Source độc lập; feature-resolve là tiền xử lý cấp hints.
- `D-7` Yêu cầu import đầy đủ mọi source enabled; thứ tự chỉ là default ổn định theo `Priority`.

### 3.7 `D-8` Substrate for SD-21

- `D-8` Canonical Head/Change Contract của SD-21 là source đăng ký trên registry này; SD-22 land trước SD-21.

## 4. Component Impact

- Impacted modules: runner context assembly (`flow_context_package.go`, `behaviorContextProduce`), agentpack schema (`contexts.sources`), flow resolve/validate.
- New modules: `context_source_registry.go` (interface + registry), `context_sources_builtin.go` (3 nguồn migrate), `context_source_mcp.go` (adapter MCP).
- Reused unchanged: `featurecatalog` (`HistorySlot`/`ChatSummarySlot`), `changeledger`, `readSourceExcerpts`, `WorkflowStore` persistence, renderer/prompt seams (`RenderFlowContextPackage`, `ComposeFlowCodingPrompt`).
- User-authored surface (CP-44 `P-7`/Task-196): binding `contexts.sources` áp cho pack-YAML flow; với flow/step do user tạo qua Settings UI, tập source chọn ở tầng **step-definition** (`step_definitions.context_sources`, mirror `required_mcps`, giữ contract BUG-236) và `behaviorContextProduce` resolve theo ưu tiên step → flow → default. Chỉ thêm cột, không thêm bảng (giữ `§Constraints`). Danh sách chọn được giới hạn ở source đã đăng ký (kế thừa `D-5`).

## 5. Data Model

- `ContextSource` interface: `ID() string`, `Priority() int`, `Deterministic() bool`, `Fetch(ctx, hints FlowContextHints) (FlowContextSection, error)`.
- `FlowContextSection`: `SourceType`, `Priority`, `SourceRef`, `Body`, `Omitted []string`, `Confidence`.
- `ContextSourceRegistry`: `map[ContextSourceID]ContextSource`; `Register`/`Resolve`/`Collect(ctx, enabledIDs, hints) ([]FlowContextSection, []string)`.
- `FlowContextPackage` (+): `Sections []FlowContextSection`; field cũ = projection; `PackageID` không đổi.
- State: package build một lần mỗi Plan run; lưu qua artifact/event gắn `workflow_run_id` + Plan `workflow_step_run_id` (không state song song).

## 6. Interfaces and Contracts

- Go interface: như §5 (pack không cấp implementation, chỉ chọn ID — mirror CP-42 T-2).
- Pack schema (+): `contexts.<name>.sources: [context-source-id, ...]` (giữ `ref` cũ hợp lệ).
- MCP source contract: adapter nhận driver-ref, gọi MCP đã support, trả `FlowContextSection` bounded + `SourceRef`; timeout qua `ctx`.
- Artifact/event: không đổi surface; `Sections` nằm trong payload package sẵn có.

## 7. Execution Flow

1. Plan step (`context.produce`) chạy: load catalog, `resolvePackageFeature(prompt)` → `feature_key`+confidence vào `hints`.
2. Lấy danh sách source enabled của flow (từ pack `contexts.sources`, default nếu vắng).
3. `registry.Collect(ctx, enabledIDs, hints)` → chạy từng source theo `Priority`, thu `[]FlowContextSection` + warnings; source lỗi → warning, bỏ qua.
4. Build `FlowContextPackage`: gán `Sections`, tính projection field cũ, `PackageID = f(run,step,feature)`.
5. `RenderFlowContextPackage` → prompt handoff cho node delegate (Task-169), không đổi chữ ký.

## 8. Failure and Edge Handling

- `F-1` Một source `Fetch` lỗi → append warning `"<id>: <err>"`, **không** fail run; các source khác vẫn chạy.
- `F-2` Source-id lạ trong pack → **lỗi load flow** rõ ràng (flow không chạy), không silent no-op.
- `F-3` Nguồn MCP timeout/down → section rỗng + warning; Plan step vẫn hoàn tất (`AC-9`).
- `F-4` Đăng ký source `Deterministic()==false` → `Register` trả lỗi (guard no-vector).
- `F-5` Ledger/nguồn thiếu dữ liệu → section rỗng + warning (degrade), không error (giữ Task-168 `T-4`).
- `F-6` Excerpt ngoài workspace / symlink escape → giữ omitted-reasons cũ (`outside_workspace`, `symlink_resolve_error`, `binary`, ...).

## 9. Security and Operational Concerns

- auth: nguồn ngoài chỉ dùng MCP đã được hệ thống support/connect; không endpoint/lệnh tùy ý (`D-5`).
- secrets: adapter MCP không log payload thô vào event; chỉ metadata + `SourceRef` (theo Task-170/168 bounded-logging).
- audit: package + Sections gắn `workflow_run_id`/`workflow_step_run_id`; giữ dòng `No vector retrieval used` trong render để audit.
- rollback: tắt binding `sources` → về default set (hành vi CP-41 hiện tại); disable một source → registry bỏ qua, không vỡ.

## 10. Risks and Trade-Offs

- `R-1` Refactor gây regression output package → phá renderer/audit. Mitigation: golden test byte-identical (CP-44 Task-192).
- `R-2` Nguồn MCP tại Plan-time gây latency/treo. Mitigation: timeout cứng + degrade-mềm; cache là tối ưu sau (`Q-2`).
- `R-3` Xói mòn determinism nếu source lén similarity search. Mitigation: `Deterministic()` guard, registry từ chối.
- `R-4` Coupling với SD-21. Mitigation: khóa interface `ContextSource` ổn định để Canonical Head cắm vào không sửa struct.
- `R-5` Thiếu SS user story cho extensibility (`Q-1`) → có thể lệch spec-authority. Mitigation: flag lên SS-14, không tự chế business rule.

## 11. Validation Strategy

- unit: Register dup/empty/non-deterministic; Resolve unknown fail-fast; Collect degrade-mềm + thứ tự ổn định; golden output byte-identical sau migrate; projection khớp Sections; `PackageID` bất biến; MCP source bounded + SourceRef + timeout degrade.
- integration: rag-harness chạy với default set giống CP-41 Scenario 1; flow bật `mcp.driver` → package có section MCP; source-id lạ → load fail.
- manual: kiểm prompt Plan có các section đúng nguồn + dòng no-vector; MCP tắt → degrade sạch.
- observability: log source enabled, priority, thời gian fetch mỗi source, source degrade/omit, kích thước package.

## 12. Traceability to Spec

- `AC-3` (ordered history, newest = truth) → source `feature.history` (bọc `HistorySlot`).
- `AC-8` (stale context flagged/de-prioritized) → `Confidence`/warnings trong section; nền cho SD-21 Canonical Head status.
- `AC-9` (non-fatal, retryable, never blocks) → `Collect` degrade-mềm; nguồn lỗi/MCP down không fail Plan.
- `AC-13` (tooling degrade tier khi thiếu) → nguồn MCP degrade khi không có/timeout.
- `AC-15` (local + Drive sync) → persistence không đổi; không thêm bảng.
- `US-9` (thêm loại context mới không đổi cấu trúc package) → `D-1` registry + `D-3` typed sections + `D-4` per-flow declared binding.
- `AC-16` (declarative, deterministic, degrade-mềm, nguồn ngoài chỉ integration đã support) → `D-1`/`D-2`/`D-4`/`D-5` + `F-1`/`F-3`/`F-4`.
- `US-1`/`US-2`/`US-5`/`US-7` → context giàu hơn, đúng nguồn theo `feature_key`, mở rộng được sang nguồn ngoài.
