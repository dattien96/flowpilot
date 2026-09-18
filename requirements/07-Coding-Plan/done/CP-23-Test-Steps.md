# CP-23 Test Steps — Runtime Intelligence (Budget Packer, Drift Detector & Auto-Skill)

## Metadata

- Document ID: `CP-23-TEST-STEPS`
- Title: `CP-23 Verification Steps (Automated + gate-sandbox Manual)`
- Phase: `verification`
- Status: `ready`
- Owner: `FlowPilot`
- Reviewers: `Operator`
- Created: `2026-09-12`
- Last Updated: `2026-09-13` (bổ sung chân pause 80+ đã wire thật — Task-348)
- Parent Documents: [CP-23: Auto-Learn-To-Skill](./CP-23-Auto-Learn-To-Skill.md)
- Child Documents: `None`
- Related Documents: [Task-334: Budget Packer](../../08-Task/done/Task-334-Context-Resolver-And-Budget-Packer.md), [Task-335: Drift Detector](../../08-Task/done/Task-335-Drift-Wrong-Way-Detector-And-Correction-Ladder.md), [Task-336: Mistake to Skill](../../08-Task/done/Task-336-Mistake-To-Skill-Promotion-And-Skillpack-Sync.md), [safe-fix-contract](../../../.agents/skills/safe-fix-contract/SKILL.md)
- Replaces: `None`
- Tags: `runtime-intelligence, context-budget, drift-detector, auto-skill, test-steps, verification, cp-23`
- Feature Keys: `runtime-intelligence, context-budget, drift-detector, auto-skill`

## AI Quick View

### Summary

- Quy trình kiểm thử nghiệm thu toàn diện cho [CP-23](./CP-23-Auto-Learn-To-Skill.md) bao gồm cả tự động hóa và thủ công trên `gate-sandbox`.
- Bao phủ 3 hệ thống trí tuệ vận hành cốt lõi:
  1. **Phase 1 (Task-334)**: Budget Packer cắt tỉa và nén ngữ cảnh theo hạn mức token, bảo vệ Tier-1, ghi nhận audit.
  2. **Phase 2 (Task-335)**: Drift & Wrong-Way Detector phát hiện vòng lặp xin lỗi, vi phạm scope lặp lại, kích hoạt thang ứng phó (Correction Ladder: Hint $\rightarrow$ Context Reduction $\rightarrow$ Human Pause).
  3. **Phase 3 (Task-336)**: Thăng cấp bài học thành Skill (Promotion Ladder) và đồng bộ hệ thống phân nhóm `skillpack` trên các nền tảng (common, android, kmm, golang...).

### Current Ask

- Chạy kiểm thử tự động xác nhận toàn bộ test suite của `promptpacker`, `driftdetect`, `skillpack` đều PASS.
- Thực hiện xác minh thủ công kịch bản nén token và phát hiện drift trên `gate-sandbox`.

### Key Decisions

- `V-1` **Thứ tự ưu tiên cắt tỉa Budget Packer**: Cắt tỉa từ section có độ ưu tiên thấp nhất (raw excerpts), bảo toàn tuyệt đối mục Contract (Tier 1).
- `V-2` **Drift Telemetry 2 tầng**: Tầng 1 deterministic (0-token) phát hiện lặp lại test fail $\ge 2$ lần hoặc vi phạm scope; chỉ escalate khi đạt ngưỡng.
- `V-3` **Cài đặt Skillpack an toàn**: `skillpack.Install` ghi đúng các thư mục theo nền tảng, hỗ trợ cả Claude, Codex, Grok.

### Constraints

- Không sửa test cũ của codebase.
- Bed kiểm thử thủ công: `gate-sandbox` (`/Users/tiendat/Desktop/BE/gate-sandbox` hoặc `D:\working\gate-sandbox`).

---

## 1. Goal

Xác minh tính đúng đắn và hiệu quả của Bộ trí tuệ vận hành runtime CP-23: Đảm bảo prompt không bao giờ vượt ngưỡng token gây tràn cửa sổ, các hành vi lạc lối (drift/loop) được chấn chỉnh kịp thời, và tri thức kỹ năng được đồng bộ chính xác.

---

## 2. Automated — run first

Thư mục làm việc: `apps/local-runner`.

```bash
# Chạy toàn bộ test suites của 3 module thuộc CP-23
go test ./internal/promptpacker ./internal/driftdetect ./internal/skillpack -count=1 -v

# Task-348 (CA-859): chân pause 80+ đã wire — runner-side tests
go test ./internal/runner/ -count=1 -race -run 'TestDriftPause_' -v
```

