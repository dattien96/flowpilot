# Task-192: Migrate Built-in Context Sources (Behavior-Preserving)

## Metadata

- Document ID: `Task-192`
- Title: `Migrate Built-in Context Sources (Behavior-Preserving)`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-08`
- Last Updated: `2026-07-08`
- Parent Documents: [CP-44: Pluggable Context Source Registry](../../07-Coding-Plan/todo/CP-44-Pluggable-Context-Source-Registry.md), [SD-17: Context And Regression Engine](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md)
- Child Documents: `None`
- Related Documents: [Task-191: Context Source Interface And Registry](Task-191-Context-Source-Interface-And-Registry.md), [Task-168: Flow Mode Context Package Contract](../done/Task-168-Flow-Mode-Context-Package-Contract.md), [Task-169: Plan To Coding Context Handoff](../done/Task-169-Plan-To-Coding-Context-Handoff.md)
- Replaces: `None`
- Tags: `context-regression-engine, flow-mode, context-source, refactor, regression-guard`

## AI Quick View

### Summary

- Chuyển 3 nguồn đang hardcode trong `BuildFlowContextPackage` — `feature.history` (CA + commits), `chat.summary`, `source.excerpt` — thành các `ContextSource` đăng ký trên registry Task-191.
- Refactor `BuildFlowContextPackage` để **iterate registry** thay vì gọi cứng, giữ bước feature-resolve làm tiền xử lý cấp `hints`.
- **Behavior-preserving tuyệt đối:** output package cho tập nguồn mặc định phải giống hệt (golden test) để không phá Task-169 (renderer) và Task-171 (audit draft).

### Current Ask

- Hoàn tất refactor behavior-preserving: built-in sources đã chạy qua registry và golden/regression tests đang khóa output cũ.

### Key Decisions

- `T-1` Feature-resolve **không** phải một source; nó chạy trước, cấp `feature_key`+confidence vào `FlowContextHints` cho các source dùng.
- `T-2` Mỗi source bọc đúng hàm cũ: `feature.history` → `HistorySlot`; `chat.summary` → `ChatSummarySlot`; `source.excerpt` → `readSourceExcerpts`. Không viết lại logic retrieval.
- `T-3` Thứ tự packing giữ đúng Task-168 `T-4`: source refs + confidence → newest truth (history) → issue/user ask → source excerpts → discussion → warnings/omitted.
- `T-4` Golden regression test là DOD chặn merge: output byte-identical với baseline hiện tại.

### Constraints

- Depends on Task-191.
- Không đổi `PackageID` (hàm `fcpPackageID(runID, stepID, featureKey)`) — audit `OriginalPackageID` và retry `OriginalPlanPackageID` phụ thuộc giá trị này.
- Giữ nguyên toàn bộ omitted-reasons của `readSourceExcerpts` (`outside_workspace`, `symlink_resolve_error`, `not_found`, `read_error`, `binary`, `total_cap_reached`).
- `loadFlowFeatureBlocks` trả `("","")` khi ledger lỗi — source phải degrade thành section rỗng, không error.

### Open Questions

- `Q-1` **(RESOLVED, 2026-07-08)** `feature.resolve` giữ là **tiền xử lý** (chạy trước, cấp `feature_key`+confidence vào `hints` cho mọi source), KHÔNG phải pseudo-source. Nếu CP-43 cần chèn Change Contract sớm sẽ xét lại khi tới CP-43.

### Source Refs

- `CP-44 P-2`, `DOD-2`
- `Task-168 T-2/T-4`, `Task-169`
- current code: `flow_context_package.go` (`BuildFlowContextPackage`, `resolvePackageFeature`, `loadFlowFeatureBlocks`, `readSourceExcerpts`, `fcpPackageID`, `RenderFlowContextPackage`)

## 1. Goal

Biến 3 nguồn hardcode thành registered source và refactor builder để chạy qua registry, với đảm bảo **output không đổi**. Đây là bước làm cho tính mở rộng của CP-44 trở nên thật mà không phá bất kỳ consumer nào.

## 2. Parent Links

- coding plan: `CP-44`
- tech design: `SD-17`
- system spec: `SS-13`
- specific upstream ids: `CP-44 P-2`, `CP-44 DOD-2`

## 3. Trigger

Sau Task-191, registry đã có nhưng rỗng. Cần chứng minh abstraction bằng cách migrate chính các nguồn hiện tại, đồng thời khóa regression trước khi mở cho nguồn mới (Task-194/195) và CP-43.

## 4. Exact Change

- `T-1` Viết 3 source implement `ContextSource` (file `context_sources_builtin.go`):
  - `featureHistorySource` — `Fetch` gọi `loadFlowFeatureBlocks` lấy `history`; `Body`=history block; `Priority` = vị trí "newest truth"; `Deterministic()=true`.
  - `chatSummarySource` — `Fetch` lấy `discussion`; `Priority` = vị trí "prior discussion".
  - `sourceExcerptSource` — `Fetch` gọi `readSourceExcerpts(workspace, hints.ChangedPaths+ExplicitSourcePaths)`; map excerpts → `Body`, giữ `Omitted`.
- `T-2` `NewDefaultContextSourceRegistry()` đăng ký 3 source trên với Priority khớp Task-168 T-4.
- `T-3` Refactor `BuildFlowContextPackage`:
  - Giữ nguyên: load catalog, `resolvePackageFeature` → set `FeatureKey`/`FeatureConfidence`/warnings, `fcpPackageID`.
  - Thay 3 lời gọi cứng bằng `registry.Collect(ctx, defaultSourceIDs, hints)`.
  - Gộp `[]FlowContextSection` về các field cũ (`HistoryBlock`, `DiscussionBlock`, `SourceExcerpts`, `Omitted`, `Warnings`) — projection (chi tiết struct đầy đủ ở Task-193; task này giữ field cũ, chỉ đổi nguồn dữ liệu).
- `T-4` Thêm golden test so sánh output builder trước/sau refactor trên nhiều trạng thái feature (verified/low/unresolved, có/không ledger, có/không excerpt).

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/runner/flow_context_package.go` (refactor builder)
  - mới: `apps/local-runner/internal/runner/context_sources_builtin.go`
  - `apps/local-runner/internal/runner/flow_context_package_test.go` (+ golden test)
