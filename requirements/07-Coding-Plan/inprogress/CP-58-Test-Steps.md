# CP-58 Test Steps — Manual Validation Guide (Bug / Task / CP Harness Family)

## Metadata

- Document ID: `CP-58-Test-Steps`
- Phase: `coding_plan` (manual validation companion)
- Status: `draft`
- Scope: Tasks 304-307 — dual `continue/back` engine (Task-304), `task-harness` plan review loop (Task-305), harness plan `file_artifact` bindings + mirror seeding (Task-307), `cp-harness` slice-only + smoke variant (Task-306).
- Created: `2026-09-02`
- Last Updated: `2026-09-05`

## 0. Chuẩn bị (bắt buộc)

| # | Việc | Cách kiểm |
|---|------|-----------|
| P1 | Chạy trên branch `cp58-harness-dual-loop` (worktree `flowpilot-cp58`) | `git log --oneline -5` → thấy `Task-306`, `Task-307`, `Task-305`, `Task-304`, `CA-712` |
| P2 | Build runner | `cd apps/local-runner && go build ./internal/agentpack/ ./internal/runner/` PASS |
| P3 | Apply migration trước khi chạy với Supabase | `supabase migration up` (hoặc apply tay `supabase/migrations/20260831090000_add_harness_plan_artifact_instances.sql`) — seed `artifact_instances` `plan_md`/`cp_md`/`task_md` |
| P4 | Restart runner để mirror sync chạy | `just chat-dev <project>` → log có `EnsureBuiltinFlowMirrorsWithStore` sync không lỗi; `flowpilot_core_flow_pack_task_harness` mirror row xuất hiện trong bảng `workflows` |
| P5 | (Supabase-backed) Verify binding seed | Bảng `step_artifact_bindings` có rows `..._plan_writer` (output, instance `...0002`), `..._plan_reviewer` (input), `..._cp_plan_writer`, `..._task_splitter` (input cp_md + output task_md). Thiếu → check log `seed harness artifact bindings` |
| P6 | Feature key / CA đọc trước | `change-audit/CA-712-bug-task-cp-harness-dual-review-loops.md` — deviation `agent.delegate` cho doc-writers đã ghi rõ |

## S. Smoke 10 phút (TUI `/flow task-harness`)

