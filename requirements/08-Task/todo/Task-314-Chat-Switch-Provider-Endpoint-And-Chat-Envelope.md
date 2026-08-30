# Task-314: Chat Switch-Provider Endpoint And Chat Envelope

## Metadata

- Document ID: `Task-314`
- Title: `Chat Switch-Provider Endpoint And Chat Envelope`
- Phase: `task`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-29`
- Last Updated: `2026-08-29`
- Parent Documents: [CP-59: Chat SSOT — Continuous Cross-Provider Chat](../../07-Coding-Plan/todo/CP-59-Chat-Ssot-Continuous-Cross-Provider-Chat.md), [Task-313: ChatId Data Model, Transcript Store, Timeline](./Task-313-ChatId-Data-Model-Transcript-Store-Timeline.md)
- Child Documents: `None`
- Related Documents: [Task-078: Cross-Provider Chat Handoff](../done/Task-078-Cross-Provider-Chat-Handoff.md), [Task-312: Chat SSOT Design Freeze (SD-26)](./Task-312-Chat-Ssot-Design-Freeze-SD26.md), [BUG-330](../../09-BugFix/done/BUG-330-Posture-Tab-Applies-Foreign-Provider-Model-On-Pinned-Run.md)
- Replaces: `None`
- Tags: `chat-ssot, switch-provider, handoff-envelope, actions-digest, runner, bug330`

## AI Quick View

### Summary

- Implement the atomic runner-side switch: `POST /client/chats/{chatId}/switch-provider` closes the active leg, opens a new provider leg on the same chat, and **server-side** seeds its first turn with a full-chat handoff envelope (all legs' conversation text + a new `<actions_summary>` block compiled from tool/approval records).
- Generalize the Task-078 envelope to chat scope with a budget derived from the target model's context window (fallback 64 KiB), and open opencode (and gemini when proven) as envelope sources — the chat store, not provider folders, is the source now.
- This endpoint alone kills the BUG-330 class: a foreign model can never enter the old adapter again — cross-provider always means a new leg on the right provider.

### Current Ask

- Implement CP-59 `P-4` (+ the `P-7` same-provider guard server side): guards, chat-scope envelope builder, actions digest, leg close/open ordering under the service lock, crash-heal rollback, typed errors, and the switch-matrix test over fake adapters.

### Key Decisions

- `T-1` **Three-phase linearization (SD26-S-2, review C-2/C-3 fix)**: phase **A** under `s.mu` — validate guards, stamp `switchFromRunID` durable intent, mark in-flight, release; phase **B** no lock — build chat envelope (store I/O), `createRun` (takes `s.mu` itself, `interactive_handlers.go:795` region), fire-and-return seed dispatch; phase **C** under `s.mu` — close old leg (`provider_switch`), append `chat_provider_switch` record. Ordering is **start-new first, close-old second**: every crash window leaves a healable state (matrix in §4 `T-2`; zero TBD cells).
- `T-2` **Budget derives from target context window** (`types.go:34 ContextWindowTokens`): `min(contextTokens × 3 chars, hardCap 512 KiB)`, floor `handoffMaxBytes` (64 KiB) when unknown — `packConversationTurns` (`handoff_context.go:307`) semantics unchanged, only the budget number moves.
- `T-3` **Actions digest is additive to the envelope** (`<actions_summary>` block after `<previous_conversation>`): per-turn lines compiled from chat records (tool calls + files + approval decisions). Empty history → no digest, no envelope at all (`handoff_context_unavailable` shape becomes `switch_fresh_start` in the response, not an error — CP-59 CS-07).
- `T-4` **Source gating becomes chat-store-based**: `supportsHandoffSource` (`handoff_context.go:142`) is bypassed for chat-scope builds (the chat transcript is provider-neutral); it stays only for the legacy per-run path. opencode sources work from Task-313's records; gemini per `Q-5` (target-only until extractor proven — still enforced).
- `T-5` **Same-provider switch never mints a leg**: `handoff_same_provider` 409 (existing `handoff_context.go:67` semantics) — clients fall back to in-place `set_config`/`set_model` (Task-315/316 route this; runner also exposes the existing per-turn model-change path untouched).
- `T-6` **Seed turn carries `handoffPromptPrefix`** (`handoff_context.go:22`) so feature-resolution skip (`handoff_context.go` mapping) and the per-turn injection seam keep working; the record for the seed turn's prompt is emitted as `turn_started` with `isHandoffSeed:true` in payload so UIs collapse it without string matching (prefix matching stays as replay fallback).

### Constraints

- Flag-gated (`FLOWPILOT_CHAT_SSOT`, Task-313); off = endpoint absent (404) and today's behavior byte-identical.
- Chat-kind gate absolute: non-`chat` `runKind` → `409 handoff_run_kind_unsupported` (CP-59 `R-8`).
- No UI changes; no posture changes; no provider-adapter changes (switch composes `startRun` + existing turn send path).
- Task-078's `POST /client/workflow-runs/{runId}/handoff-context` stays untouched (Desktop still uses it until Task-316).
- Additive tests only; matrix tests use fake adapters (no real provider creds in CI).

### Open Questions

- None — seed dispatch resolved to **fire-and-return** (Task-312 `T-3`): the endpoint returns after dispatch, `handle.LastEventSeq` is the client attach point; Task-315 owns the stream attach.

### Source Refs

- CP-59 Work Breakdown `P-4`, `P-7`; Key Decisions `P-3`, `P-4`, `P-5`; SD-26 `SD26-S-1`, `SD26-D-5..D-7`, `SD26-X-1..X-6`, `SD26-E-8`.
- `handoff_context.go:15` (`handoffMaxBytes`), `:22` (`handoffPromptPrefix`), `:59` (`buildHandoffContext`), `:67` (`handoff_same_provider`), `:73` (`handoff_run_busy`), `:142` (`supportsHandoffSource`), `:156` (`transcriptTurnsFromRun`), `:281` (`renderHandoffPrompt`), `:307` (`packConversationTurns`).
- `types.go:34` (`ProviderModel.ContextWindowTokens`); `interactive_handlers.go:224` (`handleStartRun`) and the `:795` region `createRun` it delegates to — `createRun` takes `s.mu` and persists under it, which is why the switch must never hold `s.mu` across phase B (review C-2 evidence).
- BUG-330 §7 `F-3`/`F-4` (this task implements the generalized form); BUG-330 §8 `V-1..V-6`.

## 1. Goal

One endpoint that performs the whole cross-provider switch for a chat — guarded, atomic-on-metadata, server-seeded — with a chat-scope envelope (conversation + actions digest) budgeted by the target model's context, typed errors for every refusal, and a fake-adapter matrix test covering all 12 directed provider pairs plus the busy/same-provider/fresh-start/missing-provider cases.

## 2. Parent Links

- coding plan: `CP-59` Work Breakdown `P-4` (switch), `P-7` (same-provider guard)
- tech design: `SD-26` (`SD26-S-1`, `SD26-D-5..D-7`, `SD26-X-1..X-6`), `SD-06`
- system spec: `SS-05` (provider-selection invariant + chat-switch note)
- specific upstream ids: CP-59 `P-4` full spec; BUG-330 `V-1..V-6` (validated here at fake-adapter level, live in Task-315/316 manual walks)

## 3. Trigger

Task-313 landed the store + timeline: the switch now has something to read (chat transcript) and write (`chat_provider_switch`, leg fields). It is the first behavioral slice of CP-59 and unblocks both UI tasks.

## 4. Exact Change

- `T-1` `apps/local-runner/internal/runner/chat_switch.go` (new) — route + DTOs.
  Current: no chat-mutation routes. Delta:
  ```go
  // route (interactive_handlers.go mux block, after Task-313's timeline route)
  mux.HandleFunc("POST /client/chats/{chatId}/switch-provider", s.handleChatSwitchProvider)

  type chatSwitchRequest struct {
      TargetProviderKey ProviderKey `json:"targetProviderKey"`
      Model             string      `json:"model,omitempty"`
      ReasoningEffort   string      `json:"reasoningEffort,omitempty"`
      YoloMode          *bool       `json:"yoloMode,omitempty"` // nil = inherit current leg
  }
  type chatSwitchResponse struct {
      Handle  RunHandle              `json:"handle"`
      ChatID  string                 `json:"chatId"`
      LegSeq  int                    `json:"legSeq"`
      Model   string                 `json:"model"` // target model — clients set it on adopt (review I-R6; Task-315 reads resp.Model)
      Handoff chatSwitchHandoffStats `json:"handoff"`
  }
  type chatSwitchHandoffStats struct {
      Mode              string `json:"handoffMode"` // raw | hybrid | target_summary | fresh_start
      IncludedTurnCount int    `json:"includedTurnCount"`
      OmittedTurnCount  int    `json:"omittedTurnCount"`
      Truncated         bool   `json:"truncated"`
      ActionsDigest     bool   `json:"actionsDigestIncluded"`
  }
  ```
- `T-2` `chat_switch.go` — the switch operation (three-phase linearization per `SD26-S-2`; **never hold `s.mu` across `createRun`, which takes `s.mu` itself and persists under it — `interactive_handlers.go:795` region; lock-across would deadlock, review C-2**).
  Current: `buildHandoffContext` (`handoff_context.go:59`) is run-scoped and refuses same-provider; `createRun` is the shared internal start entry behind `handleStartRun:224`. Delta:
  ```go
  func (s *InteractiveService) switchChatProvider(ctx context.Context, chatID string, req chatSwitchRequest) (chatSwitchResponse, *apiErr) {
      // Phase A — under s.mu: guards + durable intent, then RELEASE.
      s.mu.Lock()
      src := s.activeChatLegLocked(chatID) // 404 chat_not_found / 409 chat_no_active_leg (SD26-X-1/X-2)
      if src.runKind != "chat" { unlock; return 409 "handoff_run_kind_unsupported" } // SD26-X-… chat-kind gate
      if src.turnInFlight || src.pendingApprovalID != "" || src.pendingQuestionID != "" { unlock; return 409 "handoff_run_busy" } // SD26-X-3
      if strings.EqualFold(string(src.providerKey), string(req.TargetProviderKey)) { unlock; return 409 "handoff_same_provider" } // SD26-X-4
      if s.chatSwitchInFlight[chatID] { unlock; return 409 "handoff_run_busy" } // one in-flight switch per chat
      src.switchFromRunID = src.id // durable intent (review I-R4: source marks ITSELF — discriminator key)
      s.persistProviderSession(sessionStateFrom(src)) // persist intent BEFORE unlock (review I-R5): crash-after-A leaves the marker on disk, heal row 2 non-vacuous
      s.chatSwitchInFlight[chatID] = true
      s.mu.Unlock()

      // Phase B — NO lock: chat-store I/O + createRun + seed dispatch.
      env := s.buildChatHandoffContext(chatID, req) // T-3; fresh_start when history empty
      handle, aerr := s.createRun(chatSwitchStartInput(chatID, src, req)) // ChatID, SwitchFromRunID=src.id, target provider/model/effort/yolo
      if aerr != nil {
          s.clearSwitchIntent(chatID, src.id) // best-effort; heal rule row 2 also clears on load
          return chatSwitchResponse{}, aerr
      }
      s.dispatchSeedTurn(handle, env) // fire-and-return (Task-312 T-3); stats ride the seed payload

      // Phase C — under s.mu: close old + append record (start-new-first ordering).
      s.mu.Lock()
      closeLegLocked(src, LegClosedReasonProviderSwitch)
      appendSwitchRecordLocked(chatID, src, handle, env.Stats) // SD26-E-9, exactly once (unique from/to pair)
      s.chatSwitchInFlight[chatID] = false
      s.mu.Unlock()
      return chatSwitchResponse{Handle: handle, ChatID: chatID, LegSeq: handle.LegSeq, Model: req.Model, Handoff: env.Stats}, nil
  }
  ```
  **Crash×step heal matrix** (`SD26-S-2`, closed in SD-26 §8 with zero TBD cells — Task-312 `T-6`); heal hook lives in the `loadPersistedRun` region (`interactive_resume.go:4784`):

  | crash window | observable state | heal on load |
  |---|---|---|
  | after A, before B | old leg `active` + `switchFromRunID`, no new leg | clear intent; leg stays active |
  | after B createRun, before C close | **two** active legs; both carry the field | **Discriminator (I-R4): keep the leg whose `switchFromRunID != its own id`** (new leg — points at the source), close the leg whose `switchFromRunID == its own id` (old source), append record |
  | after C close, before record | new active, old closed, no record | append record idempotently (derived from leg fields) |
  | after record, seed fails mid-turn | new active, seed turn failed | leg stays active; typed `switch_seed_failed` on the leg; chat continuable |

  Tests: `TestSwitchCrashHealsOnLoad` (closed-no-record), `TestSwitchCrashTwoActiveHeals`, `TestSwitchSeedFailTypedContinuable`, `TestSwitchNeverHoldsLockAcrossCreateRun`. Detached chats (post-restore, zero active legs) never reach phase A — the guard returns `409 chat_no_active_leg` and clients reattach via plain `createRun(chatId, switchFromRunID=latestLeg)` (`SD26` §10 / Task-312 `T-8`; Task-315/316 own the predicate).
- `T-3` `apps/local-runner/internal/runner/handoff_context.go` — chat-scope builder + budget + digest.
  Current: `buildHandoffContext` is run-scoped (`:59`), budget fixed `handoffMaxBytes` (`:15`), text-only envelope (`renderHandoffPrompt :281`). Delta — new functions, existing ones untouched:
  ```go
  // buildChatHandoffContext — joins ALL legs of the chat from the
  // transcript store (Task-313), legacy fallback to transcriptTurnsFromRun
  // (:156) for un-backfilled chats. Empty history → HandoffMode:"fresh_start",
  // Prompt:"" (no envelope, no error — CP-59 CS-07).
  // NO lock suffix: runs in phase B (unlocked) — chat-store I/O never under s.mu.
  func (s *InteractiveService) buildChatHandoffContext(chatID string, req chatSwitchRequest) (chatEnvelope, *apiErr)

  // chatHandoffBudget — target-context-derived (Key Decision T-2)
  func chatHandoffBudget(models []ProviderModel, targetModel string) int {
      for i := range models {
          if strings.EqualFold(models[i].ModelID(), targetModel) && models[i].ContextWindowTokens > 0 {
              b := int(models[i].ContextWindowTokens) * 3 // ≈chars at ~4 chars/token, conservative
              if b > chatHandoffHardCapBytes { b = chatHandoffHardCapBytes } // 512 KiB
              if b > handoffMaxBytes { return b }
              return handoffMaxBytes
          }
      }
      return handoffMaxBytes // unknown context → Task-078 floor
  }

  // renderActionsDigest — <actions_summary> from chat records (Key Decision T-3),
  // turn-scoped lines: "turn 3: bash `npm test` (ok) · edit internal/foo.go (+42/−3) · approval: user approved bash"
  func renderActionsDigest(records []ChatTranscriptRecord, maxBytes int) string
  ```
  Envelope assembly reuses `packConversationTurns` (`:307`) + `renderHandoffPrompt` (`:281`) for the text body, appends `\n\n<actions_summary>\n…\n</actions_summary>` when digest non-empty. Tests: `TestChatHandoffBudgetFromContextWindow`, `TestChatHandoffBudgetFloorWhenUnknown`, `TestChatEnvelopeJoinsAllLegs`, `TestChatEnvelopeFreshStartNoError`.
- `T-4` `chat_switch.go` — seed dispatch + seed marker + divider single-source.
  Current: turn send lives in the runTurn path; `handoffPromptPrefix` (`:22`) marks handoff turns for feature-resolution skip. Delta: seed prompt = `env.Prompt` (already prefix-stamped by `renderHandoffPrompt`); dispatch through the same send-turn entry `createRun`'s first prompt uses; seed `turn_started` payload carries the full stats block — `isHandoffSeed:true, carriedTurnCount, omittedTurnCount, handoffMode` — so UIs render the **seed itself as the one divider** (Task-312 `T-5`; no client-synthesized divider line anywhere); `chat_provider_switch` record (SD26-E-9) appended exactly once (guard: unique `(chatId, fromRunId, toRunId)`) and is the replay-side divider source. Tests: `TestSwitchSeedsEnvelopeServerSide`, `TestSwitchSeedTurnCarriesStats`, `TestSwitchEventAppendedOnce`.
- `T-5` Typed provider availability: target provider not registered/available → `422 provider_unavailable` with install hint string (SD26-X-5); enforced before any leg mutation. Tests: `TestSwitchTargetProviderUnavailableTyped` (CS-14 runner half).
- `T-6` Matrix + guard tests (fake adapters registered in test registry; real-provider matrix runs live in Task-315/316 manual walks):
  ```go
  // TestSwitchMatrixAllDirectedPairs — table over
  // {codex,claude,grok,opencode}² off-diagonal (12 pairs), ≥3 seeded turns:
  //   → new leg on target, envelope text contains all 3 turns,
  //     chat_provider_switch once, old leg closed(provider_switch).
  // TestSwitchBusyDuringTurn / DuringApproval / DuringQuestion → 409 handoff_run_busy, no leg mutation.
  // TestSwitchSameProvider409 → no new leg.
  // TestSwitchNonChatKindRejected → workflow_kind run → 409 handoff_run_kind_unsupported.
  // TestSwitchRecordCapturesApprovalsAndTools → digest lists approval decision + tool lines from prior leg.
  ```
- `T-7` BUG-330 repro lock at runner level (fake opencode+grok adapters): pins `scan=opencode/muse-spark`, `code=opencode/deepseek`, `plan=grok-4.5` (bare) → the switch (invoked with `TargetProviderKey:grok, Model:"grok-4.5"`) produces a `provider=grok` leg seeded with the full envelope; zero `model not found` anywhere; the opencode adapter never receives a turn requesting `model=grok-4.5`. Test: `TestBug330SwitchMintsRealGrokLeg`.
- `T-8` Doc ownership riding this task (review I-6): amend `SS-05` (provider-selection invariant gains the chat-switch note) and `SD-06` §3.2 (posture/chat provider contract pointer to SD-26) in the same merge as the endpoint; update `BUG-330` (status → fixed-by-CP-59, `D-3`/`D-4` re-pointed from the Task-078 2-step to this endpoint) when Task-315 lands the TUI surface. Gemini matrix note: gemini-as-source rows assert the typed unsupported error (`Q-5`), never success; gemini-as-target rows run the normal matrix.

## 5. Touched Areas

- files: new `chat_switch.go` (+ `_test.go`); edits `handoff_context.go` (new chat-scope funcs + budget + digest — existing funcs untouched), `interactive_handlers.go` (route), `interactive_resume.go` (crash-heal hook), `interactive_service.go` (`activeChatLegLocked`, `nextChatSeqLocked` exposure)
- modules: runner interactive service; handoff subsystem
- routes: `POST /client/chats/{chatId}/switch-provider` (new)
- tables: none new (writes Task-313's stores)

## 6. Acceptance Check

- All 12 directed pairs pass on fake adapters with envelope text covering prior turns of every leg; busy/same-provider/kind/availability guards return the exact SD26-X codes with no leg mutation; seed turn streams on the new leg and is marked `isHandoffSeed`; crash-heal rollback proven; BUG-330 lock green.
- Task-078 endpoint suite untouched and green; `supportsHandoffSource` legacy behavior unchanged.

### 6.1 Definition of Done (DOD)

- [ ] `DOD-1` Route + DTOs live behind flag; off = 404 (`TestSwitchRouteFlagGated`).
- [ ] `DOD-2` Full directed-pair matrix green on fake adapters (`TestSwitchMatrixAllDirectedPairs`) — 12 pairs, envelope completeness asserted.
- [ ] `DOD-3` Guards exact: busy (turn/approval/question), same-provider, non-chat kind, unknown chat, provider unavailable — each its own test, no partial leg mutation on any refusal.
- [ ] `DOD-4` Envelope budget: context-derived, hard-capped, floored (`TestChatHandoffBudget*`); truncation ladder modes surface in `chatSwitchHandoffStats`.
- [ ] `DOD-5` Actions digest compiled from records and included per budget (`TestSwitchRecordCapturesApprovalsAndTools`, digest-overflow truncation test).
- [ ] `DOD-6` Seed server-side, prefix-stamped, stats embedded (`TestSwitchSeedsEnvelopeServerSide`, `TestSwitchSeedTurnCarriesStats`); divider single-source: seed is the only live divider (`TestSwitchSeedDividerSingleSource`).
- [ ] `DOD-7` `chat_provider_switch` appended exactly once; legs closed/opened with correct fields (`TestSwitchEventAppendedOnce`).
- [ ] `DOD-8` Crash-heal matrix implemented, all cells: intent-no-new-leg cleared, two-active resolved, closed-no-record appended, seed-failed typed + continuable (`TestSwitchCrashHealsOnLoad`, `TestSwitchCrashTwoActiveHeals`, `TestSwitchSeedFailTypedContinuable`).
- [ ] `DOD-9` Fresh-start switch (empty chat) returns `fresh_start`, no envelope, no error (`TestChatEnvelopeFreshStartNoError` — CS-07).
- [ ] `DOD-10` BUG-330 runner lock green (`TestBug330SwitchMintsRealGrokLeg`); Task-078 suite untouched green.
- [ ] `DOD-11` Lock rule proven: no `s.mu` hold across `createRun` (`TestSwitchNeverHoldsLockAcrossCreateRun` — e.g. lock-order probe with an instrumented mutex or goroutine-based deadlock watchdog).
- [ ] `DOD-12` Docs land with the endpoint: SS-05 chat-switch invariant note, SD-06 §3.2 pointer to SD-26; BUG-330 re-pointed when Task-315 lands (`T-8`).

### 6.2 Test Signatures

```go
// chat_switch_test.go
func TestSwitchRouteFlagGated(t *testing.T)                          // flag off → POST returns 404; on → 200 path
func TestSwitchMatrixAllDirectedPairs(t *testing.T)                  // 12 off-diagonal pairs × ≥3 seeded turns → target leg, envelope contains turns of all legs, single switch record, source leg closed(provider_switch)
func TestSwitchBusyDuringTurn(t *testing.T)                          // turnInFlight → 409 handoff_run_busy; leg fields untouched
func TestSwitchBusyDuringApproval(t *testing.T)                      // pendingApprovalID → 409
func TestSwitchBusyDuringQuestion(t *testing.T)                      // pendingQuestionID → 409
func TestSwitchSameProvider409(t *testing.T)                         // opencode→opencode → 409 handoff_same_provider; zero new legs
func TestSwitchNonChatKindRejected(t *testing.T)                     // runKind=workflow_step → 409 handoff_run_kind_unsupported
func TestSwitchTargetProviderUnavailableTyped(t *testing.T)          // unregistered provider → 422 provider_unavailable + install hint; no mutation
func TestSwitchUnknownChat404(t *testing.T)                          // 404 chat_not_found
func TestSwitchSeedsEnvelopeServerSide(t *testing.T)                 // new leg's first turn prompt has handoffPromptPrefix; client sent no prompt
func TestSwitchSeedTurnCarriesStats(t *testing.T)                    // seed turn_started payload: isHandoffSeed:true + carriedTurnCount/omittedTurnCount/handoffMode
func TestSwitchSeedDividerSingleSource(t *testing.T)                 // live path renders seed as the only divider; replay path renders switch record; no duplicate
func TestSwitchEventAppendedOnce(t *testing.T)                       // double POST racing → one chat_provider_switch record, one new leg
func TestSwitchCrashHealsOnLoad(t *testing.T)                        // crash after C close, before record → record appended idempotently on load, one active leg
func TestSwitchCrashTwoActiveHeals(t *testing.T)                     // crash after B createRun, before C close → new leg (owns switchFromRunID) kept, old closed provider_switch, record appended
func TestSwitchCrashIntentNoNewLegHeals(t *testing.T)                // crash after A, before B → intent cleared, source leg stays active, switch retryable
func TestSwitchSeedFailTypedContinuable(t *testing.T)                // seed turn fails after commit → leg stays active, typed switch_seed_failed, chat accepts next turn
func TestSwitchNeverHoldsLockAcrossCreateRun(t *testing.T)           // lock-order probe: createRun's s.mu acquisition during phase B never self-deadlocks (instrumented mutex or watchdog)
func TestSwitchRecordCapturesApprovalsAndTools(t *testing.T)         // prior leg had approval approve + 2 tool calls → digest lines present in envelope
func TestChatHandoffBudgetFromContextWindow(t *testing.T)            // 200k-token target → 512KiB hard cap applied; 8k-token target → 24KiB
func TestChatHandoffBudgetFloorWhenUnknown(t *testing.T)             // no ContextWindowTokens → 64KiB handoffMaxBytes
func TestChatEnvelopeJoinsAllLegs(t *testing.T)                      // 3 legs → <previous_conversation> has turns from leg 1..3 in chatSeq order
func TestChatEnvelopeFreshStartNoError(t *testing.T)                 // empty chat → HandoffMode "fresh_start", Prompt "", 200
func TestBug330SwitchMintsRealGrokLeg(t *testing.T)                  // BUG-330 pin set → grok leg seeded; opencode adapter never sees model=grok-4.5; no "model not found"
```

## 7. Out of Scope

- Any UI (TUI Task-315, Desktop Task-316); posture pin derivation (TUI side); sync/restore (Task-317); `/model`/`/provider` TUI routing; recall tool; gemini source enablement (matrix row stays typed-unsupported until extractor proven — `Q-5`).
- Changing Task-078's run-scoped endpoint or `supportsHandoffSource` legacy semantics.

## 8. Completion Notes

- result:
- follow-ups:
- upstream docs updated:
