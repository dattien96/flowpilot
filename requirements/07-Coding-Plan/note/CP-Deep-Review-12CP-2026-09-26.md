# Deep review — 12 CP (bug và test còn thiếu)

## Metadata

- Document ID: `CP-DEEP-REVIEW-12CP-2026-09-26`
- Title: `Deep review of harness, state-durable, and support CPs — bugs and missing tests only`
- Kind: `review report` (không phải coding plan, không phải bugfix)
- Date: `2026-09-26`
- Method: 3 lượt review độc lập (A harness/vibe, B state-durable + SSOT, C support), rồi đối chiếu lại các kết luận nặng trên source HEAD
- Scope: không sửa code. Không chạy lại `go test`. "Green" lấy từ assertion trong source và bug doc.
- Ledger dùng: `requirements/07-Coding-Plan/todo/CP-Full-Live-Test.md`
- Evidence R2: build `8c95a5bb`, runner `:19400`, `~/fp-beds/lt-evidence/full-20260925-r2/LIVE-LOG.md`

## Naming

Nhãn "CP-58: SSOT cross-provider" trỏ nhầm tài liệu.

- **CP-58** là vòng review của bug/task/cp harness (ledger mục A-1).
- **CP-59** là chat SSOT cross-provider (ledger mục B-2).

Cả hai đều được review vì số và mô tả không trùng nhau. Danh sách gốc: CP-60, CP-64, CP-65, CP-51, CP-58, CP-63, CP-66, CP-71, CP-82, CP-84, CP-86, CP-87, cộng CP-59.

Ledger R2 (cuối `CP-Full-Live-Test.md`) vẫn ghi BUG-503..507 "fixes pending". Bug doc cùng ngày đã ghi FIXED. Bản này lấy bug doc + test làm chuẩn.

## 1. Bug còn thật

### B-1. CP-87 — preflight không chạy trước turn hay child spawn

Spec yêu cầu resolver trước mọi hub/node (`CP-87-Quota-Aware-Provider-Model-Rotation.md` dòng 31–34 và 132; Task-449 T-1). Trên HEAD, `enterQuotaGate` chỉ được gọi sau khi một turn đã fail vì `quota_exhausted` / `credits_exhausted` / `billing_required` (`apps/local-runner/internal/runner/interactive_service.go` khoảng 9615–9635) hoặc khi cap CP-86 xin rotate. `startTurn` (cùng file, khoảng 9740) chỉ có context-reset, không gọi `ResolveQuotaPreflight`.

Ba run Devin song song trong R2 không có `account_claimed` khớp với code này.

`noteAccountBlockedLocked` có ghi account bị chặn (`quota_claim.go`). `accountBlocked()` không có caller nào — list chặn không được đọc lại, kể cả sau restart.

### B-2. CP-87 — headroom của context-reset không được gắn

`contextResetHeadroomOK == nil` được hiểu là đủ headroom (`interactive_service.go` khoảng 158–160). Callback này chỉ được gán trong test (`task449_quota_gate_test.go` khoảng dòng 454). Spec P-3b: reset cùng binding mà hết headroom phải vào routing gate. Production không bao giờ vào nhánh đó.

### B-3. CP-86 — cap và compaction mù với Claude

Cap cộng `TokenUsage.Total` (`context_usage.go` khoảng 40–45). Compaction cũng chỉ nhìn `Total` (`context_pressure.go` khoảng dòng 90). Mapper Claude chỉ set `Last` (`claude_event_mapper.go` dòng 91 và 126). Codex, Grok, Devin, OpenCode set cả `Last` lẫn `Total`.

Pressure ladder dùng `Last`, nên banner ≥80/≥90 vẫn có thể chạy nếu có context window. `maxUsageTokens` và sự kiện compact thì không thấy usage Claude. Test parity `TestTask442_ClaudeCodexGrok_Parity` tự bơm `Total`, không đi qua mapper. Máy này không có account Claude nên live chưa lộ.

### B-4. CP-64 — bug đã hết vẫn bị ép viết test đỏ

Suite pass thì `checkReproduceRule` trả violation "add an assertion that fails" (`internal/flowgate/reproduce_rule.go` khoảng 109–114). Không có lối thoát "not reproducible". `ask_user` không tăng reprompt cap, nên cap có thể không bao giờ tới.

Live `run-90420`: child park hai lần xin unblock, operator phải Stop. `TestBug391_…` chỉ giữ counter qua một lần giao reprompt. Không có test nào drive `attempts >= maxFlowGateReprompts` tới escalate. Ledger A-64-2 gọi cap là đã chứng minh — assertion không khóa nhánh đó.

### B-5. CP-65 — conflict hứa human-merge rồi pick lại là no-progress

`behaviorTournamentMerge` escalate "human merge required" và kèm patch (`tournament_behavior.go` khoảng 471–481). Card vẫn offer đúng candidate vừa conflict. R2 `run-3688`: pick lại → "no progress since last continue" → discard. `TestBehaviorTournamentMergeEscalatesOnConflict` chỉ khóa lần escalate đầu. Không có test cho pick lại, cũng không có lối nhận diff operator đã resolve.

