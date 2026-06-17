# CA-093: Fix Codex Skill Prompt Drop and Provider-Neutral Instruction

## Scope

Two defects in the Codex turn prompt delivery path caused selected skills to be silently ignored
when using the Codex provider:

1. `codexTurnStartParams` included `"text_elements": []any{}` alongside the assembled prompt text.
   Some Codex app-server versions treat `text_elements` as the authoritative content source and
   ignore `text` when the field is present — an empty array delivers a blank user message, dropping
   the entire `## Selected Skills` block.

2. `injectSelectedSkills` used the instruction "Read each skill file with your Read tool" — wording
   specific to Claude Code's built-in `Read` file tool. The Codex AI (OpenAI model) has no tool
   named "Read" and uses shell commands instead, so the instruction was unactionable.

Claude was not affected: `writeUserTurn` never sends `text_elements`, and Claude Code's model has
a built-in `Read` tool that matches the original instruction.

## Completed

### `apps/local-runner/internal/runner/codex_appserver.go`

- `codexTurnStartParams`: removed `"text_elements": []any{}` from the text input item. The text
  item now contains only `"type": "text"` and `"text": prompt`. The field was always empty and
  served no purpose; removing it ensures the assembled prompt (including the skill block) is not
  suppressed by app-server implementations that prefer `text_elements` as the content source.
- Updated stale comment: "the prompt already carries their full content via injectSelectedSkills"
  → "the prompt already carries the path pointer block via injectSelectedSkills" (Task-065
  changed full-content embedding to path pointers; the comment was not updated at the time).

### `apps/local-runner/internal/runner/runner.go`

- `injectSelectedSkills`: changed instruction from
  "Read each skill file with your Read tool and follow its process before responding."
  to
  "Read each skill file listed below and follow its process before responding."
  The new wording is provider-neutral: Claude Code selects its Read tool independently of the
  phrase "your Read tool"; Codex and other providers use shell commands to read files.

## Verification

- `go build ./...` in `apps/local-runner` — clean.
- `go test ./internal/runner/... -run "TestInjectSelectedSkillsDeliversSelection"` — PASS.
- `go test ./internal/runner/... -run "TestCodexTurnStartParamsAppendsImageItems|TestCodexTurnStartParamsNoImagesKeepsTextOnly"` — PASS.
- `go test ./internal/runner/... -run "TestCodexAdapterAcceptsGeneratedAppServerThreadAndTurnShapes"` — PASS.
- Live end-to-end verification (Codex AI confirms receiving skill block) not run in this turn —
  requires a live Codex binary and API key.

## Residual Notes

- The root cause of `text_elements` suppressing `text` cannot be fully confirmed without the
  Codex app-server source. Removing the always-empty field is correct regardless of suppression
  behavior.
- Bug document: `requirements/09-BugFix/done/BUG-077-Codex-Skill-Prompt-Silently-Dropped-Wrong-Instruction.md`
