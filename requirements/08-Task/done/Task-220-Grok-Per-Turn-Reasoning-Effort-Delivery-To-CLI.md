# Task-220: Grok Per-Turn Reasoning-Effort Delivery To CLI

## Metadata

- Document ID: `Task-220`
- Title: `Grok Per-Turn Reasoning-Effort Delivery To CLI`
- Phase: `task`
- Status: `done`
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

- [x] `DOD-1` The ACP transport investigation (`T-1`) is complete and its outcome recorded. **Outcome: spawn-time flag, and the spawn plumbing already existed.** The live probe (`testdata/grok_acp/live_probe_raw.txt`) confirms ACP `session/new`/`session/prompt` carry no client→server reasoning-effort field — effort is session-config state (`_meta["x.ai/sessionConfig"].options[]` with `category:"mode"`) set at process launch. `grok agent --help` exposes `--reasoning-effort <EFFORT>` as a launch-time flag, and `ensureGrokProcess` (grok_process.go, added by in-flight Task-218) already builds `grok agent --model <m> --reasoning-effort <e> stdio` and respawns the shared process when the effort changes per turn. The only missing piece was the canonical→supported mapping.
- [x] `DOD-2` `req.ReasoningEffort` reaches the Grok CLI: the turn's resolved effort flows `resolveTurnModelAndEffort` → `registry.Adapter` → `newAdapterForTurn` → `ensureGrokProcess` → `--reasoning-effort` launch flag, and `grokReasoningEffortID` is now wired into that path in the Grok `newAdapterForTurn` closure (`provider_registry.go`) — no longer dead code.
- [x] `DOD-3` Unmapped/unsupported effort values degrade to a supported id (xhigh/max→high, none/minimal→low) or omit the flag so the model default applies (`GR-35`). Covered by `TestProviderRegistryForGrokMapsReasoningEffortToSupportedLaunchFlag` (table-driven: high/xhigh/max/medium/low/minimal/none/empty/unknown).
- [~] `DOD-4` **N/A** — effort IS honored per-turn (via respawn), so the desktop Reasoning control is truthful as-is; no annotation/disable needed. `Q-3` resolved: not applicable.
- [x] `DOD-5` **Base-regression (`P-0`):** the change is confined to the Grok `newAdapterForTurn` closure plus a Grok-only test; no Codex/Claude/Gemini path touched. Their reasoning-effort transmission is unchanged.
- [x] `DOD-6` Targeted and broad local-runner tests pass. Targeted: 74 Grok tests green (incl. the new degrade test). Broad `go test ./internal/runner/` shows 12 failures, all pre-existing and unrelated to this change (Codex resume, compat, cross-account, flow-executor synthesis timeout, Google Drive MCP, skills-merge, auth-workspace) — none Grok, none in the touched `newAdapterForTurn` closure; consistent with the concurrent-WIP baseline documented in Task-207/210/213/215.

## 7. Out of Scope

- Per-model reasoning-effort detection/catalog persistence and the model-aware dropdown — already delivered by Task-215.
- Any change to Codex/Claude/Gemini effort handling (Task-215/CA-277 settled those).
- Grok account/quota/desktop-surface work (Task-210/211).

## 8. Completion Notes

- result: Code complete and unit-verified; pending a live E2E confirmation against a real Grok account. A user's per-turn reasoning-effort selection now reaches the Grok CLI as a validated `--reasoning-effort` launch flag, degrading unsupported values instead of being silently dropped (the Task-207 `DOD-9` gap).
- implementation notes:
  - Investigation (`T-1`) showed the spawn-time mechanism already existed: `ensureGrokProcess` (grok_process.go, in-flight Task-218 WIP) launches `grok agent --model <m> --reasoning-effort <e> stdio` and respawns the shared process when model/effort/yolo change per turn (`grok agent --help` confirms these are launch-only flags; ACP `session/new`/`session/prompt` carry no per-turn effort field per `testdata/grok_acp/live_probe_raw.txt`). The turn's effort already flowed `resolveTurnModelAndEffort` → `registry.Adapter` → `newAdapterForTurn` → `ensureGrokProcess`, but as the **raw canonical value** — so `xhigh`/`max` (unsupported by grok-4.5) would be passed to the CLI verbatim, and `grokReasoningEffortID` was dead code.
  - Fix (`T-2`/`T-3`): the Grok `newAdapterForTurn` closure in `provider_registry.go` now maps the incoming effort through `grokReasoningEffortID` before `ensureGrokProcess` — supported values pass through, unsupported degrade (xhigh/max→high, none/minimal→low), unmappable yields "" so the flag is omitted and the model default applies (`GR-35`). Mapping the value here also fixes the respawn-reuse comparison (two turns whose efforts both degrade to "high" reuse one process).
  - `DOD-4`/`Q-3` resolved N/A: because effort is honored per-turn via respawn, the desktop Reasoning control is truthful and needs no annotation/disable.
- verification:
  - Go: `go build ./...` clean. New `TestProviderRegistryForGrokMapsReasoningEffortToSupportedLaunchFlag` (9-case table: high/xhigh/max/medium/low/minimal/none/empty/unknown) asserts the exact launch argv, plus the pre-existing `TestProviderRegistryForGrokThreadsModelAndReasoningEffortIntoLaunch` still passes; 74 Grok tests green. `grokReasoningEffortID` now has a production call site (`provider_registry.go`).
  - Broad `go test ./internal/runner/`: 12 pre-existing, unrelated failures (see `DOD-6`); none Grok/effort-related.
  - **Not yet live-verified** against a real Grok account (no credentials in this environment) that selecting e.g. `xhigh` produces observably higher-effort behavior end to end — the unit test proves the correct flag is launched; a credentialed run would prove the CLI honors it.
  - **Concurrent-WIP caveat:** `provider_registry.go` and `grok_process.go` also carry uncommitted Task-218 (`--always-approve`/YOLO) work in the same files; this task's change is the `grokReasoningEffortID` mapping in the Grok closure only. A missing `os` import in `grok_registry_test.go` (introduced by concurrent Task-218 config-yolo tests) was added so the package compiles.
- follow-ups: Live E2E verification with a credentialed Grok account (confirm the CLI observably changes behavior for a degraded/mapped effort). If a future ACP release adds a real per-turn reasoning-effort field, prefer it over respawn to avoid the process-restart cost on effort change.
- upstream docs updated: Task-207 `DOD-9` and its gap note link here.