### B-6. CP-51 — GET run đã sống sau restart, GET steps thì chưa

BUG-508: `runSnapshot` đọc session trên đĩa khi RAM trống (`interactive_handlers.go` khoảng 1734–1736). `workflowStepsRuntime` vẫn 404 khi run không nằm trong `s.runs` (cùng file, khoảng 1850–1852). Comment ngay trên hàm vẫn nói 404 giống snapshot. Sau restart, run completed trả 200 ở snapshot và `run_not_found` ở steps. Không có test cho nhánh steps, cũng không có test seed `cancelled`.

### B-7. BUG-504 và BUG-507 khóa một nửa failure live

**BUG-504.** Unit chứng minh tool được offer khi posture là `verdict_only` và `readOnlyHint: true` (`bug504_verdict_tool_exposure_test.go`). Live mới tới `tools/list`. Bug doc tự ghi owner-debate verdict còn pending (`run-23536`). File-drop `/tmp/submit_review_outcome.json` vẫn không có đường ingestion và không có test. Child review không mang posture `verdict_only` (nhánh `run-3688`) không nằm trong fix.

**BUG-507.** Unit cấm `completed` khi loop còn `running` (`interactive_service.go` khoảng 6343–6356, `bug507_approval_expiry_test.go`). Expiry có emit event trong RAM. Test không dựng `flowEngineDriven` cùng approval đang chờ, không assert card hay stall. Live `run-22241` chưa được drill lại.

## 2. Bug các turn trước — unit và live

| Bug | Unit khóa failure? | Live sau fix |
|---|---|---|
| BUG-503 cwd sai workspace | Có, 4 test trong `bug503_project_cwd_resolution_test.go` | Có trong bug doc: `run-23532`. Chưa ghi vào ledger R2 |
| BUG-508 `run_not_found` | Có với snapshot `completed` (`bug508_snapshot_rehydrate_test.go`) | Có: GET `run-6010` / `run-16950` → 200. Thiếu case `cancelled`, và thiếu steps (mục B-6) |
| BUG-509 body `text` thay `prompt` | Có, 400 (`bug509_empty_turn_body_test.go`) | Có trên `run-23515` |
| BUG-505 feedback bị nuốt làm draft | Có (`bug505_freeze_feedback_retry_test.go`) | Không |
| BUG-506 catalog outage | Có, pack fallback (`bug506_spawn_catalog_outage_test.go`) | Không. Outage Supabase khó ép lại; unit là lớp khóa chính, live vẫn trống |
| BUG-465 / 466 | 465 khóa dedupe xuyên turn. 466 khóa không `fresh_start` | Có trên `cht_10a27db90766`. 466 không khóa nội dung seed tới adapter |
| BUG-459 / 460 / 468 / 469 / 470 / 471 / 480 | Có | Có run id trong ledger |
| BUG-458 mirror mất `run` / `config` | Có `run`, `max_attempts`, context profile (`bug458_mirror_node_fields_test.go`) | `run-19067`. Assertion `Posture` không có, dù code có fill |
| BUG-388 / 397 lock deny | Có ở decision bridge | Live induce không làm: coder phải tự viết vào file đang lock. Compile-error wording thì đã có live `run-593` trong `CP-64-Test-Steps.md`; ledger A-64-3 quên cite |
| BUG-432 orphan waiting-approval | Có | Không có run id |
| BUG-391 reprompt cap | Không khóa escalate | Không |

Soft `low` 18% không chặn account đang pin là đúng spec (`quota_gate.go` khoảng 82–84). R2 không chứng minh được nhánh phải chặn (`exhausted` / `provider_limit`). Đó là live case thiếu, không phải case đã pass.

## 3. Live case còn mở

### 3.1 A — CP-60, CP-64, CP-65

Hàng A-60-4 ghi "all legs live-verified" là quá so với evidence. Đã có run id: R-SS `run-109797`, xóa CP `run-106927`, R-TK `run-102429` / `run-104945`, BUG-468..471.

Còn mở trong `CP-60-Test-Steps.md`:

- V7 `r-requirement` — `turn-22913` chỉ là probe, chưa fire card. Unit `TestRRequirement_*` có.
- F2 `/vibe off` rồi gate dev phải ra card `1/2/3`, không `vibe-owner-debate`.
- F3 thoát giữa SS-lock rồi mở lại, card và mode `vibe` còn.
- R-CP-D2 xóa cả CP lẫn SS khi đang `cp_writer`.
- N-CP / N-SS / N-TK / N-DEL. Ledger gọi "N-legs" là các ingest mới (`run-91517`, `run-91606`), không phải bốn hàng này.
- V8: Devin `run-16950` sạch. Grok `run-22241` chính là BUG-504/507. Checkbox Test-Steps vẫn `[ ]`.

A-58-4: DoD CP-58 và ledger nói cap 3, không có vòng 4. `task-harness.yaml` cố ý đặt cap 5 mỗi phase để plan loop không ăn hết budget của code loop. `cp-harness.yaml` vẫn cap 3. Test hiện nhảy thẳng `Round: 4`. Thiếu test đi 4 continue vẫn looping, continue thứ 5 thì `blocked`, và sau plan approve budget đếm lại từ 0.

