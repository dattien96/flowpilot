# CP-59 Test Steps — Manual Validation Guide (Full Coverage Task-312..317)

## Metadata

- Document ID: `CP-59-Test-Steps`
- Phase: `coding_plan` (manual validation companion)
- Status: `approved`
- Scope: Full — Task-312 (SD-26), Task-313 (chatId/timeline/backfill), Task-314 (switch endpoint/envelope), Task-315 (TUI `/provider` `/model` posture Tab + detached reattach + `/open`), Task-316 (Desktop chips/grouping), Task-317 (Drive sync/restore transcript-first detached). P-9 stretch (recall tool, chat-total token line, gemini source) vẫn deferred.
- Created: `2026-08-30`
- Last Updated: `2026-09-02` (B1-B5 posture Tab PASS — B5 đóng với re-run `cht_1e5b706a8201` sau BUG-347; guide B5 giữ làm tham chiếu)

## 0. Chuẩn bị (bắt buộc)

| # | Việc | Cách kiểm |
|---|------|-----------|
| P1 | Chạy trên branch `cp59-chat-ssot` (đã merge local) | `git log --oneline -5` → có `chat sync v2 restore`, `desktop chat switch`, `tui detached reattach` |
| P2 | (dev branch) Không cần flag | `FLOWPILOT_CHAT_SSOT` đã bỏ — luôn ON. Bỏ qua bước bật flag; nếu vẫn set `FLOWPILOT_CHAT_SSOT` thì bị ignore |
| P3 | (Supabase-backed runner) Apply migration trước | `supabase/migrations/20260830080000_chat_ssot_chat_columns_and_events.sql` — 5 cột `workflow_provider_sessions` + `workflow_chat_events` PK `(chat_id, chat_seq)`. Chưa apply mà bật flag → lỗi ghi cột mới (`nilIfEmpty` chỉ cứu flag-off) |
| P4 | Restart runner sau khi set env | `just chat-dev <project>` ; banner Ready ; `curl http://localhost:17812/client/providers` 200 |
| P5 | Posture pins chuẩn bị (repro BUG-330) | `/mode-setup` hoặc Settings → Chat Posture: `scan=opencode/muse-spark-1.2` , `code=opencode/deepseek` , `plan=grok-4.5` (bare-model, không provider) |
| P6 | Desktop nếu test Task-316 | `just web-dev` hoặc `npm run dev` trong `apps/desktop-flowpilot` ; `npm run typecheck` PASS trước khi smoke |

## S. Smoke 10 phút (TUI) — ✅ PASS 2026-08-31 (run-197698 cht_3810173c6b36, run-197689→197698; run-198151 cht_a8d253c2fe6f)

1. `/provider` → catalog có `grok` / `codex` / `claude` / `opencode` ready. ✅ `session: grok · grok-4.5` `footer grok-4.5`
2. Chat opencode: gửi "hello ban la model gi" → trả lời; status line có token usage. ✅ `run-197689 opencode muse-spark → hi A`
3. `/provider grok` → **không còn** "Cannot change provider..." — thay bằng chuyển leg; gửi "ban la model gi" → trả lời **như Grok** (không phải Muse Spark). ✅ `run-197689→197698` mint leg mới, `vậy giờ là model gì` → `Grok 4.5 do xAI`
4. Gửi tiếp 1 prompt → hội thoại vẫn liền, **không mất chữ cũ** trên màn hình. ✅ `vậy chat này mình đã hỏi bao nhiêu câu` → đếm `3 câu` gồm cả leg opencode
5. `/mode plan` (pin grok-4.5) → Tab hoạt động trên leg grok, không 404 `model not found` ở bất kỳ log nào. ✅ `Mode: plan - read-only` vẫn grok

Pass = 5/5. Fail S3/S5 → mở bug `feature_key: chat-history`, prior CA-693..701, không sửa test cũ (oracle-rule).

## A. Switch qua `/model` (cross-provider) — Task-314/315 — ✅ PASS 2026-08-31 (cht_a8d253c2fe6f legs 0,1,2)

