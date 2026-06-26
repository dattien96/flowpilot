# CA-091: Fix Selected-Skill Injection Order and Codex Multi-Skill Truncation

## Scope

Two related defects in the skill injection pipeline that caused selected skills to be silently ignored
by the AI when the user sent a direct task prompt:

1. **Prompt ordering** (Claude + Codex): `injectSelectedSkills()` appended skill content after the
   user prompt. The model read the task first and formed its plan before reaching the skill blocks,
   treating them as trailing optional context rather than mandatory process constraints.

2. **Codex multi-skill truncation**: `codex_adapter.go` extracted only `req.SelectedSkills[0]` and
   passed a single `*SkillSelection` to `codexTurnStartParams`. All selected skills beyond the first
   were silently dropped from the native Codex `turn/start` protocol field.

## Completed

### `apps/local-runner/internal/runner/runner.go`

- `injectSelectedSkills()`: changed final return from `prompt + header + blocks` to
  `header + blocks + "\n\n---\n\n" + prompt`. Skill content now leads; user task follows.
- Instruction wording strengthened: `"You MUST follow the process defined in the selected skill(s)
  below before responding to the user's request."` (was `"Read and apply them"`).
- Doc comment updated: "appends" → "prepends".

### `apps/local-runner/internal/runner/codex_adapter.go`

- Removed `var skill *SkillSelection` block and the `selectedSkills[0]` extraction entirely.
- `codexTurnStartParams` call now receives `req.SelectedSkills` (the full `[]SkillSelection` slice).

### `apps/local-runner/internal/runner/codex_appserver.go`

- `codexTurnStartParams` signature changed from `skill *SkillSelection` to `skills []SkillSelection`.
- Logic: iterates all names; 1 skill → `p["skill"] = name` (backward compat with existing Codex
  protocol); 2+ skills → `p["skills"] = names` (extended array field).

### `apps/local-runner/internal/runner/skill_injection_test.go`

- Added ordering assertion: `strings.Index(out, "## Selected Skills") < strings.Index(out, "do the task")`
  to lock the prepend contract and catch any regression back to append order.

## Verification

- `go test ./internal/runner/... -run "TestInjectSelectedSkills|TestCodexTurnStart"` — all PASS.
- `go build ./...` in `apps/local-runner` — clean build.
- `image_attachment_test.go` calls `codexTurnStartParams(..., nil, ...)` — nil is valid for
  `[]SkillSelection`; both image tests pass without modification.

## Residual Notes

- The `claudeAskUserReinforcement` / `askUserReinforcement` suffix is unchanged. It appears after the
  user's task text and governs `ask_user` tool usage; it no longer contradicts the skill since the
  skill header now leads the entire prompt.
- The Codex native `p["skills"]` array field is new; the real Codex app-server may not yet consume it,
  but unknown fields are ignored by JSON-RPC callers. The prompt text injection via `promptPrep` already
  delivers all skill content reliably for both providers.
- BugFix document: `requirements/09-BugFix/done/BUG-076-Selected-Skills-Ignored-When-User-Prompts-Main-Task.md`

# ---8<--- flowpilot:change-ledger
feature_key: skill-injection
source_doc_id: BUG-076
change_type: fix
summary: Fix Selected-Skill Injection Order and Codex Multi-Skill Truncation
# --->8---
