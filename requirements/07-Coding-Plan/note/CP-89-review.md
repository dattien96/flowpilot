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

## Post-implementation deep review — 2026-09-28 (capture only; no fixes)

Scope: CP-89 Task-451/452/453 on branch `cp89` at `f9a22d76`; current
`TestTask45[123]` suite passes. Findings below are traced from concrete code
paths, **not** claimed as newly executed live failures. The untracked Task-454
architecture-review document was not changed. Ordered by severity.

### R4-1 — Critical: failed durable flip still spawns flow

`startTurn` sets `flowArm=started` and topology in RAM, snapshots them, then
ignores the result of `persistProviderSession(snap)` at
`interactive_service.go:10645-10688`; it starts `startResolvedFlow` at
`10719-10720` regardless. A failed sessions.ndjson/Supabase upsert leaves the
durable row pending while children execute. Restart can then re-forward the
same flow, orphan the original children, or lose the original launch. The
later non-durable persist at `11023-11028` also ignores errors; neither write
is a durable commit barrier. This directly contradicts the durable-first
ordering recorded in CA-642 and AGENTS §2. Repro test: inject an upsert error
for the `started` snapshot, forward a pending flow, assert typed failure/no
entry child and durable `pending` (current code would spawn). Include a
restart/second-forward assertion; don't rely on an in-memory snapshot.

### R4-2 — Critical: committed topology does not prove a flow was launched

There is a crash window **after** the `started`+topology upsert and **before**
the `go startResolvedFlow` call (`interactive_service.go:10675-10720`). On
reconstruct the heal only returns `started` to `pending` when topology is
absent (`interactive_resume.go:943-956`); with topology present,
`normalizeResumedFlowRun` never launches the entry (`416-444`). No resume path
calls `startResolvedFlow` for this committed-but-unspawned state. An idempotent
prepared-forward replay on `started` completes synthetically without spawn
(`interactive_service.go:10623-10632`). The user gets a permanently started
flow with no child; a bare forward afterwards gets `flow_already_started`.
Repro test: persist a pending run's started+topology pre-spawn snapshot with
no child/session/step-start evidence; restart, then replay the prepared
forward. Assert one entry child is eventually claimed or a truthful
retryable/uncertain outcome (current branch short-circuits). The original
packed entry prompt is not carried in the pre-spawn snapshot either.

### R4-3 — Important: provider switch drops the pinned CP source

`switchChatLeg` creates the new leg with `FlowArm`, `FlowRefFallback`, and
`WorkingMode`, but omits `SourceDocID` (`chat_switch.go:337-363`). `createRun`
initializes `rs.sourceDocID` solely from the new input
(`interactive_handlers.go:1240-1263`). A pending `vibe-cp-ingest` run with a
valid create-time CP source therefore loses that pin on switch. A bare forward
on the new leg falls back to an empty source and returns `invalid_cp_source`
(`interactive_service.go:10145-10153`; `vibe_cp.go:429-443`) instead of
starting the flow. Repro: create pending vibe-cp-ingest with a valid
`SourceDocID`, switch provider/same-provider leg, bare-forward on the active
leg; assert source pin and forward success. Existing L-11 checks only the arm
and flow pin (and is quota-skipped before full forward).

### R4-4 — Important: failed provider turn enters “settled” prompt package

`transcriptTurnsFromRun` flushes a turn on `EventTurnFailed` into the returned
list (`handoff_context.go:169-177,207-211`). `settledChatTurnsForRun` only
removes an **open trailing** turn and system prompts
(`forward_prompt.go:47-80`); it does not remove failed turns. Forwarding after
one failed chat turn passes that failed user's unagreed request (and any
partial assistant output) to the entry child, contrary to Task-453's
“failed turn is excluded” contract. Repro: append TurnStarted + partial
MessageCompleted + TurnFailed followed by a settled chat turn; pack forward,
assert the failed turn's marker is absent. The current only-settled test
covers an open tail, not a failed turn.

### R4-5 — Important: mandatory forward text can exceed the hard budget

