---
id: CA-930b
title: Canonical-head rejects Windows reserved device names (BUG-441)
type: BugFix
feature: change-contract
date: 2026-09-23
status: done
---

## Context

BUG-441: `unsafeHeadFeatureKey` (CA-927b) rejected NTFS-illegal characters and
`..` traversal, but still accepted reserved Windows device names — `CON`,
`PRN`, `AUX`, `NUL`, `COM1`–`COM9`, `LPT1`–`LPT9`, with or without a file
extension. Staging `CON.json` as a Head file is unwritable on Windows and can
silently open a device stream — a portability gap the same class as the `?`
rejection.

## Change

`internal/changecontract/head.go`:

- `unsafeHeadFeatureKey` now rejects the reserved device-name set
  (case-insensitive, extension-tolerant: `con`, `CON.json`, `lpt3` all reject)
  alongside the existing illegal-char/traversal checks. `StageHeadWrite` fails
  deterministically; `LoadHead` treats them as absent.

## Tests (added only)

- `internal/changecontract/bug441_reserved_device_keys_test.go` — every
  reserved name + extension variants rejected at staging; `LoadHead` absent;
  ordinary keys unaffected. Red before the change.

## Result

- `go test -count=1 ./internal/changecontract` — all green.

# ---8<--- flowpilot:change-ledger
feature_key: change-contract
source_doc_id: BUG-441
change_type: bugfix
summary: Canonical-head feature-key validation rejects Windows reserved device names (CON/PRN/AUX/NUL/COM1-9/LPT1-9)
# --->8---
