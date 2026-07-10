# Task-220: Grok Per-Turn Reasoning-Effort Delivery To CLI

## Metadata

- Document ID: `Task-220`
- Title: `Grok Per-Turn Reasoning-Effort Delivery To CLI`
- Phase: `task`
- Status: `in_progress`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-10`
- Last Updated: `2026-07-10`
- Parent Documents: [CP-46: Grok Build Controlled Adapter Over ACP Transport](../../07-Coding-Plan/inprogress/CP-46-Grok-Build-Controlled-Adapter-Over-ACP.md), [Task-207: Grok Controlled Adapter MVP (Chat/Stream/Resume)](../done/Task-207-Grok-Controlled-Adapter-MVP.md)
- Child Documents: `None`
- Related Documents: [Task-215: Per-Model Reasoning-Effort Detection And Model-Aware UI](../done/Task-215-Per-Model-Reasoning-Effort-Detection-And-Model-Aware-UI.md), [Task-210: Grok Account Model — Detect, Connect, Switch, Quota](./Task-210-Grok-Account-Model-Detect-Connect-Switch-Quota.md)
- Replaces: `None`
- Tags: `grok, grok-build, adapter, reasoning-effort, acp, provider-runtime`

## AI Quick View

### Summary

- Task-207 (`DOD-9`) shipped `grokReasoningEffortID` — a mapping from FlowPilot's canonical effort values (`none/minimal/low/medium/high/xhigh/max`) to Grok ACP effort ids — but the adapter never calls it. `grok_adapter.go`'s `SendTurn` builds `grokACPPromptParams(sessionID, prompt)` with no effort argument, so `grokReasoningEffortID` is currently dead code from the adapter's perspective.
- The value IS present end to end up to the adapter boundary: the desktop passes it, the runner builds `TurnRequest.ReasoningEffort` (`interactive_service.go`), and it is even written to the per-turn diagnostic log (`turn-<id>-params.json` shows e.g. `"reasoning_effort": "medium"`). It is then silently dropped when the ACP prompt is assembled — the user's UI selection has **no effect on the actual Grok CLI process**, which always runs at the model's default effort.
- Root cause is protocol, not a simple omission: ACP `session/new`/`session/prompt` (per the fetched spec and CP-46 §10.2) expose **no `reasoningEffort` field**, so there is nowhere obvious to put the value. Resolving it needs one of: (a) an `_x.ai/*` ACP extension method/param (undiscovered so far), or (b) passing `--reasoning-effort` at process spawn time — which applies per-account/per-process (the Grok process is shared across turns), not per-turn.

### Current Ask

- Make a user's per-turn Grok reasoning-effort selection actually reach the Grok CLI, or — if the protocol genuinely cannot carry it per-turn — decide and document the honest degraded behavior (e.g. process-level effort, or surfacing to the user that Grok effort is fixed per session) instead of silently discarding the value.

### Key Decisions

- `T-1` **Investigate the ACP surface first.** Before choosing spawn-time flags, probe the live `grok agent stdio` ACP handshake (`initialize`, `session/new`, `session/prompt`) and any `_x.ai/*` / `_meta` extension for a documented or de-facto per-turn reasoning-effort channel. The `initialize` response already reports a per-model `reasoningEfforts` list (`grok_acp_types.go`), which strongly implies the CLI accepts the value *somewhere*.
- `T-2` **Reuse `grokReasoningEffortID`, don't re-derive.** Whatever the transport, the canonical→Grok id mapping and the explicit degrade-to-default rule (`GR-35`) already exist and are unit-tested; this task wires them in, it does not rewrite them.
- `T-3` **Degrade honestly, never fabricate.** If no per-turn channel exists, the fallback (process-level `--reasoning-effort` or documented fixed-per-session behavior) must be explicit and, if it changes what the user sees, surfaced — not a silent no-op as today.
- `T-4` **Per-turn vs per-process semantics must be stated.** If the only viable path is spawn-time, document that switching effort mid-session may require a process restart (or is simply not honored until the next process spawn), and decide whether that is acceptable for MVP.

### Constraints

- **PLUGIN-ONLY / ZERO BASE REGRESSION (CP-46 `P-0`):** no change to Codex/Claude/Gemini reasoning-effort paths; the fix is Grok-adapter-local plus, if needed, Grok process-spawn plumbing.
- Do not change how a chosen effort is transmitted for any other provider (Codex's `-c model_reasoning_effort=`, Claude's `--effort`) — Task-215/CA-277 already settled those.
- If the spawn-time path is chosen, respect the existing Grok ambient-MCP-scan-disabled discipline and `FLOWPILOT_GROK_BIN`/`FLOWPILOT_GROK_AGENT` overrides.

### Open Questions

- `Q-1` Does `grok agent stdio` accept a per-turn reasoning-effort via an `_x.ai/*` extension method or a `_meta`/params field on `session/prompt` that CP-46's spec capture missed? (Primary investigation, `T-1`.)
- `Q-2` If only spawn-time works: is per-process (per-account) effort acceptable for MVP, or must the process be recycled when the user changes effort mid-session? (`T-4`.)
- `Q-3` Should the desktop Reasoning dropdown be disabled/annotated for Grok if effort cannot be honored per-turn, to avoid implying an effect that does not occur?

### Source Refs

- Task-207 `DOD-9` and its "Gap found during this pass" note (the origin of this task).
- `apps/local-runner/internal/runner/grok_event_mapper.go` (`grokReasoningEffortID`, currently uncalled).
- `apps/local-runner/internal/runner/grok_adapter.go` (`SendTurn`, `grokACPPromptParams` — the drop point).
- `apps/local-runner/internal/runner/grok_acp_types.go` (`initialize` per-model `reasoningEfforts` list).
- `apps/local-runner/internal/runner/interactive_service.go` (`TurnRequest.ReasoningEffort` populated at ~2944; `logTurnProviderParams` at ~2933 proving the value arrives).
- `apps/local-runner/internal/runner/prompt_log.go` (`turn-<id>-params.json` shape).
- CP-46 §10.2 (real `session/new` fields — no `reasoningEffort`).

## 1. Goal

Make a user's Grok reasoning-effort selection take real effect on the Grok CLI process — or, if the ACP transport cannot carry it per-turn, replace today's silent drop with an explicit, documented, user-visible degraded behavior.

## 2. Parent Links

- coding plan: `CP-46`
- tech design: `SD-06`
- system spec: `SS-05`
- specific upstream ids: `CP-46 P-4`, `GR-35`; Task-207 `DOD-9`

## 3. Trigger

Task-207 shipped the Grok adapter MVP and, during that pass, discovered that `req.ReasoningEffort` is never forwarded from the adapter into the ACP call — `grokReasoningEffortID` was written but left unwired because ACP `session/new`/`session/prompt` expose no effort field. A live turn (`turn-7217-params.json`, `provider=grok`, `model=grok-composer-2.5-fast`, `reasoning_effort=medium`) confirms the value is logged by the runner but has no observable effect on the CLI, since nothing carries it across the process boundary.

## 4. Exact Change

- `T-1` **Probe the transport.** Capture a live `grok agent stdio` ACP session and inspect `initialize`/`session/new`/`session/prompt` (and any `_x.ai/*` extension or `_meta` params) for a per-turn reasoning-effort channel. Record findings in `Open Questions`/CP-46.
- `T-2` **If a per-turn channel exists:** wire `req.ReasoningEffort` → `grokReasoningEffortID` → the discovered ACP field inside `grok_adapter.go SendTurn` (via `grokACPPromptParams` or the extension call), with the `GR-35` degrade-to-default path for unmapped/unsupported values.
- `T-3` **If only spawn-time works:** thread the resolved Grok effort id into the `grok agent stdio` spawn args (`--reasoning-effort <id>` or the real flag name), and define/implement the per-process semantics decided in `Q-2` (e.g. recycle the process when the active effort changes, or document that it is applied at next spawn).
- `T-4` **If neither works:** annotate/disable the desktop Reasoning control for Grok (`Q-3`) so the UI does not imply an effect that does not occur, and document the limitation in CP-46.
- `T-5` **Tests.** Adapter test asserting the resolved effort id reaches the chosen transport for each canonical input, plus the degrade case; a regression test that Codex/Claude/Gemini effort transmission is byte-identical.

## 5. Touched Areas

- files: `apps/local-runner/internal/runner/grok_adapter.go`, `grok_event_mapper.go` (wire existing mapper), `grok_process.go`/`grok_acp.go` (only if spawn-time path); possibly `apps/desktop-flowpilot/src/components/ChatInput.tsx` (only under `T-4`).
- modules: Grok provider runtime; possibly desktop chat composer.
- routes: existing `/client/workflow-runs/.../turns` (no new routes).
- tables: none.

## 6. Acceptance Check

- Selecting a non-default reasoning effort for a Grok chat produces an observable behavior change in the Grok CLI turn (verified live), OR the UI honestly reflects that effort is fixed/process-scoped for Grok.
- `grokReasoningEffortID` is no longer dead code — it is exercised on the real turn path.
- Unmapped/unsupported efforts degrade to the model default explicitly, never sending an invalid flag.
- Codex/Claude/Gemini reasoning-effort transmission is unchanged (regression).

### 6.1 Definition of Done (DOD)

- [ ] `DOD-1` The ACP transport investigation (`T-1`) is complete and its outcome (per-turn field found / spawn-time only / not supported) is recorded in this doc and CP-46.
- [ ] `DOD-2` `req.ReasoningEffort` reaches the Grok CLI through the chosen mechanism, or the chosen honest-degrade behavior is implemented — `grokReasoningEffortID` is wired into the real turn path either way.
- [ ] `DOD-3` Unmapped/unsupported effort values degrade to the model default explicitly (`GR-35`), covered by a test.
- [ ] `DOD-4` If effort cannot be honored per-turn, the desktop Reasoning control for Grok is annotated/disabled so it does not imply a false effect (`Q-3`).
- [ ] `DOD-5` **Base-regression (`P-0`):** Codex/Claude/Gemini reasoning-effort transmission is byte-identical; Grok changes are additive.
- [ ] `DOD-6` Targeted and broad local-runner tests pass.

## 7. Out of Scope

- Per-model reasoning-effort detection/catalog persistence and the model-aware dropdown — already delivered by Task-215.
- Any change to Codex/Claude/Gemini effort handling (Task-215/CA-277 settled those).
- Grok account/quota/desktop-surface work (Task-210/211).

## 8. Completion Notes

- result:
- implementation notes:
- verification:
- follow-ups:
- upstream docs updated:
