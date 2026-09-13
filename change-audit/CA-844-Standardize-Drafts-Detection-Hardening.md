# CA-844 — harden /standardize draft detection and gitnexus exec teardown

# ---8<--- flowpilot:change-ledger
feature_key: context-regression-engine
source_doc_id: Task-333
change_type: bugfix
summary: independent-review hardening — un-approved SS-Lock drafts under todo/ no longer count as existing docs (post-restart /standardize re-offers the approval modal instead of stranded conformance) and gitnexus exec gets a WaitDelay so a hung npx grandchild cannot block past the context deadline
# --->8---

## Why

Fresh-eyes review round 1: (1) matchPhaseDocs' recursive walk descended into `todo/` segments, so SS-Lock drafts stranded after a runner restart (in-memory gate lost) counted as existing docs — a re-run reported conformance/mixed and never re-offered the approval modal, permanently stranding the drafts; (2) exec.CommandContext kills only the direct child — via the npx path a hung grandchild holding the output pipe could block the synchronous handler past the 20s query context.

## Change

- `runner/standardize_cmd.go`: matchPhaseDocs skips todo/ subtrees (un-approved drafts are not published docs; wholeProjectHasDocs already only read phase roots).
- `runner/reverse_doc.go`: cmd.WaitDelay = 5s on the gitnexus exec.

## Tests

`task333_standardize_test.go`: TestStandardize_StrandedDraftsInTodo_ReofferReverseDoc — a todo/ SS draft yields Mode=reverse_doc + waiting_ss_lock (fresh approval offer). All 12 Task-333 tests green.

## Providers

Case 1 agnostic (deterministic Go, 0 LLM).

## Prior claims intact

CA-836 — SS-Lock non-bypassability map unaffected (publish path unchanged); the 12 targeted tests and Task-331/334 seam tests stay green.
