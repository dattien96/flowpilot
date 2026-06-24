# Task-114 — Chat Mode Selector: Normal / Task / Bug

## Metadata

- Document ID: `Task-114`
- Title: `Chat mode selector — declare Task or Bug intent at chat-start; inject prompt and stamp ChangeType`
- Phase: `task`
- Status: `todo`
- Owner: `DatNguyen`
- Reviewers: `CP-35`
- Created: `2026-06-24`
- Last Updated: `2026-06-24`
- Parent Documents: `CP-35`
- Child Documents: none
- Related Documents: `Task-113`, `Task-099`, `SD-20`, `CA-126`, `CA-128`
- Replaces: none
- Tags: `context-regression-engine, flow-gate, chat-ui, r-task, r-bug`

## AI Quick View

### Summary

- Adds a mode picker to chat creation: **Normal** (default) | **Task** | **Bug**
- When Task or Bug is selected, the runner injects a targeted instruction block into the first turn's prompt — the AI knows upfront exactly which file to create
- Runner stamps `TurnResult.ChangeType` from the declared mode, so the gate fires on explicit declaration instead of scanning the AI's final message for keywords
- The declared mode is stored on the run after the first send, so later turns and gate reprompts keep the same Task/Bug intent even when the assistant reply is just a plain "done" message
- Eliminates the v1 heuristic gap: r-task and r-bug no longer miss silent completions for declared-mode chats; message scanning remains as fallback for Normal chats only

### Current Ask

- Design only (this doc). Implementation follows in a separate turn after design is reviewed.

### Key Decisions

- `T-1` Mode is declared **once at chat-start** (new chat creation point), not mid-chat — avoids retroactive prompt injection complexity
- `T-2` ID input is **optional** (free-text, e.g. `Task-113`). No ID = mode-only injection with generic `Task-<NNN>` placeholder. Provides flexibility for quick undocumented work while still guiding the AI
- `T-3` Injection is a **prompt prefix**, not a system-prompt edit — keeps it provider-agnostic (Claude, Codex, Gemini all receive it as user-visible context)
- `T-4` `ChangeType` stamped on `interactiveRun`; passed to every `TurnResult` for that run — gate uses it from turn 1, no regex needed
- `T-5` Message heuristic (`taskIDRegex`, "bug fix" keyword) is **kept as fallback** for Normal-mode chats — no regression for existing behavior

### Constraints

- Must not break Normal mode: zero change to current chat behavior when mode is not declared
- Must be provider-agnostic: no dependency on system-prompt support; injection is prepended to first turn prompt only
- Must not require ID: mode without ID still injects generic guidance and still stamps ChangeType
- Gate logic for `tr.ChangeType == "task"` is already wired in `evaluate.go` — this Task only needs to make the runner actually set it

### Open Questions

- `Q-1` Where exactly in the new-chat UX does the picker appear? Options: (a) inline below the first message composer, (b) a "Start as..." dropdown in the chat header, (c) a small pill/toggle row above the composer. Recommendation: (c) — minimal friction, visible, collapsible.
- `Q-2` Should selecting Task/Bug mode also automatically add the FORMAT-REFERENCE file as a context attachment, so the AI can read the exact structure? Nice-to-have, deferred.
- `Q-3` Should the ID field auto-complete from `requirements/08-Task/todo/` and `requirements/09-BugFix/todo/`? Deferred to follow-up.

### Source Refs

- `CP-35`, `SD-20 §2.6 / §2.7`, `Task-113` (r-task rule), `Task-099` (gate hook)

## 1. Goal

Let the user declare at chat-start whether they are working on a tracked Task, a Bug fix, or a normal chat. The declaration drives two things: an upfront prompt injection that tells the AI exactly what artifact to create, and a `ChangeType` stamp that makes the gate enforce without relying on keyword scanning of the AI's final message.

## 2. Parent Links

- coding plan: `CP-35-Context-And-Regression-Engine-Rollout.md`
- tech design: `SD-20-Flow-Gate-Rule-Semantics.md §2.6/§2.7`
- system spec: `SS-14` (AC-11: force required outputs)
- specific upstream ids: `CP-35`, `SD-20`

## 3. Trigger

The v1 heuristic (scan AI final message for `Task-NNN` / "bug fix") only fires when the AI explicitly names the identifier in its summary. A silent completion — "Done. Added the comment." — never triggers r-task or r-bug. Declaring intent at chat-start eliminates the gap for tracked work and gives the AI clear upfront instructions, reducing the chance of needing a reprompt at all.

## 4. Exact Change

### 4.1 UI — `apps/desktop-flowpilot/src/`

- `T-1` Add a **mode row** above the message composer in the new-chat view. Three options rendered as small toggleable pills: `Normal` (default, selected) | `Task` | `Bug`.
- `T-2` When `Task` or `Bug` is selected, show an optional free-text ID input inline (`Task-113`, `BUG-141`, etc.). Placeholder text: `Task-NNN (optional)` / `BUG-NNN (optional)`.
- `T-3` Store the selection in local component state. On first message send, include `changeType` and `sourceDocID` in the turn start payload.
- `T-4` The mode row is **only shown before the first turn**. Once the chat has turns it collapses (mode is locked).

