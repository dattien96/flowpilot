# Task-304: Dual Back-Edge Validator And Edge-Driven Re-entry

## Metadata

- Document ID: `Task-304`
- Title: `Dual Back-Edge Validator And Edge-Driven Re-entry`
- Phase: `task`
- Status: `inprogress`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-27`
- Last Updated: `2026-09-01`
- Parent Documents: [CP-58: Bug / Task / CP Harness With Plan Artifact And Dual Review Loops](../../07-Coding-Plan/todo/CP-58-Bug-Task-Cp-Harness-Plan-Review-Loop.md)
- Child Documents: `None`
- Related Documents: [CP-55: Flow-First Preflight Contract](../../07-Coding-Plan/done/CP-55-Flow-First-Preflight-Contract-Context-Retrieval-And-Canonical-Acceptance.md), [rag-harness.yaml](../../../apps/local-runner/internal/agentpack/flow-pack/flows/rag-harness.yaml), [pack.go](../../../apps/local-runner/internal/agentpack/pack.go), [flow_executor.go](../../../apps/local-runner/internal/runner/flow_executor.go)
- Replaces: `None`
- Tags: `agent-flow-engine, pack, validator, flow-executor, back-edge`

## AI Quick View

### Summary

- Fixes the single `continue/back` limitation (`pack.go:845` `backEdgeSources[when]` + `flow_executor.go:910` first-match) so a flow can declare **two** `when:continue kind:back` edges (`plan_synthesis→plan_writer` and `validate→implement`) needed for 11/12-step harnesses.
- Makes `ValidateFlowDefinition` key on `(from,when)` not just `when`, and makes `resolveContinueBackEdgeTarget(edges, fromNodeID)` source-aware.
- Keeps all existing single-loop flows (`rag-harness`, `review-loop`, `context-coding-review-synthesis`) green with zero behavior change.

### Current Ask

- Ship the only engine change CP-58 needs; unblock pack-level YAML for Task-305/306 to declare two loops without validator reject.

### Key Decisions

- `T-1` Key on `(from,when)` — two `continue/back` with different `From` pass, same `From+When` still fails (true duplicate).
- `T-2` Resolver stays backward-compatible: variadic `resolveContinueBackEdgeTarget(edges, from ...string)` — with `from` it matches the loop's anchor edge, without it falls back to first-match so every existing call site compiles unchanged (additive-tests-only).
- `T-3` Hub-emitted continue is disambiguated by `activeHubNodeID` (Task-235 run state), **not** by `edge.From==hub`: the code-loop hub `synthesis` emits continue on the `validate→implement` edge (From=validate, per `rag-harness.yaml:130-136`), so the resolver rule for a hub `from` is: (a) edge with `From==from`; (b) back-edge whose From is a non-hub.inline node and `from ∈ forwardReachableNodeIDs(edges, edge.From)`; (c) first-match fallback.
- `T-4` `hubInlineNodeID` first-match (`flow_step_runtime.go:577`) is safe only while a flow has ≤1 hub.inline node; dual-hub harnesses (Task-305/306) rely on `activeHubNodeID` being set whenever a hub.inline step goes RUNNING (cohort-join path `interactive_service.go:4711` + `loopIsAdvancing` sites) — single-hub flows keep the existing helper untouched.
- `T-5` Validate's own retry path (`edgeTargetFrom`, `flow_validate_audit_dispatch.go:53/601`) is already source-aware — not touched.
- `T-6` No topology change to `rag-harness.yaml` in this task; only engine.

### Constraints

- Additive, fail-closed. Existing `ValidateFlowDefinition` cases still reject writer-as-entry, missing `acceptance_nodes`, etc. (`CP-55 P-1`).
- Do not change `forwardReachableNodeIDs` semantics; it already scopes correctly per re-entry (`flow_executor.go:936`).
- Do not extend the `FlowControlInput`/`ReviewOutcomeInput` schemas (`agent_orchestrator.go:597/772`) — emitter disambiguation is run-state (`activeHubNodeID`), not payload.

### Open Questions

- `Q-1` Independent `cap` per loop vs shared `policy.cap:3` — engine already shares `cap`; scoping via `forwardReachableNodeIDs` keeps reset correct regardless. Decide in Task-305.

### Source Refs

- `CP-58` `P-1`, `P-5`; `pack.go:794 ValidateFlowDefinition`, `pack.go:845 backEdgeSources`, `flow_executor.go:904 resolveContinueBackEdgeTarget`, `flow_step_runtime.go:577 hubInlineNodeID`, `interactive_service.go:1507/2900/4711`, `agent_orchestrator.go:597/772 FlowControlInput/ReviewOutcomeInput`, `flow_validate_audit_dispatch.go:53/601 edgeTargetFrom`, `flow_executor.go:936 forwardReachableNodeIDs`, `rag_harness_live_continue_back_edge_test.go:81`.

## 1. Goal

Allow `task-harness` (11-step) and `cp-harness` (12-step) to declare two distinct `when:continue kind:back` edges without validator failure, and make `continue` re-entry resolve to the correct writer (`plan_writer` vs `implement`) based on which hub emitted it.

## 2. Parent Links

- coding plan: `CP-58` `P-1`
- tech design: `SD-19`, `SD-18` (back-edge contract), `SD-21` (topology safety)
- system spec: `SS-16`, `SS-15`
- specific upstream ids: `CP-58 P-1`, `CP-55 P-1 ValidateFlowSafetyTopology`, `pack.go:845`, `flow_executor.go:910`

## 3. Trigger

`CP-58 §3.1` task-harness needs `plan_synthesis→plan_writer` + `validate→implement` (both `continue/back`). Current validator rejects second edge as `duplicate back-edge for status "continue"` and resolver returns first-match regardless of source — blocking YAML authoring for Task-305/306.

## 4. Exact Change

- `T-1` `apps/local-runner/internal/agentpack/pack.go:845` — change `backEdgeSources map[string]string` (key `when`) to `map[string]map[string]string` or `map[string]string` keyed `from+"\x00"+when`. On `kind=="back"` check `if seen[when][from]` duplicate; else `seen[when][from]=from`. Error message includes `from` pair for diagnosability.
- `T-2` `apps/local-runner/internal/runner/flow_executor.go:910` — change to `func resolveContinueBackEdgeTarget(edges []agentpack.FlowEdge, from ...string) (string,bool)` (variadic so every existing call site — `flow_executor_test.go:976/986/989`, `phase_a_dod_test.go:400`, `rag_harness_live_continue_back_edge_test.go:81`, `interactive_service_test.go:~4953` — compiles unchanged; zero pre-existing test edits). With `from[0]` set, resolve in order: (a) edge with `From==from`; (b) if `from` is a hub.inline node, the back-edge whose From is a non-hub.inline node and `from ∈ forwardReachableNodeIDs(edges, edge.From)` (code-loop hub `synthesis` → `validate→implement`); (c) first-match fallback. Empty variadic preserves single-loop behavior exactly.
- `T-3` `apps/local-runner/internal/runner/interactive_service.go:1507` (`applyFlowControl` `NextAction=="looping"` reset) and `:2900` (`maybeReinvokeCoderForContinue`) — pass `rs.activeHubNodeID` (fallback: `hubInlineNodeID(nodes)` when the flow has exactly one hub.inline node, else `""`) as `from`. `FlowControlInput`/`ReviewOutcomeInput` carry no node/cohort id (`agent_orchestrator.go:597/772`), so the emitter id MUST come from run state. Keep the `forwardReachableNodeIDs(edges, reentryID)` call unchanged (already correct per re-entry).
- `T-4` Hub-inline activity tracking: set `rs.activeHubNodeID = node.ID` wherever a hub.inline step transitions RUNNING — the cohort-join path (`interactive_service.go:4711`), the `loopIsAdvancing` sites (`:1131/:2459/:2527/:5065`), and the hub.inline dispatch path — replacing the first-match `hubInlineNodeID(nodes)` (`flow_step_runtime.go:577`) at those sites only when the flow declares >1 hub.inline node. Single-hub flows: zero behavior change.
- `T-5` Do NOT touch validate's own retry path — `edgeTargetFrom(edges, node.ID, "continue", "back")` (`flow_validate_audit_dispatch.go:601`) is already source-aware.
- `T-6` `apps/local-runner/internal/agentpack/pack_test.go` — add `TestValidateFlowAllowsTwoContinueBackEdgesWithDifferentFrom`, `TestValidateFlowRejectsDuplicateFromWhen`, `TestResolveContinueBackEdgeIsSourceAware` (table covers `plan_synthesis→plan_writer` vs `validate→implement`, with and without `from`).
- `T-7` `apps/local-runner/internal/runner/flow_executor_test.go` — add `TestResolveContinueSourceAware` table for the same edges, plus fallback when `from` is omitted.
- `T-8` `apps/local-runner/internal/runner/interactive_service_test.go` — add `TestApplyFlowControlContinueHubRouting`: flow with both back-edges; `activeHubNodeID=plan_synthesis` + continue re-enters `plan_writer` (PENDING reset covers only `plan_writer→test_signatures→plan_reviewer→plan_synthesis`; `context`/`freeze` stay DONE); `activeHubNodeID=synthesis` + continue re-enters `implement` (reset covers `implement→validate→reviewer→synthesis`; plan-loop nodes stay DONE).

## Code Guide

### CG-1: Validator — `backEdgeSources` key change (`pack.go:845`)

**BEFORE** (current — keys on `when` only, rejects any 2nd `continue/back`):
```go
// pack.go:845
backEdgeSources := make(map[string]string, len(def.Edges)) // when -> first-seen from
// ...
if kind == "back" {
    if prev, exists := backEdgeSources[edge.When]; exists {
        return fmt.Errorf("flow %q has duplicate back-edge for status %q (from %q and %q)",
            def.ID, edge.When, prev, edge.From)
    }
    backEdgeSources[edge.When] = edge.From
}
```

**AFTER** (key on `from+"\x00"+when` — two `continue/back` with different `From` pass, same `From+When` still fails):
```go
// pack.go:845
backEdgeSources := make(map[string]string, len(def.Edges)) // "from\x00when" -> from
// ...
if kind == "back" {
    dedup := edge.From + "\x00" + edge.When
    if prev, exists := backEdgeSources[dedup]; exists {
        return fmt.Errorf("flow %q has duplicate back-edge for status %q from %q (already declared from %q)",
            def.ID, edge.When, edge.From, prev)
    }
    backEdgeSources[dedup] = edge.From
}
```

### CG-2: Resolver — source-aware `resolveContinueBackEdgeTarget` (`flow_executor.go:904`)

**BEFORE** (current — first-match, no source awareness):
```go
// flow_executor.go:904-919
func resolveContinueBackEdgeTarget(edges []agentpack.FlowEdge) (string, bool) {
    for _, edge := range edges {
        if strings.EqualFold(strings.TrimSpace(edge.Kind), "back") &&
            strings.EqualFold(strings.TrimSpace(edge.When), "continue") {
            if edge.To != "" {
                return edge.To, true
            }
        }
    }
    return "", false
}
```

**AFTER** (variadic `from` — all existing call sites compile unchanged, new call sites pass `activeHubNodeID`):
```go
// flow_executor.go:904 — SIGNATURE CHANGE (variadic, backward-compatible)
func resolveContinueBackEdgeTarget(edges []agentpack.FlowEdge, from ...string) (string, bool) {
    fromID := ""
    if len(from) > 0 {
        fromID = from[0]
    }

    // (a) Exact match: edge.From == fromID
    if fromID != "" {
        for _, edge := range edges {
            if !isContinueBack(edge) || edge.To == "" {
                continue
            }
            if edge.From == fromID {
                return edge.To, true
            }
        }
    }

    // (b) Hub-aware: fromID is a hub.inline node → find the back-edge whose
    //     From is a non-hub.inline node and fromID ∈ forwardReachableNodeIDs(edges, edge.From)
    if fromID != "" {
        for _, edge := range edges {
            if !isContinueBack(edge) || edge.To == "" || edge.From == fromID {
                continue
            }
            reachable := forwardReachableNodeIDs(edges, edge.From)
            if reachable[fromID] {
                return edge.To, true
            }
        }
    }

    // (c) First-match fallback (preserves single-loop behavior for all existing flows)
    for _, edge := range edges {
        if isContinueBack(edge) && edge.To != "" {
            return edge.To, true
        }
    }
    return "", false
}

