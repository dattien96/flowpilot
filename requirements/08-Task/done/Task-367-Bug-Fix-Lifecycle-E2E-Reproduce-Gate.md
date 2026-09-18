# Task-367: Bug-Fix Lifecycle E2E Validation With Reproduce Gate

## Metadata

- Document ID: `Task-367`
- Title: `Bug-Fix Lifecycle E2E Validation With Reproduce Gate`
- Phase: `task`
- Status: `done`
- Owner: `Claude Sonnet MAX`
- Reviewers: `Claude agent review`
- Created: `2026-09-15`
- Last Updated: `2026-09-16`
- Parent Documents: [CP-64 P-4](../../07-Coding-Plan/todo/CP-64-Reproduce-First-TDD-Gate.md)
- Child Documents: `None`
- Related Documents: [SS-14](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md), [SD-20](../../06-System-Tech-Design/SD-20-Flow-Gate-Rule-Semantics.md), [SP-06](../../04-System-Principle/SP-06-Oracle-Rule-And-Schema-First-Gate.md), [CP-62](../../07-Coding-Plan/done/CP-62-Zcode-Harness-Parity.md)
- Replaces: `None`
- Tags: `reproduce-first, e2e, bug-harness, red-green, fail-closed`
- Feature Keys: `reproduce-first-gate`

## AI Quick View

### Summary

- Slice cuối của CP-64: test E2E mô phỏng trọn vòng đời fix bug qua cổng `r-reproduce` — từ test ĐỎ (assertion) → coder fix production → test XANH → gate pass toàn bộ chuỗi.
- Kèm test fail-closed: khi bug KHÔNG được tái hiện (test Xanh ngay từ đầu, hoặc compile error), flow KHÔNG được phép chuyển sang node `implement` — Coder không bao giờ được chạm production code khi chưa có bằng chứng reproduce.
- Đây là bằng chứng Definition of Done của CP-64: cả 2 bug flow chạy mượt với quy trình mới, không gãy regression, additive tests green.

### Current Ask

- Implement P-4 theo CP-64 §4: viết E2E test đóng gói P-1 → P-2 → P-3. 2 test signatures cho sẵn phải xanh; toàn bộ DOD CP-64 phải tick được.

### Key Decisions

- `T-1` E2E dùng harness trong repo (mock provider turn, không gọi provider thật) theo pattern E2E hiện có của runner — mỗi turn được điều khiển bằng fixture: turn 1 reproduce viết test assertion-fail, turn 2 coder sửa production, validate chạy suite thật trong workspace tạm.
- `T-2` Fail-closed là bất biến số 1: mọi đường không tạo ra assertion failure (all-pass, compile error, test không chạy, EnvError) đều kết thúc ở reprompt của node `reproduce_test`, không bao giờ tiến tới `implement` với production write enabled.
- `T-3` Chuyển màu phải được chứng minh bằng oracle thật: run 1 (trước fix) `Failed` chứa test reproduce; run 2 (sau fix) suite green và test reproduce nằm trong `Passed` — không chấp nhận mock mờ (assert trên 2 state này).
- `T-4` Regression guard: 1 case chạy `task-harness` tương đương với `FLOWPILOT_ENABLE_REPRODUCE_GATE` bật để chứng minh flow tính năng mới KHÔNG bị gate chặn (CP-64 constraint "không làm gãy quy trình tạo tính năng mới").

### Constraints

- Additive tests only — không edit pre-existing tests; old test đỏ hoặc không compile → STOP, báo tên test + output, không sửa test, chờ user (R1 safe-fix-contract).
- Provider parity: Case-1 agnostic — E2E dùng mock provider turn + oracle thật, không nhánh `providerKey`; 1 representative provider đủ, ghi evidence grep.
- Prior CA: none (new feature `reproduce-first-gate`); đóng task phải kèm CA-NNN + tick toàn bộ DOD CP-64 §10.
- Không sửa production code của P-1→P-3 trừ khi E2E bóc ra bug thật (khi đó sửa trong slice tương ứng, không sửa băng qua ranh giới).
- E2E phải deterministic (không sleep-based, timeout reproduce 15s đã chốt ở Task-364 Q-1 áp dụng nhất quán).
- GitNexus impact analysis nếu phải đụng symbol của các slice trước.

### Open Questions

- None.

### Source Refs

- CP-64 §4 P-4, test signatures 10–11, §10 Definition of Done.
- `internal/runner/gate_hook.go` (runFlowGateAtEpoch), `internal/flowgate/` (Evaluate/precedence), `internal/agentpack/flow-pack/flows/bug-harness.yaml`.

## 1. Goal

E2E chứng minh quy trình Reproduce-First hoạt động trọn vẹn trên `bug-harness`: bug phải được tái hiện bằng test ĐỎ trước khi được sửa, sửa xong phải XANH, coder không thể sửa test, và bug không tái hiện được thì flow chặn đứng (fail-closed).

