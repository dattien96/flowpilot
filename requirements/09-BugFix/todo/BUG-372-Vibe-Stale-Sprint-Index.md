# BUG-372: Stale vibeSprintIndex after CP rewrite / re-slice

## Metadata

- Document ID: `BUG-372`
- Title: `Chip task 2/3 with Task-904 still draft after R-TK-D2`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-11`
- Last Updated: `2026-09-11`
- Parent Documents: [CP-60-Test-Steps](../../07-Coding-Plan/done/CP-60-Test-Steps.md), [CA-831](../../../change-audit/CA-831-Vibe-Stale-Sprint-Index-Reset.md)
- Feature Keys: `vibe-mode`

## AI Quick View

### Summary

- After delete Task+CP → rewrite CP → slicer, UI showed `task 2/3` while Task-904 stayed `draft` and sprint ran Task-905.
- Fix: clear sprint cursor on CP rewrite; after slicer always reload disk plan and reset index to 0.

### Current Ask

- Landed CA-831. Rebuild before next R-TK-D3.

## 8. Completion Notes

- result: `done` — `TestBUG372_*` green