| # | Bước | Kết quả mong đợi | Kết quả thực tế |
|---|------|------------------|-----------------|
| A1 | Trên chat codex (≥3 turn), `/model opencode-go/muse-spark-1.2-contributor` | Chuyển leg sang opencode; statusline provider đổi; 1 divider hệ thống hiện (seed `isHandoffSeed`, không hiện raw blob) | ✅ `opencode:197929→grok:197970` `chat_provider_switch:22 raw included 5` và `grok:197970→opencode/longcat:198151` `chat_provider_switch:29 raw included 6` `truncated false`; legs 0,1,2 |
| A2 | Hỏi "2 câu trước tôi hỏi gì?" | Model mới trả lời được nội dung các turn cũ (envelope `raw`, `<previous_conversation>` chứa 3 turn) | ✅ `198151:32` longcat liệt 5 câu `hi A / ok ban la nam / ok hcm / toi cung the` và `198151:36` `Muse Spark 1.2` vẫn nhớ `A/Nam/HCM/chó mèo`; `197970:27` grok cũng liệt 4 câu |
| A3 | Trong log runner: `handoffMode` + `includedTurnCount` | Xuất hiện đúng số turn; `chat_provider_switch` ghi đúng 1 lần (check `GET /client/chats/{chatId}/timeline` có 1 record `type=chat_provider_switch`) | ✅ `timeline cht_a8d253c2fe6f` 2 `chat_provider_switch` (22,29) mỗi 1 lần, `handoffMode raw included 5/6` |
| A4 | Footer/session panel sau switch | Provider/model hiển thị = provider/model ĐÚNG của leg mới (không footer dối) | ✅ `run-198151` footer `opencode-go/muse-spark-1.2-contributor` truthful; session `opencode` (ảnh `198151` lúc longcat→muse-spark) |
| A5 | `/model` cùng provider (ví dụ opencode→opencode scan→code) | **In-place** `session/set_config_option`, không leg mới, không divider (Task-314 `handoff_same_provider` 409, TUI fallback) | ✅ `198151 longcat-2.0 → muse-spark` `Model set to … (next prompt uses this model)` không leg mới (vẫn `198151` leg 2), `ban la model nao` → `Muse Spark 1.2 do Meta` |

## B. Posture Tab (BUG-330 repro chính thức) — Task-315 slice2 — ✅ PASS 2026-09-02 (B1-B5, operator)

| # | Bước | Kết quả mong đợi | Kết quả thực tế |
|---|------|------------------|-----------------|
| B1 | Chat opencode, Tab/mode sang `plan` (pin grok-4.5 bare) | Switch sang **leg grok thật**; divider `⇄ switched to grok · grok-4.5 — carried 3 turns (raw)` hiện; reply identity = Grok | ✅ PASS — switch sang leg grok thật, divider carried turns hiện, reply identity Grok |
| B2 | Prompt tiếp theo → trả lời bằng Grok; log runner | 0 occurrences `model not found`; opencode adapter KHÔNG nhận turn `model=grok-4.5` | ✅ PASS — 0 `model not found`; opencode adapter không nhận `model=grok-4.5` |
| B3 | Lần đầu derive pin bare-model (nếu pin chưa có provider) | 1 cảnh báo system "derived provider grok persisting"; `/mode` hiển thị pin đã có provider (persisted — không derive lại lần 2) | ✅ PASS — derive đúng 1 lần, cảnh báo persist hiện; `/mode` không derive lại lần 2 |
| B4 | Tab về mode cùng provider (opencode→opencode) | **In-place**: không leg mới, tiếp tục cùng session như BUG-329 (CA-680) | ✅ PASS — in-place, không leg mới, session tiếp tục (model đổi trong cùng leg) |
| B5 | Rapid double-Tab (Tab liên tiếp trong lúc switch) | Chỉ **1** leg mới (guard `chatSwitchInFlight`); message không nhân bản; queued posture apply sau `ChatSwitchedMsg` | ✅ PASS 2026-09-02 — `cht_e4975cb1f769` `run-494554→run-494566` `legSeq:1` `raw included 1`; đúng 1 `chat_provider_switch` (seq 6), 0 duplicate `chatSeq` 1..13; Grok identity sau switch; Tab 2 không mint leg code. Re-run xác nhận sau BUG-347: `cht_1e5b706a8201` `run-500159→run-500181` — divider `⇄ switched to grok · grok-4.5 — carried 1 turns (raw)` hiện live, seed reply bị drop (không còn bubble chào mồ côi), Tab in-flight báo "switch in progress" (CA-722) |

