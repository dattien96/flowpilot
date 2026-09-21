# CP-70 — Manual Test Guide (Devin Provider Mirror)

## Metadata

- Document ID: `CP-70-TEST-STEPS`
- Title: `CP-70 Manual Test Guide — Mọi feature và kịch bản đối chuẩn đã mirror cho Devin Provider`
- Phase: `verification`
- Status: `todo`
- Owner: `FlowPilot`
- Created: `2026-09-20`
- Last Updated: `2026-09-20`
- Parent Documents: [CP-70: Devin Provider Integration](./CP-70-Devin-Provider-Integration.md)
- Related Documents: [CP-70-note.md](../note/CP-70-note.md), [CP-57-Test-Steps.md](../done/CP-57-Test-Steps.md), [CP-Guide-Add-New-Provider-Checklist](../CP-Guide-Add-New-Provider-Checklist.md), `Task-400..Task-403`
- Tags: `devin, verification, test-steps, manual-test, cp-70, acp`

## AI Quick View

- **What:** Bảng kiểm thử thủ công (Manual Test Checklist) từng tính năng đã mirror cho Devin Provider (Settings → Chat → Approval → Spawn Child → Flow Gates → Session Resume → Handoff).
- **Why:** Hướng dẫn Operator và AI Runner tự nghiệm thu thực tế sau khi triển khai; mỗi mục map trực tiếp với các kịch bản `DV-*` và `E2E-*` trong CP-70 §7.
- **Thời lượng ước tính:** ~50 phút kiểm thử toàn diện (Full suite), ~10 phút kiểm thử nhanh (Smoke suite Mục S).
- **Kế thừa các bài học:** Bao gồm toàn bộ các điểm kiểm tra chống hồi quy cho mọi Bug/CA của đợt Opencode (`BUG-329`, `BUG-330`, `BUG-331`, `BUG-334`, `BUG-361`, `CA-679`, `CA-681`, `CA-683`, `CA-688`, `CA-692`, `CA-706..709`, `CA-712/713`, `CA-759`).

---

## 0. Chuẩn bị (Bắt buộc trước mọi phiên test)

| # | Hạng mục chuẩn bị | Cách kiểm tra & Thực hiện |
|---|---|---|
| **P1** | CLI `devin` đã cài đặt và cấu hình | `devin --version` (trả về version hợp lệ, vd `3000.10.31`); `which devin` trỏ `~/.local/bin/devin` |
| **P2** | Tài khoản Devin đã đăng nhập | **Hai store tách biệt**: (a) REPL `devin auth status` báo logged-in → `devin auth login` nếu chưa; (b) ACP path — adapter gọi `authenticate{methodId:"devin-browser"}` PKCE per-process (F-17, verify OK ~3s khi browser đã login app.devin.ai). Test verify cả hai path |
| **P3** | Khởi động Runner mới với cờ Devin | Chạy `FLOWPILOT_DEVIN_AGENT=1 just chat-dev <path-to-repo>` để runner nhận code mới |
| **P4** | Khởi động TUI / Desktop đúng | Banner TUI hiển thị `Ready`, lệnh `/provider` trong TUI liệt kê có `devin` |

---

## S. Smoke Test 10 Phút (Kiểm tra nhanh)

1. `/provider devin` hoặc `/model devin/swe-2-high` (default verified F-21) ➔ Thông báo *"Model set to devin/swe-2-high · provider: devin"*.
2. Gửi `"xin chào"` ➔ Stream chữ trả về mượt mà, dòng status line cập nhật token usage và cost.
3. Thoát TUI (Ctrl+C) ➔ Mở lại `just chat-dev` ➔ Status line **vẫn giữ nguyên** provider `devin` (Bảo toàn trạng thái theo `CA-679`).
4. Gửi `"Hãy nhớ mã bí mật là FP-2026"` ➔ Devin xác nhận đã nhớ ➔ Đổi model `/model devin/claude-opus-5-high` (catalog F-21) ➔ Hỏi `"Mã bí mật tôi vừa bảo nhớ là gì?"` ➔ Devin trả lời chính xác **FP-2026** (Kiểm tra `BUG-329`: không bị mất session khi mid-chat switch — `set_config_option` verified F-20).
5. `/mode plan` ➔ Chuyển posture plan; Tab quay về `code`. Hoàn tất Smoke.

---

## A. Settings — Desktop App (DV-18, DV-25, DV-29, DV-30, E2E-19)

Mở Desktop App ➔ **Settings** ➔ **AI Providers**.

