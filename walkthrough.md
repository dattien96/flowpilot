# Task-224 / BUG-277 Implementation Review

## Scope

Reviewed the Task-224 / BUG-277 / CA-289 implementation in:

- `apps/local-runner/internal/runner/feature_history.go`
- `apps/local-runner/internal/runner/flow_executor.go`
- `apps/local-runner/internal/runner/artifact_type_registry.go`
- `apps/local-runner/internal/runner/artifact_type_registry_test.go`
- `apps/local-runner/internal/runner/interactive_service_test.go`

Reviewed against:

- `requirements/08-Task/done/Task-224-Flow-Prompt-Scoping-And-Coder-Output-Why-Template.md`
- `requirements/09-BugFix/done/BUG-277-Flow-Ledger-History-Over-Injected-On-Non-Context-Consumers.md`
- `change-audit/CA-289-task-224-flow-prompt-scoping-and-coder-why-template.md`

Limitations:

- GitNexus MCP tools required by `AGENTS.md` were not exposed in this session, so I could not run `gitnexus_impact` or `gitnexus_detect_changes`.
- `code-review-skill` asks for reviewer-agent delegation, but current tool policy only allows spawning sub-agents when the user explicitly asks for delegation. This review is direct/self-contained.

## Verdict

**PASS**

The implementation matches the Task-224 / BUG-277 acceptance criteria. I found no blocking correctness bug in the requested changed files. The remaining items are test hardening and one scope-risk note for generic non-context artifact inputs.

## Acceptance Match

| AC | Result | Notes |
|---|---|---|
| AC-1 Coder still gets Flow Context Package | PASS | `startInlineEntryChain` still renders the context package before applying node artifact prompts, then `injectFeatureHistoryPrompt` skips double injection via `isFlowContextHandoff`; see `flow_executor.go:392-395` and `feature_history.go:21-23`. |
| AC-2 Reviewer does not get full Prior work ledger inject | PASS | `injectFeatureHistoryPrompt` now skips flow-engine prompts and review handoffs; review handoff detection uses `strings.Contains`, so it still works after child-prompt wrapping; see `feature_history.go:21-23` and `feature_history.go:45-47`. |
| AC-3 Reviewer with file INPUT gets no full coder final message | PASS | `tryAdvanceFlowFromNode` builds per-target prompts, and `buildFlowReviewHandoffPrompt` omits `resultMessage` when `nodeHasFileArtifactInput` is true; see `flow_executor.go:783-788` and `flow_executor.go:856-864`. |
| AC-4 OUTPUT template requires What / Why / Baseline | PASS | Required file output prompt now includes the three markdown headings and explicit closed-decision guidance in Why; see `artifact_type_registry.go:327-336`. |
| AC-5 Hub pure synthesis does not re-dump Prior work | PASS | `isFlowEnginePrompt` classifies synthesis/join/system flow prompts and `injectFeatureHistoryPrompt` returns them unchanged; see `feature_history.go:146-164`. |
| AC-6 Task-223 write gate + BUG-276 path-only still green | PASS | OUTPUT write contract still lists required paths, INPUT file artifacts remain path-only, and focused tests pass; see `artifact_type_registry.go:282-292` and `artifact_type_registry.go:309-337`. |
| Do not regress context package handoff skip | PASS | Existing `isFlowContextHandoff` sentinel remains in the injection skip path; focused wrapped-context test also passes. |

## Bugs / Edge Cases / Null Paths

No blocking bugs found.

Edge cases reviewed:

- `nodeHasFileArtifactInput` handles nil or missing `ConfigJSON` safely because `fileArtifactPathsFromConfig` returns nil for nil maps, missing `paths`, non-string values, blank strings, and duplicates; see `artifact_type_registry.go:236-258` and `artifact_type_registry.go:297-306`.
- Empty or unconfigured file INPUT paths do not trigger coder-body omission. That is correct for Task-224 because omission is tied to actual bound file path(s), not merely an empty binding.
- Wrapped review prompts are covered by `isFlowReviewHandoffPrompt` using `Contains`, so a child prompt with an agent-system prefix still skips ledger injection. This is important because `isFlowEnginePrompt` alone would only catch the review handoff when `[flow-engine]` is at the beginning.
- Review handoff body is still included, truncated, when the target has no file artifact INPUT. That matches the implementation note and does not violate AC-3.

Scope-risk note:

- `resolveInputArtifactPrompt` now bypasses `DefaultArtifactTypeRegistry().Resolve` for every non-context input and only emits `config_json.paths` when present; see `artifact_type_registry.go:197-231`. For the current product surface this is fine because `file_artifact.v1` is the only registered non-context artifact, and BUG-276 requires path-only. If SD-23 later adds another resolver-backed non-context artifact whose prompt input is not path-based, this helper will need to be generalized again. Not a Task-224 blocker.

## Test Gaps

The added tests cover the core helper behavior:

- Path-only INPUT and no body paste: `artifact_type_registry_test.go:197-214`, `artifact_type_registry_test.go:272-305`.
- Required OUTPUT What / Why / Baseline template: `artifact_type_registry_test.go:231-247`.
- Review handoff omits coder final body when file INPUT is bound: `artifact_type_registry_test.go:249-270`.
- Injection skips review / engine / package prompts: `interactive_service_test.go:1064-1082`.

Gaps worth closing later:

- `TestInjectFeatureHistorySkipsFlowReviewAndEnginePrompts` uses an empty temp workspace, so it proves the skip path returns unchanged, but not that a real ledger/catalog would otherwise have injected. Add a fixture with `FEATURE-KEYS` and a ledger entry to make this a stronger regression test.
- There is no full `tryAdvanceFlowFromNode` integration test asserting the spawned reviewer provider prompt simultaneously has no `## Prior work`, no coder final body, and has the bound file path section. The helper tests strongly cover this, but an end-to-end prompt capture would better protect the composition order.
- There is no explicit test for a wrapped review handoff prompt with agent-system prefix. The implementation should pass because `isFlowReviewHandoffPrompt` uses `Contains`; add a direct test to prevent future refactors from switching back to `HasPrefix`.
- AC-6 references Task-223 write gate and BUG-276 path-only. Existing focused tests passed, but I did not run the entire runner/flowgate suite in this review.

## Scope Creep

No feature rewrite or unrelated architecture change found in the requested files.

The only mild scope expansion is the generic non-context artifact input behavior noted above: unknown/non-file artifact inputs are no longer resolved through the registry and only get path mentions if they expose `paths`. This is aligned with the current file-artifact policy but should be documented as current limitation rather than a general artifact framework rule.

## Verification

Ran:

```bash
go test ./internal/runner -run 'TestRequiredFile|TestBuildFlowReview|TestInjectFeatureHistorySkips|TestComposeFlow|TestFlowCodingPromptSpawnWrappedDoesNotDuplicateHistory' -count=1
```

Result: PASS (`6 passed in 1 packages`).

## Concrete Fix List For Grok

Verdict is **PASS**, so there is no required fix before acceptance.

Recommended follow-ups:

1. Strengthen `TestInjectFeatureHistorySkipsFlowReviewAndEnginePrompts` with a real catalog + ledger fixture so the test proves injection would happen without the new skip.
2. Add an end-to-end prompt-capture test for coder -> reviewer auto-advance with file INPUT: assert no `## Prior work`, no unique coder final body, and bound path section present.
3. Add a wrapped review-handoff detector test where the prompt starts with an agent-system prefix before `[flow-engine] Review this result from node`.
4. Add a short comment or future task noting that non-context artifact INPUT prompt rendering is currently path-oriented and will need extension for future resolver-backed artifact types.
