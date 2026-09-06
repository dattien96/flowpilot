# Progress — Ẩn review-loop + dual-loop cap 5 + reset Round (Slice A)

Ngày: 2026-09-07. Trạng thái: **tạm dừng** — code + test mới xong, còn verify full suite và docs Slice B.

## Mục tiêu (đã chốt với operator)

1. Ẩn `review-loop` khỏi picker (pattern `rag-harness`: `selectableIn: []`), giữ YAML làm cloneable reference.
2. `task-harness` + `bug-plan-harness`: `policy.cap` 3 → **5**, + **reset `Round=0`** khi plan approve sang freeze (mỗi phase có budget riêng).
3. CP-53 P-2 (machine verdict) **không** đem vào slice này → làm **CP-61** riêng, **đóng CP-53**.

## Đã xong (code)

| # | Việc | Files |
|---|------|-------|
| 1 | Ẩn review-loop: `selectableIn: []`, bỏ `chatSubModes` + `chatUI` | `flows/review-loop.yaml`, `manifest.yaml` |
| 2 | Cap 3 → 5 (chỉ 2 dual-loop flow) | `flows/task-harness.yaml`, `flows/bug-plan-harness.yaml` |
| 3 | Helper `resetPlanPhaseRound` (Round=0 + diag log) | `runner/plan_approval_park.go` |
| 4 | Reset khi `plan_synthesis --done--> freeze` dispatch thành công | `runner/interactive_service.go` (`advanceHubDoneThroughEdge`) |
| 5 | Reset khi Task-325 resume-approve (chỉ khi dispatch ok) | `runner/plan_approval_park.go` (`resumePlanApproval`) |
| 6 | Không reset: park, continue, `synthesis→audit`, terminal done | — (mặc định, có test khóa) |

`bug-harness` / `rag-harness` / `cp-harness` / `review-loop` **giữ cap 3**. Không đụng topology 4 harness, adapter, CP-53 gate.

## Đã xong (test)

**Mới (additive, file mới):**
- `runner/plan_phase_round_reset_test.go` — reset khi freeze dispatch (matrix Claude/Codex/Grok), reset khi resume-approve (matrix 3), `synthesis→audit` không reset, continue vẫn park ở cap 5, Bug submode không còn offer review-loop + ref cũ bị reject.
- `agentpack/dual_loop_cap_test.go` — 2 dual-loop cap=5, 4 single-loop cap=3, review-loop hidden-nhưng-còn (cloneable).

**Cũ (allow-list đã duyệt, sửa tối thiểu):**
- `bug_plan_harness_pack_test.go` (cap 3→5), `pack_test.go` (ChatUI rỗng), `chat_builtin_orchestration_test.go` + `chat_builtin_orchestration_handler_test.go` (Bug mode empty).
- Mở rộng sau đó: `flow_executor_test.go` — 3 test `TestChatMode*` chuyển sang contract mới (hidden ref → `400 invalid_flow_ref`, không spawn, không flag engine-driven, không ghi history).

Kết quả targeted run: **tất cả PASS** (matrix 3 provider xanh).

## R1 (so với baseline cây sạch)

- Baseline stash: **20 failures** có sẵn (môi trường: provider detection, Drive/Supabase, Run144900/147126 parks, flake goroutine/TempDir…).
- Nhánh này trước khi sửa 3 test: 22 failures = 20 cũ + 3 chat-mode do hide gây ra − 1 flake baseline (`TestProviderAdaptersReceiveEquivalentFrozenContractPayload`).
- Sau khi sửa 3 test: targeted PASS. **Full suite re-run lần cuối bị abort giữa chừng → CHƯA có kết luận cuối.**
- Guard `TestDomainHardcodeGuardMatchesFrozenBaseline`: production edit chỉ dùng identifier (`planSynthesisNodeID`), không thêm literal → an toàn (chưa chạy lại sau cùng, nằm trong todo).

## Todo còn lại

- [ ] Re-run **full** `go test ./internal/runner/ -count=1`, diff failure set với baseline 20.
- [ ] Chạy `go test ./internal/agentpack/ ./internal/flowgate/ ./internal/changecontract/` (agentpack đã xanh 1 lần).
- [ ] `gofmt -l` trên các file Go đã chạm.
- [ ] Viết `change-audit/CA-755-*` (kế tiếp sau CA-754): `feature_key: agent-flow-engine`, source_doc_id, R1/R2 evidence (agnostic grep + matrix), will-not-undo (CA-712/731/749/403), liệt kê test cũ đã sửa theo allow-list.
- [ ] Slice B docs: draft **CP-61** (H-3 harness: gate verdict trước `advanceHubDoneThroughEdge` cho `plan_synthesis`/`synthesis`/`cp_synthesis`) + note **đóng CP-53** (leftover → CP-61, không claim DoD Task-272…277).
- [ ] Commit (chưa commit gì cả — xem `git status`, 10 modified + 2 untracked).

## Quyết định operator đã chốt (đừng đảo ngược khi resume)

- Hide kiểu rag-harness (`selectableIn: []`), không xóa YAML.
- Cap 5 **+** reset Round (cả hai, không phải một).
- `bug-harness` giữ nguyên (có review loop sẵn).
- CP-53 P-2 trên harness để dành cho CP-61.
