# BUG-334: opencode spawn_agent fails "MCP -32000 Connection closed" — child session/new resets the shared process's MCP client

## Metadata

- Document ID: `BUG-334`
- Title: `opencode spawn_agent fails MCP -32000 Connection closed — child session/new resets the shared process's MCP client`
- Phase: `bugfix`
- Status: `in_progress`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-30`
- Last Updated: `2026-08-30`
- Feature Keys: `ai-providers`
- Parent Documents: [CP-57-Test-Steps](../../07-Coding-Plan/done/CP-57-Test-Steps.md) (section F), [BUG-329](../todo/BUG-329-Opencode-Midchat-Model-Switch-Session-Load-No-SessionId.md) (shared-process reuse it builds on), [CA-690](../../../change-audit/CA-690-opencode-always-ask-permission-overlay.md)
- Child Documents: `none`
- Related Documents: runner.log 2026-08-30 13:25:04–13:25:12 (run-368168 / run-368174 / run-368185), tui.log pid 80544
- Replaces: `none`
- Tags: `opencode, acp, mcp, spawn-agent, process-scope, severity-high`

## AI Quick View

### Summary

- Operator test F1 (`flowpilot_spawn_agent agent=helper prompt="trả lời: child-ok" wait=true`) failed twice within ~400ms each with `MCP error -32000: Connection closed`; the child kept running in the background and finished AFTER the parent turn had already ended (`child terminal ... completed finalMsgLen=8` at 13:25:07/12 vs tool failures at 13:25:04.9/06.8) — the result never reached the model.
- Runner log proof of the mechanism:
  1. spawn #1 created child `run-368174` whose turn opened `session/new` (id=6) on the **shared** `opencode acp` process (BUG-329 same-scope reuse) carrying the CHILD's per-turn MCP token.
  2. opencode keys its MCP clients by server NAME per PROCESS — the child's `session/new` replaced the "flowpilot" connection mid-parent-turn, so the parent's in-flight `tools/call` died with the connection.
  3. The model's retry then rode the REPLACED connection: `[agent-spawn] request parent="run-368174"` (the first CHILD — not the chat run!) and spawned a nested grandchild `run-368185`. Both children completed; both results were lost.
- Same-class hazard confirmed in `FetchOpencodeModelVariants`: its probe `session/new` sends EMPTY `mcpServers` on the shared process — a probe landing mid-parent-turn would kill an in-flight MCP call too. (The CA-690-review "sessions are independent" accepted-tradeoff note is superseded: they are independent for prompts, NOT for the process-level MCP client.)

### Fix (BUG-334)

1. `ensureOpencodeProcessSegmented(ctx, scopeBase, scopeSegment, …)` (opencode_process.go): handles gain `scopeBase`/`scopeSegment`; the account-switch reclaim closes by BASE only, so isolating a child NEVER tears down a live parent; within one segment the BUG-329 same-scope reuse rules apply unchanged. Legacy `ensureOpencodeProcess(scopeKey, …)` = segmented with segment "" (old tests untouched).
2. Child runs isolate: `runTurn` passes `opencodeChildScopeHint(rs)` (child run id when `parentRunID != ""`, else "") through the new `ProviderRegistry.AdapterWithScope` → `newAdapterForTurn(model, effort, childScope)` → scope `account|child:<runID>`. Grok's registration accepts and ignores the hint (its process keying is model-based; no behavior change).
3. Child cleanup: `CloseOpencodeProcessesForChildRun(runID)` tears the child's isolated process down on the child's terminal event (chat-scope segment "" never touched) — short-lived children do not leak processes.
4. Variants probe: `FetchOpencodeModelVariants` probes on segment "probe" and closes the throwaway process after each probe.
5. Chat turns keep segment "" → the shared chat process and BUG-329 mid-chat model-switch reuse are intact.

## Validation

- `bug334_opencode_child_process_scope_test.go` (7, additive): child segment spawns its own handle and leaves the parent alive; same-child follow-up reuses its own process; account-base switch still reclaims; per-child cleanup removes exactly that child's handle (empty id no-ops); scope hint only for spawned children; registry threads the hint (legacy `Adapter` passes ""); probe segment isolated from chat scope.
- R1: old opencode suites (`Opencode|CA679|Bug329|Bug331|ChatPosture|Bug333`) green; full runner package failures are pre-existing — stash-verified for the two not in the recent baseline (`TestSupabaseCatalogStoreShaping`, `TestResumeFlowWithFeedbackAfterEscalate` fail identically without the change; `TestStartTurnGrokCrossAccountLegacyThread…` passes in isolation → order-dependent pre-existing).
- Live verify (operator): rebuild → re-run F1/F2/F3 — expect the card-free spawn to return `child-ok` in the SAME turn; runner log should show exactly one `[agent-spawn] request parent=<chat-run>` per call and no nested parent chain.

## Residual notes

- Every opencode turn still re-registers the per-turn MCP connection at `session/new`; that is harmless for SEQUENTIAL turns (the previous turn has ended) and only mattered mid-flight — which the child/probe isolation now prevents.
- If a child run ever receives a follow-up turn after its process was closed at terminal, ensure respawns a fresh process and `session/load` adopts per BUG-329 (history for that late follow-up is lost — children are one-shot today).

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: CP-57
change_type: bugfix
summary: Isolate opencode child-run and variants-probe ACP processes by scope segment so child session/new cannot reset a parent turn's MCP connection
# --->8---
