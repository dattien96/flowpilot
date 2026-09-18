# CP-64 Test Steps — Reproduce-First TDD Gate

## Metadata

- Document ID: `CP-64-TEST-STEPS`
- Title: `CP-64 Verification Steps (Automated + gate-sandbox Manual)`
- Phase: `verification`
- Status: `ready`
- Owner: `FlowPilot`
- Reviewers: `Operator`
- Created: `2026-09-16`
- Last Updated: `2026-09-17`
- Parent Documents: [CP-64: Reproduce-First TDD Gate](./CP-64-Reproduce-First-TDD-Gate.md)
- Child Documents: `None`
- Related Documents: [Task-364](../../08-Task/done/Task-364-Reproduce-Gate-Rule-FlowGate.md), [Task-365](../../08-Task/done/Task-365-Reproducer-Prompt-Agent-Behavior.md), [Task-366](../../08-Task/done/Task-366-Bug-Flows-Reproduce-Node-Test-Lock.md), [Task-367](../../08-Task/done/Task-367-Bug-Fix-Lifecycle-E2E-Reproduce-Gate.md), [safe-fix-contract](../../../.agents/skills/safe-fix-contract/SKILL.md)
- Replaces: `None`
- Tags: `reproduce-first-gate, tdd, oracle-rule, test-steps, verification, cp-64`
- Feature Keys: `reproduce-first-gate`

## AI Quick View

### Summary

- Danh mục kiểm thử tự động hóa và thủ công trên `gate-sandbox` cho toàn bộ CP-64 (P-1→P-4).
- Xác thực:
  1. Rule `r-reproduce`: chặn khi test xanh, chặn khi compile error, pass khi assertion failure (`P-1`).
  2. Prompt/agent/behavior `agent.reproduce` (`P-2`).
  3. Node `reproduce_test` trong `bug-harness`/`bug-plan-harness` + khóa read-only file test ở node coder (`P-3`).
  4. E2E đỏ→xanh fail-closed (`P-4`).
- Giường thử thủ công (Manual Bed): `gate-sandbox` (`/Users/tiendat/Desktop/BE/gate-sandbox`).

### Current Ask

- Chạy kiểm thử tự động xác nhận rule engine, topology, behavior và E2E lifecycle đều PASS.
- Dogfood thủ công trên 1 bug thật: quan sát RED bắt buộc → lock → GREEN.

### Key Decisions

- `V-1` **Compile error ≠ reproduce**: chỉ assertion failure mới pass gate (oracle classifier).
- `V-2` **Provider-agnostic**: grep `providerKey|ProviderKey` trên rule/oracle/lock/gate files = 0 hit (CA-872).
- `V-3` **Flag-off legacy**: `FLOWPILOT_ENABLE_REPRODUCE_GATE` tắt → về empty-signatures cũ, không gate không lock.

### Constraints

- Không sửa test cũ.
- Bed kiểm thử thủ công: `gate-sandbox`.

---

## 1. Goal

Chứng minh 100% bug fix có bằng chứng vật lý RED→GREEN: không test đỏ assertion thì không được chạm production code, và coder không được viết lại test để né lỗi.

---

## 2. Automated — run first

Thư mục làm việc: `apps/local-runner`.

```bash
# 1. Rule engine + oracle classifier (P-1)
go test ./internal/flowgate/ -run 'TestRuleReproduce|TestClassifySuite|TestReproduceRule' -count=1 -v

# 2. Topology + behavior + lock (P-2, P-3)
go test ./internal/agentpack/ -count=1
go test ./internal/changecontract/ -count=1
go test ./internal/runner/ -run 'TestBugHarnessTopology|TestBugPlanHarnessTopology|TestCoderNodeHasTestFileAsReadOnly|TestReproducer|TestReproduceBehavior' -count=1 -v

# 3. Lifecycle E2E red-green (P-4, mock provider + REAL go toolchain)
go test ./internal/runner/ -run 'TestBugFixLifecycleEndToEndWithReproduceGate|TestBugFixFailsClosedWhenBugNotReproduced|TestTaskHarnessSignatureTurnPassesWithReproduceGateOn|TestBugHarnessLegacyPathWithReproduceGateOff' -count=1 -v
```

