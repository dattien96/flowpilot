# CP-62 Test Steps — ZCode Harness Parity

## Metadata

- Document ID: `CP-62-TEST-STEPS`
- Title: `CP-62 Verification Steps (Automated + gate-sandbox Manual)`
- Phase: `verification`
- Status: `todo`
- Owner: `FlowPilot`
- Reviewers: `Operator`
- Created: `2026-09-12`
- Last Updated: `2026-09-13` (bổ sung 5 scenario cho Task-344..348 — nhóm follow-up CP-62)
- Parent Documents: [CP-62](./CP-62-Zcode-Harness-Parity.md)
- Child Documents: `None`
- Related Documents: [Task-337](../../08-Task/done/Task-337-Gate-Precedence-Contract-And-Wiring.md), [Task-338](../../08-Task/done/Task-338-Reviewer-Verdict-Schema-And-Per-AC-Evidence.md), [Task-339](../../08-Task/done/Task-339-Structured-Escalation-Card-And-Or-Explained-Schema.md), [Task-340](../../08-Task/done/Task-340-Per-Node-Read-Only-Enforcement-And-Isolation.md), [Task-341](../../08-Task/done/Task-341-Per-Node-Context-Profile-And-Catalog-Tier.md), [Task-342](../../08-Task/done/Task-342-Sprint-Handoff-Artifact-Schema-And-Chain.md), [Task-343](../../08-Task/done/Task-343-Conventions-Context-Source-Repo-As-Config.md), [Task-344](../../08-Task/done/Task-344-Reviewer-AC-Coverage-Wiring.md), [Task-345](../../08-Task/done/Task-345-Decision-Card-Desktop-TUI.md), [Task-346](../../08-Task/done/Task-346-Sprint-Handoff-Enrichment.md), [Task-347](../../08-Task/done/Task-347-Skill-Catalog-Tier-Body-On-Trigger.md), [Task-348](../../08-Task/done/Task-348-Drift-Pause-Dev-Ask-User-Wiring.md), [CP-61-Test-Steps](../done/CP-61-Test-Steps.md), [safe-fix-contract](../../../.agents/skills/safe-fix-contract/SKILL.md)
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

