# CP-89 Review — Flow launch: `immediate` vs `chat_then_forward`

- Reviewer: Devin (live-review pass)
- Date: 2026-09-28
- Source reviewed: `requirements/07-Coding-Plan/note/CP-89-note.md` (draft)
- Code base: `apps/local-runner/internal/runner/` @ branch `flowpilot`

## Verdict

The design is **coherent and the right shape** — an explicit `forwardFlow` turn
flag + a durable `flowArm` latch is the honest way to add "pick flow, chat
first, forward when ready" without teaching the model to infer consent. All
seven of the note's claims about today's behavior verified against the code.
The gaps found are boundary conditions at the pin/start seam — listed below
for whoever implements it. No implementation is implied by this review.

## Verified claims (checked against code)

| Note claim | Code ground truth |
|---|---|
| §2: `turnCount == 0` + `flowRef` → `flowStartOnly = true`, `startResolvedFlow` async, hub suppressed | `interactive_service.go:10496-10514` sets the flags; `10753-10770` emits a synthetic `turn_completed` and clears `turnInFlight` without calling the provider |
| §2: `vibeAwaitingLock` / `vibeSprintBudget` / `vibeLockedCP` arm at start | `interactive_service.go:10506-10513` |
| §2: `vibe-cp-ingest` without a CP source → 422 `invalid_cp_source` | `validateVibeCpIngestSource` (`vibe_cp.go:429-463`), invoked at turn admission under the `turnCount==0` gate (`interactive_service.go:10213-10219`) |
| §4: the start door is `turnCount == 0` and closes permanently | `resolveWorkflowFlowRef` bails on `turnCount != 0` (`flow_executor.go:354-358`); `startTurn` gates the whole flow-start block on `rs.turnCount == 0` (10456) |
| §4: Drive-restore never re-starts (BUG-315) | `restoredFrom` guards at `interactive_service.go:10496` and `flow_executor.go:365-370` |
| §3.2: working-mode fence exists | `FlowAllowedForWorkingMode` at turn admission (`interactive_handlers.go:556-570`) and at run-create |
| §3.2: pinned ref lives on the run | `rs.chatFlowRef` stamped at create (`interactive_handlers.go:1241-1247`) and on the starting turn (`interactive_service.go:10504`); persisted via `ChatFlowRef` in the session row (`interactive_service.go:5017`) |

## Findings for the implementer

