# CA-847 — round-3 hardening: generated draft links resolve into requirements/

# ---8<--- flowpilot:change-ledger
feature_key: context-regression-engine
source_doc_id: Task-333
change_type: bugfix
summary: round-3 blocking fix — draft metadata template links used three ../ (overshooting requirements/ to the workspace root); corrected to two ../ and locked with a link-depth regression test (mutation-verified)
# --->8---

## Why

Round-3 review caught that commit 42326c6a fixed the stale todo→done filename in the draft metadata template but kept the wrong depth: drafts live at requirements/<phase>/todo/, so `../../../07-Coding-Plan/...` resolves to the workspace root, not requirements/. Every generated — and later published — SS/SD draft carried dead Related/Parent/Source-Ref links.

## Change

- `runner/reverse_doc.go`: 4 template sites corrected to `../../07-Coding-Plan/done/CP-49-...` / `../../05-System-Specs/SS-13-...`.
- `task333_standardize_test.go`: TestReverseDoc_DraftLinksResolveFromDraftDir — every parent-link in a generated draft must resolve INSIDE requirements/ (brownfield targets legitimately lack the referenced docs yet, so the invariant is depth, not existence). Mutation-verified: reverting to three ups fails the test.

## Tests

All 13 Task-333 targeted tests green with the new regression test.

## Providers

Case 1 agnostic (deterministic Go, 0 LLM).

## Prior claims intact

CA-836/CA-844 — SS-Lock semantics and the todo/ exclusion untouched.
