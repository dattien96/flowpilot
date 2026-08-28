# Task-244: Canonical Head Thành First-Class Context Source (Default Set)

## Metadata

- Document ID: `Task-244`
- Title: `Canonical Head First-Class Context Source`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-15`
- Last Updated: `2026-07-15`
- Parent Documents: [CP-50: Context Source Completion](../../07-Coding-Plan/done/CP-50-Context-Source-Completion.md) (P-1), [SD-21: Change Contract And Canonical Intent Signature](../../06-System-Tech-Design/SD-21-Change-Contract-And-Canonical-Intent-Signature.md) (D-3), [SD-22: Pluggable Context Source Registry](../../06-System-Tech-Design/SD-22-Pluggable-Context-Source-Registry.md), [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md) (AC-3, AC-9)
- Child Documents: `None`
- Related Documents: [Task-243: Review Và Capture Các Context Artifact Source](./Task-243-Context-Artifact-Sources-Review-And-Capture.md) (G-1/G-4/G-5/G-8, Q-1), [Task-188: Canonical-Head Packing And Admin](../inprogress/Task-188-Canonical-Head-Packing-And-Admin.md) (T-1 prepend bị task này supersede), [Task-192: Migrate Built-in Context Sources](./Task-192-Migrate-Builtin-Context-Sources.md), [CP-43: Change Contract And Canonical Intent Signature](../../07-Coding-Plan/done/CP-43-Change-Contract-And-Canonical-Intent-Signature.md) (§4.5 sửa wording), [BUG-268](../../09-BugFix/done/BUG-268-Flow-Coding-Prompt-Duplicates-Feature-History.md) (bẫy duplicate phải né)
- Replaces: `None`
- Tags: `canonical-head, context-source, registry, default-set, local-runner, desktop-flowpilot`

## AI Quick View

### Summary

- Tách Canonical Head khỏi prepend trong `featureHistorySource` (Task-188 T-1) thành source đăng ký thật id `canonical.head`, priority 1, nằm trong `defaultContextSourceIDs`.
- Renderer cho block Head dẫn đầu package (trước `### Change History`), heading `##` ngang hàng đúng template CP-43 §4.5.
- Đóng luôn 3 micro-finding của Task-243: `G-4` (warning bị suppress), `G-5` (heading lồng sai cấp), `G-8` (path separator trong SourceRef).

### Current Ask

- Implement đúng T-1..T-6 dưới đây; code skeleton đã được rehearse trên codebase 2026-07-15 — bám theo, không tự chế biến lại kiến trúc.

### Key Decisions

- `T-D1` Head là source **độc lập trong default set** (owner chốt, Task-243 Q-1) — KHÔNG giữ prepend song song trong feature.history (né duplicate kiểu BUG-268).
- `T-D2` Priority 1 (< feature.history 2) để `Collect` sort Head lên đầu `Sections`; render đặt Head trước mọi block khác bằng special-case theo tên, KHÔNG đổi thứ tự render của 3 block cũ (golden phải nguyên trạng).

### Constraints

- `TestBuildFlowContextPackageOutputUnchangedAfterRegistryRefactor` phải pass **không sửa expectation** (fixture không có Head → section rỗng → render y hệt).
- Không đụng chuỗi `filterString(sourceIDs, ...)` trong `startInlineEntryChain` (`flow_executor.go`) — đó là filter cho runtime-target-note sources (mcp.driver/jira/firebase), `canonical.head` là source collect thật.
- Thiếu/hỏng Head → section rỗng, KHÔNG error, KHÔNG warning (SS-14 AC-9).
- UI chỉ sửa `apps/desktop-flowpilot` (`apps/admin-web` đã deprecated).

### Open Questions

- Không — mọi quyết định đã chốt ở CP-50/Task-243.

### Source Refs

- `CP-50 §4.1`. `Task-243` G-1/G-4/G-5/G-8, Q-1. `SD-21` D-3. `SD-22` D-3/D-5. `CP-44` DOD-7.

## 1. Goal

`canonical.head` là source đăng ký trên `ContextSourceRegistry`, chạy mặc định trong mọi Plan-step build, render dẫn đầu package; `feature.history` trở về đúng vai trò history thuần (warning semantics khôi phục); flow giới hạn source subset vẫn chọn được Head độc lập qua CP-45 artifact instance.

