# BUG-268: Flow Mode Coding Prompt Duplicates Feature History

## Metadata

- Document ID: `BUG-268`
- Title: `Flow Mode Coding Prompt Duplicates Feature History`
- Phase: `bugfix`
- Status: `done` — fixed and verified live 2026-07-09, found while manually re-running CP-44 E2E cases 11.1/11.2.
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-09`
- Last Updated: `2026-07-09`
- Parent Documents: [CP-44: Pluggable Context Source Registry](../../07-Coding-Plan/done/CP-44-Pluggable-Context-Source-Registry.md), [Task-169: Flow Context Package Renderer](../../08-Task/done/Task-169-Flow-Context-Package-Renderer-And-Coding-Handoff.md)
- Child Documents: `none`
- Related Documents: [BUG-243: Flow Mode Validate And Audit Behaviors Disconnected](../../09-BugFix/done/BUG-243-Flow-Mode-Validate-And-Audit-Behaviors-Disconnected-From-Task-170-171.md) (found in the same live E2E session)
- Replaces: `none`
- Tags: `agent-flow-engine, flow-mode, context-source, feature-history, prompt-composition`

## AI Quick View

### Summary

- Every Flow Mode Coding step's (agent.delegate, e.g. rag-harness's `implement`) turn-1 prompt duplicated the feature's change history: once inside the rendered `Flow Context Package` (`### Change History`), and once more in an outer preamble (`## Prior work on "X"`) that `injectFeatureHistoryPrompt` is supposed to skip whenever a Flow Context Package is already present.
- `isFlowContextHandoff` (the skip check) used `strings.HasPrefix`, requiring the package marker `[FlowPilot flow context package]` to sit at position 0 of the prompt. It never does for a freshly spawned Coding agent: `composeAgentSpawnPrompt` wraps `[agent system prompt]` + `[FlowPilot sub-agent — ...]` AROUND the already-package-embedded prompt `startInlineEntryChain` built, pushing the marker into the middle of the string.
- Net effect: every Flow Mode Coding turn's provider prompt carried the same feature history and (when chat-summary was enabled) chat-discussion text twice — wasted tokens on every single Coding turn, not just an edge case.

### Current Ask

- Fix `isFlowContextHandoff` so it correctly detects an already-embedded Flow Context Package regardless of what legitimately wraps it, and add a regression test that exercises the real spawn-wrapped shape (not just the isolated unwrapped shape the existing unit tests used).

### Key Decisions

- `K-1` **Fix at the detection boundary, not the composition order.** `composeAgentSpawnPrompt` wrapping the agent identity line around the rest of the prompt is legitimate and load-bearing (BUG-128: keeps the same agent producing an identical prompt shape across providers) — the bug is `isFlowContextHandoff` assuming it would never have to look past position 0, not the wrapping itself.
- `K-2` **`Contains`, not a smarter parse.** The marker is a unique, deliberately-placed sentinel string (`flowContextHandoffPrefix`); switching `HasPrefix` → `Contains` is the minimal fix that stays correct under any future wrapper text placed ahead of it, without needing to track/strip that wrapper first.

### Constraints

- Must not change the shape of the rendered `Flow Context Package` itself, `ComposeFlowCodingPrompt`'s output for the case the existing unit tests already assert on (Task-169's contract), or `composeAgentSpawnPrompt`'s own ordering (BUG-128).

### Open Questions

- None.

### Source Refs

- `apps/local-runner/internal/runner/flow_context_handoff.go:15-19` (`isFlowContextHandoff`, the fix).
- `apps/local-runner/internal/runner/feature_history.go:12-20` (`injectFeatureHistoryPrompt`, the consumer whose skip check silently failed).
- `apps/local-runner/internal/runner/interactive_service.go:1466-1479` (`composeAgentSpawnPrompt`, the wrapper that breaks a naive prefix check).
- `apps/local-runner/internal/runner/flow_executor.go:336-338` (`startInlineEntryChain`, embeds the package into the raw prompt BEFORE `composeAgentSpawnPrompt` wraps it).

## 1. Issue Summary

A Flow Mode Coding step's first turn always carried the feature's change history (and chat-discussion summary, when enabled) twice in its provider prompt: once from the intentional `Flow Context Package` render, once more from a legacy preamble injector whose own duplicate-prevention check never actually fired for this shape.

## 2. Parent Links

- impacted coding plan: `CP-44` (its own E2E test matrix §11.1/§11.2 is where this was found while re-verifying)
- impacted tech design: none identified
- impacted system spec: none identified

## 3. Environment and Reproduction