A-65-4 parent resume: `TestResumeParentAfterTournament` chỉ set state `running` / `blocked`. Không apply patch, không chạy tiếp validate của parent. Mọi live tournament (`run-25153`, `run-3688`) là standalone.

Chip Desktop BUG-367 / 369 / 371: unit TUI có, live UI không.

`CP-65-Test-Steps.md` tự mâu thuẫn: §5 còn PARTIAL/BLOCKED, §6 đánh `[x]` "DONE via automated", §7 lại nói PARTIAL. P3 (Claude + Codex auth) và P4 (seed bug đã fail single-shot) vẫn `[ ]`. Claude không có account trên máy này.

### 3.2 B — CP-51, CP-58, CP-59

B-51-1..5 đã khóa cả unit lẫn live: kill-9 không phantom retry, stop không ghost RUNNING (BUG-464), reprompt key tăng, repair replay 200 / stale 409 (BUG-407/408).

Còn thiếu:

- Restart giữa turn trên chat nhiều leg. B-59-5 chỉ drill lúc idle.
- Seed prompt của switch thực sự vào `SendTurn`. Live Codex trên bed này trả "Done." cố định, nên byte seed không chứng minh model đọc context. `TestReattachEnvelopeProviderAgnostic` phủ opencode/grok/codex/claude giả, không có Devin.
- Clone `cp-harness-smoke` vẫn `flow is not startable` vì id builtin bị ẩn. Chưa có live clone với FlowRef mới đi hết implement chain.
- `TestBug404_ParkedSprintStateRoundTripsSession` không đi persist → load lại → cấm false-done. Boot `orphaned_work` tự re-drive không có unit riêng. Live B-51-7 `run-49191` có.
- Drive G1–G7 và reattach `closed/restored` bị chặn môi trường. Unit manifest có. Không tính là test thiếu.
- BUG-489 Phase C và BUG-493 timeline-khi-đã-có-resident-legs: store load một lần lúc boot, không inject được từ filesystem. Unit có. Ledger đã ghi. Không tính là test thiếu.

### 3.3 C — CP-63, CP-66, CP-71, CP-82, CP-84, CP-86, CP-87

- **CP-63.** R2 không chạy lại vì không có `gopls`. C-63-1 vẫn là hàng bắt buộc (`run-79069`, sev-1 trong reprompt, BUG-380). Degrade thiếu binary và crash budget có unit không cần `gopls`.
- **CP-66.** Live trên bed này chỉ là degrade 0 flow. Happy-path `knowledge.flow` trong prompt planner chỉ có evidence Windows cũ (`run-247069`). R2 distill fail trên `/tmp/fp-live-2026` là hệ quả BUG-503, không phải leg sạch. BUG-477 khóa replay ledger đã seed, không thay happy-path. Thiếu unit `ensureKnowledgeBaseForWorkspace` trên `.flowpilot/worktrees/` không gọi `Distill` (BUG-460 chỉ khóa autoindex).
- **CP-71.** L-1..L-8: unit HTTP và live b501 đủ, gồm BUG-472 / 473 / 481 / 501. R2 không chạy lại; b501 vẫn là case of record. M-1..M-10 desktop UI vẫn `[ ]`.
- **CP-82.** Hai project trên một mux và hai worktree khác dir/branch đã live b501. Unit second-project không restart process. M-1..M-7 desktop vẫn `[ ]`.
- **CP-84.** L-1..L-7: unit runner và live b501. BUG-479 / 482 là client unit; server không có chỗ inject frame hỏng. M-1..M-11 desktop vẫn `[ ]`. TUI không dùng mux này.
- **CP-86.** Telemetry đã live (`inputTokens` 19358 so với est 18; Devin `size` 262000). Ladder ≥80/≥90, compaction, cap extend/rotate/stop, và flag OFF byte-identical vẫn chỉ fixture. OpenCode/Devin không nằm trong loop parity Task-442/443.
- **CP-87.** Test-Steps §7 M-1..M-8 vẫn `[ ]` dù DoD đánh `[x]`. Live hard-quota và UI gate chưa có. Xem thêm B-1 và B-2.

## 4. Đã khóa — không cần đào lại

- CP-60 mode gate, snake chain `run-37268`, checkpoint R-SS / R-CP / R-TK, BUG-468..471 và 480.
- CP-64 RED→GREEN `run-14071` và `run-20370`, compile wording `run-593`, drift binary/log (BUG-456/457).
- CP-65 cohort + merge `run-25153`, empty-winner `run-23455`, discard `run-478live`, tie card BUG-414.
- CP-51 kill / stop / repair / reprompt (B-51-1..5), BUG-502 flock + rewind `sessions.ndjson` (live `git checkout` + runner thứ hai).
- CP-59 switch leg và dedupe BUG-465. Same-provider 409. Fail-closed store unreadable: BUG-488 live 502, BUG-489 Phase A live 502, BUG-493 history live 502.
- CP-71 / CP-82 / CP-84 ma trận b501.
- CP-63 missing-binary và doctor.
- CP-66 empty degrade và hook audit không chặn flow.

