# CA-147: Agent Flow Pack and Generic Node Behavior Refactor

## Scope

Move the built-in agent and flow definitions into the embedded `internal/agentpack/flow-pack` data bundle and reduce runner hardcode around node behavior matching.

## Completed

- Added `internal/agentpack` with an embedded pack loader, manifest validation, YAML parsing, and canonical behavior alias normalization.
- Added the embedded FlowPilot pack contents for built-in agents, flows, context artifacts, prompts, and tool faces.
- Wired `AgentCatalog` to load built-in agents from the pack first, with legacy hardcoded fallback only if pack loading fails.
- Wired `ReviewLoopFlowConfig` to prefer the pack-defined built-in review-loop flow and keep the legacy template only as a fallback.
- Replaced hardcoded `plan/coding` step-type switches with behavior-ID normalization so step matching now follows the pack alias registry.
- Added tests covering pack loading, flow parsing, catalog loading, and review-loop compatibility.

## Verification

- `go test ./internal/agentpack ./internal/runner -run 'TestLoadBuiltinPack|TestLoadBuiltinReviewLoopFlow|TestLoadBuiltinRAGHarnessFlow|TestValidateManifestRejectsMissingReference|TestLoadAgentSpecParsesMarkdownFrontmatter|TestAgentCatalogReturnsBuiltinsWhenEmpty|TestReviewLoopFlowConfigValid'`
- `go test ./internal/runner -run 'TestE2EReviewLoopApprovedPath|TestE2EReviewLoopChangesRequestedFeedbackReachesCoderPrompt|TestE2EReviewLoopCapHitBlocked|TestE2EReviewLoopEscalatePath|TestE2EReviewLoopExtendCapResumesFromBlocked'`

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CP-42
change_type: refactor
summary: move built-in agents and flows into the embedded flow pack and normalize generic node behavior matching
# --->8---
