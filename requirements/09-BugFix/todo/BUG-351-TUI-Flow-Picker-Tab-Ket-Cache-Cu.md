# BUG-351: Tab `/flow` kẹt cache cũ, chỉ Enter `/flow list` mới tươi

## Metadata

- Document ID: `BUG-351`
- Title: `Tab /flow kẹt cache cũ, chỉ Enter /flow list mới tươi`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-04`
- Last Updated: `2026-09-04`
- Feature Keys: `agent-flow-engine`
- Parent Documents: [CP-58-Test-Steps](../../07-Coding-Plan/done/CP-58-Test-Steps.md)
- Child Documents: `none`
- Related Documents: [Task-320](../../08-Task/inprogress/Task-320-Per-Node-Model-Tiering-For-Harness-Delegate-Nodes.md)
- Replaces: `none`
- Tags: `tui, flow-picker, stale-cache, mirror-sync-race`

## AI Quick View

### Summary

- Symptom (live 2026-09-04): sau restart runner có harness mới, picker `/flow` + Tab chỉ show 16 dòng (`workflows=15` + 1 builtin), thiếu Bug/Task/Cp Harness; gõ `/flow` + Enter (`/flow list`) thì đủ 19+1.
- Expected: Tab picker luôn thấy list tươi như Enter.
- Actual: Tab render từ cache session-start, kẹt cả session.
- Root cause (code): `collectSuggestions` → `filterFlowSuggestions(in, m.flowBuiltins, m.flowWorkflows, ...)` (`app.go:3048`) render thuần từ cache; `cmdMaybePrefetchFlows` (`app.go:6593`) skip fetch khi cache non-empty; prefetch silent lúc session start (`FlowListMsg silent=true`) race với runner `EnsureBuiltinFlowMirrorsWithStore` chạy goroutine nền (`cli/root.go:166`) — snapshot partial (12→15 giữa chừng sync, đủ 19 sau đó) kẹt luôn vì không bao giờ refetch.
- Đề xuất của reporter (fetch flow cùng providers/accounts lúc session start rồi cache) KHÔNG hết root cause — session start chính là lúc race xảy ra. Hướng đúng: mỗi lần mở picker Tab dispatch silent background refetch (hiện cache trước, merge khi `FlowListMsg` về, `Update` đã replace `flowBuiltins/flowWorkflows` ở `app.go:1233`); cân nhắc dedup in-flight + TTL nhẹ để khỏi spam Supabase.

### Current Ask

- Tab `/flow` sau fix: mở picker lúc nào cũng tự tươi (không cần Enter `/flow list`), vẫn hiện cache cũ trong lúc chờ fetch, không spam request.

## Evidence

- `tui.log` pid 15228: `Update FlowListMsg builtins=1 workflows=15` lúc `07:39:14` (+1s sau start); các session trước đó toàn `workflows=12`.
- `GET /client/workflows` sau khi sync xong: 19 rows gồm Bug/Task/Cp Harness + Smoke, tất cả `projectId` rỗng (không bị project-filter).
- `/flow` + Enter gọi `cmdListFlows()` (`app.go:4120`) fetch tươi → đủ; Tab chỉ đi `collectSuggestions` → cache.

## Fix (2026-09-04, done — code + unit, live-verify pending rebuild)

- `internal/tui/app/model.go`: thêm `flowListInflight` + `flowListFetchedAt`.
- `internal/tui/app/app.go` `cmdMaybePrefetchFlows`: picker đang mở + cache non-empty → dispatch silent background refresh (`cmdFetchFlows(true)`), dedup in-flight + bound `flowPickerRefreshInterval = 10s`; cache cũ vẫn hiển thị trong lúc chờ. Các điểm dispatch khác (`ConnectedMsg` prefetch, empty-cache refresh, Enter `/flow list`) cũng set flag.
- `FlowListMsg` handler: luôn clear `flowListInflight`; chỉ replace cache khi `CatalogErr == ""` (refresh lỗi không xóa list tốt), stamp `flowListFetchedAt` khi thành công.
- Tests (mới, `bug351_flow_picker_refresh_test.go`): stale-cache Tab dispatch đúng 1 refresh + merge tươi có `task-harness`; error giữ cache; interval bound không bắn per-keystroke.
- Verification: 3 tests mới + 54 tests flow/slash/launch liên quan PASS (`go test ./internal/tui/app/ -run 'TestFlow|TestSlash|TestLaunch'`). Live-verify: rebuild TUI, restart trong lúc runner đang sync → Tab lúc đầu thiếu, vài giây sau tự đủ không cần Enter.
- Không sửa pre-existing tests (oracle-rule); `TestFlowListMsg_ShowsBuiltinsWhenCatalogFails` vẫn green (message text on-error giữ nguyên, chỉ cache được giữ lại).