| Step | Kiểm tra | Pass khi | Tick |
|---|---|---|---|
| 2.1 | Rule core (`P-1`) | `TestRuleReproduceFailsWhenAllTestsPass`, `TestRuleReproduceFailsOnCompileError`, `TestRuleReproducePassesOnAssertionFailure`, `TestRuleReproduceSkipsOnNonBugFlow` green | [x] PASS 2026-09-16 |
| 2.2 | Classifier + wiring (`P-1`) | `TestClassifySuiteOutputPerRunner`, `TestReproduceRuleOptInAndTestRuleSuppression` green | [x] PASS 2026-09-16 |
| 2.3 | Prompt/agent/behavior (`P-2`) | `TestReproducerAgentPromptRender`, `TestReproducerArtifactBinding`, `TestReproduceBehaviorRuntimeBinding` green | [x] PASS 2026-09-16 |
| 2.4 | Topology + lock (`P-3`) | `TestBugHarnessTopologyContainsReproduceGate`, `TestBugPlanHarnessTopologyContainsReproduceGate`, `TestCoderNodeHasTestFileAsReadOnly` green | [x] PASS 2026-09-16 |
| 2.5 | E2E red→green (`P-4`) | `TestBugFixLifecycleEndToEndWithReproduceGate` green (RED proof → lock → coder-deny → fix → GREEN, zero regression) | [x] PASS 2026-09-16 |
| 2.6 | E2E fail-closed (`P-4`) | `TestBugFixFailsClosedWhenBugNotReproduced` green (xanh-ngay/compile-error/bogus-binary đều reprompt, implement không dispatch, diff production rỗng) | [x] PASS 2026-09-16 |
| 2.7 | Compat (`P-4`) | `TestTaskHarnessSignatureTurnPassesWithReproduceGateOn`, `TestBugHarnessLegacyPathWithReproduceGateOff` green | [x] PASS 2026-09-16 |

---

## 3. Manual prep — project gate-sandbox

| # | Việc | Cách kiểm | Tick |
|---|---|---|---|
| P1 | Sandbox tồn tại | Thư mục `/Users/tiendat/Desktop/BE/gate-sandbox` | [x] PASS 2026-09-17; dirty worktree preserved |
| P2 | Runner biên dịch | `cd apps/local-runner && go build ./...` thành công | [x] PASS 2026-09-17; current checkout only, not a running-binary freshness check |
| P3 | Bật gate | `FLOWPILOT_ENABLE_REPRODUCE_GATE=1` trong env chạy flow | [x] PASS 2026-09-17; PID 67754 / :18754 env confirmed; sandbox gateMode=enforce |
| P4 | Seed 1 bug thật | 1 hàm sai logic kèm test đúng (chưa chạy) trong sandbox | [x] PASS 2026-09-18 (BUG-005 Square n+n vs n*n) |
| P5 | Provider | Mọi manual run có agent turn trên máy này: **Grok only**, model **grok-4.5** | [x] PASS 2026-09-18 (live runs run-250638, run-252241) |

---

## 4. Manual Verification Steps on gate-sandbox

### Kịch bản M-1: Bug thật đi trọn RED → lock → GREEN

1. Chạy flow `bug-harness` với bug đã seed (P4).
2. **Quan sát node `reproduce_test`**: agent viết test mới, runner chạy → gate `r-reproduce` PASS chỉ khi test fail bằng assertion (log trigger `reproduce_not_demonstrated` biến mất).
3. **Quan sát node `implement`**: thử bảo coder sửa file test vừa tạo → request bị silent-deny (test nằm trong `ReadOnlyPaths`).
4. Coder sửa production → suite GREEN → gate pass → audit done.

### Kịch bản M-2: Fail-closed khi không tái hiện được

1. Seed bug mà test viết ra **pass ngay** (green-on-arrival).
2. **Quan sát**: gate reprompt "chưa tái hiện được bug", node `implement` không bao giờ dispatch, diff production rỗng.
3. Lặp với test **lỗi cú pháp** → reprompt "sửa cho compile được" (không tính là reproduce).

### Kịch bản M-3: Flag-off về legacy

1. Tắt `FLOWPILOT_ENABLE_REPRODUCE_GATE`, chạy lại `bug-harness`.
2. **Quan sát**: node reproduce degrade về empty-signatures cũ, không gate, không lock — flow chạy như trước CP-64.

---

## 5. Log Grep (Bằng chứng Kiểm toán)

```text
r-reproduce
reproduce_not_demonstrated
ReadOnlyPaths
reproduceLockCommandTargetsLockedPath
```

