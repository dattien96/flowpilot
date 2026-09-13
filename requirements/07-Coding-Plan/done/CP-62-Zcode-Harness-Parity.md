# CP-62: Nâng cấp Harness học từ ZCode (Gate Schema, Node Isolation, Context Profile, Sprint Handoff)

## Metadata

- Document ID: `CP-62`
- Title: `Nâng cấp Harness học từ ZCode (Gate Schema, Node Isolation, Context Profile, Sprint Handoff)`
- Feature Keys: `zcode-parity, gate-schema, node-isolation, context-profile, sprint-handoff, gate-precedence`
- Phase: `coding_plan`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `Operator`
- Created: `2026-09-12`
- Last Updated: `2026-09-14` (bổ sung 9 task follow-up: Task-344..348 thực thi + Task-349..352 fix theo review)
- Parent Documents: [SD-19: Agent Flow Engine](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md), [SD-20: Flow Gate Rule Semantics](../../06-System-Tech-Design/SD-20-Flow-Gate-Rule-Semantics.md), [SD-10: Memory and Prompt Architecture](../../06-System-Tech-Design/SD-10-Memory-And-Prompt-Architecture.md), [SD-22: Pluggable Context Source Registry](../../06-System-Tech-Design/SD-22-Pluggable-Context-Source-Registry.md), [SS-08: Approve Gate](../../05-System-Specs/SS-08-Approve-Gate.md), [SS-13: AI-Followable Document Contract](../../05-System-Specs/SS-13-AI-Followable-Document-Contract.md), [SS-18: Vibe Working Mode](../../05-System-Specs/SS-18-Vibe-Working-Mode.md)
- Child Documents: [Task-337: Precedence Contract](../../08-Task/done/Task-337-Gate-Precedence-Contract-And-Wiring.md), [Task-338: Reviewer Verdict Schema](../../08-Task/done/Task-338-Reviewer-Verdict-Schema-And-Per-AC-Evidence.md), [Task-339: Structured Escalation Card](../../08-Task/done/Task-339-Structured-Escalation-Card-And-Or-Explained-Schema.md), [Task-340: Node Isolation](../../08-Task/done/Task-340-Per-Node-Read-Only-Enforcement-And-Isolation.md), [Task-341: Context Profile](../../08-Task/done/Task-341-Per-Node-Context-Profile-And-Catalog-Tier.md), [Task-342: Sprint Handoff Artifact](../../08-Task/done/Task-342-Sprint-Handoff-Artifact-Schema-And-Chain.md), [Task-343: Conventions Source](../../08-Task/done/Task-343-Conventions-Context-Source-Repo-As-Config.md), [Task-344: AC Coverage Wiring](../../08-Task/done/Task-344-Reviewer-AC-Coverage-Wiring.md), [Task-345: Decision Card Desktop TUI](../../08-Task/done/Task-345-Decision-Card-Desktop-TUI.md), [Task-346: Sprint Handoff Enrichment](../../08-Task/done/Task-346-Sprint-Handoff-Enrichment.md), [Task-347: Skill Catalog Tier](../../08-Task/done/Task-347-Skill-Catalog-Tier-Body-On-Trigger.md), [Task-348: Drift Pause Dev Ask User](../../08-Task/done/Task-348-Drift-Pause-Dev-Ask-User-Wiring.md), [Task-349: FlowNode Posture Gated Modes](../../08-Task/done/Task-349-FlowNode-Posture-Gated-Provider-Modes.md), [Task-350: Decision Card Desktop Channel](../../08-Task/done/Task-350-Decision-Card-Desktop-Channel-And-State-Resets.md), [Task-351: Handoff Standard Path And Precedence](../../08-Task/done/Task-351-Handoff-Standard-Path-And-Precedence-Unification.md), [Task-352: Review Hygiene Batch](../../08-Task/done/Task-352-Review-Hygiene-Batch.md)
- Related Documents: [CP-23: Auto-Learn-To-Skill](./CP-23-Auto-Learn-To-Skill.md), [CP-47: DOD Gate](./CP-47-DOD-Gate.md), [CP-48: Standardize Doc](./CP-48-Standardize-Doc.md), [CP-49: Reverse-Documentation](./CP-49-Reverse-Documentation-And-Doc-Ingestion.md), [CP-60: Vibe Working Mode](./CP-60-Vibe-Working-Mode.md), [SD-17: Context And Regression Engine](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md), [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md)
- Replaces: `None`
- Tags: `zcode-parity, gate-schema, node-isolation, context-profile, sprint-handoff, gate-precedence, flowgate, vibe-mode`

## AI Quick View

### Summary

