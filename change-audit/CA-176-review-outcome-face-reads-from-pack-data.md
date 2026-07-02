# CA-176: reviewOutcomeFace Reads Its Status Map From Pack Data (BUG-NOTE-CP42 #11)

## Scope

Verified and fixed a P2 issue from `requirements/09-BugFix/todo/BUG-NOTE-CP42.md`: `submit-review-outcome.yaml` declares the review-loop tool face's status map as pack data, but the runtime never actually read it — it built an independent, hardcoded Go literal with the same values instead.

## The bug

`submit-review-outcome.yaml` declares `statusMap: {approved: done, changes_requested: continue, blocked: escalate}`. `agentpack.pack.go`'s `toolFaceFromMap` correctly parses this into `ToolFace.StatusMap`. But `reviewOutcomeFace()` (`agent_orchestrator.go`) built its `FlowControlFace` from a hardcoded Go map literal with the exact same three entries, never consulting the parsed pack data at all. CP-42 P-5 requires declared faces to actually become pack data — this was a case where the parsing existed, the pack data existed, but nothing downstream ever used it, so a future edit to the YAML's `statusMap` would silently have no effect on runtime behavior.

## Fix

Added `agentpack.LoadBuiltinToolFace(id string) (ToolFace, bool, error)`, mirroring the existing `LoadBuiltinPrompt` pattern (load the pack, scan its `Tools` for a matching `ID`). Changed `reviewOutcomeFace()` to build `FlowControlFace{Tool: face.ID, Map: face.StatusMap}` from this lookup, falling back to the pre-existing hardcoded literal only if the pack can't be loaded or has no such face — matching this codebase's established safe-degrade convention rather than breaking the review loop outright on a packaging problem.

`reviewOutcomeToFlowControl` needed no changes; it already calls `reviewOutcomeFace()` rather than duplicating the map itself, so the fix is fully contained to `reviewOutcomeFace()`'s own implementation.

**Left out of scope**: `ToolFace.PayloadMap` (also declared in the YAML, also parsed) is not yet read anywhere in the live path — `reviewOutcomeToFlowControl` still builds its `Payload` map directly from `ReviewOutcomeInput` fields. Wiring that up is a separate, smaller follow-up; folding it into this change would have mixed two independent "declared but unused" gaps into one commit.

## Verification

- New test `TestReviewOutcomeFaceReadsFromPackDeclaredStatusMap` (`agent_orchestrator_test.go`): asserts `reviewOutcomeFace()`'s `Tool`/`Map` equal `agentpack.LoadBuiltinToolFace("submit_review_outcome")`'s own `ID`/`StatusMap` — proving they now share one source of truth instead of two independently-maintained copies.
- Existing `TestResolveFaceStatusReviewOutcome`/`TestSubmitReviewOutcomeCannotChangeCap` (which exercise `reviewOutcomeFace()`'s actual mapping behavior) pass unchanged, since the pack's values are identical to the old literal.
- Full suite: 1007 passed, 15 pre-existing/environmental failures (unchanged from before this fix).
- `go build ./...` clean.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-NOTE-CP42
change_type: bugfix
summary: reviewOutcomeFace now builds its FlowControlFace from the pack's own declared submit-review-outcome.yaml statusMap via a new agentpack.LoadBuiltinToolFace helper, instead of an independent hardcoded Go literal duplicating the same values
# --->8---