## 5. Việc nên làm tiếp (chỉ test và doc, chưa vá trong lượt này)

1. Sửa ledger R2: BUG-503 / 508 / 509 đã có live re-verify trong bug doc; 504 / 505 / 506 / 507 còn thiếu live sau fix.
2. Thêm unit: steps endpoint đọc session trên đĩa; `accountBlocked` được preflight đọc; Claude mapper điền `Total`; reprompt cap thực sự escalate; cap 5 của task-harness đi hết vòng; `Posture` của BUG-458.
3. Live còn đáng chạy khi có môi trường: V7, F2, F3, R-CP-D2; parent resume sau tournament; hard-quota CP-87; pressure/cap card CP-86; `knowledge.flow` trên bed có process; sev-1 gopls khi binary có lại; owner-debate verdict sau BUG-504.
4. Desktop UI (CP-71 M-1..M-10, CP-82 M-1..M-7, CP-84 M-1..M-11, chip BUG-367/369/371) vẫn cần session Desktop. API không thay được các hàng đó.

## 6. Disposition — 2026-09-26 (post-review fix round)

Mỗi claim đã được verify lại với source trước khi capture. Kết quả:

| Claim | Verdict | Outcome |
|-------|---------|---------|
| B-1 preflight chỉ chạy post-failure | Đúng (phần cốt lõi); `accountBlockReason` THỰC RA được đọc trên claim path — chỉ wrapper `accountBlocked()` + initial-pin check là dead | **BUG-511 FIXED** (CA-1012): `startTurn` chặn pinned-blocked trước dispatch → `quota_route_required` + quota card |
| B-2 `contextResetHeadroomOK` prod-nil | Đúng — chỉ test assign | **BUG-512 FIXED** (CA-1013): default wired trong `newInteractiveService`, P-3b branch sống lại |
| B-3 Claude mapper thiếu `Total` | Đúng — `mapClaudeResult` chỉ điền `Last`; parity tests inject `Total` trực tiếp | **BUG-513 FIXED** (CA-1014): result usage marked `UsageScopeQuery`, emit seam cộng dồn per provider-session, seed từ durable events sau restart; codex/grok/devin cumulative passthrough (parity: flag chỉ Claude set) |
| B-4 reproduce không có lối thoát "not reproducible"; `ask_user` không tính reprompt | Đúng — park trong-turn, `repromptAttempts` không tăng, cap vô hạn (run-90420) | **BUG-514 FIXED** (CA-1015): reproduce-gated `ask_user` đếm vào `repromptAttempts`; hết budget → refuse → turn kết thúc → gate escalate theo path có sẵn. Non-reproduce không đổi |
| B-5 conflict card re-offer winner không conflict marker | **KHÔNG đúng** — merge card đã gắn `(picked winner — patch conflicts)` trong label; include là chủ đích (BUG-478: recorded patch vẫn mergeable tay) | Không fix — hành vi đúng thiết kế |
| B-6 steps endpoint 404 post-restart | Đúng — cùng class BUG-508 | **BUG-510 FIXED** (CA-1011): `handleRunSteps` fallback `durableRunSnapshot` |
| B-7 file-drop ingestion thiếu | **Không phải bug** — "file-drop convention" là review tưởng tượng; không có consumer nào đọc `/tmp/submit_review_outcome.json` | Chỉ test-strengthening đề xuất — chưa làm (không có semantic cần khóa) |
| CP-63 gopls env-blocked | Đã unblock — user cài `gopls` | **C-63-1 live re-verified** trên fixed binary: `lsp.start binary="gopls"` + sev-1 cross-GOOS diagnostic trong reprompt turn-1865606 (run-1865052). Unit `internal/lsp` + runner LSP green |

Suite sau 5 fix: chỉ còn env/baseline fails (6 Supabase/provider-discovery/firebase leaks — fail identically trên clean HEAD, stash-verified; + suite-pollution flakes pass isolated). Zero regression quy về fix set.

## 7. Fix report — session 2026-09-26 (sau review)

Trạng thái tổng: **11 bug đã fix trong session** (7 từ R2 live-test trước + 5 từ review này — BUG-510..514; BUG-510..514 là capture mới của round review). Tất cả đều theo safe-fix: red test → prod fix → additive tests → CA → suite → live khi có thể.

### 7.1 ĐÃ FIX + LIVE-VERIFIED

| Bug | Fix | Live evidence |
|-----|-----|---------------|
| BUG-503 | `createRun`: explicit cwd → existing `Project.Path` → runner workspace (CA-1004) | run-23532 bound `/Users/tiendat/fp-beds/full` (pre-fix sibling bound `/tmp/fp-live-2026`) |
| BUG-504 | verdict tool cho `verdict_only`/verdict-flow hosts; `readOnlyHint` trên 4 interaction tools (CA-1009) | live `tools/list` trên devin+grok; grok `plan_reviewer` run-25737 đã ghi verdict `review_verdict_recorded` |
| BUG-508 | `runSnapshot` durable fallback (CA-1005) | run-6010/run-16950 → 200 post-restart (trước đó `run_not_found`) |
| BUG-509 | turn body không nội dung → 400 (CA-1006) | `{"text":…}`/`{"stepId"}`-only → 400 live |
| BUG-510 | `handleRunSteps` durable fallback (CA-1011) | unit-only (post-restart read cùng class BUG-508 đã live) |
| CP-63 C-63-1 | — env unblock (gopls installed) | run-1865052 fixed binary: gate-clean turn → `lsp.start binary="gopls"` → sev-1 `probe_windows.go:5:8 fmt unused [windows,amd64]` vào reprompt turn-1865606 |