- Chuẩn hóa 6 bài học kỹ thuật rút ra từ harness của ZCode thành 7 slice công việc (`P-1`..`P-7`), áp lên 4 flow chính (`task-harness`, `bug-plan-harness`, `vibe-ingest`, `vibe-cp-ingest` + `vibe-sprint`/`vibe-owner-debate`), không đổi engine domain-free (SD-19) — mọi thay đổi là data + wiring.
- **Pattern xuyên suốt — schema-first 4 tầng**: (T0) tự tính bằng code deterministic khi được, không hỏi model; (T1) enforce qua tool-call input schema ở transport layer; (T2) validate ở runner + đúng 1 reprompt kèm lỗi validation cụ thể; (T3) fail-closed (park/escalate, không tự diễn giải) — kèm escape hatch 1 field free-text. Áp cho verdict reviewer, card hỏi user, và giải trình or-explained.
- **Node isolation cấu trúc**: reviewer/plan_reviewer/owner/scout chạy read-only *bị chặn* (silent-deny), không chỉ được "dặn" — tái dùng engine `chat_posture_policy` (BUG-344) hiện có cho flow nodes; vá lỗ hổng `Bash` trong persona.
- **Context phân tầng trên Budget Packer (CP-23)**: per-node context profile + catalog tier (metadata rẻ luôn có, body nạp theo trigger) áp cho **cả hai gia đình flow** (harness + vibe, Q-2); sprint-handoff artifact có schema truyền *why* giữa các sprint; conventions dự án nạp từ file tĩnh qua SD-22 (pattern AGENTS.md).
- **Hợp đồng precedence giữa các hệ thống gate** (mới, phát sinh từ CP-23/47/48/49): r-requirement > drift > owner-debate > r-dod, chặn xung đột Budget Packer vs Drift Ladder vs debate cần full context.

### Current Ask

- Operator đã duyệt 2026-09-12, sau khi `Q-1`..`Q-3` resolved (bọc-quanh + fallback; profile cả hai gia đình; Bash classify cho reviewer, `verdict_only` cho owner) và Task-337..343 pass compliance review. Thực hiện theo thứ tự `P-1`..`P-7`, mỗi slice một PR độc lập, bắt đầu từ Task-337.

### Key Decisions

- `D-1` **Schema-first 4 tầng là pattern bắt buộc** cho mọi đầu ra của agent cần routing: T0 deterministic > T1 tool-call schema > T2 validate + 1 reprompt > T3 fail-closed. Không bao giờ parse văn xuôi để ra quyết định engine.
- `D-2` **Verdict reviewer là data có schema, per-AC, kèm evidence** (`P-2`): mỗi acceptance criterion 1 dòng verdict + dẫn chứng file:line; hub nhận raw verdict rows, không tự viết lại.
- `D-3` **Escalation card là tool call có cấu trúc** (`P-3`): `{question, options[{id,label,consequence}], recommended, evidence}`; or-explained của `r-dod-complete` chuyển từ text-match sang field có schema. Card **bọc quanh** đường prose hiện có (Q-1): turn gọi tool thành công → card render từ options; không gọi được → card prose hôm nay là fallback; deprecate prose sau khi ổn định.
- `D-4` **Read-only là enforcement, không phải lời dặn** (`P-4`): áp silent-deny posture (tái dùng BUG-344 engine) cho node loại review/owner/scout; reviewer **giữ `Bash` với classify per-command** (BUG-344 composition invariant), owner siết **`verdict_only`** không Bash (Q-3); khai báo trong FlowDefinition (data).
- `D-5` **Context profile theo node cho cả hai gia đình flow, đặt LÊN TRÊN Budget Packer** (`P-5`): profile chọn candidate set per node type — áp đồng thời harness (`task-harness`, `bug-plan-harness`, `cp-harness`) và vibe (`vibe-sprint`) trong cùng slice (Q-2); Budget Packer giữ nguyên làm engine phân bổ — không viết engine thứ hai (CP-23 `D-1` giữ nguyên thứ tự phụ thuộc).
- `D-6` **Sprint-handoff artifact có schema** (`P-6`): node `audit` (hub-inline, không phải coder) ghi `sprint_handoff.v1`; sprint kế tiêu thụ như context source ưu tiên cao; why được ghi lại, không bịa (Hard Ceiling Rule CP-49 áp vào runtime).
- `D-7` **Conventions dự án là context source tĩnh** (`P-7`): phân tầng user > workspace qua SD-22, inject như contract section trong Budget Packer — flow YAML không fork theo project.
- `D-8` **Precedence gate tường minh** (`P-1`): requirement-class luôn cao nhất (user-only, SS-18 `BR-4`); drift 80+ ở vibe đi qua `classifyVibeGate` thay vì dev card thô; drift thu hẹp context không áp cho turn owner-debate; Budget Packer không cắt section contract/verdict-evidence.

