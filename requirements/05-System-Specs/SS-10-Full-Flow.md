# SS-10: Full Flow

## 1. Goal

Define the end-to-end product flow for how a user starts and runs AI work inside a project.

FlowPilot is not a chat-first system.
It is a project-based workflow runner where:

- a user owns many projects
- each project can be linked to many real local working directories
- each project can start many workflow runs
- each workflow run executes one workflow definition or one selected step
- execution is coordinated by the Go runner

The main purpose of this spec is to describe:

- the launch flow from the project detail page
- the runtime responsibilities of the Go runner
- the role of step definitions
- how artifacts are passed between steps
- how model selection works per step
- how the runner chooses the correct CLI command for each model family

## 2. Core Mental Model

The system model is:

`Project -> Workflow Definition -> Workflow Run -> Step Execution`

Important clarification:

- a project is not a chat thread
- a workflow run is not a conversation
- the workflow run is the execution unit
- each step execution is controlled by its step definition

## 3. Project Requirements

Each project create flow must collect at least one required local path when created.

Example:

```text
C:/working/xxx_project
```

That path is mandatory during create flow because the system must establish the first directory binding for the project.

The shared project record should store:

- project id
- name
- description
- platform
- repository url if available

The directory binding record should store:

- project id
- local path
- optional label

The runtime rule is:

- one shared project
- many local workspace paths
- bindings are unique by `(project_id, local_path)`

The system must not rely on one global shared `project_path` as the only workspace source of truth.

## 3.1 Directory binding resolution

When workflow execution starts:

1. load all directory bindings for `project_id`
2. Golang runner checks each path
3. if one path is usable, run there
4. if no path is usable, fail the trigger and notify the root cause

## 4. Project Detail Page as Execution Entry Point

The main project detail page is the place where users trigger workflow execution.

The page must provide one primary workflow trigger component with 3 start modes:

1. Start from workflow definition
2. Start from single step
3. Create new workflow

### 4.1 Start From Workflow Definition

The user selects a workflow definition from a dropdown list and starts a new workflow run from that definition.

The begin prompt is passed to the first enabled step in the workflow.

### 4.2 Start From Single Step

The user selects one step definition from a dropdown list and starts execution directly from that step.

This mode is useful for targeted execution such as:

- product spec only
- tech spec only
- code review only
- analytics review only

The begin prompt is passed directly to the selected step.

### 4.3 Create New Workflow

The user creates a new workflow definition from the project page and can then immediately start it or save it for later use.

## 5. Begin Prompt

Before starting any workflow run, the UI must allow the user to provide a begin prompt.

Example:

```text
I want to implement the login feature
```

This prompt is the seed input for execution.

Behavior:

- if the user starts from a workflow definition, the prompt is passed to the first step
- if the user starts from a single step, the prompt is passed to that step directly

This prompt is not a chat message.
It is execution input.

## 6. Step Definition

Step Definition is the main orchestration contract used by the Go runner.

Each step definition may contain:

- `step_type`
- `name`
- `description`
- `team_role`
- `prompt_base`
- `required_mcps`
- `skills`
- `subagent`
- `input_artifacts`
- `output_artifacts`
- `model`
- optional provider or execution overrides
- optional approval requirements

### 6.1 Skills

`skills` is a text-linked list of skills used to enrich or constrain the step behavior.

Skills are prompt/runtime guidance.
They are not the execution engine.

### 6.2 Subagent

`subagent` is a text-linked value on the step definition, similar to how `skills` is stored.

If a step defines a subagent, the runner must include subagent execution intent in the assembled prompt and execution contract for that step.

Subagent is different from skills:

- `skills` provide supporting instructions or context
- `subagent` defines which delegated agent role should execute the task

### 6.3 Model

Each step definition must be able to define a `model` value.

This model is selected from a dropdown list in the UI.

Supported model options are:

- `gemini-flash`
- `gemini-pro`
- `claude-haiku`
- `claude-sonnet`
- `claude-opus`
- `gpt-5.4-mini`
- `gpt-5.4`
- `gpt-5.5`

The selected model must be stored as part of the step definition and used at runtime by the Go runner.

### 6.4 Prompt Base

Each step definition may define a `prompt_base` that describes the step-specific execution intent.

The prompt base is different from `skills`:

- `prompt_base` defines the core instruction for this step
- `skills` provide supporting guidance and reusable context

The runner must combine the step prompt base with runtime context when assembling the final prompt.

## 7. Workflow Start Flow

When the user starts a workflow:

1. User opens the project detail page
2. User chooses one of the 3 trigger modes
3. User selects workflow definition or step definition if required
4. User enters begin prompt
5. System creates a workflow run
6. System resolves the first step to execute
7. Go runner receives:
   - project id
   - project directory bindings
   - workflow run id
   - current step definition
   - begin prompt

## 8. Go Runner Responsibilities

The Go runner is the runtime orchestrator.

For each step execution, the runner must:

1. load all directory bindings for the project
2. check each bound path and choose one usable path
3. stop execution if no usable local path exists
4. load the current step definition
5. check required MCPs
6. verify the project has those MCPs enabled
7. stop execution and return a user-facing error if a required MCP is missing
8. load linked skills
9. load linked subagent if defined
10. load step prompt base
11. resolve input artifact paths
12. resolve output artifact paths
13. resolve selected model
14. assemble the final prompt
15. map the selected model to the correct provider command
16. run the provider command in the chosen valid project directory
17. capture the response
18. save output artifact if configured
19. continue to the next step if one exists

## 9. MCP Validation Rules

If the current step requires MCP integrations:

1. runner checks the `required_mcps` on the step definition
2. runner checks whether the current project has each MCP enabled
3. if any required MCP is missing:
   - stop the step
   - mark run as blocked or failed based on product rule
   - return a message telling the user which MCP must be enabled