`buildForwardPromptPackage` reserves room for `forwardText` when calculating
the transcript allowance but never checks that the forward text itself fits
(`forward_prompt.go:111-140`). `PackPrompt` retains mandatory sections whole
even when their tokens exceed `TotalMaxTokens`, returning a **warning**, not
an error (`promptpacker/packer.go:199-234`). Thus a sufficiently long forward
message produces `pkg.Bytes` / tokens over the entry-node budget while
`forwardFlow` succeeds. Repro: use an entry budget of 100 tokens and a
>100-token forward message, no transcript; assert either a typed rejection
or a package within cap (current code returns an over-budget package). This
is a contract conflict with “forward text intact” requiring an explicit
oversize policy, not silent truncation.

### R4 fix pass — 2026-09-28 (all five findings fixed, red→green)

All findings reproduced with assertion-red tests in
`internal/runner/cp89_review4_test.go`, then fixed:

- **R4-1** — `startTurn` now checks the `persistProviderSession` result of
  the started+topology commit. On failure it rolls the in-memory run back
  to the durable truth (pending latch, cleared topology/vibe markers,
  restored prior turn metadata) and returns typed `persist_failed` 500 —
  no child can spawn off a flip the durable record does not know.
  Test: `TestR4_PersistFailureKeepsPendingNoSpawn` (injects a started-row
  upsert failure; asserts typed error, zero children, durable row still
  pending, retry succeeds after recovery).
- **R4-2** — reconstruct now treats `started`+topology **without a durable
  entry-child row** (parent_run_id + label==node id; child rows are never
  pruned) as the never-launched crash window and heals to pending, so the
  retry forwardFlow launches cleanly instead of wedging on
  `flow_already_started`. Unreadable session index fails closed toward
  keeping started (a false heal would double-launch). Healed rows also
  drop the orphan topology from the pin inference. Tests:
  `TestR4_StartedTopologyNoEntryChildHealsPending`,
  `TestR4_StartedTopologyWithEntryChildStaysStarted` (control),
  `TestR4_RestartedCrashWindowForwardRetriesAndLaunches`.
  `TestTask451_RestartStartedDoesNotReArm`'s fixture gained the entry
  child row — a real started run always has one.
- **R4-3** — `switchChatLeg` now carries `SourceDocID` onto the new leg so
  the create-time CP pin survives a provider switch; the forward-time
  ingest fence reads it without a repaste. Test:
  `TestR4_ProviderSwitchKeepsSourceDocPin` (pending vibe-cp-ingest +
  pinned CP → codex→devin switch → bare forward passes the fence).
- **R4-4** — `transcriptTurnsFromRun` now marks turns closed by
  `EventTurnFailed`; `settledChatTurnsForRun` excludes them, so a failed
  prompt + partial assistant output can never enter the forward package.
  Handoff renderers keep including failed turns (history ≠ settled
  context). Test: `TestR4_FailedTurnExcludedFromForwardPackage`.
- **R4-5** — `buildForwardPromptPackage` rejects a forward text whose own
  token estimate exceeds the entry-node budget with typed 422
  `forward_prompt_too_large` (mandatory sections are never truncated —
  the explicit policy is reject, not silent oversize). Test:
  `TestR4_OversizedForwardTextRejected`.

Verification: `TestR4_*` + `TestTask45[123]` green, `-race` green; the
only sweep failures are the pre-existing missing-binary environment noise
(codex/opencode/agy).

Live re-verification (`LIVE=1`, devin + grok-alt, rebuilt binary,
`c46478de`): **TestCP89Live 11 PASS + 3 named skips, 299s** — including a
new live case **L-14** covering R4-3 end-to-end (pending vibe-cp-ingest +
pinned source → provider switch → leg row carries `source_doc_id`, the
forward passes the fence and is blocked only by the alt provider's
`quota_route_required`). L-14's first run caught a deeper same-class gap:
the createRun durable-row literal never wrote `source_doc_id` — the pin
was RAM-only until the first post-create persist and a restart before any
turn lost it on the ORIGIN leg too (same shape as the earlier FlowArm
literal gap). Fixed by stamping `SourceDocID` onto the create row.
Skips unchanged and justified: L-5 (flow still running → `hub_parked`,
unit-covered), L-11 (latch ride verified; grok quota blocks the forward),
L-12 (go:embed — no runtime file to corrupt).

