# BUG-417: Auto-cataloged mega-glob features dominate `ResolveFeature` and hijack canonical-head injection

## Metadata

- Document ID: `BUG-417`
- Title: `Auto-created feature with ~100 file_globs out-scores real features — wrong bucketing, polluted r-fk suggestions, wrong canonical head in coder prompts`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Created: `2026-09-21`
- Last Updated: `2026-09-23`
- Parent Documents: [CP-37-Prompt-Context-Continuity](../../07-Coding-Plan/done/CP-37-Prompt-Context-Continuity.md), [CP-43-Test-Steps](../../07-Coding-Plan/done/CP-43-Test-Steps.md), evidence `~/fp-beds/lt-evidence/cp37/RESULT.md` (BUG-LIVE-CP37-003) + `~/fp-beds/lt-evidence/cp43/RESULT.md` (BUG-LIVE-CP43-003)
- Feature Keys: `feature-catalog`, `context-injection`, `canonical-head`, `gate-reprompt`

## AI Quick View

### Summary

- `ResolveFeature` scores +2 per query token found as a substring of a feature's joined `file_globs`; a chore commit (`30eb286`) auto-created feature `claude` with ~100 glob entries (`calc_test.go`, `stringutil/**`, `requirements/**`, `.claude/skills/**`, …), so almost any token hits it.
- Observed live: calc-work bucketed under `feature_key=claude` in `chat_summary`; the r-fk reprompt suggested non-registered keys `calc, calc-core, claude`; both bad-key commits ingested as feature `calc`.
- Same noise feature later hijacked canonical-head injection: run-961's implement coder prompt rendered `## Canonical "claude"` + `## History "claude"` while the frozen contract for that node declared `feature_key: calc-core` — the exact negative-knowledge channel CP-43 is meant to guarantee.

### Current Ask

- Fixed and verified in the bug-fix wave — see Completion Notes (implemented 2026-09-23).

## Bug report

### Symptom

Feature resolution, bucketing, gate suggestions, and flow-coder canonical injection all prefer an auto-cataloged noise feature (`claude`, created from a chore commit's whole-repo glob list) over the real registered feature.

### Expected

- Auto-derived features with giant glob lists do not out-score registered FEATURE-KEYS.md entries.
- r-fk reprompts suggest only registered feature keys.
- Flow coder prompts render the canonical head/history for the frozen contract's `feature_key`.

### Actual

1. `chat_summary` row for run-1/turn-1973 bucketed calc-work content under `feature_key=claude`.
2. r-fk reprompt text suggested `calc, calc-core, claude` — `claude`/`calc` are auto-derived ledger keys, not registered FEATURE-KEYS.md entries.
3. Both bad-key commits (`calculator`/`utils`) ingested as feature `calc`.
4. CP-43 run-961 implement coder prompt: `## Context - feature: claude` + `## Canonical "claude" [current spec-less]` + `## History "claude"` — while the same prompt's `### change.contract` block correctly showed the frozen `feature_key: calc-core`. The existing mitigation (declared-contract preference at audit time, catalog-confidence downgrade — cf. run-127174 "grok" noise case, `flow_validate_audit_dispatch.go` ~:1265) does NOT cover the coder-child prompt path.

### Impact

Resolution/bucketing quality degrades as chore commits accumulate; coders are shown canonical state and history for the wrong feature — a wrong-context channel worse than no context (it actively teaches stale/rejected decisions for an unrelated feature).

## Reproduction

1. Let a chore commit touching many paths (e.g. `chore: accumulated sandbox state`, commit `30eb286`) auto-create a catalog feature whose `file_globs` cover most of the repo (`claude` with ~100 entries).
2. Run any chat turn mentioning calc-work → `ledger-chat-summary.ndjson` buckets it under `claude`.
3. Trigger an r-fk reprompt → suggested-key list contains `claude`/`calc`.
4. (CP-43) Run bug-harness with a frozen `calc-core` contract after the noise feature exists → implement child's prompt shows `## Canonical "claude"`.

## Root cause

- `apps/local-runner/internal/featurecatalog/resolve.go:15` — `ResolveFeature` gives +2 per query token appearing as a substring of a feature's joined `file_globs`; a ~100-glob feature matches nearly every token (`calc`, `go`, `add`, `min` via `naming`, `int` via `injection`, `commit` via `git-commit-format`).
- Auto-cataloged entries are never penalized vs registered FEATURE-KEYS.md entries, and the coder-child prompt path does not apply the declared-contract preference that the audit path has (`flow_validate_audit_dispatch.go` ~:1265).

## Evidence

- `~/fp-beds/lt-evidence/cp37/RESULT.md` — BUG-LIVE-CP37-003: `catalog-features.ndjson` (`claude` glob list), `ledger-chat-summary.ndjson` (run-1/turn-1973 → claude), `l372-reprompt-queued.txt` (suggested keys).
- `~/fp-beds/lt-evidence/cp43/RESULT.md` — BUG-LIVE-CP43-003: `runner.log:13880` (implement session/prompt payload), `l435-claude-head.json`, `catalog features.ndjson` `claude` row.

## Severity

- `medium` — context quality + wrong-feature canonical injection into coder prompts; no crash, but it defeats the CP-43 negative-knowledge guarantee whenever noise features exist.

## Completion Notes (implemented 2026-09-23, CA-924)

- Root cause: `ResolveFeature` scored glob-substring hits per glob per query
  token, so an auto-derived feature with a huge ledger glob set accumulated
  unbounded score from volume alone; r-fk suggestions then echoed whatever
  won, including unregistered catalog rows.
- Fix: `internal/featurecatalog/resolve.go` caps glob-substring contribution
  per query token; `internal/runner/gate_hook.go` filters r-fk suggestion
  candidates through `loadKnownFeatureKeys` (FEATURE-KEYS.md registered set).
- Tests: `internal/featurecatalog/bug417_resolve_test.go`,
  `TestBug417_RFKSuggestionsOnlyRegisteredKeys` (red by assertion pre-fix).
- Live: `/tmp/fp-live-i` run-3221 — produced package resolved
  `feature: calc-core` (declared) over the seeded `mega-noise` catalog row;
  run-2926 without a contract still resolved `mega-noise`, matching the
  reported pre-fix behavior.
