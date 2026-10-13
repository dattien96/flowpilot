# BUG-669 — Reattach Leg Inherits `flowEngineDriven` And Becomes A Phantom Loop Owner

## Metadata

- Document ID: `BUG-669`
- Title: `reattach leg with inherited flowArm=started/flowEngineDriven latches its own flow loop — stalls, parks, blocks relay`
- Phase: `bugfix`
- Status: `FIXED — CA-1237`
- Owner: `flowpilot-runner`
- Created: `2025-01-24`
- Last Updated: `2025-01-24`
- Parent Documents: `CP-89 (flowArm latch)`, `BUG-1187` (flow_in_progress rotate guard)
- Related Documents: `BUG-666`, `BUG-668`, `BUG-660`
- Tags: `chat-legs, flow-latch, orchestration, severity:high`

## AI Quick View

### Summary

- `createRun` reattach inherit (`interactive_handlers.go:1350-1358`) stamps `flowArm=started` + `flowEngineDriven=true` + `workflowID` on the new leg — intended to prevent a second launch.
- But `flowEngineDriven=true` on a `parentRunID==""` chat leg makes every `loopStateFor(leg.id)`/`mutateLoop(leg.id)`/`ensureLoopLocked(leg.id)` consumer treat the leg as a **flow owner**: a loop is materialized on `leg.id`, the hub-stall watchdog arms on it, no progress accrues (real flow runs on the ORIGINAL leg's loop id), watchdog parks it `blocked`.
- Result: relayed turns to the leg fail `flow_awaiting_user` — the successor leg that exists to host relayed turns refuses them. Live `run-816045` latched `flowpilot-core-flow-pack`, loop parked ~2min, every relay rejected.

### Current Ask

- A reattach leg must remain a **session host**, not a loop owner: prevent loop materialization/stall-watch on legs that inherited (not launched) `flowEngineDriven`, or stop the inherit from claiming ownership.

### Key Decisions

- `V-1` The inherit's purpose is launch-dedup only ("never re-arm into a second launch") — it must not grant loop ownership.
- `V-2` The real flow's loop is keyed by the original hub run id; sibling legs must never accumulate loop state under their own id.

### Source Refs

- live `run-816045` (legSeq 2 reattach): own loop `flowpilot-core-flow-pack` observed `blocked`; turns rejected `flow is waiting for your decision`. Live `run-816751` (legSeq 3): same phantom latch → cancelled.
- `interactive_handlers.go:1322-1363` (inherit block); `agent_orchestrator.go:494` (`ensureLoopLocked` zero-value materialization for any id).

## 1. Issue Summary

Reattached chat legs inherit the full started-latch so a forward cannot re-launch the flow — correct intent. The side effect: `flowEngineDriven=true` + `workflowID` set on a root (`parentRunID==""`) chat-kind run qualifies it as a flow hub. Orchestration code that materializes loop state under the run id then creates a phantom loop on the leg; the stall watchdog parks it; `startTurn`'s root-admission guard (`flow_awaiting_user` on blocked loop) rejects every turn relayed to it — the exact legs meant to serve as relay targets become unusable.

## 3. Environment and Reproduction

- environment: chat whose flow launched on leg A; leg B minted via reattach/switch inherit while flow runs
- reproduction: mint leg B (inherit fires) → inject any turn on B (relay or direct) → `mutateLoop`/`ensureLoopLocked` materializes a loop keyed `B.id` → stall watchdog parks it → subsequent turns fail `flow_awaiting_user`.
- frequency: deterministic once a reattach leg exists while the parent flow is alive

## 4. Expected vs Actual

- expected: leg B hosts turns as a passive session; loop state lives only on the real hub
- actual: leg B materializes a phantom loop that stalls and blocks it

## 5. Impact

- severity: high — every successor leg created for provider rotation during a live flow inherits the poison; relay targets die on arrival

## 6. Root Cause

- confirmed cause: inherit block stamps `flowEngineDriven=true` (launch-dedup) but that flag doubles as the "am I a loop owner" discriminator everywhere (`parentRunID=="" && flowEngineDriven`). Two meanings on one flag.
- evidence: `interactive_handlers.go:1350-1358`; `interactive_service.go` loop-owner checks (`flowRootIDLocked`, root-admission `flow_awaiting_user`, stall watchdog arming).

## 7. Fix Strategy

- `F-1` Distinguish ownership from dedup: add `flowLoopOwner bool` (or equivalent marker) set ONLY on the run that actually launched the flow — the reattach inherit keeps `flowArm=started`/`chatFlowRef` for dedup but does NOT set `flowEngineDriven`/`workflowID`, OR sets a separate `flowDedupOnly` flag. All `parentRunID=="" && flowEngineDriven` owner-checks then see the truth.
- `F-2` Guard the materialization seam: `ensureLoopLocked` callers that create state for a `parentRunID==""` leg which never launched a flow should no-op (belt: `switchLegFlowRefFallback`-style discriminator — `flowEngineDriven` true but `workflowID` empty AND no loop ever transitioned = passive leg).
- `F-3` Additive tests: reattach leg with inherited started-latch → injected turn → no loop materialized under leg id; stall watchdog never parks the leg.
