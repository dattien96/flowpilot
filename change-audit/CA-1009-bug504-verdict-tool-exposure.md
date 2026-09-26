# CA-1009 — BUG-504: verdict-tool exposure on verdict-bearing sessions + readOnlyHint annotations

## Context

Live runs 1/3688/22241: `submit_review_outcome` was unreachable in two
distinct ways.

1. `offerReviewOutcomeTool` enumerated only mapped hub cohorts
   (plan/review via `hubInboundCohortName`), autoOrchestrate hub turns,
   and locked coders. Children executing `verdict_only` posture nodes
   (vibe-owner-debate `owner_1`/`owner_2`, cohort `owner_debate`) never
   had the tool registered — the same posture whose approval matrix
   approves the verdict call as its only allowed action. Grok correctly
   reported the tool absent and invented a
   `/tmp/submit_review_outcome.json` file-drop nobody ingests.
2. Even where registered, providers whose session modes refuse
   non-`readOnlyHint` MCP calls (devin `ask`/`accept-edits` — the
   gated-posture mapping in `resolveDevinSessionMode`) rejected the call
   upstream of the runner bridge; the runner's `allow_once` never
   mattered.

## Changes

- `interactive_service.go` (`runTurn`): resolves `verdictOnlyChild`
  before the turn lock (`flowNodePostureFor` takes `s.mu` internally) and
  offers the tool when the child executes a `verdict_only` node, or when
  a parent run hosts a flow containing a `verdict_only` node
  (`flowHasVerdictOnlyNode`) — the inline trigger/synthesis turns run on
  that session and must be able to submit the debate outcome.
- `node_isolation.go`: `flowHasVerdictOnlyNode` helper.
- `claude_mcp_server.go`: `annotations: {readOnlyHint: true}` on the four
  FlowPilot-hosted interaction tools — `submit_review_outcome`,
  `vibe-requirement-outcome`, `ask_user`, `approve`. These mutate only
  the runner's orchestration ledger, never the session environment — the
  same classification the runner's own read-only policy already applies
  (`chat_posture_policy.go`: "runner-hosted and enforced at
  SubmitFlowControl — a read_only reviewer MUST be able to submit it").
  `spawn_agent` deliberately stays unannotated (real side effect).

## Tests

`bug504_verdict_tool_exposure_test.go` (new, additive): verdict_only
child is offered the tool; verdict-flow host offers it on inline turns
without autoOrchestrate; tools/list carries `readOnlyHint` on the
interaction tools and not on `spawn_agent`.

## Verification

`go test -count=1 -run TestBug504 ./internal/runner/` — 3/3 green.
Live re-verify: devin reviewer child can submit; grok owner-debate
sessions see the registered tool.