### 7.2 ĐÃ FIX + UNIT-VERIFIED (chưa có live organic)

| Bug | Fix | Live gap |
|-----|-----|----------|
| BUG-505 | freeze feedback parse-as-draft else retry planner delegate (CA-1007) | cần organic freeze-escalate |
| BUG-506 | `FlowRefFallback` → embedded-pack resolution on catalog outage (CA-1008) | không inject outage an toàn live |
| BUG-507 | `approval_expired`/`question_expired` durable events; `completed` withheld khi loop mở (CA-1010) | run-23536 devin vibe → `completed` clean không loop (consistent); expiry cần TTL để lên organic |
| BUG-511 | blocked pinned account → quota gate tại admission (CA-1012) | cần ledger blocked thật / multi-account env |
| BUG-512 | `contextResetHeadroomOK` wired prod (CA-1013) | cần node-cap reset scenario organic |
| BUG-513 | Claude result usage → query-scoped accumulation vào `Total` (CA-1014) | cần Claude account thật; parity structurally proven (flag chỉ Claude set) |
| BUG-514 | reproduce ask_user đếm vào reprompt budget; hết budget → refuse → escalate (CA-1015) | cần organic reproduce park (run-90420 pattern) |

### 7.3 REVIEW ĐÃ ĐÁNH GIÁ — KHÔNG FIX

- **B-5** (conflict card re-offers winner): không phải bug — `(picked winner — patch conflicts)` marker đã có trong label; inclusion là chủ đích (BUG-478).
- **B-7** (file-drop ingestion): convention tưởng tượng — không consumer; chỉ test-strengthening được đề xuất.

### 7.4 REGRESSION ĐÃ TÌM + FIX TRONG SESSION

- `TestWorkflowDrivenQuestion` / finalizer: fake `Project.Path` bind thật → BUG-503 thêm existence check.
- `TestTryAdvanceFlowThroughInlineDispatchesHubNotify`: BUG-507 withhold `break` trước `signalChild` → giữ signal, chỉ suppress status.

### 7.5 PENDING — cần môi trường/hoặc còn thiếu

| Item | Trạng thái | Cần gì |
|------|-----------|--------|
| Live BUG-505/506/507/511/512/513/514 | unit-verified only | organic trigger paths (quota blocked ledger, catalog outage, approval TTL expiry, reproduce park, Claude account) |
| `run-1` continue post-restart | `run_not_found` — mutation/continue paths vẫn memory-bound | gap durability còn mở (BUG-508 chỉ phủ reads) — candidate bug mới |
| CP-60 V7/F2/F3, R-CP-D2 | chưa chạy | live legs |
| CP-65 parent resume sau tournament | unit-only | live tournament escalation |
| CP-66 happy path `knowledge.flow` | chỉ degrade/Windows cũ | bed có process; + thiếu unit `ensureKnowledgeBaseForWorkspace` trên worktrees (không gọi Distill) |
| CP-86 pressure ≥80/≥90, compaction, cap extend/rotate/stop; flag OFF byte-identical | fixture-only | live legs; OpenCode/Devin parity chưa trong loop Task-442/443 |
| CP-87 hard-quota, UI gate (§7 M-1..M-8 `[ ]` nhưng DoD `[x]`) | chưa live | quota env + desktop session |
| CP-71 M-1..M-10 / CP-82 M-1..M-7 / CP-84 M-1..M-11 | `[ ]` — API không thay được | desktop UI session |
| task-harness cap-5 full loop | thiếu unit | viết test mới |
| BUG-458 `Posture` surface | thiếu unit | viết test mới |
| Devin verdict_only child leg | run-26952 đang chạy (doc-writer → reviewer) | chờ reviewer spawn |

### 7.6 Baseline failures (KHÔNG phải regression — verified trên clean HEAD)

`TestCatalogStoreForFallsBackToFake`, `TestFlowDefinitionStoreForUnconfiguredRunnerYieldsNil`, `TestCleanupSessionsTearsDownProviderPools`, `TestDetectProvidersPopulatesInventoryShape` (6 providers), `TestFirebaseToolsMcpAdapterFetchEndToEnd`, `TestResolveGoogleDriveMcpProviderStatuses_AllNotStarted` — env leaks (Supabase creds thật, claude process thật, CLIs). Suite-pollution flakes (5) pass isolated; `TestBUG462` flake 3/5 trên HEAD unmodified.

---

## 8. ROUND-2 FIXES — review follow-up của Grok (2026-09-26)