The workflow must not continue when a required MCP dependency is missing.

## 10. Artifact Input and Output Rules

Artifacts are file-based runtime inputs and outputs between steps.

### 10.1 Input Artifacts

If a step defines input artifacts:

- the runner resolves the artifact records
- the runner resolves the physical file paths
- the prompt includes those paths as step context

### 10.2 Output Artifacts

If a step defines output artifacts:

- the runner resolves the target artifact path and artifact name
- the prompt tells the model where output must be saved
- after execution, the runner stores or syncs the output to that artifact target

## 11. Prompt Assembly Contract

The Go runner assembles a runtime prompt from the step definition and project context.

The prompt must include:

- team role
- step prompt base
- chosen valid local path
- selected model
- begin prompt or current user input
- input artifact references
- skills
- subagent if defined
- output artifact destination

Example final prompt structure:

```text
SYSTEM INSTRUCTION
You are a {team_role}.
Provider: {provider}
Model: {model}
This is the chosen valid local path for this run: {resolved_local_path}

WORKFLOW CONTEXT
Workflow: {workflow_name}
Step: {step_name}
Previous steps completed: {previous_step_summaries}

Step prompt base:
{prompt_base}

Please execute the required task with the information below.

1. User input:
{user_input}

2. Input artifacts:
See artifact paths:
{input_artifact_paths}

3. Skills:
{skills}

4. Subagent:
{subagent}

5. MCP Context:
{mcp_context}

6. Selected Working Memory:
{selected_working_memory}

7. Source Artifacts:
{source_artifacts}

8. Raw Artifact Excerpts:
{raw_artifact_excerpts}

9. Reviewer Feedback:
{rejection_note}

10. Output artifact:
Please save the output to:
{output_artifact_path}
```

If no subagent is defined, that section may be omitted.

## 12. Model-to-Provider Routing

Model selection is defined at the step level.

The Go runner must automatically route execution to the correct provider command based on the selected model.

### 12.1 Routing Rules

- models in the `gpt-*` family must route to the Codex command adapter
- models in the `gemini-*` family must route to the Gemini command adapter
- models in the `claude-*` family must route to the Claude command adapter

Example:

- `gpt-5.4-mini` -> use Codex CLI execution
- `gpt-5.4` -> use Codex CLI execution
- `gpt-5.5` -> use Codex CLI execution
- `gemini-flash` -> use Gemini CLI execution
- `gemini-pro` -> use Gemini CLI execution
- `claude-haiku` -> use Claude CLI execution
- `claude-sonnet` -> use Claude CLI execution
- `claude-opus` -> use Claude CLI execution

### 12.2 Command Adapter Responsibility

The Go runner must not hardcode workflow logic separately for each provider.

Instead, it should:

1. identify the model family
2. select the matching provider adapter
3. build the correct command format for that provider
4. execute the provider command

Examples:

- GPT-family step -> run through Codex command
- Gemini-family step -> run through Gemini command
- Claude-family step -> run through Claude command

The exact CLI flags may differ per provider, but routing must happen automatically from the selected step model.

## 13. Working Directory Execution

The Go runner must execute inside the project's local directory.

Runtime behavior:

1. load project directory bindings
2. choose one valid path
3. change working directory to that path
4. invoke the provider command from that directory

This allows the model and tools to work against the real project files.

## 14. Step Completion

After provider execution:

1. runner captures the response
2. if output artifact exists, runner saves the output
3. runner updates step status
4. runner decides whether to continue

Possible outcomes:

- success and continue to next step
- success and finish workflow
- blocked because MCP is missing
- failed because command execution failed
- waiting for approval if approval is required

## 15. Multi-Step Continuation

If the current step is not the last step:

1. runner resolves the next step
2. carries forward required run context
3. uses artifacts produced by previous steps as input where configured
4. repeats the same execution process

The workflow continues until:

- all steps are done
- a step fails
- a step is blocked
- a step waits for approval
- the run is canceled

## 16. UI Requirements Summary

The admin UI must support:

- project creation with one or more initial directory bindings
- project detail page with workflow trigger component
- project detail `Directory Binding` tab
- dropdown for workflow definition selection
- dropdown for single step selection
- begin prompt input
- workflow creation entry
- step definition editor with:
  - skills
  - subagent
  - model dropdown
  - artifact input/output selection
  - MCP requirements

## 17. Acceptance Criteria

- [ ] Project creation requires at least one local path -> Open file system browser to select directory paths and create initial directory bindings
- [ ] One shared project can store many local paths
- [ ] Project detail page has a `Directory Binding` tab for add/edit/delete
- [ ] Missing usable bound path fails execution and returns root-cause guidance
- [ ] Project detail page can start execution in 3 modes
- [ ] Begin prompt is passed to first step or selected single step
- [ ] Step definitions can store `skills`
- [ ] Step definitions can store `subagent`
- [ ] Step definitions can store `model`
- [ ] Model is selected from a dropdown in the UI
- [ ] Supported model list includes Gemini, Claude, and GPT variants defined in this spec
- [ ] Go runner includes selected model in prompt assembly
- [ ] Go runner automatically maps model family to the correct provider command
- [ ] GPT-family models use Codex command execution
- [ ] Gemini-family models use Gemini command execution
- [ ] Claude-family models use Claude command execution
- [ ] Runner loads project bindings and executes inside one chosen valid bound local path
- [ ] Missing required MCP stops workflow execution with user-visible guidance
- [ ] Input artifacts are resolved and passed to the step
- [ ] Output artifacts are written after step execution
- [ ] Multi-step workflows can continue step by step until completion or interruption
