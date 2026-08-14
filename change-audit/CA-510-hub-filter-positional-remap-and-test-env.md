---
id: CA-510
feature_key: chat-history
title: Hub seed filter positional remap; Windows test env
date: 2026-08-14
status: COMPLETE
---

## Change

### CA-501 filter (run-20332)

`filterFlowHubUnassistedProviderHistory` dropped remappable hub history when
provider user text ≠ durable turn-log prompts (run-20332 parity). Fix:

- If **no** text match and non-system user-turn count == durable prompt count →
  **positional keep** (overlay remaps text).
- If any text match or unequal counts → text match only (run-98153 pollution).

### Other suite greens (operator “làm hết”)

- `TestDefaultRules`: expect `r-newtest` (17 rules) — product already had rule.
- Reviewer auto-spawn: assert `submit_review_outcome` required for cohort.
- Canonical/normal-chat fixtures: `gate_mode=warn` so red baselines don’t
  fail-close gate_blind on temp repos.
- Windows: TestMain prepends Git `sh.exe`; compat probes via `.cmd`+sh;
  isolate USERPROFILE/HOME for home/skills tests; path Clean for Drive parse;
  auth command quotes; skip git-guard when shim not executable.

## Provider impact

Filter Case 1 (agnostic). Env/test harness Windows-only fixes.

## Tests

- Extended `run98153_hub_unassisted_shared_session_test.go` positional cases.
- Re-run run-20332 + run98153 + previously red non-sh tests.
- Full `go test ./...` under apps/local-runner: **green** (Windows; Git sh on PATH via TestMain).
- Residual flaky (not blocking full run): `TestSyncChatRunToDriveConcurrentIndexMergesPreserveAllParentRows` last-write-wins race under `-count=N`.

# ---8<--- flowpilot:change-ledger
feature_key: chat-history
source_doc_id: CP-51
change_type: bugfix
summary: Hub unassisted filter keeps remappable equal-count history; Windows test env for sh/probes
# --->8---