// isContinueBack is a helper — extract from the repeated condition
func isContinueBack(e agentpack.FlowEdge) bool {
    return strings.EqualFold(strings.TrimSpace(e.Kind), "back") &&
        strings.EqualFold(strings.TrimSpace(e.When), "continue")
}
```

### CG-3: Hub-inline activity tracking (`interactive_service.go`)

**Call site 1** — `applyFlowControl` continue reset (`:1504`):
```go
// BEFORE:
reentryID, ok := resolveContinueBackEdgeTarget(edges)

// AFTER — pass activeHubNodeID from run state:
hubFrom := rs.activeHubNodeID
if hubFrom == "" {
    hubFrom = hubInlineNodeID(nodes)
}
reentryID, ok := resolveContinueBackEdgeTarget(edges, hubFrom)
```

**Call site 2** — `maybeReinvokeCoderForContinue` (`:2900`):
```go
// BEFORE:
targetNodeID, _ = resolveContinueBackEdgeTarget(parent.activeFlowEdges)

// AFTER:
hubFrom := parent.activeHubNodeID
if hubFrom == "" {
    hubFrom = hubInlineNodeID(parent.activeFlowNodes)
}
targetNodeID, _ = resolveContinueBackEdgeTarget(parent.activeFlowEdges, hubFrom)
```

**Hub-inline tracking** — set `rs.activeHubNodeID = node.ID` at cohort-join RUNNING transition:

Affected sites (all under `s.mu.Lock()`):
- `:4711` — cohort-join path (hub.inline step goes RUNNING)
- `:1131` / `:2459` / `:2527` / `:5065` — `loopIsAdvancing` sites

```go
// Pattern at each site — ADDITIVE, guarded for >1 hub.inline:
if canonical, ok := agentpack.NormalizeBehaviorID(node.Behavior); ok && canonical == "hub.inline" {
    rs.activeHubNodeID = node.ID
}
```

### CG-4: `hubInlineNodeID` guard for >1 hub.inline (`flow_step_runtime.go:575`)

**NO SIGNATURE CHANGE** — the existing function stays as-is:
```go
// flow_step_runtime.go:575 — UNCHANGED
func hubInlineNodeID(nodes []agentpack.FlowNode) string {
    for _, n := range nodes {
        if canonical, ok := agentpack.NormalizeBehaviorID(n.Behavior); ok && canonical == "hub.inline" {
            return n.ID
        }
    }
    return ""
}
```
The function returns the first hub.inline as a **fallback only**. Dual-hub flows use `rs.activeHubNodeID` set by CG-3. No edit needed.

### CG-5: Test Signatures

```go
// --- pack_test.go ---

