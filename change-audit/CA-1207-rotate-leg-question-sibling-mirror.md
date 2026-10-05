# CA-1207 — rotate_leg question invisible on the leg the operator watches

**Kind:** live defect (BUG-1186 residual)
**Evidence:** `.flowpilot/vibe-watch/PENDING-BUGS.ndjson` entry
`rotate_leg_creates_new_run_record_hiding_questions` — live
run-204891 → run-225672. A `rotate_leg` answer minted a NEW root run for the
same chat; the hub's `ask_user` q-226099 registered on run-225672 while the
operator watched run-204891's detail view — which stayed `running` with no
card for ~25min. Only the mux lane surfaced the stall.

## Root cause

`turnBridge.askQuestion` mirrors `EventUserQuestionRequired` onto the flow
root via `flowRootIDLocked` (CA-642). That walk follows `parentRunID`
chains — a rotated leg is a new ROOT run (`parentRunID=""`), so the mirror
set never includes the sibling legs of the same chat. The superseded leg's
event stream and status stayed untouched.

## Fix

- `askQuestion`: after the CA-642 flow-root mirror, emit the same mirrored
  `EventUserQuestionRequired` onto every non-terminal sibling leg of the
  owner's `chatID` (`activeChatLegsLocked`, skipping the owner, the already-
  mirrored root, and terminal legs). `emitLocked` flips the sibling to
  `waiting_question` and marks its mux lane dirty; the card renders on
  whichever leg the client streams. `pendingQuestionID` is stamped only on
  the owner — answering resolves globally by question id.
- `healMirroredQuestionWaitLocked`: heals ALL mirror targets — the flow
  root AND sibling chat legs — restoring `running` (or `waiting_approval`
  when an approval still pends) only when no other pending question's owner
  still mirrors onto that target.

## Validation

- `TestCA1207_RotatedLegQuestionMirrorsToSiblingLeg` — red before fix
  (mirror never reached the sibling); green after: sibling stream carries
  the card, status flips to `waiting_question`, answer unblocks the bridge,
  and the mirrored wait heals back to `running`.
- `TestCA1207_TerminalSiblingLegReceivesNoMirror` — terminal legs get no
  mirror and their status is never resurrected.
- Regression: `Question|Heal|Mirror|Leg|CA642|Bug470|AskUser|Approval`
  family green (one unrelated flake passed on isolated rerun).
