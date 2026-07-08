# Task-191: Context Source Interface And Registry

## Metadata

- Document ID: `Task-191`
- Title: `Context Source Interface And Registry`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-08`
- Last Updated: `2026-07-09`
- Parent Documents: [CP-44: Pluggable Context Source Registry](../../07-Coding-Plan/done/CP-44-Pluggable-Context-Source-Registry.md), [SD-22: Pluggable Context Source Registry](../../06-System-Tech-Design/SD-22-Pluggable-Context-Source-Registry.md), [SD-17: Context And Regression Engine](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md), [SS-13: AI-Followable Document Contract](../../05-System-Specs/SS-13-AI-Followable-Document-Contract.md)
- Child Documents: `None`
- Related Documents: [CP-42: Flow Pack And Generic Node Behavior Refactor](../../07-Coding-Plan/done/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md), [Task-176: Node Behavior Registry And Dispatch](../done/Task-176-Node-Behavior-Registry-And-Dispatch.md), [Task-168: Flow Mode Context Package Contract](../done/Task-168-Flow-Mode-Context-Package-Contract.md)
- Replaces: `None`
- Tags: `context-regression-engine, flow-mode, context-source, registry, extensibility`

## AI Quick View

### Summary

- Định nghĩa interface `ContextSource` + `ContextSourceRegistry` — bản sao kiến trúc của `BehaviorRegistry` (Task-176) nhưng ở tầng nguồn context.
- Đây là slice nền của CP-44 `P-1`: chưa đổi hành vi builder, chỉ dựng contract + registry để các task sau (192-195) cắm nguồn vào.
- Registry chặn trùng ID, `Resolve` fail-fast khi unknown, và từ chối source non-deterministic (giữ bất biến no-vector của CP-41).
- `Collect` chạy các source enabled theo priority, degrade-mềm khi một source lỗi (thành warning, không fail run).

### Current Ask

- Tạo interface + registry + singleton default, kèm test, chưa migrate nguồn nào (nguồn thật ở Task-192).

### Key Decisions

- `T-1` `ContextSource` là interface Go thuần; pack/config chỉ chọn source theo ID, không cấp implementation (mirror Task-176 `T-2`).
- `T-2` `FlowContextSection` là đơn vị trả về chung của mọi source (body + `SourceRef` + omitted-reasons + confidence), dùng lại ở Task-193.
- `T-3` `Register` chặn trùng ID (error); `Resolve` unknown → error (fail-fast, không no-op) — mirror `BehaviorRegistry`.
- `T-4` Registry từ chối source có `Deterministic()==false` trong v1 (bảo vệ CP-41 no-vector).
- `T-5` `Collect` không bao giờ fail vì một source: lỗi source → append warning + bỏ qua section đó.

### Constraints

- Không đổi output của `BuildFlowContextPackage` trong task này (migrate là Task-192).
- Không thêm dependency vector/embedding/similarity.
- Mirror sát API `behavior_registry.go` để review dễ và nhất quán.

### Open Questions

- `Q-1` **(RESOLVED, 2026-07-08)** Đặt trong `internal/runner` (cạnh `behavior_registry.go`) để tái dùng `FlowContextPackage`/`FlowContextHints`, tránh import cycle.
- `Q-2` `Collect` nhận danh sách source-id enabled từ đâu ở task này? Tạm: nhận `[]string` tham số; nguồn khai báo per-flow là Task-194.

### Source Refs

- `CP-44 P-1`, `DOD-1`, `DOD-6`
- `Task-176 T-2/T-3` — mẫu `Register/Resolve/Dispatch`
- current code: `apps/local-runner/internal/runner/behavior_registry.go`, `behavior_registry_builtin.go`

## 1. Goal

Dựng contract + registry cho context source để CP-44 có nền cắm nguồn động. Kết thúc task: có interface `ContextSource`, type `FlowContextSection`, `ContextSourceRegistry` với `Register/Resolve/Collect`, một `DefaultContextSourceRegistry()` rỗng (chưa đăng ký nguồn nào), và test bao phủ các nhánh lỗi.

## 2. Parent Links

- coding plan: `CP-44`
- tech design: `SD-17`
- system spec: `SS-13`
- specific upstream ids: `CP-44 P-1`, `CP-44 DOD-1`, `CP-44 DOD-6`

## 3. Trigger

CP-44 cần một registry ổn định trước khi migrate 4 nguồn hardcode (Task-192) và trước khi CP-43 cắm Canonical Head như một source. Task-176 đã chứng minh mẫu registry này hoạt động cho node behavior; ta nhân bản cho context source.

## 4. Exact Change

- `T-1` Định nghĩa type nguồn — file mới `apps/local-runner/internal/runner/context_source_registry.go`.
  ```go
  type ContextSourceID string

  type FlowContextSection struct {
      SourceType string               // = ContextSource.ID(), vd "feature.history"
      Priority   int                  // để pack theo Task-168 T-4
      SourceRef  string               // bắt buộc: nguồn gốc excerpt (CP-41 T-3)
      Body       string               // nội dung bounded đã render
      Omitted    []string             // lý do bỏ (outside_workspace, too_large, ...)
      Confidence FlowContextConfidence
  }

  type ContextSource interface {
      ID() string
      Priority() int
      Deterministic() bool
      Fetch(ctx context.Context, hints FlowContextHints) (FlowContextSection, error)
  }
  ```

- `T-2` `ContextSourceRegistry` mirror `BehaviorRegistry`:
  - `Register(src ContextSource) error` — lỗi khi ID rỗng, khi trùng ID, và khi `src.Deterministic()==false` (T-4/`P-1`).
  - `Resolve(id string) (ContextSource, error)` — unknown → error, không no-op.
  - `Collect(ctx, enabledIDs []string, hints FlowContextHints) ([]FlowContextSection, []string)` — trả sections + warnings.

- `T-3` `Collect` semantics (edge-case cứng):
  - Chạy từng source theo thứ tự `enabledIDs`; ổn định.
  - Source `Fetch` trả error → **không** trả lỗi ra ngoài; append `"<id>: <err>"` vào warnings, bỏ qua section đó.
  - Section rỗng (Body=="" và không Omitted) → vẫn bỏ qua trong pack nhưng giữ SourceRef nếu có (tương thích Task-168 T-4 "giữ ref kể cả khi content omitted").
  - Sắp xếp kết quả theo `Priority` tăng dần, tie-break theo `SourceType` để deterministic.

- `T-4` `DefaultContextSourceRegistry()` — singleton `sync.Once` rỗng (chưa đăng ký nguồn), mirror `DefaultBehaviorRegistry()`; Task-192 sẽ thêm `NewDefaultContextSourceRegistry()` đăng ký nguồn built-in.

- `T-5` Không đụng `BuildFlowContextPackage` ở task này. Chỉ thêm file + test.

## 5. Touched Areas

- files:
  - mới: `apps/local-runner/internal/runner/context_source_registry.go`
  - mới: `apps/local-runner/internal/runner/context_source_registry_test.go`
- modules:
  - runner context assembly (chỉ thêm scaffolding)
- routes: none
- tables: none

## 6. Acceptance Check

- `Register` trùng ID trả error; ID rỗng trả error.
- `Register` source `Deterministic()==false` trả error (guard no-vector).
- `Resolve` unknown ID trả error (không trả nil/no-op).
- `Collect` với source lỗi → warning chứa ID, các source còn lại vẫn chạy, không panic.
- `Collect` sắp thứ tự theo Priority, tie-break theo SourceType, ổn định giữa các lần chạy.
- `BuildFlowContextPackage` chưa thay đổi (test hiện có của Task-168 vẫn xanh).

### 6.1 Test Items

- `TestContextSourceRegistryRegisterRejectsDuplicate`
- `TestContextSourceRegistryRegisterRejectsEmptyID`
- `TestContextSourceRegistryRegisterRejectsNonDeterministic`
- `TestContextSourceRegistryResolveUnknownFails`
- `TestContextSourceCollectDegradesOnSourceError`
- `TestContextSourceCollectStableOrderByPriorityThenID`

### 6.2 Definition of Done

- [x] `DOD-1` Interface + `FlowContextSection` + registry tồn tại.
- [x] `DOD-2` `Register/Resolve` mirror `BehaviorRegistry` (dup/empty/unknown/non-deterministic đều fail đúng).
- [x] `DOD-3` `Collect` degrade-mềm + thứ tự ổn định, có test.
- [x] `DOD-4` Không regression Task-168 (`go test ./internal/runner/ -run TestBuildFlowContextPackage` xanh).

## 7. Out of Scope

- Migrate 4 nguồn hardcode (Task-192).
- Sửa `FlowContextPackage` để chứa `Sections` (Task-193).
- Khai báo nguồn per-flow trong pack (Task-194).
- Nguồn MCP (Task-195).

## 8. Completion Notes

- result: `done` — 2026-07-08: `context_source_registry.go` + `context_source_registry_test.go` implemented exactly per T-1..T-5; all 6 test items pass; `go test ./internal/runner/ -run TestBuildFlowContextPackage` unaffected (Task-168 unchanged).
- follow-ups: Task-192 populates `NewDefaultContextSourceRegistry()` with built-in sources.
- follow-ups: Task-192 đăng ký nguồn built-in vào registry này.
- upstream docs updated: `TBD`