---

## 6. CP-64 Verification Complete When

- [x] §2 Automated chạy xanh 100% (2026-09-16: flowgate + agentpack + changecontract + runner E2E).
- [x] **DONE — oracle cancellation race regression:** `TestRunOracleContextCanceledIsNotRegression` PASS sau sửa `executeSuite`; toàn bộ `internal/flowgate` PASS với `go test -race -count=1 -timeout=120s ./internal/flowgate` (exit 0, 6.603s), log `/tmp/cp-flowgate-final-race.log`.
- [ ] **Overall regression closeout:** full local-runner `-race` trước đó FAIL ngoài flowgate; package flowgate xanh không đồng nghĩa toàn repo xanh. Không nâng toàn bộ CP-64 thành DONE.
- [x] M-1: **DONE — live 2026-09-18 (Windows, run-250638, Grok/grok-4.5, bug-harness on D:\working\gate-sandbox)**
  - [x] **DONE — RED→lock→GREEN live:** run-250638; reproducer generated `square_test.go` asserting `Square(3)==9`, executed `go test` and gave genuine RED assertion failure (`Square(3) = 6, want 9`).
  - [x] **DONE — lock enforced:** runner logged `[gate] reproduce-first: locked [D:/working/gate-sandbox/square_test.go] read-only for coder step "implement" (contract v2)`; coder observed lock, left `square_test.go` untouched, and modified only production `calc.go` (`return n * n`).
  - [x] **DONE — GREEN validation & autonomous audit completion:** validation suite ran green (`PASS: TestSquare_`), reviewer called `submit_review_outcome` with `status: approved`, synthesis completed, and audit reached `DONE` with `change-audit/CA-951` and `BUG-005` in place without requiring operator unblock.
  - [x] **DONE — green-on-arrival fail-closed live (macOS & Windows):**
    - macOS `run-761459`: runner reject suite GREEN và reprompt; implement không dispatch, production không đổi.
    - Windows `run-252241` (2026-09-18, Grok/grok-4.5, `D:\working\gate-sandbox`): reproducer wrote `add_reproduce_test.go` asserting `Add(2,3)==5`, `go test` returned exit 0; gate `r-reproduce` rejected suite GREEN and reprompted (`Reproduce-first gate: the suite passed, so the bug was not reproduced...`), reproducer asked user via `flowpilot__ask_user` (`q-253146`), answered via API `POST /client/questions/q-253146/answer`, `implement` remained PENDING throughout, production code `calc.go` had 0 changes (diff empty).
  - [ ] **PARTIAL — compile-error live 2026-09-18 (×2):** (1) `run-706448`/`run-706612` bed `cp64-compile`; (2) `run-712781`/`run-712938` bed `cp64-compile2`. Both: syntax-broken `manual_sum_test.go` still fails `go test` with `missing ',' in parameter list` / `[setup failed]` on disk; `implement` stayed **PENDING**; production untouched. Gate fired `child gate reprompt` but text was **GREEN** ("the suite passed…") not compile-specific — misclassification reproduced. **Not DONE until live log shows compile-error reprompt wording.** Automated `compile_error` subtest remains classifier evidence.
- [x] M-3: **DONE — live 2026-09-17 (macOS, flag-off legacy)** — run-837439 (runner không flag, `:18767`, bed `/Users/tiendat/fp-beds/cp64m3`) hoàn tất `completed`, audit=DONE: node reproduce chạy bằng LEGACY tester + `test-signatures` prompt, KHÔNG có dòng `reproduce-first: locked`, KHÔNG lock, flow chạy như trước CP-64. Chi tiết ở §9.
- [x] Historical §7 audit only: no production/test edits, sandbox fixtures, commits, or run resumes were performed during that bounded audit. This does **not** describe §9, which baselined/committed sandbox WIP, created fixtures and operator documents, and resumed verification runs.
- [ ] M-1 sub-step "coder thử viết vào locked test và bị silent-deny" chưa induce live (coder tuân thủ lock và báo cáo "not edited" trong handoff) — lock được chứng minh bằng dòng `reproduce-first: locked` + nội dung test byte-identical RED→GREEN + automated `TestCoderNodeHasTestFileAsReadOnly`.

## 7. Bounded verification audit — 2026-09-17

**Overall: PARTIAL; live closeout BLOCKED. New live runIDs: none. New agent turns: 0/4.**

