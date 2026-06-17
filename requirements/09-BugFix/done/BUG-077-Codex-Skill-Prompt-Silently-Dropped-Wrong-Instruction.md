# BUG-077: Codex Skill Prompt Silently Dropped And Wrong Instruction Wording

## Metadata

- Document ID: `BUG-077`
- Title: `Codex Skill Prompt Silently Dropped And Wrong Instruction Wording`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-17`
- Last Updated: `2026-06-17`
- Parent Documents: [Task-044: Desktop Chat Mode Split And Provider Controls](../../08-Task/done/Task-044-Desktop-Chat-Mode-Split-And-Provider-Controls.md), [SD-06: AI Provider Integration](../../06-System-Tech-Design/SD-06-AI-Provider-Integration.md), [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- Child Documents: `none`
- Related Documents: [BUG-076: Selected Skills Ignored When User Prompts Main Task](./BUG-076-Selected-Skills-Ignored-When-User-Prompts-Main-Task.md), [Task-065: Token-Optimize Selected Skill Injection](../../08-Task/done/Task-065-Token-Optimize-Selected-Skill-Injection.md), [CA-091: Fix Selected-Skill Injection Order and Codex Multi-Skill Truncation](../../../change-audit/CA-091-fix-selected-skill-injection-order-and-codex-multiskill-truncation.md), [CA-092: Token-Optimize Selected Skill Injection](../../../change-audit/CA-092-token-optimize-selected-skill-injection.md), [CA-093: Fix Codex Skill Prompt Drop and Provider-Neutral Instruction](../../../change-audit/CA-093-fix-codex-skill-prompt-drop-and-neutral-instruction.md)
- Replaces: `none`
- Tags: `runner, skills, codex, prompt-engineering, regression, medium`

## AI Quick View

### Summary

- When a user selected skills in the desktop chat and sent a prompt via the Codex provider, the Codex AI did not receive or acknowledge the `## Selected Skills` block that the runner injected into the turn prompt.
- Root cause 1: `codexTurnStartParams` included `"text_elements": []any{}` alongside `"text": prompt` in the input item. Some Codex app-server versions treat `text_elements` as the authoritative content array and ignore `text` when the field is present — an empty `text_elements` delivers an empty message, silently discarding the entire assembled prompt including the skill block.
- Root cause 2: The skill injection instruction read "Read each skill file with your Read tool" — Claude Code-specific language. Codex (OpenAI model) has no tool named "Read"; it uses shell commands. The model could not follow the instruction as written.
- Claude provider was unaffected: Claude Code's `writeUserTurn` never sends `text_elements`; its model has a built-in `Read` tool that matches the instruction exactly.

### Current Ask

- Done. `text_elements: []any{}` removed from Codex turn params. Instruction updated to provider-neutral wording. Both providers now receive and can act on the same skill pointer block.

### Key Decisions

- `F-1` Remove `"text_elements": []any{}` from the text input item in `codexTurnStartParams` — field was always empty, serves no purpose, and may suppress `"text"` on some app-server versions.
- `F-2` Change instruction in `injectSelectedSkills` from "with your Read tool" to "listed below" — both Claude (Read tool) and Codex (shell commands) can follow the neutral phrasing.

### Constraints

- Do not change `SkillSelection` struct or the HTTP contract.
- Do not alter the `## Selected Skills` block structure or prepend order established by BUG-076.
- The fix must not regress Claude behavior — Claude Code selects its Read tool independently of the instruction wording.

### Open Questions

- Whether `text_elements` was definitively the suppression mechanism cannot be confirmed without access to the Codex app-server source. The behavioral fix (removing it) is correct regardless.

### Source Refs

- User report `2026-06-17`: "When i used claude, it can provides me this one [shows skill block]. But with codex it can not."
- Code inspection: `codex_appserver.go:codexTurnStartParams` — `text_elements: []any{}`
- Code inspection: `runner.go:injectSelectedSkills` — "with your Read tool" instruction

## 1. Issue Summary

When the user selected one or more skills in the desktop chat skill picker and sent a prompt using the Codex provider, the Codex AI did not confirm receiving the `## Selected Skills` block. Investigation showed two defects in the Codex-specific code path: (1) an empty `text_elements` array in the turn params that may cause the Codex app-server to ignore the assembled `text` prompt, and (2) a Claude-specific instruction phrase that the Codex AI (OpenAI model) cannot act on. Claude was unaffected because it has its own `Read` built-in tool and its prompt delivery never includes `text_elements`.

