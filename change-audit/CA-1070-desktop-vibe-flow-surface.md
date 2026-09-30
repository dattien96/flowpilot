# CA-1070 — desktop: vibe mode is flow-only; flow picker follows working mode

## What changed

Vibe mode was wired as a chat-controller toggle and the Flow tab's workflow
select listed every catalog row, so the surface broke the working-mode family
gate the runner already enforces (`workingmode.FlowAllowedForWorkingMode`,
kind=user):

- Normal mode → Flow tab listed `vibe-*` builtin mirrors (selecting one is
  rejected server-side anyway — dead option).
- Vibe mode → the same list showed dev harness/user flows (also forbidden).
- The Normal/Vibe toggle lived in the Chat controller, which hides when the
  user switches to the Flow tab — no way back without re-entering Chat.

Desktop changes:

- `src/state/workingMode.ts`: added the gate's client mirror —
  `bareFlowId` (pack-prefix strip), `userFlowSelectableForMode(mode, ref)`
  (vibe → only `vibe-ingest`/`vibe-cp-ingest`; dev → harness five + untracked
  catalog ids; hidden mirrors like `review-loop` are never startable in
  either mode), and `filterWorkflowsForWorkingMode` which keys a row off
  `packFlowId ?? id`.
- `src/types/contract.ts` + `src/app/navigatorCatalog.ts`: `Workflow.packFlowId`
  now carries through `mapNavigatorWorkflow` — without it builtin mirrors are
  indistinguishable from user workflows and no family filter is possible.
- `src/state/store.ts`:
  - `setWorkingMode("vibe")` while on the Chat tab now coerces
    `chatMode → workflow_step_auto` through the normal `setChatMode` path
    (persist + resetRun — identical to clicking the tab);
  - `setChatMode("normal_chat")` is refused while vibe is on — the Chat tab
    cannot be re-entered by any path;
  - a `selectedWorkflowId` that the new mode cannot start is cleared on the
    flip so the select can't carry a forbidden pick;
  - module init and the `fp:lastChatMode` boot restore coerce
    `normal_chat → workflow_step_auto` when the persisted working mode is
    vibe.
- `src/components/ChatWorkspace.tsx` (`WorkflowControlPanel`): the toggle is
  now a highlighted `working-mode-toggle` button at the top of the right rail
  section (always mounted, both surfaces); the Chat tab renders `disabled`
  with an explanatory title while vibe is on; the workflow select filters via
  `filterWorkflowsForWorkingMode`. The toggle is locked while a run/scaffold
  turn is live — same guard the old controller toggle had (`blocked`).
- `src/components/ChatInput.tsx`: the old chat-controller Mode switch is
  removed (the controller hides in flow mode; the toggle moved to the rail).
- `styles.css`: `.working-mode-toggle` — subtle in Normal, flow-surface
  gradient highlight in Vibe (same blue→purple→pink→orange identity as
  `.chat-area-flow`); `.tab:disabled` for the locked Chat tab.

## Invariant

Vibe mode is flow-only: with vibe on, the user cannot be on or reach the Chat
tab; the flow picker can only offer flows the mode can start. Dev mode keeps
full access to both tabs and every non-vibe flow. `openHistoryRun` is
deliberately not coerced — a CP-89 armed (`flowArm:"pending"`) chat legitimately
resumes on the chat surface so the user can discuss before forwarding.

## Tests

- `state/ca1070_vibe_flow_surface.test.ts` (new, 8/8): dev/vibe list filtering
  incl. hidden + system vibe flows, gate predicate parity, chat→vibe auto
  switch, flow+ vibe stays, chat re-entry refused, vibe-off keeps flow tab,
  stale selection cleared.
- `tests/phase1/navigatorCatalog.test.ts` (+`packFlowId` expectation — the new
  mapped field, not a weakened assertion).
- `npx tsc --noEmit` clean; phase1 suite green except pre-existing HEAD
  failures (history-replay ordering ×3, findDuplicateJiraIntegration,
  styles.tokens path ENOENT, store.test ×5, importBoundary, health payload —
  all reproduce on HEAD without this diff).
