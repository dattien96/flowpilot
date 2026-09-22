# CP-81 — Manual Test Guide (Shared Runner Lifecycle)

## Metadata

- Document ID: `CP-81-TEST-STEPS`
- Title: `CP-81 Manual Test Guide — Shared Runner Lifecycle, Close UX, Restart, Stale Replacement`
- Phase: `verification`
- Status: `ready`
- Owner: `FlowPilot`
- Created: `2026-09-22`
- Last Updated: `2026-09-22`
- Parent Documents: [CP-81: Shared Runner Lifecycle](./CP-81-Shared-Runner-Lifecycle.md)
- Related Documents: [SS-24](../../05-System-Specs/SS-24-Shared-Runner-Lifecycle.md), [SD-28](../../06-System-Tech-Design/SD-28-Shared-Runner-Lifecycle.md), `Task-413..Task-420`, `BUG-240`, `BUG-328`, `CA-445`, `CA-474`, `CA-901`, `CA-911`, `CA-913`
- Tags: `runner-lifecycle, verification, manual-test, windows, tui, desktop, supervisor`

## AI Quick View

- **What:** Manual/live verification checklist for the shared runner lifecycle contract.
- **Why:** The bug class spans process ownership, HTTP lifecycle, Electron quit behavior, Windows Job Objects, and durable AI work — unit tests alone cannot prove the UX.
- **Primary machine:** Windows dev machine using `just chat-dev` and `just desktop-dev`.
- **Key invariant:** a runner is never accidentally killed by one client, and it is never leaked forever after the last client disappears.

---

## 0. Chuẩn bị

| # | Hạng mục | Cách kiểm tra |
|---|---|---|
| P1 | Repo đã build code CP-81 | `cd C:\working\flowpilot && go build ./apps/local-runner/...` |
| P2 | Không còn runner cũ trên port 4317 | `netstat -ano | findstr :4317`; nếu process lạ chiếm port, dùng test `P4` để kiểm tra không bị kill mù |
| P3 | Log mở sẵn | `tail -f "$LOCALAPPDATA/flowpilot/logs/cli-runner.log"` hoặc path log tương đương |
| P4 | Kiểm chứng unknown port process | Nếu port 4317 bị process không phải FlowPilot chiếm, client phải báo port conflict, không kill process |
| P5 | Test project | Dùng `C:\working\DnStudio` hoặc repo scratch hợp lệ |

---

## S. Smoke Test 10 Phút

1. Chạy `just chat-dev C:\working\DnStudio` → TUI attach và status hiển thị runner `ready`.
2. Mở Desktop app cùng lúc → `GET /system/lifecycle` hoặc status UI hiển thị 2 clients (`tui`, `desktop`).
3. Đóng TUI bằng `/exit` → chọn `Close TUI only` → Desktop vẫn hoạt động; runner vẫn online.
4. Đóng Desktop bằng X → chọn `Close Desktop only` → runner vào `idle_grace`.
5. Trong vòng 30 giây, mở lại TUI → idle shutdown bị hủy; runner giữ nguyên `runnerInstanceId`.
6. Đóng hết clients và không mở lại → sau ~30 giây runner exit; port 4317 được giải phóng.

---

## A. Lease Attach / Heartbeat / Release

| # | Bước thực hiện | Kết quả mong đợi |
|---|---|---|
| A1 | Mở TUI | `POST /system/clients/register` trả `leaseId`, `leaseToken`, `generation`; log có lease registered với `kind=tui`, PID, client instance |
| A2 | Quan sát 20s | Heartbeat mỗi ~5s; `expiresAt` liên tục được refresh; runner không vào `idle_grace` |
| A3 | `/exit` khi TUI là client duy nhất và không có work | `POST .../release` thành công; runner chuyển `idle_grace` với `shutdownAt ≈ now+30s` |
| A4 | Trong grace, mở lại TUI | Register mới cancel countdown ngay; phase quay về `ready` |
| A5 | Gọi release lặp lại bằng tay nếu cần | Idempotent; không panic, không double cleanup |

---

## B. Shared Close UX — TUI + Desktop

