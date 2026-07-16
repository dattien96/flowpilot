# Task-252: FCP Marker Run-ID Binding End-To-End

## Metadata

- Document ID: `Task-252`
- Title: `FCP Marker Run-ID Binding End-To-End`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `Codex review`
- Created: `2026-07-16`
- Last Updated: `2026-07-16`
- Parent Documents: [CP-51: Durable Turn Dispatch State Machine](../../07-Coding-Plan/todo/CP-51-Durable-Turn-Dispatch-State-Machine-And-Recovery-Reconciliation.md), [SD-24: Durable Turn Dispatch](../../06-System-Tech-Design/SD-24-Durable-Turn-Dispatch.md) (§6.6)
- Child Documents: `None`
- Related Documents: [BUG-288](../../09-BugFix/inprogress/BUG-288-Flow-Mode-Three-Tier-Gate-And-Change-Contract-Reentry-Gaps.md)
- Replaces: `None`
- Tags: `agent-flow-engine, feature-history, fcp-marker, security`

## AI Quick View

### Summary

- The outer feature-history check binds the FCP marker to `rs.id` (`interactive_service.go:4906`), but the inner `injectFeatureHistoryPromptWithSecret` re-verifies with the service secret only and passes **no** expected run IDs (`feature_history.go:33`). Since `isFlowContextHandoffWithSecret` only enforces id-binding when `len(expectedIDs) > 0` (`flow_context_handoff.go:260`), a valid **same-service** marker minted for a different run suppresses feature history.
- Thread the allowed run-ID set (self + declared source/parent) through the inner helper so every path binds secret **and** run ID.

### Current Ask

- Close the same-service cross-run FCP marker replay (`DOD-I4`) by making run-ID binding consistent on every suppression path.

### Key Decisions

