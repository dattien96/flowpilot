# BUG-243: Flow Mode `validate`/`audit` Behaviors Disconnected From Task-170/171 Implementations

## Metadata

- Document ID: `BUG-243`
- Title: `Flow Mode validate/audit Behaviors Disconnected From Task-170/171 Implementations`
- Phase: `bugfix`
- Status: `deferred` — per owner direction (2026-07-06), the validate/audit fix (F-0 mid-flow inline-node execution + F-1/F-2 handler wiring + F-3 audit UI) is the "RAG-harness pipeline execution" mechanism and is **folded into CP-43's planned context-harness rework**, not fixed as a standalone bug. This doc stays as the diagnosis of record; the fix is tracked under [CP-43](../../07-Coding-Plan/todo/CP-43-Change-Contract-And-Canonical-Intent-Signature.md). Not a blocker for the two priorities now in flight (Review-Loop-in-chat = done via BUG-244; custom-flow authoring = Task-189).
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-06`
- Last Updated: `2026-07-06`
- Parent Documents: [CP-41: RAG Harness Flow Mode](../../07-Coding-Plan/inprogress/CP-41-RAG-Harness-Flow-Mode.md), [CP-42: Flow Pack And Generic Node Behavior Refactor](../../07-Coding-Plan/inprogress/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md)
- Child Documents: `none`
- Related Documents: [Task-170: Testing Feedback Retry Loop](../../08-Task/done/Task-170-Testing-Feedback-Retry-Loop.md), [Task-171: Audit Step Draft And Commit Prep](../../08-Task/done/Task-171-Audit-Step-Draft-And-Commit-Prep.md), [Task-176: Node Behavior Registry And Dispatch](../../08-Task/todo/Task-176-Node-Behavior-Registry-And-Dispatch.md)
- Replaces: `none`
- Tags: `agent-flow-engine, rag-harness, flow-mode, behavior-registry, validation, audit-draft, regression`

## AI Quick View

### Summary

- Prepping a manual E2E test of the built-in RAG Harness Flow Mode pipeline (`context` → `implement` → `validate` → `audit`) surfaced that the last two nodes do not do what their governing tasks (Task-170, Task-171) implemented and tested in isolation.
- **SCOPE CORRECTION (2026-07-06, deeper trace): the root cause is bigger than "two stub handlers." The flow executor has NO path to execute an inline behavior node reached mid-flow (via a forward edge from a completed node).** `tryAdvanceFlowFromNode` (`flow_executor.go:440-448`) only auto-advances to `agent.delegate` targets (it spawns child runs); when a completed node's forward-`done` target is an inline node (`validate` = `command.validate`, `audit` = `artifact.audit_draft`), it logs "target node is not a spawnable delegate" and **bails**, falling back to the note+reinvoke-hub path. The behavior registry's `Dispatch` is only ever called for the flow's ENTRY inline node (`startInlineEntryChain`, `flow_executor.go:228`) and the coding-prompt context handoff (`renderFlowContextPrompt`) — never for a mid-flow inline node. So in RAG Harness, after `implement` (the coder) completes, `validate` and `audit` are **never executed at all**; the stub-handler problem below is secondary to there being no dispatch to them in the first place.
- `behaviorCommandValidate` (the registered `command.validate` handler, `apps/local-runner/internal/runner/behavior_registry_builtin.go:153-163`) never runs a shell command — it only reads `RawArgs["exitCode"]`, a value nothing in the live dispatch path ever populates. The real command-execution/retry machinery (`RunValidationCommand`, `NewFlowValidationRetryState`, `AdvanceRetryState`, `ComposeRetryPrompt` in `flow_validation_retry.go`) is fully implemented and unit-tested but has **zero production callers** — it is only invoked by its own tests.
- `behaviorArtifactAuditDraft` (the registered `artifact.audit_draft` handler, `behavior_registry_builtin.go:209-219`) is a stub that echoes `RawArgs["summary"]` back — it never calls the real `BuildAuditDraft` (`flow_audit_draft.go:68-127`), the function that actually produces the `flowpilot:change-ledger` block and commit-message suggestion. `BuildAuditDraft` is likewise only called from its own tests and one hand-driven "E2E" test that calls it directly, bypassing `startTurn`/the behavior registry entirely.
- No desktop UI surface exists anywhere to view an audit draft even if one were produced (`AuditDraft`/`EventFlowAuditDraft` — zero matches in `apps/desktop-flowpilot/src` or `packages/flowpilot-client-core/src`).
- Net effect: in a real Flow Mode run of RAG Harness today, the `validate` and `audit` nodes will not behave as designed — this is a code-level defect, not something a user can configure their way around.

### Current Ask

- Wire `behaviorCommandValidate` to actually invoke `RunValidationCommand` (needs a source for the command string and cwd — no config surface for this exists yet either, see Open Questions) and drive `FlowValidationRetryState`/`AdvanceRetryState` through the retry loop.
- Wire `behaviorArtifactAuditDraft` to call the real `BuildAuditDraft` and persist/expose its result.
- Add a desktop UI surface to display a produced audit draft before any write/commit (per Task-171's own "inspectable before any write/commit" acceptance criterion, which cannot be met with zero UI surface).

### Key Decisions

- None yet — this is filed as a diagnosed-but-unfixed defect pending a fix session.

### Constraints

- Do not weaken the existing Task-170/171 unit-level guarantees (bounded retries, max-3 cap, environment-error exclusion, non-write-without-approval) — the fix is strictly "call the already-correct functions from the live dispatch path," not a rewrite.
- Any command-string source added must not silently run arbitrary user-injected shell input without a validation/allowlist boundary consistent with the rest of the codebase's command-execution conventions.

### Open Questions

- `Q-1` Where should the validate node's shell command come from? `.flowpilot/guard/test_baseline.json`'s `test_command` field (owned by a separate CP-35 gate/regression-guard mechanism, confirmed present per-project, e.g. `"go test -v ./..."` in the `gate-sandbox` test fixture) is a plausible existing source to reuse — but it is a different subsystem (post-commit regression baseline, not an interactive Flow Mode input) and reusing it needs a deliberate design decision, not an assumption.
- `Q-2` Should the audit draft render inline in the chat/flow timeline, or in a dedicated panel? Task-171's own doc doesn't specify a UI location.

### Source Refs

- `apps/local-runner/internal/runner/behavior_registry_builtin.go:153-163` (`behaviorCommandValidate`), `:209-219` (`behaviorArtifactAuditDraft`).
- `apps/local-runner/internal/runner/flow_validation_retry.go` (`RunValidationCommand`, `NewFlowValidationRetryState`, `AdvanceRetryState`, `ComposeRetryPrompt` — all real, all unit-tested, all unreferenced outside their own test file).
- `apps/local-runner/internal/runner/flow_audit_draft.go:68-127` (`BuildAuditDraft` — real, unit-tested, called only by `flow_audit_draft_test.go` and a hand-driven direct call in `interactive_service_e2e_test.go` that bypasses the live dispatch path).
- `apps/local-runner/internal/agentpack/flow-pack/flows/rag-harness.yaml` (`validate`/`audit` node declarations).
- `D:\working\gate-sandbox\.flowpilot\guard\test_baseline.json` (`test_command` field — the separate CP-35 gate/regression-guard config referenced in Q-1).

## 1. Issue Summary

The built-in RAG Harness Flow Mode pipeline's last two nodes (`validate`, `audit`) are wired to behavior-registry handlers that do not call the real Task-170/Task-171 implementations governing them. The implementations themselves are correct and well-tested in isolation; the live dispatch path simply never reaches them.

## 2. Parent Links

- impacted coding plan: `CP-41` (Task-170/171 completion claims), `CP-42` (behavior registry dispatch, Task-176)
- impacted tech design: none identified
- impacted system spec: none identified

## 3. Environment and Reproduction

- environment: built-in RAG Harness flow (`apps/local-runner/internal/agentpack/flow-pack/flows/rag-harness.yaml`), any project, Flow Mode launch.
- reproduction steps:
  1. Launch the RAG Harness workflow in Flow Mode against any project.
  2. Let the flow progress through `context` → `implement` to the `validate` node.
  3. Observe: no shell command is ever executed; `behaviorCommandValidate` only inspects `RawArgs["exitCode"]`, which nothing populates in this path.
  4. If/when the flow reaches `audit`, observe: the produced "draft" is just an echo of whatever summary text was passed in, not a real audit draft with a change-ledger block or commit-message suggestion, and there is nowhere in the desktop UI to see it.
- frequency: deterministic — this is a permanent code-path gap, not a race or environment-dependent flake.

## 4. Expected vs Actual

- expected: `validate` runs a real, configured test/build command and drives the bounded (max 3) retry loop with failure feedback folded into the coder's next prompt; `audit` produces a real, inspectable draft with a change-ledger block and commit-message suggestion once validation succeeds.
- actual: `validate` performs no command execution at all; `audit` echoes input text with no real draft content and no UI to inspect it.

## 5. Impact

- users affected: anyone attempting to run the built-in RAG Harness Flow Mode end-to-end.
- workflows affected: the entire CP-41 Testing/Audit half of Flow Mode; a manual E2E test session for CP-41 cannot exercise these two nodes meaningfully until this is fixed.
- severity: high for CP-41 Flow Mode completeness — the first two nodes (`context`, `implement`) work correctly, but the pipeline is only half-functional end-to-end.

## 6. Root Cause

- hypothesis: Task-170/171 were implemented and verified at the unit level (their own named tests all pass), and CP-42's behavior-registry refactor (Task-176) added the `command.validate`/`artifact.audit_draft` behavior IDs as part of the generic dispatch scaffolding — but the two were never connected. Task-176's own doc candidly notes the registry is "additive only" and several legacy paths aren't migrated onto it yet; this appears to be the mirror problem — the registry's own handlers for these two IDs were stubbed rather than wired to the pre-existing Task-170/171 code, and no task or CP doc flagged the disconnect because each side's own tests pass in isolation.
- confirmed cause: exhaustive grep confirms `RunValidationCommand` and `BuildAuditDraft` have zero callers outside their own test files (and one hand-driven direct-call test for `BuildAuditDraft` that bypasses the dispatch path entirely).
- evidence: see Source Refs.

## 7. Fix Strategy

Ordered by dependency — `F-0` is the prerequisite the original scoping missed:

- `F-0` **(NEW, prerequisite) Mid-flow inline-node execution.** Give the flow executor a path to run an inline behavior node reached via a forward edge from a completed node: when `tryAdvanceFlowFromNode`'s forward-`done` target is an inline behavior (not `agent.delegate`), dispatch it through the behavior registry in-process (as `startInlineEntryChain` already does for the entry node), apply its `BehaviorOutput` (status → follow forward/back edge; e.g. `command.validate` "continue" → back-edge to `implement`), and continue advancing. Without this, `F-1`/`F-2` are unreachable no matter how they're wired. This is real flow-engine work, not a wiring tweak.
- `F-1` Wire `behaviorCommandValidate` to call `RunValidationCommand` with a command sourced per `Q-1`'s resolution, threading the result through `FlowValidationRetryState`/`AdvanceRetryState` instead of expecting a pre-populated `exitCode`.
- `F-2` Wire `behaviorArtifactAuditDraft` to call `BuildAuditDraft` with the real accumulated flow state (feature key, source doc id, validation result, changed files) instead of echoing `RawArgs["summary"]`.
- `F-3` Add a desktop UI surface (chat/flow timeline card or dedicated panel per `Q-2`) so a produced audit draft is genuinely inspectable before any write/commit, satisfying Task-171's own acceptance criterion.

**Revised size estimate:** with `F-0` added, this is a medium-large change to the flow engine + two handler rewrites + a command-source decision + a desktop UI surface — not the small "wire two functions" fix the first draft of this doc implied.

## 8. Validation

- `V-1` Not yet run — this bug is filed diagnosed-but-unfixed. Once fixed: extend `TestStartResolvedFlowStartsInlineEntryFlow` (or add a new test) to drive a full `context → implement → validate → audit` run with a real (fake-adapter) validation command and assert the retry loop and the produced draft both reflect real command output.

## 9. Regression Guard

- tests: to be added alongside the fix (see V-1).
- alerts: none.
- audit checks: none yet — this note itself is the first record of the gap.

## 10. Follow-Up Document Updates

- upstream docs: `Task-170`/`Task-171`'s Completion Notes have been annotated with a pointer to this bug so their "done" status isn't read as "wired into production" — see their §8 updates.