## Deep review pass 5 (R5-*) — all-findings sweep of e13c5509..aeee1ebc

A full-diff audit pass over every CP-89 commit, tracing durability/recovery,
forward, provider-switch, prompt-pack and Drive-restore paths. All findings
were reproduced with assertion-red tests first (`cp89_review5_test.go`),
fixed, and the suite re-run green.

### Findings + fixes

- **R5-1 — a rejected forward consumed the latch (Critical).**
  `forwardPinnedFlow` flipped `pending→started` before the prompt pack ran,
  so a `forward_prompt_too_large` rejection left the run permanently started
  with no child (every retry → `flow_already_started`). The flip is now
  `commitPendingFlowStartLocked`, invoked only after fences AND the pack
  succeed; rejection paths roll back the turn-metadata adoption and the
  fence-stamped `vibeCpDocID` — a rejected forward leaves the run
  byte-identical. Tests: `TestR5_RejectedForwardKeepsLatchPending`,
  `TestR5_ForwardRetrySameKeyAfterRejectionLaunches`.
- **R5-2 — idempotent replay could ack a launch that never happened
  (Critical).** The replay check runs before the forward seam: a rejected
  forward's non-durable key was already stored (`idem[key]=turnID`), and a
  durable key on a crash row healed back to pending replayed the synthetic
  `TurnCompleted` as launch-ack. Both returned the old turnID with no flow
  ever starting. Fix: `in.ForwardFlow && flowArm==pending` never
  short-circuits — it re-enters the launch path reusing the recorded turnID
  (the latch is the forward's real idempotency record). Tests:
  `TestR5_ForwardRetrySameKeyAfterRejectionLaunches`,
  `TestR5_DurableKeyRetryOnHealedPendingLaunches` (asserts turnID reuse).
- **R5-3 — Drive restore dropped pending-pin fields (Important).** The sync
  manifest carried `flowArm`/`chatFlowRef` but not `sourceDocID`,
  `workingMode`, or `changeType` — a restored pending vibe-cp-ingest leg came
  back as a dev-mode chat with no CP pin and wedged on `invalid_cp_source`.
  All three fields now round-trip the manifest (omitempty; absent == pre-fix).
  Test: `TestR5_DriveManifestRoundTripsPendingPin`.
- **R5-4 — assembled prompt could exceed the hard cap (Important).**
  `TotalMaxTokens` bounds section content only; headers/joiners are added at
  assembly and a mandatory forward section is never truncated — an in-content-
  budget forward could assemble over the cap (measured 210 vs 200 in the red
  test). `buildForwardPromptPackage` now post-checks the ASSEMBLED prompt
  against the budget and rejects over-cap with `forward_prompt_too_large`.
  Test: `TestR5_ForwardPackageAssembledWithinBudget` (asserts both the
  in-cap invariant and the over-cap rejection).
- **R5-5 — child rows were the only launch evidence (Important).**
  `flowEntryChildExists` also consults the durable step-transition log and
  step rows moved past PENDING: an entry RUNNING line or a node step with a
  `StartedAt` stamp means the executor actually reached the node (inline-entry
  chains write the entry DONE before their delegate spawn), so the run is
  mid-flight flow work owned by resume machinery — not a never-launched crash
  row. Transition-log unreadable fails closed to started (same as the
  unreadable-index rule). Reseeded PENDING rows are deliberately not evidence:
  reseed runs at executor start AND at resume. Test:
  `TestR5_StepTransitionEvidenceKeepsStarted`. Residual: a crash mid-Dispatch
  before ANY durable evidence is written is safely-retryable — inline
  behaviors are deterministic context producers and re-dispatch converges.

Verification: `TestR5_*` (6) + `TestR4_*` + `TestTask45[123]` green,
`-race` green on the CP-89 set. `TestTask450_*` quota tests flake under the
wider -race glob (timing-sensitive, pass standalone, untouched code).
