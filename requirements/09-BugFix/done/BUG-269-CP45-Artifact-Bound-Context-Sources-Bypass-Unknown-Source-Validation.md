# BUG-269: CP-45 Artifact-Bound Context Sources Bypass Unknown-Source-Id Validation

## Metadata

- Document ID: `BUG-269`
- Title: `CP-45 Artifact-Bound Context Sources Bypass Unknown-Source-Id Validation`
- Phase: `bugfix`
- Status: `done` — fixed and verified 2026-07-09, found while manually re-running CP-44 E2E case 11.3 ("Unknown Source ID Fails Fast") against the live desktop app.
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-09`
- Last Updated: `2026-07-09`
- Parent Documents: [CP-44: Pluggable Context Source Registry](../../07-Coding-Plan/done/CP-44-Pluggable-Context-Source-Registry.md), [CP-45: Generic Artifact Types And Instances](../../07-Coding-Plan/done/CP-45-Generic-Artifact-Types-And-Instances.md), [SD-23: Generic Artifact Framework](../../06-System-Tech-Design/SD-23-Generic-Artifact-Framework.md)
- Child Documents: `none`
- Related Documents: [Task-194: Per-Flow Context Source Binding](../../08-Task/done/Task-194-Per-Flow-Context-Source-Binding.md) (owns the "fails flow-load fast" guarantee this bug violated), [Task-201: Context Artifact Migration From Context Sources](../../08-Task/done/Task-201-Context-Artifact-Migration-From-Context-Sources.md) (added the CP-45 tier that bypassed it), [Task-203: Artifact Framework Validation And Fallback](../../08-Task/done/Task-203-Artifact-Framework-Validation-And-Fallback.md) (owns `ValidateFlowArtifactBindings`, the function extended here), [BUG-268: Flow Coding Prompt Duplicates Feature History](../../09-BugFix/done/BUG-268-Flow-Coding-Prompt-Duplicates-Feature-History.md) (found in the same live E2E session)
- Replaces: `none`
- Tags: `agent-flow-engine, context-source, artifact-framework, validation, regression`

## AI Quick View

### Summary

- CP-44's `ValidateFlowContextSources` fails flow-load fast when a flow's `contexts.<name>.sources` (flow-level) or a node's `ContextSources` (step-level) names a source id the `ContextSourceRegistry` doesn't recognize — an explicit Task-194 T-2 guarantee.
- CP-45 added a third, higher-precedence tier (a bound `context_artifact.v1` instance's `config_json.sources`, SD-23 D-6) but never wired an equivalent check for it. `ValidateFlowArtifactBindings` (the CP-45 sibling validator) only checked for a dangling/missing artifact instance, never the source-id *content* of a resolved one.
- Net effect: an unknown source id inside a bound `context_artifact.v1` instance's config never blocked flow-load. `resolveArtifactBoundContextSources` passed it straight to `ContextSourceRegistry.Collect`, which degrades an unrecognized id per-source into a buried `### Warnings` line in the rendered package instead of failing the run — the exact "silent no-op" CP-44 P-4/Task-194 T-2 was written to prevent, just reachable through the newer tier that takes precedence over the two already-guarded ones.

### Current Ask

- Extend `ValidateFlowArtifactBindings` to also resolve every `context_artifact.v1` binding's `config_json.sources` entries against `DefaultContextSourceRegistry()`, failing flow-load with a clear error when any entry is unregistered — matching `ValidateFlowContextSources`' existing contract for the two older tiers.

### Key Decisions