### Proven configuration / historical API evidence

- Current checkout HEAD: `30c31a80c927e5eaf0512c7e2e3bf8e817fba352`.
- `/health` reports both runners online, version `dev`, workspace `/Users/tiendat/Desktop/flowpilot/flowpilot`. Sandbox project ID `db51ec26-1a0f-4b92-8ceb-b03dc8e9b363` resolves to `/Users/tiendat/Desktop/BE/gate-sandbox`; its catalog default model is `gpt-5.4-mini`, so a future run must explicitly select Grok and `grok-4.5` rather than inherit that default.
- Process environment: :18754 PID 67754 has `FLOWPILOT_ENABLE_REPRODUCE_GATE=1`; :18753 PID 50739 has no such variable. Both sandbox-scoped `GET /client/projects/{projectId}/engine/gate-config` responses return `{"gateMode":"enforce"}`.
- Gate-on executable currently at `/var/folders/rj/tly4yvwx29gfjcd6cjfr2f4m0000gn/T/opencode/flowpilot` reports build revision `617709bf9ebfa1074963f32fd0b8759cdb9dc38d`, `vcs.modified=true`. This on-disk metadata does not prove the loaded process matches the current checkout. Legacy executable `/var/folders/rj/tly4yvwx29gfjcd6cjfr2f4m0000gn/T/go-build853567022/b001/exe/flowpilot` is absent. Neither runner was restarted.
- Read-only API audit used `GET /client/workflow-runs/run-704076` and `/agent-graph`; observed IDs/statuses are recorded above. No POST, simulated event injection, provider session creation, or agent invocation was performed.
- Inspected `StartRunInput` / `TurnInput` in `/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner/internal/runner/provider_event.go`: no explicit hard total-agent-turn budget. Built-in bug flow policy `cap: 3` is not proof of a four-total-turn limit across planner, reproducer, retries, coder and reviewer. No bounded live vehicle was established, so autonomous scenarios were not launched.

### Validation actually executed on current checkout

| Check | Result / scope |
|---|---|
| `go build ./...` in `/Users/tiendat/Desktop/flowpilot/flowpilot/apps/local-runner` | PASS (exit 0); not proof that listening processes use this build |
| Focused flowgate rule/classifier tests from §2 | PASS; 0.361s |
| Four lifecycle tests from §2 | PASS; 4.546s. `TestBugFixFailsClosedWhenBugNotReproduced` passes `green_on_arrival`, `compile_error`, `suite_never_ran`; red→green, task compatibility and flag-off legacy also pass |
| `go test ./... -count=1` in `/Users/tiendat/Desktop/BE/gate-sandbox` | PASS: `gatesandbox`, `gatesandbox/stringutil`, `gatesandbox/textutil` |

Lifecycle tests use controlled/mock provider events with the real Go toolchain. Their test-local IDs such as `repro-turn-green` are **not live runIDs**. Topology/full-package results in §2 remain historical 2026-09-16 claims; those broader suites were not rerun in this audit.

Local evidence files (temporary, not committed):
- `/tmp/cp64_verify_api_20260917.json` — timestamped GET responses and zero-new-run/turn declaration.
- `/tmp/cp64_verify_lifecycle_20260917.log` — exact lifecycle test output.
- `/tmp/cp64_verify_rule_20260917.log` — rule/classifier output.
- `/tmp/cp64_verify_build_20260917.log` — build output (empty on successful exit 0).

## 8. Windows re-verification — 2026-09-17 & 2026-09-18 (this machine)

- §2 rerun on Windows (go 1.26.2, updated 2026-09-18): flowgate rule/classifier 6/6 PASS (incl. 11 classifier subtests); agentpack reproducer scope 4/4 PASS; changecontract full PASS with 1 SKIP (`TestNormalizeDeclaredCodePathsRejectsSymlinkEscape` — needs admin symlink privilege, not claimed PASS); runner topology/lock scope 4/4 PASS (incl. 7 lock subtests); lifecycle E2E 4/4 PASS (`TestBugFixLifecycleEndToEndWithReproduceGate` 17.20s PASS, `TestBugFixFailsClosedWhenBugNotReproduced` 6.84s PASS across all 3 subtests: `green_on_arrival`, `compile_error`, `suite_never_ran`); `go build ./...` PASS; `go vet` PASS.
- Lifecycle tests use controlled/mock provider events with the real Go toolchain — not live provider proof. No new live `bug-harness` flow was started on this machine; M-1/M-2/M-3 stay PARTIAL/BLOCKED as above. Mock IDs such as `repro-turn-green` are not live runIDs.
- Sandbox `D:\working\gate-sandbox`: `go test ./...` PASS; no seed bug added, no fixtures created by this verification.

