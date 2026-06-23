# BUG-129: Child Agent Ignores Parent YOLO And Requests Approval

## Metadata

- Document ID: `BUG-129`
- Title: `Child Agent Ignores Parent YOLO And Requests Approval`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `self-review`
- Created: `2026-06-23`
- Last Updated: `2026-06-23`
- Parent Documents: [CP-19: Multiple Agents](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md), [SD-16: Agent Spawn And Tool-Calling Design](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md)
- Child Documents: `None`
- Related Documents: [BUG-128: Inconsistent Agent Spawn Prompt Composition](./BUG-128-Inconsistent-Agent-Spawn-Prompt-Composition-Across-Providers.md), [CA-120: Agent spawn UX hardening](../../../change-audit/CA-120-agent-spawn-ux-hardening.md)
- Replaces: `None`
- Tags: `multi-agent, yolo, approval, spawn, codex, claude, runner`

## AI Quick View

### Summary

- With YOLO enabled on the parent run, a child spawned via `spawn_agent` still prompts for approval on gated actions (e.g. file writes), stalling the (usually `wait=true`) parent turn on the child's approval gate.
- Root cause: `spawnChildRun` never copies the parent's YOLO posture into the child's `StartRunInput`, so the child run is created with `yolo=false`; additionally `runTurn` never persisted a per-turn YOLO override back onto the run, so a UI-toggled YOLO was invisible to a spawn happening during that turn.
- YOLO works for the main agent (verified by Test 2 regression) because the main turn resolves YOLO per-turn; only children were affected.
- Fix: inherit `parentRun.yolo` into the child `StartRunInput`, and make the per-turn YOLO sticky on the run so a spawn during the turn reads the live posture. Both Claude and Codex children then derive their sandbox/approval from the inherited posture.

### Current Ask

- Make a spawned child agent honor the parent's YOLO posture so YOLO-on children auto-approve instead of requesting approval, on both Codex and Claude (Test 1 cases 18 and 20).

### Key Decisions

- `V-1` A child inherits the parent run's YOLO posture at creation time (`StartRunInput.YoloMode = parentRun.yolo`).
- `V-2` A per-turn YOLO override is sticky: `runTurn` writes it back to `rs.yolo` so a spawn during that turn (which reads `parentRun.yolo`) sees the live posture, and subsequent turns default to it.
- `V-3` No change to the YOLO→sandbox/approval derivation (`resolveYoloPosture`); the bug was missing propagation, not wrong derivation.

### Constraints

- Do not weaken gating when YOLO is off: a YOLO-off parent must still spawn a YOLO-off child that requests approval.
- Do not change per-turn override semantics for the main run (Test 2 must keep passing).

### Open Questions

- None.

### Source Refs

- `apps/local-runner/internal/runner/interactive_service.go` — `spawnChildRun` (StartRunInput build), `runTurn` (per-turn YOLO resolution), `turnBridge.RequestApproval`.
- `apps/local-runner/internal/runner/interactive_handlers.go` — `createRun` (`yolo: in.YoloMode`).
- `apps/local-runner/internal/runner/yolo_resolver.go` — `resolveYoloPosture`.

## 1. Issue Summary

A spawned child agent created while the parent had YOLO enabled still hit the approval gate for gated actions. The child run was created with the default `yolo=false` because `spawnChildRun` built its `StartRunInput` without the parent's YOLO. Compounding this, when the user toggled YOLO on per-turn (the UI sends `YoloMode` per turn rather than mutating the run), `runTurn` resolved a local `yolo` but never wrote it back to `rs.yolo`, so even reading the parent's run-level YOLO at spawn time returned the stale `false`.

## 2. Parent Links

- impacted coding plan: [CP-19](../../07-Coding-Plan/inprogress/CP-19-Multiple-Agents.md) (P-9 child gates)
- impacted tech design: [SD-16](../../06-System-Tech-Design/SD-16-Agent-Spawn-And-Tool-Calling-Design.md) (§7.3, §9, Test 1 cases 18/20)
- impacted system spec: [SS-11](../../05-System-Specs/SS-11-Workflow-With_Session.md)

