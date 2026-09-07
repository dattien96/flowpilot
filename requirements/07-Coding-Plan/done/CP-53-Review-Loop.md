# CP-53 Review Loop — Bịt các lỗ rò của Verifier Gate trong Flow-Coding

## Metadata

- Document ID: `CP-53`
- Title: `Review Loop — Bịt các lỗ rò của Verifier Gate trong Flow-Coding`
- Phase: `coding_plan`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `<chờ phân công>`
- Created: `2026-07-22`
- Last Updated: `2026-09-07`
- Parent Documents: [CP-35 (nguồn gốc flow gate, P-4/P-5)](../), [CP-51 (durable turn dispatch)](./CP-51-PhaseAB-Timeline-And-Verification-Log.md), [CP-50 / CP-43 (context sources)](../)
- Child Documents: [Task-272](../../08-Task/done/Task-272-CP53-Gate-Observability-Metrics.md) (P-6), [Task-273](../../08-Task/done/Task-273-CP53-Gate-Blind-Baseline-Fail-Closed.md) (P-1), [Task-274](../../08-Task/done/Task-274-CP53-Review-Loop-Done-Requires-Machine-Verdict.md) (P-2), [Task-275](../../08-Task/done/Task-275-CP53-Dogfood-Gate-Check-Hooks.md) (P-3), [Task-276](../../08-Task/done/Task-276-CP53-Waiver-Ledger-With-Expiry.md) (P-4), [Task-277](../../08-Task/done/Task-277-CP53-R-Newtest-Reprompt-Rule.md) (P-5), [CP-53-Test-Steps](../inprogress/CP-53-Test-Steps.md)
- Related Documents: `Task-155 (r-reg decision card), Task-156 (baseline), Task-223/225/242/247 (artifact + tier gates), BUG-288, BUG-289, SD-21 (change contract)`, [safe-fix-contract](../../../.agents/skills/safe-fix-contract/SKILL.md), [CP-43-52-53-54-note](../note/CP-43-52-53-54-note.md), [CP-61](../inprogress/CP-61-Harness-Done-Verdict-Gate.md) (takes over P-2)
- Replaces: `<không>`
- Tags: `flow-gate, review-loop, verifier, regression, flowgate, quality`

## AI Quick View

### Summary

- FlowPilot **đã có sẵn một verifier gate thật** (`internal/flowgate`): green-test baseline + regression oracle + `r-reg`/`r-tests` always-block, kèm flow maker/checker `review-loop`. Đây không phải plumbing — nó đúng là thứ bài tham khảo gọi là "trái tim của loop".
- Bug vẫn lọt **không phải vì thiếu gate, mà qua 5 lỗ rò cụ thể** — nơi gate degrade *mở* (im lặng cho qua) thay vì *đóng*, hoặc nơi không có test nào để gate kích hoạt.
- Lỗ nặng nhất: (H-1) baseline thiếu/hỏng làm tắt âm thầm việc phát hiện regression; (H-3) hành vi mới chưa có test chỉ được bảo vệ bởi LLM reviewer + một quyết định `done` do LLM đưa ra ("Ralph Wiggum loop" trong bài); (H-5) quá trình phát triển **chính FlowPilot** lại không chạy gate của FlowPilot.
- Gate chạy ở **mọi mode** (chat lẫn flow — đã xác nhận Q-1), nên 5 lỗ này là toàn bộ câu chuyện, không có vùng "thoát ngoài flow". Điều đó khiến H-1 (fail-open baseline) càng quan trọng vì đó là đường degrade dùng chung.
- CP này bịt H-1…H-5 và thêm observability (cost-per-accepted-change, block/override rate) để có thể tin được gate.
- Bài tham khảo (Kopadze, "Loops") chỉ dùng làm nguồn framing/thuật ngữ; nửa sau là quảng cáo vendor và không mang trọng số thiết kế.

### Current Ask

