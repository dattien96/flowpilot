# Task-451: `flowArm` Field — Pin, Persist, Reconstruct

- Document ID: `Task-451`
- Title: `Run-level flowArm latch (immediate|pending|started) — admission field, durable homes, restart/switch semantics`
- Phase: `task`
- Status: `todo`
- Created: `2026-09-28`
- Parent Documents: `CP-89`
- Related Documents: `CP-59` (chat SSOT), `CP-42`, `Task-452`, `Task-453`, `CP-89-Test-Steps`
- Tags: `flow`, `durability`, `restart`, `provider-switch`

## AI Quick View

### Summary

CP-89 needs a run/chat-level latch `flowArm` (`immediate` default | `pending`
| `started`) that decides whether a pinned flow starts on turn one or waits
for an explicit `forwardFlow` turn. This task lands the field itself: request
admission, working-mode fence at pin time, the pin-vs-arm split for vibe
start markers, and every durable home so `pending` survives restart and
provider switch.

### Current Ask

Implement `flowArm` end-to-end as data + durability only. The forward-turn
behavioral seam (Task-452) and transcript-packing prompt (Task-453) build on
the field but are not part of this task.

### Key Decisions

- `flowArm` belongs to the **run/chat**, never the leg — a provider switch
  creates a new leg but `pending` rides the run (CP-59 pin semantics).
- Default absent/empty → `immediate` — byte-identical behavior for existing
  clients (all current flow tests stay green untouched).
- Pin happens at run create (`flowRef`/`chatFlowRef` + `flowArm=pending`);
  vibe start markers (`vibeAwaitingLock`, `vibeSprintBudget`) arm only when
  `flowArm != pending` — split today's fused branch at
  `interactive_handlers.go:1244-1247` (F-3).
- Working-mode fence runs at **pin** time for both modes — `pending` does
  not defer `FlowAllowedForWorkingMode`.
- Reconstruct honors the latch: `pending` → chat surface, no
  `startResolvedFlow`; `started` → normal flow run, no re-arm (BUG-315).

### Constraints

- Durable record is source of truth: `flowArm` must round-trip through
  `sessions.ndjson`, Drive-restore manifest (BUG-315 `TurnCount` precedent),
  and reconstruction. Never RAM-only.
- `pending` is fail-closed: unknown/corrupt `flowArm` value on restore →
  `repair_required`-style surface, not a silent default.
- No `startResolvedFlow` call for `pending` runs on any code path — boot
  scan, continue, orphan-cure included.

## 1. Goal

A pinned-but-unstarted flow is durable, restart-safe, and switch-safe
first-class run state.

## 2. Parent Links

- coding plan: `CP-89` §3.2, §4, §8 (F-3, F-7)

## 3. Trigger

Today `FlowRef` at create both pins the flow AND arms the vibe start markers
— there is no "selected but not running" state.

## 4. Exact Change

- `T-1` `StartRunInput.FlowArm string` + validation: `"" | "immediate" |
  "pending" | "chat_then_forward"` (alias → `pending`); unknown → 400.
- `T-2` `interactiveRun.flowArm` field; stamped at create alongside
  `chatFlowRef`; `pending` requires a flow pin (else 400 `flow_arm_requires_flow`).
- `T-3` Split create-time arm: `chatFlowRef` always stamps; vibe markers arm
  only when `flowArm != "pending"`.
- `T-4` `ProviderSessionState.FlowArm` + `sessions.ndjson` round-trip.
- `T-5` Drive manifest + `reconstruct`: `pending` → do not start flow;
  `started` → BUG-315 guard keeps "never re-arm".
- `T-6` `flowArm` transition `pending→started` is the Task-452 forward seam —
  this task only provides `setFlowArmStartedLocked(rs)` helper; nobody calls
  it yet except a `stopped` terminal path.

## 5. Touched Areas

- files: `interactive_handlers.go` (create/admission), `interactive_service.go`
  (run struct, session state), session row schema, Drive manifest builder +
  reconstruct, `chat_ssot.go` (state mirror)
- modules: `runner`
- routes: `POST /runs` body field
- tables: existing durable session rows; no Supabase mutation

## 6. Code Guide Signatures

```go
// run-level latch — chat-scoped, NOT leg-scoped
type FlowArm string
const (
    FlowArmImmediate FlowArm = "immediate" // default: start on turn 1
    FlowArmPending   FlowArm = "pending"   // pinned, chat until forwardFlow
    FlowArmStarted   FlowArm = "started"   // forwarded; never re-arm
)

// interactiveRun
flowArm FlowArm

// ProviderSessionState
FlowArm string // sessions.ndjson round-trip

func parseFlowArm(raw string, hasFlowPin bool) (FlowArm, *apiErr)
func (rs *interactiveRun) setFlowArmStartedLocked()
func reconstructFlowArm(manifestField string, storedField string) (FlowArm, *apiErr) // fail-closed on unknown
```

## 7. Test Signatures

- `TestTask451_DefaultAbsentArmIsImmediate` — every current flow test shape
  unchanged: create+turn1 still starts the flow.
- `TestTask451_PendingRequiresFlowPin` — `flowArm=pending` with no
  flowRef/workflowID → 400 `flow_arm_requires_flow`.
- `TestTask451_PendingCreateKeepsVibeMarkersOff` — `vibeAwaitingLock`/
  `vibeSprintBudget` false at create; `chatFlowRef` still stamped.
- `TestTask451_WorkingModeFenceAtPinTime` — vibe flow + `dev` mode → 400
  under both `immediate` and `pending`.
- `TestTask451_SessionRowRoundTrip` — `flowArm` persisted and re-read.
- `TestTask451_RestartPendingReconstructsAsChat` — kill/reopen service: run
  comes back `pending`, no `startResolvedFlow`, no child.
- `TestTask451_RestartStartedDoesNotReArm` — BUG-315 shape preserved.
- `TestTask451_CorruptArmValueFailsClosed` — unknown manifest value →
  repair surface, never silent `immediate`.
- `TestTask451_ProviderSwitchKeepsPending` — new leg, same `flowArm`.

## 8. Acceptance Check

A `pending` run restarted at any point (mid-chat) returns as a chat run with
the pin intact and zero flow machinery armed.

## 9. Out of Scope

- `forwardFlow` turn handling and entry-prompt building (Task-452/453).
- UI Forward button (client-side, separate surface).

## 10. Definition of Done

- [ ] §6 signatures landed or deviation documented
- [ ] §7 tests additive + green; full `./internal/runner/...` suite green
- [ ] `pending`/`started` survive kill + restart + provider switch
- [ ] No existing test edited; default behavior byte-identical
- [ ] CA entry + commit `[Feature][flow-arm] ...`