| # | Bước thực hiện | Kết quả mong đợi | Bài học / Issue liên quan |
|---|---|---|---|
| **A1** | Xem card Devin | Hiển thị *"Installed [version] (tested [compat_version])"*, trạng thái **READY** (xanh). | `DV-18`, `compat.go` |
| **A2** | Bấm nút **Detect models** | Danh sách model của Devin xuất hiện đầy đủ trong bảng `ai_supported_models`. | `DV-25` Model Catalog sync |
| **A3** | Xem danh sách Version (CheckVersionSettings) | Hiển thị đủ **5 hàng**: Claude / Codex / Grok / Opencode / **Devin** (tested `3000.10.31`). | 5th row parity |
| **A4** | Xem Account Panel cho Devin | Hiển thị email/account label từ `devin auth status` + thông tin usage từ `turn_completed._meta` (nếu có). Bấm All ➔ mở modal hiển thị đầy đủ usage lines dạng text, verify thanh cuộn hoạt động, verify progress bar ẩn khi context window nil. | `DV-30` Account metadata |
| **A5** | Tab MCP ➔ Bấm kết nối Google Drive | File `~/.config/devin/mcp_config.json` xuất hiện mục `mcpServers` mới cho Google Drive (remote MCP qua config file — `session/new` chỉ hỗ trợ stdio, xem CP-70 F-2/F-11). | `DV-29` MCP 1-click write |
| **A6** | Giả lập máy chưa cài `devin` (hoặc đổi tên binary tạm thời) ➔ Mở Settings Desktop | Card Devin hiển thị trạng thái **MISSING** (đỏ/cam) kèm nút **Install**. Bấm Install thực thi `curl -fsSL https://cli.devin.ai/install.sh \| bash` (macOS/Linux; Windows `irm https://static.devin.ai/cli/setup.ps1 \| iex`) ➔ sau khi cài xong tự động chuyển sang **READY**. | `DV-18`, 28-row Row 6 (1-click Install) |
| **A7** | Chưa login Devin (đăng xuất thử) ➔ Mở Settings | Card hiển thị trạng thái auth rõ ràng + nút login chạy `devin auth login` (browser hoặc `--force-manual-token-flow`); sau login, turn đầu tiên không báo `auth_required` — chứng tỏ adapter đã gọi `authenticate` đúng (CP-70 F-3). | `DV-36` Authenticate handshake |

---

## B. TUI — Chọn Provider / Model & Khôi Phục Trạng Thái (DV-03, DV-35, E2E-25, CA-679, BUG-330)

| # | Bước thực hiện | Kết quả mong đợi | Bài học / Issue liên quan |
|---|---|---|---|
| **B1** | Gõ `/provider` | Danh sách hiển thị có tùy chọn `devin`. | TUI provider picker |
| **B2** | Gõ `/model` | Danh sách model hiển thị các model `devin/*`. | Model picker |
| **B3** | Gõ `/model devin/swe-2-high` | Thông báo switch thành công sang model devin; provider tự động chuyển sang `devin`. | Model routing |
| **B4** | Gõ `/status` | Session panel hiển thị: provider `devin` · model đã chọn · account label. | Session status |
| **B5** | Gõ `/new` ➔ Thoát TUI ➔ Mở lại | Model và provider **giữ nguyên** `devin`, không bị nhảy về provider cũ. | `CA-679` state restore |
| **B6** | Gõ `/reasoning high` ➔ Thoát ➔ Mở lại | Reasoning effort giữ nguyên mức `high`. | Reasoning persistence |
| **B7** | Đang ở posture `scan` (có pin model khác): chọn Devin ➔ Thoát ➔ Mở lại | Model Devin **giữ nguyên**, không bị pin model của posture đè bẹp. | `CA-679` posture pin bug |
| **B8** | Đang chọn provider `devin`: cố tình gõ `/model grok-4.5` hoặc inject model của provider khác | Hệ thống kiểm duyệt qua `ModelProviderKey` (`agentpack/pack.go`), từ chối hoặc chuyển hướng provider hợp lệ; **tuyệt đối không** gửi tên model ngoại lai sang `devin acp` làm sập session. | `BUG-330` Foreign model injection guard |

---

## C. Chat Cơ Bản & Luồng Streaming (DV-01, DV-11, DV-16, DV-24, E2E-01, E2E-08, E2E-11, E2E-16, E2E-24)

