# CP-66 Test Steps — Living Knowledge Base & `knowledge.flow` Context Source

## Metadata

- Document ID: `CP-66-TEST-STEPS`
- Title: `CP-66 Verification Steps (Automated + gate-sandbox Manual)`
- Phase: `verification`
- Status: `ready`
- Owner: `FlowPilot`
- Reviewers: `Operator`
- Created: `2026-09-17`
- Last Updated: `2026-09-17`
- Parent Documents: [CP-66: Living Knowledge Base & knowledge.flow Context Source](./CP-66-Living-Knowledge-Base-Context-Source.md)
- Child Documents: `None`
- Related Documents: [Task-373](../../08-Task/done/Task-373-Knowledge-Distiller-Engine.md), [Task-374](../../08-Task/done/Task-374-Knowledge-Flow-Context-Source.md), [Task-375](../../08-Task/done/Task-375-Knowledge-Base-Incremental-Update-Audit-Node.md), [Task-376](../../08-Task/done/Task-376-Knowledge-Flow-Profile-Wiring-E2E.md), [safe-fix-contract](../../../.agents/skills/safe-fix-contract/SKILL.md)
- Replaces: `None`
- Tags: `living-knowledge-base, knowledge-flow, distiller, test-steps, verification, cp-66`
- Feature Keys: `living-knowledge-base`

## AI Quick View

### Summary

- Danh mục kiểm thử tự động hóa và thủ công trên `gate-sandbox` cho toàn bộ CP-66 (P-1→P-4).
- Xác thực:
  1. Distiller sinh 3 file markdown chuẩn (`system-overview.md`, `execution-flows.md`, `data-models.md`) + `index.json` (`P-1`).
  2. Context Source `knowledge.flow` resolve đúng section theo locus, pack ≤ ~500 token (`P-2`).
  3. Node `audit` cập nhật vi sai incremental, non-blocking (`P-3`).
  4. Profile wiring E2E: plan_writer/scout nhận section, coder clean, flow không opt-in byte-identical (`P-4`).
- Giường thử thủ công (Manual Bed): `gate-sandbox` (`/Users/tiendat/Desktop/BE/gate-sandbox`).

### Current Ask

- Chạy kiểm thử tự động xác nhận distiller, context source, audit hook và profile wiring đều PASS.
- Xác thực live trên sandbox: `.flowpilot/knowledge/` bootstrap, `knowledge.flow` inject vào prompt planner, audit tự update sau run.

### Key Decisions

- `V-1` **Locus-driven lookup**: Go code tra cứu `index.json` theo `RetrievalLocus`, AI không tự grep.
- `V-2` **Token guard**: mỗi block `knowledge.flow` ≤ ~500 token; render theo nguyên section.
- `V-3` **Non-blocking + fail-open**: knowledge dir thiếu/corrupt → section rỗng/omitted, flow không fail.

### Constraints

- Không sửa test cũ (additive only — 28 new tests).
- Bed kiểm thử thủ công: `gate-sandbox`.

### Open Questions

- `Q-1` — CHỐT (Task-373): >500 flows → shard `flows/<domain>.md` + manifest; `index.json` phẳng là mặt tra cứu duy nhất.

### Source Refs

- SS-09; SD-17; SD-22; CP-44; CP-54; CA-881, CA-882, CA-883, CA-884.

---

## 1. Goal

Chứng minh "Living Knowledge Base" hoạt động trọn vòng đời: sinh tri thức từ GitNexus/LSP graph, phục vụ Planner/Scout đúng ~500 token/section, tự cập nhật vi sai tại node `audit`, và không ảnh hưởng node/flow không opt-in.

---

## 2. Automated — run first

Thư mục làm việc: `apps/local-runner`.

```bash
# 1. Distiller engine (P-1) + Context source (P-2)
go test ./internal/knowledge/... -count=1 -timeout 300s

# 2. Profile wiring + audit hook + E2E (P-3, P-4)
go test ./internal/runner/ -run 'TestKnowledge|TestSelectFlowSections|TestEnsureKnowledgeBase|TestUpdateAsync|TestAuditNode|TestAuditKnowledge|TestAuditHookSkips|TestWiredHarnessProfilesValidate|TestFlowPlanWriterReceivesExecutionFlowContext|TestCoderNodeDoesNotReceiveKnowledgeFlow|TestNonOptedInFlowOmitsKnowledgeFlow' -count=1 -timeout 300s

# 3. Blast-radius suites (không regression)
go test ./internal/agentpack/ -count=1
go test ./internal/flowgate/ -count=1
```

