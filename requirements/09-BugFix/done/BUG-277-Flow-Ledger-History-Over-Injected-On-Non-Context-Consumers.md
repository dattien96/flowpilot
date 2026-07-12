# BUG-277: Flow Ledger History Over-Injected On Non-Context Consumers

## Metadata

- Document ID: `BUG-277`
- Title: `Flow Ledger History Over-Injected On Non-Context Consumers`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `Codex / owner`
- Created: `2026-07-11`
- Last Updated: `2026-07-12`
- Parent Documents: [CP-45: Generic Artifact Types And Instances](../../07-Coding-Plan/done/CP-45-Generic-Artifact-Types-And-Instances.md), [Task-224: Flow Prompt Scoping And Coder Output Why Template](../../08-Task/done/Task-224-Flow-Prompt-Scoping-And-Coder-Output-Why-Template.md)
- Child Documents: `None`
- Related Documents: [Task-223](../../08-Task/done/Task-223-File-Artifact-Output-Contract-And-Review-Input-Chain.md), [BUG-275](../done/BUG-275-Hub-Synthesis-Prompt-Duplicates-Joined-Notes-And-Misresolves-Feature-History.md), [BUG-276](../done/BUG-276-File-Artifact-Input-Should-Mention-Paths-Not-Paste-Content.md), [CA-288](../../../change-audit/CA-288-cp45-e2e-edge-dirty-file-path-only-hub-dedupe.md)
- Replaces: `None`
- Tags: `agent-flow-engine, feature-history, prompt-composition, regression, review-handoff`

## AI Quick View

### Summary

- After the context package is handed to coder, **reviewer** (and other late flow nodes) still receive a second full `## Prior work` / `## Prior discussion` block via `injectFeatureHistoryPrompt`.
- Reviewer also receives a large paste of the **coder final message** even when a `file_artifact` path already carries the deliverable (path-only inject is BUG-276 done; final-message dump remains).
- Owner policy: full history bulk belongs to **main hub (user turns)** and the **step immediately after `context.produce`** only; review is deliverable-centric.

### Current Ask

- Fix inject placement and review handoff per Task-224; pair with coder Why template so review does not need the ledger.

### Key Decisions

- `V-1` Do not double-serve full history to review when coder already consumed the package.
- `V-2` When file INPUT paths exist, omit full coder final message (path+tools first); no optional long brief in v1.
- `V-3` Implementation is Task-224; this bug is the observed residual symptom.

### Constraints

- Must not remove package from first post-context consumer.
- Must not reintroduce full file body paste (BUG-276).

### Open Questions

- None blocking; defaults fixed in Task-224.

### Source Refs

- `run-658` / `prompt-turn-663.txt` — Prior work at top + full coder final + path-only file section.
- `run-314` / `prompt-turn-319.txt` — package + required outputs (desired coder shape).
- `run-309` / `prompt-turn-1212.txt` — hub synthesis after join.

## 1. Issue Summary

Flow Mode over-injects feature ledger history onto non-context consumers (especially reviewer). Combined with pasting the full coder final message into the review prompt, the reviewer prompt is noisy and redundant with the file deliverable path contract.

## 2. Parent Links

- impacted coding plan: `CP-45`
- impacted tech design: `SD-17`, `SD-23`
- impacted system spec: `SS-14` (context continuity) as applicable

## 3. Environment and Reproduction

- environment: desktop + local-runner, gate-sandbox, flow context→coder→review→synthesis
- reproduction steps:
  1. Run flow with context package + file OUTPUT on coder + file INPUT on review.
  2. Open reviewer `prompt-turn-*.txt`.
  3. Observe leading `## Prior work` / `## Prior discussion` and large coder final message and path-only bound file section.
- frequency: every such flow advance (deterministic)

## 4. Expected vs Actual

- expected: reviewer gets review instruction + file path(s) (+ optional no body); history only on hub user turns + coder package hop.
- actual: reviewer gets full ledger inject + full coder dump + paths.

## 5. Impact

- Prompt bloat and duplicate history on review nodes.
- Reviewer attention diluted; risk of reopening settled decisions already closed in package/history.
- Harder audit of “what review actually used” (file vs chat paste).

## 6. Root Cause

- hypothesis (confirmed by log + code paths; implement under Task-224):
  - `injectFeatureHistoryPrompt` runs on child turns without a “first post-context consumer only” / package-provenance gate.
  - `tryAdvanceFlowFromNode` embeds full coder `resultMessage` regardless of file_artifact INPUT bindings.

## 7. Fix Strategy

- `F-1` Task-224 §4.1 — scope ledger inject; track first post-context consumer by **flow transition / package provenance**, not agent display name.
- `F-2` Task-224 §4.1 `T-2` — when file INPUT paths bound, **omit** full coder final body (default).
- `F-3` Task-224 §4.2 — coder OUTPUT Why template so review need not re-read ledger.

## 8. Validation

- `V-1` Equivalent to Task-224 `AC-1`–`AC-7`.
- `V-2` Reviewer prompt must not start with full Prior work block when not first post-context consumer.
- `V-3` Reviewer with file INPUT: no full coder final body; path-only file section remains.

## 9. Regression Guard

- tests: reviewer composition without full prior-work; no full coder final when file paths exist; coder still receives Flow Context Package; BUG-276 path-only; Task-223 OUTPUT gate.
- alerts: none
- audit checks: live gate-sandbox retest with calc-core summary file

## 10. Follow-Up Document Updates

- upstream: optional note on Task-223/SD-23 that INPUT is path-only (BUG-276/CA-288 already product rule).
- CA-288 remains closure for BUG-274/275/276; closed with Task-224 / CA-289 (2026-07-12).
