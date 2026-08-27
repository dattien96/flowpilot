# Task-283: TUI Flow And Step Slash Launch (CP-56 P-4)

## Metadata

- Document ID: `Task-283`
- Title: `TUI Flow And Step Slash Launch`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-12`
- Last Updated: `2026-08-12`
- Feature Keys: `cli-tui, agent-flow-engine, workflow-runtime`
- Parent Documents: [CP-56](../../07-Coding-Plan/inprogress/CP-56-Terminal-TUI-Chat-And-Flow-Client.md) (P-4, Q-1), [SD-19](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md), [04-02 Runner Contracts](../../10-Refactor/New-System/04-02-Phase2-Runner-Contracts-And-APIs.md), [Task-282](./Task-282-TUI-Image-Attach-Via-Existing-Turn-API.md)
- Child Documents: `none`
- Related Documents: CP-42 builtin orchestration; desktop `flowRef`/`subMode` first-turn path
- Replaces: `None`
- Tags: `cli-tui, flow, step, slash`

## AI Quick View

### Summary

- `/flow` and `/step` arm launch; resolver: builtin pack ref first, then workflow catalog (Q-1).
- Builtin → `normal_chat` run + first turn `subMode`+`flowRef`; workflow/step → `StartRun` with workflowId/stepId.
- Empty catalog → message pointing to Desktop Settings (no authoring in CLI).

### Current Ask

Implement P-4 mode switching + resolvers.

### Key Decisions

- `T-1` Builtin match uses `GET /client/chat/builtin-orchestration-options` (pass `subMode=bug` when using review-loop style options — mirror desktop).
- `T-2` Workflow match: id exact, then name case-insensitive.
- `T-3` `/mode chat|flow|step` clears incompatible arm fields.
- `T-4` First prompt after arm consumes `FirstTurnExtras`; subsequent turns omit flowRef/subMode.
- `T-5` Current handlers can return broad workflow/step catalogs; resolvers must filter `Workflow.ProjectID == selected project` and `Step.WorkflowID == selected workflow` client-side before matching.
- `T-6` Arming a different launch target is forbidden after a run starts; require `/new`.
- `T-7` Builtin option resolution carries the subMode that produced the match. For v1 `bug` options, the first turn also sends `changeType:"bugfix"` and optional `sourceDocId`, matching Desktop; later turns omit all first-turn extras.

### Constraints

- No Settings UI. No new runner endpoints. No runner core edits.

### Open Questions

- None (Q-1 resolved).

### Source Refs

- CP-56 P-4, D-4; A4.1–A4.10; M4, M6, M7.

---

## 1. Goal

Launch Flow (builtin or workflow) and Step runs from slash commands using existing APIs only.

## 2. Parent Links

- coding plan: CP-56 P-4, D-4
- tech design: 04-02 start-run/turn contract; CP-42 builtin orchestration contract
- system spec: SD-19 flow engine
- specific upstream ids: CP-56 A4.1–A4.10

## 3. Trigger

Tasks 278–282 provide catalogs, controls, and the chat turn pipeline needed to add flow/step launch arms.

## 4. Exact Change

### 4.1 Files

```text
internal/tui/app/launch.go
internal/tui/app/launch_test.go
internal/tui/client/catalog.go   # ListWorkflows, ListSteps, ListBuiltinOrchestrationOptions if not in 278
```

### 4.2 Types

```go
type Mode int // ModeChat, ModeFlow, ModeStep

type LaunchArm struct {
    Mode       Mode
    FlowRef    string
    SubMode    string // "bug" for builtin orchestration picker parity
    ChangeType string // "bugfix" for a matched bug subMode
    SourceDocID string
    WorkflowID string
    StepID     string
}

func ResolveFlowRef(ctx context.Context, c *client.Client, projectID, ref string) (LaunchArm, error)
func ResolveStepRef(ctx context.Context, c *client.Client, projectID, workflowHint, ref string) (LaunchArm, error)
func (a LaunchArm) ToStartRunInput(projectID string, ctrl SessionControls, cwd string) client.StartRunInput
func (a LaunchArm) FirstTurnExtras() (subMode, flowRef, changeType, sourceDocID string)
```

### 4.3 ResolveFlowRef algorithm

```text
1. Query each supported chat subMode (v1: `bug`) and retain which subMode produced each option.
2. if ref matches opts.flowRef (exact) → ModeFlow arm with FlowRef, matched SubMode, and `ChangeType:"bugfix"` for bug; start as normal_chat
3. wfs := ListWorkflows(); filter by projectID; match id/name → ModeFlow with WorkflowID (startRun workflow path)
4. else error: `flow not found — configure in Desktop → Settings`
```

Note: For builtin, `ToStartRunInput` still uses `chatMode=normal_chat` (desktop path); extras go on **first turn**.

### 4.4 ResolveStepRef

- Parse `workflow/step` or bare step with current workflow hint
- `ListSteps()`; filter by workflowID; match id/name
- `ToStartRunInput` sets workflowId+stepId (no chatMode)

### 4.5 Slash

| Cmd | Behavior |
|---|---|
| `/flow list` | print builtin + workflows |
| `/flow <ref>` | arm |
| `/step list` | list steps for armed/selected workflow |
| `/step <ref>` | arm |
| `/mode chat\|flow\|step` | switch + clear incompatible |

### 4.6 Send path changes

```go
if session.RunID == "" {
  in := arm.ToStartRunInput(...)
  handle := StartRun(in)
  // first turn:
  sub, fr, changeType, sourceDocID := arm.FirstTurnExtras()
  turn.SubMode, turn.FlowRef = sub, fr
  turn.ChangeType, turn.SourceDocID = changeType, sourceDocID
}
```

## 5. Touched Areas

- files: `apps/local-runner/internal/tui/app/launch.go`, `internal/tui/client/catalog.go`, additive tests
- modules: existing TUI packages
- routes: existing workflow/step and builtin-orchestration catalog routes, start-run, turn POST
- tables: none

## 6. Acceptance Check

- [ ] A4.1–A4.10 green, including project/workflow filtering, matched subMode/changeType propagation, and post-start arm rejection
- [ ] Manual M4, M6, M7

## 7. Out of Scope

- Authoring flows; agent focus UI (285); gate cards (286)

## 8. Completion Notes

- result: pending
- follow-ups: Task-284
- upstream docs updated: CP-56-Test-Steps evidence when complete
