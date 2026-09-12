# CA-843 — harden skilllearn export path safety and slug hygiene

# ---8<--- flowpilot:change-ledger
feature_key: runtime-intelligence
source_doc_id: Task-336
change_type: bugfix
summary: independent-review hardening — candidate Group is rendered as a safe single path segment (traversal like ../.. collapses; ordinary groups untouched) and byte-truncated slugs stay valid UTF-8
# --->8---

## Why

Fresh-eyes review round 1: `filepath.Clean(group)` still allowed `..` segments — a user-editable Group of `../..` (via UpdateCandidate) could write outside the skills dir; and Slugify's byte truncation at maxSlugLen could split a multi-byte rune, producing an invalid-UTF-8 directory name.

## Change

- `skilllearn/exporter.go`: GroupDirName — groups already in [a-z0-9_-] pass through unchanged ("golang" stays "golang"), anything else (separators, dots, unicode) collapses via Slugify; empty → "common". trimTrailingPartialRune keeps byte-truncated slugs valid UTF-8. reserveSkillDir uses GroupDirName.

## Tests

`skilllearn_test.go`: GroupDirName path-safety table (golang/common unchanged; "../.."→safe; separators collapse) + end-to-end export with Group "../.." asserts every written path stays inside the workspace root. Full skilllearn suite green (20 tests).

## Providers

Case 1 agnostic (deterministic Go, 0 LLM).

## Prior claims intact

CA-839 — DOD export targets (.agents/.claude/.grok/flow-pack), version suffixing, and the conflict gate all re-verified by the unchanged suite.
