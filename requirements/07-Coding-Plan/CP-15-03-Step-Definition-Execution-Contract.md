# CP-15-03: Step Definition Execution Contract

## 1. Goal

Finalize the step-definition fields that drive runtime orchestration so the workflow system stays execution-first instead of chat-first.

## 2. Core Rule

Step definition is the orchestration contract for runtime execution.

Each step definition must be able to declare:

- `team_role`
- `prompt_base`
- `required_mcps`
- `skills`
- `subagent`
- `model`
- input artifact bindings
- output artifact bindings

## 3. Scope

This CP covers:

- schema updates for step runtime fields
- admin UI for create/edit
- field validation
- supported model dropdown values

This CP does not cover:

- project workspace binding model
- multi-device workspace UX
- full Go-runner execution flow

## 4. Required Changes

### 4.1 Data Model

Step definitions must support:

- `step_type`
- `name`
- `description`
- `team_role`
- `prompt_base`
- `required_mcps`
- `skills`
- `subagent`
- `model`
- input artifact bindings
- output artifact bindings

### 4.2 Model Dropdown

The UI dropdown must expose exactly these values:

- `gemini-flash`
- `gemini-pro`
- `claude-haiku`
- `claude-sonnet`
- `claude-opus`
- `gpt-5.4-mini`
- `gpt-5.4`
- `gpt-5.5`

### 4.3 Field Semantics

`skills`

- prompt/runtime guidance
- not the execution engine

`prompt_base`

- step-specific execution intent
- distinct from `skills` and `subagent`

`subagent`

- delegated execution role for the step
- separate from `skills`

`model`

- required runtime selector
- source of truth for provider-family mapping

### 4.4 Admin UI

Create/edit screens must support:

1. `team_role` input
2. `prompt_base` input or template selector
3. `skills` selector or linkage
4. `subagent` input
5. `model` dropdown
6. required MCP selection
7. input artifact selection
8. output artifact selection

### 4.5 Validation

1. `model` is required
2. `subagent` is optional
3. `prompt_base` must be present or derivable from the selected step type
4. invalid model values are rejected
5. required MCP values must map to known MCP definitions

## 5. Acceptance Criteria

- [ ] step definitions can store `skills`
- [ ] step definitions can store `prompt_base`
- [ ] step definitions can store `subagent`
- [ ] step definitions can store `model`
- [ ] model is selected from a dropdown in the UI
- [ ] supported model list includes Gemini, Claude, and GPT variants defined in spec
- [ ] field validation rejects unsupported model values

## 6. Exit Condition

This CP is complete when step definitions carry the runtime fields needed for orchestration and the admin UI can edit them safely.