### Constraints

- Additive-only: không đổi ngữ nghĩa `r-ca`/`r-tests`/`r-scope`/`r-dod` hiện có; dev mode phải byte-for-byte không đổi (SS-18 `AC-8` style); engine không mang role string (SS-16 `BR-1`).
- Safe-fix contract: không sửa test cũ; test mới TDD-signatures-first, phủ use/edge/error (SS-04 §3.5.8).
- Tối đa deterministic: mọi tầng kiểm tra mới ưu tiên 0-token (pattern CP-23 tầng 1); LLM chỉ ở nơi thật sự cần phán đoán.
- Fail-closed tuyệt đối: payload sai schema → park/escalate rõ trạng thái (BUG-231/BUG-363), không bao giờ done im lặng.
- GitNexus impact analysis trước khi sửa symbol; HIGH/CRITICAL phải dừng báo operator.
- **Không copy từ ZCode** (FlowPilot đang mạnh hơn, tránh regress): (a) không chuyển gate sang trạng thái session-ephemeral; (b) không thêm todo-list protocol song song với flow engine; (c) không xây compaction engine cho single-long-context — node isolation đã tránh vấn đề gốc; (d) mistake-to-skill của CP-23 giữ nguyên ladder có human approval.

### Open Questions

- `Q-1` Resolved 2026-09-12 — `request_user_decision` **bọc quanh** card hiện có; giữ đường card prose làm fallback, deprecate sau khi tỉ lệ payload-schema ổn định (đo qua gate events).
- `Q-2` Resolved 2026-09-12 — profile áp cho **cả hai gia đình** (harness flows + vibe flows) trong cùng slice `P-5`, không phân kỳ.
- `Q-3` Resolved 2026-09-12 — reviewer **giữ `Bash`** với classify per-command (BUG-344 invariant) kèm regression test cho các lệnh ghi nguy hiểm; owner trong `vibe-owner-debate` là **`verdict_only`** (không Bash); audit silent-deny events — classifier lọt thì siết reviewer về bỏ-hẳn-Bash.

### Source Refs

- SD-19 `D-3`/`D-7` (flow là data, field vocabulary); SD-20 `D-1`..`D-7` (gate hook, `r-*` semantics); SS-18 `BR-1`/`BR-4`/`BR-5`, `AC-5`..`AC-8`; SS-14 `AC-6` (oracle); SS-13 (metadata/artifact contract); SS-08 §2-§3 (gate SSOT).
- CP-23 `D-1`..`D-5` (Budget Packer, drift ladder, skill ladder); CP-47 `D-3` (or-explained); CP-48 `P-2` (report phân cấp); CP-49 `P-2` (anti-hallucination citation); CP-60 (SS-Lock `user.confirm`).
- Code anchors: `apps/local-runner/internal/agentpack/flow-pack/tools/submit-review-outcome.yaml`; `agentpack/flow-pack/agents/reviewer.md` (frontmatter `tools:`); `runner/agent_catalog.go:25` (Tools parsed, chưa có consumer); `runner/chat_posture_policy.go:255-267` (flow runs excluded khỏi read-only posture); `runner/vibe_gate.go` (`classifyVibeGate`, `injectVibeSSDrift`); `runner/vibe_cp.go` (`onVibeCpNodeDone`, sprint chain); `runner/gate_hook.go` (or-explained path); `internal/flowgate/dod.go`.

---

## 1. Goal

Nâng cấp harness FlowPilot theo 6 bài học rút ra từ harness ZCode + 1 hợp đồng tích hợp mới phát sinh từ CP-23/47/48/49, biến các điểm hiện đang *phụ thuộc vào việc model tự tuân thủ* thành *cấu trúc bị chặn bởi runner*, và tách bạch context theo node để giảm token mà không mất ngữ cảnh cần thiết. Trọng tâm là 3 loại chuyển đổi:

1. **Prose → schema**: verdict reviewer, card hỏi user, giải trình or-explained (P-2, P-3).
2. **Khai báo → enforcement**: read-only node, context profile (P-4, P-5).
3. **Mờ đục → artifact có schema**: handoff giữa sprint, conventions dự án (P-6, P-7).

Kèm hợp đồng precedence (P-1) bảo đảm 4 hệ thống đã landing (Budget Packer, Drift Ladder, owner-debate, r-dod) không xung đột nhau trong edge case.

Ánh xạ từ danh sách bài học sang slice:

| Bài học (ZCode) | Follow-up CP-23/47/48/49 | Slice |
|---|---|---|
| (mới) Precedence giữa các gate | phát sinh từ CP-23/47/48/49 | `P-1` |
| Bài học 3: verdict schema + evidence | F-2: kéo pattern sang 3 bề mặt | `P-2`, `P-3` |
| Bài học 1: escalation card có cấu trúc | anchor mới: r-dod or-explained | `P-3` |
| Bài học 2: read-only cấu trúc | — | `P-4` |
| Bài học 4: progressive disclosure | F-1: 3 delta trên Budget Packer | `P-5` |
| Bài học 5: sprint-handoff artifact | — | `P-6` |
| Bài học 6: repo-as-config | — | `P-7` |

## 2. Input Documents

- [SD-19: Agent Flow Engine](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md) — FlowNode/FlowEdge/FlowPolicy vocabulary, flow-as-data.
- [SD-20: Flow Gate Rule Semantics](../../06-System-Tech-Design/SD-20-Flow-Gate-Rule-Semantics.md) — `r-*` rules, gate hook, dev cards.
- [SD-10: Memory and Prompt Architecture](../../06-System-Tech-Design/SD-10-Memory-And-Prompt-Architecture.md) — context sources, `artifact_memories`.
- [SD-22: Pluggable Context Source Registry](../../06-System-Tech-Design/SD-22-Pluggable-Context-Source-Registry.md) — registry cho context source mới.
- [SS-08: Approve Gate](../../05-System-Specs/SS-08-Approve-Gate.md), [SS-18: Vibe Working Mode](../../05-System-Specs/SS-18-Vibe-Working-Mode.md), [SS-13](../../05-System-Specs/SS-13-AI-Followable-Document-Contract.md), [SS-14](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md).
- [CP-23](./CP-23-Auto-Learn-To-Skill.md), [CP-47](./CP-47-DOD-Gate.md), [CP-48](./CP-48-Standardize-Doc.md), [CP-49](./CP-49-Reverse-Documentation-And-Doc-Ingestion.md), [CP-60](./CP-60-Vibe-Working-Mode.md).

## 3. Implementation Strategy

- **Overall approach**: mỗi slice là một P độc lập, ship và revert được riêng lẻ; chung một pattern kỹ thuật (schema-first 4 tầng, additive YAML/data-only, fail-closed). Không slice nào sửa engine domain-free của SD-19.
- **Sequencing logic**: `P-1` (hợp đồng precedence) là doc-contract + wiring nhẹ, chốt TRƯỚC vì ảnh hưởng routing của `P-2`/`P-3`. `P-2` (verdict schema) là nền shared-infra cho `P-3`. `P-4`/`P-5`/`P-6`/`P-7` độc lập, có thể song song hoặc dừng sau bất kỳ slice nào mà không phá các slice trước.
- **Dependencies**: `P-3` phụ thuộc pattern tool-schema của `P-2`; `P-5` phụ thuộc Budget Packer (CP-23 Task-334) đã landing; `P-6` phụ thuộc artifact framework (SD-23); `P-7` phụ thuộc SD-22 registry.

## 4. Work Breakdown

### `P-1` Hợp đồng precedence giữa các hệ thống gate (contract + wiring nhẹ)

- **Before (hiện trạng)**: 4 hệ thống cùng đụng vào prompt/loop nhưng chưa có hợp đồng ai thắng ai: (1) Budget Packer cắt context theo priority list global; (2) Drift Ladder (Task-335) thu hẹp context và `drift_score ≥ 80` → pause-for-human; (3) vibe owner-debate cần đầy đủ ngữ cảnh vi phạm để tranh luận; (4) `r-dod`/`r-requirement`/`r-scope` có thể bắn trên cùng một turn. Drift 80+ ở vibe mode đi resolver nào (dev card thô hay `classifyVibeGate`) chưa chốt tường minh trong SD-20/SS-18.
- **After (đích)**: bảng precedence tường minh, ghi vào SD-20 và pin bằng test thuần:
  1. Requirement-class rules (`r-requirement`, weaken→`r-requirement`) luôn cao nhất — user-only, không bao giờ owner-resolve (SS-18 `BR-4`).
  2. Drift `80+` ở `working_mode=vibe` route qua `classifyVibeGate` (owner debate nếu không phải requirement-class), không xuất dev card thô.
  3. Drift thu hẹp context KHÔNG áp dụng cho turn đang chạy owner-debate (debate cần full violation context); Budget Packer không cắt section contract/verdict-evidence.
  4. `r-dod-complete` (or-explained) đánh giá trước `r-requirement` trên cùng turn done.
  5. Tất cả settle-state tuân BUG-231/BUG-234: blocked-awaiting-user ≠ running ≠ failed.