- modules:
  - runner context assembly, feature catalog (qua bọc), change ledger (qua bọc)
- routes: none
- tables: none

## 6. Acceptance Check

- Output `FlowContextPackage` cho 4 trạng thái Task-168 (verified/ambiguous/missing/no-history) **giống hệt** baseline.
- `PackageID` không đổi cho cùng input.
- `RenderFlowContextPackage` vẫn phát dòng `No vector retrieval used` và các section như cũ.
- Ledger lỗi → history/discussion rỗng, package vẫn dựng, không error.
- Excerpt ngoài workspace vẫn cho omitted `outside_workspace`.

### 6.1 Test Items

- `TestBuildFlowContextPackageOutputUnchangedAfterRegistryRefactor` (golden, đa trạng thái)
- `TestFeatureHistorySourceDegradesOnLedgerError`
- `TestSourceExcerptSourcePreservesOmittedReasons`
- `TestPackageIDStableAfterRefactor`
- toàn bộ test Task-168 hiện có vẫn xanh

### 6.2 Definition of Done

- [x] `DOD-1` 3 source built-in tồn tại và đăng ký vào default registry.
- [x] `DOD-2` Builder chạy qua `Collect`, không còn 3 lời gọi retrieval cứng.
- [x] `DOD-3` Golden test chứng minh output byte-identical.
- [x] `DOD-4` `PackageID` + toàn bộ test Task-168/169 không regression.

## 7. Out of Scope

- Thêm field `Sections` vào struct package (Task-193).
- Khai báo nguồn per-flow (Task-194).
- Nguồn MCP (Task-195).

## 8. Completion Notes

- result: `Done` — `BuildFlowContextPackage` now resolves feature once, then runs `feature.history`, `chat.summary`, and `source.excerpt` through the registry path with golden coverage (`TestBuildFlowContextPackageOutputUnchangedAfterRegistryRefactor`).
- follow-ups: Task-193 phơi `Sections` ra ngoài + renderer generic. BUG-266 was discovered during the golden-test hardening and fixed separately.
- upstream docs updated: `CP-44` DOD/progress updated to reflect Task-192 shipped.
