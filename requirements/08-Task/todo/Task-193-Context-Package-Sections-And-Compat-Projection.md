# Task-193: Context Package Sections And Compatibility Projection

## Metadata

- Document ID: `Task-193`
- Title: `Context Package Sections And Compatibility Projection`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-08`
- Last Updated: `2026-07-08`
- Parent Documents: [CP-44: Pluggable Context Source Registry](../../07-Coding-Plan/todo/CP-44-Pluggable-Context-Source-Registry.md), [SD-17: Context And Regression Engine](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md)
- Child Documents: `None`
- Related Documents: [Task-192: Migrate Built-in Context Sources](Task-192-Migrate-Builtin-Context-Sources.md), [Task-169: Plan To Coding Context Handoff](../done/Task-169-Plan-To-Coding-Context-Handoff.md), [Task-171: Audit Step Draft And Commit Prep](../done/Task-171-Audit-Step-Draft-And-Commit-Prep.md)
- Replaces: `None`
- Tags: `context-regression-engine, flow-mode, context-source, package-model, backward-compat`

## AI Quick View

### Summary

- Thêm `Sections []FlowContextSection` vào `FlowContextPackage` để package có thể chứa **N nguồn động** thay vì các block cố định.
- Giữ field cũ (`HistoryBlock`, `DiscussionBlock`, `SourceExcerpts`, `Warnings`, `Omitted`) làm **compatibility projection** suy ra từ `Sections`, để renderer (Task-169) và audit (Task-171) không đổi.
- `RenderFlowContextPackage` render thêm các section generic (nguồn mới) mà vẫn giữ nguyên các section quen thuộc + dòng `No vector retrieval used`.

### Current Ask

- Hoàn tất mở rộng package model: `Sections` + compatibility projection + generic renderer path đã ship mà không phá consumer cũ.

### Key Decisions

- `T-1` `Sections` là nguồn sự thật; field top-level cũ trở thành projection (đọc từ Sections theo `SourceType`).
- `T-2` JSON của `FlowContextPackage` giữ backward-compat: field cũ vẫn serialize như trước; `Sections` là field bổ sung.
- `T-3` `PackageID` vẫn tính từ `fcpPackageID(runID, stepID, featureKey)` — **không** hash `Sections` (giữ ổn định cho audit/retry).
- `T-4` Renderer render Sections theo `Priority`; các nguồn không biết trước render bằng template generic có heading = `SourceType` + SourceRef.

### Constraints

- Depends on Task-192.
- Consumer hiện có chỉ đọc `pkg.FeatureKey`, `pkg.SourceDocIDs`, `pkg.PackageID`, `pkg.FeatureConfidence`, và các block cũ — không được đổi ngữ nghĩa các field này.
- `BuildAuditDraft` đọc `pkg.SourceDocIDs[0]` và `pkg.FeatureKey` — projection phải giữ.
- Renderer output cho tập nguồn mặc định phải ổn định (bổ sung Task-192 golden test).

### Open Questions

- `Q-1` **(RESOLVED, 2026-07-08)** Giữ các field cũ (`HistoryBlock`/`DiscussionBlock`/`SourceExcerpts`) làm projection, **không deprecate** trong scope này. Deprecation là quyết định sau.

### Source Refs

- `CP-44 P-3`, `DOD-3`
- `Task-169` (renderer consumer), `Task-171` (`BuildAuditDraft` consumer)
- current code: `flow_context_package.go` (`FlowContextPackage`, `RenderFlowContextPackage`, `ComposeFlowCodingPrompt`), `flow_audit_draft.go` (`BuildAuditDraft`)

## 1. Goal

Cho `FlowContextPackage` khả năng chứa danh sách section động và render chúng, trong khi mọi consumer cũ tiếp tục chạy qua projection tương thích. Sau task này, thêm nguồn = section mới xuất hiện trong package + prompt, **không sửa struct**.

## 2. Parent Links

- coding plan: `CP-44`
- tech design: `SD-17`
- system spec: `SS-13`
- specific upstream ids: `CP-44 P-3`, `CP-44 DOD-3`

## 3. Trigger

Task-192 đã đưa dữ liệu qua `Collect` nhưng vẫn nén về field cũ. Để CP-43 (Canonical Head) và Task-195 (MCP source) hiện diện trong package/prompt, package phải phơi `Sections` ra ngoài và renderer phải biết render nguồn chưa biết trước.

## 4. Exact Change

- `T-1` Thêm `Sections []FlowContextSection` vào `FlowContextPackage`.
- `T-2` `BuildFlowContextPackage` gán `pkg.Sections = collectedSections`, rồi tính projection:
  - `HistoryBlock` = Body của section `feature.history`.
  - `DiscussionBlock` = Body của section `chat.summary`.
  - `SourceExcerpts`/`Omitted` = từ section `source.excerpt`.
  - `Warnings` = warnings từ resolve + `Collect`.
- `T-3` Cập nhật `RenderFlowContextPackage`:
  - Render các section theo `Priority`.
  - Section quen thuộc (`feature.history`/`chat.summary`/`source.excerpt`) giữ heading như hiện tại (giữ golden output).
  - Section lạ render generic: `### <SourceType>` + SourceRef + Body (bounded).
  - Giữ dòng `No vector retrieval used`.
- `T-4` Đảm bảo `ComposeFlowCodingPrompt` (Task-169) và `ComposeRetryPrompt` (Task-170) vẫn dùng renderer này không đổi chữ ký.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/runner/flow_context_package.go`
  - `apps/local-runner/internal/runner/flow_context_package_test.go`
- modules:
  - runner context assembly, prompt render
- routes: none
- tables: none (Sections nằm trong payload artifact/event sẵn có)

## 6. Acceptance Check

- Thêm một fake source qua registry → xuất hiện trong `pkg.Sections` và trong output renderer, **không sửa struct**.
- Projection field cũ khớp Sections tương ứng; consumer Task-169/171 chạy nguyên trạng.
- `PackageID` không đổi.
- Renderer cho tập nguồn mặc định vẫn khớp golden của Task-192.
- Serialize/deserialize JSON giữ field cũ + thêm `sections`.

### 6.1 Test Items

- `TestRegisterCustomSourceAppearsInPackageAndRender`
- `TestPackageProjectionMatchesSections`
- `TestRenderFlowContextPackageStillHasNoVectorLine`
- `TestPackageIDUnaffectedBySections`
- `TestAuditDraftStillReadsFeatureKeyAndSourceDocFromPackage`

### 6.2 Definition of Done

- [x] `DOD-1` `Sections` tồn tại; projection field cũ đúng.
- [x] `DOD-2` Renderer render Sections + render generic cho nguồn lạ; giữ dòng no-vector.
- [x] `DOD-3` Thêm nguồn = không sửa struct (test chứng minh).
- [x] `DOD-4` Không regression Task-169/170/171 (`go test ./internal/runner/...`).

## 7. Out of Scope

- Khai báo nguồn per-flow (Task-194).
- Nguồn MCP (Task-195).
- Deprecate field cũ.

## 8. Completion Notes

- result: `Done` — `FlowContextPackage` now carries `Sections`, legacy fields are projected from those sections, and the renderer/audit consumers still work unchanged.
- follow-ups: Task-194 cho phép flow chọn nguồn; CP-43 cắm Canonical Head như section.
- upstream docs updated: `CP-44` DOD/progress updated to reflect Task-193 shipped.