- **Touched**: `internal/flowgate/evaluate.go` (thứ tự), `runner/vibe_gate.go`, `docs/` hoặc SD-20 sync, test mới.
- **Test pin**: unit table mode×gate×drift-tier → asserted route; dev non-regression (không Owner debate, không r-requirement — SS-18 `AC-8`).

### `P-2` Verdict reviewer có schema + per-AC evidence

- **Before (hiện trạng)**: `submit_review_outcome` là `{status: approved|changes_requested|blocked, summary, payload}` — payload tự do; findings là prose trong `summary`/`FinalMessage`; hub `plan_synthesis`/`synthesis` consolidate và có thể làm mất/mutate bằng chứng; back-edge `continue` quay lại `plan_writer`/`implement` chỉ mang văn xuôi; `r-requirement` (injectVibeSSDrift) và reviewer verdict dùng hai dạng dữ liệu khác nhau.
- **After (đích)**: mở rộng tool input schema của `submit-review-outcome.yaml`:
  ```yaml
  verdicts:
    - ac_id: AC-4            # map 1:1 với AC của plan/task/SS-slice
      verdict: pass|fail|blocked
      evidence:
        - { path: string, line: int, excerpt: string }
      note: string           # optional
  ```
  - Reviewer bắt buộc trả 1 row cho mỗi AC (danh sách AC được inject vào prompt reviewer từ artifact đã lock).
  - Tầng T1: transport từ chối payload sai shape (tool-call schema). Tầng T2: runner validate → sai thì đúng 1 reprompt kèm lỗi cụ thể → vẫn sai thì fail-closed escalate. `summary` giữ làm escape hatch free-text.
  - Back-edge mang `verdicts` rows **nguyên văn** tới node đích (hub không paraphrase); round/cap accounting đếm trên rows.
  - `r-requirement` so khớp signature↔SS tái dùng cùng row shape → thành structural diff (F-2 surface 1).
- **Touched**: `flow-pack/tools/submit-review-outcome.yaml`, `agents/reviewer.md` (prompt bắt verdict per-AC), `flow_executor.go` (consolidate raw rows), `flowgate` (drift rule), prompts/review-*.md.
- **Test pin**: schema-valid/invalid matrix; reprompt-once-then-escalate; back-edge payload identity test (row không đổi qua hub).

### `P-3` Escalation card / ask_user có cấu trúc (+ or-explained có schema)

- **Before (hiện trạng)**: mọi `escalate` → `ask_user` kèm `GateReason` văn xuôi (ví dụ `"task_slicer produced no Task files under requirements/08-Task/todo/; refusing to sprint from fallback"`); `r-requirement` block kèm detail prose; `r-dod-complete` or-explained **text-match** đoạn giải trình trong FinalMessage/doc (CP-47) — giải trình viết lệch cú pháp → block oan; user vibe non-tech không thể quyết từ prose.
- **After (đích)**:
  - Tool mới `request_user_decision` (flow-pack/tools) schema: `{question, options: [{id, label, consequence}], recommended, evidence[], detail}` — runner render card TUI/Desktop từ options, không regenerate.
  - **Rollout bọc-quanh (Q-1)**: turn gọi tool thành công → card render từ options; tool không được gọi hoặc payload sai schema sau reprompt → card prose hôm nay là fallback (T3 vẫn park rõ trạng thái, không mất card). Đường prose deprecate sau khi tỉ lệ payload-schema ổn định theo gate events.
  - Áp cho 3 bề mặt: card `ask_user` khi escalate; card `r-requirement` (kèm evidence drift); **or-explained** của `r-dod-complete` chuyển sang field `{explanation, referencing_ac}` trong doc/turn — hết text-match (F-2 surface 2).
  - Enforcement đủ 4 tầng D-1; card không sản sinh ở dev mode khác hôm nay cho các gate dev-card hiện hữu (chỉ đổi *payload* nội bộ, UI dev giữ 1/2/3).
- **Touched**: `flow-pack/tools/` (tool mới), `runner/gate_hook.go`, `runner/vibe_gate.go` (`parkVibeRequirement`), `internal/flowgate/evaluate.go` (or-explained), TUI/Desktop card renderer.
- **Test pin**: or-explained structured pass/explain/block; card payload schema; dev cards non-regression.

### `P-4` Per-node read-only enforcement (node isolation)

