# Task-097: Feature Catalog And Resolver

## Metadata

- Document ID: `Task-097`
- Title: `Feature Catalog And Resolver`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-23`
- Last Updated: `2026-06-23`
- Parent Documents: [CP-35: Context And Regression Engine Rollout](../../07-Coding-Plan/inprogress/CP-35-Context-And-Regression-Engine-Rollout.md), [SD-17: Context And Regression Engine](../../06-System-Tech-Design/SD-17-Context-And-Regression-Engine.md), [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md)
- Child Documents: `None`
- Related Documents: [Task-096: Commit-History Ledger](./Task-096-Commit-History-Ledger.md), [SD-10: Context Resolver & RAG](../../06-System-Tech-Design/SD-10-Context-Resolver-RAG.md)
- Replaces: `None`
- Tags: `featurecatalog, resolver, context-slots, local-runner`

## AI Quick View

### Summary

- Map a natural-language ask (e.g., "update the chat UI") to a `feature_key`, then inject that feature's ordered history into the step prompt.
- Resolution = lexical match + an LLM pick-from-list over a small catalog. No vector DB.
- Wires the `feature.resolve` and `feature.history` context slots at priority 1.

### Current Ask

- Implement `internal/featurecatalog/` (`catalog`, `resolve`, `slots`) per CP-35 §4.2.

### Key Decisions

- `T-1` Catalog built from SS/CP titles + `AI Quick View` summaries + ledger `feature_key`s + file globs.
- `T-2` Ambiguous resolution falls back to an LLM pick over the inline candidate list; embeddings are an optional later flag only.

### Constraints

- Depends on Task-096 (ledger). Extends CP-10 `internal/contextresolver/`; if absent, ship the minimal slot dispatch in CP-35 §4.2.

### Open Questions

- Confidence threshold + confirm UX (`SD-17 Q-1`).

### Source Refs

- `CP-35 §4.2` (P-2); `SD-17 D-4`, §3.3; `SS-14 AC-10`, `AC-3`.

## 1. Goal

A resolver that turns a plain-language request into the right `feature_key` and packs that feature's ordered history (newest last) into the prompt.

## 2. Parent Links

- coding plan: `CP-35` P-2
- tech design: `SD-17` `D-4`, §3.3, §4.2.1 packing
- system spec: `SS-14` AC-10, AC-3
- specific upstream ids: `P-2`, `D-4`, `AC-10`, `AC-3`

## 3. Trigger

The user cannot supply a `feature_key`; the engine must resolve it from natural language before retrieving history.

## 4. Exact Change

- `T-1` `catalog.go` — `Feature` type + build from sources (a) SS/CP docs (b) ledger keys (c) file globs (d) `change-audit/FEATURE-KEYS.md` registry (authoritative keys, `SS-13 §13`) → `.flowpilot/catalog/features.ndjson`.
- `T-2` `resolve.go` — `ResolveFeature(nl) []Candidate`: lexical rank → LLM pick-from-list when ambiguous → candidates for user confirm.
- `T-3` `slots.go` — register `feature.resolve` + `feature.history` (priority 1); extend `step_context_slots.resolver` enum.
- `T-4` Prompt packing per CP-35 §4.2.1 (oldest→newest, "build on the newest, do not undo it").
- `T-5` unit tests: ranking, ambiguity → pick, packing order.

## 5. Touched Areas

- files: `apps/local-runner/internal/featurecatalog/*`, `internal/contextresolver/` (slot dispatch)
- modules: `featurecatalog`, `contextresolver`
- routes: none
- tables: `step_context_slots` (enum values)

## 6. Acceptance Check

- CP-35 P-2 DoD: "update the chat UI" resolves to the chat feature; ambiguous asks return >1 candidate; resolved history packed newest-last.
- `go test ./internal/featurecatalog/...` passes.

## 7. Out of Scope

- Vector/embedding resolution (optional later flag), the desktop confirm UI, the history source itself (Task-096).

## 8. Completion Notes

- result: planned
- follow-ups: consumed by the Flow Gate context (Task-099) and prompt assembly
- upstream docs updated: none