## 2. Parent Links

- coding plan: `CP-64-Reproduce-First-TDD-Gate.md` P-4
- tech design: `SD-20-Flow-Gate-Rule-Semantics.md`
- system spec: `SS-14-Code-Context-And-Regression-Safety.md`, `SP-06-Oracle-Rule-And-Schema-First-Gate.md`

## 3. Trigger

P-1 → P-2 → P-3 hoàn tất từng mảnh riêng lẻ; cần một bài test hội tụ end-to-end để chốt DOD CP-64 trước khi chuyển CP sang done.

## 4. Exact Change

- `T-1` **`TestBugFixLifecycleEndToEndWithReproduceGate`** (`apps/local-runner/internal/runner/bug_fix_reproduce_e2e_test.go` new, không sửa test cũ): fixture workspace Go với 1 bug thật (hàm trả sai giá trị); mô phỏng chuỗi node bug-harness — contract freeze → reproduce (agent turn viết test assertion-fail, gate pass, file test khóa ghi) → implement (coder sửa production, write vào file test bị deny) → validate (suite green, reproduce test chuyển ĐỎ→XANH) → gate sạch.
- `T-2` **`TestBugFixFailsClosedWhenBugNotReproduced`** (cùng file `bug_fix_reproduce_e2e_test.go`): 3 biến thể fail — test Xanh ngay từ đầu; test compile-lỗi; test không chạy được (EnvError) — khẳng định flow dừng ở node `reproduce_test` với reprompt, `implement` chưa được dispatch, production diff rỗng.
- `T-3` **Regression case** (thuộc trong T-1 hoặc test riêng): `task-harness` với flag bật → không có violation `r-reproduce`, `test_signatures` signature rỗng vẫn đi qua như cũ.
- `T-4` **Legacy fallback case** (CP-64 §8): chạy bug-harness với `FLOWPILOT_ENABLE_REPRODUCE_GATE` tắt → chuỗi chạy đúng cơ chế cũ — Tester viết khung signature rỗng, Coder tự điền assertion, không lock file test, gate `r-reproduce` không xuất hiện.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/runner/bug_fix_reproduce_e2e_test.go` (new)
- modules: `runner` (test only)
- routes: none
- tables: none

## 6. Acceptance Check

- [x] AC-1: Lifecycle ĐỎ→XANH hoàn chỉnh qua 2 node reproduce/implement với khóa read-only có hiệu lực (coder write vào file test bị deny).
- [x] AC-2: Fail-closed cho cả 3 biến thể (all-pass / compile error / EnvError) — không dispatch sang implement.
- [x] AC-3: Flow tính năng mới (`task-harness`) không bị gate chặn khi flag bật.
- [x] AC-4: 2 test signatures green:
  - `TestBugFixLifecycleEndToEndWithReproduceGate`
  - `TestBugFixFailsClosedWhenBugNotReproduced`
- [x] AC-5: Toàn bộ DOD CP-64 tick được (r-reproduce chặn đúng, compile error không tính reproduce, coder bị khóa file test, 2 bug flow chạy mượt, additive tests green).
- [x] AC-6: Flag tắt → E2E bug-harness chạy đúng legacy path CP-64 §8: signature rỗng, Coder tự điền, không lock file test, không violation `r-reproduce` (T-4).

## 7. Out of Scope

- Bổ sung rule/engine/prompt/flow mới — chỉ đóng gói những gì P-1→P-3 đã xây.
- Áp dụng reproduce gate cho `behavior-change` trong task-harness/vibe-sprint (follow-up sau CP-64 nếu cần).
- Perf/timeout tuning của oracle (Q-1 Task-364 đã chốt riêng).

## 8. Completion Notes

- result: done 2026-09-16 — 4 E2E green (`TestBugFixLifecycleEndToEndWithReproduceGate`, `TestBugFixFailsClosedWhenBugNotReproduced` ×3 subtests, `TestTaskHarnessSignatureTurnPassesWithReproduceGateOn`, `TestBugHarnessLegacyPathWithReproduceGateOff`), mock provider turns + REAL go toolchain/oracle, T-1→T-4 / AC-1→AC-6 ticked. Fixture: committed seed baseline, `commitBugFixTree` excludes `.flowpilot/`, debug instruments reverted.
- follow-ups: pre-existing reds outside CP-64 scope need owner triage before any full-suite DOD claim — `TestRun144900_*` (skill-drift baseline), `TestRootFlowEngineDefersCompletedUntilGate`, 2× `Test540927Oracle*` Windows `.sh` fixtures. R1 STOP: none touched.
- upstream docs updated: CA-872 (change-audit/CA-872-CP-64-Reproduce-First-TDD-Gate-Closeout.md); CP-64 moved to done with DOD §10 ticked.