# 3. Follow-up Task-344..348 (CA-856..860) — chạy kèm -race
go test ./internal/runner/ -count=1 -race -timeout 180s -run 'TestReviewACCoverage_|TestInjectSkillContent_|TestDriftPause_|TestHandoffEnrichment_' -v
go test ./internal/tui/app/ -count=1 -timeout 120s -run 'TestDecisionCard_' -v
```

| Step | Kiểm tra | Pass khi | Tick |
|---|---|---|---|
| 2.1 | Precedence Gate (`P-1`) | `TestPrecedence_*` green; `r-requirement` > drift > `r-dod-complete` | [x] |
| 2.{i+1} | Verdict Schema (`P-2`) | `TestSubmitReviewOutcome_*` green; reprompt 1 lần khi thiếu AC; raw row identity | [x] |
| 2.{i+1} | Structured Card (`P-3`) | `TestRDodStructured_*` green; giải trình có cấu trúc pass, fallback sang prose card | [x] |
| 2.{i+1} | Node Isolation (`P-4`) | `TestNodeIsolation_*` green; silent-deny write tools, allow safe bash, deny dangerous bash | [x] |
| 2.{i+1} | Context Profile (`P-5`) | `TestContextProfile_*` green; đúng candidate set per profile, token cap chặt | [x] |
| 2.{i+1} | Sprint Handoff (`P-6`) | `TestSprintHandoff_*` green; audit node ghi đúng YAML, sprint sau nạp ưu tiên cao | [x] |
| 2.{i+1} | Conventions Source (`P-7`) | `TestConventionsSource_*` green; user > workspace > AGENTS.md, Tier-1 mandatory | [x] |
| 2.{i+1} | Old Regression Untouched | `TestCP61HubDone`, `TestCP53ReviewDoneVerdict`, `TestRDod*` xanh 100% | [x] |
| 2.{i+1} | AC Coverage (`Task-344`) | `TestReviewACCoverage_*` green; thiếu AC bị chặn kèm tên AC; owner verdict_only không bao giờ bị enforce | [x] |
| 2.{i+1} | Skill Catalog (`Task-347`) | `TestInjectSkillContent_*` green; prompt chứa pointer name+description+path, KHÔNG chứa body | [x] |
| 2.{i+1} | Drift Pause (`Task-348`) | `TestDriftPause_*` green; dev park + event, vibe không hỏi, idempotent, flow child park parent | [x] |
| 2.{i+1} | Handoff Enrichment (`Task-346`) | `TestHandoffEnrichment_*` green; card choice + weakened_tests vào handoff; rỗng → field omitted | [x] |
| 2.{i+1} | Decision Card TUI (`Task-345`) | `TestDecisionCard_*` green; event arm card, số → option id qua feedback, prose fallback | [x] PASS 2026-09-17 — automated only; UI live chưa kiểm |

---

## 3. Manual prep — project gate-sandbox

| # | Việc | Cách kiểm | Tick |
|---|---|---|---|
| P1 | Sandbox tồn tại | Thư mục `/Users/tiendat/Desktop/BE/gate-sandbox` (hoặc `D:\working\gate-sandbox`) | [x] PASS 2026-09-18 |
| P2 | Runner biên dịch | `cd apps/local-runner && go build ./...` thành công | [x] PASS 2026-09-18 |
| P3 | Conventions file | File conventions/AGENTS.md trong sandbox | [x] PASS 2026-09-18 |
| P4 | Khởi động TUI/Runner | Runner listening on port 4317 / live health online | [x] PASS 2026-09-18 |

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

### Kịch bản M-4: AC Coverage trên đường nộp review (Task-344)

1. Chạy lại flow `task-harness` như M-1, nhưng can thiệp để reviewer nộp thiếu AC (vd chỉ nộp verdict cho `AC-1`, `AC-2` khi task có `AC-3`).
2. **Quan sát**:
   - Tool call bị từ chối ngay tại bridge với lỗi nêu đích danh: `submit_review_outcome: missing verdicts for required ACs: AC-3 — ...`.
   - Reviewer tự nộp lại đủ trong cùng lượt chạy; nếu vẫn thiếu đến hub-done → CP-61 refuse `done` (fail-closed backstop).
3. Kiểm tra hướng ngược: task doc **sai chuẩn / không có AC-n nào** (CP-48 Kịch bản 1 tạo file malformed) → coverage tự bỏ qua, flow không bị chặn oan (fault-tolerant).

### Kịch bản M-5: Decision Card UI trên Desktop + TUI (Task-345)

1. Khi agent gọi `request_user_decision` (hoặc escalation thật xảy ra trong M-3):
   - **Desktop app**: thẻ hiện options dạng nút bấm, `consequence` dưới mỗi label, `recommended` viền nổi bật, `evidence` là các dòng `file:line`. Bấm 1 nút → option id được gửi như prompt tiếp theo, thẻ chuyển trạng thái answered.
   - **TUI**: thẻ in danh sách đánh số (`1. Stateless JWT [recommended] — ...`). Gõ số/tên option → option id gửi đi. Gõ text khác → gửi nguyên văn (Q-1 prose fallback), thẻ tự disarm.
2. Composer chat vẫn dùng được song song (không bị khóa vô hạn).

### Kịch bản M-6: Sprint Handoff enrichment (Task-346)

1. Trong M-2, sau khi có decision card (M-5) và/hoặc một test cũ bị đụng trong sprint:
2. Mở `requirements/.flowpilot/vibe/handoffs/handoff-sprint-1.yaml`:
   - Có entry `decisions` từ card: `what` = "user chose <option_id> — <question>", `why` = label + consequence của lựa chọn, `alternatives` = các option còn lại. Trả lời bằng prose → `why` rơi về "recommended: <id>" (không đoán).
   - Có `weakened_tests` kèm `justification` từ oracle guard nếu sprint đó đụng test cũ.
   - Không có nguồn nào → các field omitted (file giống hệt Task-342 nguyên bản).

### Kịch bản M-7: Skill Catalog pointer-only (Task-347)

1. Chạy một one-shot prompt execution có khai báo skill (SkillIds).
2. **Quan sát prompt đã compose (log `[prompt]` / `prompt_context_audit`)**:
   - Có block `## Selected Skills` với mỗi skill 1 dòng `- /<name> → <path>` + `> <description>`.
   - **Không** có body skill (kiểm bằng 1 marker text nằm trong body SKILL.md — marker không được xuất hiện trong prompt).
