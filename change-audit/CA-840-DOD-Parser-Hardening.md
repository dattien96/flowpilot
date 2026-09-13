# CA-840 — harden DOD parser (fence-aware) and done-signal detection

# ---8<--- flowpilot:change-ledger
feature_key: context-regression-engine
source_doc_id: Task-330
change_type: bugfix
summary: independent-review hardening — fenced code blocks no longer open or feed the DOD section (gaming vector closed), strings.Split removes the 64KB silent-truncation limit, path traversal/symlink guard added to MissingDodDocs, and the done-metadata regex accepts the backticked `- Status: `done`` form (Task-331 fail-open closed)
# --->8---

## Why

Fresh-eyes review round 1 (7 independent agents): (1) a fenced example DOD satisfied the r-dod-present gate without a real DOD and fenced fake checkboxes inflated counts; (2) bufio.Scanner's 64KB/line limit silently truncated parsing (false reprompt); (3) MissingDodDocs lacked the EvalSymlinks/`..` guard its sibling MissingRequiredFileArtifactOutputs has; (4) `dodDoneMetadataRegex` missed the backticked `- Status: `done`` metadata form this repo's own docs use — a fail-open on r-dod-complete masked only by the done/ path backup.

## Change

- `flowgate/dod.go`: ParseDefinitionOfDone tracks ``` / ~~~ fences (heading/checkbox lines inside fences are skipped; unclosed fences degrade safely), strings.Split replaces bufio.Scanner; MissingDodDocs rejects absolute/`..` paths and resolves symlinks like MissingRequiredFileArtifactOutputs.
- `flowgate/evaluate.go`: dodDoneMetadataRegex accepts optional backticks around the done value (mirrors vibeDocMetadataStatus).

## Tests

`dod_test.go`: fenced-fake-DOD ignored (+ unclosed fence), 200KB-line no-truncation. `r_dod_complete_test.go`: backticked done metadata in a todo/ dir triggers DodDoneTransition. Full flowgate suite green.

## Providers

Case 1 agnostic (deterministic Go, 0 LLM).

## Prior claims intact

CA-833..CA-839 — hardening only; every Task-330/331 behavior test still passes unchanged.
