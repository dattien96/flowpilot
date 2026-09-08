# CA-768 — CP-60 remaining code: persist, vibe face, tdd artifact, lock browse

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: CP-60
change_type: feature
summary: Persist vibe lock/plan, advertise vibe-requirement-outcome, bind tdd_signatures, non-tech r-requirement copy, Desktop Browse
# --->8---

## Change

Closes remaining code gaps after CA-767 so only live/manual demo is left.

- Persist `vibeAwaitingLock`, `vibeTaskPlan`, locked SS/CP, lock node/path on `sessions.ndjson` via `sessionStateOf` + reconstruct.
- Slicer records the plan as read-only `runArtifacts` (timeline/API).
- Go handler + MCP/Codex advertisement for `vibe-requirement-outcome` (`aligned/drift_fixable/requirement_change`). Default off; old hub tests unchanged.
- `vibe-sprint` tdd node required `tdd_signatures` file_artifact; guard accepts that path or `*_test.go`.
- `FormatRequirementCard` names drifted `AC-*` and the SS edit.
- Desktop Vibe panel Browse… file input for SS/CP markdown.

## Tests

- `flowgate/requirement_card_test.go`
- `agentpack/task323_tdd_binding_test.go`
- `runner/task321_p6_persist_artifacts_test.go`
- `runner/vibe_requirement_outcome_test.go`
- `desktop-flowpilot/src/state/detectVibeEntry.test.ts` (path strip)

## Provider impact

**Case 1 mapper + Case 2 advertisement.** Face mapping is provider-agnostic. Tool is offered on Claude MCP, Grok/OpenCode MCP, and Codex dynamic tools only when `OfferVibeRequirementTool` (vibe + synthesis hub).

## Residual

- Live lock-card click-through and N× sprint demo (operator).
- Kill-Review on CP-60 docs.

## Will not undo

CA-763..CA-767. `claudeMCPToolDefs(bool)` one-arg signature. TestDefaultRules count.