| Step | Gói kiểm thử | Kịch bản kiểm tra | Pass khi | Tick |
|---|---|---|---|---|
| 2.1 | `promptpacker` | `TestBudgetPacker_WithinBudget_RetainsAll` | Giữ nguyên toàn bộ sections khi prompt nhỏ hơn budget | [x] PASS 0.00s |
| 2.2 | `promptpacker` | `TestBudgetPacker_OverBudget_PrunesLowestPriority` | Cắt tỉa section độ ưu tiên thấp nhất khi vượt ngân sách | [x] PASS 0.00s |
| 2.3 | `promptpacker` | `TestContextDeduplicator_RemovesDuplicateText` | Loại bỏ các đoạn văn bản trùng lặp | [x] PASS 0.00s |
| 2.4 | `promptpacker` | `TestPromptContextAudit_RecordsDroppedItems` | Ghi log kiểm toán các mục bị cắt bỏ | [x] PASS 0.00s |
| 2.5 | `promptpacker` | `TestBudgetPacker_MandatorySectionExceedsBudget_Retained` | Section bắt buộc (Tier 1) không bao giờ bị cắt | [x] PASS 0.00s |
| 2.6 | `driftdetect` | `TestDriftDetector_HealthyTurn_NoDrift` | Turn bình thường có drift score = 0 | [x] PASS 0.00s |
| 2.7 | `driftdetect` | `TestDriftDetector_ApologyLoop_InjectsNote` | Vòng lặp xin lỗi kích hoạt chèn ghi chú hệ thống (System Note) | [x] PASS 0.00s |
| 2.8 | `driftdetect` | `TestDriftDetector_ScopeViolationAndTestLoop_EscalatesLadder` | Vi phạm scope + lặp lỗi test leo thang lên nấc thu hẹp context | [x] PASS 0.00s |
| 2.9 | `driftdetect` | `TestDriftDetector_CriticalDrift_PausesForHuman` | Điểm drift $\ge 80$ kích hoạt tạm dừng chờ người dùng | [x] PASS 0.00s |
| 2.10 | `driftdetect` | `TestDriftDetector_SuccessfulCorrection_ScoreDecays` | Sau khi sửa lỗi thành công, điểm drift tự động giảm dần | [x] PASS 0.00s |
| 2.11 | `skillpack` | `TestInstall_CommonOnlyForNonePlatform` | Nền tảng None chỉ cài đặt nhóm Common | [x] PASS 0.03s |
| 2.12 | `skillpack` | `TestInstall_AndroidIncludesCommonAndAndroid` | Nền tảng Android cài Common + Android | [x] PASS 0.06s |
| 2.13 | `skillpack` | `TestInstall_WritesGrokSkillsRoot` | Cài đặt đúng đường dẫn kỹ năng cho Grok | [x] PASS 0.01s |
| 2.14 | `skillpack` | `TestProviderStatuses_IncludesGrokWithoutAlteringOthers` | Quản lý trạng thái cài đặt trên cả Claude, Codex, Grok | [x] PASS 0.00s |
| 2.15 | `runner` (Task-348) | `TestDriftPause_DevModeParksRunAndEmitsEvent` | Dev mode drift ≥80 → run bị park (BlockReason `drift`) + event `drift_pause_required` | [x] PASS -race |
| 2.16 | `runner` (Task-348) | `TestDriftPause_VibeModeNeverAsksUser` | Vibe mode không bao giờ park/hỏi user cho drift (owner debate sở hữu) | [x] PASS -race |
| 2.17 | `runner` (Task-348) | `TestDriftPause_IdempotentWhileParked` + `TestDriftPause_FlowChildParksParent` | Không duplicate event; flow child drift → park parent hub | [x] PASS -race |

---

## 3. Manual prep — project gate-sandbox

| # | Việc | Cách kiểm | Tick |
|---|---|---|---|
| P1 | Sandbox sẵn sàng | Thư mục `/Users/tiendat/Desktop/BE/gate-sandbox` hoặc `D:\working\gate-sandbox` sạch trạng thái git | [x] PASS 2026-09-18 |
| P2 | Engine khởi chạy | Runner listening on port 4317 / live health online | [x] PASS 2026-09-18 |
| P3 | Kiểm tra log file | Đảm bảo runner phát log ra terminal hoặc file log | [x] PASS 2026-09-18 |

---

## 4. Manual Verification Steps on gate-sandbox

### Kịch bản 1: Kiểm thử Budget Packer (Nén Context khi Prompt Quá Dài)

