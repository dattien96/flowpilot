# Task-188: Canonical-Head Packing And Admin Visibility

## Metadata

- Document ID: `Task-188`
- Title: `Canonical-Head Packing And Admin Visibility`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-03`
- Last Updated: `2026-07-03`
- Parent Documents: [CP-43: Change Contract And Canonical Intent Signature](../../07-Coding-Plan/todo/CP-43-Change-Contract-And-Canonical-Intent-Signature.md) (P-5), [SD-21: Change Contract And Canonical Intent Signature](../../06-System-Tech-Design/SD-21-Change-Contract-And-Canonical-Intent-Signature.md), [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md) (AC-3, AC-8, AC-15)
- Child Documents: `None`
- Related Documents: [Task-186: Canonical Head And Intent Signature](./Task-186-Canonical-Head-And-Intent-Signature.md), [Task-187: Superseding Decision Records And Retire](./Task-187-Superseding-Decision-Records-And-Retire.md), [Task-097: Feature Catalog And Resolver](../../08-Task/done/Task-097-Feature-Catalog-And-Resolver.md), [Task-103: Engine Local Store And Drive Sync](../../08-Task/done/Task-103-Engine-Local-Store-And-Drive-Sync.md)
- Replaces: `None`
- Tags: `prompt-packing, contextresolver, contextsync, admin-web, canonical-head, local-runner`

## AI Quick View

### Summary

- Make the prompt lead with the Canonical Head (behavior + signature status + rejected decisions) **before** any ordered history; demote raw churn to a lower-priority budget item.
- Sync `canonical/*.json` via the CP-35 `context-engine/` Drive mechanism; keep `contracts.ndjson` local-only.
- Add an Admin Web "Canonical Head" panel: behavior, signature-status chip, rejected-decisions list, and the active step's in/out-of-scope diff.

### Current Ask

- Implement `SD-21 P-5`: Head-first packing extension, contextsync shared-set update, and the Admin panel.

### Key Decisions

- `T-1` Head is a mandatory packing slot; raw history is optional/lower priority (`SD-21 D-3`).
- `T-2` `spec_less` Heads render a low-confidence flag in the packed block.

### Constraints

- Extend CP-35 §4.2.1 `feature.history` packing; reuse CP-35 P-8 `contextsync` Drive sync; do not add DB tables.
- Non-fatal: a packing/sync failure degrades to prior behavior (`AC-9`).

### Open Questions

- Budget threshold at which raw history is dropped entirely vs summarized (ties to CP-23/CP-10 §5 packer).

### Source Refs

- `SD-21 §4`, `§6` (packing/sync contracts), `§7` step 2/8, `§13.3`. `CP-43 §4.5`. `SS-14 AC-3`, `AC-8`, `AC-15`.

## 1. Goal

On a resolved code turn, the AI reads the Canonical Head first (current truth + rejected dead-ends), not the chaotic ordered log; operators can inspect Head status and step scope in Admin.

## 2. Parent Links

- coding plan: `CP-43` P-5
- tech design: `SD-21` §6 (packing + sync), D-3
- system spec: `SS-14` AC-3, AC-8, AC-15
- specific upstream ids: `P-5`, `AC-3`, `AC-8`

## 3. Trigger

Task-186/187 produce the Head + decisions, but they only help if the AI reads the Head *before* the log. This task wires the Head into prompt assembly and exposes it for human inspection.

## 4. Exact Change

- `T-1` `featurecatalog`/`contextresolver` `feature.history` slot — prepend a Canonical Head block: `## Canonical state of "<feature>"` with `behavior_statement`, `intent_signature` short + `status` chip, and a "do NOT re-attempt" list from `decisions`; then the ordered history as a lower-priority section.
- `T-2` Budget: mark the Head block mandatory; the raw history block is lower priority in the packer (CP-23/CP-10 §5) and may be dropped/summarized under budget pressure — `log()` when dropped.
- `T-3` `spec_less` rendering — annotate the block "(spec-less — low confidence)" when `spec_confidence="spec_less"`.
- `T-4` `contextsync` (CP-35 P-8) — add `canonical/*.json` to the shared/Drive-synced set under `context-engine/`; assert `contracts.ndjson` stays local-only.
- `T-5` Admin Web "Canonical Head" panel (per feature): behavior statement, signature-status chip (`current`/`spec_drifted`/`code_drifted`/`spec_less`/retired), rejected-decisions list, and the active step's `Contract` with in-scope/out-of-scope path highlighting.
- `T-6` Tests — packing unit: Head prepended before history, `spec_less` flag present, decisions rendered; drop-history-under-budget path logs the drop. Integration: prompt leads with Head across Claude/Codex. Sync: `canonical/*.json` appears in the `context-engine/` manifest; `contracts.ndjson` never syncs.

## 5. Touched Areas

- files: `apps/local-runner/internal/featurecatalog/` (history slot), `internal/contextsync/` (shared set), `apps/admin-web/src/**` (Canonical Head panel)
- modules: `featurecatalog`, `contextresolver`, `contextsync`, admin-web
- routes: engine read endpoints for Head/contract (reuse existing engine API surface)
- tables: none (local JSON + Drive manifest)

## 6. Acceptance Check (DoD)

- [ ] The `feature.history` slot prepends the Canonical Head block before any ordered history on a resolved code turn.
- [ ] The Head block includes `behavior_statement`, `intent_signature` (short) + `status`, and the rejected-decisions ("do NOT re-attempt") list.
- [ ] Positive churn (`A → B → C → A`) is **not** replayed in the packed prompt by default; raw history is lower priority and its drop is logged.
- [ ] `spec_less` Heads render a low-confidence annotation.
- [ ] `canonical/*.json` syncs to `context-engine/` via the CP-35 P-8 mechanism with a manifest entry; `contracts.ndjson` never syncs.
- [ ] Admin "Canonical Head" panel shows behavior, signature-status chip, rejected decisions, and the active step's in/out-of-scope diff.
- [ ] Manual: confirmed the packed prompt leads with the Head for both Claude and Codex turns.
- [ ] `go test ./internal/featurecatalog/... ./internal/contextsync/...` passes; admin-web builds.

## 7. Out of Scope

- Head/signature/drift computation (Task-186) and decision folding/retire (Task-187).
- Contract capture and scope rules (Task-184/185).
- Per-code-unit Heads (`SD-21 Q-4`).

## 8. Completion Notes

- result: planned
- follow-ups: none — this completes the CP-43 P-1..P-5 slice.
- upstream docs updated: none