### B5 — Guide test rapid double-Tab (Task-315 slice2, guard `chatSwitchInFlight`)

**Cơ chế trong code** (đã đọc: `chat_switch.go` `routePostureSwitch`/`cmdSwitchChatProvider`/`applyChatSwitched` + `chat_posture.go` apply:):

1. Tab lần 1 (cross-provider, ví dụ `code` → `plan`): `routePostureSwitch` set `chatSwitchQueuedPosture="plan"` rồi gọi `cmdSwitchChatProvider` → `chatSwitchInFlight=true`, **1** HTTP `POST .../switch-provider`.
2. Tab lần 2 trong lúc in-flight: `routePostureSwitch` guard trả nil (không gọi endpoint lần 2); `apply:` rơi vào busy-check → notice **"Provider switch in progress — wait for it to finish, then Tab again"** (BUG-347; trước đây nhầm message question/approval) — không leak in-place, không mint leg.
3. `ChatSwitchedMsg` về: `applyChatSwitched` consume `chatSwitchQueuedPosture` → re-apply **full profile** (reasoning/yolo/model pin) trên leg mới (CA-685) → guard reset `inFlight=false`. Tab tiếp theo route bình thường.

**Các bước test:**

| # | Bước | Cách kiểm |
|---|------|-----------|
| 1 | Chat opencode ≥3 turns, posture pins theo P5 (`plan=grok-4.5`) | `just chat-dev`; statusline `opencode` |
| 2 | Tab sang `plan` → **ngay lập tức** Tab lần 2 (double-tap nhanh, trong ~1-2s khi switch chưa xong) | Mắt thường: lần 2 không mint leg mới, có notice busy; statusline vẫn chờ switch lần 1 |
| 3 | Đợi switch lần 1 xong (`ChatSwitchedMsg`) | Statusline = `grok · grok-4.5` (không nhảy lệch sang posture khác) |
| 4 | Verify timeline: đúng **1** leg mới + **1** `chat_provider_switch` mới | `curl -s http://localhost:17812/client/chats/<chatId>/timeline \| jq '[.legs[] \| select(.legSeq>0)] \| length, [.records[] \| select(.type=="chat_provider_switch")] \| length'` |
| 5 | Verify message không nhân bản | `chatSeq` monotonic, không duplicate `(chatId,chatSeq)`; đếm dòng transcript khớp số turn thật |
| 6 | Verify guard trong log runner | Chỉ **1** dòng `switch-provider` HTTP call; Tab 2 không tạo request; sau `ChatSwitchedMsg` không còn `chatSwitchInFlight` |
| 7 | Tab lần 3 (sau khi in-flight đã clear) | Route bình thường — không kẹt, không cần double-Tab như lỗi cũ B-4 (417944→417970) |
| 8 | Gửi prompt → identity đúng provider của posture đã chọn ở Tab 3 | Hỏi "ban la model gi" → model của posture đó trả lời |

Fail (leg nhân bản / message trùng / Tab kẹt phải bấm 2 lần) → mở bug `feature_key: chat-history`, prior CA-693..701, không sửa test cũ (oracle-rule).

## C. Guards + trạng thái đặc biệt — Task-314