## 2. Parent Links

- coding plan: `CP-50` P-1
- tech design: `SD-21` D-3 (Head-first packing), `SD-22` D-3/D-5 (section contract, degrade)
- system spec: `SS-14` AC-3, AC-9
- specific upstream ids: `CP-50 P-1`, `Task-243 Q-1`, `CP-44 DOD-7`

## 3. Trigger

Task-243 `G-1`: CP-44 hứa `canonical.head` là source đăng ký nhưng Task-188 ship dạng prepend bên trong `feature.history` — hệ quả là không chọn được head-only, mất Head khi loại feature.history khỏi subset, UI không list được. Owner chốt (Q-1, 2026-07-15): nâng thành first-class + vào default set.

## 4. Exact Change

- `T-1` **File mới `apps/local-runner/internal/runner/context_source_canonical_head.go`:**

```go
package runner

import (
    "context"
    "path/filepath"
    "strings"

    "flowpilot-runner/internal/changecontract"
)

// ContextSourceCanonicalHead — Task-244 (CP-50 P-1, CP-43 P-5, CP-44 DOD-7).
const ContextSourceCanonicalHead ContextSourceID = "canonical.head"

type canonicalHeadSource struct{ priority int }

func (s *canonicalHeadSource) ID() string          { return string(ContextSourceCanonicalHead) }
func (s *canonicalHeadSource) Priority() int       { return s.priority }
func (s *canonicalHeadSource) Deterministic() bool { return true }

func (s *canonicalHeadSource) Fetch(_ context.Context, hints FlowContextHints) (FlowContextSection, error) {
    section := FlowContextSection{SourceType: s.ID(), Priority: s.priority}
    // Gate giống featureHistorySource: chỉ tra khi feature verified —
    // hint không chắc chắn không bao giờ được inject Head của feature khác.
    if hints.FeatureConfidence != ConfidenceVerified || hints.FeatureKey == "" {
        return section, nil
    }
    head, found, err := changecontract.LoadHead(hints.Workspace, hints.FeatureKey)
    if err != nil || !found {
        return section, nil // degrade AC-9: không error, không warning
    }
    // G-8 fix: filepath.ToSlash cho SourceRef nhất quán trên Windows.
    section.SourceRef = filepath.ToSlash(filepath.Join(hints.Workspace, ".flowpilot", "canonical", hints.FeatureKey+".json"))
    section.Body = strings.TrimSpace(changecontract.RenderHeadBlock(head))
    return section, nil
}
```

- `T-2` **`context_sources_builtin.go` — 3 chỗ:**
  1. `defaultContextSourceIDs`: thêm `string(ContextSourceCanonicalHead)` lên đầu slice; cập nhật comment (3 id gốc reproduce CP-41; canonical.head vào theo Task-243 Q-1 — rỗng khi feature chưa có Head nên fixture/flow cũ không đổi).
  2. `registerBuiltinContextSources`: thêm `mustRegisterContextSource(r, &canonicalHeadSource{priority: 1})` trước dòng featureHistory (priority 2).
  3. `featureHistorySource.Fetch`: **xóa nguyên block** prepend Head (block comment "Task-188 (CP-43 P-5, SD-21 D-3)…" + `if head, found, err := changecontract.LoadHead(...) {...}`), thay bằng comment ngắn "Task-244: Head là source riêng (canonical.head, priority 1), không còn prepend ở đây — warning semantics pre-CP-43 khôi phục". Xóa import `flowpilot-runner/internal/changecontract` (verify bằng `go build`).