func TestValidateFlowAllowsTwoContinueBackEdgesWithDifferentFrom(t *testing.T) {
    // Build a FlowDefinition with 2 continue/back edges:
    //   plan_synthesis -> plan_writer (continue/back)
    //   validate       -> implement   (continue/back)
    // Assert: ValidateFlowDefinition returns nil
}

func TestValidateFlowRejectsDuplicateFromWhen(t *testing.T) {
    // Build a FlowDefinition with 2 continue/back edges from SAME From:
    //   validate -> implement (continue/back)
    //   validate -> coder     (continue/back)
    // Assert: ValidateFlowDefinition returns error containing "duplicate back-edge"
}

// --- flow_executor_test.go ---

func TestResolveContinueBackEdgeIsSourceAware(t *testing.T) {
    // Table-driven:
    // edges = [
    //   {From:"plan_synthesis", To:"plan_writer",  When:"continue", Kind:"back"},
    //   {From:"validate",      To:"implement",     When:"continue", Kind:"back"},
    // ]
    // | from              | expected target | ok   |
    // |-------------------|-----------------|------|
    // | "plan_synthesis"  | "plan_writer"   | true |
    // | "validate"        | "implement"     | true |
    // | "synthesis"       | "implement"     | true | // hub-aware: synthesis ∈ forwardReachable(validate)
    // | ""                | "plan_writer"   | true | // first-match fallback
}

