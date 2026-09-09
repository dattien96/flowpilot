# CA-816 — sealed-loop OK resumes only with a resume target

# ---8<--- flowpilot:change-ledger
feature_key: vibe-mode
source_doc_id: CP-60
change_type: bugfix
summary: SubmitGateDecision OK on a sealed loop resumes only when the gate carries vibeResumeFromNode; target-less OK stays Stop-wins and keeps the stop fence
# --->8---

## Why

CA-812 deleted the `loopSealedForReinvoke` early-return in
`SubmitGateDecision` so operator OK releases the hub stop fence. That broke
CA-803 at HEAD: `TestCA803_StoppedOKDoesNotHeal` went red
(`Stop must win: status="running"`). The two tests describe different gate
shapes sharing one code path:

- CA-803: `Cancelled + vibeResumeConfirm + loop stopped + from == ""` —
  stale/poison gate, Stop must win.
- CA-812: `Cancelled + vibeResumeConfirm + from == "validate" + unfinished
  successor` — genuine resume gate, OK is explicit resume intent.

CA-812's own test re-blocks the loop before OK, so the deleted seal check
was never load-bearing for it — only for the live post-Stop /open shape
where the loop is still stopped at OK time.

## Change

- `gate_hook.go` OK path: restore the seal guard narrowed by the
  discriminator — `loopSealedForReinvoke(runID) && from == ""` returns
  early (no heal, no unseal, no fence release). `from != ""` proceeds to
  the CA-812 behavior (heal Cancelled, release fence, set running,
  advance from node).
- `change-audit/CA-807-Chat-Gate-Init-Loading.md`: ledger `feature_key`
  `tui-chat` → `cli-tui` (`tui-chat` was never registered in
  `FEATURE-KEYS.md`; the commit itself used `[cli-tui]`).

## Tests

- `ca816_sealed_resume_target_test.go` (new, additive-only):
  - `SealedLoopWithResumeTargetOKResumes` — stopped loop + `from=validate`
    + validate DONE: OK heals to Running, unseals loop, releases fence,
    dispatches synthesis with no stop-fence TurnFailed.
  - `SealedLoopWithoutTargetOKKeepsFence` — stopped loop + `from=""` on a
    real post-Stop fence: OK keeps Cancelled/stopped AND keeps the fence.
- Existing guards green unchanged: `TestCA803_StoppedOKDoesNotHeal`,
  `TestCA812_ResumeOKReleasesStopFence`.

## Providers

Agnostic Case 1: the branch keys off loop status + `vibeResumeFromNode`
only; no `providerKey` branching. Tests use `ProviderKeyCodex` via the
shared `setupPostStopHub` helper like CA-812.

## Will not undo

CA-803 Stop-wins (poison gate). CA-812 fence release on genuine resume
(including the blocked/paused-loop shape its test pins). CA-801→806
pause-gate chain. BUG-308 generation stays elevated (fence release only
clears `Stopped`, never resets generation).
