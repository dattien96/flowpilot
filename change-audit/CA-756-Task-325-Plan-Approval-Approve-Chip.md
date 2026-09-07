# CA-756 — Task-325 live follow-up: plan_approval [Retry] → [Approve] + decision highlight

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: Task-325
change_type: feature
summary: plan_approval park renders [Approve] (same retry target) plus a failed-review decision line; blocked-chip click routing wins over the approval generic-text fallback
# --->8---

## Live proof (run-206538, 2026-09-07, TUI Task Harness on gate-sandbox)

- Prompt `Change Divide(a,b) in calc.go to panic on zero divisor` forced a CA-contradiction bounce (Task-905 → changes_requested), writer revised (Task-906 → Task-907 per operator note), and every churned approval parked before freeze — gate holds.
- UX trap found live: the operator typed `approve Task-907, forward preflight_contract_freeze` as a message. `resumePlanApproval` classifies any non-empty, non-`continue` text as feedback, so the "approve" became writer round 3 instead of an approval. Bare `/continue` / `[Retry]` was the only exit, but `[Retry]` reads as "run again with old scope" while it really approves the plan and forwards freeze.
- `[Retry]` click approved correctly (freeze → test_signatures → implement), closing the loop.

## Change (TUI-only, label + copy + click tiebreak; no dispatch change)

- `tui/app/step_runtime.go` `renderBlockedBar`: on `plan_approval` the first chip renders `[Approve] - approve plan, forward freeze` (same `retry` target) and a highlight line `decision: plan failed review — [Revise] to rewrite, [Approve] to freeze and continue`. All other parks keep `[Retry]` byte-for-byte.
- `tui/app/action_ring.go` `actionRingItems`: same label swap on the ring item (target stays `retry`; indices unchanged).
- `tui/app/mouse.go`: `hitBlockedChrome` accepts `[Approve]` → `retry`; `clickTargetAt` consults the blocked bar before the approval generic-text fallback (`hitApprovalChrome` claims any `Approve` substring unconditionally and runs first — it swallowed the new chip as `approve` in the first click test). The reorder only diverts cells where a blocked chip positively matches; every other park's tokens never match the approval fallback, so they are unaffected.
- Tests: 1 old file edited **with operator approval** (`blocked_revise_chip_test.go`: plan_approval model expects `[Approve]`, asserts no `[Retry]`); new `plan_approval_approve_chip_test.go` (render/matrix, other-parks-keep-Retry, ring-target-stable, click-approve-unparks — all claude/codex/grok).

## Prior CA intact

- CA-751: bare `/continue` and `[Retry]`/`[Approve]` clicks still POST legacy `feedback:"continue"`; `/continue <text>` still rides as writer feedback.
- CA-753: `[Revise]` affordance, ring order `[Retry]/[Approve] [Stop] (Allow) [Revise]`, parked plain-text submit, Tab-trap fix — all untouched and green.
- CA-754/755 (runner BUG-360/CP-58): untouched.

## Verification

- 4 new tests PASS incl. 3-provider subtests; related old suites (`TestBlockedBar_*`, `TestReviseChip_*`, `TestContinueWithText*`, missing-CA copy, drift/cap chips) green.
- Full `tui/app` suite: 11 failures (YouBox ×6, spinner ×2, bottom-notice ×2, drive-indicator ×1) proven pre-existing — identical set on the clean tree via `git stash` (`/tmp/clean.txt` vs `/tmp/run1.txt`, byte-identical). No regression from this change.
- Provider parity: changed code paths are provider-agnostic (label/copy depend only on `flowBlockReason`); render + click tests matrix claude/codex/grok.
