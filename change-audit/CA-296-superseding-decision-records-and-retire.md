# CA-296 — Superseding Decision Records And Retire (Task-187, CP-43 P-4)

## Scope

Implemented (partially — see caveat) [Task-187](../requirements/08-Task/todo/Task-187-Superseding-Decision-Records-And-Retire.md) (P-4 of [CP-43](../requirements/07-Coding-Plan/todo/CP-43-Change-Contract-And-Canonical-Intent-Signature.md)): fold negative knowledge (rejected/reverted approaches) into each [Canonical Head](CA-295-canonical-head-and-intent-signature.md) (Task-186), and an end-of-life `RetireHead` that preserves that knowledge across rename/merge.

## Changes

- New `apps/local-runner/internal/changecontract/decisions.go`: `FoldDecisions(featureKey, chatLedger, ledger)` — extracts rejected approaches from `changeledger.ChatSummaryLedger` bullets and reverted approaches from `bugfix`-type `changeledger.Ledger` entries. Both sources are matched via plain keyword lists (`rejectionMarkers`, `revertMarkers`) since neither has a structured `tried/outcome/reason` field — see caveat below. Reuses the pre-existing `Decision` struct from Task-186's `head.go`.
- New `retire.go`: `RetireHead(h, action, targets)` — pure function (no I/O, mirrors `UpdateHead`/`RebaselineWithSpec`'s shape) setting `status`/`superseded_by`/`retired_at` on `h`; for `renamed`/`merged` (not `deprecated`), copies `h.Decisions` into each target Head and recomputes the target's signature. Callers are responsible for `LoadHead`/`SaveHead` I/O and for `SaveHead`-ing the retired Head itself (never deleted, kept for provenance).
- `flowgate/rules.go`: new default rule `r-retire` (`feature_rename_merge_or_deprecate`, `approve`) + `TurnResult.HeadRetirePending` field, same signal-passed-in pattern as Task-186's Head rules.
- `flowgate/evaluate.go`: matching `checkRule` case.
- `runner/gate_hook.go`: `updateCanonicalHead`'s existing gate-passing fold path now also calls `FoldDecisions` (via the new `foldCanonicalHeadDecisions` helper, which loads both ledgers) and, only on success, overwrites the Head's `Decisions` and recomputes its signature — a load/fold failure leaves the Head's prior `Decisions` untouched rather than silently clobbering them with an empty set (`SS-14 AC-9`).

## Caveat — not fully done

Two gaps, left explicit rather than glossed over:

1. **Heuristic extraction, not structured data.** `ChatSummaryEntry.Summary` is a free-text bullet list (the AI/heuristic chat summarizer's own output) and `changeledger.Entry` has no `revert` `ChangeType` — neither source carries a structured `tried/outcome/reason` field. `FoldDecisions` therefore does deterministic *keyword matching* against existing free text (mirroring `runner/chat_summary.go`'s own `summarizeSentences` heuristic-classification idiom), not true structured extraction. The task doc's "only decision text is AI-generated; ordering/linkage are deterministic" requirement is satisfied in that no second AI pass decides inclusion — but a differently-worded rejection (e.g. "we backed off approach X" with no keyword match) will be silently missed.
2. **`r-retire` cannot fire in practice.** Unlike Task-186's `r-attach-spec` (whose *detection* signal is computed automatically in `gate_hook.go`, even though the approval-confirmation caller is still missing), **no code anywhere sets `HeadRetirePending` to true**. `SD-21`'s own Open Question — is a retire intent detected from a `FEATURE-KEYS.md` diff, or only from an explicit user/admin action? — is unresolved, so this task built the rule/signal/`RetireHead` mechanics but not a trigger. `r-retire` is reachable today only by a test, or a future caller (e.g. Task-188's Admin Web) setting the signal by hand.

Task-187 stays `in_progress`, not `done`; CP-43's `P-4` DoD checkbox is left unchecked with both gaps spelled out.

## Verification

- `go build ./...` clean; `go vet` clean on `changecontract`, `flowgate`, `runner`.
- `go test ./internal/changecontract/... ./internal/flowgate/...` — 183 passed (9 new `decisions_test.go`/`retire_test.go` cases in `changecontract`; `flowgate`'s `TestDefaultRules` count updated 14→15, plus 2 new `r-retire` rule tests).
- Full `go test ./internal/runner/... -count=1` regression **deliberately deferred** per explicit owner instruction ("tiếp cho all task, khi nào done hết mới test") until Task-188 also lands — will run once, covering Task-186/187/188 together, before this pass closes out.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: Task-187
change_type: feature
summary: canonical heads now fold rejected/reverted approaches (from chat-summary bullets and revert-type bugfix ledger entries, keyword-matched) into a Decisions list on every gate-passing turn, and RetireHead preserves that knowledge across rename/merge — the r-retire rule and RetireHead mechanics exist but nothing yet detects a retire intent to actually trigger them
# --->8---
