# CA-1146 — BUG-614 + BUG-615 + vibe cap contract (20 rounds/task)

Live trigger: CP-03 watch on run-150388 / chat cht_ce24f4dd4cbc (PrivateVault,
Task-032 sprint). User report: reopening the chat hid the Flow Mode sidebar,
and subagent/debate prose rendered as ordinary chat rows in the main UI.

## BUG-614 — flowArm:"immediate" runs reopen as normal_chat

- **Bug:** `openHistoryRun` classified a reopened run as workflow/flow mode
  only when `flowArm === "started"` (BUG-575). Launch-time armed runs —
  vibe-CP ingest parents and spawned flow legs — report
  `flowArm: "immediate"` with `runKind: "chat"`, so they reopened into
  `normal_chat` and the Flow Timeline sidebar stayed hidden even though the
  flow was live.
- **Fix:** new `flowArmRanFlow(arm)` helper accepts `"started" | "immediate"`
  and is used at every restore site: `openHistoryRun` mode classification,
  the `flowStarted` latch, and the provider-switch reattach path.
- **Tests:** `store.bug575-history-flow-mode.test.ts` extended — immediate
  arm restores `workflow_step_auto` + `flowStarted` (5/5 pass).

## BUG-615 — engine-internal turns leak prose into the main chat

- **Bug:** flow-engine turns (hub reinvokes, joined-result notes, debate /
  synthesis instructions, reprompts) reuse the normal provider-turn event
  vocabulary with no internal marker. Their `message_completed` /
  `tool_*` events were captured into `workflow_chat_events` and rendered as
  prompt/assistant/tool bubbles in the human chat.
- **Fix:** turn-scoped `Internal` flag, stamped once at dispatch and
  propagated on every prose event of the turn — durable events keep
  everything, the chat read model and the desktop reducer hide only prose.
  - `ProviderEvent.Internal` (omitempty) + `TurnInput.Internal` (`json:"-"`,
    never client-settable).
  - Classification at the turn_started emit site:
    `in.Internal || isFlowEnginePrompt(in.Prompt)` — evaluated before the
    display prompt is redacted, so gate reprompts and handoff seeds stay
    non-internal (their replies are user-visible output). Engine dispatch
    paths mark `Internal` via `engineTurnOnFlowHub(runID)`: true only for
    the flow hub itself (`parentRunID == "" && flowEngineDriven`) — child
    legs keep their work visible in their own chats.
  - `emitLocked` stamps `Internal` on prose events (`internalTurnProseEvent`:
    turn_started, message delta/completed, turn completed/failed, tool
    started/completed) while `currentTurnInternal` holds; cleared at the
    turn's terminal event. Approvals, questions, decision cards, file
    changes and token usage are never stamped — actionable rows surface.
  - `chatRecordsFromProviderEvent` skips internal prose/tool records at
    capture; the turn_started boundary stays (it is the read-side marker)
    and omits the `prompt` key for non-internal redacted turns so
    `suppressInternalTurnRecords` fails open on their replies.
  - `suppressInternalTurnRecords` on the four record readers
    (chat_timeline, run_timeline, chat_switch handoff context,
    forward_prompt context): per-leg internal window, explicit
    `internal:true` boundary opens, `prompt == ""` opens for pre-fix
    records, absent `prompt` fails open. Applied before
    `collapseRepeatedFinals`.
  - `userFacingTranscriptEvents` propagates the same window through replay
    reconstruction (`e.Internal || isFlowEnginePrompt`).
  - Desktop: `ProviderEventBaseDTO.internal?: boolean`; `timelineReducer`
    skips prompt/assistant/tool row pushes for stamped events while still
    finalizing streaming state and `turn_failed` recoverability.
- **Tests:** `bug615_internal_turn_suppression_test.go` (window semantics,
  explicit flag, fail-open pre-boundary and absent-prompt, capture-side
  skips, replay stamping) + `bug615-internal-turn.test.ts` (reducer drops
  internal rows, approvals inside internal turns still surface, next user
  turn renders).

## Vibe cap contract — 20 rounds per Task

- **Contract (user):** default max cap is 20 rounds per Task; a CP's total
  cap is `len(taskPlan) × 20` (CP-03's 10 tasks → 200).
- **Before:** `vibe-sprint.yaml` `policy.cap: 3` gated each sprint's review
  loop after 3 rounds (live CP-03 parked on `blocked | cap` repeatedly and
  needed manual extend-cap to converge). `vibeSprintBudget` was a fixed 8
  sprint slots, so a 10-task CP would silently stop after sprint 8.
- **Fix:**
  - `vibe-sprint.yaml` `policy.cap: 3 → 20` (extend ladder kept).
  - `defaultVibeTaskRoundCap = 20` + `vibeSprintBudgetForPlan(tasks)` =
    `max(defaultVibeSprintBudget, tasks × 20)`; applied when the task plan
    is (re)loaded — only ever raises, never shrinks an operator-extended
    budget — and as the plan-aware `budget <= 0` fallback in
    `decideNextVibeSprint` and the three boundary readers.
- **Tests:** `vibe_sprint_budget_scale_test.go` — 10-task plan ⇒ 200, all
  10 sprints trigger, empty plan keeps the floor.

## Follow-up — internal turns are hub-only (BUG-615 regression)

- **Regression (user-reported, CP-02 was clean):** `isFlowEnginePrompt`
  matches the `[FlowPilot sub-agent — …]` spawn envelope, so the classifier
  at `startTurn` marked every CHILD leg's first turn internal → the child's
  whole reply dropped from its own chat/transcript. Sub-agent responses
  stopped rendering.
- **Fix — scope classification to the flow hub:**
  - `rsIsFlowHub(rs)` = root run + `flowEngineDriven`; `turnIsInternal` =
    `rsIsFlowHub && (in.Internal || isFlowEnginePrompt)` at the emit site.
  - `engineTurnOnFlowHub` (dispatch marking) already hub-scoped — unchanged.
  - Replay: `userFacingTranscriptEvents(historical, hubRun)` gates BOTH the
    persisted `e.Internal` flag and the prompt marker on `hubRun` — pre-fix
    runs that mis-stamped child events heal on resume/reopen.
  - Read-side `suppressInternalTurnRecords(recs, isHubLeg)` gates the window
    on `s.legIsFlowHubFunc()` — memoized predicate over resident fields, or
    the durable row (`ParentRunID==""` + `AutoOrchestrate`/`ActiveFlowNodes`,
    the same fields `reconstructRun` re-engages `flowEngineDriven` from).
    Unknown legs fail open (records kept). Both the `internal` flag and the
    legacy empty-prompt marker require a hub leg, so pre-fix child-leg
    records reappear.
- **Tests (additive):** `TestTurnIsInternal_HubOnly`,
  `TestUserFacingTranscriptEvents_ChildLegTurnsStayVisible`,
  `TestSuppressInternalTurnRecords_ChildLegLegacyRecordsKept`. Existing
  replay tests updated to pass the hub context they simulate.
- **detect_changes:** risk HIGH — blast radius on timeline/read seams only;
  changes are additive predicates, no state-shape or wire-schema change.
