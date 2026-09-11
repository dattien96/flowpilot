# CP-23: Context Control, Wrong-Way Detection, and Mistake-to-Skill Learning

**Maps from:** Product Vision, SP-02, SP-03, SS-09, SD-05, SD-10, CP-10, CP-11, CP-21
**Phase:** Cross-cutting runtime intelligence
**Depends on:** CP-10, CP-11, CP-21
**Supersedes:** CP-24, CP-25, CP-39 (token/prompt optimize one-liner)

## 1. Core Concept

These three topics should be delivered as one runtime intelligence plan, not as three unrelated features:

1. Context control keeps prompts small and relevant.
2. Wrong-way detection monitors whether the agent is making progress or drifting.
3. Mistake-to-skill learning turns repeated failures into reusable guidance.

The dependency order matters:

- Wrong-way detection is noisy if context quality is poor.
- Auto-learning is dangerous if the system cannot distinguish a real repeated mistake from a one-off failure.
- Therefore the system must first control context, then detect drift, then promote repeated lessons into reusable skill rules.

## 2. Why This Must Be One CP

Keeping CP-23, CP-24, and CP-25 separate creates three problems:

- it hides the fact that all three depend on the same prompt assembly and session telemetry
- it encourages duplicate schema and logging work
- it makes rollout order unclear

This umbrella CP keeps one shared architecture, one set of audit tables, one telemetry model, and one rollout plan with three phases.

## 3. Problem Statement

Today the workflow runtime still has these gaps:

- prompt assembly is mostly additive, so context can grow larger than needed
- skill injection can become expensive because full markdown skill files are appended at runtime
- follow-up sessions can continue for too long without a clear notion of "progress"
- repeated model mistakes are visible to the user but are not turned into durable project guidance

This makes FlowPilot weaker at its core promise:

- reliable workflow execution
- explicit context
- cross-session engineering memory
- reduced token waste
- self-correcting autonomy with human-safe controls

## 4. Shared Architecture

The solution is one runtime loop with three layers:

1. `Context Resolver + Budget Packer`
2. `Drift / Wrong-Way Detector`
3. `Lesson Candidate -> Skill Promotion`

### 4.1 Runtime loop

For every workflow step or follow-up message:

1. Resolve mandatory context.
2. Retrieve relevant working memory.
3. Pack prompt sections into a fixed token budget.
4. Execute provider step.
5. Record telemetry about prompt size, outputs, retries, and progress.
6. Run wrong-way detection on the result.
7. If drift is detected, choose a correction action.
8. If repeated drift patterns accumulate, create or update a lesson candidate.
9. Promote only approved or repeated high-confidence lessons into runtime skill rules.

### 4.2 Shared principles

- Default to summaries, not raw history.
- Favor deterministic rules before LLM judgement.
- Use LLM judgement only as a secondary classifier when heuristics fire.
- Never auto-create a permanent skill from one bad run.
- Every selected context item and every correction action must be auditable.

## 5. Phase 1 - Auto Size-Down Context

### 5.1 Goal

Make the final prompt as small as possible while preserving the minimum context required for the current step.

### 5.2 Scope

Build a proper `Context Resolver + Budget Packer` instead of a simple truncation step.

### 5.3 Required behavior

- resolve context by step policy, not by blind concatenation
- reserve budget per prompt section
- prefer `artifact_memories` summaries over raw artifacts
- include raw excerpts only when the summary is insufficient
- deduplicate overlapping context items
- record exactly which context items were selected
- persist the final assembled prompt after skill/context injection for auditability

### 5.4 Packing order

Suggested priority from highest to lowest:

1. system and execution contract
2. current task / follow-up request
3. mandatory workflow context
4. pinned or required context sources
5. selected working memory summaries
6. raw artifact excerpts
7. optional skill guidance

### 5.5 Shared data model

Use the architecture already planned in CP-10 and SD-10:

- `artifact_memories`
- `step_context_slots`
- `workflow_prompt_context_items`

Add runtime metrics if missing:

- `prompt_token_estimate`
- `selected_context_token_estimate`
- `dropped_context_count`
- `packed_context_strategy`

### 5.6 Runner changes

- move away from full-file skill injection as the only strategy
- support compact runtime skill cards for prompt packing
- keep the current full markdown skill file as fallback for explicitly required skills
- persist the final packed prompt, not only the raw user prompt

### 5.7 Output

Each step should produce a prompt context audit that answers:

- what context was eligible
- what context was selected
- what context was dropped
- why it was dropped
- what token budget was used

## 6. Phase 2 - Wrong-Way Detection

### 6.1 Goal

Detect when the workflow is drifting, looping, wasting tokens, or editing in the wrong direction, then trigger a safe correction path.

### 6.2 Detection model

Use a hybrid detector:

- primary: deterministic heuristics
- secondary: lightweight classifier / judge model only when heuristics trigger

This avoids paying LLM cost on every turn while still handling nuanced cases.

### 6.3 Drift signals

Minimum signals for MVP:

- repeated agreement filler such as "you are absolutely right"
- repeated apologies or circular explanations
- same failed command or test executed multiple times without a new approach
- no artifact delta after several turns
- output shape does not match the current step contract
- edits outside the declared scope
- token spend grows while useful progress stays flat
- repeated retries against the same failure root cause

### 6.4 Detection score

Each signal contributes to a `drift_score`.

Suggested severity bands:

- `0-29`: healthy
- `30-59`: warning
- `60-79`: drifted
- `80+`: blocked or unsafe

### 6.5 Correction ladder

The runtime should not jump directly to rollback. Use an escalating ladder:

1. add a corrective system note with the detected issue
2. narrow the active scope and re-run with a smaller context pack
3. switch to isolated reviewer/subagent session
4. pause and require human approval

### 6.6 Safety rule

Do not implement auto-rollback in MVP. Rollback is too risky before the system has strong causality and change ownership logic.

### 6.7 Logging and observability

For each detection event, record:

- `drift_score`
- triggered rules
- evidence snippets
- chosen correction action
- whether the correction succeeded

This should live alongside the existing session and AI output logs.

## 7. Phase 3 - Auto Learn To Skill

### 7.1 Goal

Convert repeated, verified failure patterns into reusable project guidance without polluting prompt context or overfitting to one bad run.

### 7.2 Learning model

Do not write permanent `.md` skills directly from a single runtime event.

Use this promotion ladder:

1. mistake event
2. lesson candidate
3. approved runtime rule
4. durable project skill

### 7.3 Lesson candidate shape

Each candidate should store:

- title
- scope
- trigger pattern
- anti-pattern
- preferred corrective behavior
- confidence
- repeat count
- supporting run IDs
- supporting step IDs
- status: `candidate`, `approved`, `rejected`, `promoted`

### 7.4 Promotion rules

- one event creates a candidate only
- repeated confirmed events can auto-approve into a runtime rule
- permanent skill generation requires human approval in MVP
- approved skills should be small, rule-focused, and easy to inject selectively

### 7.5 Output forms

There are two valid outputs:

- compact runtime rule card used by the budget packer
- durable markdown skill file synced with project skills

The compact rule card is the default runtime artifact. The markdown skill file is the long-term knowledge form.

### 7.6 Guardrails

- never promote from low-confidence evidence
- never promote if the issue was caused by missing context only
- never promote style preference as if it were engineering law
- allow users to reject a bad lesson and suppress future promotion for that pattern

## 8. Shared Schema Additions

If not already covered elsewhere, add runtime intelligence tables or equivalent models for:

- `workflow_drift_events`
- `workflow_lesson_candidates`
- `workflow_skill_rules`

Suggested fields:

### 8.1 `workflow_drift_events`

- id
- workflow_run_id
- workflow_run_step_id
- provider
- model
- drift_score
- triggered_signals jsonb
- evidence_summary
- correction_action
- correction_status
- created_at

### 8.2 `workflow_lesson_candidates`