## 3. Environment and Reproduction

- environment: FlowPilot local runner, parent chat with YOLO toggled ON, child spawned via `spawn_agent` (`wait=true`) on the other provider.
- reproduction steps (SD-16 Test 1):
  1. Claude chat, YOLO = true, prompt: `spawn_agent agent="coder", provider="codex", wait=true` with a child task that writes a file (case 18).
  2. Observe the child requests approval to write the file even though YOLO is on.
  3. Repeat from a Codex chat spawning a Claude child (case 20) — same failure.
- frequency: always, on both providers, whenever YOLO is on at the parent.

## 4. Expected vs Actual

- expected: with YOLO on, the child's gated actions auto-approve and the parent turn proceeds without a child approval prompt.
- actual: the child requested approval, blocking the `wait=true` parent turn behind the child gate.

## 5. Impact

- users affected: anyone running YOLO-on multi-agent flows; the child gate stalls the parent and breaks the "fire-and-forget under YOLO" expectation.
- workflows affected: every `spawn_agent` (and UI spawn) under YOLO.
- severity: High for multi-agent UX — defeats YOLO for delegated work.

## 6. Root Cause

- hypothesis: provider-specific approval handling.
- confirmed cause: two missing propagations in `interactive_service.go`:
  1. `spawnChildRun` built `StartRunInput` without `YoloMode`, so `createRun` set the child `yolo=false`.
  2. `runTurn` computed a per-turn `yolo` from `in.YoloMode` but never persisted it to `rs.yolo`, so the parent's run-level YOLO stayed `false` even after the user toggled YOLO on for the turn that triggered the spawn.
- evidence: `createRun` copies `in.YoloMode` to `rs.yolo`; `turnBridge.RequestApproval` reads the turn `yolo` derived from `rs.yolo`; `resolveYoloPosture(false).RunnerAutoApprove == false`. A stash-comparison of the unit suite shows the fix adds one passing test and zero new failures.

## 7. Fix Strategy

- `F-1` In `spawnChildRun`, extract `parentYolo := parentRun.yolo` and set `StartRunInput.YoloMode = parentYolo` so the child inherits the parent's posture.
- `F-2` In `runTurn`, when `in.YoloMode != nil`, write the resolved `yolo` back to `rs.yolo` (sticky) so a spawn during the turn reads the live posture and later turns default to it.
- `F-3` Leave `resolveYoloPosture` and per-turn override precedence unchanged.

## 8. Validation

- `V-1` `go test ./internal/runner/ -run TestSpawnedChildInheritsParentYolo -count=1` — pass. The test enables YOLO per-turn on the parent, then spawns a child via the tool path and asserts the child's first provider turn runs with `YoloMode=true` (covers both the sticky write-back and inheritance).
- `V-2` No-regression: `go test ./internal/runner/ -run 'Yolo|YOLO|Spawn|Agent|Turn|Approval' -count=1` with and without the fix — failure set identical except the new test passes (5 pre-existing environment failures remain: real-`codex`-binary resume, provider-home skill merges).
- `V-3` `go build ./internal/runner/...` — pass.
- `V-4` Manual (SD-16 Test 1 cases 18/20): YOLO-on child auto-approves on both providers; YOLO-off child still requests approval (cases 17/19 unchanged).

## 9. Regression Guard

- tests: `TestSpawnedChildInheritsParentYolo` guards inheritance + sticky write-back.
- alerts: a YOLO-on child prompting for approval indicates the inheritance regressed.
- audit checks: any new field copied from parent to child in `spawnChildRun` must keep YOLO included; per-turn overrides must remain sticky.

## 10. Follow-Up Document Updates

- upstream docs that must change: SD-16 Test 1 cases 18/20 annotations updated to FIXED; §9 already states child gates obey YOLO.
- notes left unchanged on purpose: the YOLO→sandbox/approval derivation and per-turn override precedence are unchanged.