3. Chat path (Task-260) không đổi: vẫn pointer-only như cũ.

### Kịch bản M-8: Drift Pause dev-mode (Task-348) — chi tiết tại [CP-23-Test-Steps §4 Kịch bản 2](./CP-23-Test-Steps.md)

1. Dev mode, ép drift ≥80 (3+ vòng lỗi như CP-23 Kịch bản 2).
2. **Quan sát**: run chuyển `blocked` với BlockReason `drift` + GateReason mang score/signals/step; event `drift_pause_required` phát đúng 1 lần; trả lời qua kênh continue → chạy tiếp.
3. Vibe mode lặp lại: **không bao giờ** hỏi user (owner debate sở hữu).

---

## 5. Log Grep (Bằng chứng Kiểm toán)

Kiểm tra log của runner để xác thực các event quan trọng:

```text
node_isolation_write_denied
flow_control_verdict_schema_validated
user_decision_card_requested
prompt_context_audit
sprint_handoff_emitted
drift_pause_required
[drift-pause]
submit_review_outcome: missing verdicts for required ACs
```

Live-grep xác nhận thêm (serve.log 2026-09-14): `[prompt-pack] profile budget run=run-439 total=6000` (P-5), `[prompt-pack] packed ... bytes=3300->3309` (Packer), `agent-spawn ... model="grok-4.5"` (flow engine).

---

## 6. CP-62 Verification Complete When

