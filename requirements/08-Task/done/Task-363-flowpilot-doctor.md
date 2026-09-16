# Task-363: flowpilot doctor (LSP Environment Check)

## Metadata

- Document ID: `Task-363`
- Title: `flowpilot doctor (LSP Environment Check)`
- Phase: `task`
- Status: `done`
- Owner: `Claude Sonnet MAX`
- Reviewers: `Claude agent review`
- Created: `2026-09-15`
- Last Updated: `2026-09-15`
- Parent Documents: [CP-63](../../07-Coding-Plan/done/CP-63-IDE-Grade-LSP-Runtime.md) (R-1 partial mitigation)
- Child Documents: `None`
- Related Documents: [Task-356](../done/Task-356-LSP-Platform-Registry-And-Auto-Detection.md), [CA-871](../../../change-audit/CA-871-CP-63-Followup-Batch.md)
- Replaces: `None`
- Tags: `lsp, doctor, cli, diagnostics, environment`
- Feature Keys: `lsp-runtime`

## AI Quick View

### Summary

- `flowpilot doctor` kiểm tra mọi language server trong registry có resolve được qua PATH không, in bảng per-platform kèm câu lệnh cài đặt, exit 1 khi thiếu (scriptable), exit 0 khi đủ.
- Logic thuần (`doctorCheck` + `formatDoctor`) tách khỏi cobra I/O để test hermetic (stub lookPath).
- Không đụng runner đang chạy, không spawn process nào.

### Current Ask

- Done.

### Key Decisions

- `T-1` Exit non-zero khi thiếu (standard doctor behavior); FlowPilot vẫn chạy không cần chúng.
- `T-2` Sort theo platform để output ổn định.
- `T-3` Đăng ký cạnh `chat` command (1 dòng additive trong `NewRootCommand`).

### Constraints

- Additive tests only — không edit pre-existing tests.
- Không network, không spawn, chỉ PATH lookup.

### Open Questions

- None.

### Source Refs

- CP-63 §9 R-1.
- `internal/cli/doctor.go`, `DefaultRegistry()`.

## 1. Goal

User gõ 1 lệnh biết ngay máy thiếu server nào và cài bằng gì, thay vì đọc log hay mò docs.

## 2. Parent Links

- coding plan: `CP-63-IDE-Grade-LSP-Runtime.md` R-1
- tech design: `SD-17-Context-And-Regression-Engine.md`
- system spec: `SS-14-Code-Context-And-Regression-Safety.md`

## 3. Trigger

CP-63 R-1 hẹn `flowpilot doctor` kiểm tra PATH cho required binaries; Task-362 tách riêng việc này ra.

## 4. Exact Change

- `T-1` **`internal/cli/doctor.go`** (new): `doctorCheck` (stub-able lookPath), `formatDoctor` (aligned table), `newDoctorCommand` (exit 1 khi thiếu).
- `T-2` **`internal/cli/root.go`**: +1 dòng đăng ký.
- `T-3` **`internal/cli/doctor_test.go`** (new): register + table logic + format.

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/cli/doctor.go` (new)
  - `apps/local-runner/internal/cli/doctor_test.go` (new)
  - `apps/local-runner/internal/cli/root.go` (1 dòng đăng ký)
- modules: `cli`
- routes: none
- tables: none

## 6. Acceptance Check

- [x] AC-1: `flowpilot doctor` có mặt trên root command.
- [x] AC-2: Table logic đúng cả 2 case (đủ/thiếu), sort ổn định.
- [x] AC-3: Format hiện MISSING + hint + summary.
- [x] AC-4: 4 tests green, không edit test cũ.

## 7. Out of Scope

- Auto-install binaries.
- Kiểm tra runtime (go/node/python) — chỉ LSP servers.
- Full `flowpilot doctor` cho mọi subsystem (mở rộng sau).

## 8. Completion Notes

- result: Implemented as specified.
- follow-ups: mở rộng doctor cho subsystems khác nếu cần.
- upstream docs updated: CA-871.
