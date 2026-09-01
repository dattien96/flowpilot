# CA-712 — Bug / Task / CP harness family with plan review loop (CP-58 Tasks 304-307)

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CP-58
change_type: feature
summary: three tiered harnesses (task-harness 12-node with plan review loop, cp-harness slice-only + smoke variant), source-aware dual continue/back edges, and typed plan file_artifact bindings (plan_md/cp_md/task_md)
# --->8---

## What changed

Engine (Task-304, commit 32bd3581):

- `agentpack/pack.go` `ValidateFlowDefinition`: back-edge duplicate check keys on `(from, when)` instead of `when` alone, so a flow may declare the plan loop's `plan_synthesis -> plan_writer` and the code loop's `validate -> implement` together; a same-From duplicate still fails fast.
- `runner/flow_executor.go` `resolveContinueBackEdgeTarget(edges, from ...string)`: source-aware resolution — (a) exact anchor match, (b) hub-aware nearest forward ancestor anchor (the hub-driven emitter's loop), (c) first-match fallback preserving every pre-existing call site (variadic keeps them compiling unchanged).
- `runner/interactive_service.go`: both continue call sites (`applyFlowControl`, `maybeReinvokeCoderForContinue`) pass `activeHubNodeID` (Task-235 run state) with the single-hub first-match fallback; cohort-join sites persist the hub a joined cohort feeds into when the flow declares >1 `hub.inline` node (`hubNodeIDForCohortJoin`, single-hub flows byte-identical).

Packs (Task-305/306, commits e54801fa, 9dcf5541):

- `flows/task-harness.yaml` (12 nodes): scout -> context (draft) -> plan_writer -> plan_reviewer (cohort plan) -> plan_synthesis, plan continue/back re-entry, done -> freeze -> test_signatures -> implement -> validate -> reviewer -> synthesis -> audit; `acceptance_nodes:[plan_synthesis, validate, synthesis, audit]`.
- `flows/cp-harness.yaml` (7 nodes, slice-only): plan loop over the CP doc, then task_splitter -> audit; coding-chain nodes deliberately absent (an unreferenced delegate node would be an entry spawn, an incoming edge would auto-advance into coding). `flows/cp-harness-smoke.yaml` (13 nodes, `selectableIn:[]`) chains the first sliced Task's coding.
- New prompts: `plan-task.md`, `review-plan.md`, `plan-cp.md`, `review-cp.md`, `task-splitter.md` — encode SS-13 doc contracts + safe-fix-contract gates (feature_key override defense, additive-tests-only, CA-contradiction checks) as writer output contracts and reviewer criteria.

Artifacts (Task-307, commit a5626cd7):

- `agentpack/pack.go` `LoadFlowFS` now parses node `artifactBindings` (previously only the Supabase mirror populated the field — a YAML binding silently vanished) and `ValidateFlowDefinition` fails closed on malformed OUTPUT bindings; `validateArtifactOutputPath` requires file_artifact OUTPUT paths to stay workspace-relative under `requirements/` (no traversal/absolute/Windows drive).
- `runner/artifact_type_registry.go`: templated (`pathTemplate`) OUTPUT write contract + INPUT mention for plan_md/cp_md/task_md — prompt-level only, deliberately excluded from the flowgate exact-path check so a template can never cause an endless reprompt.
- Migration `20260831090000_add_harness_plan_artifact_instances.sql` seeds `is_builtin=true` instances (fixed UUIDs continuing the Task-201 sequence).
- `TestLoadBuiltinPack` inventory count 6 -> 9 (CP-58 §4 sanctioned inventory update, Task-293 precedent — the ONLY pre-existing test assertion touched).

## Documented deviation (Task-305 T-1 / Task-306 CG-1 vs CP-55)

The task docs sketch `plan_writer`/`cp_plan_writer`/`task_splitter` as `agent.code`. That is unsatisfiable together with CP-55 P-1 `ValidateFlowSafetyTopology`, which requires every `agent.code` writer to be dominated by a `contract.freeze` node — while CP-58's core purpose is running those writer nodes BEFORE freeze (the freeze locks the approved plan; cp-harness slice-only has no freeze at all). Shipped resolution: those doc-writer nodes are `agent.delegate` + `lifecycle: reinvoke`, which is runtime-identical (CP-55 P-1 registered `agent.code` as the same handler verbatim; artifact write contracts key on `artifactBindings`, not behavior) while keeping CP-55's code-writer safety contract intact for real code writers (`test_signatures`, `implement` remain `agent.code`, freeze-dominated, acceptance-gated). Proven by `TestTaskHarnessValidateFlowDefinition` (loads through full validation) and by the topology tests asserting no `agent.code` in the plan loop.

## R1 evidence (old suite untouched)

- Branch `cp58-harness-dual-loop` (base cp59-chat-ssot c169d214) vs pristine cp59 baseline, same command `go test ./internal/runner/ -count=1 -skip 'Grok|Codex|Jira|OpenAI|Gemini|Opencode|Live|Claude'`: 17 failures baseline vs 17 on the branch, 16 IDENTICAL (pre-existing environmental: TempDir-cleanup/goroutine-leak flakes, provider detection, Run144900/Run147126 parks, Google Drive MCP statuses, Supabase catalog shaping). The one-name difference each side (`TestTryAdvanceSpawnsAgentCodeWriterWithWriterPrompt` failed on baseline, `TestMultiWorkspaceRunsIndependent` on the branch) is the same flake family — both pass 3x in isolation on the respective branches.
- Full `go test ./internal/agentpack/ ./internal/flowgate/ ./internal/changecontract/ ./internal/skillpack/` green on the branch (the `skillpack/.../assets/examples` build failure is a pre-existing asset-folder quirk, present before any change).
- New tests (all additive): dual-edge validator accept/reject, source-aware resolver table, single-loop `from=hub` unchanged, `TestApplyFlowControlContinueHubRouting` (reset scoping across both loops), task-harness live plan/code loop re-entry + session reuse, cp-harness entry-spawn + plan-loop reuse, artifact path contract table + fail-closed validation + pack binding assertions + composed prompt contracts.

## R2 provider classification

- Provider-agnostic by construction: the harness family is pack data + engine routing. No provider branch was added; behavior nodes reuse `behaviorAgentDelegate` verbatim for every provider, and the review outcome path (`submit_review_outcome`) is the existing CP-53 machine-verdict contract. The dual-loop routing tests run on the fake Codex adapter; routing reads only edge/node data. Residual: live `/flow task-harness` + `/flow cp-harness` runs on Claude/Codex/Grok real binaries are the DOD-4/DOD-5 manual checks, still pending (see Honest gaps).

## Honest gaps

- Live DODs pending: `/flow task-harness` full round (plan review reject -> revise -> approve -> freeze -> code to done), `/flow cp-harness` slice + splitter output inspection, artifact panel rendering of plan_md/cp_md/task_md. Automated coverage proves topology + routing + contracts, not live provider behavior.
- `activeHubNodeID` is not persisted across a runner restart (pre-existing Task-235 limitation); a dual-hub flow resumed mid-hub falls back to first-match hub.inline, which is wrong for one edge until the next cohort join re-persists it.
- Hub attribution on DELEGATE-FAILURE paths (not cohort joins) of a dual-hub flow can keep a stale hub id for the display stamp; escalate/fail-closed paths do not use the continue resolver, so routing is unaffected. Revisit if a live failure shows a mis-stamped hub.
- Rule (b) of the resolver needs forward edges between the loop anchor and the hub (always present in the shipped packs; a hand-edited flow missing them falls back to first-match).
- Supabase mirror of the new harness flows (`EnsureBuiltinFlowMirrorsWithStore`) was not exercised against a live Supabase in this task; `step_artifact_bindings` seeding for the new instances beyond the built-in context artifact path (Task-201 seeder) is follow-up work.

## Prior CA not undone

- CA-628/CA-629 (rag-harness TDD + live continue topology), CA-640/CA-645/CA-647/CA-648/CA-649 (freeze/gate contracts), CA-671-era BUG-286 forward-reachability scoping — all preserved; `rag_harness_live_continue_back_edge_test.go` passes unedited and rag-harness/review-loop/context-coding-review-synthesis YAML are byte-identical to base.
