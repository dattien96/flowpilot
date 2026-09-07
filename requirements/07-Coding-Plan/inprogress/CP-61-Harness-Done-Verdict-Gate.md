# CP-61 Harness Done-Verdict Gate — Bịt lỗ H-3 trên các harness hub

## Metadata

- Document ID: `CP-61`
- Title: `Harness Done-Verdict Gate — bịt lỗ H-3 trên các harness hub`
- Phase: `coding_plan`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `<chờ phân công>`
- Created: `2026-09-07`
- Last Updated: `2026-09-08`
- Parent Documents: [CP-53](../done/CP-53-Review-Loop.md) (H-3, S-2, D-2), [Task-274](../../08-Task/done/Task-274-CP53-Review-Loop-Done-Requires-Machine-Verdict.md) (review-loop landed CA-439; harness leftover is this CP)
- Child Documents: [CP-61-Test-Steps](./CP-61-Test-Steps.md)
- Related Documents: [CA-755](../../../change-audit/CA-755-Slice-A-Hide-Review-Loop-Dual-Cap-Reset.md) (Slice A — deliberately excludes this gate), [CP-61-Test-Steps](./CP-61-Test-Steps.md), [CP-53-Test-Steps](../done/CP-53-Test-Steps.md) (review-loop P-2 only), `tools/submit-review-outcome.yaml`, [safe-fix-contract](../../../.agents/skills/safe-fix-contract/SKILL.md)
- Replaces: `<none>` (takes over CP-53 P-2 harness leftover; review-loop gate stays Task-274 / CA-439)
- Tags: `agent-flow-engine, harness, review-verdict, fail-closed`

## AI Quick View

### Summary

- Bịt lỗ **H-3 (Ralph Wiggum done)** của CP-53 trên các **harness hub**: `plan_synthesis` (plan loop), `synthesis` (code loop), `cp_synthesis` (cp-harness) — prose `done` của synthesizer không được advance khi chưa có verdict PASS kiểm bằng máy.
- Kế thừa nguyên vẹn Task-274 (T-1…T-4): tool `submit-review-outcome` đã tồn tại (CP-53 F-2) — task này enforce consumption, không tạo tool face mới trừ khi phát hiện schema gap.
- Gate đặt **trước `advanceHubDoneThroughEdge`** cho 3 hub trên; verdict missing / FAIL / escalate → đi continue / escalate / ask_user theo edge hiện có, không được lấy `when: done`.
- Slice A (CA-755: ẩn review-loop, cap 5 + reset Round) cố ý **không** đụng đường này — không claim DoD Task-272…277 trong slice này hay slice này.

### Current Ask

- Review draft này; sau approve: tách Task con (mỗi P một Task), implement theo thứ tự P-1→P-2→P-3, tuân `safe-fix-contract` (additive tests only; **matrix Claude+Codex+Grok** bắt buộc vì shared flow runtime).
- Outcome: không harness hub nào tới `done` nếu chưa có verdict PASS đã ghi nhận.

### Key Decisions

- `D-1` Done yêu cầu verdict PASS đã ghi nhận qua `submit-review-outcome` (Task-274 T-1/T-2). Missing / FAIL / escalate verdict → không được lấy `when: done`.
- `D-2` Vị trí gate: trước `advanceHubDoneThroughEdge`, áp cho đúng 3 hub `plan_synthesis` / `synthesis` / `cp_synthesis` (mở rộng Task-274 vốn chỉ nói review-loop `synthesis`, vì review-loop đã ẩn — loop thật sống trong harness).
- `D-3` Bất đối xứng reviewer (model/effort mạnh hơn coder) theo CP-53 D-2 — cấu hình có default được document trong Task, tune sau (Q-B).
- `D-4` Normal chat không qua harness/review-loop giữ nguyên (Task-274 T-4).

### Constraints

- `feature_key: agent-flow-engine`; provider class = shared flow runtime → **R2 matrix Claude+Codex+Grok** (fake adapters OK).
- additive-tests-only; ưu tiên file test mới `cp61_*_test.go`; old test fail → STOP, report.
- **Will not undo:** CA-428 (Canonical finalize only at terminal done), CA-431 (review-loop migration / context render), `acceptance_nodes` on synthesis (Task-274), Slice A contracts (CA-755: review-loop hidden-nhưng-còn cloneable, dual-loop cap 5 + reset Round per-phase, Task-325 park/approve), dual back-edge routing (Task-304), freeze-before-code (CP-58).
- Không rewrite reviewer prompts vì chất lượng nội dung (chỉ verdict gate); `r-newtest` và dogfood hooks ngoài phạm vi (thuộc Task-277/275 nếu CP-53 mở lại).

### Open Questions

- `Q-B` (kế thừa Task-274): default model/effort cụ thể cho reviewer — chốt trong Task, tune sau khi có cost metrics.
- Verdict store theo phạm vi nào: per-hub-activation hay per-run? (đề xuất mặc định: per-hub-activation, reset cùng Round khi plan approve — quyết trong Task).
- Thứ tự với Task-325 park: verdict check chạy trước hay sau park? (đề xuất: park trước — con người đọc plan trước khi máy verdict, tránh đốt reviewer cost cho plan sẽ bị bounce).

### Source Refs

- CP-53 H-3, S-2, P-2, D-2, F-2; Task-274 T-1…T-4; CA-755 (scope exclusion); `flow-pack/flows/{task-harness,bug-plan-harness,cp-harness}.yaml` hub edges; `runner/advanceHubDoneThroughEdge`.

## 1. Goal

