# CA-181: Persist Tracked Flow Topology Across Restart (BUG-NOTE-CP42 #16); BUG#31 Investigated

## Scope

Verified and fixed a P2 issue from `requirements/09-BugFix/todo/BUG-NOTE-CP42.md`: a resolved flow's tracked edges/nodes (`interactiveRun.activeFlowEdges`/`activeFlowNodes`) never survived a runner restart. Also investigated a related P2 item (#31) about cohort barrier state and determined it is not reproducible in the current architecture — documented rather than "fixed."

## BUG#16 — activeFlowEdges/activeFlowNodes lost across restart

`activeFlowEdges`/`activeFlowNodes` (set by `startResolvedFlow`/`startInlineEntryChain`, read by `tryAdvanceFlowFromNode` and `resolveContinueBackEdgeTarget`) lived only in the in-memory `interactiveRun`. `ProviderSessionState` persisted `LoopState`/`AutoOrchestrate`/`FlowCohortID` but not the tracked topology, and `sessionStateOf` never snapshotted it. Confirmed reachable: the affected run is the top-level chat/hub run, which `loadPersistedRun`/`reconstructRun` *does* restore when a user reopens it after a restart (only child agent runs are excluded, via the `RunKind != "chat"` guard in `loadPersistedRun`). A user reopening a chat mid-flow after a restart silently lost edge-driven back-edge routing (Task-180) and forward auto-advance (Task-180 follow-up / CA-163), reverting to legacy `isCoderRun` role matching.

**Fix**: added `ActiveFlowEdges []agentpack.FlowEdge` / `ActiveFlowNodes []agentpack.FlowNode` to `ProviderSessionState` (`workflow_store.go`). `sessionStateOf` (`interactive_service.go`) now snapshots both from the run; `reconstructRun` (`interactive_resume.go`) restores both onto the rebuilt run. Also wired the local file session store's NDJSON record shape (`ndjsonSessionRecord`, `local_file_session_store.go`) to round-trip both fields through disk.

**Scoped out**: the Supabase-backed session store (`SupabaseWorkflowStore.UpsertProviderSession`/`GetProviderSession`) has a separate, larger, pre-existing gap — it doesn't persist `LoopState`, `AutoOrchestrate`, or `FlowCohortID` either, none of which this bug note calls out. Fixing that is a bigger, separate task; not attempted here to avoid scope creep beyond what BUG#16 actually reported (which cites `workflow_store.go`/`local_file_session_store.go` specifically, not the Supabase store).

## BUG#31 — cohort expected count lost across restart (investigated, not reproducible)

The persistence gap itself is real: session state persists `FlowCohortID` but not the cohort's expected member count, and `cohortComplete` requires `cohortExpected > 0`. But tracing the actual failure mechanism the note describes — "a reviewer cohort completes after restart and its result never joins" — found it unreachable: `reconstructRun` is called exclusively from `loadPersistedRun`, which rejects any session with `RunKind != "chat"`. Child agent runs (cohort members) are never persisted as `"chat"` kind — only the top-level hub run is. So after a restart, a child run is never reconstructed as a live `interactiveRun` at all; there's no code path by which a cohort member could "complete after restart," because nothing brings it back to a state where it could run a turn and emit `EventTurnCompleted` to begin with. The underlying provider CLI subprocess a child drives also isn't designed to survive/reattach across a runner restart.

Concretely: `cohortExpected` not surviving a restart is real, but currently dead code — nothing can read a stale-vs-fresh value for a cohort whose members can't be resurrected to complete in the first place. Documented in `BUG-NOTE-CP42.md` as investigated/not-reproducible rather than adding speculative persistence plumbing for an unreachable scenario, per the instruction to report false positives rather than "fix" a non-issue. Flagged as the first thing to revisit if a future feature adds child-run resumption.

## Verification

- New tests: `TestSessionStateOfSnapshotsTrackedFlowTopology` (write side), `TestReconstructRunRestoresTrackedFlowTopology` (in-memory round-trip via `loadPersistedRun`), `TestLocalFileSessionStoreActiveFlowTopologyRoundTrip` (disk persistence round-trip through a real NDJSON write + reload).
- Full suite: 1012 passed, 15 pre-existing/environmental failures (unchanged from before this fix).
- `go build ./...` clean.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-NOTE-CP42
change_type: bugfix
summary: persist activeFlowEdges/activeFlowNodes through ProviderSessionState and the local file session store so a chat reopened after a restart keeps edge-driven flow routing instead of reverting to role-name matching; investigated and documented BUG#31 (cohort expected count) as not reproducible since child runs are never reconstructed post-restart
# --->8---