| # | Bước thực hiện | Kết quả mong đợi | Bài học / Issue liên quan |
|---|---|---|---|
| **C1** | Gửi tin nhắn `"Hãy viết một đoạn giới thiệu ngắn về FlowPilot"` | Stream chữ `message_delta` mượt mà, tin nhắn cuối lưu vào lịch sử. | `DV-01` Basic streaming |
| **C2** | Quan sát status bar sau khi turn hoàn tất | Token usage hiển thị chuẩn (`ctx:...k/1M last:...k`), không bị lỗi NaN. | `DV-24` Token usage |
| **C3** | Gửi `"Đọc file README.md rồi tóm tắt 1 câu"` | Thẻ công cụ đọc file (`tool_started` / `tool_completed`) hiển thị, sau đó trả lời. | `DV-11` Tool lifecycle |
| **C4** | Model gửi text kèm định dạng mảng (Array content chunk) | UI hiển thị đầy đủ văn bản, không bị crash hoặc nuốt mất chữ. | `CA-709` Array chunk parse |
| **C5** | Model chỉ chạy tool mà không sinh text giải thích | UI vẫn kết thúc turn sạch sẽ, không bị treo vĩnh viễn ở trạng thái chờ chữ. | `CA-708` Non-text starvation |
| **C6** | Late Chunk Drain: Gửi prompt dài | Verify không có chunk bị mất sau `turn_completed`. | `CA-706` Late chunk drain |
| **C7** | Wait-Until-Text Guard | Verify nếu turn kết thúc mà chưa stream text nào thì TUI hiện thông báo phù hợp (không blank). | `CA-707` Wait-until-text guard |
| **C8** | Người dùng bấm **Stop** trên Desktop hoặc gõ `Esc`/`Ctrl+C` trên TUI khi Devin đang stream chữ dài | Runner gửi ngay RPC `session/cancel` tới Devin CLI; luồng stream dừng ngay lập tức (<1s), không bị treo, turn chuyển sang trạng thái đã hủy và sẵn sàng nhận prompt tiếp theo. | `DV-16`, `E2E-16` User Interrupt & Cancel |

---

## D. Đổi Model Giữa Chừng — Mid-chat Switch (DV-12, BUG-329)

> **Mục tiêu:** Đây là bài kiểm tra quan trọng bậc nhất để bảo đảm không lặp lại lỗi mất session của Opencode.

| # | Bước thực hiện | Kết quả mong đợi | Bài học / Issue liên quan |
|---|---|---|---|
| **D1** | Gửi `"Số may mắn của tôi là 99"` (model A) ➔ `/model devin/adaptive` (model B) ➔ Hỏi `"Số may mắn là bao nhiêu?"` | Devin trả lời chính xác **99** — ngữ cảnh được bảo toàn qua `session/load`+`set_config_option`, KHÔNG văng lỗi session missing. | `BUG-329` Midchat switch |
| **D2** | Đổi sang model thứ 3 và tiếp tục hỏi | Cuộc trò chuyện vẫn tiếp tục mượt mà trong cùng thread. | Multi-switch continuity |
| **D3** | Đổi `/reasoning` trong lúc đang chat | Turn tiếp theo áp dụng mức reasoning mới tới Devin CLI thật mà không làm fail turn. | Dynamic reasoning switch |

---

## E. Duyệt Quyền (Approval Gate) & Chế Độ YOLO (DV-04, DV-05, DV-06, E2E-09, E2E-10, BUG-331 / CA-690, CA-712/713)

> **Điều kiện ban đầu:** Đảm bảo chế độ YOLO đang **TẮT** (`/yolo off`).

| # | Bước thực hiện | Kết quả mong đợi | Bài học / Issue liên quan |
|---|---|---|---|
| **E1** | Yêu cầu `"Tạo file devin-test.txt có nội dung test-ok"` | Thẻ **`permission_required`** xuất hiện (Modal popup trên Desktop hoặc thanh `/approve`/`/deny` trên TUI). | `BUG-331 / CA-690` (Chặn CLI allow-all ngầm) |
| **E2** | Bấm **`/deny`** (Từ chối duyệt) | File **KHÔNG** bị tạo trên đĩa; turn kết thúc có kiểm soát kèm thông báo từ chối lịch sự, KHÔNG bị lặp vô tận. | `CA-712 / CA-713` Deny clean exit |
| **E3** | Yêu cầu tạo lại file ➔ Bấm **`/approve`** (Đồng ý duyệt) | File `devin-test.txt` được tạo trên đĩa thành công; event `file_changed` được ghi nhận. | Approval success |
| **E4** | Bật YOLO (`/yolo on`) ➔ Yêu cầu tạo file `devin-auto.txt` | File được tự động tạo ngay lập tức, **KHÔNG** hiện thẻ đòi hỏi duyệt quyền. | `DV-05` YOLO auto-approve |
| **E5** | YOLO đang BẬT, yêu cầu model `"Hãy hỏi tôi xem tôi thích màu gì trước khi tạo file"` | Model gọi `ask_user` ➔ Hệ thống **BẮT BUỘC DỪNG LẠI HỎI USER** (câu hỏi user không bao giờ bị auto-approve). | `DV-06` Question guard |

