# Task-352: Review Hygiene Batch (P2 findings + doc drift + test pins)

## Metadata

- Document ID: `Task-352`
- Title: `Review Hygiene Batch — P2 findings, doc drift, test pins`
- Feature Keys: `zcode-parity, gate-schema, drift-pause, sprint-handoff`
- Phase: `task`
- Status: `done`
- Created: `2026-09-14`
- Last Updated: `2026-09-14`
- Parent Documents: [CP-62](../../07-Coding-Plan/done/CP-62-Zcode-Harness-Parity.md)
- Related Documents: [CA-864](../../../change-audit/CA-864-Review-Hygiene-Batch.md)
- Tags: `zcode-parity, hygiene, docs, tests`

## AI Quick View

### Summary
- Gom các P2 từ 5 agent review: malformed `verdicts` arg bị bỏ im lặng (fail-open); ExtractACIDs comment sai thứ tự; drift pause đè foreign park; test pins thiếu (read_only zero-rows, applyFlowControl card emit, capture by label, sprint reset, pinned-index); doc drift ở Task-337/339/344/348 + FEATURE-KEYS + Task-345 channel claim.

### Key Decisions
- `D-1` `parseReviewOutcomeInput`: verdicts PRESENT nhưng không phải array → reject ("verdicts must be an array").
- `D-2` `armDriftPause`: CHỈ arm khi loop chưa bị park (BlockReason rỗng) — không đè `cap`/`escalate`/boundary, không duplicate event.
- `D-3` Comment `ExtractACIDs` sửa "lexicographically sorted".
- `D-4` Test pins mới: zero-rows reject, malformed verdicts, applyFlowControl emit card, capture by label, sprint-advance reset, emitAt pinned index, drift non-clobber.
- `D-5` Doc corrections: Task-337 §5/§11; Task-339 Current-Ask/Touched; Task-344 tick AC + deviation note; Task-348 D-1/T-2 superseded design; Task-345 D-3 channel; FEATURE-KEYS drift-pause wording.

## 6. Acceptance Check
- [x] AC-1: malformed verdicts → tool error in-turn (test pin).
- [x] AC-2: foreign park được bảo toàn (test pin).
- [x] AC-3: các test pin liệt kê ở D-4 xanh.
- [x] AC-4: docs khớp code (section liên quan đã sửa).

## 8. Completion Notes
- Toàn bộ surface `-race` xanh; 2 test nghi vấn từ review xác nhận là nhiễu full-suite (pass ×2 khi chạy đơn). Chi tiết CA-864.
