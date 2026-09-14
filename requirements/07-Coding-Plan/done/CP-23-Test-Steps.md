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
| P1 | Sandbox sẵn sàng | Thư mục `/Users/tiendat/Desktop/BE/gate-sandbox` sạch trạng thái git | [ ] |
| P2 | Engine khởi chạy | `cd apps/local-runner && just chat-dev /Users/tiendat/Desktop/BE/gate-sandbox` | [ ] |
| P3 | Kiểm tra log file | Đảm bảo runner phát log ra terminal hoặc file log | [ ] |

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
- [ ] Kịch bản 2: Drift Detector phát hiện vòng lặp; ở ≥80 run bị park + hỏi user thật (Task-348), vibe không hỏi. *(LIVE 2026-09-14: drift event ghi thật `drift_score=20, zero_delta_progress, action=none` vào `workflow_drift_events.json` — dưới ngưỡng, no-op đúng; chân ≥80 chưa induce được live)*
- [x] Kịch bản 3: **LIVE 2026-09-14**: skills có mặt trên sandbox (`.agents/skills/safe-fix-contract/SKILL.md` được pointer block tham chiếu; grok session load `.grok/skills/`).
