# CA-174: Harden AgentPack Load-Time Validation (BUG-NOTE-CP42 #5, #6, #25, #33)

## Scope

Verified and fixed four P2/P3 pack-validation gaps from `requirements/09-BugFix/todo/BUG-NOTE-CP42.md`, all in `internal/agentpack/pack.go`. Each was a case of validation existing in name but not actually checking the thing it claimed to (or not being wired into the real load path at all), so a malformed pack could load successfully instead of failing fast.

## The bugs and fixes

- **#5 — duplicate flow IDs not rejected.** `ValidateManifestFS`'s `seenFlowIDs` was keyed by the manifest entry's file *path*, which is inherently unique per list entry — it never looked at the flow file's own content-level `id:` field. Two different files could both declare `id: review-loop` and pass validation. Fixed by adding a cross-file `def.ID` uniqueness check in `LoadPackFS`, right after each flow file is actually parsed (where the real content ID is available).
- **#6 — context render template mis-parsed, and not validated against the manifest.** The YAML shape is a nested `render.promptTemplate` field; `contextArtifactFromMap` read `render` as a scalar string, coercing the whole nested map via `fmt.Sprint` into a Go stringified-map string. Fixed the parse to read the nested field. Also added the Task-173 T-8 half: `LoadPackFS` now cross-checks every context's `RenderedTemplate` and every flow node's `PromptTemplate` against `manifest.Prompts`, rejecting a reference to a prompt path the manifest doesn't declare.
- **#25 — duplicate `(kind=back, when=continue)` edges not rejected.** `agent_orchestrator.go`'s `validateFlowEdges` implements exactly this check but is only ever called from its own tests — never from the real pack-loading path (and `agentpack` can't import `runner` to reuse it without a cycle). Added the same check directly inside `agentpack.validateFlowDefinition`.
- **#33 — `dependsOn` references not validated.** A node's `dependsOn` entries are supposed to reference another node's ID within the same flow (per the migration's own comment), but `validateFlowDefinition` only validated node IDs and edge endpoints, never each node's `dependsOn` list. A typo'd `dependsOn` entry would silently pass load/mirror instead of failing fast. Added the check.

## Severity/impact note

#5 and #33 would cause a pack to *load with unintended ambiguity/drift* rather than crash — real, but non-fatal until something actually collides. #6's `RenderedTemplate` field, like BUG#10's `Inputs`/`Outputs`, is not read anywhere at runtime yet (confirmed via search) — this fix corrects a latent data-fidelity bug, not an active one. #25 is the most consequential: a real duplicate back-edge would silently misroute a `continue` signal to the wrong node with no error anywhere.

All four are now caught at pack-load time (`LoadPackFS`/`LoadBuiltinPack`), which runs at process startup and in the mirror-sync path — a malformed pack fails immediately and loudly instead of shipping ambiguous or subtly wrong behavior.

## Verification

- New tests in `pack_test.go`: `TestLoadPackFSRejectsDuplicateFlowContentID`, `TestContextArtifactParsesNestedRenderPromptTemplate`, `TestLoadPackFSRejectsPromptRefNotDeclaredInManifest`, `TestValidateFlowDefinitionRejectsDuplicateContinueBackEdge`, `TestValidateFlowDefinitionRejectsMissingDependsOnReference`.
- Confirmed the real embedded `flow-pack` (both built-in flows, its one context, its four declared prompts) still passes all four stricter checks: `go test ./internal/agentpack/...` (12 passed, up from 7) and the full `internal/runner` suite (1005 passed, 15 pre-existing/environmental failures unchanged) both stay green, proving `LoadBuiltinPack()` — exercised extensively by `internal/runner`'s own tests — doesn't trip any of the new checks against real shipped content.
- `go build ./...` clean.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-NOTE-CP42
change_type: bugfix
summary: harden agentpack's load-time validation to actually catch duplicate flow content IDs, mis-parsed/undeclared prompt template refs, duplicate continue back-edges, and dangling dependsOn references, instead of a name that implied checks that weren't wired in
# --->8---
