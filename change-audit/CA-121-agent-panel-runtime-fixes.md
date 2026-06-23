# CA-121: Agent Panel Runtime Fixes

## Scope

- "Recently closed" agents vanish while a new agent runs (BUG-132)
- UI wait=true spawn does not block the main run (BUG-133)
- `/a` slash command does not reliably open the spawn-agent UI (BUG-134)

## Completed

### BUG-132 (frontend)
- `store.ts`: added `mergeAgentRunsById`; both `agent_graph_updated` handlers now merge the SSE snapshot into `agentRuns` by runId instead of replacing, and no longer bump `_agentRunsLoadSeq`. Closed children from the HTTP superset persist across live in-memory-only SSE updates.

### BUG-133 (backend + frontend)
- `agent_orchestrator.go`: added `WaitForResult` to `AgentRunSummary`.
- `interactive_service.go`: set `WaitForResult` from the run's wait flag at spawn upsert, on status-update upsert, and in `listAgentRunSummaries`.
- `contract.ts`: added `waitForResult?: boolean` to `AgentRunSummary`.
- `AgentsPanel.tsx`: `hasBlockingChild` (running child with `waitForResult`) → `spawnBlocked = mainCardBusy || hasBlockingChild` gates the spawn button and modal.
- `ChatInput.tsx`: `hasBlockingChild` added to `canSend`; waiting placeholder when blocked by a wait=true child.
- Added unit test `TestAgentSummaryCarriesWaitForResultFlag`.

### BUG-134 (frontend)
- `ChatInput.tsx`: slash classification broadened — `"a"`/`"agent"` → agent command, `"s"`/`"skill"` → open skill UI; agent command opens the spawn panel via `openAgentSpawnGuide`.

## GitNexus Impact

- GitNexus MCP tools were not connected and the index is flagged stale; impact assessed by local inspection per repo fallback policy.
- Backend change is an additive summary field set from the existing run flag; no provider/adapter changes.
- Frontend changes are store/component-local; the backend list/SSE contracts are unchanged.
- No HIGH/CRITICAL impact.

## Verification

- Backend: `TestAgentSummaryCarriesWaitForResultFlag`, `TestSpawnedChildInheritsParentYolo` — pass. No-regression: `go test ./internal/runner -run 'Yolo|Spawn|Agent|Turn|Approval' -count=1` — with changes 157 pass / 5 fail, clean tree 156 / 5 (same 5 pre-existing env failures: real-codex resume, provider-home skill merges); +1 new test. `go build ./internal/runner/...` — pass.
- Frontend: `npx tsc --noEmit` — pass.
- NOT runtime-verified: the Electron + Go-runner desktop app was not run in this environment, and the desktop vitest suite fails to load with a pre-existing ESM config error (`vite-plugin-electron` requiring under ESM via `vite.config.ts`). BUG-132/133/134 UI behaviors need user retest.

## Residual Notes

- The vitest ESM load failure is a pre-existing toolchain issue, out of scope here; it blocks component-level automated verification.
- These fixes correct three runtime defects in the prior agent-panel work (BUG-130/131, Task-087).
