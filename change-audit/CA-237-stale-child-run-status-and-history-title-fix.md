# CA-237: Reconcile Child Run Status On Flow Done; Stop Internal Prompts From Overwriting Chat-History Title

## Summary

BUG-235 tracked four observations from live testing after BUG-234. Fixed two genuine bugs: (1) a reviewer child agent could stay "running" in the Agents panel and the main chat's inline run card even after a flow completed cleanly, and (2) a flow run's chat-history title could be overwritten by an internal engine prompt ("[flow-engine] Agent results ready...") instead of showing the user's original request. A third item (returning to an active flow after switching chats) is investigated and documented as a deferred UX design question, not fixed this round. A fourth ("main agent doing the reviewer's work") was confirmed not a bug via code inspection.

## What Changed

### Runner (`apps/local-runner`)

- `flow_step_runtime.go` `markFlowRunComplete`: now calls new `reconcileChildRunsOnFlowDone`, which force-settles any child run that is not `turnInFlight` and still `running`/`waiting_approval`/`waiting_question` to `completed` (both the run's own status and its orchestrator summary), emitting one fresh `agent_graph_updated` so the desktop clears it immediately instead of waiting for an event that will never come.
- `interactive_service.go` `startTurn`: `lastPrompt` (the field the chat-history list titles a run from) is no longer overwritten by an internal `"[flow-engine]"`-prefixed turn (hub auto-reinvoke, coder re-entry, Continue-resume) unless it's still unset — so a flow run's history title stays the user's original request.
- Tests: `TestMarkFlowRunCompleteSettlesLingeringChildAgentRun`, `TestStartTurnPreservesOriginalPromptOverInternalFlowEngineTurns`.

### Desktop (`apps/desktop-flowpilot`)

- `state/store.ts` `mergeAgentRunsById` (now exported): treats agent-run status as monotonic — an incoming non-terminal status can no longer overwrite an existing terminal one. This closes the actual race behind the stale-"running" symptom: `refreshAgentRuns`'s HTTP request is fire-and-forget and can be captured server-side before a child finishes, then resolve and land after the SSE event that already correctly marked it terminal; unconditional "incoming wins" let that stale snapshot revert the correct status, and since a terminal child fires no further event, it stayed wrong forever.
- Tests (`store.test.ts`): three new `mergeAgentRunsById` cases (revert-prevention; still-updates for a never-terminal run; terminal→terminal corrections still land).

### Docs

- `requirements/09-BugFix/done/BUG-235-...md` — investigation of all four observations, decisions (`D-1`..`D-4`), DoD, code change plan, validation. #3 (cross-chat/project active-run navigation) is documented but explicitly NOT implemented — it needs a UX design decision (`Q-1`) first, not a contained bug fix.

## Verification

- `go build ./...` / `go vet ./internal/runner/...` — clean. Desktop `npm run typecheck` / `npm run build` — clean.
- `go test ./internal/runner/...` — 13 pre-existing environment-specific failures only (identical baseline through BUG-233/234); no new failures.
- New tests all green: 2 Go (backend reconciliation, lastPrompt preservation), 3 desktop (`mergeAgentRunsById` monotonicity + edge cases).
- Not executed: a live click-through in the running desktop app (same sandbox limitation as prior CA notes this session — no reachable local-runner backend).

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-235
change_type: bugfix
summary: Settle lingering child agent runs when a flow completes and stop internal flow-engine prompts from overwriting a run's chat-history title
# --->8---
