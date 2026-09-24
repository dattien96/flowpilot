# BUG-370 — Frozen-scope drift parks on markdown (FEATURE-KEYS.md, tdd-signatures.md)

## Metadata

- Document ID: `BUG-370`
- Title: `Frozen coder-gate parks on *.md writes — FEATURE-KEYS.md and tdd-signatures.md loop ask_user`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-11`
- Last Updated: `2026-09-24`
- Parent Documents: [CP-60](../../07-Coding-Plan/done/CP-60-Vibe-Working-Mode.md), [CP-55](../../07-Coding-Plan/done/CP-55-Flow-First-Preflight-Contract-Context-Retrieval-And-Canonical-Acceptance.md)
- Child Documents: `None`
- Related Documents: [BUG-366](./BUG-366-Allow-Doc-Drift-Amend-422.md), [BUG-327](../done/), [CA-427](../../../change-audit/CA-427-enforce-frozen-scope-at-coder-gate-and-amendments.md)
- Replaces: `None`
- Tags: `vibe-mode, change-contract, frozen-scope, live-bed`

## AI Quick View

### Summary

- Live run-678326: coder/TDD wrote `change-audit/FEATURE-KEYS.md` and `requirements/.flowpilot/vibe/tdd-signatures.md`. Frozen gate parked (`flow scope drift: … FEATURE-KEYS.md` / `tdd-signatures.md`). The implementer then asked the operator "what is the FlowPilot failure?" on every retry.
- Operator: markdown is not product code; ignore `*.md` on the frozen drift gate. Keep `.flowpilot/settings/flow-rules.json` as drift (CA-427).
- Ask_user loop is the agent reacting to those parks — no separate ask_user parser fix.

### Current Ask

- Writing `*.md` (FEATURE-KEYS, tdd-signatures, SS/CP/Task, CA notes) does not park the frozen coder-gate. `src/extra.go` and `flow-rules.json` still park.

### Key Decisions

- `V-1` Skip `IsMarkdownDocPath` in `gate_hook` next to the other bookkeeping exemptions. Do not use full `IsDocOrAuditFile` (that exempts all of `.flowpilot/**`).
- `V-2` BUG-327 FEATURE-KEYS-still-blocks assertion is superseded for markdown; extra.go still drifts.
- `V-3` Allow/amend for docs (BUG-366) stays; it is unused when the gate never parks.

### Constraints

- `feature_key: vibe-mode` (live) / `change-contract` (gate). Dominant: vibe-mode.
- R1: new tests plus a required assertion flip on `TestBUG327_FeatureKeysWriteStillBlocks` (operator AC). Other old tests untouched.
- R2: agnostic.

### Open Questions

- None.

### Source Refs

- TUI 2026-09-11: drift banners for `tdd-signatures.md` and `FEATURE-KEYS.md`; ask_user "What is the FlowPilot failure blocking Continue?"

## 1. Issue Summary

Vibe TDD/coder must write markdown the freeze never declares. Parking those writes loops the human on an unanswerable ask_user card.

## 2. Parent Links

- impacted coding plan: CP-60, CP-55 P-4
- impacted tech design: SD-24, SD-21
- impacted system spec: SS-18

## 3. Environment and Reproduction

- environment: TUI vibe-ingest gate-sandbox, grok-4.5, run-678326
- reproduction steps: sprint writes FEATURE-KEYS.md or tdd-signatures.md → escalate drift → coder ask_user
- frequency: every vibe sprint that follows audit-logging / TDD pack paths

## 4. Expected vs Actual

- expected: markdown writes are ignored; only extra source/json gate-rules park
- actual: drift park + ask_user loop

## 5. Impact

- users affected: vibe operators
- workflows affected: vibe-ingest / vibe-sprint
- severity: high (run cannot Continue without answering a nonsense question)

## 6. Root Cause

- hypothesis: frozen gate exemptions are bookkeeping-only; *.md is still "code"
- confirmed cause: `codeOnlyWritten` filter in `gate_hook.go` did not skip markdown
- evidence: live gate reason strings; CA-427 comment that "every doc file remains subject to enforcement"

## 7. Fix Strategy

- `F-1` `IsMarkdownDocPath` (*.md or `requirements/**`)
- `F-2` Skip it in the frozen drift filter
- `F-3` Invert FEATURE-KEYS park assertion (BUG-327 / BUG-366 first-write) to match operator AC

## 8. Validation

- `V-1` New `TestBUG370_*`: md does not park; extra.go parks; flow-rules.json parks
- `V-2` `TestFlowScopeDriftBlocksAcceptance` still parks extra.go

## 9. Regression Guard

- tests: `changecontract/bug370_markdown_doc_path_test.go`, `runner/bug370_markdown_not_scope_drift_test.go`
- alerts: none
- audit checks: CA-825

## 10. Follow-Up Document Updates

- upstream docs that must change: none (CA-427 still owns flow-rules.json)
- notes left unchanged on purpose: CA-427 bookkeeping-only `.flowpilot/**` rule
