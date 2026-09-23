# CA-922 — Desktop Devin-UX polish: project-group Navigator, header attention inbox, long-chat perf

## Context

User-driven UI/UX pass on the desktop chat workspace to bring it closer to
Devin Desktop: consistent tokens/icons, responsive narrow layouts, a less
noisy attention surface, project-grouped navigation, and resilience for
day-long chats with thousands of timeline items.

## Changes

### Chrome & icons

- New shared `src/components/icons.tsx` SVG set (panel toggles, send,
  paperclip, inbox, warn/gate, carets, close, plus, spinner, etc.) — all
  icons carry explicit `width`/`height` so unsized SVGs can no longer inflate
  buttons (Chat/Code posture buttons) on first paint before CSS applies.
- `App.tsx` header uses `PanelLeftIcon`/`PanelRightIcon` ghost buttons plus a
  new `AttentionInbox`; `main.ts` window `backgroundColor` aligned to the
  theme bg to remove the blue flash on launch; preload exposes `platform`.

### Composer (ChatInput)

- Single-card composer: borderless textarea + icon toolbar (attach paperclip,
  collapsed-controls menu, circular send/stop). Text "Send" button removed.
- Chat Intent panel removed from `ChatWorkspace` right rail (underlying
  `chatStartMode` plumbing intentionally left in place).

### Responsive

- `matchMedia`-driven auto-collapse of left/right sidebars at narrow widths;
  single-column layout under ~980px; header actions/provider chips wrap;
  `min-width: 0` on flex children to kill horizontal overflow. Vertical
  scroll only, per request.

### Navigator → collapsible project groups

- Project `<select>` removed. Each project is a group: caret toggles expand
  (peek without switching), name switches project, `+` starts a new chat in
  that project (auto-sets current project), group badge shows chat count and
  a warn-colored attention count.
- Inactive (expanded) groups render a read-only peek list; clicking a chat
  auto-selects the owning project and opens the run.
- `projectHistoryById` moved from Navigator-local state into the store;
  `loadProjectHistory`/`loadAllProjectHistories` warm every project's history
  (30s refresh) so counts and attention badges are correct before visiting;
  a store subscription keeps the selected project's slot synced with
  `runHistory` mutations.

### Attention queue → header inbox

- `attentionQueue` now keeps per-project history slices, so items aggregate
  across ALL loaded projects (previously active-project-only — runs stuck in
  other projects were invisible). `AttentionItem` gains `projectId`.
- New `AttentionInbox` header component: inbox icon + badge count + popover
  listing waiting runs oldest-first with kind chip, project name, and wait
  time. `openRunAtAttention(runId, chatId, projectId?)` switches project
  before replaying via `openHistoryRun`.
- `openHistoryRun(runId, item?)` accepts an optional history-item override so
  cross-project opens supply provider/chat hints without waiting for the
  destination project's history fetch.

### Long-chat resilience

- `Timeline` autoscroll is now sticky-bottom only: no more
  `scrollIntoView({behavior:"smooth"})` on every streamed token; instant jump
  while pinned, and sending a new prompt re-engages the pin.
- `sliceTimelineFromPrompt`/`buildTimelineGroups` memoized.
- `content-visibility: auto` + `contain-intrinsic-size` on timeline rows —
  off-screen rows skip layout/paint; sticky `.load-earlier-btn`, `.crumb`,
  and `.translate-popover` are excluded.

## Validation

- `tsc --noEmit` clean (this also fixes the pre-existing
  `store.chat-mode-persist.test.ts` compile error — it already called
  `openHistoryRun` with two args).
- `npm run build` (tsc + vite + electron main/preload) green.
- Targeted phase1 tests: 17/17 pass (`attention_queue`, `tokens`,
  `chat-mode-persist`, `chatHistory`).
- Full `src/state` suite: 9 failures all reproduced identically on stashed
  HEAD — pre-existing (`history replay order`, `localStorage` stub,
  `historyOpenError` leakage between tests, terminal-parity Thinking).
- `tests/phase1/adminLogic.test.ts` has a separate pre-existing compile
  break (`SupportedModel.inputImage` missing) that gates `npm run
  test:phase1` wholesale — untouched.

## Notes / follow-ups

- `AttentionQueue.tsx` is now dead code (superseded by `AttentionInbox`) but
  kept because `styles.tokens.test.ts` guards it — removing it requires
  editing that test's file list (additive-tests-only rule).
- Timeline store growth is still unbounded (DOM is bounded by prompt paging +
  content-visibility); a store-level trim/virtualization is a possible
  follow-up for multi-day sessions.
- Chat Intent feature plumbing (`chatStartMode`, `subMode`, `flowRef`,
  `builtinOrchestrationOptions`) remains — UI-only removal per request.

## GitNexus

MCP server unreachable this session (`mcp_list_tools` fails) — impact
analysis done manually via grep: `openRunAtAttention` has a single prior
caller (the queue component) plus its test; `ingestHistory` keeps its
signature with additive per-project semantics; `openHistoryRun` change is an
optional param, all call sites verified.

# ---8<--- flowpilot:change-ledger
feature_key: desktop-ui-consistency
source_doc_id: Task-405
change_type: feature
summary: Devin-style desktop UX pass — shared SVG icons, single-card composer, collapsible project-group navigator, cross-project header attention inbox, sticky-bottom autoscroll + content-visibility for long chats
# --->8---
