# Task-167: Gemini Resume Handoff And Live DOD

## Metadata

- Document ID: `Task-167`
- Title: `Gemini Resume Handoff And Live DOD`
- Phase: `task`
- Status: `in_progress`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-27`
- Last Updated: `2026-06-28`
- Parent Documents: [CP-40: Gemini Controlled Adapter Over ACP Transport](../../07-Coding-Plan/todo/CP-40-Gemini-Adapter-Plan.md), [SD-12: Refactor Workflow With Session](../../06-System-Tech-Design/SD-12-Refactor-Workflow-With_Session.md), [SD-06: AI Provider Integration](../../06-System-Tech-Design/SD-06-AI-Provider-Integration.md), [SD-14: Codex Cross-Account Chat Resume And Home Sync](../../06-System-Tech-Design/SD-14-Codex-Cross-Account-Chat-Resume-And-Home-Sync.md), [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md), [SS-14: Code Context And Regression Safety](../../05-System-Specs/SS-14-Code-Context-And-Regression-Safety.md)
- Child Documents: `None`
- Related Documents: [Task-165: Gemini Controlled Adapter MVP](../done/Task-165-Gemini-Controlled-Adapter-MVP.md), [Task-166: Gemini Controlled Tools And Approvals](../done/Task-166-Gemini-Controlled-Tools-And-Approvals.md), [Task-078: Cross-Provider Chat Handoff](../done/Task-078-Cross-Provider-Chat-Handoff.md), [Task-071: Cross-Account Chat Resume Definition of Done Checklist](../done/Task-071-Cross-Account-Chat-Resume-DOD-Checklist.md)
- Replaces: `None`
- Tags: `gemini, resume, handoff, account, validation, ai-providers`

## AI Quick View

### Summary

- Prove Gemini resume, account isolation, summary generation, flow gates, and cross-provider handoff behavior.
- Enable Gemini-as-source handoff only if transcript extraction is reliable.
- Close parity gaps found in later Codex/Claude bugfixes: usage/quota failures, external MCP, child-agent lifecycle, Drive sync/restore, token/context display, model metadata, replay state, and attachment fallback.
- Close CP-40 DOD with a live/manual parity pass and documented unsupported gaps.

### Current Ask

- Finish Gemini controlled-mode readiness by validating lifecycle and cross-account behavior end to end.

### Key Decisions

- `T-1` Same-provider cross-account resume must be proven safe or blocked with a typed mismatch.
- `T-2` Gemini-as-source handoff remains disabled until transcript extraction reconstructs visible chat history.
- `T-3` Live DOD must not mark unsupported capabilities true.
- `T-4` Gemini resume metadata can be unit-proven with ACP `session/load`; full live resume parity remains blocked until local Gemini auth/API key is configured.
- `T-5` Parity validation must include the post-Codex/Claude regression inventory, not only the original adapter DOD.

### Constraints

- Do not silently resume a Gemini session under a different account home.
- Do not regress Codex/Claude resume and handoff paths.
- Do not claim Gemini parity for usage/quota, MCP, child agents, Drive sync, token/context, models, replay, or attachments without a passing unit/live check.

### Open Questions

- Gemini session artifact portability and transcript format must be captured from live CLI behavior.
- Local Gemini CLI is installed, but authenticated `session/new` cannot run because Gemini API key/auth is missing.
- Live probe update: the runner process using current Codex HOME still initializes Gemini ACP but `session/new` fails with `Gemini API key is missing or not configured`; a probe using `HOME=/Users/tiendat` finds OAuth/config files but `session/new` fails with `UNSUPPORTED_CLIENT`, so this client cannot be used for Gemini Code Assist for individuals.

### Source Refs

- `CP-40` sections `P-9`, `P-11`, `P-12`, `P-13`, `P-14`, `P-15`, `G-10` through `G-27`.

## 1. Goal

Close the remaining Gemini lifecycle, resume, handoff, and full-DOD validation work after the adapter and tool surfaces are implemented.

## 2. Parent Links

- coding plan: `CP-40`
- tech design: `SD-12`, `SD-06`, `SD-14`
- system spec: `SS-11`, `SS-14`
- specific upstream ids: `CP-40 P-9`, `CP-40 P-11`, `CP-40 P-12`, `CP-40 P-13`, `CP-40 P-14`, `CP-40 P-15`, `CP-40 G-10`, `CP-40 G-11`, `CP-40 G-12`, `CP-40 G-13`, `CP-40 G-14`, `CP-40 G-15`, `CP-40 G-16`, `CP-40 G-17`, `CP-40 G-18`, `CP-40 G-19`, `CP-40 G-20`, `CP-40 G-21`, `CP-40 G-22`, `CP-40 G-23`, `CP-40 G-24`, `CP-40 G-25`, `CP-40 G-26`, `CP-40 G-27`

## 3. Trigger

Gemini is not fully ready until resume, account switching, summary generation, flow gates, handoff behavior, and the post-Codex/Claude regression parity areas are proven or explicitly blocked.

## 4. Exact Change

- `T-1` Persist and reload Gemini provider session ids through existing session-state paths.
- `T-2` Add cross-account Gemini resume tests for same-home, wrong-home, and missing-account cases.
- `T-3` Add Gemini transcript extractor only if readable session artifacts support it.
- `T-4` Validate summary generation and context-summary injection for Gemini chats.
- `T-5` Run flow-gate checks and repair prompts using Gemini.
- `T-6` Add operator notes for supported local auth/env setup and unsupported capabilities.
- `T-7` Add Gemini ACP `session/load` request support and capture provider-owned session ids for restart/resume metadata.
- `T-8` Validate Gemini usage/quota/rate-limit/auth failure classification, including no misleading login guidance and no retry loop for terminal limits.
- `T-9` Validate Gemini account metadata and model/usage bucket rendering, including missing/expired auth and stale metadata reset behavior.
- `T-10` Validate Gemini external MCP availability for FlowPilot-managed servers such as Google Drive in both YOLO states, or keep MCP capability disabled with diagnostics.
- `T-11` Run the Gemini child-agent lifecycle matrix: graph updates, approval/question gates, restart visibility, `wait=true`, `wait=false`, inherited YOLO, final result delivery, and provider-independent spawn prompt composition.
- `T-12` Validate Gemini Drive sync/restore and stale provider-account recovery, or return typed unsupported state when Gemini session evidence cannot be verified.
- `T-13` Validate Gemini token/context display, supported-model metadata, reasoning controls, and history replay state.
- `T-14` Validate Gemini attachment fallback while `Vision=false`; any future vision enablement must pass the Task-052 normalized image contract.

## 5. Touched Areas

- files: Gemini adapter/session store/transcript extractor tests, summary and handoff docs if needed, provider account metadata tests, MCP/tool tests, child-agent lifecycle tests, sync/restore tests, replay tests, attachment fallback tests
- modules: local runner provider runtime, chat history/resume, handoff, summarizer, flow gate, provider accounts, MCP config, agent orchestration, Drive sync, token/context reporting, supported-model routing
- routes: existing history/resume, handoff, chat-summary, provider-account, MCP setup, sync/restore, and turn endpoints
- tables: no expected schema change

## 6. Acceptance Check

- Runner restart can preserve Gemini history and provider session metadata once a live authenticated session exists; deterministic tests now cover real session-id capture and `session/load` request shape.
- Account mismatch behavior remains explicit through existing provider-account resolution; full two-home Gemini artifact portability still needs live evidence.
- Gemini summaries and flow gates remain on shared runner paths but authenticated live Gemini validation is still blocked locally.
- Gemini-as-source handoff remains disabled because readable session artifacts have not been proven.
- Gemini usage/quota failures, account metadata, external MCP, child-agent lifecycle, Drive sync/restore, token/context display, model metadata, replay state, and attachment fallback are either validated against Gemini or explicitly marked unsupported with capability flags false.
- Codex/Claude regression paths for resume, handoff, approvals, MCP, child agents, Drive sync, attachments, models, token/context, and replay remain green after Gemini changes.
- CP-40 live/manual DOD checklist must remain partial until a supported Gemini CLI auth method accessible to the runner process can create sessions.

## 7. Out of Scope

- New desktop layouts beyond capability/status rendering.
- Vision attachments unless Gemini ACP attachment flow is separately proven.
- Implementing new provider-account UI or attachment UX beyond capability/status and fallback behavior.

## 8. Completion Notes

- result: partial implementation; live DOD blocked by missing Gemini auth/API key
  - current live probe confirms two distinct blockers: current Codex HOME lacks Gemini API key/config, and `HOME=/Users/tiendat` hits `UNSUPPORTED_CLIENT` for Gemini Code Assist for individuals
- completed:
  - extracted ACP `session/load` params and wired Gemini adapter resume selection from persisted/provider-owned session ids
  - added shared runner Gemini session-id map and post-turn capture into `realProviderSessionID`
  - persisted Gemini provider session metadata through the existing `ProviderSessionStore`
  - added tests for `session/load`, prompt session id reuse, provider-session upsert, and interactive resume-handle capture
- unsupported until live validation:
  - Gemini-as-source handoff/transcript extraction
  - cross-Gemini-account artifact portability
  - end-to-end desktop approval, spawned agent, summary, flow-gate, and resume parity
  - Gemini usage/quota classification, external MCP/Google Drive availability, child-agent lifecycle matrix, Drive sync/restore, token/context reporting, model metadata, replay-state parity, and attachment fallback beyond unit-proven capability gates
- operator notes:
  - local Gemini CLI probe found `gemini` version `0.40.1` and ACP `loadSession`/HTTP MCP capabilities
  - current Codex HOME probe still fails `session/new` because Gemini API key/config is missing
  - `HOME=/Users/tiendat` probe reaches Gemini OAuth/config files but `session/new` fails with `UNSUPPORTED_CLIENT`; the CLI states this client is no longer supported for Gemini Code Assist for individuals
  - next live-validation step is to configure a supported Gemini CLI auth method that the runner process can actually read, preferably `GEMINI_API_KEY` or `GOOGLE_API_KEY`, or a provider account home that this CLI accepts
  - unsupported capability flags remain false until live checks pass
- upstream docs updated: `CP-40`, `CA-138`
