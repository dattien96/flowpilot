# CA-846 — round-3 review hardening: h3/multi-part/parenthesized DOD heading forms

# ---8<--- flowpilot:change-ledger
feature_key: context-regression-engine
source_doc_id: Task-330
change_type: bugfix
summary: third independent-review round — accept h3 DOD headings ("### 6.1 Definition of Done (DOD)" etc., 62 of 79 real contract-form docs), parenthesized variants ("## 6. Acceptance Check (Definition of Done)"), widen the locked real-repo ground truth past its h2 bias (78/78 recognized)
# --->8---

## Why

Round-3 fresh-eyes review caught the same real-world-mismatch defect class round 2 fixed, one level deeper: dodHeadingRegex still required h2, while 62 of 79 real Task/BUG docs carrying contract-form checklists open their DOD with h3 headings (multi-part numbering "6.1"/"6.2") and 8 more embed the phrase parenthesized ("## 6. Acceptance Check (Definition of Done)"). The round-2 locked acceptance test could not see the miss because its ground-truth regex shared the parser's h2-only bias — circular acceptance evidence.

## Change

- `flowgate/dod.go`: dodHeadingRegex accepts h2/h3 openings with multi-part numbering (`\d+(\.\d+)*[.)]?`); new dodParenRegex for parenthesized DOD phrases in h2/h3 headings; gofmt realigned.
- `flowgate/dod_test.go`: ground truth of the locked real-repo test widened to the same any-level forms (independently stated), plus unit tests for the h3/paren/lookalike cases.

## Tests

`TestParseDefinitionOfDone_H3AndParenHeadingForms` (h3 multi-part, h3 plain, parenthesized, non-DOD h3 rejection) and the re-locked `TestParseDefinitionOfDone_RecognizesRealRepoDocs`: 78/78 contract-form done-dir docs parse Present; 9 legacy-style docs (prose bullets, backticked markers) intentionally non-conforming per CP-47 R-1. Full flowgate suite green; gofmt/vet clean.

## Providers

Case 1 agnostic (deterministic Go, 0 LLM).

## Prior claims intact

CA-833..CA-845 — behavior of previously recognized forms unchanged (all prior tests green); the "16/16" claim in CA-845 was an artifact of its own h2-biased ground truth, superseded by this note.