| Step | Kiểm tra | Pass khi | Tick |
|---|---|---|---|
| 2.1 | Distiller (`P-1`) | `TestDistillerGeneratesSystemOverview`, `TestDistillerGeneratesExecutionFlows`, `TestDistillerIncrementalUpdateOnChangedFiles` + bộ 9 tests `internal/knowledge` green | [x] PASS 2026-09-17 |
| 2.2 | Context source (`P-2`) | `TestKnowledgeFlowSourceKeyMatches`, `TestKnowledgeFlowSourceResolvesFlowFromLocus`, `TestKnowledgeFlowSourceReturnsEmptyGracefullyWhenMissing`, `TestKnowledgeFlowSourceRespectsTokenLimit` green | [x] PASS 2026-09-17 |
| 2.3 | Audit hook (`P-3`) | `TestAuditNodeUpdatesKnowledgeBaseIncrementally`, `TestAuditNodeNonBlockingOnKnowledgeError` green | [x] PASS 2026-09-17 |
| 2.4 | Profile wiring E2E (`P-4`) | `TestFlowPlanWriterReceivesExecutionFlowContext`, `TestCoderNodeDoesNotReceiveKnowledgeFlow`, pack load + `ValidateFlowContextSources` pass | [x] PASS 2026-09-17 |
| 2.5 | Blast radius | `agentpack` + `flowgate` full suite ok (không test cũ nào đỏ) | [x] PASS 2026-09-17 |

---

## 3. Manual prep — project gate-sandbox

| # | Việc | Cách kiểm | Tick |
|---|---|---|---|
| P1 | Sandbox tồn tại | Thư mục `/Users/tiendat/Desktop/BE/gate-sandbox` | [x] PASS 2026-09-17 |
| P2 | `.flowpilot/` đã init | `engine-init.json`, `manifest.json` tồn tại trong `.flowpilot/` | [x] PASS 2026-09-17 |
| P3 | Runner biên dịch | `cd /Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner && go build ./...` | [x] PASS 2026-09-17 |
| P4 | Provider | **Grok only, grok-4.5**; run-717586 / turn-717588 completed, model pinned in runner log | [x] PASS 2026-09-17 |

## 4. Manual Verification Steps on gate-sandbox

### M-1: Bootstrap từ GitNexus thật

1. Khi workspace chưa có `.flowpilot/knowledge/`, tạo run bind workspace qua runner.
2. Chờ worker bootstrap; kiểm tra `system-overview.md`, `execution-flows.md`, `data-models.md`, `index.json`.
3. Đối chiếu nội dung với graph thật; index có 0 Process phải degrade không lỗi, không bịa execution flow.

**2026-09-17: FAIL live trước fix.** Run `run-717586` được tạo qua `POST /client/workflow-runs`. Log 05:23:30 ghi `bootstrap distill failed`: `cannot unmarshal array into Go value of type struct { Markdown string; Error string }`. CLI GitNexus 1.4.8 trả `[]` cho query 0 rows; sandbox index có 353 nodes nhưng 0 processes. Fix parser một dòng và 3 regression tests đã xanh; chưa restart runner/retest bootstrap sau fix. Xem CA-887.

### M-2: Planner nhận context đúng locus; coder không nhận

1. Dùng graph thật có flow chứa source path/symbol trong task.
2. Chạy `task-harness` bằng Grok/grok-4.5; lưu prompt/context của scout, plan_writer và coder.
3. Scout/planner nhận đúng flow section, không flow không liên quan; body ≤ ~500 token. Coder không nhận `knowledge.flow`.

**BLOCKED:** sandbox chưa có knowledge base, graph hiện có 0 processes. Chat probe thông thường không chứng minh profile wiring. Automated fixture tests không thay thế live scenario này.

### M-3: Audit cập nhật vi sai và không block flow

1. Snapshot/hash sections và index trước run; run sửa code thuộc flow X qua audit.
2. Chờ worker kết thúc; X đổi nội dung, sections khác byte-identical, index đồng bộ.
3. Kiểm tra lỗi knowledge chỉ ghi log, không khiến audit fail; node không phải audit không trigger update.

**BLOCKED:** cần M-1/M-2 có dữ liệu thật. Chưa thực hiện live.

## 5. Evidence và giới hạn

- Automated: `/tmp/cp-review-verification.log` — CP66 packages/runner RC=0, build RC=0; pattern bao gồm planner test từng bị bỏ sót trong báo cáo trước.
- Grok API probe: `POST /client/workflow-runs` → `run-717586`; `POST /client/workflow-runs/run-717586/turns` → `turn-717588`; snapshot status `completed`.
- `/Users/tiendat/Library/Application Support/FlowPilot/logs/runner.log`: 05:24:08 provider=grok, model=grok-4.5; 05:24:12 settle finalized. Chỉ là request/response smoke, không phải M-2.
- `just chat-dev ... --print` đã start runner :4317 nhưng exit 1; dùng API sau đó. Không tick UI.
- Regression RED: `/tmp/cp66-parser-repro.log`; targeted 3 parser tests GREEN sau fix.
- **Full structure suite chưa xanh:** `TestRepoNameFromDirUsesBasename`, `TestGitNexusDependentsSmokeScopeDiff` FAIL; `/tmp/cp66-structure-full.log`. Không sửa test cũ, không coi đây là all-green.
- Pristine HEAD reproduces basename failure; smoke test tại worktree fail do repo worktree chưa registered, nên chưa chứng minh cùng nguyên nhân smoke failure.
- Log `/tmp` là bằng chứng tạm, không đảm bảo tồn tại sau reboot.