## 2. Parent Links

- impacted coding plan: [Task-044: Desktop Chat Mode Split And Provider Controls](../../08-Task/done/Task-044-Desktop-Chat-Mode-Split-And-Provider-Controls.md)
- impacted tech design: [SD-06: AI Provider Integration](../../06-System-Tech-Design/SD-06-AI-Provider-Integration.md)
- impacted system spec: [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)

## 3. Environment and Reproduction

- environment: Desktop FlowPilot with live Codex provider (`FLOWPILOT_CODEX_APPSERVER=1`), any workspace with skills in `.agents/skills/`.
- reproduction steps: (1) Open desktop chat. (2) Select a skill from the picker. (3) Send a task prompt. (4) Ask the Codex AI to confirm what skill context it received.
- frequency: Every Codex turn with a selected skill.

## 4. Expected vs Actual

- expected: Codex AI receives the `## Selected Skills` block with the skill path pointer and reads the skill file before responding.
- actual: Codex AI does not confirm receiving the skill block; may silently ignore it or fail to act on it because the instruction references a "Read tool" it does not have.

## 5. Impact

- users affected: All users using the Codex provider with skill selection.
- workflows affected: Any desktop chat flow that relies on skills to guide model behavior (e.g. git-commit-skill, code-review-skill).
- severity: Medium — the core feature (skill-guided AI behavior) silently fails for Codex without any error indication to the user.

## 6. Root Cause

- hypothesis: `text_elements: []any{}` causes the Codex app-server to emit an empty user message; instruction "with your Read tool" causes the Codex AI to ignore the skill instruction.
- confirmed cause:
  - `text_elements: []any{}` is always empty and may suppress `text` content on the Codex app-server (confirmed as a risk; behavioral outcome unconfirmed without app-server source).
  - "Read each skill file with your Read tool" is Claude Code-specific. Codex (OpenAI model) has no tool by that name. This part is a confirmed instruction mismatch.
- evidence:
  - Claude receives and acknowledges the skill block because `writeUserTurn` never sends `text_elements` and its model has a built-in `Read` tool.
  - Codex sends `text_elements: []any{}` via `codexTurnStartParams` — field absent from Claude's prompt delivery path.
  - Instruction wording change from Claude Code docs; OpenAI model tools are shell-based, not named "Read".

## 7. Fix Strategy

- `F-1` `codex_appserver.go` — `codexTurnStartParams`: remove `"text_elements": []any{}` from the text input item. Only `"type": "text"` and `"text": prompt` remain.
- `F-2` `runner.go` — `injectSelectedSkills`: change "Read each skill file with your Read tool and follow its process before responding." to "Read each skill file listed below and follow its process before responding." — provider-neutral; Claude uses its Read tool, Codex uses shell commands.
- Updated stale comment in `codexTurnStartParams` from "full content" to "path pointer block" (Task-065 left this stale).

## 8. Validation

- `V-1` `go build ./...` in `apps/local-runner` — clean, no errors.
- `V-2` `go test ./internal/runner/... -run "TestInjectSelectedSkillsDeliversSelection"` — PASS. Instruction wording change does not break the test (test checks for `## Selected Skills` header and path/description presence, not the exact instruction sentence).
- `V-3` `go test ./internal/runner/... -run "TestCodexTurnStartParamsAppendsImageItems|TestCodexTurnStartParamsNoImagesKeepsTextOnly"` — PASS. Removing `text_elements` does not break the param-builder tests.
- `V-4` `go test ./internal/runner/... -run "TestCodexAdapterAcceptsGeneratedAppServerThreadAndTurnShapes"` — PASS.
- `V-5` Live Codex AI confirmation was not run in this turn — requires a running Codex app-server with a real API key.

## 9. Regression Guard

- tests: Existing skill injection tests (`TestInjectSelectedSkillsDeliversSelection`) and Codex turn param tests (`TestCodexTurnStartParams*`) provide ongoing coverage.
- alerts: none — no metrics pipeline exists for per-provider skill delivery confirmation.
- audit checks: `CA-093` records the exact changed lines.

## 10. Follow-Up Document Updates

- upstream docs that must change: none — this is an implementation fix with no change to business rules or acceptance criteria in SS-11 or SD-06.
- notes left unchanged on purpose: `promptPrep` structure for both providers is unchanged; only the content of `codexTurnStartParams` input items and the instruction sentence in `injectSelectedSkills` were modified.