- environment: any Flow Mode flow whose entry is an inline `context.produce` node forward-edged to an `agent.delegate` Coding node (the built-in `rag-harness` shape; also CP-45's `context-coding-review-synthesis`), any project with existing feature change-audit history.
- reproduction steps:
  1. Run any such flow via "Run Flow" on a feature with at least one committed change-audit entry.
  2. Inspect the spawned Coding agent's (e.g. `coder`) turn-1 provider prompt (`.flowpilot/runs/<runId>/<childRunId>/prompt-turn-*.txt`).
  3. Observe `## Prior work on "<feature>"` appears once BEFORE the `---` separator (the legacy preamble), and the identical history appears again under `### Change History` inside the `[FlowPilot flow context package]` block further down.
- frequency: deterministic — every Flow Mode Coding turn-1 prompt for a feature with any history, not an edge case or a race.

## 4. Expected vs Actual

- expected: the feature's change history (and chat-discussion summary) appears exactly once in the Coding agent's prompt, inside the rendered `Flow Context Package` — `injectFeatureHistoryPrompt`'s own doc comment states this is the intended contract ("A flow context package (Task-169) carries its own history block; skip to avoid duplicating the same feature history in the Coding prompt.").
- actual: it appeared twice, roughly doubling that portion of every Coding turn's prompt size (measured live: 4041 bytes with the bug vs 1733 bytes for the same restricted-source case once fixed).

## 5. Impact

- users affected: every Flow Mode run reaching a Coding step on a feature with existing history — i.e. nearly all real usage past the first turn on a feature.
- workflows affected: token cost on every Coding turn; no correctness/behavioral impact (the agent still saw the right history, just twice), but it silently violated Task-169/CP-44's own stated no-duplication contract and inflated prompt size on every single Coding turn in Flow Mode.
- severity: medium — no functional breakage, but a real, live, always-on inefficiency plus a genuine contract violation that CP-44's own E2E test matrix is meant to catch.

## 6. Root Cause

- hypothesis: `isFlowContextHandoff`'s `strings.HasPrefix` check was written and unit-tested only against `ComposeFlowCodingPrompt`'s own direct output (marker legitimately at position 0 in that isolated shape) and never against a turn-1 prompt as actually assembled for a freshly spawned agent, where `composeAgentSpawnPrompt` (BUG-128, added independently) wraps agent-identity text around the already-package-embedded prompt.
- confirmed cause: reproduced deterministically with `go test`, both against a throwaway diagnostic harness and the added permanent regression test (`TestFlowCodingPromptSpawnWrappedDoesNotDuplicateHistory`) — driving the real `rag-harness` flow end to end via `startResolvedFlow` and capturing the actual provider prompt sent for the spawned coder's turn 1. Confirmed the same duplication and fix live against the real desktop app (`.flowpilot/logs/features/agent-flow-engine/run-13065.ndjson` and `run-13592` pre-fix; `run-4291`/`run-4357` post-fix, both clean).
- evidence: see Source Refs; the existing `TestFlowCodingPromptIncludesPackageOnce` test never caught this because it calls `injectFlowContextIfCoding` with a plain, unwrapped coding instruction string — a shape that never occurs for a real spawned agent.

## 7. Fix Strategy

- `F-1` Change `isFlowContextHandoff` from `strings.HasPrefix(strings.TrimSpace(prompt), flowContextHandoffPrefix)` to `strings.Contains(prompt, flowContextHandoffPrefix)`. The marker is a unique sentinel string; requiring it at position 0 was never necessary for correctness, only an accident of what the original unit tests happened to construct.
- `F-2` Add `TestFlowCodingPromptSpawnWrappedDoesNotDuplicateHistory` (`flow_context_handoff_test.go`), which drives the real `rag-harness` flow through `startResolvedFlow` with a fake provider adapter capturing the spawned coder's actual turn-1 prompt, asserting `"Prior work on"` and `flowContextHandoffPrefix` each appear exactly once — closing the exact blind spot the pre-existing isolated-string tests had.

## 8. Validation

- `V-1` **Unit regression** — done: `TestFlowCodingPromptSpawnWrappedDoesNotDuplicateHistory` passes post-fix; confirmed it fails pre-fix by re-running with the fix reverted (`git stash`) before restoring.
- `V-2` **Existing contract tests unaffected** — done: `TestFlowCodingPromptIncludesPackageOnce`, `TestFlowCodingPromptIncludesPlanContextPackage`, `TestE2EPlanCodingFlowContextHandoff` all still pass (the `Contains` relaxation is a strict superset of what `HasPrefix` accepted).
- `V-3` **Live E2E, both CP-44 cases** — done: re-ran §11.1 (`run-4352`/child `run-4357`, built-in `rag-harness`, default 3 sources) and §11.2 (`run-4286`/child `run-4291`, cloned flow, `feature.history`-only binding) against the real desktop app. Both prompts now carry `### Change History`/`### Prior Discussion` exactly once each, with no outer preamble duplication — see CP-44 §11.1/§11.2 VERIFIED notes for the exact run/file references.

## 9. Regression Guard

- tests: `flow_context_handoff_test.go` (+1 new test). `go test ./...` (local-runner): 1 failing test (`TestStartInteractiveAuthLaunchesFromWorkspace`, an `agy` binary-path quoting assertion), confirmed pre-existing and unrelated by reproducing it identically with this fix stashed out.
- alerts: none.
- audit checks: this note is the closing record.

## 10. Follow-Up Document Updates

- upstream docs: [CP-44](../../07-Coding-Plan/done/CP-44-Pluggable-Context-Source-Registry.md) §11.1/§11.2 VERIFIED notes updated with the post-fix re-verification run references.