- **Closed 2026-09-07:** Task-272…277 filed `done` (CA-437…442). Harness leftover of P-2 → [CP-61](../inprogress/CP-61-Harness-Done-Verdict-Gate.md). See §12.
- Outcome cuối: flow-coding loop **fail closed** — regression hoặc "done" không kiểm chứng được phải **dừng turn**; repo FlowPilot tự dogfood oracle.

### Key Decisions

- `D-1` Coi "gate bị mù (không baseline / lỗi test env / red-at-capture)" là một **trạng thái block/cảnh báo lớn hạng nhất**, không phải green im lặng. Gate mù thì phải tuyên bố là nó đang mù.
- `D-2` Quyết định `synthesis → done` trong `review-loop` phải yêu cầu một **verdict reviewer kiểm bằng máy được** (PASS qua `submit-review-outcome`), không phải văn xuôi của synthesizer, để bịt lỗ hành vi chưa có test.
- `D-3` **Dogfood** oracle `flowgate` trên chính repo FlowPilot qua git `pre-commit` hook + Claude Code `Stop` hook, để build FlowPilot bị gate y hệt khi dùng FlowPilot.
- `D-4` Task hành-vi-mới phải mang một **test fail mới suy ra từ AC** trước khi coding gate cho qua (để oracle có "răng" trên code mà nó vốn không thấy được).
- `D-5` **Waiver override test được ghi thành nợ** (waiver ledger) và có hạn; một regression đã được waive không bao giờ được tồn tại âm thầm.
- `D-6` Dogfood gate dùng **2 baseline tách biệt** cho Go và TS, quyết định block độc lập (chốt Q-3): regression ở bộ nào chặn commit vì bộ đó, và lỗi env của một bộ không làm mù bộ kia.

### Constraints

- Go runner (`apps/local-runner`) + TS desktop (`apps/desktop-flowpilot`); dogfood gate phải baseline **hai** bộ test (`go test`, và desktop `vitest`/`tsc`).
- Không được làm yếu các hợp đồng fail-closed đã hardened trong BUG-288 (diff-observe, corrupt-baseline, contract-commit đều fail closed).
- Reviewer dùng chung dirty worktree của coder và zero-cost cho doc/scope rule (BUG-152) — thay đổi verdict không được làm reviewer phát sinh lại chi phí gate.
- Dev harness hiện chạy `defaultMode: bypassPermissions`, không có `.claude/hooks` — dogfood hook là điểm dừng cơ học duy nhất tồn tại ở đó.

### Open Questions

- `Q-A` Leak nào rò nhiều nhất trong thực tế? Chỉ trả lời được sau khi P-6 (observability) chạy — nên P-6 land trước.
- `Q-B` Reviewer nên chạy model/effort mạnh hơn coder ở mức nào (chọn cụ thể model cho reviewer trong `review-loop`)?
- `Q-C` Ai sở hữu và quy trình bảo trì `flaky-quarantine.json` (để "suite red at capture" được sửa, không bị dung túng)?

> Các câu hỏi mở ban đầu đã chốt: **Q-1** — gate chạy ở **mọi mode** (không chỉ flow); **Q-2** — P-6 land trước (đã đồng ý); **Q-3** — tách baseline Go/TS, block độc lập (xem `D-6`).

### Source Refs

- Code: `internal/flowgate/{enforce,baseline,oracle,rules}.go`, `internal/runner/{gate_hook,engine_gate_config}.go`, `flow-pack/flows/review-loop.yaml`.
- Upstream: CP-35 (gate), Task-155/156, Task-242 (tiered gate), BUG-288/289.
- External: Anatoli Kopadze, *"Loops explained: Claude, GPT, Mira and what actually works"*, X, 2026-06-20 — https://x.com/i/status/2068328135611822149 (chỉ dùng làm framing).

## 1. Goal