### F-1 (contract gap): bare `forwardFlow` turns will 400 today
`handleStartTurn` rejects bodies with no actionable content
(`interactive_handlers.go:504-509` — BUG-509's guard): prompt, attachments,
flowRef, subMode, sourceDocID, changeType are the only fields that count. The
note allows Forward with no message ("câu user gửi kèm nút Forward, **nếu
có**"). `forwardFlow` must be added to that content check or a bare Forward
button dies at admission. **The note should list this explicitly** — it's the
kind of line that gets missed.

### F-2 (state-machine gap): the pin lives on the *turn field*, not the run, in two places
The start gate checks `in.FlowRef` (the turn input), and the cp-source fence
checks `in.FlowRef` too. Under `chat_then_forward` the pinned ref sits on
`rs.chatFlowRef` and the forward turn carries no `flowRef` at all — so both
gates must read the pinned ref when `forwardFlow` is set. `resolveWorkflowFlowRef`
also bails on `turnCount != 0`, so workflowID-mounted pending runs need a
**second resolution seam** at forward time rather than a relaxation of the
`turnCount==0` guard (that guard carries BUG-315/BUG-261 protections — do not
soften it).

### F-3 (durability detail): `vibeAwaitingLock` arms too early for `pending`
`createRun` arms `vibeAwaitingLock` + `vibeSprintBudget` the moment `FlowRef`
is present (`interactive_handlers.go:1244-1247`) — at *pin* time, not
first-turn time. §3.2 requires these markers stay off during the chat phase.
Implementation must split "pin" (`chatFlowRef` stamp) from "start markers"
(vibe flags) behind the `flowArm` value. Not currently separable — the two are
fused in one branch.

### F-4 (input semantics): CP source scoping on the forward turn is under-specified
`validateVibeCpIngestSource` resolves the source as: explicit `SourceDocID` →
then CP-shaped tokens **in that turn's prompt** (`vibe_cp.go:430-439`). §3.2(1)
says "turn forward không có source thì 422" — but if the user named the CP in
an earlier *chat* turn and forwards with just "ok chốt", the forward turn has
no source → 422. Decide whether the launch arm's pinned `SourceDocID` (the
comment at vibe_cp.go:425 says the launch arm's `@path` is already the intended
first source) carries into forward-time validation, and say so in the note.
Otherwise the feature teaches users to re-paste the path on Forward.

### F-5 (missing transitions): forward with no pinned flow / corrupted pin
The note doesn't define:
- `forwardFlow: true` on a run with no `chatFlowRef`/`workflowID` → should be
  a typed error (400/422), not a silent chat turn.
- The pinned definition corrupted between pin and forward → must fail-closed
  `invalid_flow_definition` (mirror `pendingFlowRefInvalidErr` at
  `interactive_handlers.go:531-538`), not degrade to chat — the user asked
  for a flow, silently chatting instead is the BUG-261 shape in reverse.

### F-6 (prompt contract): "settled transcript" needs a bound
§3.2(2) folds the whole chat transcript into the entry prompt. Unspecified:
size cap, ordering, which roles/turns are included, and how it interacts with
`promptpacker`. Raw concat of N chat turns can blow the child prompt budget;
the entry prompt should ride the existing packing path or the note should
declare the budget explicitly.

### F-7 (storage): `flowArm` homes not yet named
`pending` must survive restart + provider switch (§4). Concretely it needs:
(a) a field on the durable session row (`sessions.ndjson`), (b) carriage in
the Drive-restore manifest — the BUG-315 `TurnCount` precedent — and (c)
reconstruct honoring `pending` (no `startResolvedFlow`) and `started` (no
re-arm). The note says all this behaviorally; naming the three homes would
make it implementable on first read. Also worth stating: `flowArm` belongs to
the **chat/run**, not the leg — a provider switch mid-chat (new leg) must
keep `pending` (per the CP-59 pin semantics the note already cites).

### F-8 (minor): test list additions
§7's seven tests are the right core. Three worth adding:
- Bare `{"forwardFlow": true}` with no prompt (F-1) — assert it forwards.
- Provider switch during `pending`, then Forward (F-7) — pin rides the chat.
- Forward after the pinned flow's definition was deleted/corrupted (F-5) —
  fail-closed, not chat.

## Non-issues (checked, consistent)

- **Hub suppression mechanics** — `flowStartOnly`'s synthetic `turn_completed`
  works on any turn index; nothing about it is turnCount-locked. Reuse on the
  forward turn is safe.
- **`flowEngineDriven` set inline under `s.mu`** — the note's design doesn't
  disturb the deadlock-avoidance rationale in the comment at 10469-10484.
- **Double-forward** — `started` latch + "turn after forward is a normal flow
  turn" is consistent with `turnCount==0` being gone by then; the latch just
  needs to be checked before `in.FlowRef` acceptance on later turns.
- **`restoredFrom` + `pending`** — a restored pending run should reconstruct
  as pending, not started; the note implies it and the existing guard shape
  supports it.

## Bottom line

Approve-in-shape. The design respects the repo's invariants (fail-closed
fences, durable latch, no model-inferred consent, default preserved). The
work is concentrated in four seams: `handleStartTurn` admission (F-1),
`startTurn`'s `turnCount==0` block (F-2, F-3), `flowArm` durability (F-7),
and the forward-time fence set (F-4, F-5). None require new machinery — all
are extensions of existing gates, which is what the contract asks for.