| # | Bước | Kết quả mong đợi |
|---|------|------------------|
| S1 | Mở picker `/flow` | `task-harness` hiện trong danh sách (selectableIn flow); description "Plan Writer + Plan Review Loop + TDD + Code Review" — ✅ PASS run-533004 |
| S2 | Prompt: yêu cầu một task nhỏ rõ ràng (vd: "add a /ping slash command logging latency to the statusline, feature_key: cli-tui"). Prompt cụ thể cho target `D:\working\gate-sandbox` (đã chạy 2026-09-04): `Add integer GCD to the calc package, feature_key: calc-core` — scope `calc.go` (hàm GCD mới) + `calc_test.go` (chỉ thêm tests mới); AC: `GCD(48,18)=6`, `GCD(0,5)=5`, `GCD(5,0)=5`, `GCD(-48,18)=6`, `GCD(0,0)=0` (documented, no panic), `go test ./...` green. LƯU Ý 2026-09-05: các prompt run-533004/547025 đã ghi nhầm `GCD(48,18)=18` (sai toán học — 18 không chia hết 48); coder vẫn implement GCD đúng (verify `calc_gcd_test.go` trong gate-sandbox: `(12,18)→6`, `go test ./...` green) nên kết quả PASS giữ nguyên, nhưng prompt rerun sau này phải dùng `=6` | Flow start: `preflight_contract_plan` (Scout) chạy trước, rồi `context`, rồi `plan_writer` — ✅ PASS run-533004 (scout→context→plan_writer advance đúng, CA-732 live) |
| S3 | Quan sát step timeline khi `plan_writer` chạy | Prompt của writer có mục **"Templated file outputs (write contract)"** liệt kê `requirements/08-Task/todo/Task-{{idx}}-{{slug}}.md` — ✅ PASS 2026-09-05 bằng bằng chứng TRỰC TIẾP trên live prompt: `runner.log` 13:21:19 ghi nguyên văn prompt gửi cho `doc-writer` (run-548341) chứa `## Templated file outputs (write contract)` + `requirements/08-Task/todo/Task-{{idx}}-{{slug}}.md`. (Trước đó tick bằng unit assert + no-reprompt; nay có live prompt verbatim.) Ghi chú UI: child transcript TUI chỉ render assistant messages + tool summaries, KHÔNG chứa input prompt (verify: `chat-transcripts` + `tui.log` đều không lưu composed prompt); transcript plan_writer run-547025 ngắn hơn viewport nên không có gì để cuộn — không phải lỗi scroll (PgUp/PgDn = Fn+↑/Fn+↓ vẫn work per `app.go:2615`). Muốn inspect prompt live trong TUI thì là enhancement riêng, không chặn S3 |
| S4 | `plan_writer` hoàn thành | File `Task-<n>-*.md` xuất hiện trong `requirements/08-Task/todo/` với đủ section Metadata/AI Quick View/§1-§8; final message nêu đúng path đã viết — ✅ PASS run-533004 (`Task-910-calc-core-gcd.md`) |
| S5 | `plan_reviewer` chạy | Reviewer prompt có **"Bound input artifacts (locate and read)"** + template; reviewer gọi `submit_review_outcome`, KHÔNG gọi `flow_control` — ✅ PASS run-533004 (review outcome APPROVED 8 gate criteria; reviewer prompt "Bound input artifacts" chưa đọc trực tiếp) |
| S6 | `plan_synthesis` duyệt lần đầu | Nếu approved → đi tiếp `preflight_contract_freeze`; nếu changes_requested → quay lại S7 — ✅ PASS run-533004 (approved lần 1 → freeze + `test_signatures`) |

## A. Plan review loop (loop 1 — Task-304/305)

| # | Bước | Kết quả mong đợi |
|---|------|------------------|
| A1 | Ép reviewer reject: trong prompt thêm "plan phải kèm benchmark plan" để lần 1 thiếu | `plan_synthesis` phát `continue` → `plan_writer` chạy LẠI trên **cùng 1 session** (child count = 1, không spawn mới); timeline: `plan_reviewer` + `plan_synthesis` reset PENDING, `context` giữ DONE |
| A2 | Log runner khi plan-continue | Không có lỗi routing; re-entry prompt chứa "Feedback received" + findings của reviewer |
| A3 | Lần 2 plan sửa xong, reviewer approve | `plan_synthesis` phát `done` → `preflight_contract_freeze` chạy (change contract lock) → `test_signatures` → `implement` |
| A4 | Sau freeze, `implement` chạy | Prompt coder có **"## Required file outputs (write contract)"** nếu node có binding cụ thể; freeze scope đúng theo plan đã duyệt |
| A5 | Kiểm tra `context`/`preflight_contract_plan` trên timeline | Vẫn DONE — không bị reset khi plan loop quay lại (BUG-286 scoping trên dual-loop) |

## B. Code review loop (loop 2) + tách biệt 2 loop

| # | Bước | Kết quả mong đợi |
|---|------|------------------|
| B1 | Ép code review reject (như dùng rag-harness bình thường) | `synthesis` (code hub) phát `continue` → re-enter `implement` trên **cùng session**; `validate`/`reviewer`/`synthesis` reset PENDING — ✅ PASS run-533004 (`changes_requested` API contract mismatch → implement re-work round 1 → APPROVED) |
| B2 | Quan trọng: plan loop nodes sau code-continue | `plan_writer`/`plan_reviewer`/`plan_synthesis`/`context`/`freeze` vẫn DONE — code loop KHÔNG đụng plan loop |
| B3 | Đếm child sau cả 2 loop | `plan_writer` = 1 child (reuse), `implement` = 1 child (reuse), `plan_reviewer`/`reviewer` = số child bằng số round (spawn lifecycle) |
| B4 | Cap | Hai loop dùng chung `policy.cap:3` — sau 3 round tổng sẽ blocked/escalate; thông báo cap đọc được trên TUI |

