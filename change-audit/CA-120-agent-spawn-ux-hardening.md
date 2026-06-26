# CA-120: Agent Spawn UX Hardening

## Scope

- Child agent YOLO inheritance (BUG-129)
- Agents panel count stability + collapsible list (BUG-130)
- Spawn control disabled while main run busy (BUG-131)
- Chat slash commands `/s` (skill UI) and `/a` (spawn-agent UI) (Task-087)

## Completed

### Backend (BUG-129)
- `spawnChildRun` (interactive_service.go): extract `parentYolo := parentRun.yolo` and set `StartRunInput.YoloMode = parentYolo` so the child inherits the parent's YOLO posture.
- `runTurn`: when `in.YoloMode != nil`, persist the resolved `yolo` back to `rs.yolo` (sticky), so a spawn during the turn reads the live posture and later turns default to it.
- Added unit test `TestSpawnedChildInheritsParentYolo`.

### Frontend
- `AgentsPanel.tsx` (BUG-130/131): de-dup `agentRuns` by runId + sort by `createdAt` desc (latest on top); collapse active/closed lists to 5 with a `.ag-more` toggle; disable `+ Spawn agent` and modal `Spawn ▸` under `mainCardBusy` with tooltips.
- `store.ts` (BUG-130): added `_agentRunsLoadSeq` stale-response guard in `refreshAgentRuns`; both `agent_graph_updated` SSE branches bump the seq so a late fetch cannot overwrite a fresher snapshot.
- `styles.css`: `.ag-more` toggle style.
- `ChatInput.tsx` (Task-087): slash command classification (`/a` → agent, else skill); `/a` opens the spawn panel via `openAgentSpawnGuide` and strips the fragment; `/s` opens the full skill UI; agent command excluded from picker + `canSend`; one-action agent popover.

## GitNexus Impact

- GitNexus MCP tools were not connected in this thread and the index is flagged stale; impact assessed by local inspection per repo fallback policy.
- `spawnChildRun` / `runTurn`: changes are field propagation + a sticky write under the existing lock; no change to provider adapters or `resolveYoloPosture`.
- Frontend changes are component/store-local; the backend agent summary contract and SSE shape are unchanged.
- No HIGH/CRITICAL impact: single spawn path shared by tool + UI; YOLO derivation untouched.

## Verification

- Backend: `TestSpawnedChildInheritsParentYolo` — pass. No-regression: `go test ./internal/runner -run 'Yolo|YOLO|Spawn|Agent|Turn|Approval' -count=1` failure set identical with/without the change (clean 155 pass / 5 fail → 156 pass / same 5 fail; the 5 are pre-existing environment failures). `go build ./internal/runner/...` — pass.
- Frontend: `npx tsc --noEmit` — pass. The desktop vitest suite fails to load on this machine with a pre-existing ESM config error (`require is not defined in ES module scope`) affecting all five suites including untouched files, so component unit tests could not be executed here; verified pre-existing by stash comparison.

## Residual Notes

- The vitest ESM load failure is a pre-existing toolchain/config issue, out of scope for these fixes.
- YOLO-off parents still spawn YOLO-off children that request approval (gating preserved).

# ---8<--- flowpilot:change-ledger
feature_key: agent-spawn
source_doc_id: BUG-129
change_type: feature
summary: Agent Spawn UX Hardening
# --->8---