| # | Bước | Kết quả mong đợi | Kết quả thực tế |
|---|------|------------------|-----------------|
| C1 | `/provider codex` ngay khi turn đang stream | `handoff_run_busy` 409 — dòng lỗi, chat vẫn dùng leg cũ, không leg mồ côi | ✅ PASS 2026-09-02 — `cht_b97b54d05a27` `run-511323`: notice "A turn is in progress — wait for it to finish, then switch provider/model" (CA-723), 0 `chat_provider_switch`, 0 leg mới, turn stream tiếp tục |
| C2 | Switch khi đang pending approval / question | 409 `handoff_run_busy`, card vẫn hiện, không leg mới | ✅ PASS 2026-09-02 — `cht_9f956dc8f850` `run-511461`: transcript có `approval_requested` exec, 0 switch, 1 leg; UI hiện notice "Cannot switch…", card Approve vẫn show |
| C3 | Switch sang provider CHƯA cài (ví dụ gỡ gemini, hoặc target `gemini` khi chưa installed) | `provider_unavailable` 422 + install hint; **zero mutation** (leg cũ nguyên, `switchFromRunID` rỗng) | ⏭️ DONE-SKIP theo quyết định operator (không có provider chưa cài để test live; contract 422 đã covered bởi automated `chat_switch_test.go` provider_unavailable case) |
| C4 | Chat mới tạo, chưa gửi turn nào → switch ngay | `fresh_start` — leg mới không envelope (`handoffMode=fresh_start`, `Prompt=""`), không lỗi | ✅ PASS 2026-09-02 — operator-confirmed (run-id bổ sung sau) |
| C5 | Switch trên workflow run (`runKind != chat`) | 409 `handoff_run_kind_unsupported`, block text giữ nguyên |
| C6 | Flag off → `/provider` cross-provider | Block text cũ `Cannot change provider after a run has started. Use /new...` byte-identical, không gọi endpoint |

### C — Guide test từng ô (operator, 2026-09-02)

> Cách verify nhanh cho MỌI ô: đếm leg/switch record không tăng =
> không mint leg:
> ```bash
> F=~/.flowpilot/chat-transcripts/chats/<chatId>/transcript.ndjson
> jq -r '.type' "$F" | sort | uniq -c          # đếm chat_provider_switch
> jq -r '.legRunId' "$F" | sort -u             # danh sách leg
> ```

**C1 — `/provider` khi turn đang stream**
1. Chat opencode, gõ prompt dài (vd "viết 1 bài essay 1000 từ về Go") → Enter → đang stream.
2. Ngay lúc stream, gõ `/provider grok` Enter. **Chú ý: phải dùng `/provider`, KHÔNG dùng `/model <model cùng provider>`** — `/model` cùng provider là in-place theo thiết kế (footer đổi, không lỗi, áp từ turn sau — không phải lỗi; test `run-505761` xác nhận 0 leg mới).
3. Pass nếu: TUI hiện dòng lỗi `A turn is in progress — wait for it to finish, then switch provider/model` (CA-723; trước đây nhầm "question or approval"); footer vẫn opencode; turn stream tiếp tục bình thường; **không** có dòng chào Grok/không mint leg.
4. Gửi mình: runId + chatId.

**C2 — switch khi pending approval/question**
1. Mở flow có gate (hoặc chat với YOLO off để model xin quyền chạy lệnh) → để card Approve/Question hiện.
2. Gõ `/provider grok` Enter.
3. Pass nếu: notice busy (`handoff_run_busy`), card **vẫn hiện**, trả lời/approve xong vẫn dùng leg cũ, không leg mới.
4. Gửi mình: runId + chatId.

**C3 — switch sang provider chưa cài**
1. `/provider gemini` (nếu gemini chưa install) hoặc 1 provider không tồn tại.
2. Pass nếu: dòng lỗi chứa `provider_unavailable` + hint cài đặt; runId trong statusline **không đổi**; transcript không có `chat_provider_switch` mới.
3. Gửi mình: runId + chatId.

**C4 — switch trên chat mới chưa có turn**
1. `/new` (chat mới, chưa gửi gì) → ngay lập tức `/provider grok` Enter.
2. Pass nếu: switch thành công, divider hiện `fresh_start` / carried 0 (không envelope, không có khối `<previous_conversation>`); transcript có đúng 1 `chat_provider_switch` với `handoffMode=fresh_start` và `includedTurnCount=0`.
3. Gửi mình: runId + chatId.

**C5 — switch trên workflow run**
1. `/mode flow` (hoặc chạy workflow) → trong lúc flow đang chạy gõ `/provider grok` Enter.
2. Pass nếu: block text cũ `Cannot change provider after a run has started. Use /new to start fresh.` hiện; không có request switch; flow tiếp tục.
3. Gửi mình: runId.

