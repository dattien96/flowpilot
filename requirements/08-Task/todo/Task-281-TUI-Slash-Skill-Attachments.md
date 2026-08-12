# Task-281: TUI Slash Skill Attachments (CP-56 P-3)

## Metadata

- Document ID: `Task-281`
- Title: `TUI Slash Skill Attachments`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-12`
- Last Updated: `2026-08-12`
- Feature Keys: `cli-tui, skill-injection`
- Parent Documents: [CP-56](../../07-Coding-Plan/inprogress/CP-56-Terminal-TUI-Chat-And-Flow-Client.md) (P-3), [04-02 Runner Contracts](../../10-Refactor/New-System/04-02-Phase2-Runner-Contracts-And-APIs.md), [Task-280](./Task-280-TUI-Session-Controls-Provider-Model-Reasoning-Yolo.md)
- Child Documents: `none`
- Related Documents: `GET /client/provider-skills`
- Replaces: `None`
- Tags: `cli-tui, skills, slash`

## AI Quick View

### Summary

- Load skills via `ListSkills(provider, cwd)`; `/skill` list/attach/clear.
- Pending skills become `selectedSkills` with `source:"slash_picker"` on next turn, then clear.

### Current Ask

Implement P-3 skill slash UX.

### Key Decisions

- `T-1` Match by name (case-sensitive first, then case-insensitive fallback).
- `T-2` Carry `path` from catalog when present (desktop BUG-063 follow-up).
- `T-3` Refresh catalog on `/skill list` and when provider/cwd changes.
- `T-4` Snapshot pending skills into the turn, but clear them only after the turn POST returns `{turnId}`; transient POST failure keeps the queue for retry.

### Constraints

- Depends on 278–280. No runner edits.

### Open Questions

- None.

### Source Refs

- CP-56 P-3; A3.1–A3.5; M3.

---

## 1. Goal

Attach workspace/provider skills to chat turns from the TUI.

## 2. Parent Links

- coding plan: CP-56 P-3
- tech design: 04-02 `GET /client/provider-skills` and turn `selectedSkills`
- system spec: n/a; client-only delta
- specific upstream ids: CP-56 A3.1–A3.5

## 3. Trigger

Task-280 provides slash routing and provider/cwd state needed to load the correct skill catalog.

## 4. Exact Change

### 4.1 Files

```text
internal/tui/app/skills.go
internal/tui/app/skills_test.go
```

### 4.2 Code

```go
type SkillState struct {
    Catalog []client.ProviderSkill
    Pending []client.SkillSelection
}

func (s *SkillState) Refresh(ctx context.Context, c *client.Client, provider, cwd string) error
func (s *SkillState) HandleSlash(args []string) (msg string, err error)
// args: empty|list → list names; <name> → attach; clear → wipe pending
func (s SkillState) SnapshotForTurn() []client.SkillSelection
func (s *SkillState) ClearAfterTurnAccepted()
```

### 4.3 Wire

- Slash router: `/skill` → `SkillState.HandleSlash`
- On send: snapshot pending skills; clear only after `SendTurn` succeeds
- Show pending names above input or in status placeholder: `skills: a, b`

### 4.4 Types in client (if missing)

```go
type ProviderSkill struct {
    Name        string `json:"name"`
    Path        string `json:"path,omitempty"`
    Description string `json:"description,omitempty"`
    Source      string `json:"source"`
}
```

## 5. Touched Areas

- files: `apps/local-runner/internal/tui/app/skills.go`, additive tests
- modules: existing TUI app/client packages
- routes: existing `GET /client/provider-skills`
- tables: none

## 6. Acceptance Check

- [ ] A3.1–A3.5 green, including pending skills retained after turn-POST failure
- [ ] Manual M3

## 7. Out of Scope

- Fuzzy picker UI beyond list text; image attach (282)

## 8. Completion Notes

- result: pending
- follow-ups: Task-282
- upstream docs updated: CP-56-Test-Steps evidence when complete
