# CP-70: Devin Provider Integration (Controlled ACP Adapter + Settings + Chat/Flow Parity)

## Metadata

- Document ID: `CP-70`
- Title: `Devin Provider Integration (Controlled ACP Adapter + Settings + Chat/Flow Parity)`
- Phase: `coding_plan`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-20`
- Last Updated: `2026-09-21`
- Parent Documents: [SS-05: Workflow AI Provider](../../05-System-Specs/SS-05-Workflow-Ai-Provider.md), [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md), [SS-12: Multiple Agents](../../05-System-Specs/SS-12-Multiple-Agents.md), [SD-06: AI Provider Integration](../../06-System-Tech-Design/SD-06-AI-Provider-Integration.md), [SD-16: Agent Spawn And Tool Calling Design](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md), [SD-11: MCP Connection Flows](../../06-System-Tech-Design/SD-11-MCP-Connection-Flows.md)
- Child Documents: [Task-400: Devin ACP Transport And Process Dispatcher](../../08-Task/done/Task-400-Devin-ACP-Transport-And-Process-Dispatcher.md) (done), [Task-401: Devin Controlled Adapter MVP](../../08-Task/done/Task-401-Devin-Controlled-Adapter-MVP.md) (done), [Task-402: Devin Settings — Detect/Install/Models/MCP/Account](../../08-Task/done/Task-402-Devin-Settings-Detect-Install-Models-MCP-Account.md) (done), [Task-403: Devin Chat/Flow — Model/Reasoning/YOLO/Cards/Tools](../../08-Task/done/Task-403-Devin-Chat-Flow-Model-Reasoning-YOLO-Cards-Tools.md) (done)
- Related Documents: [CP-70-Test-Steps.md](./CP-70-Test-Steps.md), [CP-70-note.md](../note/CP-70-note.md), [CP-Guide-Add-New-Provider-Checklist](../CP-Guide-Add-New-Provider-Checklist.md), [CP-57: Opencode Provider Integration](../done/CP-57-Opencode-Provider-Integration.md), [CP-57-Test-Steps](../done/CP-57-Test-Steps.md), [CP-46: Grok Build Controlled Adapter Over ACP](../done/CP-46-Grok-Build-Controlled-Adapter-Over-ACP.md), [Skill: add-new-provider](../../.agents/skills/add-new-provider/SKILL.md)
- Replaces: `None`
- Tags: `devin, ai-providers, adapter, acp, mcp, local-runner, desktop-chat, settings`
- Feature Keys: `ai-providers`

## AI Quick View

### Summary

- Tích hợp **Devin CLI** (`devin`, do Cognition AI phát triển) trở thành AI Provider chính thức (first-class controlled-mode provider) trong FlowPilot bên cạnh Codex, Claude, Gemini, Grok và Opencode, kế thừa toàn bộ kiến trúc `ProviderRuntimeAdapter` / `TurnBridge` / `ProviderEvent` (`apps/local-runner/internal/runner/provider_registry.go:88`).
- Devin CLI sử dụng trực tiếp **Agent Client Protocol (ACP)** qua `devin acp` (JSON-RPC 2.0 trên stdio) — chuẩn mở do Cognition và Zed khởi xướng, tương thích 100% với kiến trúc ACP hiện hữu của FlowPilot (`opencode_acp.go`, `grok_acp.go`, `gemini_acp_transport.go`).
- **Follow trọn vẹn 100% toàn bộ những gì Opencode đã làm (CP-57)**: Từ 28 hạng mục đối soát (28-Row Mirror Matrix), cấu trúc phân rã 4 Tasks (`Task-400..403`), bảng đối soát Base-Regression Guard (`DV-BR`), bộ kiểm thử chuẩn `DV-01..35` và `E2E-01..27`.
- **Cài cắm sẵn giải pháp cho 9 bài học xương máu (BugFixes & Change Audits) từ đợt Opencode**:
  1. `BUG-331 / CA-690`: Cấu hình overlay permission chặt chẽ (`ask` cho file/bash) tránh việc CLI mặc định allow-all khi YOLO=OFF.
  2. `BUG-334`: Cô lập Child Process Scope (`scopeKey: account|child:<runId>`) khi chạy `spawn_agent` để tránh reset MCP client làm đứt kết nối parent.
  3. `BUG-329`: Chuyển model giữa chat (`session/load`) mang theo `sessionId` hợp lệ, fallback chuyển context an toàn.
  4. `BUG-361`: Heartbeat & timeout guard chống treo turn khi hết quota hoặc lỗi mạng.
  5. `CA-679`: Bảo toàn model người dùng chọn khi restore phiên, không để Posture pins ghi đè.
  6. `CA-688`: Lưu trữ session local (`ai_provider_sessions`) để khôi phục được chat cũ sau khi restart runner.
  7. `CA-692 / CA-693` (Task-318/319): Giữ `Vision = false` cho tới khi có golden fixture chứng minh multimodal.
  8. `CA-706..709`: Xử lý streaming chunk linh hoạt (polymorphic text/array content, late chunk drain, non-text starvation guard).
  9. `CA-712 / CA-713`: Kết thúc turn êm thấm khi người dùng từ chối cấp quyền (Deny Permission) kèm thông báo lịch sự.
- **Nguyên tắc P-0 (Plugin-Only, Zero Base Regression)**: Tuyệt đối không làm hồi quy 5 provider hiện tại. Mọi file dùng chung chỉ append nhánh `case "devin"` ở vị trí cuối cùng.

### Current Ask

- Thiết lập Coding Plan hoàn chỉnh, bám sát 100% cấu trúc và độ chi tiết của CP-57, cung cấp đầy đủ thông số kỹ thuật, bảng đối soát, ma trận hồi quy và kế hoạch kiểm thử cho Devin CLI.

### Key Decisions

- `P-0` **Plugin-only, Zero Base Regression**: Devin là additive module. Mọi enum, switch-case, registry dùng chung đều append `case "devin"` ở vị trí cuối cùng. Mỗi file dùng chung phải có regression test chứng minh các provider cũ chạy không đổi.
- `P-1` **ACP Protocol Wire (`devin acp`)**: Tương tác interactive turn qua tiến trình nền `devin acp` (JSON-RPC 2.0 stdio). Tác vụ tóm tắt one-shot (summarizer) dùng headless `devin -p` hoặc `devin acp --agent-type summarizer` (agent chuyên dụng, không tools, persist ra `~/.local/share/devin/summaries/<session_id>.md`) — chốt phương án sau live probe Task-400; flag `--json` cho `-p` **chưa được docs xác nhận**, không freeze.
- `P-2` **Dedicated Process & Scoped Dispatcher**: Xây dựng module độc lập `devin_acp.go` + `devin_process.go` + `devin_adapter.go` (copy từ pattern đã kiểm chứng của `opencode_process.go` và `grok_process.go`).
- `P-3` **Child Process Isolation (BUG-334)**: Mọi child run từ `spawn_agent` hoặc variant probe chạy trên tiến trình ACP riêng biệt theo scope `account|child:<runId>`.
- `P-4` **Explicit Permission Posture (BUG-331 / CA-690)**: Devin `normal`/`auto` mode mặc định đã prompt cho write/exec; FlowPilot giữ process ở `DEVIN_PERMISSION_MODE=auto` để mọi thao tác nguy hiểm phát `session/request_permission` qua `TurnBridge`. Overlay chặt hơn (posture `scan`) dùng `permissions.ask`/`deny` trong managed `config.json` hoặc flag `--config <PATH>` — **không có** `DEVIN_CONFIG_CONTENT`.
- `P-5` **Midchat Model Switch & Session Resume (BUG-329)**: Truyền đúng `sessionId` trong `session/load` (đã xác nhận `loadSession: true`), fallback an toàn nếu đổi model yêu cầu fresh session; không đưa `model` vào `processKey`.
- `P-6` **Capability Honesty (P-12 CP-57)**: Cờ khả năng (`Vision`, `Mcp`, `ApprovalEvents`) giữ `false` cho đến khi có golden test kiểm chứng thực tế — kể cả khi `initialize` quảng cáo `promptCapabilities.image: true`.
- `P-7` **Stream & Chunk Robustness (CA-706..709)**: Mapper event hỗ trợ cả text string lẫn array chunk, drain sạch queue trước khi kết thúc turn; dispatcher phải bỏ qua êm các notification mở rộng `_cognition.ai/*` (`_cognition.ai/output`, `_cognition.ai/mcp/serversChanged`).
- `P-8` **Denied Turn Clean Exit (CA-712/713)**: Khi người dùng bấm Deny quyền, adapter gửi mã từ chối hợp lệ và kết thúc turn sạch sẽ kèm thông báo.
- `P-9` **ACP `authenticate` Handshake (live-verified 3000.10.31)**: `devin acp` **không** đọc `WINDSURF_API_KEY` hay `credentials.toml` — stderr log: *"ACP host is the sole source of credentials… Waiting for the ACP host to call `authenticate`."* Mọi process sau `initialize` bắt buộc gọi `authenticate` (method `devin-browser` hoặc method token nếu probe ra) trước `session/new`; lỗi auth phải map thành typed `auth_required` với hướng dẫn `devin auth login`.

### Constraints

- Không sửa đổi bất kỳ logic hay file nào của `claude_adapter.go`, `codex_adapter.go`, `grok_acp.go`, `opencode_acp.go`, `gemini_acp_transport.go`.
- Giữ vững regression suite: Tất cả bài test cũ của 5 provider hiện tại phải tiếp tục xanh 100%.
- Runner làm chủ toàn bộ state, database Supabase, transcript, approval gates, flow rules. Frontend Desktop/TUI chỉ đóng vai trò hiển thị và gửi lệnh qua giao diện chuẩn.

### Open Questions

- `Q-1` ✅ **Đã trả lời** (F-5/F-18/F-24): sessions trong SQLite `~/.local/share/devin/cli/sessions.db`; sessionId = **slug `adjective-noun`** (`sore-router`, `frost-plywood`); `session/list` keyed theo **canonical cwd** (macOS `/tmp`→`/private/tmp`); `session/load` verified end-to-end.
- `Q-2` ✅ **Đã trả lời** (F-21): model catalog ~380 entries lấy từ `session/new.result.configOptions.model.options` — tốt hơn `devin models list` (REPL auth tách biệt, hiện fail). Default `swe-2-high` (currentValue trong probe).
- `Q-3` ⏳ Còn thiếu: `session/request_permission` shape chưa capture (probe `accept-edits` auto-approved `exec`) — Task-400 trigger bằng mode `smart`/`auto` + dangerous command để freeze optionIds (F-25).
- `Q-4` ✅ **Đã trả lời** (F-17): `authenticate{methodId:"devin-browser"}` → `{}`, PKCE hoàn tất ~3s nhờ browser session sẵn có. **Lưu ý**: ACP credential store tách khỏi REPL (`devin auth status` báo "Not logged in" nhưng ACP authenticate vẫn thành công) — `StartInteractiveAuth` nên drive PKCE qua ACP hoặc `devin auth login` cho REPL path.
- `Q-5` ✅ **Đã trả lời** (F-20): reasoning effort = **suffix trong model ID** (`-low/-medium/-high/-xhigh/-max`, variant `-fast`/`-priority`, context `-1m`); đổi qua `session/set_config_option{configId:"model"}` mid-session. `devinReasoningEffortID` map FlowPilot effort → suffix của family đang chọn.
- `Q-6` ✅ **Đã trả lời** (F-20/F-22): `session/set_config_option{configId:"mode"}` hỗ trợ `accept-edits/smart/ask/plan/bypass` — posture map trực tiếp (`scan`→`ask`, `plan`→`plan`, `code`→`accept-edits`, YOLO→`bypass`).
- `Q-7`: `devin -p` có flag output JSON không (docs không list `--json` cho `-p`)? Và `devin -p` có cần `--respect-workspace-trust false` trong workspace untrusted không? Chốt trong Task-400 để quyết summarizer: `devin -p` vs `devin acp --agent-type summarizer`.

### Source Refs

- `requirements/07-Coding-Plan/done/CP-57-Opencode-Provider-Integration.md` (chuẩn tham chiếu gốc)
- `requirements/07-Coding-Plan/CP-Guide-Add-New-Provider-Checklist.md`
- `requirements/07-Coding-Plan/done/CP-57-Test-Steps.md`
- `.agents/skills/add-new-provider/SKILL.md`
- [SS-05: Workflow AI Provider](../../05-System-Specs/SS-05-Workflow-Ai-Provider.md)
- [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- [SD-06: AI Provider Integration](../../06-System-Tech-Design/SD-06-AI-Provider-Integration.md)

---

## 1. Goal

Đưa **Devin CLI** (`devin`) trở thành một AI Provider đầy đủ (first-class controlled provider) trong hệ sinh thái FlowPilot, đạt được parity 100% với những gì Opencode và Grok đã làm:
1. **Quản lý & Cài đặt (Settings)**: FlowPilot phát hiện được binary `devin` trên máy, hiển thị trạng thái đăng nhập/account, hỗ trợ cài đặt one-click, hiển thị model catalog, cấu hình token và MCP tool.
2. **Trò chuyện & Thực thi (Chat & Flow)**: Hỗ trợ streaming phản hồi qua ACP, hiển thị card công cụ (tool cards), diffs file thay đổi, duyệt quyền (approval) khi sửa file / chạy lệnh, hỗ trợ chế độ YOLO (`DEVIN_PERMISSION_MODE=dangerous` hoặc adapter auto-approve — F-7).
3. **Độ ổn định cao**: Áp dụng triệt để các kinh nghiệm xử lý lỗi từ Opencode để đảm bảo adapter chạy trơn tru, hỗ trợ spawn child agents, đổi model giữa chừng, và tiếp tục phiên (resume chat).

---

## 2. Input Documents

- [SS-05: Workflow AI Provider](../../05-System-Specs/SS-05-Workflow-Ai-Provider.md)
- [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- [SS-12: Multiple Agents](../../05-System-Specs/SS-12-Multiple-Agents.md)
- [SD-06: AI Provider Integration](../../06-System-Tech-Design/SD-06-AI-Provider-Integration.md)
- [SD-16: Agent Spawn And Tool Calling Design](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md)
- [SD-11: MCP Connection Flows](../../06-System-Tech-Design/SD-11-MCP-Connection-Flows.md)
- [CP-Guide-Add-New-Provider-Checklist.md](../CP-Guide-Add-New-Provider-Checklist.md)
- [CP-57: Opencode Provider Integration](../done/CP-57-Opencode-Provider-Integration.md)
- [CP-57-Test-Steps.md](../done/CP-57-Test-Steps.md)
- Các hồ sơ Bug / Change Audit liên quan: `BUG-329`, `BUG-331`, `BUG-334`, `BUG-361`, `CA-679`, `CA-688`, `CA-690`, `CA-692`, `CA-706`..`CA-713`.

---

## 3. Implementation Strategy

- **Phương pháp tiếp cận tổng thể (Overall approach)**:
  - Tái sử dụng trọn vẹn hợp đồng kiểm soát chuẩn: `ProviderRuntimeAdapter`, `TurnBridge`, `ProviderEvent`, `InteractiveService`, `ProviderRegistry`.
  - Sử dụng **ACP qua `devin acp`** làm tầng truyền thông cơ bản (transport substrate); sao chép kiến trúc async dispatcher đã được kiểm chứng từ `opencode_process.go` và `grok_process.go`, tuyệt đối không refactor làm ảnh hưởng các transport cũ.
  - Tích hợp Devin qua các điểm mở rộng độc lập; phía Desktop chỉ thêm enum/label/icon/brand style.
  - Tuân thủ thứ tự các lớp: transport/process → event mapper → adapter → registry gate → settings UI → chat/flow UI → hardening test.
- **Trình tự thực thi (Sequencing logic)**:
  - Bắt gói tin thực tế (freeze ACP wire shapes) từ phiên chạy `devin acp` (Task-400) trước khi bật bất kỳ capability nào; giữ `devin -p` (hoặc `devin acp --agent-type summarizer`) riêng cho one-shot summarizer.
  - Dựng transport/process/dispatcher trước (Task-400), sau đó dựng adapter + mapper (Task-401), xử lý permission/YOLO + MCP + `ask_user`/`spawn_agent` (Task-403), hoàn thiện Settings (Task-402), sau đó chạy nghiệm thu DOD.
  - Giữ `DefaultProviderRegistry` **không có Devin** (giống Grok và Opencode); chỉ bật `Available` trong `ProviderRegistryFor` khi cờ `FLOWPILOT_DEVIN_AGENT=1` và tài khoản hợp lệ.

### 3.0 Live-Verified Facts (probe thật trên `devin 3000.10.31`, macOS arm64, 2026-09-20)

Các sự thật sau đã được kiểm chứng trên binary thật — **không cần probe lại**, chỉ cần bổ sung golden fixtures chi tiết payload trong Task-400:

| # | Sự thật | Bằng chứng / Nguồn | Tác động thiết kế |
|---|---------|--------------------|-------------------|
| F-1 | `devin acp` tồn tại, JSON-RPC 2.0 stdio; trả lời `initialize` kể cả khi chưa login | Live probe `initialize` | Transport khả thi 100% |
| F-2 | `initialize.result.agentCapabilities`: `loadSession:true`, `promptCapabilities:{image:true,audio:false,embeddedContext:true}`, `mcpCapabilities:{http:false,sse:false}` (**chỉ stdio**), `sessionCapabilities:{list,delete,additionalDirectories}`; `authMethods:[{id:"devin-browser"}]`; `_meta.mcpConfigPath:"~/.config/devin/mcp_config.json"`; `agentInfo.name:"affogato"` | Golden response đã capture | `session/load` resume OK; `session/list` cho history; `session/new{mcpServers}` chỉ nhận **stdio** server — remote MCP phải đi qua `mcp_config.json` |
| F-3 | **Credential policy đặc biệt**: `devin acp` KHÔNG đọc `WINDSURF_API_KEY`/`credentials.toml` — "ACP host is the sole source of credentials… Waiting for the ACP host to call `authenticate`". Method `authenticate` tồn tại (trả `-32602 Invalid params` khi gọi sai, không phải `-32601`) | stderr log + probe `authenticate` | Boot sequence bắt buộc `initialize → authenticate → session/new`; thiếu bước này = không chat được |
| F-4 | Custom notifications `_cognition.ai/mcp/serversChanged`, `_cognition.ai/output` (log channel có `sessionId`) phát ngoài chuẩn ACP | Live probe | Dispatcher/mapper phải tolerate unknown `_*` methods |
| F-5 | Session storage = **SQLite** `~/.local/share/devin/cli/sessions.db` (+`-shm`/`-wal`), `session_locks/`, `summaries/`, `mcp/oauth/`, `plugins/`, `_versions/` | `ls` thực tế | CA-688 pattern: bypass `LocateSessionFile` file-based, `isDevinRealSessionID` guard; Drive sync copy DB-level hoặc skip (probe T-2b) |
| F-6 | Credentials ở `~/.local/share/devin/credentials.toml` (KHÔNG phải `~/.devin` hay `~/.config/devin/credentials.json`); `devin auth status` in ra path + trạng thái | `devin auth status` output | `hasValidProviderAuthFile`/`defaultAuthCandidates` trỏ `credentials.toml` |
| F-7 | Permission modes: `auto`(=normal, default: read-only auto, write/exec prompt) / `accept-edits` / `smart` / `dangerous`(alias `yolo`,`bypass`) / `autonomous`(cần `--sandbox`). Flag `--permission-mode`, env `DEVIN_PERMISSION_MODE`. **Không có flag `--auto`** — `auto` là giá trị mode | `devin --help` | YOLO=ON → `dangerous` hoặc adapter auto-approve; YOLO=OFF → `auto`/`normal` + `session/request_permission` gate |
| F-8 | `devin acp` options: `--agent-type {summarizer,review}`, `--model` (env `DEVIN_MODEL`), `--refusal-fallback` (env `DEVIN_REFUSAL_FALLBACK`); global flags `--config <PATH>`, `--model`, `--permission-mode`, `-p/--print`, `-c/-r`, `--export` (ATIF) | `devin acp --help`, `devin --help` | Summarizer agent-type là phương án thay `devin -p`; `--config` cho managed-home overlay; `--export` ATIF → transcript replay |
| F-9 | Model commands: `devin models list [--format json]` (cần login); alias `opus/sonnet/swe/codex/gemini/gpt/adaptive/fusion` — **trùng tên provider khác** | docs + `devin --help` | Mọi model trong `ai_supported_models` bắt buộc prefix `devin/`; BUG-330 guard chiều ngược: alias Devin trần (`opus`,`gpt`) sẽ bị `providerKeyFromModel` route sang provider khác nếu thiếu prefix |
| F-10 | Auth CLI: `devin auth login [--force-manual-token-flow]`, `devin auth status`, `devin auth logout`, `devin setup`. **`devin configure` không tồn tại** | `devin --help`, docs | `StartInteractiveAuth` → `devin auth login` |
| F-11 | MCP quản lý: `devin mcp add/list/get/remove/login/logout/enable/disable`; config files `~/.config/devin/mcp_config.json`, `.devin/mcp_config.json`, `.devin/mcp_config.local.json` (từ v3000.3 — KHÔNG còn trong `config.json`) | docs + `_meta.mcpConfigPath` | `devin_mcp_config.go` ghi `mcpServers` vào `mcp_config.json`, hoặc dùng `devin mcp add -s user` |
| F-12 | Install: `curl -fsSL https://cli.devin.ai/install.sh \| bash` (mac/Linux/WSL → `~/.local/bin/devin` symlink `_versions/current/bin/devin`); Windows `irm https://static.devin.ai/cli/setup.ps1 \| iex`; upgrade `devin update`. **Không có brew cask trong docs** | docs troubleshooting + symlink thực tế | `runProviderInstallCommand` dùng curl/irm, không brew |
| F-13 | Data/config dirs: config `~/.config/devin/` (XDG), data `~/.local/share/devin/` (XDG data), Windows `%APPDATA%\devin\`. Env: `DEVIN_MODEL`, `DEVIN_PERMISSION_MODE`, `DEVIN_SANDBOX`, `DEVIN_REFUSAL_FALLBACK`, `RUST_LOG`, `CHISEL_LOG_STDOUT`. **Không có `DEVIN_HOME`, `DEVIN_CONFIG_CONTENT`** | docs + fs thực tế | Env isolation qua `HOME`+`XDG_CONFIG_HOME`+`XDG_DATA_HOME`; strip `WINDSURF_API_KEY` |
| F-14 | Slash commands advertised over ACP (`/login` `/status` `/plan` `/fast` `/ask` `/mcp`…); workspace commands chỉ hiện khi host nhường quyền quản roots | docs commands.mdx | TUI có thể surface Devin slash commands; posture `plan` của Devin ≈ read-only |
| F-15 | Devin native subagents: config `subagents_enabled` + tools `run_subagent`/`read_subagent` — **khác** FlowPilot `spawn_agent` (MCP loopback) | docs config-file.mdx | Hai cơ chế cộng tồn: FlowPilot child = managed run; Devin subagent = tool_call thường trong timeline. Quyết định giữ/tắt `subagents_enabled` trong managed config |
| F-16 | `devin list --format json` list sessions theo cwd; `devin rm` xóa session; `devin doctor` có trong build 3000.10.31 | `devin --help` | Session discovery/restore + diagnostics |
| F-17 | **`authenticate` params confirmed**: `{methodId:"devin-browser"}` → `result:{}`; PKCE browser flow hoàn tất ~3s không cần thao tác (tái dùng browser session app.devin.ai đang login); stderr `PKCE authentication successful` | Live probe id=2 | `devinACPAuthenticateParams` chỉ cần methodId; adapter gọi sau `initialize`, trước `session/new` |
| F-18 | **sessionId format = slug `adjective-noun`** (`sore-router`, `sturdy-fang`, `frost-plywood`, `generated-iguanodon`) — KHÔNG `ses_*`, KHÔNG UUID | `session/new` result, `session_db` inventory log | `isDevinRealSessionID`: slug regex `^[a-z]+-[a-z]+$`-ish (vẫn validate bằng `session/list` khi cần chắc chắn); `thread-*` không bao giờ resume |
| F-19 | **`session/new` result** trả `{sessionId, modes:{currentModeId,availableModes}, configOptions:[mode,model]}` — `modes` (legacy) + `configOptions` (mới) cùng tồn tại | Golden capture id=3 | Parse cả hai; `configOptions` là nguồn model catalog + mode hiện tại |
| F-20 | **`session/set_config_option` hoạt động cho `model` VÀ `mode`** — đổi `model→swe-2-medium` và `mode→plan` mid-session đều trả configOptions mới + emit `config_option_update`/`current_mode_update` | Live probe id=4/5 | Midchat model switch (BUG-329) + posture qua `set_config_option`, không cần respawn; reasoning effort nằm TRONG model ID (suffix `-low/-medium/-high/-xhigh/-max`) → `devinReasoningEffortID` map effort→suffix của model family đang chọn |
| F-21 | **Model catalog ~380 entries** qua `configOptions.model.options` — nguồn catalog TỐT HƠN `devin models list` (CLI đó cần REPL auth riêng, hiện báo "Not logged in" dù ACP authenticate OK — hai credential store tách biệt). Pattern `<family>-<version>-<effort>[-fast/-priority][-1m]` + `fusion-*` combos + `adaptive` | Golden capture | `detectDevinModels` nên probe qua ACP `session/new` (không phụ thuộc REPL login); mỗi option có `_meta.cognition.ai/supportsImages` |
| F-22 | **Session modes = permission system per-session**: `accept-edits`(Code) / `smart` / `ask`(read-only) / `plan` / `bypass`(auto-approve all). Default `accept-edits`. `bypass` ≡ YOLO | `session/new` result + `available_commands_update` | Posture map: `scan`→`ask`, `plan`→`plan`, `code`→`accept-edits`; YOLO→`bypass` qua `set_config_option` — **ưu tiên hơn** `DEVIN_PERMISSION_MODE` env (chỉ set lúc spawn) |
| F-23 | **`session/prompt` stream**: `agent_thought_chunk`, `agent_message_chunk`, `tool_call`{kind:"execute",rawInput}, `tool_call_update`{status,inferenceToolName,cwd,terminal_exit}, `usage_update`{used,size,_meta.inputTokens/outputTokens}, `session_info_update`{title}; kết thúc `result:{stopReason:"end_turn",usage:{totalTokens,inputTokens,outputTokens,cachedReadTokens}}` + `_cognition.ai/agent_stopped`{modelLabel,ttftMs,tokensPerSec} | Golden capture prompt `PROBE_OK` | Event mapper có đủ shape thật; usage/đã có sẵn cho status line; `agent_stopped` cho stats panel |
| F-24 | **`session/list` keyed theo canonical cwd** — `/tmp` trả `[]` nhưng `/private/tmp` trả đúng 2 sessions (`{sessionId,cwd,title,updatedAt,_meta:{createdAt,isLocked}}`); **`session/load` OK** trả modes+configOptions (mode `plan` persist qua process restart → DB-backed state) | Live probe | `LocateSessionFile` phải canonicalize cwd (filepath.EvalSymlinks); resume đã verify end-to-end |
| F-25 | **`session/request_permission` chưa capture được** — `accept-edits` auto-approve `exec` trong probe; cần prompt nguy hiểm hơn hoặc mode `smart`/`auto` để trigger | Probe chưa trigger | Golden fixture permission còn thiếu — Task-400 phải capture trước khi freeze optionIds |
| F-26 | **Vision thật sự support**: `promptCapabilities.image:true` + per-model `_meta.cognition.ai/supportsImages:true` — vision là **per-model** chứ không phải provider-wide false | initialize + configOptions | `supportsVisionFor("devin")` nên theo model capability (hoặc conservative false v1, bật khi model có flag); sửa parity matrix Row vision |

### 3.1 Required Parity Matrix (So sánh đối chuẩn đầy đủ như CP-57 §3.1)

| Tính năng / Năng lực | Baseline (Codex/Claude/Grok/Opencode) | Yêu cầu đối với Devin | Bằng chứng thực tế (Live evidence) | Mã kiểm thử |
| :--- | :--- | :--- | :--- | :--- |
| **Normal chat & streaming** | Turn phát normalized event, lưu history | ACP `session/update` text chunks → `message_delta` / `message_completed`; ACP `turn_completed` kết thúc turn | `devin acp` hỗ trợ streaming JSON-RPC 2.0 | `DV-01`, `DV-10` |
| **Task / Bugfix mode** | `changeType=task\|bugfix` chạy qua interactive run | Devin nhận task/bugfix turn giống hệt các provider khác | Tương thích prompt assembly chung | `DV-02` |
| **Workflow-step routing** | `workflow_step_auto` phân giải provider theo model | Prefix `devin/*` → `ProviderKeyDevin` | Router đọc model prefix | `DV-03` |
| **Tool events & Diffs** | `tool_started`/`tool_completed` render UI | Devin `tool_call` → mapper → tool events; `FileEvents` từ file diffs | `session/update` tool_call payload | `DV-11` |
| **YOLO off (Approval gate)** | Thao tác nguy hiểm → `permission_required`; Deny chặn | ACP `session/request_permission` → `TurnBridge.RequestApproval`; Deny chặn | Devin mode `auto`/`normal` mặc định prompt write/exec (F-7) | `DV-04` |
| **YOLO on** | Runner tự duyệt thao tác đủ điều kiện; `ask_user` không auto | `DevinPermissionMode` trong `YoloPosture` → adapter auto-approve `session/request_permission` (không restart process); `ask_user` luôn hỏi | `DEVIN_PERMISSION_MODE=dangerous` chỉ là phương án launch-time, ưu tiên client-side auto-approve | `DV-05`, `DV-06` |
| **User questions** | `ask_user` → `user_question_required` | Devin `ask_user` chặn chờ user trả lời trên Desktop/TUI | Tool MCP FlowPilot hoặc question shim | `DV-06` |
| **Spawn agents** | `spawn_agent` → `TurnBridge.SpawnAgent` | Devin gọi `spawn_agent` qua MCP → spawn child agent | Tách process scope `child:<runId>` | `DV-07`, `DV-22` |
| **Skill injection** | Skill được chèn theo đúng thứ tự | `promptPrep` gọi `injectSelectedSkills`, bổ sung tool reinforcements | Dùng chung injector của runner | `DV-08` |
| **Context injection** | Feature history + audit + summary được chèn | Dùng chung bộ ghép prompt của runner | Shared prompt assembly | `DV-09` |
| **Flow rules** | `r-ca`/`r-bug`/`r-task` kiểm duyệt qua `finishTurn` | Devin turns đi qua `finishTurn`; gate-repair prompt chạy trên Devin | Flow gate hooks trong runner | `DV-10` |
| **MCP (FlowPilot + External)** | FlowPilot tools + Google Drive hiển thị đồng thời | `session/new{mcpServers}` chỉ hỗ trợ **stdio** (F-2 `mcpCapabilities http/sse:false`); remote MCP (Google Drive URL) ghi vào `~/.config/devin/mcp_config.json` hoặc `devin mcp add -s user` | `_meta.mcpConfigPath` + live connect `flowpilot_drive` | `DV-17`, `DV-21` |
| **Setting: Detect CLI** | `tooling.json` + `flowpilot providers list` | `devin --version` + `CompatTestedDevinVersion="3000.10.31"`; CheckVersion row 5 | Binary `devin` có sẵn trên máy | `DV-25`, `DV-18` |
| **Setting: Install CLI** | Nút bấm cài đặt 1-click | `providerInstallCommand` devin case → `curl -fsSL https://cli.devin.ai/install.sh \| bash` (mac/Linux); Windows `irm https://static.devin.ai/cli/setup.ps1 \| iex`; upgrade `devin update` | Installer script chính thức (F-12) | `DV-18` |
| **Setting: Supported models** | Hiển thị danh mục model cho provider | `devin models list --format json` → `ai_supported_models` prefix `devin/`; fallback list tĩnh khi chưa login | Catalog sync (F-9) | `DV-25` |
| **Setting: MCP 1-click** | Ghi config Google/Jira vào config provider | Ghi `mcpServers` vào `~/.config/devin/mcp_config.json` (dedicated file từ v3000.3), atomic merge | Config updater (F-11) | `DV-29` |
| **Setting: Account & Quota** | Panel hiển thị model, quota, auth | `devin auth status` + `credentials.toml`; ACP `authenticate` cho managed account; usage lấy từ `turn_completed._meta` | Load account metadata (F-6) | `DV-20`, `DV-30` |
| **Reasoning Effort** | Điều chỉnh mức độ suy nghĩ | Effort = suffix trong model ID (`-low/-medium/-high/-xhigh/-max`); switch qua `session/set_config_option{configId:"model"}` (F-20) | Live-verified | `DV-35` |
| **Token usage & Context** | `EventTokenUsageUpdated` + `ModelContextWindow` | Lấy tokens/cost từ `turn_completed._meta` (probe shape) hoặc `/session-stats` | Usage metadata trong ACP | `DV-24` |
| **Resume / History** | Session ID thật được lưu DB; resume sau restart | `ProviderSessionStore.UpsertSession` + `session/load` (`loadSession:true` F-2); discovery qua `session/list`/`devin list --format json` | Session ID lưu trong DB | `DV-12`, `DV-15` |
| **Cross-account Safety** | Định danh account an toàn | Cô lập môi trường biến per-account | Process env isolation | `DV-12`, `DV-14` |
| **Interrupt / Cancel** | `ctx.Done()` → hủy lệnh | ACP `session/cancel` → kết thúc turn ngay lập tức | Cancel RPC call | `DV-16` |
| **Vision Gating** | Giữ `Vision=false` cho tới khi kiểm chứng | Khóa nhận ảnh cho tới khi golden fixture chứng minh thành công | Gated capability flag | `DV-27` |

---

## 4. Work Breakdown (P-1..P-11 mapped to Task-400..403)

Phân chia công việc theo chuẩn CP-57 Work Breakdown §4. **Lưu ý: Key Decisions P-* (§3) và Work Breakdown P-* (§4) là hai namespace khác nhau.**

| Work Breakdown ID | Nhiệm vụ chính | Child Task tương ứng | Các quyết định kiến trúc bao phủ |
| :--- | :--- | :--- | :--- |
| `P-1` | Đóng băng wire contract của Devin ACP (Freeze ACP Contract) | Task-400 | `P-1` (ACP Transport Wire) |
| `P-2` | Dựng ACP transport + process/dispatcher độc lập | Task-400 | `P-2` (Dedicated Process), `P-3` (Child Isolation) |
| `P-3` | Xây dựng Adapter `devin_adapter.go` thực thi `ProviderRuntimeAdapter` | Task-401 | `P-2`, `P-6` (Honest Caps) |
| `P-4` | Xây dựng Event Mapper `devin_event_mapper.go` | Task-401 | `P-7` (Stream & Chunk Robustness) |
| `P-5` | Xử lý Permission Channel & YOLO Posture | Task-401 (MVP) + Task-403 (Full) | `P-4` (Permission Overlay), `P-8` (Denied Clean Exit) |
| `P-6` | Tích hợp công cụ `ask_user` + `spawn_agent` qua MCP | Task-403 | `P-3` (Child Process Scope Isolation) |
| `P-7` | Settings: Detect, Install, Models, MCP, Account, Quota | Task-402 | Cài đặt & cấu hình môi trường |
| `P-8` | Chat / Flow UI Parity (Desktop & TUI) | Task-403 | Giao diện, cards, model picker, posture |
| `P-9` | Quản lý Session Resume, History, Handoff | Task-401 (Persist) + Task-403 (Locate/Sync) | `P-5` (Midchat Switch & Resume) |
| `P-10` | Skill Injection, Context Injection, Flow Gates, Summarizer | Task-403 | Prompt assembly, gate rules, one-shot |
| `P-11` | Kiểm thử hồi quy, Golden Fixtures, Operator Notes | Task-400..Task-403 | `P-0` (Zero Base Regression), `DV-BR` |

- `P-1` **ACP JSON-RPC wire frames cho `devin acp`** (Task-400). Đóng băng các payloads thực tế từ `devin acp` (`initialize`, `session/new`, `session/update`, v.v.). Quyết định capability ban đầu.
- `P-2` **Process management (`devin_process.go`)** (Task-400). Dựng `devin_acp.go` + `devin_process.go` với dispatcher async có `waiters`, notification subs, cô lập theo account.
- `P-3` **Adapter (`devin_adapter.go`)** (Task-401). Hiện thực `ProviderRuntimeAdapter`, truyền đủ params từ `TurnRequest` sang `session/new`, có MCP ready gate.
- `P-4` **Event Mapper (`devin_event_mapper.go`)** (Task-401). Ánh xạ chuỗi chunk ACP sang `ProviderEvent` chuẩn hóa (`message_delta`, `tool_started`, `turn_completed`, tokens cost).
- `P-5` **Permission Channel & YOLO Posture** (Task-401/403). Map inbound `session/request_permission` sang `bridge.RequestApproval`. Cấu hình biến `DevinPermissionMode` trong `YoloPosture` — khuyến nghị client-side auto-approve (F-7).
- `P-6` **`ask_user` + `spawn_agent` via MCP** (Task-403). Expose công cụ FlowPilot qua ACP `session/new{mcpServers}` (stdio only, F-2) hoặc managed `mcp_config.json`. Ánh xạ native `spawn_agent` qua `TurnBridge`.
- `P-7` **Settings page — Detect/Install/Models/MCP/Account** (Task-402). Thêm `devin` vào `providerSpecs`, `providerInstallCommand` (curl/irm, F-12), đồng bộ models qua `devin models list --format json`, UI ghi `mcpServers` vào `~/.config/devin/mcp_config.json`.
- `P-8` **Chat / Flow UI parity** (Task-403). Cập nhật `ProviderKey` union, thẻ card UI, logic pick model/reasoning truyền từ `TurnRequest`.
- `P-9` **Resume / history / handoff** (Task-401/403). `ProviderSessionStore.UpsertSession` lưu ID thực; `LocateSessionFile` định vị file local Devin.
- `P-10` **Skill / context / flow gates / summaries** (Task-403). Gọi `promptPrep` dùng chung. Định hướng node tasks/bugfix sửa bằng Devin.
- `P-11` **Tests, operator notes, fallback policy** (Task-400..403). Bảo đảm `DV-BR` 100% xanh cho mọi file share.

---

## 5. Touched Areas & Base-Regression Guard

- **files (runner, extend):**
  - `apps/local-runner/internal/runner/provider_registry.go` (register devin, `providerKeyFromModel`)
  - `apps/local-runner/internal/runner/provider_event.go:10`
  - `apps/local-runner/internal/runner/yolo_resolver.go`
  - `apps/local-runner/internal/runner/provider_accounts.go`
  - `apps/local-runner/internal/runner/runner.go`
  - `apps/local-runner/internal/runner/interactive_service.go`
  - `apps/local-runner/internal/runner/summarizer.go`
  - `apps/local-runner/internal/runner/handoff_context.go`
  - `apps/local-runner/internal/runner/session_file_locator.go`
  - `apps/local-runner/internal/runner/chat_session_sync.go`
  - `apps/local-runner/internal/agentpack/pack.go` (`ModelProviderKey`)
  - `apps/local-runner/internal/runner/compat.go`
- **files (runner, new):**
  - `devin_acp.go`, `devin_process.go`, `devin_adapter.go`, `devin_event_mapper.go`, `devin_mcp_config.go`, `devin_reasoning.go`
- **files (desktop, extend):**
  - `types/contract.ts`, `components/settings/AiProvidersSettings.tsx`, `ProviderAccountsPanel.tsx`, `GoogleDriveSettings.tsx`, `ChatInput.tsx`, `AgentsPanel.tsx`, `visionProviders.ts`, `store.ts`
- **modules:** local runner provider runtime, ACP transport, desktop settings
- **database:** none (schema no change, enum on client only)
- **external systems:** devin CLI, devin acp

### 5.1 Base-Regression Guard (Bảo vệ tuyệt đối 5 Provider cũ theo P-0)

| File dùng chung | Loại thay đổi | Biện pháp bảo vệ hồi quy (Regression Guard) |
| :--- | :--- | :--- |
| `gemini_acp_transport.go`, `grok_acp.go`, `codex_adapter.go`, `claude_adapter.go`, `opencode_acp.go` | **TUYỆT ĐỐI KHÔNG SỬA** | Devin có module độc lập (`devin_acp.go`, `devin_process.go`). Không được refactor code cũ. |
| `provider_registry.go` | Additive (thêm `devin` vào `ProviderRegistryFor`, append prefix `devin/`) | `DefaultProviderRegistry` giữ nguyên không có Devin; test case `TestDefaultProviderRegistryHasNoDevin` chứng minh các provider cũ không đổi. |
| `provider_event.go` | Additive | Append `ProviderKeyDevin = "devin"` ở dòng cuối cùng của enum. |
| `yolo_resolver.go` | Additive | Append trường `DevinPermissionMode` vào struct `YoloPosture`; các trường cũ giữ nguyên. |
| `provider_accounts.go`, `runner.go` | Additive | Append `case "devin"` vào các switch-case. Test hồi quy kiểm tra kết quả trả về của 5 provider cũ không đổi. |
| `compat.go` | Additive | Append trường `CompatTestedDevinVersion="3000.10.31"` vào struct; không đổi thứ tự các trường để UI không bị lệch hàng. |
| `claude_mcp_server.go`, `agent_orchestrator.go` | **TÁI SỬ DỤNG, KHÔNG SỬA** | Dùng chung hạ tầng MCP Server sẵn có của runner. |
| Desktop shared (`contract.ts`, `store.ts`, `ChatInput.tsx`, `AiProvidersSettings.tsx`) | Additive | Thêm union `\| "devin"`, thêm thẻ card và màu brand, không làm thay đổi logic render của các thẻ cũ. |
| `interactive_service.go`, `summarizer.go`, `handoff_context.go`, `session_file_locator.go`, `chat_session_sync.go`, `agentpack/pack.go` | Additive | Thêm nhánh `case "devin"` ở cuối các switch-case. Logic tóm tắt, handoff, locate cho provider cũ giữ nguyên. |

### 5.2 Full Provider/Model Feature Mirror Checklist (28-Row Mirror Matrix)

| # | Hạng mục (Area) | Vị trí file trong Runner | Vị trí file trên Desktop / Client | Yêu cầu đối soát cho `devin` | Task |
|---|---|---|---|---|---|
| 1 | `ProviderKey` const + Capabilities | `runner/provider_event.go:10` | `desktop/types/contract.ts:11`, `packages/.../adminModels.ts` | Thêm `ProviderKeyDevin="devin"` + Capabilities `{Streaming:true, Resume:true, Interrupt:true, Vision:false, ApprovalEvents:false, FileEvents:true, SkillSelection:true, Mcp:false}` | Task-401 |
| 2 | `ProviderRegistry` / Registration | `runner/provider_registry.go` | `desktop/types/contract.ts:11` | Đăng ký trong `ProviderRegistryFor` chỉ khi `FLOWPILOT_DEVIN_AGENT=1`; `DefaultProviderRegistry` giữ nguyên. Quyết định `newAdapter` vs `newAdapterForTurn` sau Task-400 probe | Task-401 |
| 3 | `providerKeyFromModel` + Default Model | `agentpack/pack.go:338`, `runner/interactive_service.go`, `runner/workflow_state_machine.go` | `packages/.../adminLogic.ts` | Append prefix `devin/`; `defaultModelForProvider("devin") = "devin/swe-2-high"` (currentValue verified F-21; fallback `devin/adaptive`); thêm `resolveTurnModelAndEffort` | Task-403 |
| 4 | One-shot Summarizer | `runner/runner.go` (`resolvePromptExecutionAdapter`) | — | Nhánh `devin -p` (+ `--respect-workspace-trust false` cho untrusted cwd, `--model` cheap tier) hoặc `devin acp --agent-type summarizer` (F-8); chốt sau probe Task-400, `usesPromptArg=true` | Task-403 |
| 5 | Detection / Inventory | `runner/runner.go` (`providerSpecs`), `tooling/check.go` | `desktop/components/settings/AiProvidersSettings.tsx` | Thêm `{Key:"devin", Binary:"devin"}` + probe `devin --version` | Task-402 |
| 6 | Install Command | `runner/runner.go` (`runProviderInstallCommand`) | `desktop/AiProvidersSettings.tsx` | Nhánh `devin`: `curl -fsSL https://cli.devin.ai/install.sh \| bash` (mac/Linux); Windows `irm https://static.devin.ai/cli/setup.ps1 \| iex`; upgrade `devin update` (F-12) | Task-402 |
| 7 | Live Model Catalog Probe | `runner/runner.go` (`resolveProviderModels`) | `desktop/AiProvidersSettings.tsx` | `detectDevinModels()` qua `devin models list --format json` (auth-gated, fallback static); **mọi row lưu prefix `devin/`** — alias Devin (`opus`,`gpt`,`codex`,`swe`) trùng tên provider khác (F-9) | Task-402 |
| 8 | Managed Homes / Multi-account | `runner/provider_accounts.go` | `desktop/ProviderAccountsPanel.tsx` | Hỗ trợ `.devinHome` / `isValidDevinAccountPath` (check `credentials.toml` dưới `.local/share/devin/`); update `PROVIDERS` array `ProviderAccountsPanel.tsx` | Task-402 |
| 9 | Auth File Check | `runner/runner.go` (`hasValidProviderAuthFile`, `defaultAuthCandidates`) | — | `~/.local/share/devin/credentials.toml` + `devin auth status`; Windows `%APPDATA%\devin\` (F-6) | Task-402 |
| 10 | Env Isolation | `runner/runner.go` (`getEnvForExecution`) | — | `HOME` + `XDG_CONFIG_HOME` + `XDG_DATA_HOME` trỏ managed home (config `~/.config/devin`, data `~/.local/share/devin`); strip `WINDSURF_API_KEY`/secrets host; optional `--config <PATH>` (F-13) | Task-402 |
| 11 | Interactive Auth | `runner/provider_accounts.go` (`StartInteractiveAuth`) | `desktop/ProviderAccountsPanel.tsx` | `devin auth login` (`--force-manual-token-flow` cho SSH); lưu ý ACP vẫn cần `authenticate` per-process (F-3/F-10) | Task-402 |
| 12 | Reasoning / Effort Mapping | `runner/devin_reasoning.go` | `desktop/ChatInput.tsx` | Probe `session/set_config_option`/session mode; không có `--variant`; honest no-op nếu unsupported (Q-5) | Task-401/403 |
| 13 | YOLO / `YoloPosture` SSOT | `runner/yolo_resolver.go` | `desktop/store.ts`, `ChatInput.tsx` | Thêm `DevinPermissionMode`; khuyến nghị giữ CLI `auto`/`normal` + adapter auto-approve khi YOLO (không restart process khi toggle) (F-7) | Task-401 |
| 14 | Chat Posture (scan/plan/code) | `runner/chat_posture.go` | `desktop/ChatPosturePanel.tsx` | Ép read-only cho `scan`/`plan` qua permission `deny`/`ask` overlay; probe `session/set_mode` mode `plan` làm phương án sạch (Q-6) | Task-401 |
| 15 | Skill Selection Injection | `runner/runner.go` | `desktop/ChatInput.tsx` | Inject skill vào prompt qua `promptPrep` (SSOT runner); skill roots tham chiếu `.devin/skills/` + `~/.config/devin/skills/` (F-13) | Task-403 |
| 16 | Runner-hosted MCP | `runner/devin_mcp_config.go` | `desktop/McpSettings.tsx`, `desktop/components/settings/GoogleDriveSettings.tsx` | Ghi `mcpServers` vào `~/.config/devin/mcp_config.json` (F-11) hoặc `devin mcp add -s user`; `session/new{mcpServers}` chỉ stdio (F-2); update `PROVIDER_LABELS` | Task-402 |
| 17 | Spawn Agent / Cohort | `runner/agent_orchestrator.go` | `desktop/AgentsPanel.tsx` | Hỗ trợ badge `isDevinSource`, hiển thị màu đặc trưng brand Devin qua `resolveMainAgentDisplay` + CSS | Task-403 |
| 18 | `LocateSessionFile` | `runner/session_file_locator.go` | — | Devin dùng SQLite `sessions.db` không có per-session file (F-5) → bypass như CA-688 + `isDevinRealSessionID` + reject `thread-*`; discovery qua `session/list` | Task-403 |
| 19 | Session Store `UpsertSession` | `runner/interactive_service.go` | — | `ProviderSessionStore` lưu real `sessionId` (format probe Q-1); DB-backed transcript condition | Task-401 |
| 20 | History Replay Loader | `runner/devin_transcript_loader.go` | `desktop/Timeline.tsx` | Nguồn transcript: SQLite `sessions.db` hoặc `--export` ATIF (F-5/F-8); replay khi mở lại chat cũ | Task-403 |
| 21 | Cross-account Resume | `runner/chat_session_sync.go` | — | `BuildChatSessionSyncManifest`: session Devin nằm trong `sessions.db` — quyết định sync DB-level hay skip-to-metadata (probe T-2b CP-57 analog) | Task-401 |
| 22 | Summarizer Cheap Tier | `runner/summarizer.go` (`summarizerModelFor`) | — | `devin/swe-1-6-fast` (default rẻ) hoặc `devin/adaptive`; `--agent-type summarizer` không cần model flag | Task-403 |
| 23 | Token Usage Tracking | `runner/devin_event_mapper.go` | `desktop/ChatInput.tsx` | Lấy `tokens`/`cost` từ `turn_completed` và emit `EventTokenUsageUpdated` | Task-401 |
| 24 | Context Window Tokens | `runner/types.go` | `desktop/ChatInput.tsx` | Populate nếu `devin models list --format json` expose context window; nếu không → `ModelContextWindow=nil` + tooltip N/A (CA-683 analog) | Task-402 |
| 25 | Compat Version Check | `runner/compat.go` | `desktop/CheckVersionSettings.tsx` | `CompatTestedDevinVersion="3000.10.31"` (live-verified), hiển thị hàng thứ 5 trên UI Settings | Task-402 |
| 26 | Agent Catalog | `runner/agent_catalog.go` | `desktop/AgentsPanel.tsx` | Đọc thư mục `.devin/agents` hiển thị danh mục agent | Task-403 |
| 27 | TUI Picker & Cards | `internal/tui/app` | `desktop/ChatInput.tsx` | Thêm icon `[D]`, thẻ card Devin trong danh sách chọn provider | Task-403 |
| 28 | Image Attachment Gating | `runner/devin_adapter.go` | `desktop/ChatInput.tsx`, `visionProviders.ts` | Chặn gửi ảnh trước khi verify khả năng multimodal (`Vision: false`); cấu hình `supportsVisionFor` | Task-401 |

---

## 6. Data or Migration Steps

1. **Database Migration (Supabase)**:
   - schema: none, database: no schema change. Enum chỉ tồn tại phía client/runtime (contract.ts, adminModels.ts, provider_event.go). Rows ai_supported_models được populate qua CLI sync (detectDevinModels).
2. **Cấu hình Client**:
   - Thêm `"devin"` vào union `ProviderKey` tại `apps/desktop-flowpilot/src/types/contract.ts` và `packages/flowpilot-client-core`.
3. **Cấu hình Môi trường**:
   - `FLOWPILOT_DEVIN_AGENT`: Bật/tắt provider Devin, mặc định `1` trên dev.
   - `FLOWPILOT_DEVIN_BIN`: Cho phép ghi đè đường dẫn binary `devin` phục vụ mock testing.

---

## 7. Validation Plan

### 7.1 Automated Unit & Integration Tests (Tương đương OC-01..OC-35)

- `DV-01` **Normal Chat**: Devin stream `message_delta`, kết thúc bằng `message_completed` + `turn_completed`, lưu trữ `sessionId` thật.
- `DV-02` **Task / Bugfix**: Giữ nguyên `changeType=task|bugfix` và chuyển tiếp đến finalizer.
- `DV-03` **Model Routing**: `devin/*` chuyển hướng đúng sang `ProviderKeyDevin` qua `workflow_step_auto`.
- `DV-04` **Approval Gate (YOLO=OFF)**: Thao tác sửa file / chạy shell phát `permission_required`; Deny chặn thao tác (áp dụng bài học BUG-331 / CA-690).
- `DV-05` **YOLO Policy (YOLO=ON)**: Tự động duyệt thao tác đủ điều kiện; lệnh hỏi user không bao giờ bị auto-approve.
- `DV-06` **User Question (`ask_user`)**: Devin gọi tool hỏi user -> phát `user_question_required` -> chờ phản hồi từ Desktop -> tiếp tục turn.
- `DV-07` **Child Spawn Isolation**: Gọi `spawn_agent` -> spawn child trên tiến trình ACP độc lập `account|child:<runId>`, không làm đứt kết nối MCP parent (áp dụng bài học BUG-334).
- `DV-08` **Skill Injection**: Chèn đúng nội dung và thứ tự các skill được chọn vào prompt context.
- `DV-09` **Context Injection**: Tự động chèn feature history + change audit + summary trước khi gửi turn.
- `DV-10` **Flow Rules & Gates**: Kiểm duyệt `r-ca`/`r-bug`/`r-task` qua `finishTurn`; repair prompts chạy trực tiếp trên Devin.
- `DV-11` **Tool Events**: Ánh xạ chuẩn `tool_started`, `tool_completed` và phát hiện `EventFileChanged` từ file diffs.
- `DV-12` **Midchat Model Switch**: Chuyển model giữa cuộc hội thoại qua `session/load` mang theo `sessionId` hợp lệ mà không văng lỗi (áp dụng bài học BUG-329).
- `DV-13` **Unknown Action Gate**: Chặn các hành động không xác định một cách an toàn.
- `DV-14` **Cross-account Safety**: Ngăn chặn rò rỉ session giữa các account Devin khác nhau.
- `DV-15` **Session Resume Persistent**: Đảm bảo khôi phục session mượt mà sau khi restart ứng dụng.
- `DV-16` **Interrupt**: Hủy context (`ctx.Done()`) -> phát ACP `session/cancel` -> kết thúc turn ngay lập tức.
- `DV-17` **MCP Readiness Gate**: Chặn gửi `session/prompt` cho đến khi danh sách tool MCP của FlowPilot sẵn sàng.
- `DV-18` **Settings Detect**: Kiểm tra `devin --version` + thông báo trạng thái `ok` hoặc `missing` kèm hướng dẫn cài đặt.
- `DV-20` **Account Metadata Sync**: Cập nhật thông tin token/tài khoản chính xác lên UI Settings.
- `DV-22` **Child Spawn Wait**: Chờ đợi child agent thực thi và trả kết quả ổn định.
- `DV-24` **Token Usage Tracking**: Trích xuất đúng số tokens/cost từ `turn_completed._meta` và cập nhật giao diện.
- `DV-25` **Model Catalog Sync**: Đồng bộ danh mục model từ Devin vào bảng `ai_supported_models`.
- `DV-27` **Vision Guard**: Chặn gửi file ảnh khi `Capabilities().Vision == false` kèm thông báo rõ ràng (áp dụng bài học CA-692).
- `DV-29` **MCP Config Write**: Ghi cấu hình MCP servers (`flowpilot`, `google-drive`) vào config của Devin an toàn.
- `DV-30` **Account Metadata**: Hiển thị thông tin tài khoản, email, plan trên Settings.
- `DV-35` **Reasoning Mapping**: Ánh xạ effort FlowPilot → suffix model Devin (`low`→`-low`, `medium`→`-medium`, `high`→`-high`, `xhigh`→`-xhigh`, `max`→`-max`) qua `set_config_option{configId:"model"}` (F-20); family không có suffix (vd `swe-1-6`) → no-op an toàn.
- `DV-36` **ACP Authenticate Handshake**: Sau `initialize`, adapter gọi `authenticate` đúng method trước `session/new`; thiếu auth → typed `auth_required` (F-3).
- `DV-37` **Extension Notifications Tolerance**: Dispatcher bỏ qua êm `_cognition.ai/*` và mọi method lạ mà không làm fail turn (F-4).
- `DV-38` **Session Discovery**: `session/list` verified — trả `{sessionId,cwd,title,updatedAt}` theo **canonical cwd** (F-24); `session/load` verified trả modes+configOptions; sessionId slug `adjective-noun` (F-18).
- `DV-BR` **Base Regression Guard**: Chạy lại toàn bộ test suite của 5 provider cũ (Codex, Claude, Gemini, Grok, Opencode) đảm bảo pass 100%.

### 7.2 E2E Test Matrix (27 ca kiểm thử đầu-cuối tương đương E2E-01..27 của CP-57)

| Mã E2E | Phạm vi | Mô tả ca kiểm thử | Kết quả mong đợi |
| :--- | :--- | :--- | :--- |
| `E2E-01` | Basic chat | Chat cơ bản với Devin trên Desktop app, stream tin nhắn | Chữ stream mượt, kết thúc turn đúng, lưu history |
| `E2E-02` | Skill injection | Chọn 1 hoặc nhiều skill và gửi prompt | Skill context xuất hiện chuẩn xác trong prompt gửi đi |
| `E2E-03` | Context summary | Chat dài -> bấm `Gen summary` -> chat tiếp | Summary được chèn vào turn tiếp theo không bị trôi ngữ cảnh |
| `E2E-04` | Same-account resume | Gửi turn liên tiếp trong cùng session | Dùng lại `sessionId` cũ thông qua process dispatcher |
| `E2E-05` | Restart resume | Tắt runner/app, khởi động lại và mở chat cũ | Khôi phục phiên chat mượt mà qua `session/load` (CA-688) |
| `E2E-06` | Cross-account safety | Chat ở tài khoản A -> chuyển tài khoản B -> resume | Chặn resume chéo tài khoản, báo lỗi typed mismatch |
| `E2E-07` | Synthetic resume guard | Thử resume với synthetic ID không hợp lệ | Từ chối load session synthetic, khởi tạo phiên mới an toàn |
| `E2E-08` | Prompt-result session id | ACP `turn_completed` trả về session ID mới | Tự động ghi nhận session ID mới vào session store |
| `E2E-09` | Approval gate | YOLO tắt, yêu cầu sửa file hoặc chạy lệnh bash | Hiện card xin quyền, bấm Deny chặn đứng thao tác (BUG-331) |
| `E2E-10` | YOLO policy | YOLO bật, yêu cầu sửa file | Tự động duyệt thao tác sửa file; câu hỏi user vẫn chặn hỏi |
| `E2E-11` | Tool events | Kích hoạt công cụ của Devin | Hiển thị đầy đủ tool card và thời gian thực thi trên UI |
| `E2E-12` | Flow gates | Chạy flow task/bugfix vi phạm quy tắc `r-ca` | Gate cảnh báo vi phạm; repair prompt chạy trên Devin |
| `E2E-13` | Summary generation | Chạy tóm tắt phiên chat | Tạo summary thành công qua lệnh one-shot `devin -p` hoặc `devin acp --agent-type summarizer` (F-8) |
| `E2E-14` | Handoff target | Chuyển giao ngữ cảnh từ Claude sang Devin | Devin nhận đủ context và tiếp tục công việc |
| `E2E-15` | Handoff source | Chuyển giao từ Devin sang provider khác | Tạm khóa (giữ `false`) cho đến khi kiểm chứng extractor |
| `E2E-16` | Capability flags | Kiểm tra cờ capabilities sau khi chạy | Chỉ báo `true` những tính năng đã kiểm chứng |
| `E2E-17` | Child spawn | Gọi `spawn_agent` chạy sub-agent nền | Child agent chạy trên process riêng, không đứt kết nối (BUG-334) |
| `E2E-18` | Failure recovery | Giả lập lỗi mạng hoặc crash tiến trình CLI | Báo lỗi rõ ràng, không treo turn ở trạng thái running (BUG-361) |
| `E2E-19` | Account metadata | Xem thông tin tài khoản trên Settings | Hiển thị đúng email/plan từ CLI config |
| `E2E-21` | External MCP | Kết nối Google Drive MCP với Devin | Devin nhìn thấy và gọi được các tool của Google Drive |
| `E2E-22` | Child lifecycle | Kiểm tra vòng đời child agent (approval, status) | Trạng thái child agent hiển thị đồng bộ trên panel |
| `E2E-23` | Drive sync/restore | Thử đồng bộ phiên chat qua Google Drive | Phục hồi phiên chat an toàn nếu có session metadata |
| `E2E-24` | Token/context UI | Quan sát số token và cost sau turn | Thanh token hiển thị số liệu thực tế từ turn |
| `E2E-25` | Model metadata | Chọn các model khác nhau của Devin | Tên hiển thị và alias model phân giải chính xác |
| `E2E-26` | History replay | Mở lại các chat cũ có chứa approval và tool | Lịch sử tái hiện đầy đủ thẻ tool và trạng thái duyệt |
| `E2E-27` | Attachments fallback | Kéo thả ảnh vào khung chat | Bị chặn trước khi gửi kèm thông báo `Vision=false` (CA-692) |

---

## 8. Rollout and Fallback

- **Trình tự triển khai (Rollout Order)**:
  1. Giữ `DefaultProviderRegistry` không có Devin; chỉ bật trong `ProviderRegistryFor` khi có cờ `FLOWPILOT_DEVIN_AGENT=1`.
  2. Triển khai Task-400 (Transport & Process) -> Task-401 (Adapter MVP) -> Task-402 (Settings) -> Task-403 (Chat/Flow UI).
  3. Chạy toàn bộ test `DV-*`, `DV-BR` và chạy nghiệm thu manual theo checklist `E2E-01..27`.
- **Cơ chế dự phòng (Fallback Path)**:
  - Nếu tiến trình `devin acp` không khởi động được, trả về lỗi `UnsupportedProviderRuntimeError` kèm hướng dẫn kiểm tra CLI, không làm crash runner.
  - Nếu `authenticate` thất bại hoặc account chưa login, trả về lỗi typed `auth_required` kèm hướng dẫn `devin auth login` (F-3), không để turn treo.
  - Nếu kết nối MCP gặp sự cố, tạm thời vô hiệu hóa cờ `Capabilities.Mcp` để chat thông thường vẫn hoạt động được.
  - Nếu `session/load` bị từ chối (session không portable giữa account/cwd), fallback `session/new` + context summary injection (P-5).

---

## 9. Risks & Mitigations

- `R-1`: **CLI Devin thay đổi giao thức ACP qua các bản cập nhật**.
  *Giảm thiểu*: Ghim phiên bản kiểm thử tương thích `CompatTestedDevinVersion`, lưu bộ golden fixtures để phát hiện sai lệch schema.
- `R-2`: **Tự ý sửa file khi YOLO=OFF do CLI mặc định allow-all (Bài học BUG-331)**.
  *Giảm thiểu*: Cấu hình overlay permission chặt chẽ ngay khi khởi chạy tiến trình `devin acp`.
- `R-3`: **Sub-agent làm đứt kết nối MCP của Parent Agent (Bài học BUG-334)**.
  *Giảm thiểu*: Bắt buộc áp dụng Child Process Scope Isolation (`account|child:<runId>`), tuyệt đối không chia sẻ chung tiến trình ACP giữa parent và child.
- `R-4`: **Treo turn khi hết hạn mức hoặc ngắt mạng (Bài học BUG-361)**.
  *Giảm thiểu*: Bổ sung turn timeout và heartbeat monitor trong `devin_process.go`.
- `R-5`: **Mất phiên khi đổi model giữa chừng (Bài học BUG-329)**.
  *Giảm thiểu*: Nạp lại phiên qua `session/load` kèm `sessionId` hợp lệ, có fallback đóng gói lại context nếu model mới yêu cầu phiên mới.
- `R-6`: **Vi phạm nguyên tắc P-0 làm ảnh hưởng các provider cũ**.
  *Giảm thiểu*: Tất cả file dùng chung chỉ append nhánh `case "devin"` ở cuối cùng, chạy bộ test hồi quy `DV-BR` trước khi merge.
- `R-7`: **ACP `authenticate` bắt buộc** (F-3/F-17) — verified PKCE browser flow hoàn tất ~3s khi browser đã login app.devin.ai; rủi ro còn lại: managed account chưa từng login browser sẽ cần PKCE thật (mở browser) hoặc REPL `devin auth login` trước.
  *Giảm thiểu*: Probe params `authenticate` trong Task-400 (token method?); nếu chỉ browser, mở browser một lần qua `StartInteractiveAuth` và kiểm chứng token persist cho các lần spawn sau; blocker → escalate trước khi làm Task-401.
- `R-8`: **Alias model Devin trùng tên provider khác** (`opus`/`sonnet`/`codex`/`gemini`/`gpt`/`swe`) (F-9) — lưu thiếu prefix `devin/` sẽ route nhầm provider (BUG-330 chiều ngược).
  *Giảm thiểu*: `detectDevinModels` bắt buộc ghi `devin/<name>`; `providerKeyFromModel` chỉ nhận prefix; test `TestDevinModelPrefixRequired`.
- `R-9`: **Permission mode là launch-time (`DEVIN_PERMISSION_MODE` env), YOLO toggle lại là per-turn** — restart process khi `/yolo` sẽ tái hiện BUG-329.
  *Giảm thiểu*: Giữ process ở `auto`/`normal`, auto-approve `session/request_permission` trong adapter theo `DevinPermissionMode` — không restart process khi toggle YOLO (F-7).

---

## 10. Definition of Done (DOD)

- A new Devin adapter exists in the runner and implements `ProviderRuntimeAdapter` (`provider_registry.go:88`).
- Live runner registry (`ProviderRegistryFor`) can return a real Devin adapter; `DefaultProviderRegistry` remains **without** Devin until Task-401 gate.
- Devin desktop chat runs through the same `/client/workflow-runs/.../turns` flow as Codex/Claude/Grok; no Devin-only endpoint.
- Devin emits normalized `ProviderEvent` into the shared stream; no raw ACP messages reach desktop.
- Devin session persistence + resume via real session ID (**slug `adjective-noun`**, verified F-18/F-24 — `session/load` trên `sore-router` trả modes+configOptions) is documented and tested (synthetic `thread-*` never resumes).
- ACP boot sequence `initialize → authenticate → session/new` implemented and documented (F-3); auth failure maps to typed `auth_required` with `devin auth login` guidance.
- Capability reporting is truthful (`Streaming/Resume/Interrupt` gated by ACP proof; `Vision=false` until proven; `Mcp`/`ApprovalEvents`/`FileEvents` only when `P-5`/`P-6`/`P-4` pass).
- Legacy Codex/Claude/Grok/Gemini/Opencode paths stay green; every shared file touched has regression coverage (`DV-BR`).
- Settings page: detect installed (`devin --version`), one-click install correct version, `Detect models` syncs `devin/*` into `ai_supported_models`, Google/Jira MCP one-click writes `devin config`, account card shows stats.
- Chat/Flow: model (`devin/*` prefix), reasoning (effort→model suffix qua `set_config_option`, F-20), YOLO (`mode:"bypass"` per-session, F-22; `DEVIN_PERMISSION_MODE=dangerous` là phương án launch-time), `chatPosture` (`scan`→`ask`, `plan`→`plan`, `code`→`accept-edits`) all propagate per turn; `ask_user` / approval / tool / `file_changed` cards render through `TurnBridge` identically to Grok/Claude/Opencode.
- `r-ca`/`r-bug`/`r-task` and flow-gate repair prompts run correctly after Devin turns; Devin can `spawn_agent` with `wait` true/false and agent panel parity.
- Summaries (manual + idle) work for Devin chats via `summarizerModelFor`.
- End-to-end manual run records one real Devin desktop run covering chat, approval-gated action, spawned child, summary, flow gate, and resume.

### 10.1 DOD Verification Checklist (Khớp 100% chuẩn CP-57 §10.1)

| Tiêu chí DOD | Phương pháp xác minh bắt buộc |
| :--- | :--- |
| **Adapter hiện thực `ProviderRuntimeAdapter`** | Unit test `TestDevinAdapterImplementsProviderRuntimeAdapter` khởi tạo `newDevinAdapter`, assert `Key() == ProviderKeyDevin`, `Capabilities()` rõ ràng, chạy `SendTurn` với fake transport và golden fixtures. |
| **Live Registry trả về adapter thật** | Test `TestProviderRegistryFor_DevinEnabled` chứng minh `DefaultProviderRegistry` không có Devin; `ProviderRegistryFor` chỉ trả về Devin khi có cờ `FLOWPILOT_DEVIN_AGENT=1`. |
| **Chat Desktop dùng chung turn endpoint** | Kiểm tra network panel trên Desktop app khi chat với Devin: gọi `POST /client/workflow-runs/.../turns`, không có endpoint riêng `/devin/*`. |
| **Chỉ phát Normalized Events** | Test `TestDevinEventMapper_ChunksToEvents` chứng minh toàn bộ payload thô từ ACP được ánh xạ thành `ProviderEvent` chuẩn (`message_delta`, `tool_started`, `permission_required`, v.v.). |
| **Lưu trữ phiên & Resume bền vững** | Test `TestDevinSessionStore_PersistAndResume` lưu trữ `sessionId` thật vào DB Supabase, restart runner, nạp lại chat cũ thành công qua `session/load`. |
| **Trung thực về tính năng (Capability Truthfulness)** | Test `TestDevinAdapter_CapabilitiesTruth` kiểm tra mọi cờ `true` đều có bài test chứng minh; cờ `Vision` giữ `false`. |
| **Bảo vệ hồi quy tuyệt đối (DV-BR)** | `TestDevinBaseRegression_ZeroImpact` `go test ./internal/runner` xanh 100% cho toàn bộ bài test của Codex, Claude, Gemini, Grok, Opencode sau khi thêm nhánh Devin. |
| **Settings: Phát hiện CLI** | Test `TestDevinCheckTool_DetectAndMissing` `CheckTool("devin")` báo `ok` khi có binary và `missing` kèm hướng dẫn cài đặt khi chưa có. |
| **Settings: Cài đặt 1-click** | Test `TestDevinInstallCommand_CorrectArgs` cho Devin chạy đúng câu lệnh cài đặt chính thức. |
| **Settings: Đồng bộ Model** | Test `TestDevinDetectModels_SyncToSupported` quét danh mục model Devin và đồng bộ thành công vào bảng `ai_supported_models`. |
| **Settings: Ghi MCP 1-click** | Test `TestDevinMcpConfigWrite_DriveAndJira` bấm kết nối Google Drive ghi đúng cấu hình `mcpServers` vào file config của Devin. |
| **Settings: Account & Quota** | Test `TestDevinAccountMetadata_PanelDisplay` hiển thị thông tin tài khoản, email, gói cước trên giao diện Settings. |
| **YOLO=OFF kiểm duyệt chặt chẽ** | Test `TestDevinAdapter_ApprovalGate_YoloOff` thao tác nguy hiểm kích hoạt `permission_required`, bấm Deny thì thao tác không được thực thi (khắc phục triệt để BUG-331). |
| **YOLO=ON tự động duyệt đúng chính sách** | Test `TestDevinAdapter_AutoApprove_YoloOn` các hành động hợp lệ được auto-approve; các lệnh hỏi user (`ask_user`) vẫn chặn hỏi bình thường. |
| **Thẻ hỏi User (`ask_user`)** | Test `TestDevinAdapter_AskUser_BlockAndResume` Devin gọi `ask_user` -> hiện thẻ câu hỏi -> user trả lời -> turn tiếp tục. |
| **Cô lập Child Agent (`spawn_agent`)** | Test `TestDevinProcess_ChildIsolation` Devin gọi `spawn_agent` -> child chạy trên process riêng, parent không bị đứt kết nối MCP (khắc phục triệt để BUG-334). |
| **Đổi model giữa chat** | Test `TestDevinAdapter_MidchatModelSwitch` đang chat đổi model -> cuộc hội thoại tiếp tục trôi chảy không bị lỗi session (khắc phục triệt để BUG-329). |
| **Skill & Context Injection** | Test `TestDevinPromptPrep_SkillAndContext` kiểm tra prompt gửi tới Devin chứa đầy đủ skill và context theo đúng thứ tự. |
| **Flow Rules & Gates** | Test `TestDevinFlowGates_Violations` kiểm tra các vi phạm `r-ca`/`r-bug`/`r-task` kích hoạt gate và prompt sửa lỗi chạy trực tiếp trên Devin. |
| **One-shot Summarizer** | Test `TestDevinSummarizer_OneShot` tóm tắt phiên chat chạy thành công qua `devin -p` (hoặc `--agent-type summarizer`), chốt flag output sau probe. |
| **Xử lý từ chối quyền êm thấm** | Test `TestDevinAdapter_CleanDenyExit` khi user Deny quyền, adapter gửi mã từ chối hợp lệ và kết thúc turn lịch sự (khắc phục triệt để CA-712/713). |
| **Ghi nhận phiên chạy thực tế E2E** | Ghi lại biên bản 1 phiên chạy thật trên Desktop bao gồm chat, duyệt quyền, spawn child agent, tóm tắt và khôi phục sau restart. |
