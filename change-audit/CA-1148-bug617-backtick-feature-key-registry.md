# CA-1148 — BUG-617: FEATURE-KEYS.md backtick-quoted keys read as unregistered

Live trigger: run-150388 / Task-032 audit card showed
`blocked_missing_feature_key — draft only`. PrivateVault's
`change-audit/FEATURE-KEYS.md` writes keys markdown-quoted
(`- `crypto-ndk` — …`); `featureKeyRegistered` only matched the bare
`- key ` form, so every key in that registry read as unregistered and every
vibe audit drafted blocked on the feature key — audits could never settle a
verified finalize, and the CA-1096 auto-finalize masked the defect.

## Fix

`featureKeyRegistered` now tokenizes the registry line (`- ` prefix +
first whitespace-separated token) and trims backticks before comparing —
both `- key — desc` and `- ``key`` — desc` forms register. Token equality
replaces prefix matching, which also closes the `crypto-ndk` /
`crypto-ndk-extra` prefix-collision hole and keeps tab-separated keys working
(`strings.Fields`).

## Tests (additive)

- `TestFeatureKeyRegisteredBacktickQuotedRegistry` — red before fix: quoted
  keys registered as missing; asserts both keys resolve and the
  `crypto-ndk`/`crypto-ndk-extra` prefix stays distinct.
- Existing audit-draft + run-127174 declared-key suites still green.

## Files

- `apps/local-runner/internal/runner/flow_audit_draft.go`
- `apps/local-runner/internal/runner/bug617_feature_key_backtick_registry_test.go` (new)
