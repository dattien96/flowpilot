# CA-015: Inject Skill Content and Seed Step-Specific Prompt Bases

## Scope

The AI orchestration workflow runtime (from CP-09) had partial implementations for context injection and step persona instruction. Specifically, the runtime recorded `skillIds` but did not provide the actual skill file content to the local provider CLI. Additionally, the database `step_definitions` table supported a `prompt_base` column but lacked actual data, causing every workflow step to fall back to a generic prompt structure regardless of the step type.

This change aims to complete both features to ensure accurate, context-rich prompting for all provider models.

## Completed

- **Skill Content Injection (Local Runner):**
  - Updated `runner.go` `ExecutePrompt` function to read the requested `SkillIds`.
  - Added `injectSkillContent` method to lookup matched skill files via `ListSkills()`.
  - The runtime now reads the full markdown file content of each matched skill and appends it to a `## Included Skills` section at the end of the `prompt.txt` file before execution.

- **Step-Specific Prompt Bases (DB Migration):**
  - Added migration `20260525160000_seed_step_specific_prompt_bases.sql`.
  - Populated the `prompt_base` column in `step_definitions` for all 17 available step types (e.g. `business_idea`, `tech_spec`, `task_breakdown`).
  - Drafted distinct personas for each step (e.g. "System Architect", "Requirements Analyst") so the generated workflows use context-aware archetypes instead of generic fallbacks.

## Verification

- Ran `go test -v ./internal/runner` inside the `apps/local-runner` module. Verified that existing tests continue to pass and `runner.go` changes are syntactically sound.
- Validated that the `SkillIds []string` property on `PromptExecutionRequest` correctly aligns with the local runner type definitions.
- Confirmed that the `deriveStepPromptBase` function in `workflow-engine.ts` correctly handles missing `prompt_base` configurations safely, ensuring backwards compatibility if steps without defined prompt bases are added in the future.

## Residual Notes

- **Sub-Agent Execution is Still Hint-Only:** The `subagent` field defined in step definitions is currently rendered only as a textual hint (e.g. `Subagent: code-reviewer`) in the prompt. It does not enforce a dedicated sub-agent loop or separate CLI profile override. This requires a product direction decision before implementation (Option B: CLI Profiles vs Option C: Full Agent Loop).
- The `supabase db reset` command requires Docker Desktop to apply the new migration. The migration file is committed, so the DB will automatically be seeded upon the next local startup.

# ---8<--- flowpilot:change-ledger
feature_key: skill-injection
source_doc_id: CP-09
change_type: feature
summary: Inject Skill Content and Seed Step-Specific Prompt Bases
# --->8---
