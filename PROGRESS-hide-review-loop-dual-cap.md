# Progress — Ẩn review-loop + dual-loop cap 5 + reset Round (Slice A)

Ngày: 2026-09-07. Trạng thái: **xong Slice A + Slice B docs** — chờ commit.

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

- Full `go test ./internal/runner/ -count=1` (2026-09-07, sau sync): **19 failures = 18 stash-proven pre-existing + 1 flake**.
  - 16-failure subset chạy lại trên cây sạch (stash): fail **y hệt** (provider detection, Drive/Supabase, Run144900 parks, portability canary, `TestResumeFlowWithFeedbackAfterEscalate`, `TestRootFlowEngineDefersCompletedUntilGate`, `TestSpawnChildEmitsGraphAndBusEvents`, `TestFinalizerHookSurfacesArtifacts`…).
  - 2 `TestValidatePassed*`: fail y hệt trên cây sạch (Windows không có executable `true` trong `%PATH%` — env).
  - `TestIntentClear_NoTOCTOUResurrectionUnderConcurrentAccess`: flake full-suite-only đã biết — PASS isolated `-count=3` trên nhánh này, PASS trên baseline lần này.
  - Không failure nào đụng đường review-loop / cap / park; 12 targeted tests xanh (re-run lần 2, lần 1 có 1 FAIL lẻ không lặp lại — flake).
- `internal/agentpack` xanh; `internal/changecontract` xanh; `internal/flowgate` 2 failures pre-existing Windows-env (fixture `.sh`: `%1 is not a valid Win32 application`), không đụng code slice này.
- Guard `TestDomainHardcodeGuardMatchesFrozenBaseline`: **PASS** (production edit chỉ dùng identifier `planSynthesisNodeID`/`planFreezeNodeID`, không thêm literal).
- `git diff --check` sạch; `gofmt -l` là noise repo-wide (CRLF checkout — 534 files bị flag ngay trên cây sạch), không thêm whitespace mới.

## Todo còn lại

- [x] Re-run **full** `go test ./internal/runner/ -count=1`, diff failure set với baseline 20 → 19 = 18 pre-existing + 1 flake (xong 2026-09-07).
- [x] Chạy `go test ./internal/agentpack/ ./internal/flowgate/ ./internal/changecontract/` (agentpack + changecontract xanh; flowgate 2 env-fail pre-existing).
- [x] `gofmt -l` trên các file Go đã chạm → noise CRLF repo-wide, `git diff --check` sạch.
- [x] Viết `change-audit/CA-755-*` (kế tiếp sau CA-754): `feature_key: agent-flow-engine`, source_doc_id, R1/R2 evidence (agnostic grep + matrix), will-not-undo (CA-712/731/749/403), liệt kê test cũ đã sửa theo allow-list.
- [x] Slice B docs: draft **CP-61** (`requirements/07-Coding-Plan/done/CP-61-Harness-Done-Verdict-Gate.md`, closed 2026-09-08 P-1+P-3) + note **đóng CP-53** (§12 closure note, Status closed, Related trỏ CP-61).
- [ ] Commit (xem ghi chú sync bên dưới).

## Ghi chú sync 2026-09-07 (quyết định operator đã chốt ở trên giữ nguyên)

- Remote `origin/cp58-harness-dual-loop` đã có commit `694a04e "ddđ"` chứa **y hệt** 11 modified files của slice này (đã diff xác nhận, backup 3 untracked files khớp byte).
- Local đã `reset --hard` về `694a04e` — workdir sạch, không mất gì. Commit message `"ddđ"` là của operator; **chưa amend** (cần operator quyết có force-push sửa message không).
- Commit tiếp theo chỉ chứa: CA-755 + CP-61 + CP-53 closure note + PROGRESS update.

## Quyết định operator đã chốt (đừng đảo ngược khi resume)

- Hide kiểu rag-harness (`selectableIn: []`), không xóa YAML.
- Cap 5 **+** reset Round (cả hai, không phải một).
- `bug-harness` giữ nguyên (có review loop sẵn).
- CP-53 P-2 trên harness để dành cho CP-61.