- `V-1` Extend the existing `ValidateFlowArtifactBindings` function rather than add a new one — it is already called from both `flow_definition_resolver.go` call sites (`ResolveFlowRef`, `ResolveBuiltin`), so no new wiring is needed, and it already owns the CP-45 binding-validation contract (SD-23 F-1) this is a sibling check to.
- `V-2` Scope the new check to `ArtifactTypeID == ArtifactTypeContext` only — a `file_artifact.v1` binding's `config_json.paths` must never be misread as a source list.
- `V-3` Do not touch the existing required/missing-instance check (SD-23 F-1's soft degrade for a dangling/optional binding stays exactly as it was) — this is an additional check on a *resolved* binding's config content, not a replacement.

### Constraints

- Must not weaken SD-23 F-1: an optional binding whose instance no longer resolves at all (`ArtifactTypeID == ""`) must still soft-degrade, not fail.
- Must not change `ContextSourceRegistry.Collect`'s own per-source runtime degrade behavior — the fix is a load-time guard added *before* that pipeline runs, not a rewrite of it.

### Open Questions

- None.

### Source Refs

- `apps/local-runner/internal/runner/context_sources_builtin.go:60-105` (`ValidateFlowArtifactBindings`, the fix; `ValidateFlowContextSources`, the sibling contract mirrored).
- `apps/local-runner/internal/runner/artifact_type_registry.go:19-56` (`resolveArtifactBoundContextSources`, the tier that bypassed validation).
- `apps/local-runner/internal/runner/flow_definition_resolver.go:118-124,145-151` (both call sites already invoking `ValidateFlowArtifactBindings` at flow-resolve time).
- Live run evidence: `db51ec26-1a0f-4b92-8ceb-b03dc8e9b363`, parent `run-4864` → child `run-4869`, `run-4869/prompt-turn-4874.txt` (the buried warning, pre-fix).

## 1. Issue Summary

A `context_artifact.v1` artifact instance bound to a flow node can declare an unrecognized context-source id in its `config_json.sources` array without the flow ever failing to load — the id is silently dropped at runtime with only a buried warning line in the rendered `Flow Context Package`, not the fail-fast, clearly-named error CP-44's older two tiers already guarantee for the identical mistake.

## 2. Parent Links

- impacted coding plan: `CP-44` (its own E2E test matrix §11.3, "Unknown Source ID Fails Fast", is where this was found), `CP-45` (the tier that introduced the gap)
- impacted tech design: `SD-23` (D-6's artifact-binding precedence tier; F-1's soft-degrade contract, which this fix must not weaken)
- impacted system spec: none identified

## 3. Environment and Reproduction

- environment: any Flow Mode flow with a node carrying an OUTPUT `context_artifact.v1` artifact binding (the CP-45 UI path — Settings → Workflows → a step's "Artifact Outputs" section), any project.
- reproduction steps:
  1. Create (or reuse) a `context_artifact.v1` artifact instance bound as output to a flow's `context.produce` node.
  2. Directly edit that instance's `config_json.sources` (e.g. via Supabase Studio SQL) to include an id the registry doesn't know, e.g. `{"sources": ["feature.history", "totally.unknown.source"]}`.
  3. Launch the flow via "Run Flow".
  4. Observe: the flow resolves and runs to completion; the spawned Coding agent's prompt carries `### Warnings\n- totally.unknown.source: context source registry: no source registered for "totally.unknown.source"` instead of the run being blocked at load time.
- frequency: deterministic — any unknown id in a bound `context_artifact.v1` instance's `sources` array, not an edge case or a race.

## 4. Expected vs Actual

- expected: flow-load fails immediately with a clear error naming the flow id, the node, the artifact instance, and the unrecognized source id — the same contract `ValidateFlowContextSources` already provides for `contexts.<name>.sources` and step-level `ContextSources` (CP-44 P-4/Task-194 T-2).
- actual: the flow loaded and ran normally; the bad id was silently skipped per-source inside `ContextSourceRegistry.Collect`, surfacing only as a low-visibility warning line inside the rendered context package sent to the Coding agent.

## 5. Impact

- users affected: anyone configuring a `context_artifact.v1` artifact instance's source list (the CP-45 UI path, now the recommended way to restrict a flow's context sources per CP-44 §11.2's own re-verification).
- workflows affected: a misconfigured/typo'd source id in an artifact instance goes unnoticed — the flow still runs, just with a silently smaller-than-intended context, and the only trace is a warning line buried inside the LLM prompt, not a message a human configuring the flow would ever see.
- severity: medium — no crash, and CP-44's older two tiers still fail correctly, but the flow's own promoted, highest-precedence tier (CP-45) diverges from the "fails fast, no silent no-op" contract adjacent tiers already honor.

## 6. Root Cause

- hypothesis: `ValidateFlowArtifactBindings` was written for Task-203's own DOD — validating that a REQUIRED artifact binding's instance still resolves (SD-23 F-1) — and CP-45's own precedence-chain doc comment in `resolveEnabledContextSourceIDs` never called out that `resolveArtifactBoundContextSources`' returned ids skip the registry-membership check `ValidateFlowContextSources` performs for the other two tiers. The two validators were written for different DODs (Task-194 T-2 vs Task-203) and nobody connected them when CP-45's tier was layered on top.
- confirmed cause: reproduced live via the desktop app (see §3) and with a new unit test (`TestValidateFlowArtifactBindingsRejectsUnknownSourceInContextArtifactConfig`) that failed before the fix (asserted no error was returned, matching a manual pre-fix run) and passes after.
- evidence: see Source Refs; the live prompt file `run-4869/prompt-turn-4874.txt` is the exact pre-fix warning-instead-of-failure artifact.

## 7. Fix Strategy

- `F-1` Extend `ValidateFlowArtifactBindings` (`context_sources_builtin.go`): for every node's `ArtifactBindings`, when `ArtifactTypeID == ArtifactTypeContext`, read `ConfigJSON["sources"]` and resolve each entry against `DefaultContextSourceRegistry()`, returning `fmt.Errorf("flow %q node %q: context_artifact binding %q declares unknown source %q: %w", ...)` on the first unregistered id — same message shape as `ValidateFlowContextSources`, naming flow/node/instance/bad-id. Left the existing required/missing-instance check (SD-23 F-1) untouched above it.
- `F-2` Added 3 new tests to `context_source_step_precedence_test.go`: rejects an unknown id inside a `context_artifact.v1` binding's config (naming both the bad id and the instance id), accepts a binding whose declared sources are all known, and confirms a `file_artifact.v1` binding's unrelated `paths` config is never misread as a source list.

## 8. Validation

- `V-1` **New unit tests** — done: `TestValidateFlowArtifactBindingsRejectsUnknownSourceInContextArtifactConfig`, `TestValidateFlowArtifactBindingsAcceptsKnownSourcesInContextArtifactConfig`, `TestValidateFlowArtifactBindingsIgnoresNonContextArtifactConfig` all pass.
- `V-2` **Existing contract tests unaffected** — done: `TestValidateFlowArtifactBindingsFailsOnMissingRequiredInstance`, `TestValidateFlowArtifactBindingsDegradesOnMissingOptionalInstance`, `TestValidateFlowArtifactBindingsAcceptsResolvedBindings` (the SD-23 F-1 checks this fix must not weaken) all still pass unchanged.
- `V-3` **Full suite** — `go build ./...` and `go vet ./...` clean; `go test ./internal/...` run alongside this change with no new failures beyond the pre-existing, already-confirmed-unrelated environment-dependent flakes (Codex/Claude CLI resume, skills-merge, live provider tests — reproduced identically with unrelated changes stashed out earlier in this session).
- `V-4` **Live re-verification** — pending the maintainer's own re-run of CP-44 E2E case 11.3 against the fixed binary (same reproduction steps as §3); this closing note is being written before that re-run to keep the fix and its test coverage in the same commit per the repo's change-audit contract — flagged here rather than claimed.

## 9. Regression Guard

- tests: `context_source_step_precedence_test.go` (+3 new tests, as above).
- alerts: none.
- audit checks: this note is the closing record; CP-44's own §11.3 VERIFIED note should be updated once the maintainer's live re-run (`V-4`) confirms the fix end to end.

## 10. Follow-Up Document Updates

- upstream docs: [CP-44](../../07-Coding-Plan/done/CP-44-Pluggable-Context-Source-Registry.md) §11.3 should be annotated VERIFIED once `V-4`'s live re-run completes.
- notes left unchanged on purpose: Task-194's and Task-203's own DoD/completion notes are not amended — both tasks' own unit-level claims were and remain accurate; the gap was in the seam between them, not in either task's own scope.