Review thứ hai của Grok chỉ ra chính xác ba chỗ round-1 nói rộng hơn code.
Toàn bộ đã fix + additive tests xanh. CAs: CA-1016..CA-1021.

### 8.1 Fixed + unit-verified

| Item | Round-2 fix | Tests mới |
|------|-------------|-----------|
| BUG-511 ledger-only + 409 loop + nuốt prompt | `pinnedAccountHardVeto` (ledger HOẶC telemetry exhausted); `resolveQuotaGate` trả outcome; rotate same-provider → repin + prompt dispatch tiếp; cross-provider → relay sang leg mới; `QuotaBlocked` → `quota_route_blocked` thành thật, không card ảo | +3 (`TelemetryExhaustedPinEntersGateWithoutLedger`, `SingleAccountBlockedFailsHonestlyNoCard`, `AutoRotateRepinsAndContinues`) |
| BUG-512 node-budget ≠ account headroom | callback giờ check `pinnedAccountHardVeto` trước (ledger/telemetry exhausted → refuse reset → routing gate), giữ node budget là guard thứ hai cho capped nodes | +3 (ledger-blocked / telemetry-exhausted / healthy trên uncapped node) |
| BUG-513 compaction vẫn mù trên Claude | round-2/3 đã thử `Last`-drop + gate ≥80% window — vẫn unsound (Last ≠ session fullness). Round 4 (CA-1025): leg query-scoped blind hoàn toàn khỏi `provider_compacted`; chỉ cumulative legs detect drop; `Total` accumulation cho caps giữ nguyên | +4 (blind on per-query drop incl. Grok's near-window shape, no false positive on growth, unknown window blind, cumulative Total drop intact) |
| BUG-514 chia counter với gate | bound giờ đếm `EventUserQuestionRequired` trên durable event stream của run — không đụng `repromptAttempts`; restart-safe free; escalate ladder BUG-391 nguyên vẹn | sửa assertion file mình viết +1 (`ReproduceAskBoundSurvivesRehydratedEvents`) |
| BUG-510 envelope thiếu model/yolo | fallback copy `sess.ModelName` + `sess.Yolo` | +1 (`StepsRuntimeHydratesModelAndYolo`) |
| B-5 nửa sau (→BUG-515) | `extractOperatorPatch(feedback)`: `diff --git` header hoặc `---/`+++` pair; strip fence, giữ trailing newline (corrupt-patch trap). Operator diff override stored snapshot; prose giữ path cũ; conflict re-park thành thật | +2 (`OperatorResolvedDiffOverridesStoredPatch`, `ProseFeedbackKeepsStoredPatch`) — file `bug515_merge_operator_diff_test.go`, bug doc BUG-515 |

### 8.2 Semantics notes

- **BUG-511 trigger**: veto string = ledger stored reason (là `ProviderLimitKind` thật khi đến từ provider 402/403) hoặc `quota_exhausted` cho telemetry → `ObservedLimit` được set, pin hard-reject trong candidate pass. Trigger `"account_blocked"` untyped của round-1 không còn.
- **BUG-512**: P-3b giờ đúng nghĩa — reseed chỉ khi account ghim có headroom; uncapped nodes không còn bypass.
- **BUG-513**: `legUsageTotals` membership (populate trước eval trong `emitLocked`) đánh dấu leg query-scoped. Cap accounting (`Total` accumulate) hoạt động bình thường; compaction detection chỉ còn trên cumulative legs — `Last` per-query của Claude không phải session fullness (round 4).
- **BUG-514**: bound đếm trên stream durable → sống sót restart; gate reprompt budget nguyên 2 lần cho turn-end violations; escalate path vẫn là nhánh `reprompt` hiện hữu của gate_hook (coverage: BUG-391).
- **BUG-515**: không thêm option `merged_manually` riêng — feedback-diff đủ nghĩa và giữ card 4 options như cũ.

### 8.3 Pending (sau round 3)

Các item đã live-verified được ghi ở 8.5 (BUG-510, BUG-511 manual+auto,
BUG-504 devin arm). Còn lại vẫn cần môi trường organic: `quota_route_blocked`
no-candidate (máy luôn có ≥1 alternate), Claude compaction
(không có Claude account), reproduce park (pattern run-90420), tournament
conflict, context-reset seam. `run-1` continue-post-restart gap (mutation
paths memory-bound) vẫn mở — BUG-508/510 chỉ phủ GET.

**Round-3 findings (Grok review) → BUG-516/517 filed; BUG-513 gated:**

- **BUG-516** (CA-1022): `startTurn` không check `legState` — closed leg
  vẫn dispatch turn trên binding cũ, âm thầm bypass committed route.
  Fix: relay sang active leg / 409 `leg_closed`. run-30802→PONG-511 ở
  trên là evidence CỦA BUG (turn chạy trên leg đã đóng); evidence của fix
  là PONG-516 relay sang grok (8.5).
- **BUG-517** (CA-1023): `respawnChildOnRoute` seed replacement bằng
  `child.lastPrompt` (stale + truncate 100 ký tự); `in.Prompt` của turn bị
  chặn không đi tới đâu. Fix: `QuotaResolution.PendingPrompt` xuyên commit.
- **BUG-513 round 3** (CA-1024): query-scoped `Last` drop >30% cũng xảy ra
  khi turn chỉ ngắn hơn — false positive `provider_compacted`. Gate
  ≥80% window đã thử nhưng vẫn unsound (round 4 sửa hẳn, xem dưới).

**Round-4 findings (Grok review) → CA-1025:**

- **BUG-513**: `Last` per-query của Claude không phải session-context
  occupancy — long query → short query vẫn thỏa "prev ≥80% + drop >30%"
  mà không hề có compaction. Fix: leg query-scoped (member
  `legUsageTotals`) hoàn toàn blind với `provider_compacted` —
  `context_degraded` không bao giờ được mark; detection chỉ còn trên
  cumulative legs (Devin/Grok/Codex `Total` thật sự session-cumulative).
  `Total` accumulation cho cap accounting giữ nguyên; pressure ladder
  (aware/ask card từ `Last`/window) giữ nguyên — nó hỏi operator chứ
  không tuyên bố compaction. Residual: Claude compaction không được phát
  hiện — đúng cho tới khi có session-fullness signal thật.
- **BUG-517**: test admission-path mới
  `TestBug517_AdmissionCarriesInflightPromptToRespawnedChild` đi qua
  `startTurn` thật (child pin blocked codex → auto rotate claude →
  respawn với in-flight prompt; stale `lastPrompt` được gài để chứng
  minh prompt thua). Verified RED khi xóa `res.PendingPrompt = in.Prompt`.

### 8.4 Suite classification (2026-09-26, round-2 full run)

`go test ./internal/runner -count=1 -timeout 15m` — completed **655s**
(round-1 "timeout" was Go's default 600s limit on a loaded machine, not a
deadlock — verified: suite completes with headroom at 15m).

| Class | Tests | Status |
|-------|-------|--------|
| Env-baseline (verified fail on clean HEAD) | Firebase MCP fetch, GoogleDrive provider statuses, CatalogStore fallback, DetectProviders inventory (6 providers), CleanupSessions pools, FlowDefinitionStore unconfigured | machine leaks — real Supabase creds/CLIs; NOT regressions |
| Pollution flakes (pass isolated) | BUG462 (3/5 on unmodified HEAD), VibeSprintFreezeSpawnsTddThenCoder, StartTurnGrokCrossAccount..., Bug414_TieCardRetry (TempDir unlinkat race — gitnexus auto-indexer writes `.gitnexus/` into test workspace post-test; pre-existing), FlowCodingPromptSpawnWrapped, ListProviderAccountsRecoversManagedCodexSlots, PlanApprovalPark/grok, RunHistoryWorktreePath | pass standalone; BUG414 repro on `-count=3` is cleanup race only — assertions green |
| Round-2 regression | — | **none** — all 29 TestBug5xx + neighbors green |

### 8.5 Live verification (round-2 binary, :19400, 2026-09-26)

| Item | Live result |
|------|-------------|
| BUG-510 | ☑ `GET /client/workflow-runs/run-6010/steps-runtime` post-restart → 200, envelope `provider:grok model:grok-4.5 yoloMode:true` hydrated from durable session row; `run-23534` returns full step rows |
| BUG-511 | ☑ **manual mode**: devin `run-30802` pinned to ledger-blocked `26d5c967` → `POST /turns` → **409 `quota_route_required` before dispatch**; card carries real `trigger:quota_exhausted`; live candidate pass scored 9 alternates with real headroom (devin `low` 18%). ☑ **auto mode**: grok `run-30810` pinned to blocked `506659bef` → durable repin to `32a2460d` → turn dispatched → `PONG-511auto` — prompt not swallowed. ⚠ **card-resolve arm**: `use_once` answer minted grok leg `run-31122` and durably closed run-30802 — but the re-sent turn dispatched on the CLOSED leg and completed `PONG-511` on devin (leg `run-31122` stayed idle). That exposed **BUG-516** (closed leg accepted turns — fixed CA-1022). **Round-3 binary re-verify**: turn re-sent to `run-30802` relayed onto `run-31122` → `PONG-516` completed on grok — cross-provider card-resolve → turn-on-new-leg now live-verified end-to-end |
| BUG-504 devin arm | ☑ `run-30824` task-harness (devin/swe-2-high): plan_reviewer child `run-31709` called `submit_review_outcome` → `appr-31867` resolved/approve → chain advanced to test_signatures — the exact leg that blocked `run-1` pre-fix |
| BUG-512 | ◑ account-headroom veto shares the live telemetry data path verified in BUG-511; the context-reset seam itself needs organic pressure — unit-verified |
| BUG-513 | ◑ no connected Claude account on this machine (`no_connected_account` in the live quota candidate pass) — unit-verified only; round-4 posture: query-scoped legs blind to compaction, cumulative legs (Devin/Grok) retain detection |
| BUG-514 | ◑ needs organic reproduce-park (run-90420 pattern) — unit-verified incl. rehydrate |
| BUG-515 | ◑ needs organic tournament conflict — run-3688 old-behavior evidence stands; unit-verified both arms |
| `quota_route_blocked` | ◑ unstageable here — machine always has ≥1 alternate provider; unit-verified |

Runner cleanup post-drill: seeded ledger removed, quota mode restored to
`manual`, fresh devin run unaffected by the removed veto.

## 9. ROUND-5 — always-on flips + tournament-chain bugs (2026-09-26 late)

Operator directive: MVP chưa release — bật hết CP feature, không cổng env.
Provider scope live: **Devin + Grok only** (không có account Claude/Codex/
Gemini trên máy — các gap đó là environmental, không phải lỗi implementation).

### 9.1 Always-on (CA-1029)

- Hard-enabled: `contextPressureEnabled`, `driftDetectorEnabled`,
  `tournamentEscalationEnabled`, `budgetPackerEnabled` (+ byte-identical
  passthrough khi không drop gì — giữ verbatim-prompt semantics).
- Đã-on sẵn: chat SSOT, reproduce gate, drive MCP, devin/grok/opencode
  agents, DispatchV2 (opt-out giữ — documented kill-switch).
- Env-gated giữ nguyên: `codexAppServerEnabled` — transport switch, không
  phải feature gate; always-on phá codex resume contract tests và máy
  không có codex binary.
- Fallout đã xử lý: cap-park tests assert `tournament_escalation` thay
  `blocked` (contract mới — rescue child spawn); drain helper
  (`waitForTournamentChildIdle`/`drainTestService`) chặn async child rơi
  vào TempDir teardown. Suite sạch regression; còn env-baseline failures
  (Firebase MCP, Supabase-backed stores, claude-pool reap) + TempDir
  flake family đã phân loại.

### 9.2 Bugs mới tìm bằng live drill

| Bug | Root | Fix | Test | Live |
|-----|------|-----|------|------|
| BUG-518 | `git add -N -- . ':(exclude).flowpilot'` exit 1 khi `.flowpilot` bị ignore → arbiter snapshot fail, escalate wipe worktrees, park không card | enumerate `ls-files -z --modified --others --exclude-standard` → `add -N --pathspec-from-file`; **vector 2** (run-69516): repo TRACK `.flowpilot` files → `--modified` không qua exclude-standard → lọc prefix `.flowpilot/` khỏi set | 2 test mới; BUG-453 giữ nguyên xanh | ☑ r9 run-76075/82594: 0 snapshot error qua mọi round |
| BUG-519 | `tournamentJoinSatisfied` chỉ scan `s.runs`; terminal children không rehydrate post-restart → join chờ vô hạn | fallback durable `StepTransitionLogStore` (DONE/FAILED/CANCELED/SKIPPED terminal; fail-closed khi thiếu evidence) | 3 test mới | ☑ run-82594: kill tại parked merge card → resume `blocked` → card + patches rehydrate → continue → merge → audit → completed |
| BUG-520 | `hasActiveFlowChild` không tính armed `pendingGateRepromptPrompt/StepID` là activity → watchdog park `hub_stalled` mid-cohort → park wipe intent → orphan (run-60145, durable `pending_gate_reprompt_prompt:null`) | armed reprompt = active + shield ghost; `pendingFlowGateSettle` riêng vẫn không count (BUG-354) | 2 test (RED→GREEN) | ☑ 0 `hub_stalled` trên r8/r9; r7 control park đúng window |

### 9.3 Live matrix round-5 (devin/grok)

CP-51 ☑ devin turn-49070 terminal_completed · CP-59 ☑ devin→grok leg
handoff · CP-60 ☑ `invalid_cp_source` admission fence · CP-82 ☑ parallel
2-project · CP-84 ☑ mux stream 4 runs/2 projects · CP-58 ☑ run-49107
audit DONE · CP-64 ☑ run-61850 grok full RED→lock→GREEN→audit ·
CP-65/71 ☑ tournament full lifecycle incl. tie-retry, conflict card,
operator-resolved-diff merge (BUG-515 live leg) · CP-86 ☑ usage fields
trên dispatch records · CP-63 ◑ gopls now in PATH; R3 evidence stands ·
CP-66 ◑ BUG-503-collateral paths; no clean-bed leg · CP-87 ◑ quota legs
fixture-only (machine has ≥1 healthy alternate always).

### 9.4 Residuals mới (filed, chưa vá)

- **run-60145 dishonest terminal** (BUG-520 residuals): parent
  `completed` khi `candidate-b` WAITING_USER_APPROVAL + arbiter/merge
  PENDING. Hai follow-up: (a) orphan-cure — child parked với wiped
  reprompt (`pending_gate_code_paths` armed) không có re-drive path khi
  unpark; (b) status-honesty — outer status không được `completed` khi
  node bắt buộc còn pending.
- **Deterministic tie**: 3 tournament liên tiếp tie 1.0000 trên task
  tầm thường → retry×cap → human card — đúng thiết kế; leg human-decision
  được exercise bằng card path.
- **Kill mid-flight → cancelled**: resume normalize persisted
  `status=running` → cancelled = designed fail-closed; restart legs phải
  kill tại parked state.