| # | Bước thực hiện | Kết quả mong đợi |
|---|---|---|
| B1 | Mở TUI + Desktop, không có work; `/exit` trong TUI | Dialog có đúng 3 lựa chọn: `Cancel`, `Close TUI only`, `Turn off FlowPilot`; hiển thị client/work inventory |
| B2 | Chọn `Cancel` | TUI không thoát; lease vẫn sống |
| B3 | Chọn `Close TUI only` | TUI release lease và thoát; Desktop giữ session; runner vẫn `ready` |
| B4 | Mở lại TUI, sau đó đóng Desktop bằng X | Dialog tương đương: `Cancel`, `Close Desktop only`, `Turn off FlowPilot` |
| B5 | Chọn `Close Desktop only` | Desktop release lease và quit; TUI tiếp tục dùng runner |
| B6 | Trong dialog, chọn `Turn off FlowPilot` | Confirmation/warning hiển thị clients + workloads; sau confirm runner force-stop và client còn lại nhận notice rồi đóng |
| B7 | Bấm nút `Turn off system` trong Desktop | Cùng một IPC/confirmation path như đóng window; không được gọi endpoint riêng khác semantics |

---

## C. Crash / Terminal Close / Task Manager Kill

| # | Bước thực hiện | Kết quả mong đợi |
|---|---|---|
| C1 | Mở TUI + Desktop; Task Manager kill TUI | Runner không chết ngay; lease TUI expire sau TTL (~15s); Desktop vẫn online |
| C2 | Kill client cuối cùng bất thường | Sau TTL + 30s idle grace, runner exit; không còn `flowpilot`/`runner`/`go run` compiled orphan |
| C3 | Đóng terminal đang chạy TUI | Tương đương client crash: lease expire, không giết runner nếu Desktop còn lease |
| C4 | Task Manager kill runner | TUI/Desktop phát hiện heartbeat/health loss, show `Runner stopped unexpectedly`, rồi đóng client |
| C5 | Runner terminal Ctrl+C khi idle | Runner bounded cleanup và exit; clients nhận unplanned loss notice nếu đang kết nối |
| C6 | Runner terminal Ctrl+C khi có client/work | Lần đầu in warning inventory + yêu cầu Ctrl+C lần 2 trong 5s; không force ngay |
| C7 | Ctrl+C lần 2 trong confirm window | Runner stop-all, cleanup, exit; clients nhận notice/đóng đúng |

---

## D. Active Work Protection

| # | Bước thực hiện | Kết quả mong đợi |
|---|---|---|
| D1 | Trong TUI bắt đầu một chat turn/scaffold đang chạy | `GET /system/lifecycle` shows workload item (`turn`/`scaffold`) with run/project/provider |
| D2 | Đóng client cuối trong lúc work active | Runner không vào `idle_grace`; phase `orphaned_work`; work tiếp tục hoặc được settle theo contract |
| D3 | Work kết thúc sau khi client đã đóng | Runner chuyển `idle_grace` và exit sau 30s |
| D4 | `Turn off FlowPilot` khi work active | Dialog warn lists run/scaffold/provider; confirm → runner cancel work, status terminal/cancelled, no stuck `running` |
| D5 | Trong forced shutdown, inspect provider process | `devin acp`, `claude`, `codex`, `grok`, `opencode` children được cleanup; không còn process con orphan |
| D6 | Sau shutdown, inspect run metadata | Không có run/turn/scaffold nào còn `running` thiếu evidence; cancelled/failed/recoverable được ghi rõ |

---

## E. Planned Restart / Reconnect

| # | Bước thực hiện | Kết quả mong đợi |
|---|---|---|
| E1 | TUI + Desktop đang online; chọn `Restart Runner` | Nếu busy/shared, dialog warn + confirm; heartbeat trả `draining_restart` + `restartId` |
| E2 | Trong restart | TUI/Desktop hiển thị `Runner restarting…` / reconnecting state; không quit |
| E3 | Runner mới online | `runnerInstanceId` khác, generation mới; clients re-register và restore state |
| E4 | Giả lập restart timeout | Sau deadline, client show restart failed/closed notice rồi exit; không spin vô hạn |
| E5 | Restart supervised stack | Supervisor chỉ restart runner theo fenced command; không kill nhầm new generation |
| E6 | Xoá/sửa stale `supervisor.cmd` | Supervisor ignore command thiếu/mismatch `runnerInstanceId` hoặc expired `restartId` |

---

## F. Stale Build / Update Pending