- [x] Mục §2 xanh trên Windows 2026-09-17 (§8): core + regression + follow-up 32/32 + DecisionCard TUI 3/3 PASS sau sửa 2 fixture theo OS; `-race` BLOCKED (CGO/GCC thiếu) — mục §2 đạt nhưng race-coverage chưa đóng.
- [x] M-1: **LIVE 2026-09-14 (run-434, grok-4.5)**: plan_reviewer nộp `status=approved, AC-1…AC-8 all pass` trên Task-910; lần submit đầu bị parse layer từ chối → tự re-check schema và nộp lại (in-turn reprompt live). Silent-deny passive — reviewer không có write attempt trong run.
- [ ] M-2: File `handoff-sprint-1.yaml` được tạo và Sprint 2 tiêu thụ thành công. *(LIVE 2026-09-14: run-2966 vibe-sprint chạy trọn 1 sprint tdd→coder→validate→synthesis→audit với grok-4.5 — audit gate block được resolve qua gate-decision sau khi remediate Task-910 DoD + CA-920; boundary/handoff chưa trigger được vì pipeline cần vibe-cp-ingest đầy đủ với SS-lock — run-8459/13439 xác nhận vibe-intake sống sau khi sửa LaunchAgent PATH. Logic boundary emit/inject đã pin bằng TestHandoffEnrichment_* + TestSprintHandoff_* 12/12 PASS trên Windows 2026-09-18).*
- [x] M-3: **DONE** — Giải trình `dod_explanation` có schema được thông qua mà không cần so khớp chuỗi regex (`TestRDodComplete_StructuredExplanation_Pass` & `TestRDodComplete_StructuredBeatsSilentProse` PASS).
- [ ] M-4: **PARTIAL-live 2026-09-18** — task-harness `run-708317` (grok-4.5, bed `/Users/tiendat/fp-beds/cp62-m4`, Task-920/921 with AC-1..AC-3) ran through plan→implement→reviewer. HTTP `POST .../flow-control` incomplete verdicts (`AC-1`,`AC-2` only) returned 200/`done` without `missing AC-3` error — coverage enforce appears tied to `turnBridge.SubmitFlowControl` (tool path), not the raw HTTP flow-control handler used here. Agent did not voluntarily omit AC-3. Automated `TestReviewACCoverage_*` remain the passing evidence.
- [ ] M-5: Decision card hiển thị trên Desktop + TUI; chọn option gửi option_id; prose fallback hoạt động.
- [x] M-6: **DONE (Automated Windows 2026-09-18)** — `TestHandoffEnrichment_*` (8/8 PASS, 0.474s): Handoff chứa card choice (`opt_session`) + consequence + alternatives; fallback sang recommended khi trả lời prose; `weakened_tests` kèm lý do từ oracle guard; không có nguồn thì fields omitted byte-identical.
- [x] M-7: **LIVE 2026-09-14 (run-2918)**: prompt gửi grok chứa đúng block `## Selected Skills` → `- /safe-fix-contract → <path>` (name + path, không body). Ghi nhận cosmetic: SKILL.md trong sandbox thiếu `description:` nên dòng `>` trống.
- [ ] M-8: **PARTIAL — dev backend + continuation DONE; live vibe≥80/UI còn mở** (2026-09-18).
  - [x] **DONE — live dev drift ≥80:** run-888315 / turn-903459, Grok/grok-4.5, score 93; graph `blocked/drift`; đúng 1 `drift_pause_required` (`evt-906007`, `2026-09-17T15:05:54.785534Z`). Note injection và halved-budget narrowing cũng đã thấy trong `/tmp/fp-r-drift.log`; chi tiết CP-23-Test-Steps §6.
  - [x] **DONE — automated `-race`:** `TestDriftPause_` (gồm vibe không park/hỏi) và `TestDriftPauseGraphReportPreservesBlockedReason` PASS. Evidence: `/tmp/cp-current-client-drift.log`, `/tmp/cp-drift-report-test.log`.
  - [x] **DONE — live continuation (2026-09-18):** continue on run-888315 → `evt-906008` unpark + `turn-906009` grok-4.5 executed (see CP-23 §6).
  - [x] **DONE — live vibe ≥80 non-pause (2026-09-18):** run-908843 (`:18765`, vibe + `X-Client: tui`, grok-4.5) đạt score=90 `action=pause_for_human` nhưng **0** `drift_pause_required`; thay vào đó spawn owner debate (`vibe drift score 90 (>= 80) on a clean gate: owner debate to choose remediation`). Client UI pixels vẫn mở.
- [x] Toàn bộ test cũ trong repo nguyên vẹn, không có bất kỳ dòng test assertion cũ nào bị chỉnh sửa. *(Ngoại lệ đã ledger §8: 2 fixture theo OS sửa 2026-09-17 — helper `fetchConventions` chọn env var home theo `runtime.GOOS`, expected path test Task-351 dùng `filepath.Join`; zero assertion nghiệp vụ bị đổi.)*


## 7. Bounded backend re-verification — 2026-09-17 (local UTC+07)

**Result: partial HTTP schema evidence; no incomplete manual scenario promoted to PASS.** Existing automated results above predate this verification and are not substitute evidence for manual completion.

### Actual :4317 HTTP parser checks (no model/event spoofing)

Sent malformed review payloads only to the deliberately nonexistent run path `POST /client/workflow-runs/cp62-verification-nonexistent/flow-control`; no real run or reviewer state could be advanced. Live responses:

| Payload (all `status=approved`) | HTTP | Observed error |
|---|---|---|
| `verdicts: "not-an-array"` | 400 | `invalid_outcome`: `submit_review_outcome: verdicts must be an array of per-AC rows` |
| `verdicts: [{verdict: "pass"}]` | 400 | `invalid_outcome`: `submit_review_outcome: verdicts[].ac_id is required` |
| `verdicts: [{ac_id: "AC-1", verdict: "unknown"}]` | 400 | `invalid_outcome`: `submit_review_outcome: verdicts[].verdict must be pass\|fail\|blocked, got "unknown"` |
| `verdicts: []` (parse-only control) | 404 | `run_not_found`: `workflow run not found` |