Biến "done" trên các harness hub thành một quyết định đã kiểm chứng bằng máy, không phải LLM tự chấm bài của chính nó.

## 2. Input Documents

- [CP-53](../done/CP-53-Review-Loop.md) (§3.3 H-3, §5.2 P-2, D-2, F-2).
- [Task-274](../../08-Task/done/Task-274-CP53-Review-Loop-Done-Requires-Machine-Verdict.md) (T-1…T-4 review-loop landed CA-439; harness 3-hub = this CP P-1 / CA-757).
- [CA-755](../../../change-audit/CA-755-Slice-A-Hide-Review-Loop-Dual-Cap-Reset.md) (ranh giới scope: Slice A không đụng gate này).

## 3. Implementation Strategy

- overall approach: enforce consumption của tool đã có — hub (hoặc synthesizer path) phải thấy verdict PASS đã ghi nhận mới cho `done` edge fire; mọi nhánh thiếu/FAIL verdict rơi về edge hiện có.
- sequencing logic: P-1 (gate 3 hub) trước — giá trị fail-closed cao nhất; P-2 (asymmetry) sau vì chỉ tăng độ tin cậy verdict; P-3 (test-steps doc) cuối để lock DoD.
- dependencies: P-1 cần đọc verdict store mà `submit-review-outcome` ghi (nếu schema gap → sửa tool tối thiểu trong cùng P-1, ghi rõ trong Task).

## 4. Work Breakdown

- `P-1` **Gate verdict trước `advanceHubDoneThroughEdge`.** Yêu cầu verdict PASS đã ghi nhận cho `plan_synthesis→done`, `synthesis→done`, `cp_synthesis→done`; missing/FAIL/escalate → continue/escalate/ask_user theo edge hiện có. Files: `runner/flow_*.go`, hub notify paths, (nếu cần) `agentpack/flow-pack/flows/*.yaml` + synthesizer agent md.
- `P-2` **Reviewer model/effort asymmetry + defaults.** Cấu hình bất đối xứng maker/checker theo CP-53 D-2; chốt Q-B defaults trong Task.
- `P-3` **[CP-61-Test-Steps](./CP-61-Test-Steps.md)** — automated + gate-sandbox manual guide for 3 hubs (replaces “update CP-53-Test-Steps §P-2”).

## 5. Touched Areas

- files: `apps/local-runner/internal/runner/flow_*.go`, hub notify paths, `apps/local-runner/internal/agentpack/flow-pack/flows/{task-harness,bug-plan-harness,cp-harness}.yaml` (nếu cần), synthesizer agent md, test mới `cp61_*_test.go`
- modules: `runner`, `agentpack`
- database: không
- external systems: không

## 6. Data or Migration Steps

- schema: không (verdict store dùng cơ chế ghi hiện có của `submit-review-outcome`; nếu cần field mới thì additive trong P-1).
- data backfill: không
- config updates: reviewer model/effort defaults (P-2, documented)

## 7. Validation Plan

- tests to add (kế thừa Task-274 T-4, mở rộng 3 hub): no verdict → cannot done; FAIL verdict → cannot done; PASS → done allowed; continue vẫn chạy mà không claim done; **matrix Claude/Codex/Grok** (fake adapters); normal chat unaffected.
- manual checks: drive một harness run tới hub `done` không verdict → xác nhận bị chặn về continue/escalate; có PASS verdict → done fire bình thường.
- failure cases: verdict vắng; verdict FAIL; verdict escalate; hub code-loop (`synthesis→audit`) không bị gate nhầm thành plan gate; restart giữa verdict và done.

## 8. Rollout and Fallback

- rollout order: P-1 → P-2 → P-3, mỗi P merge độc lập sau matrix xanh.
- fallback path: gate nằm sau cơ chế enable-rule hiện có của flow policy; bắn sai có thể tắt mà không revert code (cùng pattern CP-53 §9).
- monitoring: Task-272 metrics (khi có) đo block rate của verdict gate để phát hiện cry-wolf.

## 9. Risks

- `R-1` **Cry-wolf → bị tắt.** Verdict gate bắn quá tay đẩy user về `warn`. Giảm thiểu: chỉ chặn đúng `done`, mọi nhánh khác giữ nguyên; đo block rate.
- `R-2` **Chi phí reviewer.** Verdict bắt buộc + reviewer mạnh hơn tăng cost mỗi loop (CP-53 R-2). Giảm thiểu: coder giữ rẻ; Q-B tune sau metrics.
- `R-3` **Verdict store scope sai.** Per-run thay vì per-activation có thể cho `done` đi qua bằng verdict cũ của phase trước (đúng chỗ Slice A vừa reset Round). Giảm thiểu: mặc định per-hub-activation, test restart + reset leg trong P-1.

## 10. Definition of Done

- [ ] Không harness hub nào (`plan_synthesis` / `synthesis` / `cp_synthesis`) tới `done` khi chưa có verdict PASS đã ghi nhận (automated, matrix 3 provider).
- [ ] Verdict missing / FAIL / escalate → continue / escalate / ask_user theo edge hiện có, không stall, không silent pass.
- [ ] Normal chat / non-harness flows byte-for-byte behavior.
- [ ] Old tests untouched + green; `CA-*` ghi provider classification + matrix evidence.
- [ ] [CP-61-Test-Steps](./CP-61-Test-Steps.md) automated P-1 ticked; manual M/C on `gate-sandbox` ticked on at least one live run.
- [ ] Không claim DoD Task-272…277 (thuộc CP-53, đã đóng — xem closure note trong CP-53).