### Live M-1 attempt 2026-09-17 (this machine, run-244543, Grok/grok-4.5, bug-harness) — BLOCKED before r-reproduce

- Seed (P4): `manual_sum.go` (`ManualSum` returns `a-b`) + `manual_sum_test.go` (expects 5), unrun. RED confirmed live by direct `go test`: `ManualSum(2, 3) = -1, want 5`. Seeds removed after the attempt; `go test .` GREEN.
- Vehicle fix (worth reusing): raw `POST /client/workflow-runs/{id}/turns` 502s with empty body unless StartRun carries the TUI-equivalent `projectId` + `chatMode: normal_chat`; builtin flows mount via first-turn `flowRef` + `subMode: bug` + `changeType: bugfix` (mirrors `LaunchArm`). Proven by `PING-OK` (run-242528) before the flow runs.
- Flow behavior: contract-planner `run-244548` did real investigation (921 events): wrote `BUG-004-manual-sum-wrong-operator.md`, fought `r-dod-present` on absolute `WrittenPaths` (question `q-245878`, answered "retry relative path" via API, resolved), then parked on a HARD block with no operator recourse: `flow_gate_violation status=block`, no gate options/questions/approvals — "code changed but no change-audit note; no declared Change Contract; 5 DoD items incomplete" (fired on the uncommitted seed files). `r-reproduce` node never reached; no RED→GREEN, no lock, no coder denial observed live.
- Lesson: live M-1 needs the seed committed (or otherwise baselined) so pre-flow gates don't fire on the fixture itself; the macOS run-704076 hit the analogous wall (parent `running`/`blocked`). Automated E2E remains the passing evidence.

## 9. Full live closeout — 2026-09-17 (macOS, operator)

Vehicle (reusable): fresh runner binaries built from current checkout (`/tmp/fp-verify-runner`, `/tmp/fp-verify-runner-lspfix` — no code diff between them at build time for CP-64), each on its own port with its own projectId (unique dispatch shard under `<workspace>/.flowpilot/chats/<projectId>/` — the earlier BLOCK was a stale `dispatch.lock` on the shared `db51ec26` shard held by another runner; the project catalog is Supabase-backed so an arbitrary projectId works with explicit `cwd`). `just chat-dev` TUI was not usable (interactive input stalls); all driving was raw HTTP: `POST /client/workflow-runs` (projectId + providerKey=grok + model=grok-4.5 + chatMode=normal_chat + workingMode=dev + cwd + yoloMode=true) then `POST /client/workflow-runs/{id}/turns` (stepId + prompt + flowRef=bug-harness + subMode=bug + changeType=bugfix). Operational discovery: flow beds must NOT live under `/tmp` on macOS (symlink `/tmp→/private/tmp` trips `changecontract: declared path ... resolves outside workspace via symlink` at freeze); beds moved to `/Users/tiendat/fp-beds/`.

### M-1 RED→lock→GREEN (run-759392, flag ON, sandbox)