### 4.2 Runner — `apps/local-runner/internal/runner/`

**`TurnInput` (wherever it is defined):**
```go
type TurnInput struct {
    StepID     string `json:"stepId,omitempty"`
    Prompt     string `json:"prompt"`
    ChangeType string `json:"changeType,omitempty"` // "task" | "bugfix" | ""
    SourceDocID string `json:"sourceDocId,omitempty"` // "Task-113" | "BUG-141" | ""
}
```

**`interactiveRun` struct:**
```go
changeType  string  // set once from first TurnInput that carries it
sourceDocID string
```

**Prompt injection (in `startTurn` or `sendTurnWithRetry`, before first turn only):**
```go
func buildModePrefix(changeType, sourceDocID string) string {
    switch changeType {
    case "task":
        id := sourceDocID
        if id == "" { id = "Task-<NNN>" }
        return "[Flow context: You are completing " + id + ". " +
            "When your work is done you MUST create a NEW file " +
            "`requirements/08-Task/done/" + id + "-<short-title>.md` " +
            "following the structure in `requirements/08-Task/FORMAT-REFERENCE-TASK.md`. " +
            "Do NOT use the FORMAT-REFERENCE file itself as the artifact. " +
            "The gate re-checks after your turn.]\n\n"
    case "bugfix":
        id := sourceDocID
        if id == "" { id = "BUG-<NNN>" }
        return "[Flow context: You are fixing " + id + ". " +
            "When your fix is done you MUST create a NEW file " +
            "`requirements/09-BugFix/done/" + id + "-<short-title>.md` " +
            "following the structure in `requirements/09-BugFix/FORMAT-REFERENCE-BUGFIX.md`. " +
            "Do NOT edit the change-audit note to satisfy this. " +
            "The gate re-checks after your turn.]\n\n"
    }
    return ""
}
```

Inject as a prefix to the **first turn prompt only** (`rs.turnCount == 0`). Subsequent turns (including gate reprompts) do not re-inject.

**`TurnResult` stamping (in `runFlowGate`):**
```go
tr := flowgate.TurnResult{
    ...
    ChangeType: rs.changeType,   // already wired in evaluate.go checkRule
    SourceDocID: rs.sourceDocID,
}
```

### 4.3 Gate — `apps/local-runner/internal/flowgate/evaluate.go`

No logic change needed. `checkRule("task_referenced")` already checks `tr.ChangeType == "task"`:
```go
hasTaskRef := taskIDRegex.MatchString(tr.FinalMessage) || tr.ChangeType == "task"
```
And `checkRule("bug_fixed")` already checks `tr.ChangeType == "bugfix"`. Once the runner stamps it, both rules work without message scanning for declared-mode chats.

## 5. Touched Areas

- files: `apps/desktop-flowpilot/src/` (new-chat composer component, turn start payload), `apps/local-runner/internal/runner/` (`TurnInput`, `interactiveRun`, `startTurn`/`sendTurnWithRetry`, `gate_hook.go`)
- modules: `runner`, `desktop chat UI`
- routes: none
- tables: none (changeType is ephemeral per-run; not persisted)

## 6. Acceptance Check

- Normal mode: zero change to existing behavior; no prefix injected, no ChangeType set, gate still uses message heuristic
- Task mode without ID: prefix injected with `Task-<NNN>` placeholder; gate fires r-task on every turn of this run (ChangeType == "task") regardless of final message content
- Task mode with ID `Task-113`: prefix names the file explicitly; gate fires r-task; on reprompt AI creates `requirements/08-Task/done/Task-113-*.md` and gate passes
- Bug mode: same as Task mode but for r-bug / `requirements/09-BugFix/done/BUG-NNN-*.md`
- Mode row collapses after first turn is sent
- Gate reprompts do not re-inject the prefix (would duplicate context)

## 7. Out of Scope

- Mid-chat mode switching
- Auto-complete of Task/Bug IDs from the requirements folder (Q-3)
- Attaching FORMAT-REFERENCE as a context file (Q-2)
- Persisting changeType to Supabase or Drive
- Changing the gate's reprompt count limit (still ≤2)
- Any change to Normal-mode gate behavior

## 8. Completion Notes

- result: not yet implemented — design doc only
- follow-ups: implement UI pill row + runner injection + TurnResult stamping; the first turn saves the declared mode on the run, and later turns reuse that saved intent so the gate still works even if the final assistant message never repeats `Task-` or `Bug-`; then E2E-14 can be re-tested with declared Task mode (no forced phrase in prompt needed)
- upstream docs updated: SD-20 §2.7 already documents `ChangeType` as the future fix; this Task is the implementation of that note