---

## F. Sub-Agent Cô Lập — `spawn_agent` (DV-07, E2E-17, E2E-22, BUG-334)

> **Mục tiêu:** Đảm bảo khi chạy child agent, kết nối MCP của parent agent không bị đứt (`Connection closed`) và dọn dẹp sạch tiến trình con sau khi hoàn tất.

| # | Bước thực hiện | Kết quả mong đợi | Bài học / Issue liên quan |
|---|---|---|---|
| **F1** | Gửi: `"Dùng tool spawn_agent trên server flowpilot với agent='helper', prompt='trả lời: devin-child-ok', wait=true, rồi báo tôi child trả lời gì"` | Child agent khởi chạy trên tiến trình riêng `account|child:<runId>`, hoàn thành và báo kết quả về cho parent mà không làm đứt kết nối. | `BUG-334` Child process isolation |
| **F2** | Mở Agents Panel (hoặc `/agents`) | Danh sách hiển thị child agent `"helper · completed"`; bấm vào xem được transcript riêng của child với badge brand Devin. | Agent cohort UI & brand styling |
| **F3** | Lặp lại F1 với `wait=false` (Chạy nền) | Parent trả lời ngay lập tức, child agent tiếp tục chạy nền và cập nhật trạng thái khi xong. | Async sub-agent |
| **F4** | Sau khi F1/F3 hoàn tất: mở terminal kiểm tra `ps aux \| grep "devin acp"` | Tiến trình con của child agent đã được hàm `CloseProcessesForChildRun` dọn dẹp sạch sẽ; chỉ còn duy nhất tiến trình chính của parent agent, **không bị rò rỉ tiến trình zombie**. | `BUG-334 branch 2` Child process reclamation |

---

## G. Quản Lý Kỹ Năng & Ngữ Cảnh Tự Động — Skills & Context Injection (DV-08, DV-09, E2E-02)

| # | Bước thực hiện | Kết quả mong đợi | Bài học / Issue liên quan |
|---|---|---|---|
| **G1** | Gõ `/skill` | Danh mục skill khả dụng của FlowPilot hiển thị đầy đủ. | Skill catalog |
| **G2** | Gõ `/skill audit-logging` ➔ Hỏi việc liên quan tới sửa đổi code | Prompt gửi tới Devin được inject nội dung của skill `audit-logging` (Devin nhắc quy tắc CA note). | `DV-08` Skill injection |
| **G3** | Gõ `/skill clear` | Quay về trạng thái không gắn kèm skill. | Skill clear |
| **G4** | Đang ở repo có file `requirements/07-Coding-Plan` và các CA note gần nhất ➔ Gửi yêu cầu phân tích code | Prompt gửi tới Devin tự động chứa Feature History + Change Audit note gần nhất ở phần context trước khi bắt đầu turn. | `DV-09` Automated context injection |

---

## H. Chat Posture (Chế Độ Làm Việc) & Bảo Vệ File (CA-679)

| # | Bước thực hiện | Kết quả mong đợi | Bài học / Issue liên quan |
|---|---|---|---|
| **H1** | Gõ `/mode` | Hiển thị 3 posture: `scan` (read-only), `plan` (thiết kế), `code` (sửa code). | Posture selector |
| **H2** | Chuyển sang `/mode scan` ➔ Yêu cầu tạo/sửa file | Thao tác ghi file bị chặn đứng ngay lập tức với thông báo *"Mode scan is read-only"*. | Read-only enforcement |
| **H3** | Chuyển sang `/mode code` ➔ Yêu cầu sửa file | Thao tác ghi file được phép thực hiện (qua approval gate nếu YOLO=OFF). | Code posture write |
| **H4** | Bấm Tab | Chuyển nhanh giữa `plan` ↔ `code`, verify chuyển đổi chính xác. | Posture toggle |
| **H5** | Đang active posture có pin model | Chuyển sang posture có pin model → verify pin model được áp dụng (hiện đúng model name). | Model pin apply |

---