**C6 — bỏ** (dev branch luôn ON, không còn path flag-off).

## D. Timeline / transcript (FlowPilot SSOT) — Task-313

| # | Bước | Kết quả mong đợi |
|---|------|------------------|
| D1 | Sau switch, mở file transcript | `~/.flowpilot/chat-transcripts/chats/<chatId>/transcript.ndjson` (local) hoặc Supabase `workflow_chat_events` — có `turn_started`/`message_completed`/`tool_*`/`file_changed`/`approval_resolved`/`question_answered`/`token_usage` + **1** `chat_provider_switch` ; `chatSeq` monotonic |
| D2 | `GET /client/chats/{chatId}/timeline?afterSeq=N&limit=M` (curl) | Phân trang đúng, `truncated`/`nextSeq` đúng; `legs` sorted `legSeq` 0..N-1 ; `degraded:true` chỉ khi store fail |
| D3 | Kill runner giữa chat → mở lại `just chat-dev` | Chat cũ legacy (pre-flag) đọc timeline → backfill raw đúng 1 lần (thử đọc 2 lần, số dòng không tăng, marker `chat_backfilled` tồn tại) |
| D4 | Workflow run | `GET /client/chats/<runId-workflow>/timeline` → 404 `chat_not_found` (chat-kind gate) |
| D5 | Timeline idempotence | `ReadChatRecords` duplicate `(chatId,chatSeq)` không nhân bản (restore upsert) |

Curl mẫu:
```bash
curl -s http://localhost:17812/client/chats/cht_9f2a71c04b8d/timeline | jq '.legs, .records | length'
curl -s "http://localhost:17812/client/chats/cht_xxx/timeline?afterSeq=10&limit=20" | jq '.truncated, .nextSeq'
```

## E. Desktop chip switch + history grouping — Task-316

Prereq: runner đã bật (luôn ON trên dev branch), Desktop `store.chatSwitch` bindings đã build.

| # | Bước | Kết quả mong đợi |
|---|------|------------------|
| E1 | Chat codex → chip `claude` → Confirm | Timeline **giữ nguyên**, append đúng 1 divider `seed-divider-<runId>`; `providerSwitchLoading` reset; no `timeline: []` reset (store.test.ts `TestProviderSwitchKeepsTimeline`) |
| E2 | Double Confirm nhanh (click Confirm 2 lần trước khi resolve) | Chỉ 1 `switchChatProvider` call, 1 leg mới (guard `providerSwitchLoading`) |
| E3 | Cùng-provider chip (ví dụ claude→claude chỉ đổi model) | Không modal, model đổi in-place, không gọi switch endpoint (`TestSameProviderChipInPlaceNoModal`) |
| E4 | Navigator history | 3-leg chat `cht_x` (opencode→grok→codex) hiển thị **1 row** `cht_x` với chip `3 legs`; expand → 3 legs `legSeq` order, divider giữa legs |
| E5 | Flag off / legacy runner (không chatId) | Confirm đi path Task-078 cũ verbatim: `handoffContext` + `startRun`, timeline reset như cũ (`TestProviderSwitchLegacyFallbackPathUnchanged`) |
| E6 | Divider single-source | Seed turn `isHandoffSeed` render thành divider card, không thành user bubble; reload page không duplicate divider (dedupe by `toRunId`) |
| E7 | Posture Tab Desktop (Settings → Chat Posture) | Bare-model pin derive + persist 1 lần, cross-provider Tab gọi `switchChatProvider` (`TestPostureTabCrossProviderUsesSwitch`) |

## F. Detached reattach + `/open` restore-by-chat — Task-315 slice3

