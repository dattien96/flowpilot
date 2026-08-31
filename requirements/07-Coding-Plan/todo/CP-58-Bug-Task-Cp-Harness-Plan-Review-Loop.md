# CP-58: Bug / Task / CP Harness With Plan Artifact And Dual Review Loops

## Metadata

- Document ID: `CP-58`
- Title: `Bug / Task / CP Harness With Plan Artifact And Dual Review Loops`
- Phase: `coding_plan`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-27`
- Last Updated: `2026-08-31`
- Parent Documents: [SD-19: Agent Flow Engine](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md), [SD-18: Main-Hub Agent Review Loop](../../06-System-Tech-Design/SD-18-Main-Hub-Agent-Review-Loop.md), [SD-23: Generic Artifact Framework](../../06-System-Tech-Design/SD-23-Generic-Artifact-Framework.md), [SD-17: Context And Regression Engine](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md), [SD-21: Change Contract And Canonical Intent Signature](../../06-System-Tech-Design/SD-21-Change-Contract-And-Canonical-Intent-Signature.md), [SS-15: Agent Review Loop (Review Until Clean)](../../05-System-Specs/SS-15-Agent-Review-Loop-Until-Clean.md), [SS-16: Agent Flow Engine](../../05-System-Specs/SS-16-Agent-Flow-Engine.md), [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md)
- Child Documents: [Task-304: Dual Back-Edge Validator And Edge-Driven Re-entry](../../08-Task/todo/Task-304-Dual-Back-Edge-Validator-And-Edge-Driven-Re-entry.md), [Task-305: Task-Harness 11-Step With Plan Writer And Plan Review Loop](../../08-Task/todo/Task-305-Task-Harness-11-Step-With-Plan-Writer-And-Plan-Review-Loop.md), [Task-306: CP-Harness 12-Step With CP Plan And Task Splitter](../../08-Task/todo/Task-306-Cp-Harness-12-Step-With-Cp-Plan-And-Task-Splitter.md), [Task-307: Harness Plan Artifact Types And Panel Parity](../../08-Task/todo/Task-307-Harness-Plan-Artifact-Types-And-Panel-Parity.md)
- Related Documents: [CP-55: Flow-First Preflight Contract](../../07-Coding-Plan/done/CP-55-Flow-First-Preflight-Contract-Context-Retrieval-And-Canonical-Acceptance.md), [CP-36: Agent Review Loop And Main Hub Orchestration](../../07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md), [CP-41: RAG Harness Flow Mode](../../07-Coding-Plan/done/CP-41-RAG-Harness-Flow-Mode.md), [rag-harness.yaml](../../../apps/local-runner/internal/agentpack/flow-pack/flows/rag-harness.yaml), [Task-293: Rag-Harness TDD + Review Loop](../../08-Task/done/Task-293-RagHarness-TDD-Test-Signatures-And-Review-Until-Clean-Loop.md), [CP-57: Opencode Provider Integration](./CP-57-Opencode-Provider-Integration.md), [Task-260: safe-fix-contract Chat Plan/Code Pointer + r-additive-tests Gate](../../08-Task/inprogress/Task-260-R-Additive-Tests-Gate-Rule.md)
- Replaces: `None`
- Tags: `flow, harness, bug, task, cp, plan-artifact, review-loop, agent-flow-engine, artifact-framework`
- Feature Keys: `agent-flow-engine`

## AI Quick View

### Summary

- Keeps the current `rag-harness` 9-step as **`bug-harness`** for hotfix/bug (`plan→freeze→context→test_signatures→implement→validate→reviewer→synthesis→audit`, 1 loop `validate→implement`). No cost added for small scope.
- Adds **`task-harness` 12-node** for a normal `Task`: `plan (Scout)→context (Draft)→plan_writer (Architect)→plan_reviewer→plan_synthesis` **plan review loop** (`--continue(back)-->plan_writer`) must approve plan before scope locks; `plan_synthesis --done-->freeze (Lock approved scope)→test_signatures→implement→validate→reviewer→synthesis→audit`. `plan_writer` receives draft code context + 3-layer defense (override feature_key + tool fallback) to write `requirements/08-Task/*.md` as `file_artifact`; `freeze` locks scope only after plan review approval.
- Adds **`cp-harness` 6-node slice-only** for a `CP`: `plan→cp_plan_writer→cp_reviewer→cp_synthesis` (plan loop) → `task_splitter` → `audit`. No `freeze`/`context` in slice-only (no coding). Opt-in `cp-harness-smoke.yaml` (13 nodes) adds `context→freeze→test_signatures→implement→validate→reviewer→synthesis` after `task_splitter` for first-Task coding; per-Task coding otherwise runs via `task-harness` separately.
- Unblocks the 2-loop shape by fixing the pack validator / `resolveContinueBackEdgeTarget` (`pack.go:845`, `flow_executor.go:910`) from single `when` key to `(from,when)` so `plan loop` and `code loop` can coexist as two `continue/back` edges.

### Current Ask

- Author a coding plan that makes `BUG → 9-step`, `Task → 11-step`, `CP → 12-step` selectable built-ins, eliminates the manual `CP→Task*.md` slicing we do today (`CP-58` itself was sliced manually into `Task-300..303`), and keeps the 3 harnesses additive over `rag-harness` with zero regression to `review-loop`/`context-coding-review-synthesis`.

### Key Decisions

- `P-1` **Three harnesses, not one fat flow.** `bug-harness` (clone of current `rag-harness`, maybe renamed), `task-harness` (9 rag nodes + 3 plan nodes `plan_writer`/`plan_reviewer`/`plan_synthesis` = 12 nodes), `cp-harness` (slice-only 8 nodes; smoke variant 13 nodes: + `task_splitter` + coding chain). SelectableIn `flow`, cloneable. Sharing prompts/agents, not edges, keeps each harness' `cap`/`acceptance_nodes` honest and cost-proportional. Alternative "one 12-step with conditional skip" needs engine-level conditional edges — deferred.
- `P-2` **`preflight_contract_plan` is not the plan.** It stays as `contract-defined` JSON (`agents/contract-planner.md:23`). The *real* plan is `plan_writer` / `cp_plan_writer` (`agent.code`, bounded file_artifact md output) — cost is explicit and reviewable. This fixes the owner's #2 confusion.
- `P-3` **Plan review loop mirrors code review loop** (`P-2`/`P-3` reuse): `plan_reviewer:agent.delegate cohort:plan join:all` → `plan_synthesis:hub.inline join:all tools:[submit_review_outcome]` with single `continue/back` to its writer. Review criteria = `SS-13 §5` doc contract + `R1/R2/R3` analog for plan (missing `AC`, missing `DeclaredPaths`, single happy-path matrix, contradicts `CA` history). Code `test_signatures` sits *between* `plan_writer` and its review so the reviewer sees both.
- `P-4` **CP flow stops after slice by default.** `cp-harness` default terminal is after `task_splitter`+`audit` (plan+slice audit), not after full coding of every sliced Task. Coding uses `task-harness` per Task to keep `cap:3` per unit and avoid a 12-step linear chain that would need `cap:6` and defeat retry scoping. Optional smoke = separate `cp-harness-smoke.yaml` that chains coding of the *first* Task only.
- `P-5` **Dual `continue/back` edges are first-class.** Fix `pack.go:865 ValidateFlowDefinition` duplicate check and `flow_executor.go:910 resolveContinueBackEdgeTarget` to be source-aware (`from+when`). This is the only engine change; everything else is pack YAML + artifact types.
- `P-6` **Plan artifacts are `file_artifact` typed artifacts** (`SD-23 D-11`), not free-form writes. `plan_writer` outputs `file_artifact` instance `plan_md` (OUTPUT, required); `plan_reviewer` reads it as INPUT; `task_splitter` reads `CP.md` INPUT and writes `task_md[]` OUTPUT. Deterministic, validated, and visible in the artifact panel. No new artifact *type* DSL — reuse existing `file_artifact`.
- `P-7` **Tiered Model Strategy & 2-Phase Context (Scout vs Architect).**
  - **Phase 1 (Scout):** `preflight_contract_plan` runs with a Fast/Cheap model (e.g. `flash`/`mini`) supplied with **Broad Context** (`FEATURE-KEYS.md`, recent `CA-*` history, `chat.summary`, git branch context) to scan the codebase quickly and propose initial scope without burning expensive reasoning tokens.
  - **Phase 2 (Architect & Gate):** `plan_writer` / `cp_plan_writer` and `plan_reviewer` run with the **Highest Reasoning model** (e.g. `claude-3.7-sonnet`, `o3`, `grok-4.5 high`) to author and review the detailed HLD/LLD plan artifact (`Task-*.md` / `CP-*.md`).
  - **Phase 3 (Deep Code Context):** `context.produce` packages `change.contract` and `source.excerpt` for `tester` and `coder` once the plan is frozen.

### Constraints

- Additive only over `rag-harness`/`review-loop`/`context-coding-review-synthesis`; do not mutate their proven 1-loop `cap:3` semantics except the narrow validator fix in `P-5`.
- `agent.code` writer semantics (`CP-55 P-1` `ValidateFlowSafetyTopology`) still hold: every writer needs a dominating `contract.freeze`, every `done` path crosses `acceptance_nodes`.
- No new provider branching; harness choice is mode data.
- `Task`/`BugFix` remain deltas per `SS-13` — plan md is the Task's own spec, not a replacement for `CP`.
- Manual `CP→Task` today is ~30-60 min per Task; target harness reduces to <5 min review per plan loop round.

### Open Questions

- `Q-1` Should `cp-harness` by default stop after slice+slice-audit, or chain coding of Task-1 as smoke? (Proposed: stop after slice; smoke is opt-in clone.)
- `Q-2` Plan `file_artifact` store path: `requirements/07-Coding-Plan/todo/CP-*.md` vs `requirements/08-Task/todo/Task-*.md` — per-artifact instance `config.pathTemplate` or writer's `declared_paths`?
- `Q-3` Plan review cohort size: 1 reviewer (like `rag-harness`) vs 2 (correctness+completeness)? Proposed 1 for cost.
- `Q-4` Cap split: `plan cap:3` + `code cap:3` independent or shared `policy.cap:5`? **RESOLVED**: single shared `policy.cap:3` covers both loops — reset scoping via `forwardReachableNodeIDs` (`flow_executor.go:936`) keeps per-loop rounds honest (see §6).

### Source Refs

- `SS-15`, `SS-16` §§7; `SD-18`, `SD-19` §§5-7; `SD-23` D-11; `SD-17`, `SD-21` (CP-55); `CP-36`, `CP-41`, `CP-55`, `CP-57`.
- `rag-harness.yaml:43-148`, `review-loop.yaml`, `context-coding-review-synthesis.yaml`, `pack.go:794 ValidateFlowDefinition`, `flow_executor.go:904 resolveContinueBackEdgeTarget`, `flow_validate_audit_dispatch.go`, `artifact_type_registry.go`.

## 1. Goal

Give FlowPilot a **tiered harness family** that matches actual work size and removes manual `CP→Task` slicing:

| Tier | When | Flow | Steps | What it guarantees |
|---|---:|---|---|---|
| Bug | hotfix, 1 file, already clear `AC` | `bug-harness` 9-step (current `rag-harness` as-is, maybe renamed) | `plan→freeze→context→test_signatures→implement→validate→reviewer→synthesis→audit` | TDD signatures + code review loop |
| Task | feature slice 1-3d, needs HLD/LLD | `task-harness` 12-node (11 steps + audit) | `plan→plan_writer→plan_reviewer→plan_synthesis` (plan loop) `→freeze→context→test_signatures→implement→validate→reviewer→synthesis→audit` (code loop) | **Plan approved THEN scope frozen** + code loop |
| CP | multi-task initiative | `cp-harness` slice-only 6-node; smoke 13-node | `plan→cp_plan_writer→cp_reviewer→cp_synthesis` (plan loop) `→task_splitter→audit` (slice-only default). Smoke: `→freeze→context→test_signatures→implement→...→audit` | **CP reviewed → auto-sliced**; per-Task coding via `task-harness` |

A user picks the tier in `/flow` picker; the flow writes the correct md artifacts (`CP-*.md`, `Task-*.md`, `BUG-*.md`) via `file_artifact` so the next tier can consume them — no manual copy-paste.

### 1.1 Tổng quan Hệ thống 3 Flow Mục tiêu (Vietnamese Overview)

FlowPilot phân tách rõ ràng thành **3 Flow chuyên biệt** tùy theo quy mô công việc, áp dụng chiến lược **Phân tầng Model (Tiered Models)** và **2 Pha Ngữ cảnh (2-Phase Context)**:

```
┌─────────────────────────────────────────────────────────────────────────────────────────────┐
│ 1. BUG-HARNESS (9 steps): Hotfix / Bug nhỏ (1 file, AC rõ ràng)                            │
│    Scout (Model Rẻ) ──> Freeze ──> Context ──> TDD Signatures ──> Coder ──> Code Review     │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│ 2. TASK-HARNESS (12 nodes): Task / Feature vừa (1-3 ngày, cần HLD/LLD)                     │
│    [Pha 1: Context & Plan] Scout (Model Rẻ) ──> Context.produce (Draft Code Excerpts)      │
│            ──> Plan Writer (Model Xịn, tạo Task-*.md)                                       │
│            ──> Plan Review Loop (Model Xịn duyệt Plan) ⟲ Plan Writer                        │
│    [Pha 2: Lock Scope]     Plan OK ──> Freeze (Khóa scope chính thức từ Plan đã duyệt)      │
│    [Pha 3: Code & Review]  TDD Signatures ──> Coder ──> Code Review Loop ──> Audit         │
├─────────────────────────────────────────────────────────────────────────────────────────────┤
│ 3. CP-HARNESS (6 nodes slice-only): Big Feature / Epic lớn                                  │
│    Scout ──> CP Plan Writer (Model Xịn, tạo CP-*.md)                                       │
│    ──> CP Review Loop (Duyệt kiến trúc CP) ⟲ CP Plan Writer                                 │
│    ──> Task Splitter (Tự động bẻ CP ra Task-1, Task-2, Task-3.md) ──> Audit                 │
│    * KHÔNG có Freeze/Context/Coding (từng Task con sẽ chạy bằng task-harness riêng)         │
└─────────────────────────────────────────────────────────────────────────────────────────────┘
```

#### Chi tiết từng Flow:

1. **`bug-harness` (Hotfix / Bug nhỏ - 9 bước, 1 vòng lặp Code)**:
   - **Mục đích**: Xử lý nhanh các lỗi đơn lẻ, rõ ràng, không tốn thêm token/turn lên kế hoạch dài dòng.
   - **Quy trình**: `plan (Scout) → freeze → context → test_signatures → implement → validate → reviewer → synthesis → audit`.
   - **Đảm bảo**: TDD test signatures và code review loop hoàn chỉnh.

2. **`task-harness` (Task / Feature vừa - 12 nodes, 2 vòng lặp độc lập)**:
   - **Mục đích**: Dành cho các tính năng cần phân tích thiết kế (HLD/LLD). Đảm bảo **Plan Writer có code context để viết chi tiết, Plan được duyệt sạch sẽ rồi mới Freeze scope cho Coder**.
   - **Chiến lược Model & Ngữ cảnh**:
     - **Bước 1 (`preflight_contract_plan`)**: Dùng **Model Rẻ/Nhanh** (`flash`/`mini`) + nạp **Broad Context** (`FEATURE-KEYS.md`, `CA-*` gần nhất, branch info) để quét nhanh codebase, đưa ra `candidate_feature_keys` và `candidate_paths`.
     - **Bước 2 (`context.produce` - Draft Context)**: Đóng gói source code excerpts ban đầu dựa trên candidate scope nạp vào prompt cho Plan Writer.
     - **Bước 3 (`plan_writer`)**: Dùng **Model Xịn nhất / Reasoning cao nhất** (`Claude 3.7 Sonnet`, `o3`, `Grok 4.5 high`) để viết file artifact `requirements/08-Task/Task-*.md` chi tiết (có exact changes, signatures). Có quyền tự sửa `feature_key` nếu Scout đoán sai và dùng tool đọc code dự phòng.
     - **Bước 4 (`plan_reviewer` + `plan_synthesis`)**: **Plan Review Loop** — Model Xịn kiểm tra tính toàn vẹn của Plan và tính chuẩn xác của `feature_key`. Nếu có lỗ hổng (`changes_requested`), quay lại bắt `plan_writer` sửa lại Plan cho tới khi `approved`.
     - **Bước 5 (`preflight_contract_freeze`)**: **Khóa scope chính thức** dựa trên `Task-*.md` đã được duyệt, đảm bảo an toàn tuyệt đối trước khi Coder làm việc.
     - **Bước 6 (`test_signatures` → `implement` → `reviewer` → `synthesis`)**: **Code Review Loop** — Coder triển khai theo đúng Plan đã duyệt; Code Reviewer thẩm định chất lượng trước khi `audit` kết thúc flow.

3. **`cp-harness` (Big Feature / Coding Plan - Mặc định 6 nodes slice-only)**:
   - **Mục đích**: Tự động hóa hoàn toàn việc lập Coding Plan lớn và phân rã thành các Task con, chấm dứt việc bẻ task thủ công (manual slicing).
   - **Quy trình mặc định (Slice-only, KHÔNG có freeze/context/coding)**:
     - `preflight_contract_plan` (Scout) quét nhanh phạm vi.
     - `cp_plan_writer` (Model Xịn) viết tài liệu kiến trúc `requirements/07-Coding-Plan/CP-*.md`.
     - `cp_reviewer` + `cp_synthesis` thẩm định kiến trúc Coding Plan (loop nếu cần).
     - `task_splitter` tự động đọc `CP.md` và sinh ra danh sách file `requirements/08-Task/Task-*.md` độc lập, đầy đủ `P-*` traceability.
     - Flow kết thúc tại `audit` và lưu toàn bộ Task vào hàng đợi `todo/`. **Mỗi Task sau đó được thực thi bằng `task-harness`** (có đầy đủ context + freeze + coding).
   - **Biến thể kiểm thử (`cp-harness-smoke`, opt-in 13 nodes)**: Thêm `context→freeze→test_signatures→implement→...→audit` sau `task_splitter` để chạy thử nghiệm luồng code cho Task đầu tiên.

### 1.2 Cơ chế Phòng vệ 3 Lớp Chống Sai Lệch Feature-Key & Ngữ Cảnh (3-Layer Defense)

Khi sử dụng mô hình AI phân tầng (Tiered Model: Model Rẻ làm Scout $\rightarrow$ Model Xịn làm Architect/Coder), rủi ro lớn nhất là **Scout suy đoán sai `feature_key` hoặc bỏ sót file quan trọng**. Để giải quyết triệt để, FlowPilot áp dụng **Cơ chế Phòng vệ 3 Lớp**:

```
[User Request]
       │
       ▼
1. SCOUT (Model Rẻ) ──> Đưa ra candidate_feature_keys (Top 1-2 giả thuyết) + candidate_paths
       │
       ▼
2. CONTEXT.PRODUCE  ──> Nạp Hybrid Context (Dựa trên Feature-Key + Từ khóa tìm kiếm + Git Diff)
       │
       ▼
3. PLAN WRITER (Model Xịn)
       ├──> Nhận cả: User Request + Danh sách FEATURE-KEYS.md + Context từ Scout
       ├──> 🛡️ LỚP 1: TỰ SỬA (OVERRIDE) — Nếu thấy Scout đoán sai, Model xịn tự chọn lại Feature Key đúng
       └──> 🛡️ LỚP 2: TOOL DỰ PHÒNG — Có sẵn Read/Grep tool để tự tìm lại file đúng nếu context bị lệch
       │
       ▼
4. PLAN REVIEWER (Model Xịn)
       └──> 🛡️ LỚP 3: GATING RULE — Kiểm tra Feature Key & Declared Paths có đúng chuẩn không.
            Nếu sai ──> Reject (changes_requested) ──> Plan Writer sửa lại.
       │
       ▼
5. CONTRACT.FREEZE (Chốt Feature Key & Scope CHÍNH THỨC từ Plan đã duyệt)
```

#### Chi tiết 3 Lớp:

- **🛡️ Lớp 1 (Tự sửa / Override tại Plan Writer)**:
  - Scout chỉ coi `feature_key` là **Giả thuyết ban đầu (`candidate_feature_keys`)**, không có quyền phán quyết tuyệt đối.
  - `plan_writer` (Model Xịn) được cung cấp toàn bộ nội dung `FEATURE-KEYS.md`. Model xịn tự đối chiếu User Request với code hiện tại. Nếu Scout nhận định sai (ví dụ: lỗi paste terminal bị gán nhầm vào `chat-engine`), Model xịn sẽ **ghi đè (override)** lại `feature_key: tui-windows` chính xác vào tài liệu `Task-*.md`.

- **🛡️ Lớp 2 (Hybrid Retrieval & Tool Đọc Code Dự Phòng)**:
  - `context.produce` không chỉ filter theo 1 `feature_key`, mà chạy **Hybrid Retrieval**: kết hợp `feature_key` + `git branch/diff` + `search keywords` từ user prompt $\rightarrow$ Dù `feature_key` của Scout có lệch nhẹ vẫn gom được các file liên quan.
  - Đồng thời, `plan_writer` được cấp tool **`read_file`** và **`grep_search`** làm cứu cánh: Nếu context tự động bị thiếu một struct hay helper quan trọng, Model xịn chỉ mất 1 turn grep là đọc được ngay source code cần thiết để viết Code Guide.

- **🛡️ Lớp 3 (Gating Rule tại Plan Reviewer & Freeze Chốt Chặn)**:
  - Trong `prompts/review-plan.md`, Reviewer có tiêu chí bắt buộc: **`Feature-Key & Scope Contract`** — kiểm tra `feature_key` trong `Task-*.md` có map đúng với domain của task và tồn tại hợp lệ trong `FEATURE-KEYS.md` hay không.
  - Nếu sai, Reviewer sẽ reject (`changes_requested`), kích hoạt loop quay lại `plan_writer` sửa lại.
  - **`contract.freeze` chỉ chạy sau khi Plan Review Loop hoàn tất**: Nhờ vậy, scope được khóa cứng dựa trên sự đồng thuận của 2 Model Xịn (Writer & Reviewer), triệt tiêu hoàn toàn rủi ro scope bị khóa sai từ Scout ban đầu.

> [!IMPORTANT]
> **Nguyên tắc Cốt Lõi (Core Rule):**
> 1. **Scout (Model Rẻ)** tuyệt đối **KHÔNG có quyền chốt `feature_key`**; Scout chỉ đưa ra gợi ý ban đầu (`candidate_feature_keys`).
> 2. **`plan_writer` (Model Xịn)** bắt buộc **phải tự kiểm tra và đối chiếu User Request với `FEATURE-KEYS.md`**. Nếu Scout chọn sai, `plan_writer` có toàn quyền và trách nhiệm **GHI ĐÈ (override)** lại `feature_key` đúng vào `§Metadata` của file plan (`Task-*.md` hoặc `CP-*.md`).
> 3. **`plan_reviewer` (Model Xịn)** kiểm định lại `feature_key` lần 2. Chỉ sau khi cả 2 Model Xịn đồng thuận thì `contract.freeze` mới khóa scope chính thức cho coder.

## 2. Input Documents

- [SD-19: Agent Flow Engine](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md)
- [SD-18: Main-Hub Agent Review Loop](../../06-System-Tech-Design/SD-18-Main-Hub-Agent-Review-Loop.md)
- [SD-23: Generic Artifact Framework](../../06-System-Tech-Design/SD-23-Generic-Artifact-Framework.md)
- [SD-17: Context And Regression Engine](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md)
- [SD-21: Change Contract And Canonical Intent Signature](../../06-System-Tech-Design/SD-21-Change-Contract-And-Canonical-Intent-Signature.md)
- [CP-55: Flow-First Preflight Contract](../../07-Coding-Plan/done/CP-55-Flow-First-Preflight-Contract-Context-Retrieval-And-Canonical-Acceptance.md)
- [CP-36: Agent Review Loop And Main Hub Orchestration](../../07-Coding-Plan/done/CP-36-Agent-Review-Loop-And-Main-Hub-Orchestration.md)
- [CP-41: RAG Harness Flow Mode](../../07-Coding-Plan/done/CP-41-RAG-Harness-Flow-Mode.md)

## 3. Implementation Strategy

- **overall approach:**
  - Keep `rag-harness` byte-identical as `bug-harness` (rename or clone). Additive new packs only.
  - Fix the engine's dual-loop limitation first (smallest blast radius), then add pack YAML for `task-harness` and `cp-harness` reusing existing agents/prompts plus 2 new prompts + 1 splitter.
  - Wire `file_artifact` bindings for md outputs so Desktop already renders them (SD-23 panel) with zero new UI except harness picker labels.
  - Mirror `Task-293` additive-test style: every harness YAML ships with pack-level topology tests + a minimal `flow_executor` dual-loop test; no pre-existing test edited except inventory.
- **sequencing logic:**
  1. `P-1` Engine: dual `continue/back` (`pack.go` + `flow_executor.go`).
  2. `P-2` Pack: `task-harness` 11-step YAML + prompts + `manifest.yaml` entry.
  3. `P-3` Pack: `cp-harness` 12-step YAML + `task_splitter` + CP prompts.
  4. `P-4` Artifact: `file_artifact` instances for `plan_md` / `cp_md` / `task_md` + bindings.
  5. `P-5` Docs: `SS-13` contract checks for new harness DODs; optional TUI/WorkflowsSettings harness labels.
  - Each P independently testable; landing order is 1→2→4→3, with 3 gated on 2's dual-loop proof.
- **dependencies:**
  - `agentpack/pack.go` validator + `runner/flow_executor.go` back-edge resolver (for `P-1`).
  - `internal/agentpack/flow-pack/flows/*`, `flow-pack/prompts/*`, `flow-pack/manifest.yaml`, `supabase_workflow_flow_store.go` (mirror sync), `artifact_type_registry.go` (for `P-4`).
  - `review-loop`/`rag-harness` as reference to copy edge semantics.

### 3.1 Topology Sketches (normative)

**task-harness (12 nodes incl. `audit`, 2 loops share no edge):**

| Node | Run | Lifecycle | Behavior | Agent | Cohort/Join |
|---|---|---|---|---|---|
| `preflight_contract_plan` | delegate | once | agent.delegate | contract-planner | — (Scout) |
| `context` | inline | once | context.produce | — | — (Draft Context) |
| `plan_writer` | delegate | reinvoke | agent.code | coder | — (Architect) |
| `plan_reviewer` | delegate | spawn | agent.delegate | reviewer | cohort:plan join:all |
| `plan_synthesis` | inline | reinvoke | hub.inline | synthesizer | join:all |
| `preflight_contract_freeze` | inline | once | contract.freeze | — | — (Lock Scope) |
| `test_signatures` | delegate | once | agent.code | tester | — |
| `implement` | delegate | reinvoke | agent.code | coder | — |
| `validate` | inline | once | command.validate | — | — |
| `reviewer` | delegate | spawn | agent.delegate | reviewer | cohort:review join:all |
| `synthesis` | inline | reinvoke | hub.inline | synthesizer | join:all |
| `audit` | inline | once | artifact.audit_draft | — | — |

Edges: `plan→context→plan_writer→plan_reviewer→plan_synthesis`; `plan_synthesis --continue(back)--> plan_writer` (loop 1), `plan_synthesis --done(forward)--> freeze→test_signatures→implement`; `implement→validate --done--> reviewer→synthesis --done--> audit→done`; `validate --continue(back)--> implement` (loop 2). `acceptance_nodes:[plan_synthesis, validate, synthesis, audit]`.

**KEY ORDERING & CONTEXT FLOW**:
1. `context.produce` chạy ngay sau `scout` để cung cấp draft code excerpts cho `plan_writer`.
2. `plan_writer` có quyền tự sửa `feature_key` và tool đọc code dự phòng (3-Layer Defense).
3. `freeze` chỉ chạy sau khi `plan_synthesis` duyệt xong $\rightarrow$ Khóa scope chính thức chuẩn xác 100% trước khi Coder làm việc.

**cp-harness (slice-only default 7 nodes incl. `audit`; smoke variant 13 nodes, same engine):**

| Node | Run | Lifecycle | Behavior | Agent | Cohort/Join |
|---|---|---|---|---|---|
| `preflight_contract_plan` | delegate | once | agent.delegate | contract-planner | — (Scout) |
| `context` | inline | once | context.produce | — | — (Draft Context) |
| `cp_plan_writer` | delegate | reinvoke | agent.code | coder | — (CP Architect) |
| `cp_reviewer` | delegate | spawn | agent.delegate | reviewer | cohort:plan join:all |
| `cp_synthesis` | inline | reinvoke | hub.inline | synthesizer | join:all |
| `task_splitter` | delegate | once | agent.code | coder | — |
| `audit` | inline | once | artifact.audit_draft | — | — |

Edges: `plan→context→cp_plan_writer→cp_reviewer→cp_synthesis`; `cp_synthesis --continue(back)--> cp_plan_writer` (loop), `cp_synthesis --done(forward)--> task_splitter→audit→done`. No `freeze`/`test_signatures`/`coding` — slice-only CP has no coding step; per-Task coding runs via `task-harness` separately. Smoke variant `cp-harness-smoke.yaml` (13 nodes) inserts `freeze→test_signatures→implement→validate→reviewer→synthesis` between `task_splitter` and `audit` for first-Task coding.

## 4. Work Breakdown

- `P-1` **Engine: allow two `continue/back` edges, make re-entry source-aware.** Files: `apps/local-runner/internal/agentpack/pack.go:845` change `backEdgeSources map[when]from` to `map[when+from]from` (or `map[string]map[string]string`), `apps/local-runner/internal/runner/flow_executor.go:910 resolveContinueBackEdgeTarget` to variadic `(edges, from ...string)` with hub-aware resolution (edge.From==from; else for a hub emitter the back-edge whose From's forward-closure reaches the hub; else first-match), `apps/local-runner/internal/runner/interactive_service.go:1507/2900` pass `rs.activeHubNodeID` (Task-235) into resolver, `apps/local-runner/internal/runner/flow_step_runtime.go:577` guard `hubInlineNodeID` first-match for >1 hub.inline node (`activeHubNodeID` tracking at cohort-join `:4711` + `loopIsAdvancing` sites), `apps/local-runner/internal/agentpack/pack_test.go` dual-loop acceptance test. Guard: existing single-loop flows still load (`Resolve` picks the only `continue/back` regardless of `from`; variadic keeps all pre-existing call sites compiling unchanged).
- `P-2` **`task-harness` 11-step pack.** Files: `flows/task-harness.yaml` (new, 12 nodes), `prompts/plan-task.md` (HLD/LLD + SS-13 §5.1 metadata + `AC` + `DeclaredPaths` + `Risks` + 3-Layer Defense), `prompts/review-plan.md` (plan+signatures gate: feature_key check, `AC` completeness, `P-*` traceability, single happy-path, contradicts `CA` history, per `SS-13` doc contract), `manifest.yaml` flows/prompts entries, `flows/bug-harness.yaml` (clone of `rag-harness.yaml` if renamed). `acceptance_nodes` includes `plan_synthesis`.
- `P-3` **`cp-harness` pack.** Files: `flows/cp-harness.yaml` (new, slice-only 7 nodes) + `flows/cp-harness-smoke.yaml` (new, opt-in 13 nodes), `prompts/plan-cp.md` (CP §1-§10, `DOD`, `P-*`, `R-*`, 3-Layer Defense), `prompts/review-cp.md`, `prompts/task-splitter.md` (read `CP.md` file_artifact INPUT → write `Task-*.md` file_artifacts OUTPUT, one per `P-*`), `manifest.yaml` entries. Default terminal after `task_splitter` + slice `audit` (slice-only, `acceptance_nodes:[cp_synthesis, audit]`); `cp-harness-smoke` chains `task_splitter→freeze→test_signatures→implement→validate→reviewer→synthesis→audit` (extends `acceptance_nodes` with `validate, synthesis`).
- `P-4` **Plan artifact bindings.** Files: `internal/agentpack/flow-pack/contexts/*` if needed, `internal/runner/artifact_type_registry.go` `file_artifact` resolver reuse (Task-201/202 path injection), `supabase/migrations/*_add_harness_artifacts.sql` seeding `artifact_types`/`artifact_instances` (`is_builtin=true`) for `plan_md`/`cp_md`/`task_md`, `supabase_workflow_flow_store.go` `recordFromWorkflowRow` binding denormalization. RLS `authenticated` blocked on `is_builtin=true` (mirror `workflows` pattern `SD-23 §3.11`).
- `P-5` **Validation + docs.** Files: `internal/agentpack/task_harness_pack_test.go`, `internal/runner/task_harness_dual_loop_test.go` (plan loop + code loop both reinvoke with session reuse), `requirements/05-System-Specs/SS-13*` DOD note, `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx` harness labels (optional).
- `P-6` **Parity + operator.** Wire `skillpack`, TUI `/flow` picker `selectableIn:flow`, `cloneable:true`, `mirror:required`. No provider branching.

## 5. Touched Areas

- **files:**
  - `apps/local-runner/internal/agentpack/pack.go` (`ValidateFlowDefinition` dual back-edge)
  - `apps/local-runner/internal/agentpack/flow-pack/flows/task-harness.yaml` (new, 12 nodes), `flows/cp-harness.yaml` (new, slice-only 7 nodes), `flows/cp-harness-smoke.yaml` (new, opt-in 13 nodes), `flows/bug-harness.yaml` (new if renamed)
  - `apps/local-runner/internal/agentpack/flow-pack/manifest.yaml` (flows/prompts)
  - `apps/local-runner/internal/agentpack/flow-pack/prompts/plan-task.md` (new), `prompts/review-plan.md` (new), `prompts/plan-cp.md` (new), `prompts/review-cp.md` (new), `prompts/task-splitter.md` (new)
  - `apps/local-runner/internal/agentpack/flow-pack/agents/*` (reuse `coder.md`/`reviewer.md`/`synthesizer.md`, no edit)
  - `apps/local-runner/internal/runner/flow_executor.go` (`resolveContinueBackEdgeTarget` + `forwardReachableNodeIDs` call sites)
  - `apps/local-runner/internal/runner/interactive_service.go` (`applyFlowControl` continue reset + hub-inline `activeHubNodeID` tracking at cohort-join/`loopIsAdvancing` sites)
  - `apps/local-runner/internal/runner/flow_step_runtime.go` (`hubInlineNodeID` guard for >1 hub.inline node)
  - `apps/local-runner/internal/runner/artifact_type_registry.go`, `supabase_workflow_flow_store.go`
  - `apps/local-runner/internal/agentpack/pack_test.go`, new `*_dual_loop_test.go`
  - `supabase/migrations/*` (artifact seeding if not pack-embedded)
- **modules:** agent-pack loader/validator; flow executor; interactive service; artifact framework; desktop WorkflowsSettings (labels only)
- **database:** `workflows`/`workflow_steps`/`step_artifact_bindings`/`artifact_instances`/`artifact_types` (additive rows); no new tables; `workflows.acceptance_nodes_json` for new harnesses
- **external systems:** none; local file_artifact md outputs + Drive sync via `localFileSessionStore`

## 6. Data or Migration Steps

- **schema:** none required for engine; if `artifact_types` seeding needs migration, add `2026xxxxx_add_harness_artifacts.sql` with `is_builtin=true` rows (service role only).
- **data backfill:** `EnsureBuiltinFlowMirrorsWithStore` (def `supabase_workflow_flow_store.go:903`, call `internal/cli/root.go:167`) syncs new harnesses into `workflows` on next runner start; existing `rag-harness` rows keep `acceptance_nodes:[validate,synthesis,audit]` (no backfill on old runs).
- **config updates:** `task-harness` default `policy.cap:3` per loop (shared `cap:3` covers both via `forwardReachableNodeIDs` scoping); document bug/task/cp tier chooser in `/flow` help text.

## 7. Validation Plan

- **tests to add:**
  - `TestValidateFlowAllowsTwoContinueBackEdges` (plan loop + code loop both `when:continue kind:back` with different `From` passes).
  - `TestResolveContinueBackEdgeIsSourceAware` (given `plan_synthesis→plan_writer` and `validate→implement`, resolver with `from=plan_synthesis` returns `plan_writer`, with `from=synthesis` returns `implement`).
  - `TestTaskHarnessPackTopology` (12 nodes, 2 `continue/back`, `acceptance_nodes` includes `plan_synthesis`, `plan_writer` `file_artifact` bindings).
  - `TestCpHarnessPackTopology` (7 nodes, `task_splitter` INPUT `cp_md` OUTPUT `task_md[]`, 1 plan loop).
  - `TestTaskHarnessDualLoopReinvoke` (plan `continue` reuses `plan_writer` session; code `continue` reuses `implement` session; `context` keeps `DONE` not reset — `forwardReachableNodeIDs`).
  - `TestTaskHarnessPlanReviewBlocksWithoutArtifact` (no `file_artifact` md → `plan_synthesis` escalate).
  - `TestApplyFlowControlContinueHubRouting` (`plan_synthesis` continue re-enters `plan_writer`; `synthesis` continue re-enters `implement` via the `validate→implement` edge — engine-level, Task-304).
  - `TestCpHarnessSliceOnlyTerminal` (default stops after `task_splitter`+`audit`, `implement` never RUNNING) + `TestCpHarnessSmokeVariant` (variant B runs first-Task coding to `done`).
  - `TestHarnessArtifactBindingRejectsPathOutsideRequirements` (OUTPUT path escaping `requirements/` fails deterministically at binding validation).
  - Inventory: `pack_test.go` count 6→9 analog for new harnesses (like `Task-293` `rag_harness_tdd_review_pack_test.go`).
- **manual checks:**
  1. `/flow bug-harness` with `grok-4.5` still behaves as today (no regression, same `cap:3`).
  2. `/flow task-harness` — `plan_writer` writes `Task-xxx.md` with `P-*` + `Source Refs`, `test_signatures` writes empty signatures, `plan_synthesis` approves, then `implement→validate→reviewer→synthesis→audit` completes to `done` with `CP-58` as source doc.
  3. `/flow cp-harness` — `cp_plan_writer` writes `CP-5x.md`, `cp_review` approves, `task_splitter` writes `Task-30x.md ×2`, `audit` shows file_artifact outputs in artifact panel.
  4. Clone any harness in Desktop, edit `cap`, save — reload preserves `acceptance_nodes` (CP-55 P-1 regression).
- **failure cases:**
  - Plan reviewer `changes_requested` → only `plan_writer`/`test_signatures` reset to `PENDING` (`forwardReachableNodeIDs` from `plan_writer`), `context`/`freeze` stay `DONE` (BUG-286).
  - Second `continue/back` with same `From+When` still rejected by validator (duplicate).
  - `file_artifact` path outside workspace → deterministic fail at artifact binding (SD-23).

## 8. Rollout and Fallback

- **rollout order:** `P-1` engine (dual loop) → `P-2` task-harness → `P-4` artifact bindings → `P-3` cp-harness → `P-5` tests/docs. Each P independently shippable; `P-1` alone is safe (single-loop flows still pass).
- **fallback path:** all additive, harness-selectable. Keep `rag-harness` as fallback `bug-harness`. If dual-loop regresses, revert `task-harness`/`cp-harness` pack entries; engine's single-loop path (when caller passes empty `from`) falls back to first-match. No provider or Drive change.
- **monitoring:** `pack_test.go` full suite; `go test ./internal/runner -run DualLoop|TaskHarness|CpHarness`; TUI F2 step runtime reachability; artifact panel shows `plan_md` outputs.

## 9. Risks

- `R-1` **Dual-loop validator widening breaks existing flows.** Mit: `P-1` guard — only allow second loop when `From` differs; single-loop `rag-harness`/`review-loop` unconditionally pass; existing `rag_harness_live_continue_back_edge_test.go:81` keeps green.
- `R-2` **Loop crosstalk (plan `continue` resetting code nodes).** Mit: `forwardReachableNodeIDs` scoped to re-entry's forward closure (`flow_executor.go:936`); `context`/`freeze` not forward-reachable from `plan_writer` → stay `DONE` (BUG-286 fix).
- `R-3` **Artifact type proliferation.** Mit: reuse `file_artifact` only (SD-23 D-11); no new type DSL; instance config is path template only.
- `R-4` **Cost doubling (plan review + code review).** Mit: tiered harnesses — bug skips plan loop; Task pays +~2 turns but catches plan drift before code; CP pays splitter only, not full coding.
- `R-5` **Manual CP→Task bundle drift.** Mit: `cp-harness` enforces one `CP.md` → N `Task.md` mapping with `config.pathTemplate` and `P-*` traceability; drift caught by plan reviewer.
- `R-6` **Supabase mirror sync drops `acceptance_nodes`.** Mit: reuse CP-55 P-1 fix (`workflows.acceptance_nodes_json` + `SupabaseAdminRepository` carry); add harness rows with `acceptance_nodes` seeded.

## 10. Definition of Done

- [ ] `DOD-1` `ValidateFlowDefinition` accepts `task-harness`/`cp-harness` with two `continue/back` edges (different `From`) and still rejects same-`From` duplicate; `go test ./internal/agentpack -run ValidateFlow` green.
- [ ] `DOD-2` `resolveContinueBackEdgeTarget(edges, from...)` source-aware returns correct re-entry for each loop — hub continue routes by `activeHubNodeID` (`plan_synthesis`→`plan_writer`, `synthesis`→`implement` via the `validate→implement` edge); dual-loop reinvoke reuses `plan_writer` and `implement` sessions independently; `context` not reset — `TestTaskHarnessDualLoopReinvoke` + `TestApplyFlowControlContinueHubRouting` green.
- [ ] `DOD-3` `task-harness` 12-node builtin loads (`LoadBuiltinPack`), shows in `/flow` picker (`selectableIn:flow`), `cloneable:true`, `cap:3`, `acceptance_nodes` includes `plan_synthesis`; `bug-harness`/`rag-harness` byte-identical.
- [ ] `DOD-4` `task-harness` live: `plan_writer` writes `Task-*.md` file_artifact, `test_signatures` writes empty signatures, `plan_review` loop can reject and re-enter `plan_writer` with findings, then `implement→validate→reviewer→synthesis→audit` completes to `done`; artifact panel shows plan md.
- [ ] `DOD-5` `cp-harness` slice-only live (7-node default): `context` provides draft repo context, `cp_plan_writer` writes `CP-*.md`, `task_splitter` writes `Task-*.md ×N`, plan and slice reviews both pass before `audit`; no coding steps on the timeline (`TestCpHarnessSliceOnlyTerminal` green); `cp-harness-smoke` (13-node opt-in) runs first-Task coding to `done` (`TestCpHarnessSmokeVariant` green); manual `CP→Task` bundling no longer needed for new CPs.
- [ ] `DOD-6` No regression: `review-loop`/`context-coding-review-synthesis`/`rag-harness` flows unchanged, `go test ./internal/runner -run 'TestRAGHarnessLive|TestReviewLoop|TestContextCoding'` green, `domain_hardcode_guard` green.
- [ ] `DOD-7` Docs: `CP-58` approved, `artifact_types` seeded with `plan_md`/`cp_md`/`task_md` instances, `change-audit/CA-xxx-bug-task-cp-harness.md` with ledger block.

> **Task-260 integration note (2026-08-31, no code in this CP):** Chat Plan/Code auto-injects `safe-fix-contract` pointer (done in Task-260, `runner/chat_posture.go` + `interactive_service.go`); `r-additive-tests` (`pre_existing_test_edited` `reprompt`, `SD-20 §2.8`) is hub Chat only. Flow wiring — `preflight_contract_plan` / `plan_writer` / `plan_reviewer` / `cp_plan_writer` pointer (`promptTemplate: prompts/plan-safe-fix-contract.md` reuse or `SelectedSkills` equivalent) and coding-child `DocScopeRuleIDs` inclusion for `r-additive-tests` — is deferred to this CP-58 (no Flow code in Task-260). Task-260 is prerequisite.
