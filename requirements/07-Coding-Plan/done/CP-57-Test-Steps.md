# CP-57 — Manual Test Guide (Opencode Provider Mirror)

## Metadata

- Document ID: `CP-57-TEST-STEPS`
- Title: `CP-57 Manual Test Guide — mọi feature đã mirror cho Opencode`
- Phase: `verification`
- Status: `active`
- Owner: `FlowPilot`
- Created: `2026-08-29`
- Last Updated: `2026-08-30`
- Parent Documents: [CP-57: Opencode Provider Integration](./CP-57-Opencode-Provider-Integration.md)
- Related Documents: [Task-300..303](../../08-Task/done/), CA-679..CA-683, BUG-329
- Tags: `opencode, verification, test-steps, manual-test, cp-57`

## AI Quick View

- **What:** Checklist manual từng feature đã mirror cho Opencode (settings → chat → approval → spawn → gates → resume).
- **Why:** Operator tự xác nhận sau khi đóng CP-57; mỗi mục map OC-*/E2E-* trong CP-57 §7.
- **Thời lượng:** ~45 phút full, ~10 phút smoke (mục S).

## 0. Chuẩn bị (bắt buộc trước mọi test)

| # | Việc | Cách kiểm |
|---|------|-----------|
| P1 | `opencode` CLI installed + logged in | `opencode --version` → `1.18.x`; `opencode models` ra danh sách (không lỗi) |
| P2 | **Restart runner mới** (quan trọng — runner cũ chưa có BUG-329/CA-683) | Thoát TUI, `just chat-dev <path>` chạy lại; health runner ghi nhận start time mới |
| P3 | TUI boot đúng | `just chat-dev /path/to/project` → banner Ready, `/provider` liệt kê có `opencode` |

Lưu ý: một số bước cần Desktop app (`/settings` từ TUI mở Desktop).

## S. Smoke 10 phút (chạy nhanh nhất nếu ít thời gian) — **PASSED 2026-08-30** (S1-S5; kèm CA-688 fix /open chat cũ)

1. `/model opencode/muse-spark-1.2-contributor-free` → "Model set to …"
2. Gửi "hello" → stream trả lời, có token usage ở status line.
3. Thoát TUI (Ctrl+C) → mở lại `just chat-dev` → status line **vẫn** model opencode (B7).
4. Gửi "Nhớ số 42 nhé" → "42 nhé" → `/model opencode-go/deepseek-v4-flash` → "số mấy tôi bảo nhớ?" → trả lời **42** (D1 — BUG-329).
5. `/mode plan` → chuyển plan; Tab về code. Xong.

---

## A. Settings — Desktop (OC-18/OC-25/OC-29/OC-30)

Mở Desktop → Settings → AI Providers.

