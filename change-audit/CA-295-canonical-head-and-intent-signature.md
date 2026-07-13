# CA-295 — Canonical Head And Intent Signature (Task-186, CP-43 P-3)

## Scope

Implemented (partially — see caveat) [Task-186](../requirements/08-Task/todo/Task-186-Canonical-Head-And-Intent-Signature.md) (P-3 of [CP-43](../requirements/07-Coding-Plan/todo/CP-43-Change-Contract-And-Canonical-Intent-Signature.md)): a per-feature `CanonicalHead` carrying a reproducible `intent_signature` over the governing spec + declared behavior (never code bytes), with drift detection and a birth path for brand-new features.

## Changes

- New `apps/local-runner/internal/changecontract/head.go`: `CanonicalHead` struct (status ∈ `spec_less|current|spec_drifted|code_drifted|renamed|merged|deprecated`, `spec_confidence`, embedded `Decision` records for Task-187), `LoadHead`/`SaveHead` — one `.flowpilot/canonical/<feature_key>.json` per feature.
- New `signature.go`: `ComputeSignature(h)` — sha256 over `feature_key` + sorted governing doc ids + sorted governing doc content hashes + normalized (whitespace/case-insensitive) `behavior_statement`; `HashDoc(path)` — sha256 of a doc file, returns an error (not a panic) on a missing/unreadable file.
- New `backfill.go`: `BuildHead(workspace, featureKey, ledger, catalog, birthContract)` — governing docs from `featurecatalog.DocRefs`; behavior statement backfilled from the newest `changeledger` entry (preferring its CA excerpt over the bare commit summary), or from `birthContract.Intent` when there's no history at all ("birth"); `spec_less`/`spec_backed` set by whether any governing docs resolved. `resolveDocRefPath` — new helper resolving a `DocRefs` stem (an exact filename, not a doc-id prefix) against every `requirements/` subfolder `featurecatalog` itself globs from, since no such reverse-resolution helper existed before.
- New `update.go`: `UpdateHead(h, c)` — folds an in-contract, gate-passing turn's intent into the Head and recomputes the signature; `RebaselineWithSpec(workspace, h, docIDs)` — re-baselines a `spec_less` Head to `current` once a human confirms newly-attached governing docs (a distinct transition from spec-drift per SD-21 §5).
- New `drift.go`: `SpecDrifted(workspace, h)` — true when any governing doc's on-disk hash no longer matches the Head's recorded hash (including a deleted doc, which resolves to an empty current hash); `CodeDrifted(hasOutOfContractChange, specDrifted)` — spec-drift takes precedence on the same turn.
- `flowgate/rules.go`: three new default rules — `r-spec-drift` (`governing_spec_changed`, warn), `r-code-drift` (`code_diverged_from_intent`, warn), `r-attach-spec` (`governing_spec_added_to_spec_less`, approve) — auto-seeded via the existing `MergeDefaultRules`. New `TurnResult` fields `HeadSpecDrifted`/`HeadCodeDrifted`/`HeadAttachSpecPending` carry the pre-computed signals in (same import-cycle reason as Task-185's fields — `flowgate` cannot import `changecontract`).
- `flowgate/evaluate.go`: matching `checkRule` cases for the three new triggers.
- `runner/gate_hook.go`: `captureChangeContract` extended to a 6-return-value function; new `updateCanonicalHead(cwd, c, hasOutOfContractChange)` loads (or backfills/mints) the feature's Head, detects `spec_drifted`/`code_drifted`/`attach-spec-pending`, and only calls `UpdateHead`+`SaveHead` when none of those apply — a drifted or pending Head is left untouched for human reconciliation (BR-2), never silently overwritten.

## Caveat — not fully done

`r-attach-spec` detection is wired end-to-end (the violation fires, and the Head is correctly frozen — `UpdateHead` is skipped — while `attachSpecPending` is true), and `RebaselineWithSpec` itself is implemented and unit-tested. But **no caller invokes `RebaselineWithSpec` yet** — there is no SD-16 approval-gate UI/endpoint that a human can use to confirm "yes, this newly-attached spec matches current behavior" and trigger the re-baseline. Until that wiring exists, a `spec_less` feature that gains a governing doc will keep showing the `r-attach-spec` violation on every subsequent turn but never actually flip to `current`. Task-186 stays `in_progress`, not `done`, and CP-43's `P-3` DoD checkbox is left unchecked with this gap spelled out, matching how Task-185/`P-2` was handled.

## Verification

- `go build ./...` clean; `go vet` clean on `changecontract`, `flowgate`, `runner`.
- `go test ./internal/changecontract/... ./internal/flowgate/...` — 172 passed (48 changecontract tests incl. 23 new for `head.go`/`signature.go`/`backfill.go`/`update.go`/`drift.go`; `flowgate`'s `TestDefaultRules` count updated 11→14, plus 6 new rule tests in `head_rules_test.go`).
- Full `go test ./internal/runner/... -count=1` regression **deliberately deferred** per explicit owner instruction ("tiếp cho all task, khi nào done hết mới test") until Task-187 and Task-188 also land — will run once, covering all three, before this pass closes out.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: Task-186
change_type: feature
summary: flow gate now maintains a per-feature Canonical Head with a reproducible intent signature over governing-spec + behavior (never code), flags spec-drift and code-drift for human reconciliation, and mints a spec_less Head for brand-new features — attach-spec re-baseline detection is wired but the human-approval caller that actually triggers it is not yet built
# --->8---
