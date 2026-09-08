# CA-766 — vibe r-requirement, owner-debate resolver, SS lock auto-sprint

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: CP-60
change_type: feature
summary: Vibe-only r-requirement always-block, map other vibe gates to vibe-owner-debate, inherit working_mode on children, SS lock then auto vibe-sprint
# --->8---

## Change

CP-60 P-2 / P-3 / P-4 / P-5. Does **not** edit `DefaultRules()` (TestDefaultRules stays 22).

- P-2: `flowgate.RequirementRule` + `EnabledRulesFor(working_mode)` + `CoerceVibeRequirementDrift`. Trigger `requirement_signature_drift` is always-block. Dev omits the rule.
- P-3: `classifyVibeGate` — r-requirement parks `BlockReason: requirement`; other vibe block/reprompt starts `vibe-owner-debate` instead of Dev 1/2/3 cards. Children inherit parent `workingMode`.
- P-4: `/vibe <requirement>` arms `vibe-ingest`. Ingest start awaits `ss_lock`/`cp_lock`. Slicer done seeds plan and starts sequential `vibe-sprint` (budget 8).
- P-5: additive tests only. Pack `vibe-sprint` v2 and `vibe-cp-ingest` already landed (CA-764 / CA-765).

## Tests

- `flowgate/r_requirement_test.go`
- `runner/vibe_gate_test.go`
- `runner/task326_p4_vibe_lock_test.go`
- `tui/app/task326_p4_vibe_tui_test.go`

## Provider impact

**Case 1 agnostic.** Resolver and lock queue do not branch on `providerKey`.

## Residual

- Manual Desktop/TUI lock-card click-through and live N× sprint demo not claimed.
- `r-requirement` card copy is the violation detail string, not a dedicated non-tech renderer.
- Task-321 / Task-323 remain parked as docs; pack + queue already exist.

## Will not undo

CA-763 / CA-764 / CA-765. CP-58 splitter. TestDefaultRules count.