- **Before (hiện trạng)**: persona `reviewer.md` khai báo `tools: [Read, Grep, Glob, Bash]` — field `Tools` parse vào catalog (`agent_catalog.go:25`) nhưng **không có consumer trong spawn path**; read-only posture engine (`chat_posture_policy.go` — silent-deny, BUG-344 composition invariant) tường minh loại trừ flow runs (*"workflow runs never run a read-only posture"*); oracle SS-14 `AC-6` (never weaken a test) hiện phụ thuộc prompt.
- **After (đích)**:
  - `FlowNode` (hoặc AgentDefinition) cho phép khai báo posture: `read_only` | `verdict_only` | `standard`; khai báo là data trong flow YAML (hợp SD-19 `D-3`/`D-7`).
  - Posture mapping: `plan_reviewer`, `reviewer`, `scout (preflight_contract_plan)` → `read_only`; `owner_1/2` trong `vibe-owner-debate` → **`verdict_only`** (đọc + chỉ được gọi verdict tool, **không có Bash** — Q-3); coder/validate/tdd giữ `standard`.
  - `read_only` giữ bộ tool `Read, Grep, Glob` + **`Bash` với classify per-command** theo BUG-344 composition invariant: chỉ auto-approve read ops (`git diff`, `git log`, `ls`, `rg` — lưu ý `go vet`/`go build` là GHI theo phân loại BUG-344 do build cache), lệnh ghi (redirect, `rm`, `tee`...) silent-deny; regression test phủ danh sách lệnh ghi nguy hiểm (Q-3).
  - `agent.code` nodes giữ write theo freeze contract CP-55; `command.validate` node không đổi.
  - Audit: silent-deny events surfaced để đo tỉ lệ classify-sai; nếu classifier lọt lệnh ghi → BugFix theo BUG-344 process, kéo dài thì siết reviewer về bỏ-hẳn-Bash.
- **Touched**: `agent_catalog.go` (consumer cho Tools/posture), adapter spawn path (claude/codex/grok — mỗi adapter 1 wiring điểm), flow YAMLs, `chat_posture_policy.go` (mở composition cho flow runs).
- **Test pin**: reviewer write attempt → silent-deny + event; bảng regression lệnh ghi qua Bash (`rm`, `>`, `tee`, `git push`...) → deny; lệnh đọc (`git diff`, `git log`, `ls`) → allow; owner không có tool Bash và không sửa được file test (oracle SS-14 `AC-6`); three-provider parity (fakes).

### `P-5` Per-node context profile + catalog tier (trên Budget Packer)

- **Before (hiện trạng)**: mỗi flow có **một** `main_context` (canonical.head, feature.history, change.contract, source.dependence, chat.summary, source.excerpt) dùng chung mọi node; Budget Packer (CP-23 Task-334) có một priority list global; rule-card có trigger nhưng không có catalog tier phía trên (F-1).
- **After (đích)**:
  - **Phạm vi (Q-2): cả hai gia đình trong cùng slice** — harness (`task-harness`, `bug-plan-harness`, `cp-harness`) và vibe (`vibe-sprint`; các ingest flow node nhẹ nên profile mặc định trùng `main_context`, no-op an toàn).
  - Context profile per node (hoặc per node-type mặc định): `scout` = broad-rẻ (canonical.head + feature.history, hạn mức excerpt nhỏ); `plan_writer` = + source.excerpt; `reviewer` = artifact bindings + change.contract + diff (không cần source.dependence); `coder` = source.dependence + excerpts đầy đủ; vibe `tdd`/`coder` = SS-slice + handoff sprint trước (nối với `P-6`) + source.dependence.
  - Profile chọn *candidate set + budget split*; Budget Packer giữ nguyên làm engine phân bổ (D-5). Profile thiếu/không khai báo → fallback `main_context` như hôm nay.
  - Catalog tier: 1 dòng description cho mọi skill/artifact luôn hiện diện trong pack; body/compact-card nạp khi trigger khớp ngữ cảnh (kế thừa rule-card trigger CP-23).
  - `prompt_context_audit` (Task-334) dùng làm feedback: đo profile nào thừa/thiếu token để tinh chỉnh (không auto-tune ở P đầu tiên).
- **Touched**: `flow-pack/contexts/flow-context-package.yaml`, FlowNode schema (profile ref), `runner` (resolve profile trước khi gọi packer), skillpack catalog builder.
- **Test pin**: profile resolve/fallback cả hai gia đình; token cap per profile trên fixture; vibe-sprint `tdd`/`coder` nhận SS-slice + handoff trong pack; packer không đổi behavior khi không có profile (non-regression).

### `P-6` Sprint-handoff artifact có schema