| # | Bước thực hiện | Kết quả mong đợi |
|---|---|---|
| F1 | Runner cũ đang idle; start client với binary mới | `runnerboot` classify `idle_stale`; fenced shutdown old instance; spawn/reconnect expected build |
| F2 | Runner cũ đang có client/work; start client binary mới | Không kill runner; status `Update pending`; attach chỉ nếu protocol compatible |
| F3 | Runner cũ thiếu lifecycle fields | classify `legacy_unknown`; client yêu cầu confirmation thay vì kill mù |
| F4 | Hai clients cùng start khi runner stale | boot lock serialize; chỉ một replacement/spawn winner |
| F5 | Source thay đổi trong phiên `go run` | Build ID thể hiện mismatch khi reconnect/launch mới; không reuse mù runner cũ nếu đã idle |

---

## G. Workspace / Port Conflicts

| # | Bước thực hiện | Kết quả mong đợi |
|---|---|---|
| G1 | Runner đang serve workspace A; start TUI cho workspace B | Client detect cwd/workspace mismatch và show rõ; không attach như compatible |
| G2 | Port 4317 bị process không phải FlowPilot chiếm | Client báo `port conflict/unknown process`; không gọi kill |
| G3 | Port 4317 là FlowPilot runner nhưng version/protocol không compatible | Typed `protocol_incompatible`; UX hướng dẫn restart/replace |
| G4 | Runner đúng workspace + compatible | Reuse attach; không spawn process mới |

---

## H. Supervisor / Dev Stack

| # | Bước thực hiện | Kết quả mong đợi |
|---|---|---|
| H1 | Chạy `just dev` hoặc stack supervisor tương đương | Runner start với `lifecycleMode=supervised`; supervisor không xuất hiện như permanent user lease |
| H2 | Sau web/desktop boot | Client lease xuất hiện bình thường; boot grace không shutdown sớm |
| H3 | Explicit stack shutdown | Supervisor force-shutdown runner + web/desktop; Windows tree kill xử lý cả `go.exe` wrapper và compiled child |
| H4 | Runner unexpected exit | Supervisor không ghost-respawn phía sau clients; log reason và exit/policy đúng |
| H5 | Close supervisor terminal | Không còn runner/web/desktop orphan sau bounded cleanup hoặc lease expiry |

---

## I. Logging / Telemetry

| # | Bước thực hiện | Kết quả mong đợi |
|---|---|---|
| I1 | Mọi `/system/shutdown` và `/system/restart` | Log có `remote`, `ua`, `xclient`, requester lease, requester PID/process name, `runnerInstanceId`, phase, reason |
| I2 | Idle shutdown | Log show `zero leases`, `no active work`, `shutdownAt`, `reason=idle_timeout` |
| I3 | Force shutdown | Log lists stopped workload IDs and uncertain outcomes |
| I4 | Restart | Log includes `restartId`, handoff mode, old/new instance IDs |
| I5 | Token redaction | `leaseToken`/`confirmToken` không bị log raw |

---

## J. Regression Guards

| # | Bước thực hiện | Kết quả mong đợi |
|---|---|---|
| J1 | `go test -count=1 ./internal/lifecycle/... ./internal/cli/... ./internal/runner/... ./internal/tui/...` | PASS |
| J2 | `go test ./...` trong `apps/local-runner` | PASS |
| J3 | Desktop `npm run typecheck` / build / focused tests | PASS |
| J4 | Existing provider suites | Không regression Claude/Codex/Gemini/Grok/OpenCode/Devin |
| J5 | `quit_kills_reused_runner_test.go` semantics | Updated only under CP-81 supersession: client-only close must NOT shutdown shared runner |
| J6 | `sysprocattr_windows.go` behavior | TUI process death no longer kills runner via Job Object; process-level test proves cleanup by lease expiry |

---

## K. Sign-off Checklist

- [ ] TUI + Desktop can coexist on one runner.
- [ ] Closing one client never kills the other client's runner.
- [ ] Last client normal close → 30-second idle shutdown.
- [ ] Last client crash → TTL expiry then 30-second idle shutdown.
- [ ] Active work prevents idle shutdown and is durably stopped on force.
- [ ] `Turn off FlowPilot` is warn-then-force.
- [ ] Planned restart reconnects; unplanned runner death closes clients with notice.
- [ ] Stale runner auto-replaces only when idle.
- [ ] No unknown process is killed for port occupancy.
- [ ] No orphaned Go wrapper/compiled runner remains after shutdown.
- [ ] Logs identify requester and lifecycle reason for every destructive event.
