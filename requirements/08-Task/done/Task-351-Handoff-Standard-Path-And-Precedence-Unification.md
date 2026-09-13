# Task-351: Handoff Trên Đường Chuẩn + Unify Precedence (fix review P1)

## Metadata

- Document ID: `Task-351`
- Title: `Sprint Handoff Trên Đường Boundary Chuẩn + Precedence Unification + Child-Gate Drift`
- Feature Keys: `sprint-handoff, zcode-parity, gate-precedence`
- Phase: `task`
- Status: `done`
- Created: `2026-09-14`
- Last Updated: `2026-09-14`
- Parent Documents: [CP-62](../../07-Coding-Plan/done/CP-62-Zcode-Harness-Parity.md), [Task-342](./Task-342-Sprint-Handoff-Artifact-Schema-And-Chain.md), [Task-337](./Task-337-Gate-Precedence-Contract-And-Wiring.md)
- Related Documents: [CA-863](../../../change-audit/CA-863-Handoff-Standard-Path-Precedence-Unification.md)
- Tags: `sprint-handoff, gate-precedence, drift, cp-62`

## AI Quick View

### Summary
- Review phát hiện **P1 ×3**: (1) handoff chỉ emit trên các đường ngoại lệ — đường chuẩn park→Continue không emit và không inject; (2) `ResolvePrecedence` là dead code, live classifier là bản copy có thể lệch; (3) vibe flow CHILD gate sạch + drift ≥80 không escalate.

### Current Ask
- Nối handoff vào đường chuẩn; unified precedence; child-gate drift.

### Key Decisions
- `D-1` `maybeParkVibeSprintBoundary`: sau khi park thành công → `emitSprintHandoffAt(r, index)` với index vừa park (pin index — hết race ghi nhầm N vào N+1; `emitSprintHandoff` refactor delegate).
- `D-2` `continueVibeSprintBoundary` (case Start): prompt sprint N+1 nối `previousSprintHandoffContext(cwd, d.Sprint)` — giống `maybeStartNextVibeSprint`.
- `D-3` `classifyVibeGateWithDrift` consume `flowgate.ResolvePrecedence` (SD-20 §7 thành sự thật); `ResolvePrecedence` đếm block/reprompt của bucket dod vào routing (khớp EnforceResult.Action semantics — SS-18 AC-5). Test cũ pin shape nhân tạo (Action block không violation) sửa sang shape thật — disclosure trong CA-863.
- `D-4` Child gate clean branch thêm `applyVibeDriftOnlyResolver(runID, parentID, rs)` — drift ≥80 trên child sạch cũng escalate (SD-20 §7 scope theo working mode).

## 6. Acceptance Check
- [x] AC-1: handoff sprint N được ghi tại boundary park (đường chuẩn), sprint N+1 nhận qua Continue (test pin emitAt + reset).
- [x] AC-2: classifier = ResolvePrecedence, không còn bản copy (code; test precedence chỉnh shape thật).
- [x] AC-3: child drift escalation wired (root + child cùng cơ chế).

## 8. Completion Notes
- Suite `-race` xanh (flowgate + runner surface). Chi tiết CA-863.