- `T-3` **Renderer `flow_context_package.go` — 2 chỗ:**
  1. Trong `RenderFlowContextPackage`, ngay sau `sb.WriteString("- **No vector retrieval used**\n")`, trước `if pkg.HistoryBlock != ""`:
  ```go
  // Task-244 (SD-21 D-3): Canonical Head dẫn đầu — current truth + rejected
  // dead-ends trước mọi raw history. Body tự mang heading "## Canonical state"
  // (sibling của history theo template CP-43 §4.5 — đóng G-5), viết verbatim
  // ở đây và skip ở renderGenericSections bên dưới.
  for _, s := range pkg.Sections {
      if ContextSourceID(s.SourceType) == ContextSourceCanonicalHead && strings.TrimSpace(s.Body) != "" {
          sb.WriteString("\n" + strings.TrimSpace(s.Body) + "\n")
      }
  }
  ```
  2. `renderGenericSections`: thêm `ContextSourceCanonicalHead` vào case skip (`case ContextSourceFeatureHistory, ContextSourceChatSummary, ContextSourceSourceExcerpt, ContextSourceCanonicalHead:`). Quên chỗ này → Head render 2 lần.

- `T-4` **Viết lại `context_source_canonical_head_test.go`** (file có sẵn 2 test của Task-188):
  - THAY `TestFeatureHistorySourcePrependsCanonicalHead` → `TestFeatureHistorySourceNoLongerPrependsHead`: SaveHead + ledger rỗng → feature.history Body rỗng VÀ warning "no change history found" fire (G-4 khôi phục).
  - GIỮ `TestFeatureHistorySourceNoHeadFallsBackToPriorBehavior` nguyên trạng.
  - MỚI: `TestCanonicalHeadSourceFetchesHeadBlock` (SaveHead → Fetch → Body chứa `Canonical state of` + decision reject; SourceRef khác rỗng); `TestCanonicalHeadSourceNoHeadDegrades` (Body rỗng, Warnings nil, err nil); `TestCanonicalHeadSourceUnverifiedFeatureEmpty` (`ConfidenceLow` → rỗng); `TestCanonicalHeadInDefaultSetAndRegistered`; `TestRenderFlowContextPackageLeadsWithCanonicalHead` (dùng `fcpFixture(t)` có sẵn, SaveHead cho `agent-flow-engine` → render có `## Canonical state` đứng TRƯỚC `### Change History`).

- `T-5` **Desktop UI `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx`:** thêm `{ id: "canonical.head", label: "Canonical Head" }` vào đầu mảng `contextSourceOptions` (~dòng 68). `npx tsc --noEmit` phải sạch.

- `T-6` **Sync docs:** CP-43 §4.5 (wording prepend → source riêng, trỏ Task-244); CP-43 §10 P-5 note; Task-188 §6 T-1 note "superseded by Task-244"; CP-44 §4 P-1 note "(canonical.head landed via CP-50 P-1/Task-244)".

## 5. Touched Areas

- files: mới `internal/runner/context_source_canonical_head.go`; sửa `internal/runner/{context_sources_builtin,flow_context_package}.go`, `internal/runner/context_source_canonical_head_test.go`, `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx`; docs CP-43/CP-44/Task-188
- modules: runner context assembly, desktop settings UI
- routes: không
- tables: không

## 6. Acceptance Check

- DONE `canonical.head` đăng ký priority 1, trong `defaultContextSourceIDs`; `NewDefaultContextSourceRegistry().Resolve("canonical.head")` ok; CP-45 instance khai `sources: ["canonical.head"]` validate pass.
- DONE `feature.history` không còn prepend Head; ledger rỗng + có Head → warning "no change history found" vẫn fire.
- DONE Render: `## Canonical state` đứng trước `### Change History`; không xuất hiện lần 2 ở generic pass.
- DONE `TestBuildFlowContextPackageOutputUnchangedAfterRegistryRefactor` pass không sửa expectation.
- DONE 6 test T-4 pass; `go build ./...`, `go vet ./internal/runner/...`, `go test ./internal/runner/ -count=1` không fail mới so với baseline flake; `npx tsc --noEmit` sạch.
- DONE Docs T-6 / task status done.

## 7. Out of Scope

- Chat-mode injection (Task-245), source.excerpt producers (Task-246), change.contract (Task-247).
- Budget-pressure demote/drop-log raw history (Task-188 T-2 residual — vẫn mở).
- Sửa golden expectation dưới mọi hình thức.

## 8. Completion Notes

- result: `done` (2026-07-15).
- DOD unit: DONE (live E2E skipped).
- follow-ups: none for P-1.
- upstream docs updated: CP-50 land via this task.