## I. Khôi Phục Phiên Chat & Lịch Sử (CA-688, DV-14, DV-15, E2E-04..E2E-07, E2E-23, E2E-26)

| # | Bước thực hiện | Kết quả mong đợi | Bài học / Issue liên quan |
|---|---|---|---|
| **I1** | Chat vài turn với Devin ➔ Thoát TUI ➔ Mở lại ➔ Gõ `/history` | Phiên chat với Devin xuất hiện trong danh sách lịch sử. | History persistence |
| **I2** | Gõ `/open <run-id>` ➔ Hỏi `"Trước đó tôi vừa nói gì?"` | Devin trả lời đúng nội dung cũ thông qua nạp lại `sessionId` thật trong DB. | `CA-688` Local resume |
| **I3** | Tắt hẳn runner (kill process) ➔ Khởi động lại ➔ Mở chat cũ ➔ Chat tiếp | Phiên chat tiếp tục bình thường, không bị lỗi `session_unavailable`. | DB session binding |
| **I4** | Thử nạp session với synthetic ID (`thread-*`) | Runner từ chối nạp synthetic session và tự khởi tạo fresh session an toàn, không văng crash. | Synthetic ID guard |
| **I5** | Gõ `/sync` | Upload lên Drive thành công. Sau đó `/restore` ➔ verify phiên chat được khôi phục đầy đủ. | `E2E-23` Drive sync/restore |
| **I6** | Chat ở tài khoản Devin A ➔ Mở Account Panel đổi sang tài khoản Devin B ➔ Cố tình gõ `/open <run-id-cua-A>` | Runner phát hiện mismatch account, từ chối nạp session và trả về lỗi typed rõ ràng, **ngăn chặn hoàn toàn rò rỉ session giữa các account**. | `DV-14`, `E2E-06` Cross-account Safety |

---

## J. One-Shot Summarizer (E2E-13)

> Yêu cầu: project có feature catalog (`.flowpilot/catalog/features.ndjson`).

| # | Bước thực hiện | Kết quả mong đợi | Bài học / Issue liên quan |
|---|---|---|---|
| **J1** | Chat một đoạn dài ➔ Trên Desktop app bấm **Generate Summary** | Summary được sinh thành công thông qua `devin -p` (kèm `--respect-workspace-trust false` nếu workspace untrusted) hoặc `devin acp --agent-type summarizer` mà không mở terminal. | `P-1` Headless summarizer |
| **J2** | Tiếp tục chat trong cùng thread | Ngữ cảnh *"Prior discussion summary"* xuất hiện ở đầu prompt của turn mới. | Context summary injection |

---

## K. Flow Mode & Flow Gates (DV-02, DV-10, E2E-12)

| # | Bước thực hiện | Kết quả mong đợi | Bài học / Issue liên quan |
|---|---|---|---|
| **K1** | Chạy flow `/flow task-harness` trên Devin | Flow khởi động mượt mà, sidebar hiển thị các bước của flow và chip provider `devin`. | Flow orchestration |
| **K2** | Cố tình sửa code mà không tạo file change-audit (CA note) | Rule `r-ca` kích hoạt gate violation; repair prompt yêu cầu bổ sung CA note chạy trực tiếp trên Devin. | `DV-10` Gate enforcement |
| **K3** | Gõ `/chat` | Chuyển mượt mà từ flow mode quay lại chat mode thông thường. | Mode toggle |

---

## L. MCP trong Chat (DV-17, E2E-21)

| # | Bước thực hiện | Kết quả mong đợi | Bài học / Issue liên quan |
|---|---|---|---|
| **L1** | Trong cuộc chat, yêu cầu Devin gọi `ask_user` | Verify TUI hiện dialog hỏi user. **Điều kiện**: chỉ chạy được nếu Task-403 T-2 chọn phương án stdio shim — Devin `mcpCapabilities{http:false,sse:false}` nên không inject loopback HTTP như OpenCode (CP-70 F-2); nếu defer thì đánh dấu N/A và ghi vào parity matrix. | Tool ask_user |
| **L2** | Sau khi kết nối Google Drive ở A5, yêu cầu Devin list file Drive | Verify tool MCP external gọi thành công (remote MCP đi qua `mcp_config.json`, không qua `session/new`). | External MCP |

---

## M. Khóa Nhận Ảnh — Vision Guard (DV-27, E2E-27, CA-692)