- **Before (hiện trạng)**: sprint kế trong vibe dựa vào `chat.summary`/`feature.history` trong context package + reconcile từ disk (Task-328/329); *why* của các quyết định giữa các sprint (chọn X thay Y, test weakened có chủ đích) không nằm ở nơi sprint kế đọc được một cách deterministic.
- **After (đích)**:
  - Node `audit` (hub-inline) của `vibe-sprint` ghi artifact `sprint_handoff.v1` vào `requirements/.flowpilot/vibe/handoffs/`:
    ```yaml
    sprint: 3
    task: requirements/08-Task/todo/Task-904-....md
    done: [...]
    decisions:  [{ what: ..., why: ..., alternatives: [...] }]
    open:       [...]
    risks:      [...]
    weakened_tests: [{ path, line, justification }]   # rỗng = không có
    ```
  - Sprint kế tiêu thụ handoff như context source ưu tiên cao (Budget Packer slot riêng, profile `coder`/`tdd` của sprint kế đọc trước khi mở rộng).
  - `why` chỉ ghi lại từ quyết định đã có bằng chứng (verdict rows P-2, debate outcome) — không bịa (CP-49 hard ceiling áp vào runtime).
  - Không có handoff (legacy run) → hành vi hôm nay (fallback), không block.
- **Touched**: `flow-pack/flows/vibe-sprint.yaml` (audit node artifactBindings), SD-23 artifact type registration, `runner/vibe_cp.go` (chain đọc handoff), context resolver.
- **Test pin**: handoff ghi/xung đột-tên/idempotent; sprint kế nhận handoff trong pack; thiếu handoff → fallback legacy.

### `P-7` Conventions context source (repo-as-config)

- **Before (hiện trạng)**: convention dự án (test framework, build command, naming, Justfile targets) nằm rải trong prompt template hoặc SS; cùng một flow YAML chạy trên project khác nhau phải fork YAML hoặc nhét convention vào SS.
- **After (đích)**: thêm context source `conventions` qua SD-22 registry: đọc file tĩnh chuẩn repo (`.flowpilot/conventions.md`, fallback `AGENTS.md`), phân tầng user-level > workspace-level; inject vào mọi node profile như contract section (priority ngang tier-1 CP-23 — cố định, không cắt). Flow YAML giữ nguyên giữa các project.
- **Touched**: SD-22 registry (source mới), Budget Packer tier mapping, `flow-context-package.yaml`.
- **Test pin**: precedence user > workspace; file thiếu → source rỗng không lỗi; token attribution trong `prompt_context_audit`.

## 5. Touched Areas

- **files**: `internal/flowgate/{evaluate.go,rules.go,dod.go}`; `internal/runner/{gate_hook.go,vibe_gate.go,vibe_cp.go,flow_executor.go,agent_catalog.go,chat_posture_policy.go}`; `internal/agentpack/flow-pack/{tools/submit-review-outcome.yaml,tools/request-user-decision.yaml mới,agents/reviewer.md,contexts/flow-context-package.yaml,flows/*.yaml}`; adapter spawn paths (claude/codex/grok); TUI/Desktop card renderer; `internal/skillpack` (catalog tier); SD-20/SD-19/SD-22 doc sync.
- **modules**: `flowgate`, `runner`, `agentpack`, provider adapters, TUI/Desktop, `docscan` (không đổi, chỉ SS-13 contract nếu thêm artifact type).
- **database**: không có migration Supabase (run state vẫn local `sessions.ndjson`, CP-36 `P-5`).
- **external systems**: không đổi.

## 6. Data or Migration Steps

- **schema**: FlowNode thêm optional `contextProfile`, `posture` (additive, YAML parse lenient — pack thiếu field chạy như cũ); tool schemas mới/ mở rộng; artifact type `sprint_handoff.v1` đăng ký theo SD-23.
- **data backfill**: không bắt buộc — run cũ thiếu handoff/profile chạy fallback; `flow-rules.json` workspace cũ tự nhận rule mới qua `MergeDefaultRules` (pattern CP-47 `D-1`).
- **config updates**: pack inventory test hiện pin "11 flows / 8 agents" (Task-326) — thêm tool yaml làm tools count thay đổi; test inventory phải cập nhật có chủ đích (không phá pin flows/agents).

## 7. Validation Plan

- **tests to add**: per-P theo mục Test pin ở §4; TDD signatures-first, additive-only; pure-gate tests không cần provider; HTTP spot-check 1 primary + 1 other-provider (pattern Task-326 `T-5`).
- **manual checks**: chạy `task-harness` trên 1 task mẫu: verdict per-AC xuất hiện, back-edge mang đúng rows; chạy vibe ngắn 2 sprint: handoff sprint 1 được sprint 2 tiêu thụ; reviewer thử write → silent-deny log; dev mode regression full suite.
- **failure cases**: verdict sai schema 2 lần liên tiếp → escalate (không loop); profile resolve fail → fallback; handoff write fail → audit node vẫn done nhưng emit warn event (không block sprint — quyết định khi review P-6); posture misclassify một lệnh đọc → BugFix theo BUG-344 process.

## 8. Rollout and Fallback

