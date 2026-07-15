# Task-235: Hub Notify Node Behavior — Reinvoke Hub Session For AI-Composed Notification (No Child Agent)

## Metadata

- Document ID: `Task-235`
- Title: `Hub Notify Node Behavior — Reinvoke Hub Session For AI-Composed Notification (No Child Agent)`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-15`
- Last Updated: `2026-07-15`
- Parent Documents: [CP-42: Flow Pack And Generic Node Behavior Refactor](../../07-Coding-Plan/done/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md) (`P-3`, `P-5`), [SD-19: Agent Flow Engine](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md) (`D-6`, `D-7`), [SS-16: Agent Flow Engine](../../05-System-Specs/SS-16-Agent-Flow-Engine.md)
- Child Documents: `None`
- Related Documents: [Task-233: Telegram Output Write-Contract, Verify Gate, And Approval](./Task-233-Telegram-Output-Write-Contract-Verify-Gate-And-Approval.md) (source of the `appendTelegramOutputPrompt` write-contract this task reuses), [CP-05-05: Telegram MCP As An Output Notification Artifact](../../07-Coding-Plan/done/CP-05-05-Tele-Mcp.md), [Task-236: telegram.notify — Deterministic (No-Agent) Telegram Send Node Behavior](./Task-236-Telegram-Notify-Deterministic-Inline-Node-Behavior.md) (the Go-only sibling behavior implemented earlier in this session; backfilled after this task), [Task-237: Generalize Post-Node "Done" Edge-Walking (Audit And Hub-Inline Successor Chaining)](./Task-237-Generalize-Post-Node-Done-Edge-Walking-Audit-And-Hub-Inline-Successor-Chaining.md) (the `advanceHubDoneThroughEdge`/single-hub-node foundation this task's `activeHubNodeID` tracking generalizes further; backfilled after this task)
- Replaces: `None`
- Tags: `agent-flow-engine, node-behavior, hub-reinvoke, telegram, notification, flow-pack`

## AI Quick View

### Summary

- New inline-scope node behavior `hub.notify` (sibling of `hub.inline`): a flow node that runs as **another turn of the SAME hub session** — no child agent spawned — so the AI already driving the flow can freely compose a notification (e.g. fill a `telegram.v1` OUTPUT artifact's Message Template) instead of either a fixed/templateless Go-only send (`telegram.notify`) or spawning a brand-new child agent (`agent.delegate`) for one short turn.
- Generalizes `advanceHubDoneThroughEdge` (added earlier this same session) to track WHICH hub-driven node is currently active (`interactiveRun.activeHubNodeID`), so a **second** hub-turn node (`hub.notify` reached after `synthesis`) resolves its own "done" against its own forward edge instead of re-resolving against the flow's first `hub.inline` node and looping forever.
- Reuses the existing hub-reinvoke transport (`scheduleChildTurn`) via a new sibling `maybeAutoReinvokeHubWithPrompt` — a deliberate, additive duplicate of `maybeAutoReinvokeHubWithNote`'s guard logic, not a refactor of that load-bearing, BUG-234/BUG-275-tagged function, since GitNexus impact analysis was unavailable this session to safely verify a shared-function edit's blast radius.
- Composes its reinvoke prompt via the existing, already-softened (Task-233-adjacent) `appendTelegramOutputPrompt`, so it inherits the same reduced-injection-refusal wording instead of a third, divergent Telegram write-contract text.

### Current Ask

- Add `hub.notify` end-to-end (constants, registry, alias, dispatch from both existing inline-behavior entry points, desktop authoring dropdown, doc registry) and prove `synthesis --done--> notify(hub.notify) --done--> done` reinvokes the hub exactly once and settles correctly on `notify`'s own terminal edge.

### Key Decisions

- `T-1` `hub.notify` is `BehaviorScopeInline`, dispatched exactly like `hub.inline`/`telegram.notify` — no new scope needed; fits SD-19 `D-7`'s existing `FlowNode.run: inline|delegate` vocabulary as a `run: inline` instance.
- `T-2` New `interactiveRun.activeHubNodeID` field tracks "the currently active hub-driven node". `advanceHubDoneThroughEdge` uses it when set, falling back to `hubInlineNodeID(nodes)` (today's behavior) when empty — every existing flow's FIRST hub "done" is unaffected.
- `T-3` New sibling `maybeAutoReinvokeHubWithPrompt` duplicates `maybeAutoReinvokeHubWithNote`'s single-flight/cap/status guard rather than refactoring it — zero risk to the existing, heavily-tested cohort-join reinvoke path.
- `T-4` `hub.notify` is reachable from BOTH existing inline-behavior dispatch entry points — `advanceHubDoneThroughEdge` (reached from a hub-driven node's own "done") and `advanceToNextInlineOrDelegate` / `tryAdvanceFlowThroughInline` (reached from any other inline/delegate node's forward edge) — via one shared `dispatchHubNotifyNode` helper, so it behaves identically regardless of graph position.
- `T-5` No flow-gate write-contract verification (`r-artifact-telegram-sent`-style rule) is wired for the hub.notify path in this slice — deliberately out of scope (§7).

### Constraints

- Do not refactor `maybeAutoReinvokeHubWithNote` / `applyFlowControl` core logic — additive-only (new sibling function, new branch).
- GitNexus MCP tools were unavailable in this session's thread (confirmed via `ToolSearch`, no `gitnexus_*` tools resolved) — every symbol edit proceeded via careful manual inspection instead of automated impact analysis, per the skill's own fallback instruction.
- No DB migration for this slice — `activeHubNodeID` is in-memory only on `interactiveRun`, not persisted.
- `go test -race` could not run in this environment (CGO disabled) — not claimed as verified.

### Open Questions

- `Q-1` `activeHubNodeID` is not persisted across a runner restart (`ProviderSessionState` snapshot). Resuming a run mid-`hub.notify`-turn falls back to `hubInlineNodeID` (the flow's first `hub.inline` node), which is wrong for that one edge case. Accepted as a known limitation for this slice; needs its own task if it becomes a real-world occurrence.
- `Q-2` No flow-gate verification exists yet for a hub.notify send (unlike `telegram.notify`, which Go verifies itself via `message_id`). If a hard guarantee "the hub actually sent it" is required, a gate rule analogous to Task-233's `r-artifact-telegram-sent` — but scoped to the hub's own turn rather than a child's — would need to be added.
- `Q-3` `composeHubNotifyPrompt` currently only has write-contract content for a `telegram.v1` OUTPUT binding (via `appendTelegramOutputPrompt`). Not yet generalized to any other future OUTPUT artifact type.

### Source Refs

- `CP-42` `P-3`, `P-5`; `SD-19` `D-6`, `D-7`; `SS-16`.
- `Task-233` (source of the `appendTelegramOutputPrompt` write-contract wording this task reuses unmodified).
- Code anchors: `behavior_registry.go`, `behavior_registry_builtin.go`, `pack.go` (`NormalizeBehaviorID` alias table), `flow_validate_audit_dispatch.go` (`dispatchHubNotifyNode` call sites, `composeHubNotifyPrompt`, `tryAdvanceFlowThroughInline`/`advanceToNextInlineOrDelegate` switch cases), `interactive_service.go` (`activeHubNodeID` field, `advanceHubDoneThroughEdge`, `dispatchHubNotifyNode`, `maybeAutoReinvokeHubWithPrompt`), `adminModels.ts` (`FLOW_BEHAVIOR_OPTIONS`), `flow-pack/behaviors/registry.yaml` (doc entries).

## 1. Goal

Let a flow author place a `hub.notify` node in the graph so that, when reached, the engine reinvokes the SAME hub session already running the flow (no new child agent) for one more turn — letting the AI itself compose and send a notification (e.g. fill a `telegram.v1` OUTPUT artifact's Message Template) — and then correctly resolves that node's own forward edge once the hub reports done.

## 2. Parent Links

- coding plan: [CP-42: Flow Pack And Generic Node Behavior Refactor](../../07-Coding-Plan/done/CP-42-Flow-Pack-And-Generic-Node-Behavior-Refactor.md) — `P-3` (behavior selected by node definition, not hardcoded step names), `P-5` (declared tool faces map domain statuses onto generic `flow_control`)
- tech design: [SD-19: Agent Flow Engine](../../06-System-Tech-Design/SD-19-Agent-Flow-Engine.md) — `D-6` (the decision to spawn/act lives in the main agent, not the engine), `D-7` (`FlowNode{run: inline|delegate}` vocabulary; `hub.notify` is a new `run: inline` instance)
- system spec: [SS-16: Agent Flow Engine](../../05-System-Specs/SS-16-Agent-Flow-Engine.md)
- specific upstream ids: `CP-42 P-3`, `P-5`; `SD-19 D-6`, `D-7`

## 3. Trigger

Earlier this same session, a `context-coding-review-synthesis` flow was extended with `synthesis --done--> notify --done--> done`, where `notify` sends a Telegram message with a Message Template (e.g. containing `{{status}}` and a free-text summary placeholder). Two existing paths did not satisfy this: `telegram.notify` (Go-only, no AI, cannot fill template free-text/variables) and `agent.delegate` (spawns a brand-new child agent/session just to compose and send one short message). The user explicitly asked for a third path — a dedicated node that still runs as the next turn of the already-running hub session, reusing the reinvoke mechanism `hub.inline`/`synthesis` already uses, rather than a new agent spawn.

## 4. Exact Change

- `T-1` Add constant `BehaviorHubNotify = "hub.notify"` (`behavior_registry.go`) and register it as `BehaviorScopeInline` with a thin stub handler `behaviorHubNotify` (`behavior_registry_builtin.go`) — the real work lives in the dispatch layer, not the registry handler (same shape as `telegram.notify`).
- `T-2` Add aliases `hub.notify` / `notify.hub` to `agentpack.NormalizeBehaviorID` (`pack.go`).
- `T-3` Add field `interactiveRun.activeHubNodeID string` (`interactive_service.go`) — tracks which hub-driven node is currently awaiting its own "done".
- `T-4` Change `advanceHubDoneThroughEdge`: resolve the hub node id from `activeHubNodeID` (falling back to `hubInlineNodeID(nodes)` when empty); if the resolved forward "done" edge's target has behavior `hub.notify`, call `dispatchHubNotifyNode` (sets `activeHubNodeID` to the target and reinvokes the hub) instead of `advanceToNextInlineOrDelegate`; if the target is a terminal, clear `activeHubNodeID` back to `""` before returning `false` (so the next "done" call falls back correctly again).
- `T-5` Add `dispatchHubNotifyNode(parentRunID, node)` (`interactive_service.go`) — the single dispatch entry point for `hub.notify`, shared by `advanceHubDoneThroughEdge` and `advanceToNextInlineOrDelegate` / `tryAdvanceFlowThroughInline` (`flow_validate_audit_dispatch.go`) — added a `"hub.notify"` case to both of those existing switches.
- `T-6` Add `maybeAutoReinvokeHubWithPrompt(parentRunID, prompt)` — sibling of `maybeAutoReinvokeHubWithNote`, duplicating its guard logic (autoOrchestrate / reinvokeInFlight / turnInFlight / loop-status / round-cap) verbatim but accepting an arbitrary prompt instead of building one from `cohortNote + autoReinvokePromptText()` — avoids editing the original function.
- `T-7` Add `composeHubNotifyPrompt(node)` (`flow_validate_audit_dispatch.go`) — reuses `appendTelegramOutputPrompt` (already softened this session against injection-pattern false-positive refusals) plus a generic instruction to call the flow's control tool with `status="done"` when finished.
- `T-8` Add `hub.notify` to `FLOW_BEHAVIOR_OPTIONS` (`adminModels.ts`, `requiresAgent: false`) — now selectable in the Settings → Workflows → Steps Behavior ID dropdown.
- `T-9` Add doc entries for `hub.notify` (and backfill the missing `telegram.notify` entry) to `flow-pack/behaviors/registry.yaml` (reference-only, not runtime-authoritative per the file's own header).

## 5. Touched Areas

- files: `apps/local-runner/internal/runner/behavior_registry.go`, `behavior_registry_builtin.go`, `behavior_registry_test.go`, `interactive_service.go`, `flow_validate_audit_dispatch.go`; `apps/local-runner/internal/agentpack/pack.go`; `apps/local-runner/internal/agentpack/flow-pack/behaviors/registry.yaml`; `packages/flowpilot-client-core/src/domain/adminModels.ts`; new test `apps/local-runner/internal/runner/flow_hub_notify_test.go`
- modules: flow-engine behavior registry, flow executor dispatch, hub-reinvoke transport, desktop step-authoring UI
- routes: none
- tables: none (`activeHubNodeID` is in-memory only)

## 6. Acceptance Check

- `synthesis --done--> notify(hub.notify) --done--> done`: calling `advanceHubDoneThroughEdge` with `status="done"` returns `handled=true`, does NOT settle the flow, sets `activeHubNodeID="notify"`, and schedules the hub reinvoke (`reinvokeInFlight=true`).
- Calling it again with `activeHubNodeID="notify"` already set and `notify --done--> done` (terminal) returns `handled=false` (so `applyFlowControl` settles normally) and clears `activeHubNodeID` back to `""`.
- `coder(agent.delegate) --done--> notify(hub.notify)` (reached via `tryAdvanceFlowThroughInline`, not through `synthesis`) dispatches identically — sets `activeHubNodeID`, schedules the reinvoke.
- Existing built-in flows (`review-loop`, `context-coding-review-synthesis`) are unaffected: `synthesis --done--> done` still resolves straight to the terminal, so `advanceHubDoneThroughEdge` returns `false` exactly as it did before this task.
- `composeHubNotifyPrompt`: with a `telegram.v1` OUTPUT binding, the prompt contains the write-contract section, the Message Template verbatim, and the "call the flow control tool" instruction; without a binding, only the control-tool instruction (no Telegram section).
- `go build ./...`, `go vet ./internal/runner ./internal/agentpack` clean; `npx tsc --noEmit` (desktop) clean.
- `go test ./internal/runner -run 'HubNotify|ComposeHubNotify|ResolvesSecondHubNode'` — 5/5 pass (new tests).
- `go test ./internal/runner -run 'Flow|Review|Orchestrat|Cohort|Hub|Reinvoke|Telegram|Audit|Validate|Behavior'` — 395/395 pass (broad regression suite, no existing path broken).

## 7. Out of Scope

- Flow-gate verification (an `r-artifact-telegram-sent`-style rule) for the hub's own send — that guarantee currently exists only for `telegram.notify` (Go verifies `message_id` itself) and the `agent.delegate` + MCP path (Task-233). Would need its own task if a hard guarantee is required for `hub.notify`.
- Persisting `activeHubNodeID` across a runner restart/resume — known limitation, not solved in this slice (`Q-1`).
- Template variable substitution (`{{status}}`, etc.) — still sent verbatim; the AI is expected to compose around it (this IS the reason `hub.notify` exists, unlike `telegram.notify`), but there is no Go-side mechanism forcing correct substitution.
- Refactoring `maybeAutoReinvokeHubWithNote` / `applyFlowControl` core logic — only additive sibling functions and new switch branches were added.
- Modifying the built-in pack YAML (`context-coding-review-synthesis.yaml`) to seed a `notify` node by default — stays a user-wired addition in Settings → Workflows, matching Task-233 `T-5`'s "manual workflow wiring is the supported path" decision.

## 8. Completion Notes

- result: done. `T-1`–`T-9` all implemented; `go build`/`go vet`/`tsc --noEmit` clean; 5 new tests + a 395-test regression sweep across flow/review/orchestrator/telegram/audit/validate/behavior all pass, verified against the same suite run before this change.
- GitNexus MCP tools were unavailable in this session (confirmed via `ToolSearch`, no `gitnexus_*` tool resolved) — proceeded via careful manual inspection of every touched symbol instead of automated impact analysis, per the skill's fallback instruction.
- `go test -race` could not run in this environment (CGO disabled) — not claimed as verified; every read/write of `activeHubNodeID` was manually reviewed to confirm it always occurs inside `s.mu.Lock()/Unlock()`.
- follow-ups: (1) `Q-1`/`Q-2`/`Q-3` above; (2) the `telegram.notify` behavior and `advanceHubDoneThroughEdge` generalization this task builds on were implemented earlier in this same session and lacked their own Task documents at the time this file was written — since backfilled as [Task-236](./Task-236-Telegram-Notify-Deterministic-Inline-Node-Behavior.md) and [Task-237](./Task-237-Generalize-Post-Node-Done-Edge-Walking-Audit-And-Hub-Inline-Successor-Chaining.md).
- upstream docs updated: none — `hub.notify` is a new instance within the `run: inline` vocabulary SD-19 `D-7` already defines; it does not contradict or change upstream meaning.