- id
- project_id
- source_drift_event_id
- title
- scope
- trigger_pattern
- anti_pattern
- preferred_behavior
- confidence
- repeat_count
- supporting_run_ids jsonb
- status
- created_at
- updated_at

### 8.3 `workflow_skill_rules`

- id
- project_id
- lesson_candidate_id
- title
- rule_text
- applies_to_step_types jsonb
- applies_to_provider_keys jsonb
- priority
- is_active
- created_at
- updated_at

## 9. Runner and Prompt Assembly Changes

### 9.1 Prompt assembly

Prompt assembly should evolve from "append everything" into "pack only what is justified."

Required changes:

- support per-section budget allocation
- pack compact skill rules ahead of full skill markdown
- support context deduplication before final assembly
- persist assembled prompt text after all runtime injection

### 9.2 Session awareness

Leverage the session telemetry work from CP-21:

- retry counts
- checkpoints
- session reuse
- session death / resume events

These signals are useful inputs to wrong-way detection.

### 9.3 Artifact awareness

Use artifact deltas and output versions as evidence of progress:

- if tokens grow but artifact output does not materially change, the session may be drifting
- if a corrective prompt produces a real artifact delta, mark the correction as successful

## 10. Admin Web and Visibility

### 10.1 Prompt Context drawer

Show:

- packed sections
- selected context items
- dropped items
- budget used per section

### 10.2 Drift diagnostics

Show:

- drift score timeline
- triggered signals
- correction actions
- before/after result

### 10.3 Lesson review

Show:

- candidate lessons
- evidence runs
- approve / reject / suppress actions
- promoted skill rules

## 11. Delivery Plan

### Step 1 - Context control

- finalize prompt budget model
- implement context packing and audit rows
- persist fully assembled prompts
- add Prompt Context drawer visibility

### Step 2 - Wrong-way detection

- add drift telemetry schema
- implement heuristics
- add optional judge model path
- implement correction ladder and UI diagnostics

### Step 3 - Mistake-to-skill learning

- add lesson candidate schema
- aggregate repeated drift patterns
- add human review flow
- generate compact runtime rules and optional markdown skills

## 12. Risks

### Risk 1 - Over-compression

Too much context shrinking can remove critical information.

Mitigation:

- mandatory context slots
- pinned context
- explicit audit trail for dropped items

### Risk 2 - False positives in drift detection

The system may classify slow but correct work as drift.

Mitigation:

- heuristic thresholding
- evidence-based scoring
- lightweight judge only after heuristic trigger

### Risk 3 - Bad lessons become permanent rules

Poor promotion logic can teach the system the wrong habits.

Mitigation:

- candidate-first promotion ladder
- confidence thresholds
- human approval before durable skill creation

## 13. Definition of Done

### Phase 1 - Context control

- final prompt size is measurably smaller for equivalent runs
- prompt packing uses selected memory, not full history by default
- `workflow_prompt_context_items` or equivalent audit rows are written
- assembled prompts are persisted after runtime injection
- Admin Web can inspect selected and dropped context

### Phase 2 - Wrong-way detection

- drift score is recorded for workflow steps and follow-ups
- repeated loop patterns are detected with deterministic rules
- corrective actions can be applied without manual DB intervention
- Admin Web shows evidence and correction history

### Phase 3 - Auto-learning

- repeated mistakes create lesson candidates
- lesson candidates can be reviewed and promoted
- promoted rules are injected selectively at runtime
- permanent skill creation requires approval in MVP

### Shared

- context, drift, and lesson data remain auditable
- token usage is reduced without losing required context
- the system improves future runs without silently mutating project behavior

## 14. Notes

- CP-24, CP-25, and CP-39 are **closed redirect stubs** under `07-Coding-Plan/done/` (2026-09-11 doc cleanup).
- Empty `CP-26-Auto-Model-Reasoning` stub is **withdrawn** in `done/` — do not confuse with `done/CP-26-Env-Liked-Proxy-System.md`.
- This CP is the single source of truth for runtime context control, drift correction, and mistake-to-skill learning.