| # | Bước | Kết quả mong đợi |
|---|------|------------------|
| F1 | Restore chat trên máy mới (xem G) → chat ở trạng thái **detached** (all legs `closed(restored)`, zero active) | `GET /client/chats/{chatId}/timeline` trả full transcript; TUI/Desktop mở chat hiện full history với dividers |
| F2 | Detached → gửi prompt đầu tiên (TUI) | TUI `cmdSendTurn` intercept detached → `cmdReattachChat` → `startRun(chatId, switchFromRunID=latestLeg)` (không gọi `POST .../switch-provider` → tránh 409 `chat_no_active_leg`); prompt gửi trên leg mới, envelope seed đầy đủ |
| F3 | Detached → `/provider` hoặc `/model` | Apply locally, defer notice "will reattach on next turn", không gọi switch endpoint; reattach xảy ra ở turn tiếp theo |
| F4 | TUI `/open` trên switched chat | Fetch `GetChatTimeline`, backfill prior legs' turns + `E-9` dividers (idempotent `chatBackfillDone`, current leg skip), seed envelope collapse thành divider `carried N[ of M] turns (mode)` |
| F5 | `resolveChatIdentity` explicit chatId không seq | Leg mới lấy `legSeq = maxPersistedLegSeq+1` (không reuse), `TestResolveChatIdentityUsesPersistedLegSeq` |

## G. Drive sync / restore (chat-level) — Task-317

> Runner-level có fake Drive harness (`SyncChatV2ToDrive` map) — 10 tests `chat_sync_manifest_test.go` đã PASS. Dưới đây là manual E2E trên Drive thật.

| # | Bước | Kết quả mong đợi |
|---|------|------------------|
| G1 | Machine A: chat opencode → grok → codex (3 legs), `POST /client/workflow-runs/{anyLegRunId}/sync-chat` | Drive có `chat-sessions/chats/<chatId>/transcript.ndjson` + per-leg sidecars + `manifest.json` `schemaVersion=2`, `chatId`, `legs` sorted `legSeq`, `chatTranscriptFile` canonical |
| G2 | Re-sync sau khi switch thêm leg | Cùng Drive path `chat-sessions/chats/<chatId>/...`, legs 3→4, không dup file, `transcriptBytes` tăng, manifest overwrite idempotent |
| G3 | Machine B (chỉ có claude + opencode, không codex) restore `ReadChatSyncManifest` → `RestoreChatFromManifestV2` | Transcript-first: `ReadChatRecords` có full transcript ngay cả khi 1 leg fail; chat mở detached, timeline full text |
| G4 | Per-leg degradation | Leg thiếu sidecars → `SyncStatus=session_unavailable`; provider binary absent → `provider_unavailable` + hint `install <provider>`; chat vẫn mở, tiếp tục trên provider khác OK |
| G5 | Restore twice | Record count identical, `chatSeq`/`legSeq` monotonic, không duplicate divider (unique `(chatId,chatSeq)`) |
| G6 | v1 manifest legacy | `ReadChatSyncManifest` trên v1 bytes (`sourceRunId`, `schemaVersion=1`) → `isV2=false`, restore path cũ untouched |
| G7 | Reattach sau restore | Trên Machine B, gửi turn trên claude → `createRun(chatId, switchFromRunID=latestLeg)` mint leg mới local, envelope seed từ transcript, turn completes |

## H. Switch matrix 12-pair + truncation ladder — Task-314/315/316

| From \ To | codex | claude | grok | opencode |
|---|---|---|---|---|
| codex | in-place | switch | switch | switch |
| claude | switch | in-place | switch | switch |
| grok | switch | switch | in-place | switch |
| opencode | switch | switch | switch | in-place |

- Mỗi ô switch: ≥3 turn trước switch → timeline 100% text continuity, envelope `includedTurnCount` đúng, post-switch identity check (hỏi "ban la model gi" → target provider trả lời), footer/session panel truthful.
- `handoff_same_provider` → in-place `set_config`/`set_model`, zero leg mới.
- Truncation ladder: transcript vượt `ContextWindowTokens×3 chars` (512 KiB cap, floor 64 KiB) → `hybrid`/`target_summary` mode, marker `[Earlier conversation omitted…]`, divider hiện `carried N of M turns (truncated)` (Task-314 `chatHandoffBudget`).
- Gemini rows: target-only cho tới khi extractor proven (typed unsupported nếu làm source).

Automated: `TestSwitchMatrixAllDirectedPairs` fake adapters đã PASS (codex→claude envelope completeness + 11 pair còn lại provider-agnostic). Manual live là DOD cuối để flip flag default on.

## I. Cross-surface parity

