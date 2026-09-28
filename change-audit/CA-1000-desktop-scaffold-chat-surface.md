# CA-1000 — Desktop: AI Scaffold runs as a live Chat transcript from the Projects page

## What changed

`apps/desktop-flowpilot/src/state/store.ts`:

- New `scaffoldSession` state + `runScaffoldChat({ projectId, workingDirectory,
  platform, modelName, force })` action. The action resets the chat surface,
  seeds a transcript (prompt row + intro line + one streaming assistant
  bubble), fires `POST /client/projects/{id}/scaffold` (still the blocking
  endpoint), and pumps `GET .../scaffold/progress` — the same persisted
  CA-916 feed the TUI replays — folding `output` deltas into the assistant
  bubble, `phase` milestones into system lines, and the terminal `result`
  into a closing line with info/warn/error tone.
- The pump is a pure watcher keyed by a session token: `resetRun`,
  `openHistoryRun`, and project switches clear `scaffoldSession`, which
  detaches the pump without cancelling the server-side turn.
- Stale-tail guard: the runner clears its progress hub at dispatch begin, so
  events are only folded while the snapshot is `active` or after this run's
  `started` marker was observed — a pre-begin poll that still serves the
  previous run's events/result is skipped wholesale.
- Termination is bounded three ways: a feed `result` event, a terminal
  `snap.result` after `started` was seen, or the settled POST plus a short
  drain — a dead feed can never hang the pump.
- `sendPrompt` refuses while a scaffold session is active (a second provider
  turn would write the same workspace — TUI blocks identically) and leaves a
  warn-tone system line for programmatic callers.
- `scaffoldChatTestHooks` mirrors `runUpdatesLoopTestHooks` so unit tests
  drive the pump at millisecond cadence.

`apps/desktop-flowpilot/src/components/settings/ProjectEnginePanel.tsx` (new):

- Per-binding engine surface extracted from EngineSettings: binding picker,
  skill-pack/capability/last-init status cards, Refresh + Initialize/Re-sync,
  and the AI Scaffold trigger which now delegates to a host callback.

`apps/desktop-flowpilot/src/components/settings/ProjectsSettings.tsx`:

- New collapsible "Engine / Skill Pack" panel (default expanded) in the
  project detail column, bound to persisted bindings only. Clicking Run AI
  Scaffold calls `onOpenChat` then `runScaffoldChat` — the user lands in Chat
  watching the live transcript.

`apps/desktop-flowpilot/src/components/settings/EngineSettings.tsx`:

- Dropped the skill-pack status cards, Initialize, and Scaffold controls (now
  project-local). The project/binding picker remains to scope Flow Gate and
  the auto-approve allowlist; Global Tooling, LibreTranslate, and Quota
  Routing are unchanged.

`apps/desktop-flowpilot/src/components/SettingsShell.tsx` + `src/App.tsx`:

- `onOpenChat` threads App → SettingsShell → ProjectsSettings.

`apps/desktop-flowpilot/src/components/ChatInput.tsx` + `src/styles.css`:

- `scaffoldSession.active` blocks the composer alongside run-busy statuses;
  when it is the only blocker the toolbar shows an "AI Scaffold · {phase}
  running…" pill instead of a Stop button — the runner exposes no scaffold
  cancel endpoint, so a Stop affordance would be a lie. Placeholder copy
  explains the pause.

## Invariant

The scaffold turn's durable record stays on the runner (NDJSON feed +
dispatch result); the chat transcript is a disposable watch surface that can
be opened, abandoned, and restarted without affecting the turn. No second AI
execution is spawned — the "chat" renders the scaffold's own progress stream.

## Tests

`apps/desktop-flowpilot/src/state/store.scaffoldChat.test.ts` (new, 5 tests):
transcript seeding + output streaming + phase/result folding, stale-tail
suppression while the POST is landing, dispatch-failure error line, detach on
`resetRun`, and `sendPrompt` refusal during scaffold.

Verified: `tsc --noEmit` clean; `npm run test:phase1` — 683 tests, 670 pass;
the 13 failures reproduce identically on the unmodified baseline (env: Node
26 lacks `localStorage` without `--localstorage-file`, plus unrelated
pre-existing assertions).