1. Mở phiên chat hoặc flow trên sandbox.
2. Đưa vào một prompt kèm yêu cầu đọc 5-10 file source code lớn trong sandbox.
3. **Quan sát Log**:
   - Tìm kiếm sự kiện `prompt_context_audit`.
   - Kiểm tra xem các file excerpts có bị cắt giảm (truncated) để vừa với ngân sách định mức hay không.
   - Xác nhận header, contract và nhiệm vụ chính vẫn được giữ nguyên vẹn.

### Kịch bản 2: Kiểm thử Drift Detector (Bắt Vòng Lặp và Leo Thang Ứng Phó)

1. Cố tình đưa agent vào tình huống lỗi test lặp đi lặp lại:
   - Viết một test case luôn fail trong `calc_test.go`.
   - Bắt model sửa code để pass mà không được sửa file test.
   - Khi model giải thích xin lỗi ("I apologize for the error..."), lặp lại prompt từ chối.
2. **Quan sát Hành vi**:
   - Lần 1: Drift score tăng nhẹ $\rightarrow$ Runner tự động chèn System Note cảnh báo vào prompt tiếp theo.
   - Lần 2: Model tiếp tục vòng lặp $\rightarrow$ Ngữ cảnh được thu hẹp, tập trung vào đoạn mã gây lỗi.
   - Lần 3+: Drift score $\ge 80 \rightarrow$ **(Task-348 đã wire thật)** run chuyển `blocked` với BlockReason `drift`, GateReason ghi rõ score + signals + step; event `drift_pause_required` phát đúng 1 lần; log `[drift-pause]`.
   - Trả lời qua kênh continue/feedback (giống gate card) $\rightarrow$ chạy tiếp; trông thấy blocked card trên Desktop/TUI kèm lý do.
   - Lặp lại ở **vibe mode**: KHÔNG bao giờ hỏi user — owner debate sở hữu drift (SS-18 AC-7).

### Kịch bản 3: Kiểm thử Cài đặt Skillpack Đa Nền Tảng

1. Trong thư mục sandbox, kiểm tra thư mục `.agents/skills/`.
2. Kiểm tra xem các kỹ năng dùng chung (như `safe-fix-contract`, `additive-tests-only`) đã được cài đặt đồng bộ chưa.

---

## 5. Log Grep (Bằng chứng Kiểm toán)

```text
prompt_context_audit
drift_score_calculated
drift_correction_ladder_action
skillpack_installed
drift_pause_required
[drift-pause]
```

---

## 6. CP-23 Verification Complete When

- [x] §2 Automated tests chạy xanh 100% (14/14 tests pass).
- [x] Kịch bản 1: **LIVE 2026-09-14**: `[prompt-pack] packed run=... selected_tokens=... bytes=95->104` trên mọi turn; profile budget `total=6000` áp cho node scout (run-439). Không có dropped item vì prompt nhỏ — cơ chế verify qua log.
- [ ] Kịch bản 2: **PARTIAL — dev ladder + continuation DONE; live vibe ≥80 còn mở** (2026-09-18).
  - [x] **DONE — live dev note injection:** run-888315, Grok/grok-4.5, bed `/Users/tiendat/fp-beds/drift`; score 40 → log `21:50:08 [drift] injected system note ... turn=turn-896786 bytes=595`.
  - [x] **DONE — live dev narrow context:** score 65 → log `21:54:32 [drift] narrow_context: packing with halved budget ... turn=turn-897367 total=4000`.
  - [x] **DONE — live dev ≥80 park/confirm event:** turn-903459 đạt score 93; graph `status=blocked`, `blockReason=drift`, reason `confirm to continue`; đúng 1 event `drift_pause_required`, id `evt-906007`, lúc `2026-09-17T15:05:54.785534Z`. Snapshot cấp run vẫn `running`; không phải bằng chứng UI/question modal. Evidence: `/tmp/cp-closeout-current/graph.json`, `/tmp/cp-closeout-current/drift-pause.json`, `/tmp/fp-r-drift.log`.
  - [x] **DONE — automated race checks:** `TestDriftPause_` và `TestDriftPauseGraphReportPreservesBlockedReason` PASS với `-race -count=1`; graph report dev/vibe được kiểm tra, không dùng để thay live vibe.
  - [x] **DONE — live continuation after drift park (2026-09-18):** `POST .../agent-loop/continue` on run-888315 (`:18765`) cleared `blocked/drift` → `running` (`evt-906008`); follow-up `turn-906009` accepted with `model=grok-4.5`, settled, then re-parked at score 100 with new `drift_pause_required` `evt-906019` (expected zero-delta ping). Proves continue channel resumes execution.
  - [x] **DONE — live vibe ≥80 non-pause (2026-09-18):** run-908843 on `:18765`, `workingMode=vibe` + `X-Client: tui`, bed `/Users/tiendat/fp-beds/vibe-drift2`, model grok-4.5. Ladder: turn-911112 score=45 `apology_loop+zero_delta_progress` → inject note; turn-914490 score=**90** `action=pause_for_human` but **no** `drift_pause_required` event (count=0), graph `status=running` with **no** `blockReason=drift`. Proves vibe never parks user for drift at ≥80. UI modal still unverified.
  - [ ] Hiển thị UI drift card trên Desktop/TUI chưa xác minh.