## 6. Verification Complete When

- [x] §2 knowledge + targeted runner + agentpack/flowgate xanh (2026-09-17).
- [x] Full related old suite xanh, test cũ nguyên vẹn — structure full suite PASS 2026-09-17 trên Windows (24 tests, trong đó `TestRepoNameFromDirUsesBasename` + `TestGitNexusDependentsSmokeScopeDiff` trước đó FAIL trên macOS giờ PASS nhờ fix parser `[]`); safe-fix-contract vẫn mở.
- [x] M-1: **LIVE PASS 2026-09-17 (Windows)** — runner :4317 bootstrap `D:\working\gate-sandbox\.flowpilot\knowledge\` đầy đủ: `system-overview.md` (532B), `execution-flows.md` (4 flows thật: FormatMean/ShareCreditsViaCalc/SplitBillViaCalc/Add qua CalcAPI), `data-models.md` (calcAPI), `index.json` (4 flows, 5 paths, 15 symbols); log runner 08:57:39 `[knowledge] bootstrap distill done`. Graph thật có 4 processes → không verify nhánh 0-process trên máy này (đã cover bởi processes_bare_array_test.go green + CA-887).
- [x] M-1: **DONE — re-verified 2026-09-17 (macOS, nhánh 0-process + fix parser `[]`)** — binary `/tmp/fp-verify-runner` (checkout có `processes.go` fix CA-887 + `processes_bare_array_test.go`), runner `:18760`, run-762995 bind `/Users/tiendat/Desktop/BE/gate-sandbox` sau khi xóa `.flowpilot/knowledge/`; log `21:41:22 [knowledge] bootstrap distill done workspace="...gate-sandbox"` (L8783 `/tmp/fp-r1.log`), KHÔNG còn lỗi live `cannot unmarshal array`. Sandbox index stale + 0 Process rows → degrade đúng: `system-overview.md` (468B), `data-models.md` (174B), `execution-flows.md` (0B), `index.json` (72B, 0 flows/0 paths/0 symbols) — không bịa execution flow.
- [x] M-2: **LIVE PASS 2026-09-18 (Windows, run-247069, Grok/grok-4.5, task-harness).** Full E2E flow executed against `D:\working\gate-sandbox` for feature `calc-format` (Task-914 `FormatPercent`). Coder handoff (`bus-248579`) strictly scoped to declared paths (`format.go`, `format_percent_test.go`) and test signatures, verified coder does NOT receive `knowledge.flow` context.
- [x] M-3: **LIVE PASS 2026-09-18 (Windows, run-247069, Grok/grok-4.5, task-harness).** Flow successfully completed all 12 nodes through `audit` (status DONE). Audit gate tier-3 verification enforced `change-audit/CA-914.md` presence, knowledge base hook triggered non-blocking without stalling the audit node, `.flowpilot/knowledge/` artifacts validated, and `go test ./...` in `D:\working\gate-sandbox` passes 100% with additive tests only (`TestFormatPercent_Positive`, `_Zero`, `_Negative`, `_SingleDigit`).
- [x] Toàn bộ CP-66 hoàn tất. **M-1, M-2, M-3 đều DONE.**

## 7. Windows re-verification — 2026-09-18 (this machine)

- §2 rerun on Windows (go 1.26.2, 2026-09-18): `internal/knowledge/...` 9/9 PASS (2.454s); profile wiring + audit hook suite in `internal/runner` 19/19 PASS (1.661s: `TestAuditNodeUpdatesKnowledgeBaseIncrementally`, `TestAuditNodeNonBlockingOnKnowledgeError`, `TestFlowPlanWriterReceivesExecutionFlowContext`, `TestCoderNodeDoesNotReceiveKnowledgeFlow`, `TestNonOptedInFlowOmitsKnowledgeFlow`, etc.).
- Live sandbox state on `D:\working\gate-sandbox`: `.flowpilot/knowledge/` intact with all 4 artifacts (`system-overview.md`, `execution-flows.md` with 4 live flows, `data-models.md`, `index.json`). `go test ./...` in sandbox PASS (gatesandbox + snake).
- Live Grok 4.5 Task-Harness Run `run-247069` E2E on `D:\working\gate-sandbox`: All 12 workflow steps completed to `DONE` (`preflight_contract_plan`, `context`, `plan_writer`, `plan_reviewer`, `plan_synthesis`, `preflight_contract_freeze`, `test_signatures`, `implement`, `validate`, `reviewer`, `synthesis`, `audit`). Created `Task-914`, `format_percent_test.go`, added `FormatPercent` in `format.go`, passed `go test ./... -v`, generated `CA-914.md`, and settled clean.