- **rollout order**: `P-1` → `P-2` → `P-3` → `P-4` → `P-5` → `P-6` → `P-7`; mỗi P một PR riêng, merge tuần tự, có thể dừng sau bất kỳ P nào.
- **fallback path**: mọi rule/profile/posture đều có kill-switch: rule `Enabled: false` qua flow-rules.json; profile không khai báo → `main_context`; posture không khai báo → hành vi hôm nay; handoff thiếu → legacy chain.
- **monitoring**: `prompt_context_audit` (P-5/P-7 attribution), gate events mới (silent-deny, schema-invalid-reprompt), drift score + debate round counters (đã có từ Task-335).

## 9. Risks

- `R-1` **Tỉ lệ tuân thủ schema thấp trên provider yếu** → mitigated: field schema tối thiểu + escape hatch `summary` + T2 reprompt-once + T3 fail-closed; đo tỉ lệ từ gate events trước khi siết.
- `R-2` **Read-only quá chặt phá node `validate`/`tdd`** → mitigated: posture chỉ áp cho reviewer/owner/scout; `agent.code`/`command.validate` không đổi; classify per-command theo BUG-344.
- `R-3` **Profile thiếu context làm reviewer mù** → mitigated: reviewer profile luôn gồm artifact bindings + diff; fallback `main_context`; audit attribution bắt lệch sớm.
- `R-4` **Handoff artifact bị coder ghi sai/ bypass** → mitigated: chỉ node `audit` hub-inline ghi (không phải `agent.code`), schema validate, coder không có binding ghi vào handoffs/.
- `R-5` **Precedence sai làm vibe dừng nhiều hơn cần thiết** → mitigated: matrix test mode×gate; dev non-regression là gate bắt buộc mỗi PR.
- `R-6` **Scope creep 7 slice** → mitigated: mỗi P ship độc lập, có kill-switch; dừng sau bất kỳ P nào vẫn để lại giá trị đã merge.

## 10. Definition of Done

- [x] SD-20 có bảng precedence `P-1` (§7), pin bằng unit matrix test (`flowgate/precedence_test.go` + `runner/vibe_gate_precedence_test.go`); dev mode non-regression: flowgate full suite xanh, legacy `classifyVibeGate` byte-stable, hang full-suite runner = deadlock pre-existing `TestBUG327` xác minh identical trên base (worktree) theo CA convention Task-331.
- [x] `P-2`: reviewer verdict per-AC + evidence qua tool schema; sai schema → reprompt-once (tool error in-turn) → backstop CP-61 hub-done; back-edge payload identity test pass (`TestHubForwarding_PreservesRawVerdictsIdentity`).
- [x] `P-3`: card escalate render từ options có cấu trúc qua event `user_decision_card_requested` (client renderer consume payload — Task-339 §7 scoping); `r-dod-complete` or-explained là field có schema (`TurnResult.DodExplanation`, legacy fallback giữ nguyên); dev cards 1/2/3 không đổi.
- [x] `P-4`: reviewer/owner/scout silent-deny write — enforcement tại bridge provider-neutral (parity by construction, Case 1); event `node_isolation_write_denied` surfaced; oracle test "owner không sửa được test" pass (`TestNodeIsolation_OwnerVerdictOnly_NoBash` + pack posture pin).
- [x] `P-5`: harness flows + `vibe-sprint` chạy với per-node profile (Q-2 cả hai gia đình); không profile → fallback nguyên bản (test pin); `prompt_context_audit` đi qua pipeline Task-334 với budget override + catalog.
- [x] `P-6`: `vibe-sprint` audit ghi `sprint_handoff.v1` (verified-state only); sprint kế tiêu thụ handoff trong entry prompt; thiếu handoff → legacy fallback.
- [x] `P-7`: conventions source nạp user > workspace (AGENTS.md fallback); flow YAML không fork — 3 flagship flows tham chiếu `conventions` qua profiles, full-pack validation pass.
- [x] Safe-fix: 9 file test mới 100% additive (0 deletions, không test cũ nào bị sửa — verify bằng git diff); CA-849..855 per slice với `feature_key: zcode-parity` (registered trong FEATURE-KEYS.md).
- [x] Pack inventory: flows/agents giữ 11/8 (`TestPack_InventoryUnchanged` green); tools count thay đổi (thêm `request-user-decision.yaml`) — không phá pin flows/agents.
- Review pass bổ sung: 1 lock-order inversion (s.mu ↔ drift st.mu trong `recordDriftTelemetry`) được phát hiện và sửa trước khi đóng; `-race` trên toàn bộ test surface CP-62 xanh (trừ `TestTask330_ResumeFromTddStartsSprintWhenNoTddOutput` pre-existing — xác minh fail identical trên base).
