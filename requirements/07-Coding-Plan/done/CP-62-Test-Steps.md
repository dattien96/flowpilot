# CP-62 Test Steps — ZCode Harness Parity

## Metadata

- Document ID: `CP-62-TEST-STEPS`
- Title: `CP-62 Verification Steps (Automated + gate-sandbox Manual)`
- Phase: `verification`
- Status: `todo`
- Owner: `FlowPilot`
- Reviewers: `Operator`
- Created: `2026-09-12`
- Last Updated: `2026-09-12`
- Parent Documents: [CP-62](./CP-62-Zcode-Harness-Parity.md)
- Child Documents: `None`
- Related Documents: [Task-337](../../08-Task/done/Task-337-Gate-Precedence-Contract-And-Wiring.md), [Task-338](../../08-Task/done/Task-338-Reviewer-Verdict-Schema-And-Per-AC-Evidence.md), [Task-339](../../08-Task/done/Task-339-Structured-Escalation-Card-And-Or-Explained-Schema.md), [Task-340](../../08-Task/done/Task-340-Per-Node-Read-Only-Enforcement-And-Isolation.md), [Task-341](../../08-Task/done/Task-341-Per-Node-Context-Profile-And-Catalog-Tier.md), [Task-342](../../08-Task/done/Task-342-Sprint-Handoff-Artifact-Schema-And-Chain.md), [Task-343](../../08-Task/done/Task-343-Conventions-Context-Source-Repo-As-Config.md), [CP-61-Test-Steps](../done/CP-61-Test-Steps.md), [safe-fix-contract](../../../.agents/skills/safe-fix-contract/SKILL.md)
- Replaces: `None`
- Tags: `zcode-parity, gate-schema, node-isolation, context-profile, sprint-handoff, test-steps, verification, cp-62`
- Feature Keys: `zcode-parity`

## AI Quick View

### Summary

- Danh mục kiểm thử tự động hóa và thủ công trên `gate-sandbox` cho toàn bộ 7 lát cắt của [CP-62](./CP-62-Zcode-Harness-Parity.md).
- Xác thực:
  1. Hợp đồng Precedence giữa các Gate (`P-1`).
  2. Verdict Reviewer có Schema và bằng chứng file:line per-AC (`P-2`).
  3. Escalation card có cấu trúc và giải trình `or-explained` bằng schema (`P-3`).
  4. Thực thi cô lập node đọc-chỉ qua silent-deny và bash classifier (`P-4`).
  5. Context profile theo node và catalog tier trên Budget Packer (`P-5`).
  6. Artifact `sprint_handoff.v1` truyền *why* giữa các sprint (`P-6`).
  7. Context source `conventions` nạp tĩnh qua SD-22 registry (`P-7`).
- Giường thử thủ công (Manual Bed): `gate-sandbox` (`/Users/tiendat/Desktop/BE/gate-sandbox` hoặc `D:\working\gate-sandbox`).

### Current Ask

- Thực hiện kiểm thử sau khi các task từ `Task-337` đến `Task-343` hoàn thành mã nguồn.

### Key Decisions

- `V-1` **Schema-first validation**: Mọi tool payload phải tuân thủ đúng JSON schema, reprompt đúng 1 lần nếu thiếu hoặc sai định dạng, fail-closed sang escalate nếu lặp lại lỗi.
- `V-2` **Node isolation silent-deny**: Reviewer và Owner cố gắng ghi file phải bị chặn vật lý tại adapter; audit log phải ghi nhận sự kiện `node_isolation_write_denied`.
- `V-3` **Non-regression Dev Mode**: Chạy full test suite của `flowgate` và `runner`, đảm bảo dev mode không bị ảnh hưởng hành vi.

### Constraints

- `feature_key: zcode-parity`.
- R1: Không sửa test cũ.
- R2: Ma trận kiểm thử hỗ trợ cả 3 provider Claude, Codex, Grok.

### Open Questions

- Không còn câu hỏi mở (`Q-1`..`Q-3` đã resolved tại CP-62).

### Source Refs

- CP-62 §4 `P-1`..`P-7`, §7 Validation Plan, §10 Definition of Done.

---

## 1. Goal

Chứng minh harness của FlowPilot sau khi nâng cấp từ bài học ZCode đã chuyển hóa thành công từ "lời dặn model" sang "cưỡng chế vật lý tại runner", có cấu trúc dữ liệu máy đọc được, cô lập an toàn và tiết kiệm token.

---

## 2. Automated — run first

Thư mục làm việc: `apps/local-runner`.

```bash
# 1. Chạy toàn bộ test suites mới cho CP-62
go test ./internal/flowgate/ ./internal/runner/ -count=1 -timeout 180s -run 'TestPrecedence_|TestSubmitReviewOutcome_|TestRDodStructured_|TestRequestUserDecision_|TestNodeIsolation_|TestContextProfile_|TestSprintHandoff_|TestConventionsSource_' -v

# 2. Chạy regression suite cũ (bắt buộc xanh 100% và untouched)
go test ./internal/flowgate/ ./internal/runner/ ./internal/promptpacker/ -count=1 -timeout 180s -run 'TestCP61HubDone|TestCP53ReviewDoneVerdict|TestRDod|TestBudgetPacker' -v
```