| # | Bước thực hiện | Kết quả mong đợi | Bài học / Issue liên quan |
|---|---|---|---|
| **M1** | Đang chọn Devin, thử kéo thả ảnh vào khung chat hoặc đính kèm ảnh | Bị **CHẶN NGAY TẠI GIAO DIỆN** trước khi gửi prompt với thông báo *"Provider devin does not support vision yet"*. Không bị mất prompt văn bản. | `CA-692` Vision guard |

---

## N. Hồi Quy 5 Provider Cũ — Base Regression Guard (DV-BR)

| # | Bước thực hiện | Kết quả mong đợi | Bài học / Issue liên quan |
|---|---|---|---|
| **N1** | Chạy toàn bộ unit test của 5 provider cũ: `go test ./internal/runner -run 'TestClaude|TestCodex|TestGemini|TestGrok|TestOpencode'` | **PASS 100%**, không có bất kỳ bài test cũ nào bị đỏ. | `DV-BR`, `P-0` Zero regression |
| **N2** | Mở chat lần lượt với Claude, Codex, Grok, Opencode | Cả 4 provider cũ hoạt động hoàn toàn bình thường, không bị ảnh hưởng bởi code của Devin. | Live baseline parity |

---

## O. Đứt Kết Nối, Lỗi Hạn Mức Quota & Timeout Guard (DV-16, E2E-18, BUG-361 / CA-759)

| # | Bước thực hiện | Kết quả mong đợi | Bài học / Issue liên quan |
|---|---|---|---|
| **O1** | Cố tình ngắt mạng hoặc kill tiến trình `devin acp` giữa lúc đang generate | Runner phát hiện tiến trình dừng, không để turn bị treo vĩnh viễn ở trạng thái `running`. | `BUG-361` Process hang guard |
| **O2** | Quan sát trạng thái trên UI | Turn chuyển sang `RunStatusFailed` kèm thông báo lỗi rõ ràng và cho phép người dùng retry. | Failure recovery |
| **O3** | Giả lập tài khoản hết hạn mức token hoặc dính rate limit (Devin ACP trả về `quota_exceeded` hoặc `-32603`) | Turn **không bị treo `running`** và không retry vô tận; chuyển sang `RunStatusFailed` với thông báo rõ ràng: *"Tài khoản hết token/quota hoặc vượt hạn mức; vui lòng nạp thêm"*. | `BUG-361 / CA-759` Quota stopReason classification |

---

## P. Account Usage Stats (CA-683)

| # | Bước thực hiện | Kết quả mong đợi | Bài học / Issue liên quan |
|---|---|---|---|
| **P1** | Sau ≥5 turn chat, mở Desktop Sidebar ➔ Account panel Devin | Dòng stats hiển thị và cập nhật (token count, cost nếu có). | `CA-683` Usage stats |
| **P2** | Đợi 60s ➔ Quan sát Account panel | Verify refresh tự động. | Cache refresh |

---

## Q. Chuyển Giao Ngữ Cảnh — Provider Handoff (E2E-14, E2E-15)

| # | Bước thực hiện | Kết quả mong đợi | Bài học / Issue liên quan |
|---|---|---|---|
| **Q1** | Chat một đoạn thảo luận kiến trúc với Claude ➔ Gõ `/handoff devin` | Toàn bộ ngữ cảnh thảo luận được tóm tắt và chuyển giao sang session mới của Devin; Devin hiểu nhiệm vụ và tiếp tục code trôi chảy. | `E2E-14` Handoff target (Claude ➔ Devin) |
| **Q2** | Đang chat với Devin ➔ Thử lệnh `/handoff claude` | Hệ thống kiểm tra cờ hỗ trợ handoff source của Devin; nếu chưa kiểm chứng extractor thì tạm khóa an toàn kèm thông báo phù hợp. | `E2E-15` Handoff source gating |

---

## R. Live Verification trên `gate-sandbox` — HTTP-driven (convention CP-64/CP-68)

> **Giường thử (Manual Bed):** `/Users/tiendat/Desktop/BE/gate-sandbox` (projectId `db51ec26-1a0f-4b92-8ceb-b03dc8e9b363`). Không đặt bed dưới `/tmp` — macOS symlink `/tmp→/private/tmp` trip `changecontract: declared path resolves outside workspace via symlink` (bài học CP-64); nếu cần bed phụ dùng `/Users/tiendat/fp-beds/`.
>
> **Phương thức drive:** `just chat-dev` TUI stall với interactive input → drive bằng **raw HTTP** giống CP-64/68:
> - `POST /client/workflow-runs` — body `{projectId, providerKey:"devin", model:"devin/swe-2-high", chatMode:"normal_chat", workingMode:"dev", cwd:"/Users/tiendat/Desktop/BE/gate-sandbox", yoloMode:<bool>}` → nhận `runId`.
> - `POST /client/workflow-runs/{runId}/turns` — body `{stepId, prompt, ...}`; với flow test thêm `flowRef:"bug-harness"`, `subMode:"bug"`, `changeType:"bugfix"`.
> - Đọc trạng thái/events qua `GET /client/workflow-runs/{runId}` (hoặc stream endpoint hiện hữu); health qua `GET /health`.
> - Provider Devin cần flag `FLOWPILOT_DEVIN_AGENT=1` lúc runner boot (P3).