- [x] Kịch bản 3: **LIVE 2026-09-14**: skills có mặt trên sandbox (`.agents/skills/safe-fix-contract/SKILL.md` được pointer block tham chiếu; grok session load `.grok/skills/`).


## 7. Bounded backend re-verification — 2026-09-17 (local UTC+07)

**Result: BLOCKED / no new live drift PASS.** Existing §6 checkboxes are unchanged.

- Required runner: `http://localhost:4317`, PID `42018`; `/health` returned online (`startedAt=2026-09-16T22:19:13.87715Z`). Created dev normal-chat `run-717601`, `stepId=chat-run-717601`, provider `grok`, requested model `grok-4.5`, sandbox cwd `/Users/tiendat/Desktop/BE/gate-sandbox`.
- First turn request was rejected with `invalid_request: stepId is required`. Corrected request supplied that step ID, `chatPosture=plan`, `yoloMode=false`, and a bounded read-only source/test analysis prompt (no tests, edits, installs, subagents, or flow-control). It failed before provider dispatch: `dispatch_prepare_failed: dispatch store lock held by another process: resource temporarily unavailable`.
- No accepted turn ID; **0 provider turns executed** (maximum allowed 4). `GET /client/workflow-runs/run-717601` remained `status=idle`; `GET /admin/workflow-runs/run-717601/events` returned `[]`. `/Users/tiendat/Desktop/flowpilot/flowpilot/.flowpilot/cli-runner.log` at 05:43:32 and 05:43:40 only showed `[flow-ref-resolve] run "run-717601": bailing, no workflowID set on this run`; no drift evidence for this run.
- Lock diagnosis: `lsof` showed PID `50739` (runner on port `18753`) holding `/Users/tiendat/Desktop/flowpilot/flowpilot/.flowpilot/chats/db51ec26-1a0f-4b92-8ceb-b03dc8e9b363/dispatch.lock`. No process killed/restarted, no lock removal, no alternate project identity/port used to bypass it.
- Source inspection: `/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/driftdetect/detector.go` carries a running score; zero delta with >2000 tokens contributes +20, repeated test failure +30, apology loop +25, out-of-scope edit +35. Thresholds are 30–59 note, 60–79 narrow, ≥80 pause. Four qualifying read-only turns could reach 80, but this session observed none; feature-flag state and actual token telemetry remain unverified. No impossible failing tests were induced; the §4 legacy recipe was not executed.
- `/Users/tiendat/Desktop/BE/gate-sandbox/.flowpilot/workflow_drift_events.json` still contained only historical `run-16/turn-18` and `run-2918/turn-2920`, each score 20 / `zero_delta_progress` / `none`. These are not new verification evidence.
- Sandbox was already dirty. Before/after SHA-256 comparison of 3,887 files excluding `.git`, `.flowpilot`, `.grok` found **no differences**. Runtime stores were not manually edited. No production or test edits; no test-suite claims from this session.

**Remaining:** resolve dispatch ownership operationally before another authorized :4317 attempt; observe actual note injection/context narrowing, ≥80 blocked/drift + single event, continuation, and vibe non-pause. CP-23 scenario 2 and CP-62 M-8 remain incomplete; only Grok was requested, no Claude/Codex parity claimed.

## 8. Windows re-verification - 2026-09-17 (this machine)

- Automated CP-23 suites rerun on Windows (go 1.26.2): promptpacker + driftdetect + skillpack 35/35 PASS; TestDriftPause_ 5/5 PASS without -race. -race remains BLOCKED here (CGO/GCC unavailable), not a PASS claim.
- Kịch bản 2 (drift ≥80 live) remains UNVERIFIED on this machine: live drift event not induced; the earlier score-20 event is historical. Automated TestDriftPause_DevModeParksRunAndEmitsEvent (park at score=86, single event) is the current fail-safe evidence.
- Live turn smoke: flowpilot chat --print --provider grok on D:\working\gate-sandbox returned CP-SMOKE-OK, exit 0 (turn completed 2026-09-17 08:57:42, runner.log). No drift telemetry captured from that read-only turn (expected: score 0).
