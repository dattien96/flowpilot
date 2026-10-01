# CA-1099: continue back-edge to a hub.inline node wedged the flow (vibe-owner-debate round 2 never dispatched)

Date: 2026-10-02
Refs: live scratch run-31884 (vibe-tasks on fp-debate-live, build
41d091ee). The owner debate mounted correctly on a gate violation, owner_1
and owner_2 (grok-4.7) completed with verdict content flowing into the
synthesis cohort note (CA-1095 verified live), and debate_synthesis
submitted `continue` → `{"status":"continue","round":1,"cap":5,
"nextAction":"looping"}`. The step timeline stamped `debate_trigger`
RUNNING and reset the owners to PENDING — and then nothing ever
dispatched again: no hub turn, no child spawn, no park, no watchdog.
Observed wedged for 7+ minutes until operator intervention.

## Root cause

`maybeReinvokeCoderForContinue` resolves the continue back-edge target
(`debate_synthesis → debate_trigger`) and then only knows how to drive
*child* work:

1. `lifecycle != reinvoke` → `spawnContinueChild()` — skipped because
   `debate_trigger` is `lifecycle: reinvoke` (`flowNodeReusesChild`).
2. `reinvoke + agent.delegate` → reuse/spawn the delegate child — skipped
   because `debate_trigger` is `hub.inline`, not `agent.delegate`.
3. Fallback `reinvokeMatchingFlowChild` — hunts a child run labeled
   `debate_trigger`; inline hub nodes are executed by the parent hub
   session itself, so no such child can ever exist. Silent false return.

Result: the back-edge target stamped RUNNING with zero dispatch — a
silent wedge indistinguishable from "still working".

## Fix

`internal/runner/interactive_service.go` — new branch in
`maybeReinvokeCoderForContinue`, before the delegate-reuse and fallback
paths: when the resolved back-edge target's canonical behavior is
`hub.inline`, re-point `activeHubNodeID` to the target (so the hub's next
verdict resolves `debate_trigger`'s own forward edges → owner re-spawn)
and re-invoke the parent hub session via `maybeAutoReinvokeHubWithPrompt`
— the same seam `dispatchHubNotifyNode` uses, including its
deferral/re-arm machinery for in-flight turns, blocked loops, and caps.

Additive: delegate (`agent.delegate`/`agent.code`) targets keep the
existing paths byte-identical; only `hub.inline` targets take the new
branch. Provider-agnostic — it re-invokes the pinned hub session.

## Tests

`ca1099_hub_inline_continue_backedge_test.go` (new, additive):

- `TestCA1099ContinueBackEdgeToInlineHubReinvokesParent`: RED before fix
  (parent hub never received a turn; debate wedged). Asserts the continue
  dispatches a turn on the parent RunID and re-points `activeHubNodeID`
  to `debate_trigger`.
- `TestCA1099ContinueBackEdgeInlineHubDoesNotSpawnChild`: asserts no
  child run labeled `debate_trigger` is spawned — the inline node must
  execute on the hub, not orphan a delegate.

## Verify

- Red: both tests fail pre-fix (no parent turn → waitLoop timeout).
- Green: `go test -run 'TestCA1099'` pass.
- Adjacent: `TestRAGHarness*`, `TestE2EReviewLoop*`, CA-1074/1091/1092/
  1095/1098 suites, `go vet` — all green.
- Live: observed on run-31884 — debate round 1 `continue` now needs a
  runner rebuild + run nudge to re-drive `debate_trigger`.