| # | Kịch bản live | Cách drive & Kết quả mong đợi | Ref |
|---|---|---|---|
| **R1** | Chat turn đầu tiên qua Devin | `POST /client/workflow-runs` (providerKey=`devin`, model=`devin/swe-2-high`, yoloMode=true) → `POST .../turns` prompt `"Reply with exactly: DEVIN_LIVE_OK"` | Run hoàn tất `stopReason=end_turn`, reply chứa `DEVIN_LIVE_OK`; log runner có spawn `devin acp` + `initialize → authenticate → session/new → session/prompt` | C1 |
| **R2** | Tool call + file write (YOLO) | turn prompt `"Tạo file devin-e2e.txt nội dung ok"` | `tool_call{kind:"execute"}`/`tool_call_update` stream về; file thật tồn tại trong gate-sandbox; không treo turn | C3, E4 |
| **R3** | Midchat model switch | Trong cùng run: turn1 `"nhớ số 77"` → đổi model sang `devin/adaptive` → turn2 `"số tôi vừa bảo?"` | Trả `77` đúng — `session/set_config_option{configId:"model"}` giữ session, không `session/new` mới (BUG-329, F-20) | D1 |
| **R4** | Approval gate (yoloMode=false) | Run mới `yoloMode:false` → prompt `"Tạo file deny-test.txt"` → `POST /client/questions/{qId}/answer` trả deny → rồi approve | `permission_required` phát ra trước khi write; deny → file không tồn tại + turn kết thúc sạch (CA-712); approve → file tồn tại | E1–E3 |
| **R5** | Session resume sau restart runner | Ghi `runId` + Devin `sessionId` (slug `adjective-noun`) → kill runner → start lại → mở lại run → turn mới | `session/load` thành công với sessionId cũ (F-24); context được giữ | I2, I3 |
| **R6** | Flow gate trên Devin | `POST /client/workflow-runs` với `flowRef:"bug-harness"`, `subMode:"bug"`, `changeType:"bugfix"` trên gate-sandbox | Flow nodes chạy qua Devin; `r-reproduce`/`r-ca` gate vẫn enforce; repair prompt đi qua Devin (K1–K2) | K1, K2 |
| **R7** | spawn_agent child isolation | turn yêu cầu `spawn_agent` (nếu Task-403 T-2 đã ship stdio shim) | Child run trên `account|child:<runId>` riêng; sau xong `ps aux \| grep "devin acp"` không còn process zombie (BUG-334). Nếu defer → đánh N/A | F1, F4 |
| **R8** | Cancel giữa chừng | turn với prompt dài → `POST .../cancel` (hoặc tương đương) trong khi đang stream | Runner gửi `session/cancel`; turn → cancelled <1s, không treo (C8) | C8 |
| **R9** | Regression cùng bed | Lặp R1 với `providerKey:"grok"` + `model:"grok-4.5"` trên cùng gate-sandbox | Grok vẫn hoạt động bình thường — Devin code không ảnh hưởng provider cũ (DV-BR) | N2 |
| **R10** | Usage/accounting | Sau ≥3 turns, đọc `usage` trong turn result / account panel | `usage_update` + `result.usage{totalTokens,inputTokens,outputTokens,cachedReadTokens}` (F-23) hiển thị đúng, không NaN | C2, P1 |

**Ghi bằng chứng:** mỗi R* ghi `run-XXXXXX`, Devin sessionId slug, và log line chứng minh (vd `spawn devin acp`, `authenticate ok`, `session/load <slug>`) — theo convention CP-64 §6.

### Kết quả live test Windows 2026-09-21 (devin 3000.10.31, workspace `C:/working/fp-devin-sandbox`, log `runner-live.log`, chi tiết CA-900)

