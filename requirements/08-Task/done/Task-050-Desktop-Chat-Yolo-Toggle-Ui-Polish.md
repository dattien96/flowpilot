# Task-050: Desktop Chat Yolo Toggle UI Polish

## Metadata

- Document ID: `Task-050`
- Title: `Desktop Chat Yolo Toggle UI Polish`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-15`
- Last Updated: `2026-06-15`
- Parent Documents: [Task-049: Desktop Chat Provider Skills And Mode Regression Fix](../done/Task-049-Desktop-Chat-Provider-Skills-And-Mode-Regression-Fix.md)
- Child Documents: `none`
- Related Documents: [Task-048: Desktop Chat Skills Workflow Composer Alignment](../done/Task-048-Desktop-Chat-Skills-Workflow-Composer-Alignment.md)
- Replaces: `none`
- Tags: `desktop, ui, yolo, toggle, chat`

## AI Quick View

### Summary

- Replace the text-pill `YOLO` control with a real toggle-style switch.

### Current Ask

- Done. The `YOLO` control now renders as a toggle switch while keeping the same state behavior.

### Key Decisions

- `T-1` Keep this as a presentation-only change; no behavior or contract changes.

### Constraints

- UI only.
- No state or runner logic changes.

### Open Questions

- None.

### Source Refs

- `Task-049`

## 1. Goal

Make the desktop chat `YOLO` control visually read as a toggle switch instead of a text pill.

## 2. Parent Links

- coding plan: [Task-049: Desktop Chat Provider Skills And Mode Regression Fix](../done/Task-049-Desktop-Chat-Provider-Skills-And-Mode-Regression-Fix.md)
- tech design: [Task-049: Desktop Chat Provider Skills And Mode Regression Fix](../done/Task-049-Desktop-Chat-Provider-Skills-And-Mode-Regression-Fix.md)
- system spec: [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- specific upstream ids: `Task-049`

## 3. Trigger

The chat controls were functionally correct, but the `YOLO` UI still looked like a text pill instead of a toggle.

## 4. Exact Change

- `T-1` Replace the `YOLO` pill button with a toggle-style switch and label.

## 5. Touched Areas

- files: `apps/desktop-flowpilot/src/components/ChatInput.tsx`, `apps/desktop-flowpilot/src/styles.css`
- modules: `desktop-flowpilot`
- routes: `desktop shell`
- tables: `none`

## 6. Acceptance Check

- `YOLO` renders as a toggle switch.
- Existing on/off behavior still works.

## 7. Out of Scope

- No logic changes.
- No other chat control redesigns.

## 8. Completion Notes

- result: Implemented the toggle-style `YOLO` UI.
- follow-ups: none
- upstream docs updated: `Task-050`
