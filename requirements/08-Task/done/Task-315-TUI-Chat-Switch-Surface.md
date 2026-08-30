# Task-315: TUI Chat Switch Surface

## Metadata

- Document ID: `Task-315`
- Title: `TUI Chat Switch Surface (Posture Tab, /mode, /model, /provider)`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-29`
- Last Updated: `2026-08-30` (done — slices 1+2 CA-696/CA-697, slice 3 CA-699: detached reattach, /open restore-by-chat, seed-stats divider, persisted-legSeq reattach; 16 TUI switch tests green; R1 baselines identical)
- Parent Documents: [CP-59: Chat SSOT — Continuous Cross-Provider Chat](../../07-Coding-Plan/todo/CP-59-Chat-Ssot-Continuous-Cross-Provider-Chat.md), [Task-314: Chat Switch-Provider Endpoint And Chat Envelope](../done/Task-314-Chat-Switch-Provider-Endpoint-And-Chat-Envelope.md)
- Child Documents: `None`
- Related Documents: [BUG-330](../../09-BugFix/done/BUG-330-Posture-Tab-Applies-Foreign-Provider-Model-On-Pinned-Run.md), [CA-679](../../../change-audit/CA-679-opencode-config-file-env-and-tui-model-restore.md), [Task-313](../done/Task-313-ChatId-Data-Model-Transcript-Store-Timeline.md)
- Replaces: `None`
- Tags: `chat-ssot, tui, cli-tui, posture-tab, provider-switch, bug330`

## AI Quick View

### Summary

- Wire the four TUI switch entry points — posture Tab, `/mode`, `/model <foreign>`, `/provider <key>` — to the Task-314 switch endpoint for live chat runs, with in-place adoption: transcript (`m.messages`) is never cleared, the divider renders from the server-seeded turn (client never appends — `SD26` single-source rule), per-run stream state resets, chat continues on the new leg. Detached chats (restored, no active leg) skip the switch endpoint and reattach via `createRun(chatId, switchFromRunID=latestLeg)` (`SD26` §10).
- Make posture pins honest: a bare-model pin (BUG-330's `plan={grok-4.5}`) derives its provider once via catalog/prefix, persists the derived provider, and routes cross-provider Tab to the switch instead of swapping `m.model` inside the old provider.
- Collapse handoff seeds: `handoffPromptPrefix` turns render as a one-line divider (live and replayed), never as a raw user bubble.

### Current Ask

- Implement CP-59 `P-5` (+ TUI half of `P-7`): client bindings, `ChatSwitchedMsg` adopt path, posture pin derivation + routing, `/model`/`/provider` routing, in-flight guard, divider rendering, and the BUG-330 + rapid-Tab model tests.

### Key Decisions

- `T-1` **Adopt, don't reset — and don't self-divide**: on `ChatSwitchedMsg` the handler keeps `m.messages` (scrollback continuity is structural), swaps `m.runHandle`, resets `m.lastEventSeq = handle.LastEventSeq`, clears `m.turnStream`/`m.turnSendPending`, re-notes seq via `m.client.NoteLastSeq`, **attaches the stream to the new run** (see `T-5`), and refreshes the session panel (`app.go:1221` `RunStartedMsg` is the template — it already never clears messages). The divider is **not** appended by the client: it renders from the seed turn's `isHandoffSeed` payload arriving on the attached stream (single-source rule, Task-312 `T-5`).
- `T-2` **Pin provider derivation, once and persisted**: `pinnedProviderFor(prof)` = explicit `prof.Provider`, else `providerForModel(m.providers, prof.Model)` (catalog, `chat_posture.go:255`), else prefix mirror of `providerKeyFromModel` (`provider_registry.go:202` — TUI-side constant copy with a parity test). When derivation fills an empty `prof.Provider`, persist it back via `cmdSaveChatPosture` with a one-time system warning (CP-59 `P-6` Key Decision / BUG-330 `F-1`+`Q-2`).
- `T-3` **Routing rule**: live chat run (`m.runHandle != nil`, `RunKind=="chat"`, flag on) + pinned/target provider ≠ current leg provider → `cmdSwitchChatProvider`; **detached chat** (`m.chatDetached` — restored, no active leg; review I-R3) → never the switch endpoint (`409 chat_no_active_leg`): Tab/`/mode`/`/model` and the next prompt go through the **reattach path** — `cmdStartRun` carrying `chatId` + `switchFromRunID=latestLeg` (`SD26` §10), envelope seeded server-side identically; otherwise today's paths verbatim (`applyChatPostureProfile` `:161`, same-provider model swap). No run yet → provider/model just set locally as today (first `cmdStartRun` uses them).
- `T-4` **`/provider` block replaced only for chat runs**: the `Cannot change provider after a run has started. Use /new to start fresh.` block at `app.go:3870` stays for workflow/flow kinds and flag-off; chat runs route to the switch.
- `T-5` **In-flight guard**: `chatSwitchInFlight bool` (+ queued posture name) — a Tab during an in-flight switch records the desired posture and applies it after `ChatSwitchedMsg` lands; rapid double-Tab produces exactly one new leg (CP-59 CS-05; Desktop parity in Task-316).
- `T-6` **Divider precedence over prefix matching**: prefer the typed `isHandoffSeed` marker (Task-314 `T-6`) on the event; fall back to `handoffPromptPrefix` string match on replay paths. The prefix constant is duplicated in `tui/client` with a parity test against the runner constant to prevent drift.

### Constraints

- Flag-gated: `FLOWPILOT_CHAT_SSOT` off → all four entry points behave byte-identically to today (including the `app.go:3870` block text).
- No runner changes beyond Task-313/314 surfaces; no bubbletea input work (BUG-328 scope); no Desktop changes (Task-316).
- CA-638/641/679 semantics preserved: resume/`/new`-reapply paths must NOT re-pin user's model/reasoning choices — only real posture switches route through the switch.
- Additive tests only (new `bug330_*`/`chat_switch_*` test files).

### Open Questions

- None — divider omitted-count display is frozen by Task-312 `T-3` (stats ride the seed payload; renderer formats "carried N of M turns (mode)" when truncated).

### Source Refs

- CP-59 Work Breakdown `P-5`, `P-7`; Key Decisions `P-6`, `P-7`, `P-8`; SD-26 `SD26-D-7`, `SD26-E-9`, `SD26-X-4`.
- `tui/app/chat_posture.go:146` (`postureModelPinWins`), `:161` (`applyChatPostureProfile`), `:231` (`setPostureProvider`), `:255` (`providerForModel`); `tui/app/app.go:1221` (`RunStartedMsg` adopt template), `:3870` (`/provider` block), `:5937` (`cmdStartRun`); `tui/app/model.go:510` (`pendingPrompt`); `tui/app/chat_history_replay.go:6` (budgets).
- `provider_registry.go:202` (`providerKeyFromModel` — prefix mirror source); BUG-330 §3 repro steps (TUI), §8 `V-1`, `V-4`, `V-5`.

## 1. Goal

In the TUI, switching provider mid-chat — by Tab, `/mode`, `/model`, or `/provider` — starts a new leg via the runner endpoint and continues the same visible conversation with one truthful divider; BUG-330's exact repro ends with the user talking to a real Grok leg; rapid Tab cannot mint two legs.

## 2. Parent Links

- coding plan: `CP-59` Work Breakdown `P-5`, `P-7`
- tech design: `SD-26` (`SD26-D-7`, `SD26-E-9`, `SD26-X-4`)
- system spec: `SS-05`
- specific upstream ids: CP-59 `P-5` full spec; BUG-330 `V-1`/`V-4`/`V-5` (TUI-level)

## 3. Trigger

Task-314's endpoint exists; without the TUI surface the operator still has no cross-provider path (`/provider` blocks, Tab misfires). This is the slice where the BUG-330 user story actually changes.

## 4. Exact Change

- `T-1` `apps/local-runner/internal/tui/client/client.go` — bindings + prefix parity.
  Current: client has `StartRun` (`client.go:784`) but no chat/switch calls. Delta:
  ```go
  type ChatSwitchInput struct {
      TargetProviderKey string `json:"targetProviderKey"`
      Model             string `json:"model,omitempty"`
      ReasoningEffort   string `json:"reasoningEffort,omitempty"`
      YoloMode          *bool  `json:"yoloMode,omitempty"`
  }
  type ChatSwitchHandoffStats struct {
      Mode              string `json:"handoffMode"`
      IncludedTurnCount int    `json:"includedTurnCount"`
      OmittedTurnCount  int    `json:"omittedTurnCount"`
      Truncated         bool   `json:"truncated"`
      ActionsDigest     bool   `json:"actionsDigestIncluded"`
  }
  type ChatSwitchResponse struct {
      Handle  RunHandle              `json:"handle"`
      ChatID  string                 `json:"chatId"`
      LegSeq  int                    `json:"legSeq"`
      Handoff ChatSwitchHandoffStats `json:"handoff"`
  }
  func (c *Client) SwitchChatProvider(ctx context.Context, chatID string, in ChatSwitchInput) (ChatSwitchResponse, error)
  func (c *Client) GetChatTimeline(ctx context.Context, chatID string, afterSeq int64, limit int) (ChatTimelineResponse, error)
  ```
  `RunHandle` gains `ChatID`/`LegSeq` (Task-313 `T-2` — mirrored here). Prefix parity: `const HandoffPromptPrefix = "[FlowPilot cross-provider chat handoff]"` + `TestHandoffPromptPrefixParity` asserting equality with the runner constant (import cycle-safe: literal + test comparing against a build-tag'd runner include or a golden string kept in both test files). Tests: `TestSwitchChatProviderClientRoundTrip` (httptest server).
- `T-2` `apps/local-runner/internal/tui/app/chat_switch.go` (new) — command + messages + guard.
  Current: nothing. Delta:
  ```go
  type ChatSwitchedMsg struct {
      Resp *client.ChatSwitchResponse
      Err  error
  }

  func (m *AppModel) cmdSwitchChatProvider(targetProvider, model, reasoning string, yolo *bool, queuedPosture string) tea.Cmd {
      chatID := m.runHandle.ChatID
      runnerURL := m.runnerURL
      return func() tea.Msg {
          cl := client.New(runnerURL)
          ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
          defer cancel()
          resp, err := cl.SwitchChatProvider(ctx, chatID, client.ChatSwitchInput{
              TargetProviderKey: targetProvider, Model: model,
              ReasoningEffort: reasoning, YoloMode: yolo,
          })
          return ChatSwitchedMsg{Resp: &resp, Err: err}
      }
  }
  ```
  Fields appended to `AppModel` (`model.go`): `chatSwitchInFlight bool`, `chatSwitchQueuedPosture string`. The Tab/`/mode`/`/model`/`/provider` handlers set `chatSwitchInFlight=true` and return the cmd; `ChatSwitchedMsg` clears it and, if `chatSwitchQueuedPosture != ""`, applies that posture against the new leg (one extra switch if it differs). Tests: `TestTabDuringInFlightSwitchQueuesOnce`.
- `T-3` `apps/local-runner/internal/tui/app/chat_posture.go` — pin derivation + routing.
  Current: `applyChatPostureProfile` (`:161`) swaps `m.model` from the pin and skips provider when `prof.Provider==""` — the BUG-330 hole. Delta:
  ```go
  // pinnedProviderFor — explicit pin, else catalog, else prefix mirror (T-2).
  func (m *AppModel) pinnedProviderFor(prof client.ChatPostureProfile) string {
      if p := strings.TrimSpace(prof.Provider); p != "" {
          return p
      }
      if p := providerForModel(m.providers, prof.Model); p != "" {
          return p
      }
      if p, ok := providerKeyFromModelMirror(prof.Model); ok { // prefix mirror of provider_registry.go:202
          return p
      }
      return ""
  }
  ```
  In the posture-switch entry (Tab handler / `/mode`, before `applyChatPostureProfile` runs its pin block): if `m.runHandle != nil && m.runHandle.RunKind == "chat" && chatSSOTOn && pinned != "" && !strings.EqualFold(pinned, m.provider)` → persist derived provider when it was empty (`editChatPostureProfile` path + `cmdSaveChatPosture`) and return `m.cmdSwitchChatProvider(pinned, prof.Model, prof.ReasoningEffort, prof.Yolo, name)`. Same-provider pins keep `applyChatPostureProfile` verbatim (CA-679 semantics untouched). Tests: `TestBareModelPinDerivesProviderOnceAndPersists`, `TestCrossProviderTabRoutesToSwitch`, `TestSameProviderTabKeepsInPlacePath`.
- `T-4` `apps/local-runner/internal/tui/app/app.go` — `/model` + `/provider` routing.
  Current: `/provider` mid-run blocks at `app.go:3870` for every kind; `/model` mid-run swaps `m.model` regardless of provider ownership. Delta: both handlers gain the chat-run branch (same predicate as `T-3`, target provider from `providerForModel`/explicit key): chat+flag+cross-provider → `cmdSwitchChatProvider`; workflow/flow kinds and flag-off keep the exact block text. Tests: `TestModelForeignMidChatRoutesToSwitch`, `TestProviderMidChatChatKindRoutesToSwitch`, `TestProviderBlockStaysForWorkflowRuns`, `TestProviderBlockStaysWhenFlagOff`.
- `T-5` `app.go` — `ChatSwitchedMsg` adopt handler + stream attach (the `RunStartedMsg` template, `:1221`).
  Current: no such message. Delta:
  ```go
  case ChatSwitchedMsg:
      m.chatSwitchInFlight = false
      if msg.Err != nil {
          m.addMessage("system", fmt.Sprintf("Provider switch failed: %v — chat continues on %s", msg.Err, m.provider), "error")
          return m, nil
      }
      h := msg.Resp.Handle
      m.runHandle = &h
      m.provider = h.ProviderKey
      m.model = msg.Resp.Model // target model from chatSwitchResponse (Task-314 T-1 DTO; review I-R6)
      m.lastEventSeq = h.LastEventSeq
      m.turnStream = nil
      m.turnSendPending = false
      m.client.NoteLastSeq(h.RunID, h.LastEventSeq)
      m.refreshSessionPanel()
      // No client-side divider here: the seed turn (isHandoffSeed + stats)
      // arrives on the attached stream and renders as the single divider.
      if m.chatSwitchQueuedPosture != "" { /* apply queued posture against new leg; may issue one follow-up switch */ }
      return m, tea.Batch(m.cmdAttachRunStream(h.RunID, h.LastEventSeq))
  ```
  `cmdAttachRunStream(runID, afterSeq)` — new command reusing the `client.StreamRun` pump shape (`app.go:6010` `cmdStreamRun`) but feeding `turnStreamEventMsg`/`turnStreamClosedMsg` persistently (not just until `turn_completed`) so the server-side seed turn streams into the transcript without a client-sent turn (review I-5: seed is server-side, so the TUI must attach passively). `m.messages` is never touched — scrollback continuity is structural. Tests: `TestSwitchAdoptKeepsTranscriptAndResetsStreamState`, `TestSwitchAdoptAttachesNewRunStream`, `TestSwitchSeedDividerStatesCarriedAndMode` (divider rendered from seed payload stats), `TestSwitchFailureKeepsChatOnSourceLeg`.
- `T-6` Divider/collapse rendering — message render path (live events + `chat_history_replay.go` replay): a user-message whose text has `isHandoffSeed` (typed) or — fallback — the `HandoffPromptPrefix` prefix renders as the one-line divider instead of a bubble. Tests: `TestHandoffSeedRendersAsDividerLive`, `TestHandoffSeedRendersAsDividerOnReplay`, `TestHandoffPromptPrefixParity` (with `T-1`).
- `T-7` Restore picker (ca554) + reopen: `/open` on a chat run fetches `GetChatTimeline` when flag on and replays the full multi-leg timeline (records mapped into the same message model, seeds collapsed per `T-6`). Tests: `TestReopenChatReplaysAllLegs`.

## 5. Touched Areas

- files: new `tui/app/chat_switch.go` (+ tests `chat_switch_tui_test.go`, `bug330_posture_tab_switch_test.go`); edits `tui/client/client.go` (bindings, `RunHandle` fields, prefix const), `tui/app/chat_posture.go` (derivation + routing), `tui/app/app.go` (`ChatSwitchedMsg` case, `/model`, `/provider`), `tui/app/model.go` (guard fields), `tui/app/chat_history_replay.go` (seed collapse + multi-leg fetch)
- modules: TUI chat client; bubbletea update loop
- routes consumed: `POST /client/chats/{chatId}/switch-provider`, `GET /client/chats/{chatId}/timeline`
- tables: none

## 6. Acceptance Check

- Manual walk (flag on, real providers, gate-sandbox): scan(opencode) → code(opencode, same session) → Tab plan(grok-4.5) → divider appears, reply identity = Grok → `/provider codex` → divider, identity = codex; restart TUI → `/open` replays both legs; footer/session panel truthful at every step. BUG-330 §3 repro produces zero `model not found`.
- Flag off: Tab/`/mode`/`/model`/`/provider` byte-identical to today.

### 6.1 Definition of Done (DOD)

- [x] `DOD-1` Cross-provider Tab routes to switch and lands on the target provider with a truthful divider (BUG-330 `V-1` TUI half) — `TestCrossProviderTabRoutesToSwitch` + manual walk.
- [x] `DOD-2` Bare-model pin derives provider once, persists, warns once (`TestBareModelPinDerivesProviderOnceAndPersists`) — BUG-330 `F-1`.
- [x] `DOD-3` Same-provider Tab keeps in-place `session/load`+`set_config` path untouched (`TestSameProviderTabKeepsInPlacePath` — run-314536 turns 1–3 lock).
- [x] `DOD-4` Adopt keeps `m.messages`, resets stream state, **attaches the new run's stream**, and the divider renders from the seed payload with carried/mode stats (`TestSwitchAdopt*`, `TestSwitchAdoptAttachesNewRunStream`) — CS-09 owner for the kill-runner `/open` path is this task.
- [x] `DOD-5` Rapid double-Tab → one leg; queued posture applies after (`TestTabDuringInFlightSwitchQueuesOnce` — CS-05).
- [x] `DOD-6` `/model <foreign>` and `/provider <key>` on live chat route to switch; workflow kinds + flag-off keep the block verbatim (`T-4` tests).
- [x] `DOD-7` Switch failure keeps the chat usable on the source leg with an error line (`TestSwitchFailureKeepsChatOnSourceLeg`).
- [x] `DOD-8` Handoff seed renders as divider live + replay; prefix parity test guards drift (`T-6` tests).
- [x] `DOD-9` `/open` replays all legs of a switched chat (`TestReopenChatReplaysAllLegs`).
- [x] `DOD-10` Full `just chat-test` + `go test ./internal/tui/...` green; zero pre-existing test edits.

### 6.2 Test Signatures

```go
// tui/app/bug330_posture_tab_switch_test.go (new file)
func TestBug330PostureTabSwitchesToRealGrokLeg(t *testing.T)   // pins scan=opencode/muse, plan=grok-4.5(bare); live chat on opencode leg; Tab→plan → ChatSwitchedMsg adopted, provider==grok, transcript len unchanged+1 divider
func TestBareModelPinDerivesProviderOnceAndPersists(t *testing.T) // profile {provider:"",model:"grok-4.5"} → first apply derives grok + PUTs posture (save called once); second apply does not re-derive (no second save)
func TestCrossProviderTabRoutesToSwitch(t *testing.T)
func TestSameProviderTabKeepsInPlacePath(t *testing.T)          // scan(opencode/muse)→code(opencode/deepseek): no switch cmd; applyChatPostureProfile pin block ran (model==deepseek)
func TestTabDuringInFlightSwitchQueuesOnce(t *testing.T)        // two Tab msgs before ChatSwitchedMsg → exactly one SwitchChatProvider client call; queued posture applied after
func TestProviderBlockStaysForWorkflowRuns(t *testing.T)
func TestDetachedChatTabReattachesViaStartRun(t *testing.T)    // restored chat (no active leg): Tab plan → startRun carries chatId+switchFromRunID; switch endpoint NOT called; envelope seeded; transcript preserved
func TestProviderBlockStaysWhenFlagOff(t *testing.T)            // block text byte-identical to app.go:3870 string

