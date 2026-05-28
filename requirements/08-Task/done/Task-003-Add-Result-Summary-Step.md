# TASK-003: Add Result Summary Step

## Goal
For workflows with multiple steps, add one final step that summarizes what happened across the whole flow after all normal steps are done.

This summary step is intended to give the user one clear final result instead of forcing them to read every step output individually.

## Problem
Today, a multi-step workflow can finish with several completed outputs, logs, and session messages, but there is no dedicated final step that explains:
- what was completed
- what the important outcomes were
- what decisions or blockers appeared
- what the user should look at next

That makes the end of the workflow feel fragmented, especially for long flows.

## Expected Behavior
- When a workflow has 2 or more main steps and all required steps are completed, the flow should run one last summary step.
- The summary step should read the outputs from earlier completed steps.
- The summary step should produce a concise high-level explanation of the overall workflow result.
- The workflow run detail page should show one dedicated `Summary` item in the left sidebar.
- When the user selects `Summary`, the right side should show the final summary text directly.

## Summary Step Purpose
The final summary step should answer:
- what this workflow did
- what each important completed step contributed
- the most important final outputs or conclusions
- any unresolved issue, warning, or follow-up item

This is a workflow-level summary, not just a copy of the last step output.

## Built-In Step Rule
- Create this as a built-in step type, for example `result_summary`.
- The step is not manually added by users in the workflow editor.
- The step is automatically appended to the effective runtime execution plan.
- The step should not be persisted into the saved workflow definition as a normal authored step.

## Scope Rule
- Apply this when the workflow has 2 or more main steps.
- Do not add the summary step for one-step workflows.

## Suggested Step Behavior
- Step type can be a dedicated final step such as `result_summary`.
- It should run only after all main workflow steps that matter for the final result are completed.
- It should be appended automatically to the end of the runtime step list.
- It should gather input from:
  - completed step outputs
  - workflow logs if useful
  - approval outcomes if they affect the final state
- The generated output should be human-readable Markdown.
- It should call AI one final time to generate the workflow-level summary.

## UX Expectation
- The workflow run detail page should show a dedicated `Summary` item at the end of the left sidebar.
- The summary should help the user understand the full run without opening every earlier step.
- The summary should be easy to scan and should not be overly verbose.
- The summary view should focus on the generated summary text instead of the normal session/chat timeline UI.

## Output Expectation
The final summary output should usually contain:
- overall objective
- completed work
- key decisions
- final deliverables or produced artifacts
- open risks or remaining follow-up items

## Acceptance Criteria
- A workflow with 2 or more main completed steps produces one final summary step at the end.
- The summary step is built-in and auto-appended by the runtime.
- The summary step runs only after the main steps are done.
- The summary uses earlier step results as input.
- The summary appears in workflow run detail as the final `Summary` item.
- The summary output gives a useful high-level explanation of the overall run.
- One-step workflows do not automatically get this extra summary step.