| # | Bước | Kết quả mong đợi |
|---|------|------------------|
| I1 | Cùng `chatId` mở TUI `just chat-dev` và Desktop `store.chatTimeline` | Timeline identical (cùng `GET /client/chats/{chatId}/timeline`), dividers cùng vị trí, số record bằng nhau |
| I2 | Restart runner giữa multi-leg chat → reopen TUI `/open` + Desktop reload | Replay identical (stable `chatSeq`, `E-9` id), current leg resume via `seedTranscriptFromDisk` (engine) nhưng display từ chat store |
| I3 | Envelope collapse | `handoffPromptPrefix`/`isHandoffSeed` render thành divider ở cả TUI live stream, TUI replay, Desktop timeline — không bao giờ thành user bubble thô (`TestHandoffSeedRendersAsDivider*`) |

## J. Flag off regression (đã bỏ trên dev branch)

Dev branch `cp59-chat-ssot` đã bỏ flag — luôn ON, không còn path flag-off để test. Trên main trước merge, flag-off path từng được verify: chat mới không có `chatId`, `/provider` block cũ, workflow không ảnh hưởng, Desktop đi Task-078. Sau khi bỏ flag, chỉ còn always-ON path.

## Kết quả thực tế — đã PASS 2026-08-31

| Mục | Kết quả | Chat/Run | Ghi chú |
|-----|---------|----------|---------|
| S Smoke | ✅ 5/5 | `cht_3810173c6b36` `197689→197698`, `cht_a8d253c2fe6f` | Cross-provider switch mint leg mới, Grok identity, continuity 3 câu |
| A Switch `/model` | ✅ 5/5 | `cht_a8d253c2fe6f` legs 0,1,2 `197929→197970→198151` | `raw included 5/6`, footer truthful, same-provider `longcat→muse-spark` in-place `198151` |
| B Posture Tab | ✅ B1-B5 PASS 2026-09-02 | `cht_e4975cb1f769` `run-494554→494566`, `cht_1e5b706a8201` `run-500159→500181` | Divider carried turns, derive-once persist, in-place same-provider, rapid double-Tab 1 leg; BUG-347 (seed reply drop + sync divider + busy message) verified |
| C Guards | ✅ C1, C2, C4 PASS · C3 done-skip · C5 ⏳ | `cht_b97b54d05a27` `run-511323`, `cht_9f956dc8f850` `run-511461` | Busy turn + approval busy + fresh_start OK, 0 leg mới sai; C3 skip (422 covered bởi automated test) |
| D Timeline | ⏳ | — | |
| E Desktop | ⏳ | — | |
| F Detached | ⏳ | — | |
| G Drive | ⏳ | — | |
| H Matrix | ⏳ (auto `TestSwitchMatrix` PASS) | — | Manual live 12-pair còn lại |
| I Cross-surface | ⏳ | — | |
| J Flag off | ✅ removed | dev branch always ON | |

## Kết luận phiên

Ghi Pass/Fail từng mục kèm `chatId`/`runId`/`legSeq`/`handoffMode`/`includedTurnCount`. Fail B1/B2/A4/E1/G3 → mở bug theo `$add-new-bug`, `feature_key: chat-history`, prior CA-693..701 (không sửa test cũ — sửa production code). Khi E2E matrix §H + cross-surface §I + restore §G đều Pass trên providers thật, ghi CA đóng CP-59 (flag đã bỏ trên dev branch, không cần flip).

## Tham chiếu nhanh (endpoint)

- `GET /client/chats/{chatId}/timeline?afterSeq=&limit=` → `{chatId, legs[], records[], nextSeq, truncated, degraded}`
- `POST /client/chats/{chatId}/switch-provider` body `{targetProviderKey, model?, reasoningEffort?, yoloMode?}` → `{handle, chatId, legSeq, model, handoff:{handoffMode, includedTurnCount, omittedTurnCount, truncated, actionsDigestIncluded}}`
- Errors: `chat_not_found` 404, `chat_no_active_leg` 409 (detached → reattach via `createRun`), `handoff_run_busy` 409, `handoff_same_provider` 409, `provider_unavailable` 422, `switch_seed_failed` leg-state, `chat_store_degraded` flag
