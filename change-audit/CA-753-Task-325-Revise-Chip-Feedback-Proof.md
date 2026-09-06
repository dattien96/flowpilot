# CA-753 — Task-325 follow-ups: feedback proven live + [Revise] discoverability chip

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: Task-325
change_type: feature
summary: live proof that parked-plan feedback reaches the writer prompt verbatim; blocked bar gains a Revise chip prefilling /continue so the affordance is discoverable
# --->8---

## Live proof (run-594636, 2026-09-06)

The operator suspected `/continue <text>` was swallowed into a blind retry. Disproven via the writer child's run timeline: its `turn_started` prompt is the user's verbatim note plus the composed template —

`switch the tie rule to (a, nil) mirroring MaxChecked; add MinInt/MaxInt boundary cases to the matrix\n\n---\n\n[flow-engine] The plan was approved pending your revisions below. Revise the plan documen…`

Same-child re-entry (no second writer), loop back to running. The feedback path is live-verified end-to-end, not just unit-routed.

## Change (TUI-only)

- Blocked bar + action ring + mouse hit-test gain `[Revise]` (appended last — Retry/Stop/Allow indices stable): activating it prefills the composer with `/continue `, moves the caret to end, clears selection, and sends nothing. The loop stays blocked until the user hits Enter.
- Copy on the chip is self-documenting (`revise with a feedback note`, fills `/continue`).
- Tests (new `blocked_revise_chip_test.go`): bar renders chip + affordance (3-provider matrix), hidden when not blocked, click/keyboard prefills without cmd, ring order stable.

## Verification

- 6 new tests PASS (chip render/matrix, hidden when unblocked, prefill without cmd, ring order + Tab highlight at idx 2/3 with/without Allow, parked plain-text feedback incl. transcript echo); full `tui/...` green (old blocked-bar suites use Contains-style assertions — unbroken); zero old-test edits; gofmt clean on new lines.
- Follow-up fix 2026-09-06 (same commit line): Revise highlight index is dynamic (2, or 3 with Allow) — hardcoded 3 left Revise unhighlightable without drift, so Tab appeared to die on [Stop] (live-found run-584646).
- Follow-up 2026-09-06 (live request run-577686): plain text typed while parked IS the feedback — no `/continue` prefix needed (a chat turn would just 409 server-side). Footer hint updated (`type note + Enter to revise`).
