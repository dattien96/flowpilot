# CA-805 — 4-agent debate fixes: deadlock, heal order, resume node render

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: CP-60
change_type: bugfix
summary: Fix nested-mutex deadlock in terminal checks, heal Cancelled only on claimed advance, carry resume node to TUI and dismiss stale vibe gate
# --->8---

## Why

4 independent reviewers (StateMachine correct with 2 findings, RetryReopen pass, TuiSnapshot 2 findings, TestsGaps 6 test gaps) on CA-801→804 chain for live run-220036.

## Change

- `flowRunTerminalLocked` / `flowInlineContext`: inline leftover-paused check without re-entering `s.mu` (was self-deadlock on Completed + blocked:paused).
- `maybeAdvancePendingValidateAfterCoder`: heal Cancelled→Running only after all refuse-checks pass, just before `tryAdvance`.
- `pendingGateView` / client `GateInfo` / TUI `GateState`: new `ResumeFrom`; snapshot fills from `vibeResumeFromNode`; TUI renders `[GATE] Resume from {node}?`; stale ok/cancel gate dismisses when snapshot carries none (r-reg gates untouched).

## Tests

- New `ca805_debate_heal_order_resume_test.go` (deadlock with 5s timeout, no-heal-on-refuse, snapshot ResumeFrom).
- New `ca805_resume_gate_render_test.go` (render node, stale dismiss, non-vibe kept).
- Old CA-801/802/803/804 + BUG-288 Cancelled-always-terminal untouched.

## Providers

Agnostic Case 1: loop/status only, no providerKey branching.

## Will not undo

BUG-234 (non-pause blocked never advances), BUG-288 P1-12 (Failed + Cancelled/stopped terminal), CA-801→804 park/heal/Retry chain.