| Step | Kiểm tra | Pass khi | Tick |
|---|---|---|---|
| 2.1 | Precedence Gate (`P-1`) | `TestPrecedence_*` green; `r-requirement` > drift > `r-dod-complete` | [ ] |
| 2.2 | Verdict Schema (`P-2`) | `TestSubmitReviewOutcome_*` green; reprompt 1 lần khi thiếu AC; raw row identity | [ ] |
| 2.3 | Structured Card (`P-3`) | `TestRDodStructured_*` green; giải trình có cấu trúc pass, fallback sang prose card | [ ] |
| 2.4 | Node Isolation (`P-4`) | `TestNodeIsolation_*` green; silent-deny write tools, allow safe bash, deny dangerous bash | [ ] |
| 2.5 | Context Profile (`P-5`) | `TestContextProfile_*` green; đúng candidate set per profile, token cap chặt | [ ] |
| 2.6 | Sprint Handoff (`P-6`) | `TestSprintHandoff_*` green; audit node ghi đúng YAML, sprint sau nạp ưu tiên cao | [ ] |
| 2.7 | Conventions Source (`P-7`) | `TestConventionsSource_*` green; user > workspace > AGENTS.md, Tier-1 mandatory | [ ] |
| 2.8 | Old Regression Untouched | `TestCP61HubDone`, `TestCP53ReviewDoneVerdict`, `TestRDod*` xanh 100% | [ ] |

---

## 3. Manual prep — project gate-sandbox

| # | Việc | Cách kiểm | Tick |
|---|---|---|---|
| P1 | Sandbox tồn tại | Thư mục `/Users/tiendat/Desktop/BE/gate-sandbox` (hoặc `D:\working\gate-sandbox`) | [ ] |
| P2 | Runner biên dịch | `cd apps/local-runner && go build ./...` thành công | [ ] |
| P3 | Conventions file | Tạo file `.flowpilot/conventions.md` mẫu trong sandbox | [ ] |
| P4 | Khởi động TUI/Runner | `just chat-dev /Users/tiendat/Desktop/BE/gate-sandbox` | [ ] |

---

## 4. Manual Verification Steps on gate-sandbox

### Kịch bản M-1: `task-harness` với Verdict Schema & Node Isolation

1. Khởi chạy flow: `/flow` $\rightarrow$ `task-harness`.
2. Prompt:
   ```text
   Implement a StringReverse function in stringutil package with feature_key: calc-core.
   AC-1: Reverse("hello") == "olleh"
   AC-2: Reverse("") == ""
   AC-3: Reverse unicode "xin chào" == "oàhc nix"
   ```
3. **Quan sát Reviewer**:
   - Reviewer gọi `submit_review_outcome` với mảng `verdicts` per-AC (`AC-1`, `AC-2`, `AC-3`) kèm `evidence: [{path, line, excerpt}]`.
   - Nếu reviewer cố tình dùng Bash chạy `rm` hoặc ghi file $\rightarrow$ Runner log `node_isolation_write_denied`.
   - Hub `synthesis` nhận raw verdict rows và đẩy về `implement` mà không bị viết lại văn xuôi.

### Kịch bản M-2: `vibe-sprint` với Context Profile & Sprint Handoff

1. Bật Vibe Mode: `/vibe` trên TUI.
2. Chạy 2 sprint liên tiếp tạo module tiện ích tính toán.
3. **Quan sát Sprint 1**:
   - Node `audit` ghi file `requirements/.flowpilot/vibe/handoffs/handoff-sprint-1.yaml`.
   - File chứa các quyết định kiến trúc (`decisions`), tiêu chí đã hoàn thành (`done`), và danh sách `weakened_tests` (nếu có).
4. **Quan sát Sprint 2**:
   - Prompt context của node `tdd` và `coder` ở Sprint 2 có section `sprint_handoff` được nạp từ Sprint 1.
   - Profile `coder` nhận source dependence; profile `reviewer` chỉ nhận diff + contract.

### Kịch bản M-3: Escalation Card & Or-Explained Schema

1. Cố tình để 1 task còn checkbox `[ ]` chưa tick trong Definition of Done.
2. Chuyển trạng thái task sang `done` kèm trường `dod_explanation: {explanation: "Deferred to sprint 2", referencing_ac: "AC-3"}`.
3. **Quan sát Gate**:
   - Gate `r-dod-complete` nhận diện cấu trúc giải trình và cho phép chuyển sang `warn`, không bị chặn (block) oan như regex cũ.

---

## 5. Log Grep (Bằng chứng Kiểm toán)

Kiểm tra log của runner để xác thực các event quan trọng:

```text
node_isolation_write_denied
flow_control_verdict_schema_validated
user_decision_card_requested
prompt_context_audit
sprint_handoff_emitted
```

---

## 6. CP-62 Verification Complete When

- [ ] Mục §2 Kiểm thử tự động chạy xanh 100%.
- [ ] M-1: Reviewer trả verdict per-AC có file:line; lệnh ghi của reviewer bị chặn (silent-deny).
- [ ] M-2: File `handoff-sprint-1.yaml` được tạo và Sprint 2 tiêu thụ thành công.
- [ ] M-3: Giải trình `dod_explanation` có schema được thông qua mà không cần so khớp chuỗi regex.
- [ ] Toàn bộ test cũ trong repo nguyên vẹn, không có bất kỳ dòng test assertion cũ nào bị chỉnh sửa.