- `T-1` The service path takes a **required** `MarkerVerificationContext{Secret, AllowedMarkerIDs}` — not variadic-optional IDs a new caller can forget (plan-review #11). Empty `AllowedMarkerIDs` on the service path **fails closed** (verify nothing → inject history).
- `T-2` Provenance is **recorded, not inferred**: allowed IDs = self + the run ID stored on the durable handoff binding when that handoff/restart prompt was minted. No blanket `parentRunID`/`sourceRunID` trust (siblings/unrelated sources must not be trusted).

### Constraints

- Do not weaken the legitimate cross-provider/flow handoff path (Task-224/BUG-277): genuine source/parent handoffs must still suppress correctly.
- Keep the legacy package-level `injectFeatureHistoryPrompt` (multi-secret, no expected IDs) for tests/one-shot callers, but the service path must always pass IDs.

### Open Questions

- `Q-1` **Resolved (plan-review #2 #12):** the allowed set is **recorded, not derived** — a durable `ProvenanceRunID` field written next to each minted handoff/restart/reprompt prompt (`PendingRestartProvenanceRunID`, `PendingGateRepromptProvenanceRunID`, handoff-envelope field). Verify-time = `{rs.id} ∪ {recorded provenance for the prompt being delivered}`. No topology inference; no unbounded set.

### Source Refs

- CP-51 §10.1 `DOD-I4`; BUG-288 R20-2.
- Code: `interactive_service.go:4906` (outer, binds `rs.id`), `:4910-4913` (inner call), `feature_history.go:22-40` (inner, no IDs), `flow_context_handoff.go:237-268` (`isFlowContextHandoffWithSecret`, id-binding at `:260`).

## 1. Goal

Make FCP feature-history suppression honor run-ID binding on every code path so a foreign same-service marker cannot suppress another run's feature history, while preserving legitimate source/parent handoffs.

## 2. Parent Links

- coding plan: CP-51 (`P-5`)
- tech design: [SD-24 Durable Turn Dispatch](../../06-System-Tech-Design/SD-24-Durable-Turn-Dispatch.md) (§6.6 marker contract); SD-21 (Change Contract / context handoff); SD-20
- system spec: SS-14
- specific upstream ids: CP-51 `DOD-I4`, INV-6

## 3. Trigger

`runTurn` computes `skipHistory` using `isFlowContextHandoffWithSecret(s.markerSecret, providerPrompt, rs.id)` (run-ID bound). When `!skipHistory`, it calls `injectFeatureHistoryPromptWithSecret(...)`, whose internal handoff check calls `isFlowContextHandoffWithSecret(secret, prompt)` with no expected IDs (`feature_history.go:33`). With `len(expectedIDs)==0`, the id gate at `flow_context_handoff.go:260-268` is skipped, so any same-secret marker (from another run) matches and suppresses history.

## 4. Exact Change

- `T-1` Replace the service-path helper with `injectFeatureHistoryPromptCtx(workspace, prompt, priorTurns, MarkerVerificationContext)` — the context struct is **required** (no variadic-optional IDs a caller can forget); empty `AllowedMarkerIDs` ⇒ fail closed (no suppression, history injected). Call site `interactive_service.go:4910-4913` migrates.
- `T-2` The context's inner check forwards `AllowedMarkerIDs` into `isFlowContextHandoffWithSecret` so id-binding is always enforced (the `len(expectedIDs)>0` gate at `flow_context_handoff.go:260` becomes always-armed on the service path).
- `T-3` **Durable provenance fields** (new, `omitempty`, both backends): `PendingRestartProvenanceRunID`, `PendingGateRepromptProvenanceRunID` (+ handoff-envelope provenance) — written **at mint time** wherever those durable prompts are created; `allowedFCPMarkerIDs(rs)` reads only `{rs.id}` + the recorded value for the prompt being delivered.
- `T-4` Marker MAC scheme unchanged; minting changes only by **also recording provenance**. Keep `injectFeatureHistoryPrompt` (legacy, no IDs) for tests; the service path must never use it (grep-guard in review).

### 4.1 Code guide (step-by-step)

> Fully independent of the dispatch record — can land immediately. Sketches illustrative.

**Step 1 — add `allowedRunIDs` to the helper ([feature_history.go:22](../../../apps/local-runner/internal/runner/feature_history.go:22)).** `isFlowContextHandoffWithSecret` is already variadic on `expectedIDs` ([flow_context_handoff.go:237](../../../apps/local-runner/internal/runner/flow_context_handoff.go:237), id-gate at [:260](../../../apps/local-runner/internal/runner/flow_context_handoff.go:260)); the helper just needs to forward them:

```go
// MarkerVerificationContext is REQUIRED on the service path (SD-24 §6.6).
// Not variadic: a new caller cannot compile without deciding the allowed set.
type MarkerVerificationContext struct {
	Secret           []byte
	AllowedMarkerIDs []string // empty on the service path => fail closed (no suppression)
}

func injectFeatureHistoryPromptCtx(workspace, prompt string, priorTurns []transcriptTurn, mv MarkerVerificationContext) string {
	handoff := false
	if len(mv.Secret) > 0 && len(mv.AllowedMarkerIDs) > 0 { // BOTH required — else fail closed
		handoff = isFlowContextHandoffWithSecret(mv.Secret, prompt, mv.AllowedMarkerIDs...)
	}
	// legacy no-secret path (tests only) unchanged via the old wrapper
	// ... rest unchanged ...
}
```

**Step 2 — pass the allowed set at the service call site ([interactive_service.go:4910-4913](../../../apps/local-runner/internal/runner/interactive_service.go:4910)).**

```go
providerPrompt = injectFeatureHistoryPromptCtx(
	rs.workspaceCwd, providerPrompt, transcriptTurnsFromRun(rs),
	MarkerVerificationContext{Secret: s.markerSecret, AllowedMarkerIDs: allowedFCPMarkerIDs(rs)})
```
(The old variadic `injectFeatureHistoryPromptWithSecret` is deleted from the service path — a new caller cannot compile without a `MarkerVerificationContext`.)

**Step 3 — define the allowed set.**

```go
// allowedFCPMarkerIDs: self + the RECORDED handoff provenance only (SD-24 §6.6).
// The provenance run ID is stamped onto the durable handoff/restart prompt
// binding at mint time (e.g. alongside PendingRestartPrompt / handoff envelope);
// it is NOT inferred from parentRunID/sourceRunID at verify time — a sibling or
// unrelated source must never be trusted by topology alone (plan-review #11).
func allowedFCPMarkerIDs(rs *interactiveRun) []string {
	ids := []string{rs.id}
	if p := rs.recordedHandoffProvenanceRunID(); p != "" { ids = append(ids, p) }
	return ids
}
```
Add the provenance stamp where handoff/restart prompts are minted (the durable `PendingRestart*`/handoff write) so verify-time reads a recorded fact.

**Step 4 — provenance stamping at mint + legacy helper quarantine.** The exhaustive writer inventory is: gate reprompt at `gate_hook.go:384` and `:805` → `pendingGateRepromptProvenanceRunID`; restart at `cohort_stall.go:410` → `pendingRestartProvenanceRunID`; flow handoff at `flow_executor.go:411`/`renderFlowContextPromptWithSecret` → provenance on the handoff envelope before it is persisted. Each assignment is made under the same run lock and in the same session/dispatch persistence transaction as its matching prompt. Add fields to `ProviderSessionState`, `sessionRuntimeBlob`, `interactive_resume.go` reconstruction (`:857+`), local NDJSON mapping and Supabase mapping (`supabase_workflow_store.go:165/:631`). The legacy `injectFeatureHistoryPrompt` ([feature_history.go:13](../../../apps/local-runner/internal/runner/feature_history.go:13)) survives **only** for package-level tests; a grep/AST guard enumerates these three prompt assignments and fails if any has no matching provenance assignment or if a service-path caller remains.

### 4.2 Test skeletons (`fcp_marker_replay_test.go`)

```go
func TestFCPMarker_SameServiceForeignRun_DoesNotSuppress(t *testing.T) { /* marker bound to run B, injecting for run A -> history still injected (DOD-I4); fails pre-fix */ }
func TestFCPMarker_OwnRun_Suppresses(t *testing.T)                      {}
func TestFCPMarker_RecordedProvenance_Suppresses(t *testing.T)          { /* provenance stamped at mint -> suppresses; no regression to Task-224/BUG-277 handoffs */ }
func TestFCPMarker_SiblingWithoutProvenance_DoesNotSuppress(t *testing.T) { /* topology alone (parent/sibling) never trusted */ }
func TestFCPMarker_EmptyAllowedSet_FailsClosed(t *testing.T)            { /* service path with empty AllowedMarkerIDs -> inject history */ }
func TestProvenance_RoundTripsAcrossRestart(t *testing.T)               { /* mint -> persist -> reconstruct -> deliver: provenance survives; crash between mint and deliver keeps binding */ }
func TestFCPMarker_ForeignSecret_NeverSuppresses(t *testing.T)          { /* R20-2 guard */ }
```

## 5. Touched Areas

- files: `feature_history.go` (context-struct helper + inner call), `interactive_service.go` (call site + `allowedFCPMarkerIDs` + provenance stamping at mint sites), `flow_context_handoff.go` (verify id-binding semantics), **`workflow_store.go` + `local_file_session_store.go` + `supabase_workflow_store.go`** (the durable `Pending*ProvenanceRunID` fields must persist + round-trip on both backends — plan-review #3 #10).
- modules: `internal/runner` feature-history injection.
- routes: none.
- tables: none.

## 6. Acceptance Check

- `V-1` `fcp_marker_replay_test.go`: a prompt carrying a valid same-service marker bound to run B does **not** suppress history for run A (fails on current code).
- `V-2` Legitimate handoff test: a marker bound to the **recorded provenance** (stamped at mint) suppresses history (no regression to Task-224/BUG-277); topology alone (parent/sibling without a recorded stamp) does **not**.
- `V-3` Own-run test: a marker bound to `rs.id` suppresses correctly on both outer and inner checks.
- `V-4` Cross-service test still passes (R20-2 guard): a foreign-secret marker never suppresses.
- `V-5` `go build`, `go vet`, `go test ./internal/runner` clean.

## 7. Out of Scope

- Any change to the marker **MAC scheme** (minting DOES change minimally: it additionally records the durable provenance field — `T-3`/`T-4`).
- Dispatch state machine (other CP-51 tasks) — this task depends only on P-0 and may land any time after it.

## 8. Completion Notes

- result: **done — MarkerVerificationContext required; empty allowed set fails closed; mint-time provenance fields.**
- follow-ups: remaining live crash-matrix phase-2 / real-PG optional
- upstream docs updated: evidence + task status
