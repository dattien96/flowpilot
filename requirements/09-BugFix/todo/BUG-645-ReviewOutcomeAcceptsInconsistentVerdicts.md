# BUG-645 — `submit_review_outcome` accepts `status:"approved"` alongside per-AC `blocked` verdicts and returns `done/advancing` without validating consistency

- **ID:** BUG-645
- **Severity:** High — a review can read "approved + all ACs blocked" and
  the hub advances anyway; fail-closed verdicts are silently inverted.
- **Status:** OPEN (captured live, run-523131)

## Evidence chain (all live)

1. run-523131: a review-cohort member submitted `submit_review_outcome`
   with top-level `status:"approved"` while its per-AC verdicts were
   `blocked`; the tool returned `done`/advancing (evt-527258).
2. The verdict table was the actual gate input — the mixed payload meant
   "not reviewable" yet routed as "passed".

## Root cause (hypothesis)

`submit_review_outcome` validates AC coverage (missing AC → reject) but
not AC-status coherence: a `blocked`/`fail` verdict under an `approved`
envelope is not rejected or normalized to `blocked`.

## Fix direction

- `F-1` Validate envelope↔verdict coherence at submit time: any AC
  verdict `fail`/`blocked` forces envelope `changes_requested`/`blocked`
  — reject or coerce the inconsistent envelope (fail closed).
- `F-2` Define precedence: per-AC verdicts are the truth; envelope is
  derived server-side, not taken as input.

## Regression coverage

- `TestBug645_ApprovedWithBlockedACRejected` — mixed payload rejected.
- `TestBug645_EnvelopeDerivedFromVerdicts` — envelope computed from
  per-AC rows, not accepted verbatim.