// tui/app/chat_switch_tui_test.go (new file)
func TestSwitchAdoptKeepsTranscriptAndResetsStreamState(t *testing.T) // 4 msgs + switch → 5th msg arrives from seed stream (not client); lastEventSeq==handle.LastEventSeq; turnStream nil→attached to new run
func TestSwitchAdoptAttachesNewRunStream(t *testing.T)               // after adopt, seed turn events from new runId reach handleEvent without a client-sent turn
func TestSwitchSeedDividerStatesCarriedAndMode(t *testing.T)         // seed payload stats{14,0,raw} → "carried 14 turns (raw)"; stats{14,5,target_summary} → "carried 14 of 19 turns (target_summary)"; exactly one divider per switch
func TestSwitchFailureKeepsChatOnSourceLeg(t *testing.T)              // Err → runHandle/provider unchanged, error line appended, chatSwitchInFlight false
func TestModelForeignMidChatRoutesToSwitch(t *testing.T)
func TestProviderMidChatChatKindRoutesToSwitch(t *testing.T)
func TestHandoffSeedRendersAsDividerLive(t *testing.T)                // stream event turn_started isHandoffSeed → rendered as divider, not bubble
func TestHandoffSeedRendersAsDividerOnReplay(t *testing.T)            // replayed record with prefix text (no typed marker) → divider
func TestHandoffPromptPrefixParity(t *testing.T)                      // tui const == runner const
func TestReopenChatReplaysAllLegs(t *testing.T)                       // /open on switched chat → timeline fetch, msgs contain both legs' turns, seeds collapsed
func TestSwitchChatProviderClientRoundTrip(t *testing.T)              // httptest: request JSON shape + response decode
```

## 7. Out of Scope

- Desktop chips/timeline (Task-316); runner endpoint/envelope (Task-314); Drive sync/restore (Task-317); bubbletea input refactor (BUG-328 line); mode-setup modal redesign; opencode internal `mode=build|plan` mapping (CP-57 `F-5` stays separate).

## 8. Completion Notes

- result: DONE 2026-08-30 — CA-696 (slice 1: /provider + /model routing, adopt, client bindings), CA-697 (slice 2: posture Tab cross-provider routing + bare-model derive-once), CA-699 (slice 3: detached reattach on first prompt via persisted-legSeq startRun, /open restore-by-chat timeline backfill, seed-stats divider). 16 TUI switch tests + runner persisted-legSeq test green; R1: TUI 7 baseline / runner 13 baseline (+2 proven flakes) identical across slices.
- follow-ups: seed-stats divider on REPLAY renders from E-9 records (live path done); queued-posture second-switch chaining when the queued pin is itself cross-provider; Desktop parity in Task-316.
- upstream docs updated: Task-315 moved to done/; CP-59 + Task-316/317 links updated; SD-26 §14 (account-switch correction) recorded separately in CA-698.
