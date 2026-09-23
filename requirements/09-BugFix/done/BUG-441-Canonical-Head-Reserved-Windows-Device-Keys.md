---
id: BUG-441
title: Canonical Head validator accepts reserved Windows device names
status: done
version: 2
created: 2026-09-23
updated: 2026-09-23
owner: FlowPilot
linked: [BUG-427, CA-927]
---

## AI Quick View
- **What**: Keys such as CON/NUL can pass validation yet cannot yield portable Windows filenames.
- **Why**: BUG-427 only filters illegal characters and bare dot segments, not reserved device names.
- **Key constraint**: Reject portably invalid names before write without breaking legitimate kebab-case keys.

## 1. Metadata
- Document ID: `BUG-441`
- Phase: `bugfix`
- Status: `done`
- Feature Keys: `change-contract`
- Parent Documents: [BUG-427](../done/BUG-427-Preexisting-Suite-Failures-Frozen-Writer-Context-And-Platform-Deps.md), [CA-927](../../../change-audit/CA-927-portable-canonical-head-feature-key-validation.md)

## 2. Symptom and Impact
`unsafeHeadFeatureKey` (`internal/changecontract/head.go:69-88`) rejects blank, `.`/`..`, `<>:"/\|?*` and controls <0x20 but accepts `CON`, `con`, `NUL`, `PRN`, `AUX`, `COM1`, `LPT1`. `<key>.json` is Windows-reserved even with extension. `StageHeadWrite` (`:140-155`) can stage on POSIX but fail or target a device on Windows, contradicting CA-927's portability promise. Severity: **medium**.

## 3. Reproduction and Evidence
By each current predicate, `unsafeHeadFeatureKey("CON")` is false; `StageHeadWrite` builds `canonical/CON.json`. BUG-427's test (`bug427_head_key_safety_test.go:12-34`) excludes reserved names. Static proof only; Windows live verification pending.

## 4. Acceptance and Verification
Add red tests for CON/nul/PRN/COM1/LPT1 including extensions, prove deterministic rejection on POSIX and Windows; preserve existing valid-key tests and run changecontract/runner suites before CA/doc close. Not fixed here.

## 5. Resolution (2026-09-23, CA-930)

- `unsafeHeadFeatureKey` rejects Windows reserved device names (CON, PRN, AUX,
  NUL, COM1-9, LPT1-9; case-insensitive, extension-tolerant) at
  `StageHeadWrite`; `LoadHead` treats them as absent.
- Test: `changecontract/bug441_reserved_device_keys_test.go` — red before fix.
- `go test -count=1 ./internal/changecontract` — all green.
