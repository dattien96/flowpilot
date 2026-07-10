# Task-212: Grok Parity Hardening And Live DOD

## Metadata

- Document ID: `Task-212`
- Title: `Grok Parity Hardening And Live DOD`
- Phase: `task`
- Status: `in_progress`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-07-09`
- Last Updated: `2026-07-10`
- Parent Documents: [CP-46: Grok Build Controlled Adapter Over ACP Transport](../../07-Coding-Plan/todo/CP-46-Grok-Build-Controlled-Adapter-Over-ACP.md), [Task-209: Grok MCP, Ask-User, And Spawn-Agent Parity](./Task-209-Grok-MCP-Ask-User-Spawn-Agent-Parity.md), [Task-210: Grok Account Model — Detect, Connect, Switch, Quota](./Task-210-Grok-Account-Model-Detect-Connect-Switch-Quota.md), [Task-211: Grok Desktop UI Surface](./Task-211-Grok-Desktop-UI-Surface.md)
- Child Documents: `None`
- Related Documents: [Task-167: Gemini Resume Handoff And Live DOD](../done/Task-167-Gemini-Resume-Handoff-And-Live-DOD.md), [CA-091: Skill Injection Order](../../../change-audit/CA-091-skill-injection-order.md)
- Replaces: `None`
- Tags: `grok, grok-build, skills, context, flow-gate, summary, handoff, resume, dod`

## AI Quick View

### Summary

- Prove skill injection, context injection, flow-gate finalization, and summary generation are byte-for-byte the same shared path Codex/Claude/Gemini use — no Grok-specific formatting or reordering.
- Enable Grok in the live `ProviderRegistryFor` registry (moving off the placeholder) once Task-207/208/209/210 all pass, and re-seed resume state on runner restart.
- Decide and implement (or explicitly defer) Grok-as-handoff-source transcript extraction from `~/.grok/sessions` SQLite.
- Run the full CP-46 live acceptance pass: one real Grok desktop turn covering chat, an approval-gated action, a spawned child agent, a generated summary, a flow-rule gate, and a resume/account-switch — and close every remaining `GR-*` validation item plus the CP-46 DOD checklist.

### Current Ask

- This is the closing task for CP-46: it does not add new transport/adapter code, it wires the already-built pieces into the live registry, proves the shared-path guarantees, and executes the CP's own validation plan end to end, updating `10.2 Current Verification Status` in CP-46 with real evidence.

### Key Decisions

- `T-1` Skill/context injection must go through the exact same `promptPrep` function Codex/Claude/Gemini call — this task's tests compare Grok's assembled prompt content/order against a Codex/Claude baseline for one-skill and multi-skill selections (matching the `CA-091` regression shape).
- `T-2` Flow gates (`r-ca`/`r-bug`/`r-task`) and their repair prompts must run through the shared `finishTurn`/`runFlowGate` path with Grok as the active provider — no Grok-only gate bypass.
- `T-3` Summary generation (manual "Gen summary" + idle) must work for Grok chats via the shared summarizer, using a cheap Grok model if one exists or a documented controlled replacement.
- `T-4` Live registry enablement is the last code change in this task, gated on all upstream tasks' tests passing — mirrors CP-40's rollout order (extraction → adapter → enablement → validation).
- `T-5` Grok-as-handoff-source stays disabled unless a `~/.grok/sessions` SQLite extractor is built and proven in this task; if not attempted, CP-46's Open Question `Q-7` is answered "deferred" with the concrete blocker documented.
- `T-6` This task must update CP-46 `§10.2 Current Verification Status` and flip `§10.1` checklist items to checked only for what is actually proven here.

### Constraints

- **PLUGIN-ONLY / ZERO BASE REGRESSION (CP-46 `P-0`):** additive-only; `summarizerModelFor`/`supportsHandoffSource`/`resolvePromptExecutionAdapter`/`LocateSessionFile` get an appended `grok` case that must not alter their return values for codex/claude/gemini; the live-registry enablement must not change how the other providers are registered.
- Do not touch Codex/Claude/Gemini flow-gate, summarizer, or handoff code except where a shared function needs a Grok-safe generalization; any such change needs regression coverage for the other three providers.
- Do not flip a `ProviderCapabilities` flag to true without the corresponding `GR-*` test/live-check passing.
- The live acceptance run must be a real credentialed Grok session (per CP-46 Constraints on billing/subscription), not a fake-transport-only proof, for the final DOD sign-off.

### Open Questions

- Whether Grok's summary model should be a distinct cheap model id or reuse the same `grok-4.5`/`grok-build` model with a short system prompt (cost/latency tradeoff, decide during implementation).
- Whether resume safety across `GROK_HOME` changes can be proven session-portable or must return a typed mismatch like Gemini's cross-account caution (CP-46 `Q-2`, `R-7`).

### Source Refs

- `CP-46` sections `P-10`, `P-11`, `P-12`; parity rows `GR-08`, `GR-09`, `GR-10`, `GR-13`, `GR-14`, `GR-15`, `GR-16`, `GR-26`; `§7.1` E2E items `E2E-01` through `E2E-20`; `§10` Definition of Done + `§10.1`/`§10.2`.
- `requirements/08-Task/done/Task-167-Gemini-Resume-Handoff-And-Live-DOD.md` (closing-task template for a provider rollout).
- `change-audit/CA-091-skill-injection-order.md`, `CA-119-provider-consistent-agent-spawn-prompt.md`, `CA-132-context-summary-and-handoff.md`.
- `apps/local-runner/internal/runner/interactive_service.go` (`finishTurn`, resume re-seed), `summarizer.go`, `agent_orchestrator.go` (flow-gate finalization touch points), `provider_registry.go` (`ProviderRegistryFor` live enablement).

## 1. Goal

Close CP-46: prove Grok shares the exact prompt-assembly, flow-gate, and summary paths the other providers use; enable Grok in the live registry; decide handoff-source scope; and execute one real, credentialed, end-to-end acceptance run that satisfies CP-46's Definition of Done.

## 2. Parent Links

- coding plan: `CP-46`
- tech design: `SD-12`, `SD-19`
- system spec: `SS-11`, `SS-15`, `SS-16`
- specific upstream ids: `CP-46 P-10`, `P-11`, `P-12`

## 3. Trigger

Task-207 through Task-211 individually build/prove transport, permission, MCP/agents, accounts, and desktop UI. This task is the integration and closing pass CP-46 requires before Grok can be called "done."

## 4. Exact Change

- `T-1` Add a prompt-assembly parity test: run the same one-skill and multi-skill selection through Codex, Claude, and Grok adapters (fake transport) and assert identical assembled skill-content/order and identical context-injection blocks (feature history, change-audit, chat summary, handoff context).
- `T-2` Add flow-gate integration tests: trigger `r-ca`, `r-bug`, `r-task` on a Grok-active chat; assert violation/repair-prompt events use the shared normalized shape and repair prompts stay on Grok.
- `T-3` Wire Grok chats into the shared summarizer: **append a `grok` case to `summarizer.go summarizerModelFor`** (cheap Grok model) and to the summary path's `supportsHandoffSource`/gate, and **append a `grok` case to `runner.go resolvePromptExecutionAdapter`** for the one-shot summarizer call (the `P-13` `grok -p --output-format json` exception — the only non-ACP Grok usage). Add manual "Gen summary" and idle-summary tests for Grok chats; document the chosen summary model.
- `T-4` Flip `ProviderRegistryFor` to register the live Grok adapter (already built in Task-207/208/209) as the default for the `grok` key, once all upstream task tests are green; keep the default (placeholder) registry unchanged.
- `T-5` Add a resume re-seed branch for `grokAdapter` in `interactive_service.go` (mirror the Claude/Codex re-seed-from-run-record logic) so runner restart resumes Grok chats by real session id or returns a typed mismatch.
- `T-6` Attempt a `~/.grok/sessions` SQLite transcript extractor for handoff-source; if built, **append a `grok` case to `handoff_context.go supportsHandoffSource`** and wire summary-based handoff mode selection (`loadHandoffSummary` hybrid/target_summary/raw, Task-162); if infeasible in-scope, explicitly disable Grok-as-source and document the blocker (do not leave it silently half-wired). Grok-as-target must already work.
- `T-7` **Drive sync / cross-PC restore (`GR-23`):** append a `grok` case to `session_file_locator.go LocateSessionFile` (Grok sessions are `~/.grok/sessions/` SQLite — implement a real locator or return a typed-unsupported state), so stale-account recovery (BUG-092/093 class) and cross-PC `sessions.ndjson` resume either work or fail with a typed mismatch without corrupting history.
- `T-8` **Replay typed user prompts (`GR-34`):** add a Grok user-prompt extractor (analog to `claude_event_mapper.go claudeUserPromptText`) so reopened Grok chats show the typed user input, not the composed prompt (BUG-046/083).
- `T-9` **Grok as hub + review-loop (`GR-33`):** verify Grok can drive `autoOrchestrate`, `SubmitFlowControl`, and the shared review-loop template to convergence within cap/extend — not only participate as a child.
- `T-10` **Child stop + delete-cascade (`GR-36`):** verify stopping/deleting a Grok parent stops in-flight children and leaves no orphaned `~/.grokHomeN`/session artifacts (BUG-086).
- `T-11` Execute the full CP-46 `§7.1` E2E test list (`E2E-01`..`E2E-36`) end to end with a real credentialed Grok account; record pass/fail per item.
- `T-12` **Base-regression sweep (`P-0`):** run the complete existing Codex/Claude/Gemini local-runner + desktop suites after all Grok tasks land; assert `defaultModelForProvider`/`summarizerModelFor`/`supportsHandoffSource`/`resolvePromptExecutionAdapter`/`LocateSessionFile` return byte-identical values for codex/claude/gemini inputs; confirm `gemini_acp_transport.go` git diff is empty.
- `T-13` Update `CP-46` `§10.1` DOD checklist (check only proven items) and `§10.2 Current Verification Status` with real evidence/links from this task's test run.

## 5. Touched Areas

- files: `apps/local-runner/internal/runner/provider_registry.go` (live enablement), `interactive_service.go` (resume re-seed branch), `summarizer.go` (Grok wiring if any generalization is needed), new `grok_transcript_extractor.go` (if attempted), test files across the above; `requirements/07-Coding-Plan/todo/CP-46-Grok-Build-Controlled-Adapter-Over-ACP.md` (status/DOD updates)
- modules: prompt assembly, flow-gate finalization, summary generation, resume/session persistence, cross-provider handoff
- routes: none new
- tables: none

## 6. Acceptance Check

- Prompt-assembly parity test passes for one-skill and multi-skill cases across Codex/Claude/Grok.
- Flow-gate tests pass with Grok as the active provider for all three rule families.
- Manual and idle summary generation work for a Grok chat.
- Live registry serves the real Grok adapter; default registry remains placeholder-safe.
- Runner-restart resume test passes for Grok (or returns the documented typed mismatch).
- The full CP-46 E2E list has recorded pass/fail results from a real credentialed run.
- CP-46's own DOD checklist and verification-status table are updated to reflect actual (not aspirational) state.

### 6.1 Definition of Done (DOD)

- [ ] `DOD-1` Skill/context injection parity proven against Codex/Claude baselines. **Not tested.** The wiring is identical (`promptPrep` calls the same `r.injectSelectedSkills`, `shouldInjectFeatureHistory` includes `ProviderKeyGrok`), but no comparison test was written.
- [ ] `DOD-2` `r-ca`/`r-bug`/`r-task` flow gates proven on Grok via the shared finalizer. **Not tested.** No flow-gate code path was touched for Grok (none needed to be — `finishTurn`/gate logic is provider-neutral), but no Grok-specific gate test exists.
- [ ] `DOD-3` Manual + idle summary generation proven for Grok chats. **Wired, not tested.** `summarizerModelFor`/`supportsHandoffSource` gate/`resolvePromptExecutionAdapter` all have additive grok cases (one-shot `grok -p --output-format json`, the P-13 exception), but no summary was actually generated against a real or fake Grok exec call.
- [x] `DOD-4` Live registry returns the real Grok adapter as default for `grok`; default registry remains placeholder-safe. (`TestProviderRegistryForGrokUsesLiveWhenFlagOnAndAccountResolvable` / `...UsesPlaceholderWhenFlagOff`.)
- [ ] `DOD-5` Resume re-seed on runner restart proven (or typed mismatch proven, if cross-home resume is unsafe). **Not done.** `refreshResumeHandleLocked` (`interactive_service.go`) was never given a `ProviderKeyGrok` case — this is a real gap, not an oversight covered elsewhere; a runner restart today would not re-discover a Grok session id the way it does for Codex/Claude/Gemini.
- [x] `DOD-6` Handoff-source decision made and documented (built + tested, or explicitly deferred with blocker). **Deferred**, documented at `handoff_context.go supportsHandoffSource` and CP-46 §10.2: no `~/.grok/sessions` SQLite extractor was attempted; Grok-as-target already works (reuses the provider-neutral handoff path), Grok-as-source stays disabled.
- [ ] `DOD-7` Full CP-46 `§7.1` E2E list executed against a real credentialed Grok account with recorded results. **Not done** — see DOD-13.
- [x] `DOD-8` Drive sync / cross-PC restore works for Grok via a `LocateSessionFile` branch, or returns typed-unsupported without corrupting history (`GR-23`). (`session_file_locator.go` explicit `ProviderKeyGrok` case returning typed-unsupported; matches the "no SQLite reader built" decision.)
- [ ] `DOD-9` Reopened Grok chats replay the typed user prompt, not the composed prompt (`GR-34`). **Not attempted.** No Grok analog of `claudeUserPromptText`/transcript replay extractor was built.
- [ ] `DOD-10` Grok can drive a hub / review-loop to convergence (`GR-33`); Grok parent stop/delete cascades with no orphaned artifacts (`GR-36`). **Not attempted.** Both rely on provider-neutral orchestration code untouched by this work, so they are plausible but unverified for Grok specifically.
- [ ] `DOD-11` Token usage + `ModelContextWindow` verified live in a real Grok run (`GR-24`). Verified against **fake-transport fixtures** built from real live-captured token-usage payloads (`TestGrokAdapterSendTurnStreamsAndCompletes`), not a live run in this pass.
- [x] `DOD-12` CP-46 `§10.1`/`§10.2` updated to reflect only actually-proven state. (This edit + CP-46 §10.2 rewrite.)
- [ ] `DOD-13` Full CP-46 `§7.1` E2E list (`E2E-01`..`E2E-36`) executed against a real credentialed Grok account with recorded results. **Not done.** This requires a dedicated, deliberate live QA pass (each item touches real account state, tool execution, and in several cases account switching / spending) that was out of scope for an implementation pass; recommend scheduling it as a follow-up before removing the `FLOWPILOT_GROK_AGENT` gate in any shared/default environment.
- [x] `DOD-14` **Base-regression sweep (`P-0`):** complete existing Codex/Claude/Gemini suites pass unchanged; `gemini_acp_transport.go` diff empty; the five shared switched-functions return byte-identical values for codex/claude/gemini (`GR-BR`, `E2E-36`). (Verified: `git diff --stat` on `gemini_acp_transport.go` is empty; full `go test ./...` before and after this work shows the identical 15 pre-existing, unrelated failures — zero regressions.)

## 7. Out of Scope

- Any new transport, permission, MCP, account, or UI code — those are exclusively Task-206 through Task-211's scope; this task only integrates and validates them.
- Expanding Grok capability flags beyond what this task's own tests prove.

## 8. Completion Notes

- result:
- implementation notes:
- verification:
- follow-ups:
- upstream docs updated: `CP-46` Child Documents / Status / `§10.1`/`§10.2` on completion.
