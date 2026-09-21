# Task-386: Compiler Verification Gate & Self-Healing Loop

## Metadata

- Document ID: `Task-386`
- Title: `Compiler Verification Gate & Self-Healing Loop`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot Architecture`
- Reviewers: `Operator, Claude Sonnet MAX`
- Created: `2026-09-18`
- Last Updated: `2026-09-19`
- Parent Documents: [CP-68 P-4](../../07-Coding-Plan/done/CP-68-Skill-Anchored-Scaffold-And-AI-Guided-Init.md)
- Child Documents: `None`
- Related Documents: [Task-383](../todo/Task-383-Platform-Scaffold-Recipe-Discovery-And-Skill-Integrity-Validator.md), [Task-384](../todo/Task-384-Runner-Scaffold-Dispatcher-And-AI-Turn-Orchestration.md), [Task-385](../todo/Task-385-TUI-Subcommand-And-Desktop-UI-Adaptive-Scaffold-Trigger.md)
- Replaces: `None`
- Tags: `compiler-gate, verification, self-healing, typecheck, fail-closed, local-runner`
- Feature Keys: `skill-anchored-init, scaffold-engine`

---

## AI Quick View

### Summary

- Hiện thực hóa Lát cắt P-4 của CP-68: Xây dựng **Compiler Verification Gate** (`internal/runner/compiler_gate.go`) làm chốt chặn Oracle cuối cùng cho luồng khởi tạo Step 0.
- Sau khi AI hoàn tất lượt sinh mã nguồn Step 0, Runner tự động thực thi lệnh kiểm tra biên dịch được định nghĩa trong recipe (ví dụ: `pnpm install && pnpm tsc --noEmit` với React Native).
- **Cơ chế Fail-Closed:** Dự án chỉ được xác nhận là `init` thành công khi compiler trả về Exit Code 0.
- **Vòng lặp tự sửa (Self-Healing Loop):** Nếu compiler báo lỗi (sai kiểu, thiếu import, thiếu cấu hình), Runner trích xuất chi tiết lỗi từ stdout/stderr và chuyển tiếp cho AI trong lượt phản hồi (tối đa `cap: 3` lần) để AI tự sửa chữa cho đến khi dự án xanh 100%.
- Khi hoàn tất thành công, ghi nhận trạng thái vào file `.flowpilot/scaffold-status.json`.

### Current Ask

- Viết file `internal/runner/compiler_gate.go` chứa logic thực thi lệnh kiểm tra, bắt timeout, và đóng gói lỗi compiler.
- Tích hợp `CompilerGate` vào cuối quy trình điều phối của `ScaffoldDispatcher` (từ Task-384).
- Viết unit tests kiểm tra cả kịch bản pass ngay lần đầu và kịch bản tự sửa lỗi sau khi compiler báo lỗi trong `internal/runner/compiler_gate_test.go`.

### Key Decisions

- `T-1` Command Execution Sandbox: Lệnh kiểm tra biên dịch được chạy trực tiếp trong thư mục workspace của dự án với biến môi trường được kế thừa an toàn. Timeout được cấu hình mặc định là 300 giây (đảm bảo đủ thời gian cho `pnpm install`).
- `T-2` Structured Compiler Feedback: Khi lệnh fail, Runner lọc bớt các dòng log rác, trích xuất chính xác tên file, số dòng và thông điệp lỗi (ví dụ: `TS2307: Cannot find module '@flowpilot/core-ui'`), gửi lại cho AI kèm prompt:
  `[Compiler Gate Thất Bại] Trình biên dịch trả về lỗi sau. Hãy sửa mã nguồn để thỏa mãn compiler:`
- `T-3` Budget Cap (`cap: 3`): Vòng lặp tự sửa tối đa 3 lần. Nếu sau 3 lần vẫn không pass, Runner dừng lại và báo lỗi rõ ràng cho người dùng kèm toàn văn log để người dùng can thiệp, không để AI lặp vô tận tiêu tốn token.

### Constraints

- Không cho phép bỏ qua (bypass) Compiler Gate khi Scaffold Turn chạy tự động trong luồng `/init` (TUI) hoặc auto-trigger từ Desktop.
- Đảm bảo xử lý graceful nếu máy người dùng bị ngắt mạng trong lúc chạy `pnpm install`.

### Open Questions

- None.

### Source Refs

- [CP-68 P-4, Key Decision P-5 & Open Question Q-1](../../07-Coding-Plan/todo/CP-68-Skill-Anchored-Scaffold-And-AI-Guided-Init.md)
- `apps/local-runner/internal/runner/scaffold_dispatcher.go` (từ Task-384)
- `apps/local-runner/internal/skillpack/scaffold_recipe.go` (từ Task-383)

---

## 1. Goal

Hiện thực hóa chốt chặn kiểm định biên dịch (Compiler Gate) và vòng lặp tự sửa lỗi (Self-Healing) cho Step 0, đảm bảo mọi dự án do FlowPilot khởi tạo đều đạt tiêu chuẩn biên dịch sạch 100% trước khi bàn giao cho người dùng.

---

## 2. Parent Links

- coding plan: `CP-68-Skill-Anchored-Scaffold-And-AI-Guided-Init.md` P-4
- system spec: `SS-14-Code-Context-And-Regression-Safety.md`, `SS-20-Definition-Of-Done-Gate-Contract.md`

---

## 3. Trigger

Mã nguồn do AI sinh ra ở Step 0 cần có một "quan tòa khách quan" (Oracle) là trình biên dịch thật kiểm định, thay vì chỉ dựa vào sự tự tin của AI.

---

## 4. Exact Change

- `T-1` Tạo file `apps/local-runner/internal/runner/compiler_gate.go`:
  - Khai báo struct:
    ```go
    type CompilerGateResult struct {
        Passed        bool     `json:"passed"`
        ExitCode      int      `json:"exitCode"`
        RawOutput     string   `json:"rawOutput"`
        ParsedErrors  []string `json:"parsedErrors,omitempty"`
        ExecutionTime time.Duration `json:"executionTime"`
    }

    type CompilerGate struct {
        workingDir string
        command    string
        timeout    time.Duration
    }
    ```
  - Triển khai hàm `Run(ctx context.Context) (*CompilerGateResult, error)`:
    - Chạy lệnh qua shell `sh -c` hoặc `cmd.exe` với working directory trỏ vào project.
    - Bắt stdout + stderr kết hợp.
    - Nếu ExitCode == 0: Trả về `Passed: true`.
    - Nếu ExitCode != 0: Trả về `Passed: false` kèm output đã định dạng.
- `T-2` Tích hợp vòng lặp Healing Loop trong `ScaffoldDispatcher.Dispatch`:
  - Sau khi lượt AI Turn sinh code kết thúc:
    ```go
    for attempt := 1; attempt <= recipe.VerificationGate.Cap; attempt++ {
        res, err := gate.Run(ctx)
        if res.Passed {
            // Ghi file .flowpilot/scaffold-status.json
            return &ScaffoldDispatchResult{Status: "done"}, nil
        }
        // Gửi turn phản hồi cho AI để sửa lỗi
        aiResponse = session.SendFeedback(ctx, res.RawOutput)
    }
    ```
- `T-3` Tạo file `apps/local-runner/internal/runner/compiler_gate_test.go`:
  - `TestCompilerGate_SuccessCommand`: Chạy mock command thành công (ví dụ: `echo "success"`) -> Trả về `Passed: true`.
  - `TestCompilerGate_FailingCommand`: Chạy mock command thất bại (ví dụ: `exit 1`) -> Trả về `Passed: false` và bắt được lỗi.
  - `TestCompilerGate_Timeout`: Chạy lệnh vượt timeout -> Hủy tiến trình và báo lỗi timeout rõ ràng.

---

## 5. Touched Areas

- files:
  - `apps/local-runner/internal/runner/compiler_gate.go` (new)
  - `apps/local-runner/internal/runner/compiler_gate_test.go` (new)
  - `apps/local-runner/internal/runner/scaffold_dispatcher.go` (modified - tích hợp gate)
- modules: `runner`
- routes: none
- tables: none

---

## 6. Acceptance Check

- `go test -v ./internal/runner -run TestCompilerGate` đạt kết quả PASS 100%.
- Khi chạy thử nghiệm với một project TypeScript cố tình tạo lỗi cú pháp, Compiler Gate phát hiện chính xác và trả về `Passed: false` kèm dòng lỗi.
- Khi mã nguồn được sửa hết lỗi, Compiler Gate trả về `Passed: true` và file `.flowpilot/scaffold-status.json` được tạo thành công.

---

## 7. Out of Scope

- Không can thiệp vào các gate của `vibe-sprint` hay `task-harness` sau khi Step 0 đã hoàn thành.

---

## 8. Completion Notes

- result: Implemented. `internal/runner/compiler_gate.go` runs the recipe command through `sh -c` (`cmd /c` on Windows) pinned to the workspace with the parent env inherited, a per-attempt timeout (recipe `timeout_seconds`, default 300s) and a `WaitDelay` guard against hung grandchildren. Fail-closed: `Passed` is true only on exit 0; timeout and "cannot start" are surfaced as non-healable results so a stalled `pnpm install` never burns AI repair turns. `parseCompilerErrors` extracts bounded, de-duplicated `file:line` + `error TSxxxx` diagnostics (rune-safe truncation). Integrated at the end of `ScaffoldDispatcher.Dispatch` (Task-384): pass ⇒ write `.flowpilot/scaffold-status.json` (replay guard, CP-68 §6); fail ⇒ structured `[Compiler Gate Thất Bại]` feedback turn with the same attached skills, at most `cap` gate attempts; over cap ⇒ error result with the full log for human intervention.
- tests: `compiler_gate_test.go` — 7 additive tests (success, failing chain with TS2307 extraction + noise filtering, timeout cancellation, working-dir + env inheritance, empty-command config error, parse filter/dedupe, rune-safe line cap); plus the heal/cap/timeout dispatch scenarios in `scaffold_dispatcher_test.go`.
- note: `cap` is the number of gate attempts (1 initial scaffold turn + cap−1 repair turns), so no repair turn is wasted after the final failed verification.
- follow-ups: None (CP-68 complete).
- upstream docs updated: `CA-892`.