- Prep: sandbox baselined + committed (`1f9fb2b` baseline WIP, `a5c9d9c` seed `manual_sum.go` `a-b`, `ed8247e` removal of stale `CA-924` referencing deleted fixtures). Suite GREEN at start (no ManualSum test).
- Contract frozen (`frozen_contracts.ndjson`): feature_key `calc-core`, intent fix ManualSum a-b→a+b, declared_paths `[manual_sum.go, manual_sum_test.go]`, source_doc_id `BUG-MANUAL-SUM-001`, base_sha=ed8247e.
- RED: reproducer `run-759603` wrote `manual_sum_test.go` (untracked) asserting `ManualSum(2,3)==5`; gate ran `go test -v .` 21:14:32–21:14:34 exit 1 (assertion failures — independent direct proof `/tmp/cp64-m1-red.log`).
- LOCK: log L1819 `2026/09/17 21:14:34 [gate] reproduce-first: locked [/Users/tiendat/Desktop/BE/gate-sandbox/manual_sum_test.go] read-only for coder step "implement" (contract v2)` (also `/tmp/cp64-m1-lock.log`). Coder prompt carried the lock instruction; coder handoff: `manual_sum_test.go`: **not edited** (CP-64 reproduce-first lock).
- GREEN: coder `run-759949` changed `manual_sum.go` only (`return a+b`); gate suite 21:15:33 exit 0; reviewer `run-760390` → `approved`; synthesis → audit.
- Audit gate fired the **r-bug wall** the earlier audit also hit: `21:20:15 [flow-executor] audit tier-3 gate: Flow gate: declared bug mode but no bugfix document found` (no `requirements/09-BugFix/BUG-*.md` was created by the flow; loop parked with no question/approval — operator recourse was `POST /agent-loop/continue`).
- Operator recourse (documented, not hidden): authored `requirements/09-BugFix/BUG-MANUAL-SUM-001-manual-sum-wrong-operator.md` per FORMAT-REFERENCE, then `/agent-loop/continue` with feedback; next gate `21:28:37` demanded a DoD checklist (r-dod-present) → added `## Definition of Done` with `[x]` boxes → continue → **audit DONE, run `completed`** (final gate `21:30:26 violations=0`).
- Note: reproduce-vs-frozen binding "reproduce node writes its OWN new test file outside the frozen draft" held (contract only declared the paths; the lock, not scope drift, governed).

### M-2 fail-closed green-on-arrival (run-761459, flag ON, bed `/Users/tiendat/fp-beds/cp64m2`)

- Bed: ManualSum already correct (`a+b`), tree clean, suite green. Prompt reported the (false) BUG-MANUAL-SUM-002 claim.
- Reproducer correctly smelled the false report and asked the user (`q-761882`): "manual_sum.go already returns plain a+b... Production must stay untouched. How should I proceed?" — operator answered "write the additive matrix test anyway (will be GREEN — expect gate rejection)".
- Gate ran `go test -v .` 21:29:34–35 `err=<nil>` (GREEN) → **r-reproduce REJECTED**: reprompt text logged — "Reproduce-first gate: the suite passed, so the bug was not reproduced — add an assertion that fails against the current (unfixed) code... The gate only passes when the suite COMPILES and the new test FAILS on its assertion" — reprompt ×3 total, second question `q-762420`, third `q-762757`, then `implement` **PENDING for the whole run** (never dispatched), `manual_sum.go` untouched, closure doc `CA-924-calc-core-manual-sum-not-reproducible.md`.
- Compile-error leg was NOT induced (the reproducer never wrote a syntactically broken test).

### M-3 flag-off legacy (run-837439, NO flag, bed `/Users/tiendat/fp-beds/cp64m3`)

- Same a-b seed; runner WITHOUT `FLOWPILOT_ENABLE_REPRODUCE_GATE`, `:18767`, log `/tmp/fp-r-legacy.log`.
- Reproduce node ran the **legacy** path: child `run-837619` agent=**tester** (not reproducer), `[FlowPilot test-signatures step]` (L1088/L1142), legacy tester wrote `manual_sum_test.go` empty signatures, coder filled them (`return a+b`); **`reproduce-first: locked` count in the whole log = 0**; `agent.reproduce`/`Reproduce-First` only appear inside model chatter lines (2 grok-acp echoes), never as a gate/spawn line.
- Same r-bug + r-dod-present audit wall as M-1 → same operator recourse (BUG-MANUAL-SUM-003 doc + DoD + continue) → **audit DONE, run `completed`**.

## 10. macOS live follow-up — 2026-09-18 (compile-error attempt)

- Vehicle: existing `:18754` (`FLOWPILOT_ENABLE_REPRODUCE_GATE=1`), API-driven, `providerKey=grok`, `model=grok-4.5`, `yoloMode=true`, unique projectId `cpv-cp64c-*`, cwd `/Users/tiendat/fp-beds/cp64-compile` (clone of cp64m2 with committed syntax-broken `manual_sum_test.go`).
- Parent `run-706448` mounted bug-harness; child reproducer `run-706612`. After operator chose "leave compile-broken test untouched", gate ran `go test -v .` and issued `child gate reprompt` + follow-up turn-707160.
- **Observed gap:** reprompt body used green-on-arrival wording ("the suite passed…") while the bed still failed to compile. Agent surfaced this in `q-707255`. `implement` never left PENDING; production `manual_sum.go` unchanged.
- Operator closed with stop/not-reproducible. Compile-error live wording remains open; fail-closed (no implement) holds for this run.

