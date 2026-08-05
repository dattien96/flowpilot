# CA-435 — CP-43 P-5 Pack Budget + Admin Scope Diff (Task-188)

## Scope

Ship remaining **Task-188 / CP-43 P-5** pieces: context render budget (T-2) and admin panel enhancements (T-5 partial).

## Changes

- `runner/flow_context_pack_budget.go` (new): `applyFlowContextPackBudget` — drops lower-priority sources (`feature.history` first) when render estimate exceeds 24KB; **never** drops `canonical.head`; logs `[context-pack]` + warning on drop (Task-188 T-2).
- `runner/flow_context_package.go`: `RenderFlowContextPackage` applies budget before render.
- `runner/canonical_head_handlers.go`: step contract response adds live `ScopeDiff` (`touched_paths`, `in_scope_paths`, `out_of_scope_paths`); new `GET .../features` list endpoint.
- `runner/interactive_handlers.go`: register features list route.
- `apps/desktop-flowpilot/.../ProjectsSettings.tsx`: feature-key datalist; scope diff display on step contract.
- Tests: `flow_context_pack_budget_test.go`, `canonical_head_handlers_scope_test.go` (additive).

## Out of scope (still open for P-5 close)

- B19 live verify Head-first on Claude **and** Codex prompts (manual).
- Full CP-10 token packer integration (v1 uses char budget on render path only).

## Verification

- `go test ./internal/runner/ -run 'TestApplyFlowContextPackBudget|TestHandleGetStepContractIncludesScopeDiff|TestHandleListProjectFeatures|TestBuildFlowContextPackageOutputUnchanged' -count=1`

# ---8<--- flowpilot:change-ledger
feature_key: change-contract
source_doc_id: Task-188
change_type: feature
summary: Task-188 T-2 render budget drops history under pressure while keeping Canonical Head; T-5 adds ScopeDiff on contract API and feature list for desktop panel
# --->8---
