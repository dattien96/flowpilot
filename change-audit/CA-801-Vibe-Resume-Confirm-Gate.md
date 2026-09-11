# CA-801 — reopen paused vibe sprint asks OK/Cancel, no auto coder

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: CP-60
change_type: bugfix
summary: Reconstruct parks a pause gate (OK/Cancel) instead of auto-spawning coder
# --->8---

## Why

Operator: do not auto-resume from tdd on `/open`. Show a gate-like card: run paused, continue? Only OK and Cancel.

## Change

- Reconstruct **and in-memory `resumeRun`** call `maybeParkVibeResumeConfirm` synchronously so `/open` GetRun sees `pendingGate` (live miss: same-process reopen never reconstructed).
- Park `BlockReason=paused`; snapshot `pendingGate` options `ok`/`cancel`. Sprint graph parks even if `workingMode` empty or loop was stopped. Completed disk coder rows do not suppress the gate.
- `SubmitGateDecision` ok → unblock + `maybeResumeVibeCoderAfterTdd`; cancel → no spawn.
- TUI: `[GATE] Run paused` + `[OK]` `[Cancel]`; `/open` hydrates from snapshot and retries GetRun if the first snapshot had no gate.

## Tests

New `ca801_vibe_resume_confirm_gate_test.go` (Claude/Codex/Grok reconstruct no auto-coder; cancel; ok clears flag; r-reg keep-test still accepted when confirm is false). New `ca801_paused_resume_gate_test.go` (copy + snapshot hydrate). Old CA-798 maybeResume tests untouched (still call resume directly).


## Providers

Agnostic Case 1: park/confirm/submit take no providerKey. Reconstruct tests table Claude/Codex/Grok.

## Will not undo

CA-798 resume logic (OK still uses it). CA-799 step restore + flowRef infer. CA-800 bare statusline.