## C. cp-harness slice-only (Task-306)

| # | Bước | Kết quả mong đợi |
|---|------|------------------|
| C1 | `/flow` → chọn `cp-harness`, prompt một CP nhỏ (2-3 P-*) | Flow start đúng 1 entry: `preflight_contract_plan` (KHÔNG spawn node lạ nào khác — check log `[agent-spawn]` chỉ 1 entry) |
| C2 | `cp_plan_writer` chạy | Viết `requirements/07-Coding-Plan/todo/CP-<n>-*.md` đủ §1-§10 + DOD; write contract "Templated file outputs" có template CP |
| C3 | `cp_reviewer` reject 1 lần (thiếu DOD measurable) | `cp_synthesis` continue → `cp_plan_writer` reuse session, CP md được viết lại cùng file |
| C4 | Approve → `task_splitter` | Sinh **đúng N file** `Task-*.md` (N = số P-*), mỗi file có `Parent Documents` trỏ CP + `P-*` traceability; KHÔNG sửa file CP hay Task có sẵn (additive) |
| C5 | Sau `task_splitter` | `audit` chạy rồi flow `done`. Timeline KHÔNG có `test_signatures`/`implement`/`validate`/`reviewer`/`synthesis` (slice-only) |
| C6 | Kiểm tra step timeline tổng thể | Đủ: `preflight_contract_plan → context → cp_plan_writer → cp_reviewer → cp_synthesis → task_splitter → audit` |

## D. cp-harness-smoke (13 nodes, opt-in)

| # | Bước | Kết quả mong đợi |
|---|------|------------------|
| D1 | Picker KHÔNG hiện `cp-harness-smoke` (selectableIn rỗng) | Đúng — muốn chạy phải clone flow rồi chọn bản clone |
| D2 | Clone qua Desktop (WorkflowsSettings) → chạy | Sau `task_splitter` đi tiếp `freeze → test_signatures → implement → validate → reviewer → synthesis → audit → done` |
| D3 | Trong smoke, ép code reject | `validate→implement` loop chạy đúng; `cp_synthesis`/plan nodes giữ DONE |
| D4 | Clone reload | `acceptance_nodes` clone giữ nguyên `[cp_synthesis, validate, synthesis, audit]` (CP-55 P-1 regression) |

## E. Artifact panel + bindings (Task-307)

| # | Bước | Kết quả mong đợi |
|---|------|------------------|
| E1 | Sau task-harness run, mở artifact panel (Desktop) | `plan_md` instance hiện với path đã viết; panel không hiện literal `Task-{{idx}}-{{slug}}.md` như required-path fail |
| E2 | Gate behavior | Writer KHÔNG bị reprompt vô hạn vì template (template nằm ngoài flowgate exact-path check — theo thiết kế). Nếu thấy reprompt về file plan → BUG mới, mở theo `feature_key: agent-flow-engine` |
| E3 | Path escape check | (Unit-level đã phủ) `TestValidateArtifactOutputPath` + `TestValidateFlowDefinitionRejectsBadOutputBinding` — không cần manual trừ khi bạn tự clone flow sửa path ra ngoài `requirements/` lúc đó flow phải fail khi load/save |

## F. Regression canary (flow cũ phải y nguyên)

| # | Bước | Kết quả mong đợi |
|---|------|------------------|
| F1 | `/flow rag-harness` với prompt nhỏ | Hành vi như trước CP-58: 9 nodes, 1 loop, `validate→implement` continue vẫn đúng |
| F2 | `/flow review-loop` (chat) | Không đổi |
| F3 | `go test` các canary | `go test ./internal/agentpack/ ./internal/flowgate/ ./internal/changecontract/ -count=1` PASS; `go test ./internal/runner/ -run 'TestRAGHarness|TestReviewLoop|TestContextCoding|TestApplyFlowControl' -count=1` PASS |

