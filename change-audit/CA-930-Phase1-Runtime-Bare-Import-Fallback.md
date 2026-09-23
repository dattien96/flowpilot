# CA-930 — phase1 test runtime: bare-import fallback to app node_modules

## Summary

`node --require scripts/phase1-runtime.js --test .phase1-tests/…` failed
with `Cannot find module 'zustand'` for every test that transitively
imports the store — the compiled output lives under `.phase1-tests/`
(outside `apps/desktop-flowpilot/`), so Node's node_modules walk-up never
reaches `apps/desktop-flowpilot/node_modules`.

## Fix

In `phase1-runtime.js`, after alias mapping and after the original
resolution fails, bare (non-relative, non-absolute) specifiers fall back to
`require.resolve(request, { paths: [apps/desktop-flowpilot/node_modules,
<root>/node_modules] })`. Original resolution is tried first, so this is
a pure fallback — no behaviour change where modules already resolve.

Verified: `state/` and `lifecycle/` suites now run without `NODE_PATH`
workarounds; 20/20 lifecycle + worktree tests pass.

## Files

- `scripts/phase1-runtime.js`

# ---8<--- flowpilot:change-ledger
feature_key: dev-infra
source_doc_id: CP-83
change_type: feature
summary: phase1 runtime resolves bare package imports via the desktop app's node_modules as fallback
# --->8---
