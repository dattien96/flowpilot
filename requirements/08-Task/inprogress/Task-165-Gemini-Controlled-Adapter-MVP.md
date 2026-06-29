# Task-165: Gemini Controlled Adapter MVP

## Metadata

- Document ID: `Task-165`
- Title: `Gemini Controlled Adapter MVP`
- Phase: `task`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-27`
- Last Updated: `2026-06-27`
- Parent Documents: [CP-40: Gemini Controlled Adapter Over ACP Transport](../../07-Coding-Plan/todo/CP-40-Gemini-Adapter-Plan.md), [Task-164: Gemini ACP Transport Extraction](../done/Task-164-Gemini-ACP-Transport-Extraction.md)
- Child Documents: `None`
- Related Documents: [07 - Claude Provider Adapter Plan](../../10-Refactor/New-System/07-Claude-Adapter-Plan.md)
- Replaces: `None`
- Tags: `gemini, adapter, runner, provider-runtime, ai-providers`

## AI Quick View

### Summary

- Build the first `ProviderRuntimeAdapter` implementation for Gemini on top of the extracted ACP transport.
- Support controlled normal chat with normalized start, delta, completed, failed, and turn-completed events.
- Register Gemini in the live registry only with conservative capabilities proven by tests.

### Current Ask

- This task is complete: Gemini has a conservative controlled-mode ACP adapter in the live registry, with unproven tool/approval/resume capabilities kept disabled.

### Key Decisions

- `T-1` This MVP may advertise `Streaming`, `SkillSelection`, and `Interrupt` only when tests prove them.
- `T-2` Approval, MCP, file events, vision, and spawn-agent stay false until Task-166.
- `T-3` Default registry remains placeholder-safe; live registry may create the adapter.

### Constraints

- No Gemini-only desktop transport.
- Prompt assembly must use the shared selected-skill injection path.
- SendTurn must always terminate with `turn_completed` or return an error that the shared finalizer can normalize.

### Open Questions

- Whether live ACP supports durable resume is deferred to Task-167.

### Source Refs

- `CP-40` sections `P-3`, `P-4`, `P-8`, `P-10`, `G-01`, `G-02`, `G-03`, `G-08`, `G-09`.

## 1. Goal

Let Gemini run a controlled desktop chat turn through `ProviderRuntimeAdapter.SendTurn` and normalized provider events.

## 2. Parent Links

- coding plan: `CP-40`
- tech design: `SD-12`, `SD-06`
- system spec: `SS-11`
- specific upstream ids: `CP-40 P-3`, `P-4`, `P-8`, `P-10`

## 3. Trigger

After Task-164 extracts ACP helpers, the runner can add a real Gemini adapter without duplicating transport code.

## 4. Exact Change

- `T-1` Add `geminiAdapter` implementing `ProviderRuntimeAdapter`.
- `T-2` Add a Gemini ACP process/session wrapper that starts `gemini --acp`, initializes, creates a session, sends prompts, and maps text stream events.
- `T-3` Add fake-process/fake-transport tests for streaming, final response, provider session id, process failure, and cancellation.
- `T-4` Wire live `ProviderRegistryFor` to return the Gemini adapter only after account/env resolution succeeds.
- `T-5` Ensure selected skills and context prompt assembly use runner-owned preparation.

## 5. Touched Areas

- files: new Gemini adapter/process files, `provider_registry.go`, runner tests
- modules: local runner provider runtime
- routes: existing `/client/workflow-runs/.../turns`
- tables: none

## 6. Acceptance Check

- Gemini adapter unit tests pass.
- Default registry still reports Gemini placeholder-safe.
- Live registry returns Gemini with only proven conservative capabilities.
- Normal chat and model routing tests cover `gemini-*` and `auto-gemini-*`.

### 6.1 Definition of Done (DOD)

- [x] `DOD-1` `geminiAdapter` implements `ProviderRuntimeAdapter` and emits normalized delta/completion events.
- [x] `DOD-2` The adapter starts `gemini --acp` through the runner-owned process boundary and uses ACP initialize, `session/new`, and `session/prompt`.
- [x] `DOD-3` The MVP uses `--approval-mode plan` and does not advertise approval, MCP, file, resume, or vision capabilities.
- [x] `DOD-4` Live `ProviderRegistryFor` returns Gemini as available with only `Streaming`, `SkillSelection`, and `Interrupt`.
- [x] `DOD-5` Default registry remains placeholder-safe for Gemini.
- [x] `DOD-6` Prompt preparation uses the runner-owned selected-skill injection path.
- [x] `DOD-7` Targeted and broad local-runner tests pass.

## 7. Out of Scope

- Approval events, MCP tools, `ask_user`, `spawn_agent`, file events, and vision.
- Cross-account resume and transcript extraction.

## 8. Completion Notes

- result: done
- implementation notes: added `geminiAdapter` backed by ACP, registered it in the live runner registry, kept the default registry placeholder-safe, and forced `--approval-mode plan` so Task-165 does not grant uncontrolled tool/write authority before Task-166.
- verification: `go test ./internal/runner -run 'TestGemini(Adapter|ACP|Registry)|TestStartSessionGeminiACP|TestSendMessageGeminiACP|TestJsonRpcErrorMessageHandlesGeminiACPShapes|TestProviderKeyFromModel' -count=1`; `go test ./internal/runner -count=1`; `go test ./... -count=1` from `apps/local-runner`.
- follow-ups: Task-166, Task-167
- upstream docs updated: `CP-40` Child Documents now points to this task in `done`.
