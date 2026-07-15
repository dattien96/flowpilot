# Task-246: Producer Thật Cho source.excerpt Lúc Runtime

## Metadata

- Document ID: `Task-246`
- Title: `Source-Excerpt Runtime Hint Producers`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-15`
- Last Updated: `2026-07-15`
- Parent Documents: [CP-50: Context Source Completion](../../07-Coding-Plan/todo/CP-50-Context-Source-Completion.md) (P-3), [SD-22: Pluggable Context Source Registry](../../06-System-Tech-Design/SD-22-Pluggable-Context-Source-Registry.md), [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md) (AC-9)
- Child Documents: `None`
- Related Documents: [Task-243: Review Và Capture Các Context Artifact Source](../done/Task-243-Context-Artifact-Sources-Review-And-Capture.md) (G-3, Q-3), [Task-192: Migrate Built-in Context Sources](../done/Task-192-Migrate-Builtin-Context-Sources.md), [Task-168: Flow Mode Context Package Contract](../done/Task-168-Flow-Mode-Context-Package-Contract.md) (T-4 caps/degrade mà producer này nạp dữ liệu cho)
- Replaces: `None`
- Tags: `source-excerpt, context-source, git-diff, prompt-paths, local-runner`

## AI Quick View

### Summary

- `source.excerpt` hiện "chết lâm sàng": không production caller nào populate `FlowContextHints.ChangedPaths`/`ExplicitSourcePaths` (Task-243 G-3) — toàn bộ guard/cap của nó chỉ chạy trong test.
- Task này thêm 2 producer deterministic ở `behaviorContextProduce`: path nêu trong prompt (`ExplicitSourcePaths`) + diff chưa commit của workspace (`ChangedPaths`).
- Cố ý KHÔNG đặt trong `buildFlowContextPackage` — để ~30 test caller trực tiếp + golden test giữ nguyên hành vi.

### Current Ask

- Implement T-1..T-3; helper thuần + wire 3 dòng vào behavior; test unit + integration.

### Key Decisions

- `T-D1` Producer đặt ở caller (`behaviorContextProduce`), không ở builder (CP-50 Key Decision P-3).
- `T-D2` Diff lấy CẢ staged + untracked (`git diff --name-only HEAD` + `git ls-files --others --exclude-standard`), cap tổng 20 (CP-50 Q-1 default — được phép thu hẹp nếu package phình, ghi lại lý do).
- `T-D3` Mọi lỗi (không git, git thiếu, timeout) → nil im lặng; KHÔNG warning vào package (AC-9 — thiếu excerpt không phải lỗi).

### Constraints

- Không guard lại workspace-safety trong helper — `readSourceExcerpts` đã guard outside-workspace/symlink/binary/cap (4KB/file, 16KB tổng).
- Git exec bounded: `exec.CommandContext` timeout 3s/lệnh, tối đa 2 lệnh.
- Lọc `flowgate.IsDocOrAuditFile` khỏi cả 2 producer (mirror `InferFromDiff`).
- Không đổi struct hints/package; không thêm dependency.

### Open Questions

- `CP-50 Q-1` đã có default (lấy cả hai nguồn diff, cap 20) — chỉ mở lại nếu thực tế prompt phình to.

### Source Refs

- `CP-50 §4.3`. `Task-243` G-3, Q-3. `Task-168` T-4. `SS-14` AC-9.

## 1. Goal

Run flow live trên workspace có file đang sửa dở (hoặc prompt nêu path cụ thể) → `FlowContextPackage.SourceExcerpts` có excerpt thật của các file đó; workspace không phải git repo / prompt không nêu path → hành vi y hệt hiện tại.

## 2. Parent Links

- coding plan: `CP-50` P-3
- tech design: `SD-22` (bounded, deterministic, degrade)
- system spec: `SS-14` AC-9
- specific upstream ids: `CP-50 P-3`, `Task-243 Q-3`

## 3. Trigger

Task-243 `G-3`: producer duy nhất của `ChangedPaths:` trong production là `flowgate.TurnResult` (struct khác, không liên quan); `behaviorContextProduce` không set gì → source default thứ 3 không bao giờ đóng góp lúc runtime. Owner chốt (Q-3): không bỏ — làm producer.

## 4. Exact Change

- `T-1` **File mới `apps/local-runner/internal/runner/flow_context_hint_paths.go` — 2 helper thuần:**
  1. `extractPromptSourcePaths(prompt string) []string`:
     - Tách token theo whitespace; lột ký tự bọc `` ` ``, `'`, `"`, `(`, `)`, `,` ở 2 đầu token.
     - Token hợp lệ khi: chứa `/` hoặc `\`; KHÔNG có prefix `http://`/`https://`; `filepath.Ext(token) != ""`; không match `flowgate.IsDocOrAuditFile(token)`.
     - Chuẩn hóa `\` → `/`; dedup giữ thứ tự (dùng `fcpDedup` có sẵn); cap 8.
     - Thuần string — không chạm filesystem (validation tồn tại/an toàn là việc của `readSourceExcerpts`).
  2. `uncommittedChangedPaths(workspace string) []string`:
     - TRƯỚC KHI VIẾT: grep `exec.Command` trong `internal/flowgate/observe.go` — nếu đã có helper git-diff export được thì tái dùng; chỉ viết mới khi không có.
     - Chạy `git -C <workspace> diff --name-only HEAD` rồi `git -C <workspace> ls-files --others --exclude-standard`, mỗi lệnh `exec.CommandContext` timeout 3s.
     - Bất kỳ lỗi nào → return nil (1 dòng `log.Printf` debug là đủ, không spam).
     - Gộp 2 danh sách, lọc `flowgate.IsDocOrAuditFile`, dedup, cap 20.
     - Log 1 dòng khi tổng thời gian git exec >1s (CP-50 §8 monitoring).

- `T-2` **Wire vào `behaviorContextProduce` (`behavior_registry_builtin.go`, sau khi dựng `hints`):**
```go
// Task-246 (CP-50 P-3): derive excerpt hints deterministically at Plan-time —
// paths named in the prompt + the workspace's uncommitted diff. Both degrade
// to nil (non-git workspace, no path tokens) so fixture/behavior tests and
// the golden path are unaffected.
if in.WorkspaceCwd != "" {
    hints.ExplicitSourcePaths = extractPromptSourcePaths(in.Prompt)
    hints.ChangedPaths = uncommittedChangedPaths(in.WorkspaceCwd)
}
```
  KHÔNG đụng `buildFlowContextPackage`/`BuildFlowContextPackage*`.

- `T-3` **Tests:**
  - Unit extraction (bảng case): path backtick-quoted; path trong ngoặc/quote; path Windows-style (`apps\local-runner\foo.go` → chuẩn hóa `/`); URL bị loại; token có `/` nhưng không ext bị loại; `requirements/x.md` bị loại; >8 path bị cap; prompt không path → nil.
  - Unit git helper: `t.TempDir()` + `git init` + config user + commit 1 file; sửa file + thêm file untracked → trả đúng 2 path; dir không phải git → nil; file .md sửa dở → bị lọc.
  - Integration: `behaviorContextProduce` với workspace git-repo có file sửa dở + prompt nêu path → `pkg.SourceExcerpts` chứa các file đó; workspace temp thường + prompt không path → package không đổi so với trước task.

## 5. Touched Areas

- files: mới `internal/runner/flow_context_hint_paths.go` (+ test); sửa `internal/runner/behavior_registry_builtin.go`
- modules: runner context assembly (Plan step)
- routes: không
- tables: không

## 6. Acceptance Check

- Run flow live trên workspace có file sửa dở → prompt-log có `### Source: <path>` với excerpt tương ứng. *(skip — e2e live)*
- DONE Prompt nêu path tường minh trong workspace → file đó vào excerpt. *(extractPromptSourcePaths unit)*
- DONE Workspace không git / prompt không path → package y hệt trước (golden + behavior tests).
- DONE Doc/audit files không bao giờ vào excerpt qua 2 producer này.
- DONE Unit + integration T-3 pass; build/vet sạch.

## 7. Out of Scope

- Sửa `readSourceExcerpts` (guard/cap giữ nguyên).
- Excerpt cho Chat mode (chỉ Flow-mode Plan step).
- canonical.head / change.contract (Task-244/245/247).

## 8. Completion Notes

- result: `done` (2026-07-15).
- follow-ups: none.
- upstream docs updated: none.