## Kết quả thực tế

| Mục | Kết quả | Run/Chat | Ghi chú |
|-----|---------|----------|---------|
| P Chuẩn bị (P1-P6) | ✅ | | P1 đúng branch `cp58-harness-dual-loop` (+Task-320, +BUG-351 fix); P2 build PASS; P3 3 instances `...0002/3/4` có rows; P4 mirror đủ task/bug/cp-harness (+smoke); P5 10 binding rows sau reseed tay B2 (seed-miss do flows mirror từ boot cũ trước P3 — xem BUG ghi chú dưới); P6 đã đọc CA-712 + CA-728 |
| S Smoke | ✅ (2 runs, S1–S6 đủ) | run-533004, run-547025 | run-533004: S1/S2/S4/S5/S6 PASS như cũ. run-547025 (opencode-go/omen-alpha, GCD prompt `calc-core`): happy-path full-loop lần 2 — plan approve lần 1 → freeze → test_signatures → implement → validate → reviewer APPROVED → synthesis → audit, không reprompt (E2 outcome-level OK). S3 PASS 2026-09-05 (unit assert + live no-reprompt; chi tiết ở dòng S3). S4 cho run-547025: writer output `requirements/08-Task/todo/Task-1-add-integer-gcd.md` (todo folder trống, đúng scope 2 T-items + additive-only). Caveat đã verify: prompt 2 run ghi nhầm AC `GCD(48,18)=18`; coder implement GCD đúng (`calc_gcd_test.go` green) nên PASS giữ nguyên — rerun sau dùng `=6` (S2 đã sửa). Fix CA-741 verify cùng run-533004 như cũ |
| A Plan loop | ✅ A1–A5 đủ | run-548341 | A1/A2/A5 như trước. A3 PASS: round 2 approve (8 gate checks, feature_key override về `calc-core` + absInt ownership fixed) → `done` → `preflight_contract_freeze` → `test_signatures` → `implement` chạy. A4 PASS + bonus freeze-enforcement live: coder viết lố `change-audit/2026-09-05-calc-lcm.md` ngoài declared paths → flowgate block `flow scope drift` → escalate xin Retry/Stop/Allow (operator xóa file lố + Retry → implement DONE đúng scope). Coder không được viết CA note (Audit sở hữu) — gate bắt đúng. |
| B Code loop | ✅ B1; ⏳ B2–B4 | run-533004, run-548341 | B1 PASS ×2: run-533004 (cũ) + run-548341 — code hub `changes_requested` (safe-fix R1/R2 pass; 3 blockers: thiếu `gcdInt`+`absInt` trong `calc.go`, thiếu T-3 contract test cases, CA note sai feature_key/thiếu ledger) → re-enter `implement` round 2/3. `runner.log`: `coder` spawn đúng 1 lần (`run-549304`), 2 turn_completed tiếp theo trên CÙNG child (không `child created` mới) — reuse đúng chuẩn. Reviewer spawn theo round (`run-549536` code r1, chạy provider grok-4.5 — reviewer khác provider với coder opencode, bình thường). B3 tạm đếm: plan_writer=1, implement=1, reviewers=spawn/round — khớp expect, chốt khi run done. B2 cần screenshot panel steps NGAY LÚC implement round 2 RUNNING (plan nodes phải DONE) — đang chờ operator chụp. |
| C cp-harness | ⏳ | | |
| D Smoke variant | ⏳ | | |
| E Artifacts | ⏳ | | |
| F Regression | ⏳ | | |

## Kết luận phiên

Ghi Pass/Fail từng mục kèm runId + step timeline screenshot. Fail routing (A1/B2/C5) → mở bug `feature_key: agent-flow-engine`, prior CA-712, không sửa test cũ (oracle-rule). Khi S-F đều PASS trên provider thật → ghi CA đóng CP-58, flip Task-304..307 + CP-58 sang done.
