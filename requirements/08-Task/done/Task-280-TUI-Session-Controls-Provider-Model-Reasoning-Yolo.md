# Task-280: TUI Session Controls — Provider, Model, Reasoning, YOLO (CP-56 P-2)

## Metadata

- Document ID: `Task-280`
- Title: `TUI Session Controls Provider Model Reasoning YOLO`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-12`
- Last Updated: `2026-08-12`
- Feature Keys: `cli-tui, yolo-policy`
- Parent Documents: [CP-56](../../07-Coding-Plan/inprogress/CP-56-Terminal-TUI-Chat-And-Flow-Client.md) (P-2), [04-02 Runner Contracts](../../10-Refactor/New-System/04-02-Phase2-Runner-Contracts-And-APIs.md), [Task-279](./Task-279-Bubble-Tea-Chat-Stream-Shell.md)
- Child Documents: `none`
- Related Documents: Desktop BUG-063 per-turn controls in `store.ts`
- Replaces: `None`
- Tags: `cli-tui, yolo, provider, model`

## AI Quick View

### Summary

- Session-local controls via flags + `/provider` `/model` `/reasoning` `/yolo`.
- Chat turns re-send model/yolo/reasoning every turn (desktop BUG-063 parity).
- Lock provider after run started; require `/new` to change (minimal `/new` clear runID here or wait 287 — prefer clear runID+stepID only).

### Current Ask

Implement P-2 controls on the Bubble Tea shell.

### Key Decisions

- `T-1` `YoloMode` as `*bool` in JSON so false is distinct from omit.
- `T-2` Provider immutable once `RunID != ""` until session reset.
- `T-3` Load provider/model/reasoning options from existing `GET /providers`: `Provider.Models` carries live/static-fallback model IDs and reasoning capability metadata. Do not add a runner endpoint or static TUI list.
- `T-4` When the provider/model changes, clear an unsupported reasoning effort and use the model's advertised default.
- `T-5` Grok YOLO is not only a turn field: call existing `POST /provider-accounts/grok-yolo-posture` and update local state only after success. Other providers use the per-turn pointer.
- `T-6` In chat mode always send a non-nil model pointer, including `""` to restore provider default; workflow/step mode leaves the pointer nil so configured resolution remains authoritative.

### Constraints

- No runner edits. Chat-mode overrides only.

### Open Questions

- None.

### Source Refs

- CP-56 P-2, D-5, D-11; A2.1–A2.9.

---

## 1. Goal

User can set provider/model/reasoning/YOLO for the CLI session and have them reach the runner each chat turn.

## 2. Parent Links

- coding plan: CP-56 P-2, D-5, D-11
- tech design: 04-02 existing runner HTTP contract
- system spec: n/a; task is a client-only delta
- specific upstream ids: CP-56 A2.1–A2.9

## 3. Trigger

Task-279 provides a streaming chat shell but cannot satisfy the required provider/model/reasoning/YOLO controls.

## 4. Exact Change

### 4.1 Files

```text
internal/tui/app/controls.go
internal/tui/app/controls_test.go
internal/tui/app/slash.go          # begin slash router
internal/tui/app/slash_test.go
internal/tui/client/providers.go   # existing GET /providers
```

### 4.2 Types

```go
type SessionControls struct {
    ProviderKey     string
    Model           string
    ReasoningEffort string
    Yolo            bool
}

func (s *SessionControls) ApplySlash(name string, args []string) (userMsg string, err error)
func (s SessionControls) ApplyToStart(in *client.StartRunInput)
func (s SessionControls) ApplyToTurn(in *client.TurnInput) // set Model, YoloMode ptr, ReasoningEffort
func OptionsFromProviders(providers []client.Provider, selectedProvider string) ControlOptions
func (c *Client) ApplyGrokYoloPosture(ctx context.Context, enabled bool) error
```

### 4.3 Slash

| Cmd | Behavior |
|---|---|
| `/provider [key]` | get/set; error if run started && changing |
| `/model [name\|list]` | get/set |
| `/reasoning [level]` | get/set |
| `/yolo [on\|off\|toggle]` | get/set |
| `/new` | clear RunID/StepID/AfterSeq/timeline (minimal) |

`/provider list`, `/model list`, and `/reasoning list` render only advertised options. Setting an unknown/unavailable model or unsupported reasoning value fails locally.

### 4.4 Flags

`--provider`, `--model`, `--reasoning`, `--yolo` (bool) on main → seed `SessionControls`.

### 4.5 Wire into send path

In Task-279 `startOrContinueCmd`, call `ApplyToStart` / `ApplyToTurn` before HTTP. The slash command path awaits Grok posture success before flipping local YOLO state.

## 5. Touched Areas

- files: `apps/local-runner/internal/tui/app/{controls,slash}.go`, matching additive tests, `internal/tui/client/providers.go`
- modules: existing `flowpilot-runner`
- routes: existing `GET /providers`
- tables: none

## 6. Acceptance Check

- [ ] A2.1–A2.9 green, including live model/reasoning option projection, invalid-option rejection, explicit model clearing, and Grok posture ordering
- [ ] Manual: toggle yolo; change model between turns; provider lock after first turn

## 7. Out of Scope

- Skills, images, flow, full resume history UI

## 8. Completion Notes

- result: pending
- follow-ups: Task-281
- upstream docs updated: CP-56-Test-Steps evidence when complete
