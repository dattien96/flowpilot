# BUG-076: Selected Skills Ignored When User Prompts Main Task

## Metadata

- Document ID: `BUG-076`
- Title: `Selected Skills Ignored When User Prompts Main Task`
- Phase: `bugfix`
- Status: `done`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-17`
- Last Updated: `2026-06-17`
- Parent Documents: [Task-044: Desktop Chat Mode Split And Provider Controls](../../08-Task/done/Task-044-Desktop-Chat-Mode-Split-And-Provider-Controls.md), [SD-06: AI Provider Integration](../../06-System-Tech-Design/SD-06-AI-Provider-Integration.md), [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)
- Child Documents: `none`
- Related Documents: [BUG-063: Desktop Chat Controls Not Applied To Provider Runs](./BUG-063-Desktop-Chat-Controls-Not-Applied-To-Provider-Runs.md), [BUG-071: Codex YOLO-Off Workspace Write And Selected Skill Names Regressed](./BUG-071-Codex-Yolo-Off-Workspace-Write-And-Selected-Skill-Names-Regressed.md), [CA-091: Fix Selected-Skill Injection Order and Codex Multi-Skill Truncation](../../../change-audit/CA-091-fix-selected-skill-injection-order-and-codex-multiskill-truncation.md)
- Replaces: `none`
- Tags: `desktop-chat, skills, claude, codex, regression, medium`

## AI Quick View

### Summary

- When a skill was selected and the user sent their actual task, the AI could silently ignore the skill and respond with its default behavior.
- Root cause 1: skill content was appended AFTER the user prompt; once the AI read the task and formed a plan, the trailing skill blocks were treated as optional context rather than mandatory process.
- Root cause 2: `claudeAskUserReinforcement` / `askUserReinforcement` appeared after the skill content and instructed the model to "complete the clear, unambiguous parts directly", actively undercutting the skill when the task was already legible.
- Root cause 3 (Codex): `codex_adapter.go` extracted only `req.SelectedSkills[0]` before calling `codexTurnStartParams`, silently dropping all skills after the first from the native `turn/start` protocol field.

### Current Ask

- Done. Skill content is now prepended before the user prompt so the model reads process constraints before forming its response plan.
- Done. Instruction wording strengthened from "Read and apply them" to "You MUST follow the process defined in the selected skill(s) below before responding to the user's request."
- Done. Codex `codexTurnStartParams` now accepts the full `[]SkillSelection` slice and emits all skill names; `selectedSkills[0]` truncation removed.

### Key Decisions

- `V-1` Skill header + blocks must appear before the user's prompt text in the final turn string sent to every provider.
- `V-2` Instruction wording must be imperative ("MUST follow") so the model treats the skill as a mandatory constraint, not advisory guidance.
- `V-3` For Codex's native `turn/start` `skill` field: single skill keeps the existing `"skill": name` shape (backward compat); multiple skills uses `"skills": [names]` (extended protocol field).
- `V-4` The `askUserReinforcement` suffix remains after the user prompt and does not contradict the skill because it concerns `ask_user` tool usage, not whether to apply the skill.

### Constraints

- Do not change `askUserReinforcement` or `claudeAskUserReinforcement` text — these belong to the ask_user guidance contract, not skill enforcement.
- Do not alter the skill file content or the picker's resolution logic.
- Keep the skill-injection test assertions backward compatible; add ordering assertion instead of replacing existing content checks.

### Open Questions

- None.

### Source Refs

- User report `2026-06-17`: selecting a skill and prompting "use this skill" worked, but prompting the actual task caused the AI to ignore the selected skill.
- Investigation confirmed skill was tail-appended and reinforcement contradicted it; Codex additionally truncated to `selectedSkills[0]`.

## 1. Issue Summary

When the user selected one or more skills in the desktop chat skill picker and sent their real task (e.g. "fix this bug" or "implement this feature"), the AI provider could complete the task using its default behavior without following the skill's defined process. The selected skill was only reliably applied when the user explicitly said "use this skill", because that phrasing forced the model to look at the trailing skill content.

A secondary defect existed in the Codex path: regardless of how many skills the user selected, `codex_adapter.go` always extracted only the first element of `req.SelectedSkills` and passed a single `*SkillSelection` to `codexTurnStartParams`. All skills after the first were silently dropped from the Codex native `turn/start` protocol field. This compounded the instability since the prompt injection (via `promptPrep`) did carry all skills, but the native Codex skill hint was always truncated to one.

## 2. Parent Links

- impacted coding plan: [Task-044: Desktop Chat Mode Split And Provider Controls](../../08-Task/done/Task-044-Desktop-Chat-Mode-Split-And-Provider-Controls.md)
- impacted tech design: [SD-06: AI Provider Integration](../../06-System-Tech-Design/SD-06-AI-Provider-Integration.md)
- impacted system spec: [SS-11: Workflow With Session](../../05-System-Specs/SS-11-Workflow-With_Session.md)

## 3. Environment and Reproduction

- environment: desktop app, Claude provider, Codex provider, any number of selected skills ≥ 1
- reproduction steps:
  1. Open desktop chat, select any skill from the picker (e.g. `/cook`)
  2. Type a real task prompt (e.g. "add error handling to X") and send
  3. Observe: AI responds without following the skill's process
  4. Repeat with "use this skill" — AI now follows the skill
- frequency: consistent (every turn where the user prompt was unambiguous without the skill)

## 4. Expected vs Actual

- expected: AI reads and follows the selected skill's process before responding to any user prompt, regardless of how the prompt is worded
- actual: AI followed the skill only when the prompt explicitly referenced it; otherwise it completed the task using default behavior and treated the skill content as trailing optional context

## 5. Impact

- users affected: all desktop chat users who use the skill picker
- workflows affected: any chat turn with a selected skill where the prompt is a direct task
- severity: medium — the feature appeared to work (skills were transmitted, shown in UI chips) but the AI silently ignored them on direct task prompts

## 6. Root Cause

- hypothesis: skill content position in the prompt and weak instruction wording allowed the model to bypass it
- confirmed cause:
  1. `injectSelectedSkills()` in `runner.go` built the final prompt as `userPrompt + skillHeader + skillBlocks`. The model reads leading text first and forms its response plan before reaching the trailing skill content.
  2. `claudeAskUserReinforcement` / `askUserReinforcement` appeared after the skill content with the text "Complete the clear, unambiguous parts of the task directly", which reinforced skipping the skill when the task was already clear.
  3. In `codex_adapter.go`, `var skill = &req.SelectedSkills[0]` silently dropped all skills beyond the first from the native `turn/start` `skill` field.
- evidence:
  - `runner.go:1095` (pre-fix): `return prompt + header + strings.Join(blocks, "")`
  - `claude_adapter.go:82`: `return req.Prompt + claudeAskUserReinforcement` (default path, no skill)
  - `provider_registry.go:320`: `return r.injectSelectedSkills(...) + claudeAskUserReinforcement` — reinforcement after skill
  - `codex_adapter.go:128–130` (pre-fix): `var skill *SkillSelection; if len(req.SelectedSkills) > 0 { skill = &req.SelectedSkills[0] }`

## 7. Fix Strategy

- `F-1` `runner.go` — `injectSelectedSkills`: flip order to `skillHeader + skillBlocks + "---" + userPrompt`. Change instruction from "Read and apply them" to "You MUST follow the process defined in the selected skill(s) below before responding to the user's request." Update doc comment from "appends" to "prepends".
- `F-2` `codex_adapter.go` — `SendTurn`: remove the `var skill *SkillSelection` block entirely. Pass `req.SelectedSkills` (full slice) directly to `codexTurnStartParams`.
- `F-3` `codex_appserver.go` — `codexTurnStartParams`: change signature from `skill *SkillSelection` to `skills []SkillSelection`. Iterate all names; emit `p["skill"] = names[0]` for single skill (backward compat), `p["skills"] = names` for multiple.
- `F-4` `skill_injection_test.go` — add ordering assertion: `strings.Index(out, "## Selected Skills") < strings.Index(out, userPromptText)` to lock the prepend contract.

## 8. Validation

- `V-1` `go test ./internal/runner/... -run TestInjectSelectedSkillsDeliversSelection` — PASS. New ordering assertion confirms skill header precedes user prompt text.
- `V-2` `go test ./internal/runner/... -run TestCodexTurnStart` — PASS. Both image-attachment tests pass with nil `[]SkillSelection` argument (nil slice is valid in Go).
- `V-3` `go build ./...` in `apps/local-runner` — clean build, no errors.
- `V-4` Manual review: `codexTurnStartParams` with 1 skill emits `p["skill"] = name` (backward compat confirmed by inspection). With 2+ skills emits `p["skills"] = []string{...}`.

## 9. Regression Guard

- tests: `TestInjectSelectedSkillsDeliversSelection` now asserts ordering (prepend); will fail if anyone reverts to append order
- alerts: none required — pure prompt-assembly logic, no runtime metrics
- audit checks: `CA-091` records the exact files and lines changed

## 10. Follow-Up Document Updates

- upstream docs that must change: none — the fix corrects implementation without changing the stated spec contract in SD-06 or SS-11
- notes left unchanged on purpose: `claudeAskUserReinforcement` / `askUserReinforcement` text is unchanged; it applies after the user's task text and does not contradict the skill header that now leads the prompt
