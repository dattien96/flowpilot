# CP-15-04: Workflow Start And Runner Runtime

## 1. Goal

Wire workflow launch and Go-runner execution around project directory bindings and the step-defined execution contract.

## 2. Core Rule

The runner must execute using one valid bound local path for the project, not a single globally shared project path.

## 3. Scope

This CP covers:

- workflow start contracts
- project detail launch modes
- begin prompt delivery
- runner path resolution
- model-to-provider command mapping
- MCP gating
- artifact IO
- multi-step continuation

## 4. Required Changes

### 4.1 Start Modes

Project detail page must support:

1. start from workflow definition
2. start from single step
3. create new workflow

### 4.2 Workflow Start Payload

The start flow must include enough information for runtime kickoff:

- `project_id`
- `start_mode`
- `workflow_id` for workflow-definition mode
- `step_type` or `step_definition_id` for single-step mode
- `begin_prompt`

### 4.3 Begin Prompt

Rules:

1. begin prompt is required for workflow-definition mode
2. begin prompt is required for single-step mode
3. begin prompt reaches the first executed step

### 4.4 Runner Resolution Sequence

For each step execution:

1. load workflow run
2. load project
3. load all directory bindings for the project
4. check each bound path and choose one usable local path
5. stop if no usable local path exists
6. load step definition
7. validate selected model
8. resolve model family
9. validate required MCPs
10. stop immediately if a required MCP is missing
11. resolve skills
12. resolve subagent
13. resolve step prompt base
14. resolve input artifact paths
15. resolve output artifact target path
16. assemble final prompt
17. execute in the chosen valid local path
18. capture response
19. write output artifact
20. continue to next step if allowed

### 4.5 Prompt Assembly Contract

Prompt must include:

- `team_role`
- step prompt base
- chosen valid local path
- selected `model`
- begin prompt or current execution input
- input artifact paths
- skills
- subagent if defined
- output artifact path

### 4.6 Model Routing Rules

Mandatory mapping:

- `gpt-*` -> Codex command execution
- `gemini-*` -> Gemini command execution
- `claude-*` -> Claude command execution

The prompt base must be step-specific so each step can describe its own purpose before runtime context is attached.

### 4.7 MCP Failure Behavior

If a required MCP is missing:

1. runner must not invoke the provider
2. workflow step stops
3. user receives visible guidance about which MCP is missing

### 4.8 Artifact IO

Input artifacts:

1. resolve configured input artifacts
2. map them to actual file paths
3. pass them into prompt/runtime context

Output artifacts:

1. resolve configured output artifact target
2. write output after successful execution
3. update artifact records

### 4.9 Multi-Step Continuation

If execution succeeds and more steps remain:

1. resolve next step
2. allow next step to consume previous outputs
3. continue until completion or interruption

## 5. Acceptance Criteria

- [ ] project detail page can start execution in 3 modes
- [ ] begin prompt is passed to first step or selected single step
- [ ] Go runner includes selected model in prompt assembly
- [ ] Go runner includes step prompt base in prompt assembly
- [ ] Go runner automatically maps model family to the correct provider command
- [ ] GPT-family models use Codex command execution
- [ ] Gemini-family models use Gemini command execution
- [ ] Claude-family models use Claude command execution
- [ ] runner loads all bindings for the project and selects one usable path
- [ ] runner executes inside the chosen valid bound local path
- [ ] missing required MCP stops workflow execution with user-visible guidance
- [ ] input artifacts are resolved and passed to the step
- [ ] output artifacts are written after step execution
- [ ] multi-step workflows can continue step by step until completion or interruption

## 6. Exit Condition

This CP is complete when the launch flow, runner binding resolution, provider routing, MCP gating, and artifact-based step execution work together using one valid bound local path.
