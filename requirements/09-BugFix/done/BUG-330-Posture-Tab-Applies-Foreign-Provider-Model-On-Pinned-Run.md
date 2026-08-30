# BUG-330: Posture Tab applies foreign provider model on pinned run — plan pins grok-4.5 onto an OpenCode session

## Metadata

- Document ID: `BUG-330`
- Title: `Posture Tab applies foreign provider model on pinned run — plan pins grok-4.5 onto an OpenCode session`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-08-29`
- Last Updated: `2026-08-29` (closed — resolution follows CP-59; this doc stays as the forensic record)
- Parent Documents: [CP-57: Opencode Provider Integration](../../07-Coding-Plan/inprogress/CP-57-Opencode-Provider-Integration.md), [SD-06: AI Provider Integration](../../06-System-Tech-Design/SD-06-AI-Provider-Integration.md), [SS-05: Workflow AI Provider](../../05-System-Specs/SS-05-Workflow-Ai-Provider.md), [Task-078: Cross-Provider Chat Handoff](../../08-Task/done/Task-078-Cross-Provider-Chat-Handoff.md)
- Child Documents: `none`
- Related Documents: [CP-59: Chat SSOT — Continuous Cross-Provider Chat](../../07-Coding-Plan/todo/CP-59-Chat-Ssot-Continuous-Cross-Provider-Chat.md) (resolution — Task-314/315), [BUG-329](BUG-329-Opencode-Midchat-Model-Switch-Session-Load-No-SessionId.md) (same provider, different class), [CA-679](../../../change-audit/CA-679-opencode-config-file-env-and-tui-model-restore.md), [CA-659](../../../change-audit/CA-659-opencode-local-share-auth-discovery.md), run-314536 (`~/Library/Application Support/FlowPilot/logs/runner.log` + `~/.flowpilot/tui.log`), run-307050, `handoff_context.go`, `providerKeyFromModel`, `chat_posture.go`
- Replaces: `none`
- Tags: `tui, chat-posture, scan-plan-code, provider-switch, cross-provider, severity-high`

## AI Quick View

### Summary

- Live `run-314536` (gate-sandbox, `flowpilot chat` TUI): `scan=opencode/muse-spark`, `code=opencode/deepseek-v4-flash`, **`plan=grok-4.5` (no provider)**. First turn on scan → `opencode-go/muse-spark` OK. Tab `scan→code` → `opencode-go/deepseek-v4-flash` OK (same provider, `session/load` + `set_config`). Tab to **plan** while run is still `provider=opencode` → next 2 turns sent as `provider=opencode model=grok-4.5`; ACP `session/set_config_option model=grok-4.5` failed `model not found: grok-4.5`, reply stayed **Muse Spark** while footer showed `grok-4.5`.
- A run pins `rs.providerKey` at `startRun`. `providerKeyFromModel("grok-4.5")==grok` but that mapping only runs at `startRun`/`spawnChildRun`; a mid-run `Tab` (`applyChatPostureProfile`) swaps `m.model` without switching `m.provider` correctly (profile thiếu `provider`) and does **not** handoff — `runTurn` still builds `TurnRequest` with `provider=opencode`.
- Correct behavior: cross-provider Tab must not push a foreign model into the current adapter. It must create a **new provider run** carrying the old transcript (same as Desktop `confirmProviderSwitch` handoff), or block and require `/new`. Same-provider Tab (scan↔code, both opencode) keeps the same `ses_*` and just `set_config`.

### Current Ask

- **CLOSED 2026-08-29 — resolution follows [CP-59: Chat SSOT — Continuous Cross-Provider Chat](../../07-Coding-Plan/todo/CP-59-Chat-Ssot-Continuous-Cross-Provider-Chat.md).** The cross-provider posture Tab becomes a chat-level provider switch: Task-314 (runner switch endpoint + chat envelope) removes the foreign-model-injection path this bug describes, Task-315 wires the TUI surface. Repro is locked as `TestBug330SwitchMintsRealGrokLeg` (Task-314 DOD-10) plus the TUI walk (Task-315 DOD-1); the bare-model pin hole closes via Task-315 `T-2` (derive provider once + persist).
- This doc remains the forensic record (run-314536). Its `F-3`/`D-3`/`D-4` Task-078 two-step handoff wording is superseded by CP-59's chat-level switch — Task-314 `T-8` owns re-pointing those decisions when the endpoint lands.

### Key Decisions

- `D-1` Posture profiles are **provider + model** pairs, not model-only. A profile with `provider=""` + `model=grok-4.5` must be treated as `provider=grok` (derived via `providerKeyFromModel`) — same as Desktop `pickDefaultModel` rule — not as opencode.
- `D-2` Tab across **same provider** → in-place `session/load` + `set_config model` on the existing session (already works for opencode↔opencode; Grok↔Grok already works via `session/set_model`). No new run.
- `D-3` Tab across **different providers** (opencode↔grok, any→claude/codex) → **handoff**: `POST /handoff-context` on the pinned source run, `startRun(targetProvider)`, then send `handoff.prompt` as first turn on the new run. Source run stays in history (Task-078 T-1).
- `D-4` TUI Tab is the analogue of Desktop's provider-chip switch. Desktop gates with `pendingProviderSwitch` + `providerSwitchLoading` overlay and `handoffContext` reuse. TUI must reuse the same runner endpoint, not invent a new transcript source.
- `D-5` No change to `opencode acp` `mode` (`build`/`plan`) in this bug — `mode` is the opencode-internal build/plan dichotomy (same session, different agent). This bug's **plan** is a FlowPilot posture whose pin happened to be a foreign provider. Mapping `FlowPilot plan/scan → opencode plan` vs `code → build` is a separate hardening item, not the `grok-4.5→grok` confusion.
- `D-6` No new handoff size/privacy semantics — reuse `handoff_context.go` budget (~64 KiB raw, `[Earlier conversation omitted…]`), `</previous_conversation>` escape, summary hybrid (Task-078).

### Constraints

- additive-tests-only: new tests only, do not edit pre-existing suites without approval.
- `rs.providerKey` is authoritative for the lifetime of a run (Task-078 T-1, `interactive_service.go:rs.providerKey`). Do not migrate a live provider session.
- `isRecoverableSendError` / `sendTurnWithRetry` are not the path — the `session/set_config` 404 is degraded (turn still completes as Muse Spark), not a retryable error.
- Parity: fix must work for `flowpilot chat` (TUI) **and** Desktop posture Tab, same runner contract.

### Open Questions

- `Q-1` Tab cross-provider UX: **auto-handoff** on Tab (closest to opencode's instant Tab feel, needs a `providerSwitchLoading` guard in TUI + an info system message) vs **confirm gate** like Desktop chips (`pendingProviderSwitch` modal, no auto-creation). Auto-handoff is preferred for speed but should be explicit in code review.
- `Q-2` Should a profile with bare `grok-4.5` be implicitly promoted to `provider=grok` every time, or should the posture editor force `provider` non-empty so bare-model pins become invalid configs? Implicit via `providerKeyFromModel` matches Desktop but hides misconfig.
- `Q-3` Do we also block `/model grok-4.5` mid-run the same way (today it **does** derive provider via `providerForModel` then still runs on `rs.providerKey=opencode`, so same 404)? `/provider <key>` already blocks mid-run — `/model` should follow the same rule when provider switches.
- `Q-4` For a brand-new run (no transcript), handoff must return `handoff_context_unavailable` and the Tab should just start fresh — confirm that `buildHandoffContext` path is exercised and not treated as a failure to Tab.

### Source Refs

- `run-314536` forensic — `~/Library/Application Support/FlowPilot/logs/runner.log`:
  - `turn-314538 scan provider=opencode model=opencode-go/muse-spark-1.2-contributor` → `session/new` → `ses_fb3373262ffe5bD9WLtmLz0Kb3` → Muse Spark.
  - `turn-314548 code provider=opencode model=opencode-go/deepseek-v4-flash` → `session/load(ses_fb3…)` → `set_config model=deepseek` OK → deepseek.
  - `turn-314564 scan provider=opencode model=opencode-go/muse-spark-1.2-contributor` → OK.
  - `turn-314576 plan provider=opencode model=grok-4.5` → `set_config model=grok-4.5` → `model not found: grok-4.5` → `session/set_config` 404 → still Muse Spark. Footer showed `opencode · grok-4.5 (Google · OpenCode Go · xAI)`.
  - `turn-314590 plan provider=opencode model=grok-4.5` → same 404.
- `~/.flowpilot/tui.log` `chatPostureMsg` + `KeyMsg Tab` lines: `scan={opencode/muse-spark xhigh}`, `code={opencode/deepseek medium}`, `plan={grok-4.5 high}`.
- `apps/local-runner/internal/tui/app/chat_posture.go:applyChatPostureProfile`, `setPostureProvider`, `postureModelPinWins`, `turn_stream.go:cmdSendTurn`.
- `apps/local-runner/internal/runner/handoff_context.go:buildHandoffContext`, `renderHandoffPrompt`, `packConversationTurns` (Task-078).
- `apps/desktop-flowpilot/src/state/store.ts:confirmProviderSwitch` + `apps/local-runner/internal/tui/app/app.go:/provider` block (`Cannot change provider after a run has started. Use /new`).
- `apps/local-runner/internal/runner/provider_registry.go:providerKeyFromModel`, `opencode acp` configOptions `id:"mode"` (`build`/`plan`) — noted, not this bug's fix.
- Docs: OpenCode `agents` (Tab = primary agent mode), `config` (agent model per agent), `ACP Support`.

## 1. Issue Summary

Tabbing through FlowPilot Chat Posture `scan / plan / code` is the operator's primary mode switch. Each mode pins a provider profile (provider + model). On `run-314536` the pins were `scan=opencode/muse-spark`, `code=opencode/deepseek`, `plan=grok-4.5` (bare model, no provider). `scan↔code` (both opencode) continued correctly in the same `ses_*`. Tabbing to `plan` did **not** switch to Grok — the run's pinned `provider=opencode` stayed, the next prompt was delivered to the **opencode adapter** as `model=grok-4.5`, `session/set_config_option` returned `model not found`, the turn degraded and answered as Muse Spark. The UI footprint (session panel, `Mode: plan`) claimed `grok-4.5` while the LLM was opencode.

This is not BUG-329 (opencode same-provider respawn) — it is a cross-provider posture misuse.

## 2. Parent Links

- impacted coding plan: [CP-57](../../07-Coding-Plan/inprogress/CP-57-Opencode-Provider-Integration.md) (add posture→provider honoring)
- impacted tech design: [SD-06](../../06-System-Tech-Design/SD-06-AI-Provider-Integration.md) (provider contract per run + `providerKeyFromModel`)
- impacted system spec: [SS-05](../../05-System-Specs/SS-05-Workflow-Ai-Provider.md) (provider selection invariant)
- related bugfix: [BUG-329](BUG-329-Opencode-Midchat-Model-Switch-Session-Load-No-SessionId.md) — same-provider opencode model switch, different root cause
- relevant task: [Task-078](../../08-Task/done/Task-078-Cross-Provider-Chat-Handoff.md) — canonical handoff contract

## 3. Environment and Reproduction

- environment: `flowpilot chat` TUI (opencode headless `muse-spark-1.2-contributor` as Opencode), runner `opencode acp 1.18.25`, `grok-4.5` via Grok adapter, project `gate-sandbox` (`db51ec26`), runner `127.0.0.1:4317`, posture config `GET /client/chat-posture`.
- posture on disk (tui.log): `code: {opencode/deepseek medium}`, `plan: {grok-4.5 high}`, `scan: {opencode/muse-spark xhigh}`. Plan entry has `provider=""` + bare model.
- reproduction steps:
  1. `just chat-dev <gate-sandbox>` → posture `scan` active.
  2. Prompt `hello ban la model gi` → Muse Spark (opencode).
  3. `Tab` to `code` (opencode/deepseek) → prompt `con ban la model gi` → deepseek (OK).
  4. `/scan` or `Tab` back to `scan` → prompt `vay 2 cau hoi…` → Muse Spark remembers both questions (history OK).
  5. `Tab` to `plan` (`grok-4.5`) → prompt `vay bay gio ban la model gi` → footer `grok-4.5`, reply `Mình là Muse Spark… Câu trả lời trước nói mình là deepseek-v4-flash là không chính xác — mình vẫn luôn là Muse Spark`.
  6. Prompt `khong phai la grok a?` → same 404, still Muse Spark.
- frequency: every posture Tab where the pinned profile's provider differs from the run's `providerKey`. Scan↔code same provider never fails.

## 4. Expected vs Actual

- expected: Tabbing to a mode whose pin is a different provider either (a) continues on that provider with prior transcript context (handoff into a new `grok` run), or (b) is blocked with a clear `Use /new` message like `/provider` already does. Scan↔code (same provider) keeps the same session and just changes model.
- actual: Tab succeeds visually, run stays `provider=opencode`, next turn sends a foreign `model=grok-4.5` to the opencode ACP session, `set_config` 404s, answer comes from the wrong LLM while the UI claims the requested one. No handoff, no block, no transcript carry.

## 5. Impact

- users affected: anyone using scan/plan/code with per-mode provider pins that mix opencode ↔ grok/claude/codex (original product intent — e.g. plan=Grok, scan/code=OpenCode).
- workflows affected: normal TUI chat Tab; Desktop posture Tab (same runner contract); `/model grok-4.5` mid-run is the same shape.
- severity: **high** — silent wrong provider. The model claims a different identity with high confidence (turn 4 proves flip), history is still there but the "continue on Grok" expectation is violated and the catalog confusion (`grok-4.5` vs `opencode-go/grok-4.6` vs `xai/grok-4.6`) persists.
- scope: `run-314536` had **no crash** (degraded correctly). Bulk reports of run-307050 `session/load returned no sessionId` are a different code path (BUG-329).

## 6. Root Cause

- hypothesis: `Tab` is instant like OpenCode's native `build↔plan` Tab (one ACP session, two agents `agent.build.model` / `agent.plan.model` where both models resolve inside opencode). So FlowPilot's plan should also just be an opencode-internal `mode=plan` with `xai/grok-4.6`.
- confirmed cause: FlowPilot's `plan` is **not** an opencode agent — it is a **FlowPilot provider posture** whose pin is the **Grok CLI provider** (`grok-4.5`). A run's `providerKey` is fixed at `startRun` (`interactive_service.go: rs.providerKey`). `runTurn` re-derives `(model, effort)` from `resolveTurnModelAndEffort` but the adapter is still `registry.Adapter(rs.providerKey, …)` — provider does not move. `applyChatPostureProfile` swapped `m.model` to `grok-4.5` with `providerBefore=""` (profile had no provider), so `setPostureProvider` was skipped and `m.provider` stayed `opencode`. `providerKeyFromModel("grok-4.5")==grok` is only consulted at `startRun`/`spawnChildRun`, never on posture change. The foreign model string then entered `opencodeAdapter.applyOpencodeSessionConfig → session/set_config_option` and 404'd. The opencode `mode` (`build`/`plan` per `session/new` `configOptions`) was never toggled either — FlowPilot never sends `configId:mode`.
- evidence:
  - `runner.log:1365-1368` `set_config model=grok-4.5` → `model not found: grok-4.5` + fallback `session/set_config` 404, then `session/prompt` on the old Muse Spark session.
  - `tui.log:11286` `chatPostureMsg plan:{ grok-4.5 high }` (no provider) vs `store.ts:confirmProviderSwitch` which treats provider chips as `provider=target` + handoff.
  - `chat_posture.go:applyChatPostureProfile` reads `prof.Provider` then `providerKeyFromModel` separately — mid-run provider drift is not detected.
  - `interative_handlers.go:POST /client/workflow-runs/{runId}/handoff-context` + `handoff_context.go:renderHandoffPrompt` already implement the correct cross-provider transcript carry; `/provider` already blocks mid-run but Tab does not.

## 7. Fix Strategy

`F-1` **Make posture profiles honest** — every profile stores `{provider, model}` and `Model` is scoped to its provider's catalog. Bare `grok-4.5` on an opencode profile is a misconfig. On load, derive missing provider via `providerKeyFromModel` only once and warn (persist the derived provider). Prefer fixing config over silent inference at Tab time. (`chat_posture.go`, `store.ts` `pickDefaultModel`).

`F-2` **Same-provider Tab = in-place** — if `pinnedProvider == rs.providerKey` → keep the same run/session, `session/load` + `set_config model` (opencode) or `session/set_model` (grok) as already implemented. Keep `scan↔code` exactly as it works today (run-314536 turns 2–3 prove it).

`F-3` **Cross-provider Tab = handoff, not foreign model injection** — if `pinnedProvider != rs.providerKey`:
  - Reuse Task-078 runner contract: `buildHandoffContext(sourceRunId)` → `startRun(targetProvider, targetModel=modelOrDefault)` → `sendPrompt(handoff.prompt)` as first turn on new run. Source run stays in history.
  - TUI must call `POST /handoff-context` like Desktop does (`store.ts:confirmProviderSwitch`). Add a `providerSwitchLoading` guard so rapid Tabs/Enter cannot mint duplicate `{runId}` handoff targets (Desktop uses `providerSwitchLoading` + overlay; TUI needs `runHandle != nil` guard + a TUI `handoffInFlight` flag scoped to sourceRunId).
  - Do **not** send `grok-4.5` into the opencode adapter at all — error path is eliminated.

`F-4` **Block or trap the bare-model hole** — `/model grok-4.5` mid-run and a posture `model=grok-4.5` with empty provider must follow `F-3`, not the opencode 404. Either reuse `F-3` or block with `Cannot change provider after a run has started. Use /new` (same text as `/provider` block at `app.go:3862`). Prefer `F-3` for Tab (fluent continue) and keep the block for bare `/model` unless `/model` is extended to auto-handoff.

`F-5` **Do not conflate opencode `mode`** — opencode `mode=build|plan` (one session, `agent.build.model` vs `agent.plan.model`, values like `xai/grok-4.6` **through opencode**) stays separate. If the operator actually wants plan=opencode-hosted Grok, the fix is to pin `agent.plan.model = xai/grok-4.6` in `opencode.json` and keep `scan` all opencode. This bug's `grok-4.5` is **Grok CLI** (`provider=grok`), not `xai/*` through opencode.

`F-6` **Desktop parity** — `store.ts:setChatPosture` must go through the same branch as `confirmProviderSwitch` when `profiles[posture].provider != currentRun.providerKey` and a chat run is live. Do not mint a parallel desktop-only path.

## 8. Validation

- `V-1` Repro locks green: `scan(opencode)→code(opencode)` still single `ses_*` (run-314536 turns 1–3); `code(opencode)→plan(grok)` after fix no longer `model not found` — a new `runId` `provider=grok` is created and its first prompt is the handoff envelope `<previous_conversation>` containing the 2+ prior user/assistant turns; the second prompt after handoff reaches Grok, not Muse Spark.
- `V-2` `V-1` inspected on a real `opencode acp 1.18.25` + Grok backend, not only fixture: logs show `POST /handoff-context`, `startRun provider=grok`, `session/new` (grok) then `session prompt` = handoff text, then user prompt `vay bay gio ban la model gi` on grok.
- `V-3` `/provider` mid-run still blocked (`Cannot change provider after a run has started. Use /new`). Bare `/model grok-4.5` mid-run either blocked or handoffs (decided in F-4); no `model not found` as a successful turn.
- `V-4` Map the TUI Tab RAPID case: Tab plan→code within ~200 ms before handoff 1 finishes → still exactly **one** new target run (in-flight guard), not two. Mirror `store.test.ts` handoff double-click test.
- `V-5` Empty history / first-turn Tab: `buildHandoffContext` returns `handoff_context_unavailable` → Tab just starts fresh on target provider (no handoff envelope) — not `model not found`.
- `V-6` Truncation guard: a 70 KiB transcript (≈ past 64 KiB handoff floor) pref inserts `[Earlier conversation omitted…]` and still carries the last turns after Tab to grok.

## 9. Regression Guard

- tests: **new file only**, e.g. `bug330_posture_tab_cross_provider_handoff_test.go` (additive, no old-suite edits):
  - TUI model: `scan(opencode) → Tab plan(grok)` with a mocked `GET /handoff-context` payload containing `<previous_conversation>hello</previous_conversation>` asserts `startRun` on `grok`, `sendPrompt` gets the envelope, source `runId` unchanged.
  - In-flight guard: two rapid Tab events → single `handoff-context` call.
  - Same-provider parity: `scan(opencode/muse-spark) → code(opencode/deepseek)` → no handoff, `session/load` path invoked.
  - Bare-model case: profile `{provider:"", model:"grok-4.5"}` → derives `grok`, treated as cross-provider (or rejected as invalid — state decision).
  - No-op: `Tab plan` when plan pin is same provider as run → no new run minted.
- alerts: none.
- audit checks: spill checklist — verify no duplicate `providerSwitchLoading` state key divergence between Desktop (`store.ts`) and TUI (`app.go` `runHandle` guard).

## 10. Follow-Up Document Updates

- upstream docs that must change: `CP-57` (add posture→provider handoff branch to `P-3` after Task-300), `SD-06 §3.2` (posture `provider` as part of provider contract), `SS-05` (already has provider-selection invariant — add Tab note).
- notes left unchanged on purpose: `CA-679` (config path fix) stays; opencode internal `mode=build|plan` (`xai/grok-4.6` via opencode) remains a separate feature — not conflated with Grok CLI. `BUG-329` (opencode same-provider respawn) untouched.

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: CP-57
change_type: bugfix
summary: Posture Tab across providers handoffs via transcript envelope instead of injecting a foreign model into the current provider adapter
# --->8---