// --- interactive_service_test.go ---

func TestApplyFlowControlContinueHubRouting(t *testing.T) {
    // Setup: flow with both back-edges (task-harness topology)
    // Case 1: activeHubNodeID = "plan_synthesis" + continue
    //   → re-enters plan_writer
    //   → PENDING reset covers plan_writer→test_signatures→plan_reviewer→plan_synthesis
    //   → context/freeze stay DONE
    // Case 2: activeHubNodeID = "synthesis" + continue
    //   → re-enters implement
    //   → PENDING reset covers implement→validate→reviewer→synthesis
    //   → plan-loop nodes stay DONE
}
```

## 5. Touched Areas

- files: `apps/local-runner/internal/agentpack/pack.go`, `apps/local-runner/internal/agentpack/pack_test.go`, `apps/local-runner/internal/runner/flow_executor.go`, `apps/local-runner/internal/runner/flow_executor_test.go`, `apps/local-runner/internal/runner/interactive_service.go` (2 resolver call sites + hub-inline activity tracking), `apps/local-runner/internal/runner/flow_step_runtime.go` (hubInlineNodeID guarded for >1 hub.inline), `apps/local-runner/internal/runner/interactive_service_test.go`
- modules: agent-pack loader/validator; flow executor; interactive service (loop reset + hub-inline tracking); flow step runtime
- routes: none
- tables: none

## 6. Acceptance Check

- `go test ./internal/agentpack -run 'TestValidateFlow|TestResolveContinue'` green; new dual-loop acceptance passes, same-From duplicate still rejected.
- `go test ./internal/runner -run 'TestResolveContinue|TestRAGHarnessLive|TestReviewLoop|TestContextCoding'` green; `rag_harness_live_continue_back_edge_test.go:81` still passes unedited (variadic call compiles, single `continue/back` still returns `implement` with `from` omitted).
- `go test ./internal/runner -run TestApplyFlowControlContinueHubRouting` green — plan-loop continue re-enters `plan_writer`, code-loop continue re-enters `implement`, `context`/`freeze`/plan nodes keep DONE across both loops.
- Single-loop flows `rag-harness`/`review-loop`/`context-coding-review-synthesis` load via `LoadBuiltinPack` unchanged; `hubInlineNodeID` first-match behavior unchanged for them.

## 7. Out of Scope

- New harness YAML (`task-harness.yaml`, `cp-harness.yaml`) — Task-305/306.
- Plan artifact `file_artifact` bindings — Task-307 (but validator must not block them).
- Any change to `forwardReachableNodeIDs` or `ValidateFlowSafetyTopology` acceptance logic.

## 8. Completion Notes

- result: implemented on branch cp58-harness-dual-loop (commit 32bd3581). Validator keys back-edge dedup on (from,when); resolveContinueBackEdgeTarget is variadic source-aware (exact anchor / nearest forward ancestor for hub emitters / first-match fallback); both continue call sites pass activeHubNodeID; cohort-join persists the hub a joined cohort feeds into on dual-hub flows (single-hub byte-identical).
- tests added: TestValidateFlowAllowsTwoContinueBackEdgesWithDifferentFrom, TestValidateFlowRejectsDuplicateFromWhen (pack_test.go); TestResolveContinueBackEdgeIsSourceAware, TestResolveContinueBackEdgeSingleLoopFromUnchanged (flow_executor_test.go); TestApplyFlowControlContinueHubRouting (interactive_service_test.go).
- follow-ups: live /flow task-harness + cp-harness rounds on real providers (DOD-2 manual half); activeHubNodeID stays unpersisted across restart (pre-existing Task-235 limitation).
- upstream docs updated: `CP-58` `P-1`
