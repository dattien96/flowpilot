# CA-629: Rag-harness live continue topology lock, signatures-only override, reviewer verdict prompt lock

## What

Follow-up to CA-628's kill-review (I-2 / I-3, and closing out the I-1 false-positive). Adds tests that pin the LIVE `rag-harness` continue topology — exactly one `validate → implement` continue back-edge shared by validate retries and the synthesis hub's `changes_requested` — strengthens the `test-signatures` prompt to override the tester persona's "write and run tests" default, and locks the reviewer prompt's CP-53 machine-verdict contract (record-only via `submit_review_outcome`, never `flow_control`).

## Why

- **I-2 (test gap)**: `TestSynthesisContinueReinvokesImplementWithFindings` exercised a synthetic `synthesis → implement` continue back-edge that the pack validator (duplicate continue back-edge rejection, `pack.go` back-edge check) forbids on the real flow. Nothing locked the live topology: exactly one continue back-edge from `validate` to `implement`, so a hub `changes_requested` continues through the SAME edge.
- **I-3 (prompt conflict)**: the `test_signatures` node runs the `tester` agent, whose persona says "Write and run tests" — directly contradicting the signatures-only step. No gate existed; a model could fill bodies / run the suite at the signatures step. `tester.md` is off-limits (Task-293), so the override must live in the step's own static prompt.
- **I-1 (reviewed false-positive)**: the reviewer prompt naming `submit_review_outcome` is CORRECT under CP-53 — rag-harness declares `synthesis` in `acceptance_nodes`, so `flowRequiresSynthesisMachineVerdict` requires reviewers to record machine verdicts; `SubmitFlowControl`'s cohort-member branch is record-only (no flow advance), and CA-213/217 already gate hub-only finalization. No prompt change; instead a test locks the contract.

## Fix

- **`flow-pack/prompts/test-signatures.md`**: added an explicit "these rules OVERRIDE any default tester instructions" block, a valid empty-body example (`func TestSum_NegativeNumbers(t *testing.T) {}`), and re-stated that no bodies/suite/production code belong to this step.
- **`runner/compose_static_prompt_template_test.go`** (`TestSafeFixPromptsCoverHardRules`): added `"OVERRIDE any default tester instructions"` to the test-signatures want-list. This is the Task-293-created test file, not the legacy suite.
- **`runner/rag_harness_live_continue_back_edge_test.go`** (new, additive):
  - `TestRAGHarnessLiveEdgesHaveExactlyOneContinueBackEdge` — loads the real builtin `rag-harness` YAML, asserts exactly one `(kind=back, when=continue)` edge from `validate` to `implement`, and that `resolveContinueBackEdgeTarget` returns `implement` (unambiguous for the live flow).
  - `TestRAGHarnessSynthesisContinueOnLiveEdgesReinvokesImplement` — hub synthesis `continue` (changes_requested) on a fixture carrying the live continue back-edge source (`validate → implement`) reinvokes the implement child with the findings and reuses its session (1 child, not a fresh spawn). The `implement → validate` forward edge is omitted from this fixture because its presence auto-advances the completed implement into `runValidateNode`, which escalates with no workspace and races the hub continue; the full forward topology is pinned by the topology test above.
  - `TestRAGHarnessReviewerPromptRecordsMachineVerdictOnly` — the rag-harness reviewer static prompt (and its composed form via `composeFlowNodeAgentPrompt`) contains `submit_review_outcome` and never names `flow_control`.

## Tests

New (additive, no legacy-suite edits): 4 tests above, all green.

- `go test ./internal/agentpack ./internal/changecontract -count=1` — green.
- `go test ./internal/runner -run 'RAGHarness|RagHarness|SafeFix|SynthesisContinue|SynthesisApproved|ComposeStatic|AppendStatic|CoderCompletionAuto|StartResolvedFlow|SubmitFlowControlRejects|ForwardReachable|FlowAgentCode|TryAdvance|FlowSpawn|AdvanceAgentCode|FreezeMulti|FreezeBinds' -count=1` — green.
- `go vet ./internal/runner ./internal/agentpack ./internal/changecontract` — clean.
- Full `go test ./internal/runner -count=1` → 8 failures, **0 new vs baseline**: `TestRunCompatCheckIncludesPortabilityCanaries`, `TestProjectRunHistoryFiltersRunsByProject`, `TestProviderRegistryForUsesLiveWhenFlagOn`, `TestSupabaseCatalogStoreShaping`, `TestRun75035_HubResumeStillLoadsOwnCodexSession`, `TestRootFlowEngineDefersCompletedUntilGate` (the CA-628-documented pre-existing environment baseline) plus `TestValidatePassedSpawnsReviewerCohortMember` / `TestValidatePassedStillChainsInlineAuditForLegacyEdges`. The two `TestValidatePassed*` failures were re-verified as pre-existing: they fail identically at clean HEAD `ae2eb3a` with this change fully stashed (tracked files) and the new test file removed — they are NOT caused by this fix and were NOT edited.

Provider parity: **Case 1 agnostic** — the fix touches one static prompt markdown and test-only Go files; `grep ProviderKey` on the changed test files: 0 hits on production-path branches (the tests use `ProviderKeyCodex` purely as a fixture provider key, identical to the other Task-293 tests). No adapter code touched.

Will not undo: CA-628 topology/engine wiring, CP-53 machine-verdict gate (`flowRequiresSynthesisMachineVerdict` / cohort record-only branch), CA-213/217 hub-only finalization, CA-167 prompt hygiene (reviewers never drive routing — `flow_control` stays out of the reviewer prompt).

Residual risk (unchanged from CA-628): empty/stubbed test bodies are still caught by prompt + review gate, not by `command.validate`; hub-ful planner-fail follows the CA-355 hub-reinvoke path; hub-less fixtures (`ragHarnessNodes()`, `flowFixtureEdgesNodes()`, TUI F2) are hand-built and untouched. The two pre-existing `TestValidatePassed*` runner failures remain open at HEAD and are out of scope here.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: Task-293
change_type: bugfix
summary: lock live rag-harness single validate->implement continue back-edge + signatures-only tester override + reviewer machine-verdict prompt tests (CA-628 KR I-2/I-3, I-1 false-positive)
# --->8---