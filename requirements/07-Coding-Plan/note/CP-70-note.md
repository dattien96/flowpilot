# CP-70 Bản Ghi Chú: Tích Hợp Devin CLI & Danh Sách Bài Học Xương Máu Từ Opencode

> **Dành cho Tech Lead / Human Operator:** Tài liệu này ghi lại toàn bộ bối cảnh kỹ thuật, cơ chế kết nối và **danh sách tất cả các vấn đề (Issues/Pitfalls) cần xử lý khi adapt một AI provider mới**, được đúc kết trực tiếp từ đợt triển khai Opencode ([CP-57](file:///Users/tiendat/Desktop/flowpilot/flowpilot/requirements/07-Coding-Plan/done/CP-57-Opencode-Provider-Integration.md)) và tài liệu hướng dẫn tổng quát [CP-Guide-Add-New-Provider-Checklist](file:///Users/tiendat/Desktop/flowpilot/flowpilot/requirements/07-Coding-Plan/CP-Guide-Add-New-Provider-Checklist.md).

---

## 1. Devin CLI Có Thể Làm Adapter Trong FlowPilot Không?

**Câu trả lời là CÓ và là ứng viên tương thích hoàn hảo nhất (100% Native Fit).**

### 1.1 Cơ sở kỹ thuật: Chuẩn ACP (Agent Client Protocol)
- **Devin CLI** (`devin`, Cognition AI) cung cấp lệnh:
  ```bash
  devin acp
  ```
- Đây là chế độ chạy server nền thông qua giao thức **Agent Client Protocol (ACP)** dùng **JSON-RPC 2.0** trên `stdin`/`stdout`.
- Trong FlowPilot:
  - `grok` sử dụng `grok acp` ([`grok_acp.go`](file:///Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/grok_acp.go)).
  - `opencode` sử dụng `opencode acp` ([`opencode_acp.go`](file:///Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/opencode_acp.go)).
  - `gemini` cũng có transport ACP riêng (`gemini_acp_transport.go`).
- Vì Cognition chính là bên tích cực khởi xướng và chuẩn hóa ACP (`agentclientprotocol.com`), việc tích hợp Devin vào FlowPilot hoàn toàn tuân thủ theo chuẩn ACP đã có sẵn trong repo mà không cần viết các parser giả lập terminal (PTY) phức tạp như Claude Code.

### 1.2 Chế độ One-shot Summarizer
- FlowPilot cần một chế độ headless nhanh, rẻ để tóm tắt tiêu đề chat, tổng kết flow run:
  - Grok có `grok run`
  - Opencode có `opencode run --format json`
  - Devin có **`devin -p "<prompt>"`** (print-and-exit; flag `--json` **chưa** được docs xác nhận — probe Task-400) và đặc biệt **`devin acp --agent-type summarizer`** — agent chuyên dụng không tools, output text thuần và persist vào `~/.local/share/devin/summaries/<session_id>.md` (live-verified `devin acp --help` trên 3000.10.31).
  - Lưu ý: `devin -p` fail trong untrusted workspace nếu không truyền `--respect-workspace-trust false`.

---

## 2. Toàn Bộ Danh Sách Các Issue Cần Xử Lý Khi Adapt Provider Mới (Kinh Nghiệm Từ Opencode)

Khi làm [CP-57 (Opencode)](file:///Users/tiendat/Desktop/flowpilot/flowpilot/requirements/07-Coding-Plan/done/CP-57-Opencode-Provider-Integration.md), FlowPilot đã trải qua hàng loạt bug thực tế từ giai đoạn tích hợp đến nghiệm thu. Khi làm Devin CLI, **chúng ta phải đưa các giải pháp này vào thiết kế ngay từ Day 1**:

### Issue 1: [BUG-331](file:///Users/tiendat/Desktop/flowpilot/flowpilot/requirements/09-BugFix/done/BUG-331.md) / CA-690 — YOLO OFF nhưng CLI mặc định Allow-All (Bỏ qua xin duyệt quyền)
- **Hiện tượng**: Khi người dùng tắt YOLO (`/yolo off`), mong muốn mỗi khi AI sửa file hoặc chạy bash thì phải hiện modal/card duyệt (`permission_required`). Tuy nhiên, nhiều CLI agent khi chạy ACP mặc định coi client là tự động cho phép mọi thứ, dẫn tới việc tự ý sửa file mà không hỏi.
- **Giải pháp áp dụng cho Devin** (cập nhật sau khi verify docs + `devin --help` trên 3000.10.31):
  - **Không có `DEVIN_CONFIG_CONTENT`** — cơ chế đúng của Devin: permission modes qua `--permission-mode` / env `DEVIN_PERMISSION_MODE` (`auto`=normal default, `accept-edits`, `smart`, `dangerous`=yolo/bypass, `autonomous`+sandbox) và rules `permissions.{allow,deny,ask}` trong `config.json` (tool names `read/edit/grep/glob/exec` + scope `Read()/Write()/Exec()/Fetch()` + `mcp__server__tool`), hoặc override file bằng global flag `--config <PATH>`.
  - Mode `auto`/`normal` mặc định của Devin **đã prompt** cho write/exec → giữ process ở mode này để mọi thao tác nguy hiểm phát `session/request_permission`; FlowPilot runner đóng vai trò Gatekeeper chuyển tiếp Approve/Deny.
  - Overlay chặt hơn khi cần (posture `scan`/`plan`): ghi `permissions.ask`/`deny` vào managed `~/.config/devin/config.json` của account home, hoặc truyền `--config` trỏ file overlay.
  - YOLO=ON: ưu tiên **adapter auto-approve** `session/request_permission` (không restart process khi toggle `/yolo`); `DEVIN_PERMISSION_MODE=dangerous` chỉ là phương án launch-time.

### Issue 2: [BUG-334](file:///Users/tiendat/Desktop/flowpilot/flowpilot/requirements/09-BugFix/done/BUG-334.md) — Child Agent (`spawn_agent`) làm đứt kết nối MCP của Parent Run
- **Hiện tượng**: Khi Parent Agent gọi công cụ `spawn_agent` để tạo Child Agent, Child Agent gửi lệnh `session/new` mang token MCP mới lên cùng tiến trình ACP đang chạy. Điều này ghi đè MCP client cấp tiến trình, làm các in-flight tool calls của Parent Agent bị đứt kết nối đột ngột (`MCP error -32000 Connection closed`).
- **Giải pháp áp dụng cho Devin**:
  - Áp dụng nguyên tắc **Child Process Scope Isolation**: Mỗi Child Run hoặc Variant Probe phải được chạy trên một tiến trình `devin acp` riêng biệt với scopeKey dạng `account|child:<runId>` hoặc `account|probe:<runId>`, tách biệt hoàn toàn với tiến trình của Parent.

### Issue 3: [BUG-329](file:///Users/tiendat/Desktop/flowpilot/flowpilot/requirements/09-BugFix/done/BUG-329.md) — Đổi model giữa phiên chat (Mid-chat Model Switch) gây mất session
- **Hiện tượng**: Root cause là `ensureOpencodeProcess` copy-paste từ Grok nên đưa `model` vào `processKey`. Đổi model → kill process A → start process B mới → gọi `session/load` trên process mới trả config rỗng (lỗi văng session).
- **Giải pháp áp dụng cho Devin**:
  - Không đưa `model` vào `processKey` để tránh khởi động lại process một cách không cần thiết khi chỉ đổi model.

### Issue 4: [BUG-361](file:///Users/tiendat/Desktop/flowpilot/flowpilot/requirements/09-BugFix/done/BUG-361.md) — Hết Quota hoặc Lỗi Mạng gây treo Turn (No Turn Failed)
- **Hiện tượng**: OpenCode không có turn-level timeout (CA-759). Khi tài khoản hết hạn mức, hết token hoặc rớt mạng, các mã lỗi không được ánh xạ đúng nên turn không báo fail mà bị treo `running`.
- **Giải pháp áp dụng cho Devin**:
  - Bổ sung nhận diện mã lỗi Quota trong `isProviderUsageLimitError`.
  - Phải ánh xạ đúng `stopReason` lỗi (qua hàm như `opencodeIsQuotaStopReason`) thành `EventTurnFailed(Recoverable: false)`.

### Issue 5: CA-679 — Restore phiên chat bị Posture pin ghi đè model người dùng chọn / Config env var
- **Hiện tượng**:
  - Người dùng chọn model, thoát TUI rồi mở lại. Posture mặc định tự động ghi đè model người dùng chọn.
  - Config env var truyền sai trỏ vào directory thay vì file khiến process crash on startup.
- **Giải pháp áp dụng cho Devin**:
  - Logic restore trạng thái ưu tiên sự lựa chọn của người dùng (`active model`).
  - Đảm bảo biến môi trường cấu hình CLI phải trỏ đúng file thay vì thư mục.

### Issue 6: CA-688 — Khôi phục phiên chat local sau khi khởi động lại Runner (Resume Session)
- **Hiện tượng**: OpenCode dùng SQLite DB, không có per-session file trên ổ cứng.
- **Xác nhận trên Devin 3000.10.31**: Devin cũng dùng **SQLite** `~/.local/share/devin/cli/sessions.db` (+`-shm`/`-wal`, `session_locks/`) — y hệt tình huống OpenCode. Tin tốt: `initialize` quảng cáo `loadSession:true` và `sessionCapabilities.list/delete` → `session/load` + `session/list` đều khả dụng.
- **Giải pháp áp dụng cho Devin**:
  - Bypass logic `LocateSessionFile` file-based; guard bằng `isDevinRealSessionID` (format sessionId probe qua `session/list` — không giả định dạng `ses_*`).
  - Drive sync/restore: quyết định sync ở mức DB hay chỉ metadata (probe tương đương Task-300 T-2b).

### Issue 7: CA-692 / CA-693 (Task-318 / Task-319) — Trung thực về tính năng (Capability Honesty) & Vision Guard
- **Hiện tượng**: Khai báo cờ `Vision: true` ở cấp provider gây hiểu lầm cho các model không hỗ trợ ảnh.
- **Giải pháp áp dụng cho Devin**:
  - Vision luôn giữ `false` ở cấp Provider (trong `devin_adapter.go`).
  - Bật cấu hình per-model thông qua `devinModelSupportsImages(model)` kết hợp với `devinTurnImageAttachments()`.

### Issue 8: CA-706..709 — Xử lý dòng dữ liệu (Streaming Chunk) phức tạp
- **Hiện tượng**:
  - `CA-706`: Late chunk drain — chunk cuối cùng tới sau khi turn_completed đã phát.
  - `CA-707`: Wait until text — turn kết thúc nhưng chưa có text nào được stream lên màn hình.
  - `CA-708`: Non-text starvation — model chỉ gọi tool mà không sinh ra text giải thích khiến UI tưởng treo.
  - `CA-709`: Array content — `agent_message_chunk` trả về content là một mảng `[{type: "text", text: ...}]` thay vì một chuỗi text đơn giản.
- **Giải pháp áp dụng cho Devin**:
  - Bộ phân tích `devin_event_mapper.go` phải hỗ trợ parse polymorphic: Cả `content: "string"` lẫn `content: [{"type": "text", "text": "..."}]`.
  - Có cơ chế drain sạch hàng đợi tin nhắn trước khi chính thức chuyển turn sang trạng thái `completed`.

### Issue 9: CA-712 / CA-713 — Người dùng từ chối quyền (Deny Permission) kết thúc êm thấm
- **Hiện tượng**: Adapter gửi đúng chuẩn `{"outcome": {"optionId": "reject"}}`. CLI abort turn khi deny và kết thúc với 0 text, dẫn đến blank turn.
- **Giải pháp áp dụng cho Devin**:
  - Adapter phải tự tổng hợp một synthetic message thông báo (vd: "Thao tác đã bị người dùng từ chối") để turn kết thúc một cách rõ ràng.

### Issue 10: CA-679 — Config env var trỏ directory thay vì file gây crash on startup
- **Giải pháp áp dụng cho Devin**: Devin dùng global flag `--config <PATH>` (override `~/.config/devin/config.json`) — nếu dùng, phải trỏ tới **file** cụ thể chứ không phải thư mục. Không có `DEVIN_CONFIG` env trong docs; isolation qua `HOME`/`XDG_CONFIG_HOME`/`XDG_DATA_HOME`.

### Issue 11: [BUG-334 branch 2](file:///Users/tiendat/Desktop/flowpilot/flowpilot/requirements/09-BugFix/done/BUG-334.md) — Dọn dẹp tiến trình con
- **Giải pháp áp dụng cho Devin**: Phải có hàm `CloseProcessesForChildRun(runID)` để dọn dẹp các tiến trình con một cách sạch sẽ, tránh rò rỉ bộ nhớ hoặc xung đột tài nguyên.

### Issue 12: BUG-330 — Foreign model injection
- **Hiện tượng**: Model provider khác gửi nhầm tên model sang CLI.
- **Giải pháp áp dụng cho Devin**: Kiểm tra và routing model chính xác (dựa vào `ModelProviderKey` trong `agentpack/pack.go`).

### Issue 13: CA-689b/c — Dynamic reasoning/effort ✅ verified
- **Giải pháp áp dụng cho Devin**: Effort nằm trong **model ID suffix** (`-low/-medium/-high/-xhigh/-max`); switch mid-session qua `session/set_config_option{configId:"model"}` — probe đã confirm đổi `swe-2-high→swe-2-medium` và `mode→plan` thành công.

### Issue 14: CA-688 follow-up — Transcript recording cho DB-backed provider
- **Giải pháp áp dụng cho Devin**: Cập nhật logic trong `interactive_service.go:7679` để đảm bảo hệ thống lưu transcript đúng cách cho các provider dùng DB nền mà không có file session độc lập.

### Issue 15: [BUG-334 branch 2](file:///Users/tiendat/Desktop/flowpilot/flowpilot/requirements/09-BugFix/done/BUG-334.md) — Probe catalog `session/new` rỗng MCP hủy kết nối MCP đang chạy
- **Giải pháp áp dụng cho Devin**: Tránh gửi catalog rỗng ghi đè session của tiến trình đang chạy. Sử dụng scope `probe` phân lập cho variants probe.

### Issue 16: FlowPilot MCP Server loopback
- **Giải pháp áp dụng cho Devin**: Runner phải tự host MCP server để xử lý `ask_user` + `spawn_agent` mà không phụ thuộc vào cơ chế MCP bên ngoài.

### Issue 17: CA-683 — Account usage caching/stats format
- **Giải pháp áp dụng cho Devin**: Chuẩn hóa định dạng caching account usage/stats để không phải liên tục lấy dữ liệu từ provider, tối ưu hiệu suất. Lưu ý: Devin CLI không có lệnh quota riêng — usage lấy từ `turn_completed._meta` và/hoặc slash `/session-stats` qua ACP; `devin auth status` chỉ cho auth identity.

### Issue 18 (ĐÃ GIẢI — live-probe 3000.10.31): `devin acp` không dùng credentials local — bắt buộc ACP `authenticate`
- **Hiện tượng (stderr thật khi spawn `devin acp`)**: *"ACP server credential policy: ACP host is the sole source of credentials. Local CLI credentials (env vars, on-disk REPL store) will NOT be used. Waiting for the ACP host to call `authenticate`."* — tức `WINDSURF_API_KEY` và `credentials.toml` đều bị bỏ qua trong ACP mode.
- **Verified 2026-09-20**: `authenticate{methodId:"devin-browser"}` → `result:{}`; PKCE browser flow hoàn tất ~3s **không cần click** khi browser đã login app.devin.ai. Quirk: `devin auth status` báo "Not logged in" nhưng ACP authenticate vẫn OK → hai credential store tách biệt (REPL vs ACP).
- **Giải pháp áp dụng**: Boot sequence `initialize → authenticate → session/new` (đã verify end-to-end, prompt thật trả `PROBE_OK` + usage). `StartInteractiveAuth` cần REPL `devin auth login` cho CLI commands (`models`, `-p`) và ACP PKCE cho provider path.

### Issue 19 (MỚI): Extension notifications `_cognition.ai/*` ngoài chuẩn ACP
- **Hiện tượng (live probe)**: `devin acp` phát `_cognition.ai/mcp/serversChanged` và `_cognition.ai/output` (log channel kèm `sessionId`) ngay sau `initialize`, ngoài spec ACP.
- **Giải pháp áp dụng cho Devin**: `devinDispatcher` + `devinEventMapper` phải tolerate unknown methods (`_*` prefix) — bỏ qua êm hoặc map `_cognition.ai/output` thành log/debug channel, tuyệt đối không fail turn.

### Issue 20 (MỚI): `session/new{mcpServers}` chỉ hỗ trợ stdio
- **Hiện tượng**: `initialize` trả `mcpCapabilities: {http:false, sse:false}` → param `mcpServers` của `session/new` chỉ nhận stdio servers; FlowPilot loopback MCP (stdio `go run ./cmd/flowpilot ...`) dùng được, nhưng remote MCP (Google Drive HTTP URL, Jira) phải đi qua `~/.config/devin/mcp_config.json` hoặc `devin mcp add`.
- **Giải pháp áp dụng cho Devin**: Hai đường MCP rõ ràng — loopback FlowPilot qua `session/new` mcpServers (stdio), external MCP qua file `mcp_config.json` (atomic write, đã verify `_meta.mcpConfigPath` trỏ đúng file này). **Bonus verified**: `flowpilot_drive` stdio MCP từ `mcp_config.json` hiện tại đã auto-connect thành công trong probe.
- **Thêm từ probe**: `session/list{cwd}` trả `[{sessionId,cwd,title,updatedAt,_meta:{createdAt,isLocked}}]` — keyed theo **canonical cwd** (macOS `/tmp`→`/private/tmp`, cần `filepath.EvalSymlinks`). `session/load` verified trả modes+configOptions (mode persist qua restart). Permission modes per-session: `accept-edits/smart/ask/plan/bypass` qua `set_config_option{configId:"mode"}` — `bypass`≡YOLO, `ask`/`plan`≡read-only postures.

### Issue 21 (MỚI): Alias model Devin trùng tên provider khác (BUG-330 chiều ngược)
- **Hiện tượng**: Devin alias `opus`,`sonnet`,`swe`,`codex`,`gemini`,`gpt`,`adaptive`,`fusion` — `providerKeyFromModel` của FlowPilot sẽ route `gpt*`→codex, `gemini*`→gemini, `opus/sonnet`→claude nếu model được lưu **không prefix**.
- **Giải pháp áp dụng cho Devin**: Mọi row `ai_supported_models` và mọi điểm truyền model bắt buộc prefix `devin/`; `defaultModelForProvider("devin") = "devin/swe-2-high"` (currentValue verified trong `configOptions.model`; fallback `devin/adaptive`). Catalog ~380 entries lấy từ `session/new.result.configOptions.model.options` — không cần `devin models list` (REPL auth tách biệt).

### Issue 22 (MỚI): Devin native subagents ≠ FlowPilot `spawn_agent`
- **Hiện tượng**: Devin CLI có tool `run_subagent`/`read_subagent` riêng (config `subagents_enabled`, default true) — child nội bộ của Devin, không phải managed run của FlowPilot.
- **Giải pháp áp dụng cho Devin**: FlowPilot `spawn_agent` vẫn đi qua loopback MCP → `TurnBridge.SpawnAgent` → child scope `account|child:<runId>` (BUG-334). Subagent native của Devin chỉ render như tool_call thường; quyết định có ghi `subagents_enabled` vào managed config hay không trong Task-402.

---

## 3. Kiến Trúc Tích Hợp Đề Xuất (The Architecture)

```text
┌────────────────────────────────────────────────────────────────────────┐
│                          FLOWPILOT RUNNER CORE                         │
│                                                                        │
│   ┌───────────────────────────┐         ┌──────────────────────────┐   │
│   │   ProviderRuntimeAdapter  │         │       TurnBridge         │   │
│   │   (Key() == "devin")      │         │ (RequestApproval, Events)│   │
│   └─────────────┬─────────────┘         └─────────────▲────────────┘   │
└─────────────────┼─────────────────────────────────────┼────────────────┘
                  │                                     │
                  ▼                                     │ (Normalized Events)
┌───────────────────────────────────────┐   ┌───────────┴─────────────┐
│             devin_adapter.go          │   │ devin_event_mapper.go   │
│   - Capabilities (Honest MVP)         │   │ - Parse Polymorphic     │
│   - SendTurn()                        │──▶│ - Map stopReason        │
└──────────────────┬────────────────────┘   └─────────────────────────┘
                   │
                   │                                     (Approval Gate)
                   │             ┌─────────────────────────┐ User Approves 
                   │             │ adapter.handleInbound() ├──────────────┐
                   ▼             └─────────────▲───────────┘              │
┌───────────────────────────────────────┐      │                          ▼
│             devin_process.go          │──────┘                 SendResponse()
│   - Stdio Subprocess (devin acp)      │ (CLI -> stdout -> dispatcher)
│   - Scoped Process (account|child)    │
│   - Async JSON-RPC Dispatcher         │
└──────────────────┬────────────────────┘
                   │
                   │ (stdio: JSON-RPC 2.0)
                   ▼
┌───────────────────────────────────────┐
│              DEVIN CLI                │
│             (`devin acp`)             │
│   - Native Agent Client Protocol      │
│   - FlowPilot MCP Server loopback     │
└───────────────────────────────────────┘
```

---

## 4. Tóm Tắt Kế Hoạch 4 Tasks & Tài Liệu Liên Quan

1. [CP-70 Coding Plan (todo)](file:///Users/tiendat/Desktop/flowpilot/flowpilot/requirements/07-Coding-Plan/todo/CP-70-Devin-Provider-Integration.md)
2. [CP-Guide-Add-New-Provider-Checklist](file:///Users/tiendat/Desktop/flowpilot/flowpilot/requirements/07-Coding-Plan/CP-Guide-Add-New-Provider-Checklist.md)
3. [CP-57 Mẫu chuẩn Opencode](file:///Users/tiendat/Desktop/flowpilot/flowpilot/requirements/07-Coding-Plan/done/CP-57-Opencode-Provider-Integration.md)
4. Phân chia 4 Tasks con:
   - **Task-400**: Devin ACP Transport & Scoped Process Dispatcher
   - **Task-401**: Devin Controlled Adapter MVP & Polymorphic Event Mapper
   - **Task-402**: Devin Settings (Detect CLI, Install, Models, MCP, Account)
   - **Task-403**: Devin Chat/Flow UI Parity & Manual Verification