| # | Bước | Kết quả mong đợi |
|---|------|------------------|
| A1 ✅ PASSED 08-30 | Xem card Opencode | "Installed 1.18.x (tested 1.18.18)", trạng thái READY |
| A2 ✅ PASSED 08-30 (DB: cần chạy migration CA-689 trước — check constraint `ai_supported_models_provider_key_check`) | Bấm **Detect models** | Danh sách model opencode/* + opencode-go/* (≥10; TUI có retry warm nếu ít hơn) |
| A3 ✅ PASSED 08-30 | Xem hàng version (CheckVersion) | **4 hàng**: Claude / Codex / Grok / Opencode |
| A4 ✅ PASSED 08-30 (CA-689b scroll fix) | Account sidebar: pin Opencode | Email/label từ auth.json (Zen, Go, xAI) + **"Limit: N/A (zen proxy)"** + tối đa 2 dòng stats text (CA-683: "cost: $… total", "stats: N sessions"); bấm **All** → modal hiện đầy đủ usage lines dạng text (không có thanh %) |
| A5 | MCP tab → Google Drive connect | `~/.config/opencode/opencode.json` xuất hiện `mcpServers` mới; `opencode mcp` (CLI) list thấy |

## B. TUI — chọn provider/model + persistence (CA-679) — **PASSED 08-30 (B1-B7)**

| # | Bước | Kết quả mong đợi |
|---|------|------------------|
| B1 ✅ PASSED 08-30 | `/provider` | Danh sách có `opencode` |
| B2 ✅ PASSED 08-30 | `/model` | Danh sách model mọi provider; model opencode có detail "opencode" |
| B3 ✅ PASSED 08-30 | `/model opencode/muse-spark-1.2-contributor-free` | "Model set to … · provider: opencode"; **provider tự switch** theo model |
| B4 ✅ PASSED 08-30 | `/status` | Session panel: provider opencode · model đã chọn · account label |
| B5 ✅ PASSED 08-30 | `/new` → thoát TUI → mở lại | Model/provider **giữ nguyên** (không quay về grok-4.5 cũ) |
| B6 ✅ PASSED 08-30 | `/reasoning low` → thoát → mở lại | Reasoning giữ nguyên |
| B7 ✅ PASSED 08-30 | Chọn model opencode → thoát hẳn máy (không /new) → mở lại | Vẫn model opencode — **đây là regression CA-679**: posture scan/plan pin grok-4.5 không được đè lựa chọn |

## C. Chat cơ bản (OC-01/OC-11/OC-24) — **PASSED 08-30 (C1-C4)**

| # | Bước | Kết quả mong đợi |
|---|------|------------------|
| C1 ✅ PASSED 08-30 | Gửi "hello" | Stream `message_delta` mượt, tin nhắn cuối lưu vào history |
| C2 ✅ PASSED 08-30 | Nhìn status line sau turn | Token usage "ctx:…k/1M last:…k" (context window 1,048,576) |
| C3 ✅ PASSED 08-30 | Gửi "đọc file README.md rồi tóm tắt 1 dòng" | Tool card xuất hiện (read), rồi câu trả lời |
| C4 ✅ PASSED 08-30 | Gửi "tạo file ghi-chu.txt có nội dung xin-chao" (YOLO đang ON) | File được tạo + event file_changed; `flow_gate_violation` có thể xuất hiện ở sidebar steps (r-ca — xem mục K) |

## D. Đổi model giữa chat (BUG-329) — **test quan trọng nhất** — **PASSED 08-30 (D1-D3)**
> D3 wire proof run-345019 (cùng session `ses_fb11b1cd…`): 02:00:18 `effort=xhigh` (turn-345021) → 02:00:28 `effort=medium` (turn-345040), không turn-failed. Reasoning đổi giữa chat đi tới opencode thật.

| # | Bước | Kết quả mong đợi |
|---|------|------------------|
| D1 ✅ PASSED 08-30 | "Nhớ số 42 nhé" (model muse-spark) → `/model opencode-go/deepseek-v4-flash` → "số mấy tôi bảo nhớ?" | Trả lời **42** — history giữ nguyên, KHÔNG lỗi "session/load returned no sessionId" |
| D2 ✅ PASSED 08-30 | Đổi tiếp sang model thứ 3 (vd `opencode/gpt-5.4-nano`) và hỏi tiếp | Vẫn tiếp tục cùng cuộc hội thoại |
| D3 ✅ PASSED 08-30 | Trong lúc chat, đổi `/reasoning` | Turn sau dùng effort mới, không spawn lỗi |

## E. Approval + YOLO (OC-04/OC-05/E2E-09/10)

Trạng thái: YOLO **OFF** (`/yolo` hiện OFF).

> **BUG-331 / CA-690**: E1 fail lần đầu (opencode default allow-all, không bao giờ hỏi → không card).
> Đã fix: mọi process `opencode acp` spawn với `OPENCODE_CONFIG_CONTENT={"permission":{"edit":"ask","bash":"ask"}}`;
> YOLO/posture quyết ở runner per-turn. Kèm BUG-332 (modal clipped) + BUG-333 (Tab/arrows ring + row-cache
> highlight + chip fill + question-bar squeeze) trong quá trình re-test.
> Probe J live: reject → file KHÔNG tạo, turn end_turn sạch. Watch item: `file_changed` event đến trước
> `permission_required` trong transcript (artifact mapper, không phải bypass) — xác nhận qua E2 pass.

| # | Bước | Kết quả mong đợi |
|---|------|------------------|
| E1 ✅ PASSED 08-30 | "tạo file approval-test.txt nội dung ok" | Card **permission_required** hiện ra (F2 panel + `/approve`/`/deny`) |
| E2 ✅ PASSED 08-30 | `/deny` | File **không** được tạo; turn kết thúc có kiểm soát (không treo) |
| E3 ✅ PASSED 08-30 | Gửi lại yêu cầu tạo file → `/approve` | File được tạo |
| E4 ✅ PASSED 08-30 | `/yolo` (bật ON) → yêu cầu tạo file khác | Tự động được duyệt, **không** hiện card |
| E5 ✅ PASSED 08-30 | YOLO ON, yêu cầu model "hỏi tôi muốn tên file gì" | Câu hỏi vẫn **chặn hỏi user** (question không bao giờ auto-approve) |

## F. spawn_agent (OC-07/E2E-17)

> **BUG-334**: F1 fail lần đầu — `MCP error -32000 Connection closed` sau ~400ms: session/new của child (token MCP
> per-turn mới) trên shared acp process replace process-level MCP client → in-flight tools/call của parent đứt;
> retry còn landing nhầm bridge của child (spawn lồng grandchild). Fix: child run + variants probe chạy process
> riêng theo scope segment (`account|child:<runID>` / `probe`), reclaim theo base, đóng process khi child terminal.
> Runner log verify: `[agent-spawn] child terminal ... completed finalMsgLen=8` về đúng turn.

| # | Bước | Kết quả mong đợi |
|---|------|------------------|
| F1 ✅ PASSED 08-30 | "Dùng tool spawn_agent trên server flowpilot với agent='helper', prompt='trả lời: child-ok', wait=true, rồi báo tôi child trả lời gì" | Child agent chạy xong; model báo lại kết quả child |
| F2 ✅ PASSED 08-30 | Tab (hoặc `/agents`) | Panel agents hiện child "helper · completed"; `/agent helper` mở transcript child |
| F3 ✅ PASSED 08-30 | Lặp F1 với wait=false | Trả lời ngay, child chạy nền, xuất hiện trong panel |

## G. Skills (OC-08) — **PASSED 08-30 (G1-G3)**

| # | Bước | Kết quả mong đợi |
|---|------|------------------|
| G1 ✅ PASSED 08-30 | `/skill` | Catalog skills hiện ra |
| G2 ✅ PASSED 08-30 | `/skill audit-logging` → hỏi việc liên quan change | Prompt có inject skill (model nhắc quy tắc audit/CA) |
| G3 ✅ PASSED 08-30 | `/skill clear` | Về trạng thái không skill |

## H. Chat posture + restore giữ model (CA-679) — **PASSED 08-30 (H1-H5)**

| # | Bước | Kết quả mong đợi |
|---|------|------------------|
| H1 ✅ PASSED 08-30 | `/mode` | Hiện 3 posture scan/plan/code + profile pins |
| H2 ✅ PASSED 08-30 | `/mode scan` | "Mode: scan — read-only"; yêu cầu write bị chặn/auto-deny |
| H3 ✅ PASSED 08-30 | Tab | Cycled plan ↔ code |
| H4 ✅ PASSED 08-30 | Đang active posture có pin model (vd scan pin grok-4.5): chọn model opencode → thoát → mở lại | **Model opencode giữ nguyên** (restore không re-pin đè — CA-679); posture vẫn đúng active |
| H5 ✅ PASSED 08-30 | `/mode plan` (switch THẬT sang posture có pin model) | Pin model của posture được áp (hành vi cũ giữ nguyên) |

## I. Session resume / history (OC-12/E2E-04/05) — **PASSED 08-30 (I1-I4)**

| # | Bước | Kết quả mong đợi |
|---|------|------------------|
| I1 ✅ PASSED 08-30 | Chat vài turn → thoát TUI → mở lại → `/history` | Run opencode xuất hiện trong list |
| I2 ✅ PASSED 08-30 | `/open <run>` → "trước đó tôi nói gì?" | Model trả lời đúng nội dung cũ (resume cùng `ses_*`) |
| I3 ✅ PASSED 08-30 | Kill runner (thoát `just chat-dev`) → mở lại → mở chat cũ → hỏi tiếp | Tiếp tục được (session persist trong opencode.db) |
| I4 ✅ PASSED 08-30 | `/sync` → `/restore` với chat opencode | Trả lỗi **typed** rõ ràng (Drive file-copy restore chưa hỗ trợ opencode — known gap có chủ đích, xem CP-57 §10.2), không corrupt history. **Lưu ý CA-688**: `/open` chat opencode LOCAL thì hoạt động bình thường (I1/I2) — nếu gặp `session_unavailable` khi mở chat local, restart runner để nhận fix |

## J. Summary (E2E-13)

> **Note:** Test J sẽ thực hiện trong **CP-59** vì **CP-59 sẽ refactor lại summary** — không test ở đây.

| # | Bước | Kết quả mong đợi |
|---|------|------------------|
| J1 | Chat dài vài turn → Desktop mở chat → **Gen summary** | Summary generated (chạy qua cheap tier `opencode/gpt-5.4-nano`) |
| J2 | Chat tiếp sau summary | Turn mới có inject "Prior discussion" (sidebar/context thể hiện) |

Lưu ý: summary cần project có feature catalog (`.flowpilot/catalog/features.ndjson` trong project). Nếu báo "feature catalog unavailable" là project chưa có catalog — không phải lỗi opencode.

## K. Flow mode + gates (OC-02/OC-10/E2E-12)

| # | Bước | Kết quả mong đợi |
|---|------|------------------|
| K1 | `/flow <builtin pack>` → gửi prompt | Flow chạy trên opencode (sidebar steps, provider chip opencode) |
| K2 | Task/bugfix write file mà thiếu CA note | `flow_gate_violation` (r-ca) hiện ở steps; reprompt/warn chạy **trên opencode**, không nhảy sang codex |
| K3 | `/chat` | Quay lại chat mode |

## L. MCP (OC-17/OC-21)

| # | Bước | Kết quả mong đợi |
|---|------|------------------|
| L1 | Trong chat, yêu cầu dùng tool ask_user (xem E5) | Question chặn → trả lời → turn tiếp tục |
| L2 | A5 đã connect Google Drive → chat yêu cầu list Drive | Tool Drive xuất hiện (nếu account đã cấu hình) |

## M. Vision guard (E2E-27) — **PASSED 08-30 (M1)**

| # | Bước | Kết quả mong đợi |
|---|------|------------------|
| M1 ✅ PASSED 08-30 | `/image attach <ảnh>` hoặc drag ảnh vào composer khi model opencode | Bị **chặn trước khi gửi prompt** với thông báo rõ (Vision=false — không mất prompt) |

## N. Account usage sau nhiều turn (CA-683) — **PASSED 08-30 (N1)**

| # | Bước | Kết quả mong đợi |
|---|------|------------------|
| N1 ✅ PASSED 08-30 | Chat vài chục turn → mở Desktop sidebar account Opencode (hoặc bấm All) | Stats lines cập nhật (60s cache — chờ ~1 phút rồi refresh) |

## Kết luận phiên test

Ghi lại từng mục Pass/Fail kèm run ID. Fail mục D1/E*/F1/I* → mở bug mới theo quy trình (`$add-new-bug`, feature_key `ai-providers`), prior CA: CA-679..CA-683, BUG-329.