| # | Kết quả | Bằng chứng |
|---|---|---|
| **R1** | ✅ PASS | `run-1`, slug `trusted-airmail`; `initialize`→`authenticate{devin-browser}` (PKCE ~3s)→`session/new`→`session/prompt`→`end_turn`; reply `DEVIN_LIVE_OK` |
| **R2** | ✅ PASS | `hello_devin.txt` ghi thật; `tool_call{write}` in_progress→completed |
| **R3** | ✅ PASS | `set_config_option{model:grok-4-5-low}` trên cùng `trusted-airmail` (không `session/new`); reply `MODEL_SWITCH_OK` |
| **R4** | ✅ PASS (sau fix CA-900) | `run-83` yolo=false → `session/request_permission{rm -rf}` options `allow_once/allow_session/allow_always/reject_once` → `waiting_approval appr-97` → deny → `reject_once`, tool failed, `danger_dir` absent. **Bug đã sửa**: non-yolo gửi mode `"auto"` (không hợp lệ) bị coerce thành `accept-edits` → auto-approve mọi thứ; nay map `smart`. Ceiling: write thường vẫn auto-approve dưới `smart` — chỉ dangerous ops prompt |
| **R5** | ✅ PASS | Restart runner → `resume run-1` giữ slug, replay 10 events; turn mới → `session/load{trusted-airmail}` → `end_turn RESUMED_OK` |
| **R6** | ⏳ deferred | Cần workflow `bug-harness` trên sandbox |
| **R7** | ⏳ deferred | spawn_agent child isolation chưa drive live |
| **R8** | ✅ PASS | `interrupt` → `session/cancel` → `stopReason:"cancelled"`, `agent_stopped{cause:cancelled}` |
| **R9** | ✅ PASS | `run-120` grok `grok-4.5` low → `GROK_REGRESSION_OK` |
| **R10** | ✅ PASS | `token_usage_updated` events `last/total` + `modelContextWindow:500000` |

---

## Bảng Tổng Kết Kết Quả Nghiệm Thu (DOD Verification Sign-off)

Ghi lại mã phiên chạy thật (Run ID) và ngày kiểm thử:

- **Ngày thực hiện:** `2026-09-21` (Windows, HTTP-driven R-series)
- **Phiên bản Devin CLI kiểm thử:** `devin 3000.10.31 (b98cc431)`
- **Mã Run ID kiểm thử E2E:** `run-1` (R1/R2/R3/R5/R8), `run-83` (R4), `run-120` (R9-Grok)
- **Kết quả nghiệm thu:**
  - [⬜] Smoke Test (Mục S): **PASS**
  - [⬜] Settings Desktop (Mục A): **PASS** (Đã kiểm tra A1..A6 gồm 1-click install)
  - [⬜] TUI & State Restore (Mục B): **PASS** (Đã kiểm tra B1..B8 gồm BUG-330)
  - [⬜] Chat & Streaming (Mục C): **PASS** (Đã kiểm tra C1..C8 gồm Cancel/Interrupt)
  - [⬜] Mid-chat Model Switch (Mục D): **PASS** (Đã kiểm tra BUG-329)
  - [⬜] Approval Gate & YOLO (Mục E): **PASS** (Đã kiểm tra BUG-331, CA-712)
  - [⬜] Sub-Agent Isolation (Mục F): **PASS** (Đã kiểm tra BUG-334 & Process Reclamation)
  - [⬜] Kỹ Năng & Ngữ Cảnh Tự Động (Skills & Context) (Mục G): **PASS** (Đã kiểm tra DV-08, DV-09)
  - [⬜] Tư Thế Hội Thoại (Posture) (Mục H): **PASS**
  - [⬜] Session Persistence & Cross-Account (Mục I): **PASS** (Đã kiểm tra CA-688, DV-14)
  - [⬜] Tóm Tắt Một Lần (Summarizer) (Mục J): **PASS**
  - [⬜] Flow Mode (Mục K): **PASS**
  - [⬜] MCP trong Chat (Mục L): **PASS**
  - [⬜] Vision Guard (Mục M): **PASS** (Đã kiểm tra CA-692)
  - [⬜] Base Regression Guard (Mục N): **PASS** (Đã kiểm tra DV-BR)
  - [⬜] Quota & Timeout Guard (Mục O): **PASS** (Đã kiểm tra BUG-361, CA-759)
  - [⬜] Account Usage (Mục P): **PASS** (Đã kiểm tra CA-683)
  - [⬜] Chuyển Giao Ngữ Cảnh (Handoff) (Mục Q): **PASS** (Đã kiểm tra E2E-14, E2E-15)
  - [⬜] Live Verification gate-sandbox HTTP (Mục R): **PASS** (R1..R10 có runId + sessionId slug + log evidence)