## 11. Windows live M-1 closeout — 2026-09-18 (run-250638, Grok/grok-4.5)

- **Target Sandbox**: `D:\working\gate-sandbox` (`db51ec26-1a0f-4b92-8ceb-b03dc8e9b363`), runner `:4317`.
- **Target Bug**: `BUG-005` (`Square(n)` returned `n+n` instead of `n*n`).
- **Full Node Execution**:
  1. `preflight_contract_plan` (DONE) & `preflight_contract_freeze` (DONE): Frozen contract for `calc-core`, declared paths `[calc.go, square_test.go]`.
  2. `context` (DONE): Produced context package.
  3. `reproduce_test` (DONE): Grok reproducer wrote `square_test.go` asserting `Square(3) == 9`. Runner executed `go test` and observed genuine **RED assertion failure** (`FAIL: Square(3) = 6, want 9`).
  4. Gate `r-reproduce`: Evaluated test failure, confirmed assertion error, passed, and locked test file read-only: `[gate] reproduce-first: locked [D:/working/gate-sandbox/square_test.go] read-only for coder step "implement" (contract v2)`.
  5. `implement` (DONE): Coder prompt received read-only lock instructions. Coder explicitly checked the test file, confirmed it was locked, left it completely untouched, and modified only production `calc.go` (`return n * n`).
  6. `validate` (DONE): Validation command executed `go test` -> **PASS (GREEN)** (`PASS: TestSquare_`).
  7. `reviewer` (DONE): Full test suite executed, reviewed diff, confirmed locked test untouched, called `submit_review_outcome` with `status: approved`.
  8. `synthesis` (DONE): Consolidated reviewer outcome.
  9. `audit` (DONE): Verified `change-audit/CA-951-calc-core-square.md` and `BUG-005`, all 9 steps completed autonomously.
- **Evidence**:
  - Direct logs show lock line: `[gate] reproduce-first: locked [D:/working/gate-sandbox/square_test.go] read-only for coder step "implement" (contract v2)`.
  - Target sandbox test suite remains 100% GREEN.

## 12. Windows live M-2 green-on-arrival fail-closed — 2026-09-18 (run-252241, Grok/grok-4.5)

- **Target Sandbox**: `D:\working\gate-sandbox` (`db51ec26-1a0f-4b92-8ceb-b03dc8e9b363`), runner `:4317`.
- **Target Bug**: `BUG-006` (`Add(2, 3)` reported broken, but `calc.go` implements `return a + b` which already works).
- **Execution & Observations**:
  1. `preflight_contract_plan` (DONE) & `preflight_contract_freeze` (DONE) & `context` (DONE).
  2. `reproduce_test` (RUNNING): Reproducer generated `add_reproduce_test.go` with 5 real assertions (`Add(2, 3) == 5`, edge cases `(0, 0)`, `(-1, 1)`, `(-2, -3)`, `(1, 0)`).
  3. Gate ran `go test -v -run "TestAddReproduce" ./...` and `go test -count=1 ./...`: both exited with code 0 (GREEN ON ARRIVAL).
  4. Gate `r-reproduce` fired: rejected the suite with reprompt:
     `Reproduce-first gate: the suite passed, so the bug was not reproduced — add an assertion that fails against the current (unfixed) code... The gate only passes when the suite COMPILES and the new test FAILS on its assertion`.
  5. Reproducer evaluated tension and called `flowpilot__ask_user` with question `q-253146`:
     "BUG-006 is a false alarm: Add already returns a+b, so add_reproduce_test.go asserting Add(2,3)==5 (and edge cases) is green. The reproduce-first gate demands a failing assertion, but forcing a red test would mean asserting incorrect expected values or breaking production code — both forbidden. How should this turn proceed?"
  6. Operator answered via API `POST /client/questions/q-253146/answer`:
     `{"choice": "Keep the green reproduce test and treat this as fail-closed / bug-not-reproduced (Recommended — matches BUG-006)"}` -> `accepted`.
  7. Reproducer summarized: `"Fail-closed: Add already correct; suite stays green"`.
  8. `implement` remained **PENDING** throughout the entire run.
  9. Production code check: `git diff calc.go` was empty (0 lines changed).
- **Conclusion**: Proves complete fail-closed defense against false-alarm bug fixes live on Windows: coder is never permitted to touch production code without physical proof of a failing assertion.

(End of file)