Giảm tỉ lệ flow-coding để lọt bug (đặc biệt là regression của các test trước đó đang xanh) tới một turn "done". Cách làm: bịt đúng những điểm mà verifier gate hiện có degrade *mở* thay vì *đóng*, bịt lỗ coverage cho hành vi chưa có test, và bắt chính quá trình phát triển FlowPilot phải chịu cùng cái gate mà nó ship cho user.

## 2. Input Documents

- [CP-51 — Durable Turn Dispatch & Recovery](../done/CP-51-PhaseAB-Timeline-And-Verification-Log.md) (vòng đời turn mà gate móc vào).
- CP-35 (nguồn gốc flow gate P-4/P-5), CP-50 / CP-43 (context sources nuôi coding agent).
- SD-21 (change contract, được `changecontract/infer.go` tham chiếu).
- Tham khảo ngoài: bài "Loops" của Kopadze (xem §3.1). Chỉ dùng framing/thuật ngữ.

## 3. Bối cảnh & Phân tích vấn đề

### 3.1 Bài viết đề cập gì (mục yêu cầu #1)

Bài ("Loops explained", A. Kopadze, X, 2026-06-20, ~19.4M views) là một bài explainer phổ thông về agentic loop. **Nửa đầu là phần có giá trị**; nửa sau là quảng bá cho một sản phẩm agent trên Telegram ("Mira" qua Composio) và **không mang trọng số kỹ thuật** cho CP này.

Các luận điểm có giá trị liên quan tới chúng ta:

- Một **loop** = DISCOVER → PLAN → EXECUTE → **VERIFY** → ITERATE, cùng một mục tiêu, một cách biết khi nào xong, và một luật dừng.
- **"Verify là trái tim của loop."** Không có kiểm tra thật thì "agent chỉ tự đồng ý với chính nó lặp đi lặp lại… model vừa làm việc chấm bài của chính nó thì quá dễ dãi".
- **State** giúp loop học (nhớ cái gì đã fail); **stop condition** giữ nó tỉnh táo (success HOẶC hard cap).
- Năm building block: ① automation/heartbeat, ② skill (chỉ dẫn tái dùng), ③ **sub-agent (tách người-làm khỏi người-kiểm)**, ④ connector (act, không chỉ suggest), ⑤ **verifier/gate** — "khối duy nhất quyết định loop giúp bạn hay chỉ đốt tiền. Mọi thứ còn lại là plumbing".
- Failure mode được đặt tên — **"Ralph Wiggum loop"** (G. Huntley): agent tự cho là xong quá sớm và thoát ra khi việc mới nửa chừng; "loop không crash, chúng âm thầm tính tiền bạn".
- Metric quan trọng: **cost per accepted change** (dưới ~50% accept rate thì loop tốn hơn phần nó trả lại).
- Thứ tự build: làm cho MỘT lần chạy tay ổn định → lưu thành skill → bọc vào loop (thêm gate + stop) → *rồi mới* schedule.

Rút ra cho FlowPilot: bài xác nhận kiến trúc của ta; nó cho thuật ngữ ("Ralph Wiggum", cost-per-accepted-change, maker/checker) để gọi tên các lỗ rò bên dưới. Nó **không** đề cập attribution (baseline-diff), fail-closed degradation, hay override erosion — mà đó lại chính là chỗ gap thật của ta.

### 3.2 Tình trạng FlowPilot — as-is (mục yêu cầu #2)

FlowPilot đã hiện thực block ③ và ⑤ của bài ở mức vượt cả bài:

| Block | Cơ chế FlowPilot | Bằng chứng |
|-------|------------------|------------|
| Verifier (⑤) | `flowgate` post-turn hook: quan sát turn-scoped git diff → chạy regression **oracle** đối chiếu **baseline** xanh → đánh giá rule set → block/reprompt/escalate | [gate_hook.go:101](apps/local-runner/internal/runner/gate_hook.go#L101) |
| Attribution | `test_baseline.json` (`green_tests`, `suite_passed`, `head_sha`) + per-test overrides; phân biệt `Regressed` thật với `Failed` có sẵn | [baseline.go:16](apps/local-runner/internal/flowgate/baseline.go#L16) |
| Regression = hard stop | `r-reg`/`r-tests` là **always-block**, không downgrade được kể cả khi `gate_mode=warn`; mode mặc định = `enforce` | [enforce.go:26](apps/local-runner/internal/flowgate/enforce.go#L26), [engine_gate_config.go:30](apps/local-runner/internal/runner/engine_gate_config.go#L30) |
| Maker/checker (③) | flow `review-loop`: coder → reviewer_correctness + reviewer_security (cohort) → synthesis → quay lại coder / done / escalate; cap=3, onCap=escalate | [review-loop.yaml](apps/local-runner/internal/agentpack/flow-pack/flows/review-loop.yaml) |
| Enforce oracle-rule | `r-tamper` cảnh báo khi một file test có sẵn bị sửa | [gate_hook.go:302](apps/local-runner/internal/runner/gate_hook.go#L302) |
| Hardening fail-closed | lỗi diff-observe, baseline corrupt, contract-commit fail đều block (BUG-288) | [gate_hook.go:133](apps/local-runner/internal/runner/gate_hook.go#L133) |
| Phạm vi | Gate chạy ở **mọi mode** (chat lẫn flow), không chỉ trong flow — xác nhận Q-1 | post-turn hook `runFlowGate` |

Tình trạng dev-harness (khi build chính FlowPilot): chỉ được quản bằng **skill mềm** (`oracle-rule`, `additive-tests-only`) dạng văn xuôi, với `defaultMode: bypassPermissions` và **không có `.claude/hooks`**. Cái gate cơ học ở trên **không** chạy trên commit của chính chúng ta.

### 3.3 Bug vẫn lọt ở đâu (các lỗ rò)

- `H-1` **Baseline degrade MỞ.** Oracle chỉ kích hoạt khi `Tests.Ran = baseline != nil && oracle.EnvError == ""`. Baseline **thiếu** trả về `(nil, nil)` (không phải error) → `Ran=false` → `r-reg`/`r-tests` không bao giờ được đánh giá → regression vô hình. (Baseline corrupt thì fail closed; *thiếu/không capture được* thì pass mở.) [gate_hook.go:170-238](apps/local-runner/internal/runner/gate_hook.go#L170)
- `H-2` **Baseline red/flaky lúc capture làm mù tín hiệu thô.** `suite_failed` chỉ kích hoạt khi `baseline.SuitePassed`. Nếu suite đang đỏ hoặc flaky lúc capture, regression toàn-suite không phát hiện được, chỉ còn regression theo từng test có tên là sống sót.
- `H-3` **Lỗ coverage (bề mặt Ralph Wiggum).** Oracle chỉ bắt được regression của test *đã tồn tại và từng xanh*. Hành vi mới chưa có test → không có gì đỏ → gate cho qua. Phòng thủ duy nhất là 2 LLM reviewer + **quyết định `done` của LLM synthesizer** — một người chấm dễ dãi tự tuyên bố đã xong.
- `H-4` **Override erosion.** Decision card của `r-reg` cho phép người dùng "accept the changed test"; override đã accept thì ngừng đếm (sticky-until-green). Đây là một cú click UI, nên các guard trên code-edit (`oracle-rule`, `additive-tests-only`) không bao phủ — một tín hiệu regression có thể bị waive vĩnh viễn dưới áp lực.
- `H-5` **Lỗ self-hosting.** Build FlowPilot chạy trên prose mềm + `bypassPermissions` + không hook; regression của chính FlowPilot không bị oracle của chính FlowPilot bắt. Ta không ăn dog food của chính mình.

## 4. Chiến lược triển khai (mục yêu cầu #3 — solution)

Cách tiếp cận tổng thể: **đừng build gate mới — làm cái đang có fail closed, mở rộng nó tới hành vi chưa có test, và bật nó lên chính mình.** Mỗi solution slice map 1:1 với một lỗ rò.

- Logic sắp thứ tự: land **P-6 (observability)** trước hoặc song song — không thể ưu tiên H-1 vs H-3 (Q-A) khi chưa biết lỗ nào rò nhiều nhất. Sau đó P-1 (H-1/H-2, đòn bẩy cao nhất, rẻ nhất) → P-2 (H-3) → P-3 (H-5 dogfood) → P-4 (H-4) → P-5 (răng cho TDD).
- Phụ thuộc: P-2 phụ thuộc tool structured `submit-review-outcome` mà `review-loop.yaml` đã tham chiếu. P-3 phụ thuộc việc capture baseline chạy được cho cả hai bộ go và ts (P-1 đụng cùng đường capture).

Map solution ↔ lỗ rò:

- `S-1 → H-1/H-2`: thêm trạng thái **`gate_blind`** tường minh. Thiếu baseline / `EnvError` / red-at-capture phải phát một event block (ở mode `enforce`) hoặc cảnh báo nổi bật — không bao giờ pass im lặng. Thêm điều kiện tiên quyết **health/freshness** cho baseline và một **flaky quarantine** được bảo trì để "suite red at capture" được sửa, không bị dung túng.
- `S-2 → H-3`: bắt `synthesis → done` phải có một **verdict reviewer kiểm bằng máy được** (PASS qua `submit-review-outcome`), không phải prose; chạy reviewer trên **model/effort mạnh hơn** coder (bất đối xứng maker/checker theo bài).
- `S-3 → H-5`: một `scripts/gate-check` chạy oracle `flowgate` đối chiếu baseline repo (go + ts), gắn vào git **pre-commit** hook và Claude Code **`Stop`** hook trong `.claude/settings.json`.
- `S-4 → H-4`: một **waiver ledger** (liên kết `change-audit`) ghi mọi override test đã accept kèm lý do + hạn; waiver hết hạn thì re-arm `r-reg`.
- `S-5 → test mới`: một rule reprompt **`r-newtest`** — Task đổi code nhưng không thêm test mới sẽ bị reprompt để thêm một test fail suy ra từ AC trước.

## 5. Work Breakdown

### 5.1 Phase → Task map (execution order)

| Order | Phase | Task | Hole | `feature_key` | Provider class (safe-fix R2) |
|------:|-------|------|------|---------------|--------------------------------|
| 1 | **P-6** Observability spike | [Task-272](../../08-Task/done/Task-272-CP53-Gate-Observability-Metrics.md) | Q-A | `context-regression-engine` | Agnostic (Go emit/log only) |
| 2 | **P-1** Baseline fail-closed / `gate_blind` | [Task-273](../../08-Task/done/Task-273-CP53-Gate-Blind-Baseline-Fail-Closed.md) | H-1, H-2 | `context-regression-engine` | Agnostic (post-turn `gate_hook`, all modes) |
| 3 | **P-2** Done requires machine verdict | [Task-274](../../08-Task/done/Task-274-CP53-Review-Loop-Done-Requires-Machine-Verdict.md) | H-3 | `agent-flow-engine` | Shared flow runtime — **matrix Claude+Codex+Grok** (fake adapters OK) |
| 4 | **P-3** Dogfood `gate-check` + hooks | [Task-275](../../08-Task/done/Task-275-CP53-Dogfood-Gate-Check-Hooks.md) | H-5 | `context-regression-engine` | Agnostic (scripts/hooks; Linux primary; Windows path noted) |
| 5 | **P-4** Waiver ledger + expiry | [Task-276](../../08-Task/done/Task-276-CP53-Waiver-Ledger-With-Expiry.md) | H-4 | `context-regression-engine` | Agnostic (override path shared) |
| 6 | **P-5** `r-newtest` reprompt | [Task-277](../../08-Task/done/Task-277-CP53-R-Newtest-Reprompt-Rule.md) | H-3 (coverage) | `context-regression-engine` | Agnostic (`flowgate` rule) |

### 5.2 Phase detail (unchanged intent)

- `P-1` **Baseline fail-closed (S-1).** Thêm phân loại `gate_blind` cho baseline thiếu / `EnvError` / red-at-capture; surface thành event block ở mode `enforce`; thêm baseline health + freshness check + `flaky-quarantine.json`. Files: `flowgate/{baseline,oracle}.go`, `runner/gate_hook.go`. **Không** nới BUG-288 corrupt-baseline fail-closed.
- `P-2` **Done gate bằng verdict (S-2).** Yêu cầu `submit-review-outcome` PASS trước `synthesis→done`; thêm cấu hình bất đối xứng model/effort của reviewer. Files: `flow-pack/flows/review-loop.yaml`, agent synthesizer, `runner/flow_*`.
- `P-3` **Dogfood gate (S-3).** `scripts/gate-check` (go + ts, hai baseline tách biệt theo `D-6`), `.git/hooks/pre-commit`, `Stop` hook trong `.claude/settings.json`, script bootstrap baseline.
- `P-4` **Waiver ledger (S-4).** Waiver override được lưu bền kèm lý do + hạn; re-arm khi hết hạn; surface các waiver đang mở.
- `P-5` **Rule test mới (S-5).** Rule reprompt `r-newtest` + defaults; prompt remediation test suy từ AC. **Chỉ yêu cầu ADD test mới** — không bao giờ yêu cầu edit/delete test cũ (oracle-rule / additive-tests-only).
- `P-6` **Spike observability (Q-A).** Phát metric gate: block rate, reprompt/escalate rate, override rate, và **cost-per-accepted-change**. Trả lời được lỗ nào rò nhiều nhất. Minimal spike — không UI dashboard full.

### 5.3 Safe-fix-contract (mandatory for every Task)

Companion: [safe-fix-contract](../../../.agents/skills/safe-fix-contract/SKILL.md).

| Rule | CP-53 enforcement |
|------|-------------------|
| **R1** Old suite green / untouched | Chỉ **ADD** `*_test.go` mới (`cp53_*`, `task27x_*`). Old test fail → **STOP**, report, không sửa assertion. Exception chỉ khi operator viết rõ allow. |
| **R2** Claude + Codex + Grok | P-1/P-3/P-4/P-5/P-6: chứng minh **provider-agnostic** (gate/scripts Go-owned). **P-2 bắt buộc** parameterized/fake-adapter matrix cả 3 provider. |
| **R3** Matrix coverage | Mỗi Task: happy path + near-miss + degraded (missing baseline / no verdict / expired waiver / env error). Không ship 1 happy-path unit. |
| **History** | Trước code: đọc CA gần nhất cho `context-regression-engine` / `agent-flow-engine` (BUG-288/289 fail-closed). **Will not undo:** corrupt-baseline block, `r-reg`/`r-tests` always-block, sticky-until-green override clear. |
| **Audit** | Mỗi Task done → 1 `CA-*` + commit `[Feature|BugFix|…][feature_key]`. |

### 5.4 Plan-review findings (2026-08-11)

| # | Finding | Resolution in Task cut |
|---|---------|------------------------|
| F-1 | `LoadBaseline` missing = `(nil,nil)` vs corrupt = error — H-1 chỉ thiếu, không đụng BUG-288 | Task-273: `gate_blind` **chỉ** missing / EnvError / red-at-capture; corrupt path unchanged |
| F-2 | `submit-review-outcome` đã có trong `review-loop.yaml` — thiếu enforce done | Task-274: hub/synthesizer gate, không rewrite tool schema trừ khi cần |
| F-3 | Dogfood trên Windows thiếu `sh` (adapter tests) | Task-275: Linux/CI primary; Windows = documented degrade / Git Bash; không claim full Windows dogfood |
| F-4 | `r-newtest` dễ xung đột oracle-rule | Task-277: chỉ detect **thiếu file test mới** trong turn diff; remediation = ADD test, never edit old |
| F-5 | P-6 full dashboard scope creep | Task-272: log/metric emit + local inspect; no desktop UI |
| F-6 | Cry-wolf (R-1) nếu `gate_blind` block mọi workspace mới | Task-273: first-turn auto-capture path vẫn được; blind chỉ khi capture skip/fail **và** code diff có production paths |
| F-7 | Task-277 draft từng viết "new/changed *_test.go" — mâu thuẫn oracle-rule | **Fixed 2nd review:** only **newly added** test files satisfy `r-newtest`; editing old tests never counts |
| F-8 | P-6 land trước ≠ chờ telemetry tuần mới làm P-1 | P-6 = instrument spike same sprint; P-1 proceeds immediately after 272 ships — Q-A refines later, does not block H-1 |
| F-9 | Task-276 có thể đụng desktop decision-card | Split in impl: Go ledger+re-arm mandatory; desktop reason UI additive TS tests if accept path is UI-owned — still no old-test edits |

## 6. Touched Areas

- files: `apps/local-runner/internal/flowgate/{enforce,baseline,oracle,rules}.go`, `apps/local-runner/internal/runner/{gate_hook,engine_gate_config}.go`, `apps/local-runner/internal/agentpack/flow-pack/flows/review-loop.yaml`, thêm `scripts/gate-check`, `.claude/settings.json`, `.git/hooks/pre-commit`.
- modules: `flowgate`, `runner` (post-turn hook của interactive service), flow definitions của `agentpack`.
- database: không.
- external systems: không (toàn bộ local; không phụ thuộc Supabase dispatch).

## 7. Data or Migration Steps

- schema: `flaky-quarantine.json` mới và schema waiver-ledger dưới `.flowpilot/settings/`; thêm field `gate_blind` vào event gate-violation.
- data backfill: bootstrap baseline repo (`test_baseline.json`) cho **hai bộ** go và ts riêng (P-3, `D-6`).
- config updates: `.claude/settings.json` thêm một `Stop` hook; không đổi mặc định `gate-config.json` phía user (giữ `enforce`).

## 8. Validation Plan

- test cần thêm: unit test `flowgate` cho `gate_blind` khi baseline thiếu/`EnvError`/red-at-capture; assert block ở mode `enforce` cho từng case; test rule `r-newtest`; test re-arm khi waiver hết hạn. (Chỉ additive — theo `additive-tests-only`.)
- kiểm thủ công: chạy `scripts/gate-check` trên một thay đổi cố tình gây regression và xác nhận pre-commit hook từ chối commit; xóa baseline và xác nhận `gate_blind` block chứ không im lặng cho qua.
- failure cases: baseline vắng; test env hỏng (lỗi compile trong test harness); suite đỏ lúc capture; reviewer không phát verdict; Task đổi code mà không có test mới; waiver hết hạn.

## 9. Rollout and Fallback

- thứ tự rollout: P-6 (đo) → P-1 (fail-closed, đòn bẩy cao nhất) → P-2 → P-3 (dogfood) → P-4 → P-5.
- đường fallback: mỗi rule nằm sau cơ chế enable-rule hiện có của `flowgate` và `gate_mode`; một rule mới bắn sai có thể tắt trong `flow-rules.json` mà không revert code. Dogfood hook có thể gỡ khỏi `.claude/settings.json` độc lập.
- monitoring: dashboard metric của P-6; theo dõi block/override rate để phát hiện hồi quy kiểu "cry-wolf" (gate block quá nhiều sẽ bị tắt — xem R-1).

## 10. Risks

- `R-1` **Cry-wolf → bị tắt.** Nếu `gate_blind` hoặc `r-newtest` bắn quá tay, người dùng chuyển sang `gate_mode=warn` và mất luôn cả bảo vệ regression always-block. Giảm thiểu: baseline-diff attribution vẫn là chuẩn; `gate_blind` phải thật sự hiếm (sửa suite flaky qua quarantine, không nới lỏng gate).
- `R-2` **Chi phí/độ trễ của verdict reviewer.** Bắt buộc verdict PASS + reviewer model mạnh hơn làm tăng cost mỗi loop (cảnh báo compounding-cost của bài). Giảm thiểu: coder giữ rẻ; chỉ reviewer escalate; đo cost-per-accepted-change (P-6).
- `R-3` **Ma sát dogfood.** Một gate pre-commit trên repo của chính ta có thể làm chậm team. Giảm thiểu: chạy selective nhanh ở inner loop, full suite chỉ ở gate; quarantine giữ baseline xanh.
- `R-4` **Baseline TS chưa sẵn sàng.** Bộ desktop TS có thể chưa capture baseline sạch được ngay (dep/env). Giảm thiểu: `D-6` tách baseline độc lập — Go gate vẫn hoạt động trong khi baseline TS đang được đưa về xanh; TS `gate_blind` cảnh báo lớn thay vì chặn cho tới khi baseline TS ổn định.

## 11. Definition of Done

### 11.1 Product DoD (all must be true)

- [ ] **P-6 / Task-272:** Metric gate (block / reprompt / escalate / override rate + cost-per-accepted-change) emit và inspect được local (log hoặc `.flowpilot/` artifact).
- [ ] **P-1 / Task-273:** Baseline thiếu / `EnvError` / red-at-capture → block `gate_blind` nhìn thấy ở mode `enforce` (test + manual xóa baseline) — **không** green im lặng. Corrupt baseline vẫn fail-closed như BUG-288.
- [ ] **P-2 / Task-274:** `review-loop` không tới `done` nếu chưa có verdict PASS kiểm bằng máy (`submit-review-outcome`). Matrix Claude+Codex+Grok xanh.
- [ ] **P-3 / Task-275:** `scripts/gate-check` từ chối commit regression test xanh trước đó, qua git pre-commit **và** Claude `Stop` hook (Linux/CI chứng minh; Windows documented).
- [ ] **P-4 / Task-276:** Override accept → waiver ledger (lý do + hạn); hết hạn → re-arm `r-reg`.
- [ ] **P-5 / Task-277:** Task đổi production code không có test mới → reprompt `r-newtest` (chỉ yêu cầu ADD test).
- [ ] Verification guide [CP-53-Test-Steps](./CP-53-Test-Steps.md) tick được (automated + manual) cho mọi phase.
- [ ] Mỗi Task có `CA-*` + không undo BUG-288/289 always-block contracts.

### 11.2 Safe-fix DoD (per Task, all required)

- [ ] Pre-existing tests **untouched** và **green** trên phạm vi liên quan (Linux/CI khi đụng spawn).
- [ ] New tests cover reported repro **và** matrix near-miss / degraded (R3).
- [ ] Provider class ghi trong CA: agnostic evidence **hoặc** Claude+Codex+Grok matrix (R2).
- [ ] Prior CA claims for feature không bị undo.
- [ ] Change-audit entry written với `feature_key` đúng registry.

### 11.3 Explicit non-goals (out of CP-53)

- Không rewrite CP-43/54/55 context packing.
- Không full desktop metrics dashboard.
- Không symbol-level test attribution mới.
- Không bắt buộc Windows native dogfood parity với Linux trong v1.

## 12. Closure note (2026-09-07)

- Task-272…277 filed `done` 2026-09-07 (CA-437…442). Review-loop `synthesis→done` = Task-274 / CA-439.
- Harness leftover of P-2 (3 hubs + reviewer asymmetry + test-steps §P-2) → [CP-61](../inprogress/CP-61-Harness-Done-Verdict-Gate.md). P-1 landed CA-757 (`plan_synthesis` / `synthesis` / `cp_synthesis`).
- Mọi contract fail-closed của CP này (BUG-288/289 always-block, corrupt-baseline block) giữ nguyên hiệu lực và được liệt kê trong will-not-undo của các CA tiếp theo.
