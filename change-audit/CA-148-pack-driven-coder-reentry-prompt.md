# CA-148: Pack-Driven Coder Reentry Prompt

## Scope

Move the coder re-entry prompt used after `submit_review_outcome` continues the loop into the embedded agent pack so the runner no longer owns that prompt text as a Go literal.

## Completed

- Added `prompts/coder-reentry.md` to the embedded flow pack.
- Updated the embedded pack manifest to include the new prompt template.
- Changed `buildCoderReentryPrompt` to load the pack prompt as the base re-entry text and then append typed issue details from `FlowControlInput`.
- Kept the existing fallback text path so the runner still behaves safely if the embedded pack fails to load.
- Added coverage for the new prompt template lookup.

## Verification

- `go test ./internal/agentpack ./internal/runner -run 'TestLoadBuiltinPack|TestLoadBuiltinReviewLoopFlow|TestLoadBuiltinRAGHarnessFlow|TestLoadBuiltinPrompt|TestLoadBuiltinCoderReentryPrompt|TestValidateManifestRejectsMissingReference|TestLoadAgentSpecParsesMarkdownFrontmatter|TestAgentCatalogReturnsBuiltinsWhenEmpty|TestReviewLoopFlowConfigValid'`
- `go test ./internal/runner -run 'TestE2EReviewLoopApprovedPath|TestE2EReviewLoopChangesRequestedFeedbackReachesCoderPrompt|TestE2EReviewLoopCapHitBlocked|TestE2EReviewLoopEscalatePath|TestE2EReviewLoopExtendCapResumesFromBlocked'`

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: CP-42
change_type: refactor
summary: move coder reentry prompt into the embedded flow pack and keep typed issue formatting in Go
# --->8---