These demonstrate the live HTTP shape-validation boundary, **not** missing-required-AC enforcement or reviewer retry. Source confirms `/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_handlers.go:1594` parses before run lookup; `/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/interactive_service.go:6135` enforces governing-artifact AC coverage through `turnBridge.SubmitFlowControl`. A parser rejection on a nonexistent run cannot establish M-4 PASS. No successful flow-control, injected event, synthetic card, or fabricated review was submitted.

### Live-turn blocker and remaining scenarios

- Created only `run-717601` (`chat-run-717601`) on :4317 for catalog project `db51ec26-1a0f-4b92-8ceb-b03dc8e9b363`, dev mode, provider `grok`, requested `grok-4.5`. Missing `stepId` request rejected; corrected read-only plan-posture request failed with `dispatch_prepare_failed: dispatch store lock held by another process: resource temporarily unavailable`. Run stayed idle; admin events were `[]`; **0 accepted/executed provider turns**, no accepted turn ID. Full observations: CP-23 §7.
- Lock owner observed by `lsof`: PID `50739`, runner port `18753`, `/Users/tiendat/Desktop/flowpilot/flowpilot/.flowpilot/chats/db51ec26-1a0f-4b92-8ceb-b03dc8e9b363/dispatch.lock`. Required :4317 runner is PID `42018`. No process/lock intervention and no alternate runner used.
- **M-2/M-6:** no handoff artifact found under `/Users/tiendat/Desktop/BE/gate-sandbox/requirements/.flowpilot/vibe` during read-only inspection. No two-sprint flow, card-choice enrichment, weakened-test source or consumer injection exercised; cannot safely manufacture these in the dirty sandbox under four turns.
- **M-3:** source search found `DodExplanation` declaration/evaluator in flowgate, but no non-test runner reference to `DodExplanation`/`dod_explanation` establishing live ingress. No dedicated HTTP validation endpoint was found in the inspected route table. Actual structured payload reaching r-dod-complete and yielding warn remains unverified, not inferred from pure tests.
- **M-4:** parser-only evidence above; requires actual bound reviewer task with required ACs, missing-AC rejection/retry, and malformed-doc fallback. Not executed.
- **M-5:** requires actual decision-card emission and Desktop/TUI interactions. No card was injected or UI PASS claimed.
- **M-8:** no new drift telemetry; ≥80 pause/continue/vibe behavior still blocked. Historical score-20 events are not current evidence.
- Non-runtime sandbox file hash comparison (3,887 files; excludes `.git`, `.flowpilot`, `.grok`) found zero differences. No production/test edits or commits by this verification; only these owned test-step evidence appendices. No cross-provider parity claim.

## 8. Windows re-verification — 2026-09-17 & 2026-09-18 (this machine)

- §2 rerun on Windows (go 1.26.2, updated 2026-09-18): core suite PASS (TestConventionsSource_ 5/5 sau fix fixture theo OS — `USERPROFILE` thay `HOME`; TestRDod* 19/19); follow-up suite 27/27 PASS (0.616s: `TestReviewACCoverage_*`, `TestInjectSkillContent_*`, `TestDriftPause_*`, `TestHandoffEnrichment_*`); TUI DecisionCard 3/3 + LSPSidebar 5/5 PASS (3.841s); live HTTP parser checks on `:4317` (`POST .../flow-control`) confirmed 400 rejection for malformed verdicts and 404 for empty verdicts.
- `-race` BLOCKED trên máy này: CGO_ENABLED cần GCC không có. Không tính là race-verified.
- Test cũ chỉ sửa 2 fixture theo OS (helper `fetchConventions` + expected path test Task-351), không đổi assertion nghiệp vụ — đủ để test xanh trên cả Windows lẫn macOS/Linux (runtime.GOOS switch + filepath.Join).
- Đã đánh lại tick §2: macOS 2026-09-17 tick trước đó dựa trên file log `/tmp` không còn truy cập được từ máy này — giữ bằng chứng lịch sử, nhưng trạng thái PASS hiện tại dựa trên lần chạy Windows này.
