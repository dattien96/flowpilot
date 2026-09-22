# Task-409: Worktree Toggle & Start UX (CP-71 P-3)

## Metadata

- Document ID: `Task-409`
- Title: `Worktree Start Toggle — ChatPosturePanel Control, Navigator Badge, TUI Option`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-09-22`
- Last Updated: `2026-09-22`
- Parent Documents: [CP-71](../../07-Coding-Plan/done/CP-71-Run-Worktree-Isolation.md) `P-3`, [SD-27](../../06-System-Tech-Design/SD-27-Run-Worktree-Isolation.md) `D-5/D-9`
- Child Documents: `None`
- Related Documents: [Task-408](./Task-408-Run-Worktree-Binding-And-Owner-Resolution.md), [Task-326](../done/Task-326-Vibe-Working-Mode-Switch-And-Flow-Family-Gate.md) (toggle precedent)
- Replaces: `None`
- Tags: `worktree, desktop, tui, ux, toggle`
- Feature Keys: `run-worktree`

## AI Quick View

### Summary

- Surface the `worktree` start flag to users: a "Run in worktree" toggle in the Desktop chat controls bar (`ChatPosturePanel`, next to reasoning/yolo), a start option in TUI, and a `worktree` badge on Navigator run items.
- Toggle semantics per `D-9`: persisted per chat (restored from the chat's newest run), new chat = off, mid-run pinned, disabled with tooltip on non-git projects, rejected-off while a live binding exists.

### Current Ask

- Implement the toggle + badge on Desktop and the start option + status badge on TUI, consuming the `worktree` fields landed by Task-408.

### Key Decisions

- `T-1` Toggle sits in the chat controls bar (`ChatPosturePanel` on Desktop; mode/options line on TUI) — per the product decision, same zone as mode/reason/yolo.
- `T-2` Toggle state = `worktreeEnabled` derived from the chat's newest run (via `runHistory`); setting it writes through on next `POST /client/workflow-runs`.
- `T-3` Disabled states: non-git project → tooltip "requires a git repository"; live binding → tooltip "merge or discard the worktree first"; running turn → read-only.

### Constraints

- Client-side only; no runner changes beyond Task-408's fields.
- Follow `working_mode` toggle wiring (Task-326) — same session-toggle channel, same mid-run pin rule.
- Additive vitest/Go tests only; old suite green.

### Open Questions

- `None`

### Source Refs

- `apps/desktop-flowpilot/src/components/ChatPosturePanel.tsx`, `Navigator.tsx`, `src/state/store.ts`, `src/state/workingMode.ts` (toggle precedent), `internal/tui/app/working_mode.go` (TUI precedent), `SS-23 AC-1/AC-10`, `SD-27 D-5/D-9`.

## 1. Goal

A user can opt a chat/flow into worktree isolation from the controls they already know, see which runs are isolated, and never hit an invalid toggle state.

## 2. Parent Links

- coding plan: `CP-71 P-3`
- tech design: `SD-27 D-5, D-9`
- system spec: `SS-23 AC-1, AC-9, AC-10, E-5`
- specific upstream ids: `Task-408` (fields), `Task-326` (toggle precedent)

## 3. Trigger

Backend fields exist after Task-408; this is the user-facing surface.

## 4. Exact Change

- `T-1` Desktop `ChatPosturePanel`: `RunInWorktreeToggle` — icon + label + tooltip; disabled reasons per `T-3`.
- `T-2` `store.ts`: `worktreeEnabled` per-chat derived getter + setter queuing `worktree` onto the next `startRun` call; restore on chat switch from newest run (`runHistory` + snapshot `worktree` block).
- `T-3` `Navigator`: `worktree` badge (icon + slug) on run items where `worktreeState != ""/"none"`.
- `T-4` TUI: start option/flag for worktree (mirroring `working_mode.go` toggle plumbing) + status badge in run chrome.

## 5. Touched Areas

- files: `ChatPosturePanel.tsx`, `Navigator.tsx`, `store.ts`, `HttpWsRunnerClient.ts` (pass field), `internal/tui/app/working_mode.go`-adjacent new `worktree_toggle.go`, `styles.css`.
- modules: desktop UI + TUI.
- routes: none new. tables: none.

## 6. Code Guide Signatures

```ts
// apps/desktop-flowpilot/src/state/store.ts — additive
worktreeEnabled: boolean
setWorktreeEnabled(on: boolean): void        // T-2: rejected paths throw/notify per T-3
// restore: inside existing chat-switch load — reads newest run's worktreeEnabled

// apps/desktop-flowpilot/src/components/ChatPosturePanel.tsx
function RunInWorktreeToggle(props: {
  enabled: boolean; disabled: boolean; disabledReason?: string;
  onToggle(on: boolean): void;
}): JSX.Element // T-1

// apps/desktop-flowpilot/src/components/Navigator.tsx — badge render only, unchanged signature
```

```go
// apps/local-runner/internal/tui/app/worktree_toggle.go (new)
func (m *AppModel) worktreeEnabled() bool          // mirrors workingMode getter
func (m *AppModel) toggleWorktree() tea.Cmd        // applies to next start; mid-run pin respected
func (m *AppModel) worktreeBadge() string          // status chrome
```

## 7. Test Signatures

- `test("toggle defaults off for a new chat")` — no runs → `worktreeEnabled === false`.
- `test("toggle restores from newest run on chat reopen")` — chat with `worktreeEnabled:true` leg → pre-set on.
- `test("toggle off rejected while live binding")` — `worktreeState=active` → toggle disabled + reason.
- `test("toggle disabled on non-git project")` — disabledReason rendered.
- `test("startRun sends worktree:true when enabled")` — mock client asserts field.
- `test("Navigator shows worktree badge with slug")` — run item with `worktreeState`/`worktreeSlug`.
- `TestTUI_WorktreeToggleAppliesNextStart` — flag included in start payload.
- `TestTUI_WorktreeBadgeVisible` — chrome renders slug/state.

## 8. Acceptance Check

- New chat: toggle off; enable → next run starts in worktree (verify cwd badge).
- Reopen that chat: toggle on; attempt to disable while run live → rejected message.
- Navigator shows badge on isolated runs only.
- TUI: start option works; badge shows on status chrome.

## 9. Out of Scope

- Merge-back card (`Task-410`); `lost` notice UX (`Task-411`); Admin Web; per-project defaults.

## 10. Definition of Done

- [ ] §6 signatures landed; §7 tests exist, green, additive-only.
- [ ] Old suites untouched/green — failure → STOP + report.
- [ ] Provider parity: N/A — start flag is provider-neutral (evidence in CA).
- [ ] `feature_key: run-worktree`; CA ledger entry; `detect_changes` clean.

## 11. Completion Notes

- result: `TBD`
- follow-ups: `TBD`
- upstream docs updated: `TBD`
