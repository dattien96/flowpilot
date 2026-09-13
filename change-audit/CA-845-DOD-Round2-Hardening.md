# CA-845 — round-2 review hardening: numbered DOD headings, h1/h2 section close, fence-aware done detection

# ---8<--- flowpilot:change-ledger
feature_key: context-regression-engine
source_doc_id: Task-330
change_type: bugfix
summary: second independent-review round — accept the numbered "## 9. Definition of Done" heading form used by every real Task/BUG doc (round-2 blocking finding), close the DOD section only on h1/h2 so ### sub-headings keep the checklist intact, fence-aware hasDoneMetadata, de-vacuous the docscan pre-region test, fix stale done/-links + draft metadata template
# --->8---

## Why

Round-2 fresh-eyes review (7 independent agents) found one BLOCKING defect the synthetic fixtures had masked: dodHeadingRegex only matched the unnumbered heading, while 100% of real Task-*/BUG-* docs use the numbered form — the gate would have false-reprompted every conformant write. The same real-repo scan surfaced two more parser gaps: `###` sub-headings inside DOD sections terminated parsing early, and legacy prose/backticked-marker DODs stay non-conforming by design (CP-47 R-1).

## Change

- `flowgate/dod.go`: dodHeadingRegex accepts an optional numbered prefix (`## 9. Definition of Done`, `## 11. Definition of Done`); anyHeadingRegex closes the section only on h1/h2 (`^#{1,2}\s+`) so `###` sub-headings inside the checklist are counted.
- `flowgate/evaluate.go`: hasDoneMetadata skips fenced code blocks (a quoted `- Status: done` example is documentation, not a done signal).
- `docscan/docscan_test.go`: the pre-region autofix test was vacuous (hoist guard comparison flipped — it exercised the no-op path; mutation-verified catch now) + structural no-vacuous assertions.
- `runner/reverse_doc.go` + requirements docs: draft metadata template and Task-332/CP-48/CP-49 links updated to the done/ locations.

## Tests

`dod_test.go`: numbered heading forms (Task/BUG + lookalike rejections) and the locked real-repo acceptance test — every done/ Task/BUG doc carrying a contract-form DOD checklist (independent ground-truth scan) must parse Present: 16/16 recognized, 9 legacy-style documented. `r_dod_complete_test.go`: fenced done-metadata ignored. Full flowgate + docscan + runner targeted suites green.

## Providers

Case 1 agnostic (deterministic Go, 0 LLM).

## Prior claims intact

CA-833..CA-844 — hardening only; all previous behavior tests unchanged and green.
