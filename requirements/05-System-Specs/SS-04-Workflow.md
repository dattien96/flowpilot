# 1. What is workflow?

From the context that, before this system, when i need AI help for software developmnet, i need to create prompt manually.

Even i have some workflow prompt saved, but the issue is:
- It is not flexible, it always in the same flow, i can not change it to fit my need
- I need to search and pickup Workflow -> mention it in the prompt manually.
- And ofcourse, using it in cli or vscode plugin is not comfortable.

I need a system that allow me
- Dynamically define a workflow for AI
- Configure each step of AI
- Save workflow and reuse
- Run workflow in comfortable way, select and click button then all steps run automatically in background

# 2. Workflow in this system

In this system, workflow is a collection of steps that are executed in order. 

-> That means, we can config many flows base on our step

Workflow definition can exist in 2 scopes:
- Global workflow: reusable template that any project can pick and run
- Private workflow: custom workflow owned by a single project

Workflow run is the execution record of a workflow on a specific project.
Workflow run is started from the project detail page, not from a chat thread.
Workflow run must execute against one valid bound local path for that project.

## 2.1 Workflow Start Model

The project detail page must support 3 workflow trigger modes:

1. Start from workflow definition
2. Start from single step
3. Create new workflow

Before the run starts, the user enters one begin prompt.

Example:

```text
I want to implement the login feature
```

This prompt is passed to:

- the first step of the selected workflow definition
- or the selected single step

This input is execution context, not chat history.

Before a workflow run can complete startup, the Golang runner must confirm that at least one bound local path for the project is valid and accessible.
If not, the trigger must fail and return the root cause to the user.

# 3. Step in workflow - Core component

1 flow created by combining steps
We allow user to run each step manually.
Or we allow they create custom flow from the above existing steps.
That mean we need to have to enable/disable step and also re-oder steps

## 3.1 Basic info of step
- Name
- Description
- Team role
- Skills required
- Subagent
- Model

`Subagent` is a text-linked execution field similar to `skills`, but it means delegated execution role, not supporting context.

`Model` is a step-level runtime selection and must be configurable from a dropdown list.

Supported model options:

- `gemini-flash`
- `gemini-pro`
- `claude-haiku`
- `claude-sonnet`
- `claude-opus`
- `gpt-5.4-mini`
- `gpt-5.4`
- `gpt-5.5`

## 3.2 MCP in Step

If MCP was added in 1 step -> added to the flow then if project use this flow -> must enable this MCP on that project

- MCP servers that can be used by this step
- MCP servers that are required for this step

- Each step can have a required MCP. For example, the 'Feature Intake' step needs Jira MCP to read the ticket

## 3.3 Skills/Agents in Step
Reference SS-06-Workflow-Skill-Agent.md

- Each step can belong with 1-n SKILL
- Each step can optionally define 1 subagent
- Each step can optionally define a step-specific prompt base

Meaning:

- `prompt_base` defines the core instruction for the step
- `skills` provide guidance and reusable prompt context
- `subagent` tells the runner which delegated execution role should be used for that step

## 3.3.1 Model Routing Rule

The selected model controls which provider command the Golang runner must use.

Routing rules:

- `gpt-*` models route to Codex command execution
- `gemini-*` models route to Gemini command execution
- `claude-*` models route to Claude command execution

Example:

- if step model is `gpt-5.4`, the runner must call the Codex command adapter
- if step model is `gemini-pro`, the runner must call the Gemini command adapter
- if step model is `claude-opus`, the runner must call the Claude command adapter

## 3.3.2 Directory binding rule

Step execution must never assume one globally shared project path.

The runner must resolve the local path in this order:

1. load all directory bindings for the project
2. check each bound `local_path`
3. compatibility fallback only if a legacy global `project_path` still exists during migration
4. otherwise stop execution and fail the trigger with clear guidance

## 3.4. Output of step - Artifacts 
Reference SS-07-Workflow-Artifact.md

- Each step can optionally consume zero or more predefined input artifact definitions
- Each step can optionally produce zero or more predefined output artifact definitions
- Artifact definitions are reusable templates, similar to how workflow steps are reusable templates
- Runtime execution creates real artifact-run records that reference the artifact definition + workflow run + workflow step run
- Output artifacts can be used as input context for later steps
- If a step declares one or more required input artifact definitions and any expected local artifact file cannot be resolved, the step must fail
- We need to save artifact-run history and also allow backup/sync to online storage later

## 3.5. IMPORTANT - Supported Step for MVP

we plan to support these steps for MVP:
### 3.5.1 Business Idea Step

we need 1 skill support this step. that is 'Business Analyst Skill'

Expected artifact output is:
1 md file contain:
- Business Idea
- Summary

### 3.5.2 Feature Intake Step
**Note: Requires Jira MCP to read ticket information.**

We need 1 skill support this step. that is 'Feature Intake Skill'

Expected artifact output is:
1 md file contain:
- Feature
- Summary


### 3.5.3 Business Summary Step

We need 1 skill support this step. that is 'Business Summary Skill'

Expected artifact output is:
1 md file contain:
- Business Summary
- Summary
- Product Requirement Document (PRD)

### 3.5.4 Product Spec Step

We need 1 skill support this step. that is 'Product Spec Skill'

Expected artifact output is:
1 md file contain:
- Product Spec


### 3.5.5 Tech Spec Step

We need 1 skill support this step. that is 'Tech Spec Skill'

Expected artifact output is:
1 md file contain:
- Tech Spec


### 3.5.6 Make Plan Coding Step

We need 1 skill support this step. that is 'Make Plan Coding Skill'

Expected artifact output is:
1 md file contain:
- Coding plan


### 3.5.7 Create Architecture Step
We need 1 skill support this step. that is 'Create Architecture Skill'

Expected artifact output is:
1 md file contain:
- Architecture plan


### 3.5.8 TDD Step

We need 1 skill support this step. that is 'TDD Skill'
Based on Plan, Architecture, Tech Spec, Product Spec 
This step create unit test TDD with the signature only -> not implement body
Expected artifact output is:

1 md file contain:
- Unit test plan

IMPORTANT: **TDD plan must match to Product Spec and Business Requirement**
**TDD plan must cover all use-cases from business requirement**
**TDD plan must cover all edge-cases from business requirement**
**TDD plan must cover all error-cases from business requirement**
**Coding plan or Architect need to be changed if they do not match to TDD plan cause TDD is the guard for coding implementation**

### 3.5.9 Task Breakdown Step
Skill: Task Breakdown Skill
This step will breakdown the coding plan into smaller tasks for each developer
(based on Team skills and roles we added to PROJECTS)

Then create master schedule (human can assign to each developer)
Expected artifact output is:
1 md file contain:
- Task Breakdown
- Master Schedule

### 3.5.10 Code/Review Loop Step
Skill: Code/Review Loop Skill
This one will start new sub-agent for coding + unit test
Then if the coding result pass -> call review agent to review
Then loop until there are no error found from Review Agent

**When does coding agent stop and review agent start?**
- The program must complete build successful
- The program must pass all unit test successful
- The code coverage must be >= 80%

### 3.5.11 Release Readiness Step
Skill: Release Readiness Skill

### 3.5.12 Issue Analysis Step
**Note: Requires Firebase MCP to read crash logs and Jira MCP to read reported issues.**
Skill: Issue Analysis Skill
This step analyzes crash reports, logs, or user reported issues to find the root cause.
Expected artifact output is:
1 md file contain:
- Root Cause Analysis
- Proposed Fix Plan

### 3.5.13 Analytics Review Step
**Note: Requires Firebase MCP to read analytics data.**
Skill: Analytics Review Skill
This step gathers user usage data and analytics to provide insights on feature adoption and user behavior.
Expected artifact output is:
1 md file contain:
- Usage Analytics Report
- User Insights

### 3.5.14 Project Analysis Step
**Note: Requires Driver Google MCP to read project docs.**
Skill: Project Analysis Skill
This step provides an overview of the current project status, process efficiency, and overall progress.
Expected artifact output is:
1 md file contain:
- Project Process Overview
- Bottleneck Analysis

### 3.5.15 Telegram Notification Step
**Note: Requires Telegram MCP to send messages.**
Skill: Notification Skill
This step sends the final result, state, or summary of a workflow to a configured Telegram chat/group.
Expected artifact output is:
- Notification sent status

### 3.5.16 Code Traceability Step
**Note: Requires Jira MCP to trace tickets.**
Skill: Code Traceability Skill
This step traces a specific line of code or bug back to the original Jira ticket and PR to understand why the change was made.
Expected artifact output is:
1 md file contain:
- Ticket context and reason for change

### 3.5.17 Onboarding Walkthrough Step
**Note: Requires Driver Google MCP to read project docs.**
Skill: Onboarding Skill
This step provides a high-level summary of the product or walks through a specific coding module for a new team member.
Expected artifact output is:
none. This step is side-effect-only and does not need a persistent artifact definition.

## 3.6 Predefined Output Artifact Definitions for MVP

Every built-in step that produces a persistent file-backed output must seed at least one predefined output artifact definition.

Steps that only perform side effects may leave the output artifact empty.

| Step | Predefined output artifact definition | Default file |
|---|---|---|
| Business Idea Step | `business_idea_artifact` | `BusinessIdea.md` |
| Feature Intake Step | `feature_intake_artifact` | `FeatureIntake.md` |
| Business Summary Step | `business_summary_artifact` | `BusinessSummary.md` |
| Product Spec Step | `product_spec_artifact` | `ProductSpec.md` |
| Tech Spec Step | `tech_spec_artifact` | `TechSpec.md` |
| Make Plan Coding Step | `coding_plan_artifact` | `CodingPlan.md` |
| Create Architecture Step | `architecture_artifact` | `ArchitecturePlan.md` |
| TDD Step | `tdd_plan_artifact` | `TddPlan.md` |
| Task Breakdown Step | `task_breakdown_artifact` | `TaskBreakdown.md` |
| Code/Review Loop Step | `code_review_summary_artifact` | `CodeReviewSummary.md` |
| Release Readiness Step | `release_readiness_artifact` | `ReleaseReadiness.md` |
| Issue Analysis Step | `root_cause_analysis_artifact` | `RootCauseAnalysis.md` |
| Analytics Review Step | `usage_analytics_artifact` | `UsageAnalytics.md` |
| Project Analysis Step | `project_analysis_artifact` | `ProjectAnalysis.md` |
| Telegram Notification Step | none | none |
| Code Traceability Step | `code_traceability_artifact` | `CodeTraceability.md` |
| Onboarding Walkthrough Step | none | none |

The product model must support multiple input and output artifact definitions per step now.

For built-in MVP seeds, most steps only need one primary predefined output artifact definition at first, but that is a seed choice, not a platform limitation.

If a step later needs additional persistent outputs, the platform should allow attaching additional predefined artifact definitions without redefining the step model.

## 3.7 Step Runtime Contract

When a workflow step starts, the Golang runner must read the step definition and resolve:

1. required MCPs
2. skills
3. subagent
4. input artifacts
5. output artifacts
6. selected model

Then the runner assembles the prompt with:

- team role
- project path
- selected model
- begin prompt or current user input
- input artifact paths
- skills
- subagent if defined
- output artifact path

If a required MCP is missing on the project, the runner must stop the step and show the user which MCP needs to be enabled.


# 4. Workflow usage and configuration in project
- Allow a project to browse and pick global workflows.
- Allow a project to create and save private workflows that belong only to that project.
- Allow a project to start a workflow run from either a global workflow or a private workflow.
- Allow a project to start a single step without creating a full workflow definition first.
- Allow user to enable/disable approval gate per step.
- Current YOLO rule: do not toggle YOLO per workflow run. Workflow-definition runs read live YOLO from `workflows.yolo_mode`; direct single-step runs read live YOLO from `step_definitions.yolo_mode`. A workflow with exactly one step is still a workflow-definition run.
- Allow user to enable/disable step.
- Allow user to re-order steps.
- Allow user to add/remove steps.
- Allow user to select a step model from the supported model dropdown list.

## 4.1 Workflow execution states
- Workflow run status: `PENDING`, `RUNNING`, `DONE`, `FAILED`, `CANCELED`.
- Workflow step status: `PENDING`, `RUNNING`, `WAITING_USER_APPROVAL`, `DONE`, `FAILED`, `SKIPPED`.
- If a step is disabled for a run, it becomes `SKIPPED`.
- If approval gate is enabled and the current live YOLO SSOT is OFF, the step waits at `WAITING_USER_APPROVAL`.
- If user rejects an artifact, the same step retries with the rejection note as context.

# 5. Built-in flows for MVP by Persona Use Cases

Based on the supported steps, we provide 10 built-in workflows across 4 personas to resolve specific use cases for different roles:

## 5.1 As a Developer
Focused on execution and implementation.

### 5.1.1 Bug Fix Flow
**Use Case:** Fix an existing bug or issue.
**Workflow:**
-> Code Traceability Step (Trace code to Jira ticket)
-> Issue Analysis Step (Find root cause)
-> Tech Spec Step (If architectural changes needed)
-> Make Plan Coding Step (Plan the fix)
-> Code/Review Loop Step (Implement and test)
-> Release Readiness Step (Verify fix is ready to release)

### 5.1.2 Pre-defined Feature Flow
**Use Case:** Develop a new feature where business logic or tech design is already defined.
**Workflow:**
-> Tech Spec Step (Refine tech details if needed)
-> Make Plan Coding Step (Create implementation steps)
-> Create Architecture Step (Design components)
-> TDD Step (Write test signatures)
-> Code/Review Loop Step (Implement and review)
-> Release Readiness Step
-> Telegram Notification Step (Notify completion)

### 5.1.3 Bug Traceability Flow
**Use Case:** Trace a line of code or bug back to its origin ticket to understand the "why".
**Workflow:**
-> Code Traceability Step (Trace code to Jira ticket)

### 5.1.4 Onboarding Flow
**Use Case:** A new member wants to understand the product summary or a specific coding part.
**Workflow:**
-> Onboarding Walkthrough Step (Generate walkthrough summary)

## 5.2 As a SOLO Developer
Focused on end-to-end delivery from idea to production.

### 5.2.1 Full End-to-End Flow
**Use Case:** Take a raw business idea all the way to a complete feature.
**Workflow:**
-> Business Idea Step
-> Feature Intake Step
-> Business Summary Step
-> Product Spec Step
-> Tech Spec Step
-> Make Plan Coding Step
-> Create Architecture Step
-> TDD Step
-> Code/Review Loop Step
-> Release Readiness Step
-> Telegram Notification Step

### 5.2.2 Fast-Track Business Flow
**Use Case:** Input is already structured as business requirements; continue directly to planning and implementation.
**Workflow:**
-> Product Spec Step
-> Tech Spec Step
-> Make Plan Coding Step
-> Create Architecture Step
-> TDD Step
-> Code/Review Loop Step
-> Release Readiness Step

## 5.3 As a Leader
Focused on team management, delegation, and high-level problem solving.

### 5.3.1 Task Breakdown & Delegation Flow
**Use Case:** Break down a feature into smaller tasks and assign to the team with a master schedule.
**Workflow:**
-> Tech Spec Step
-> Make Plan Coding Step
-> Task Breakdown Step (Produces Task Breakdown & Master Schedule)

### 5.3.2 Root Cause Analysis Flow
**Use Case:** Analyze a critical issue or crash.
**Workflow:**
-> Code Traceability Step (Trace code to Jira ticket)
-> Issue Analysis Step (Find root cause and propose fix)
-> Task Breakdown Step (Assign the fix to a developer)
-> Telegram Notification Step (Alert leader)

### 5.3.3 Analytics & Usage Flow
**Use Case:** Understand user usage and feature adoption.
**Workflow:**
-> Analytics Review Step (Generate insights based on usage data)

## 5.4 As a PM/Owner
Focused on product direction, process tracking, and business outcomes.

### 5.4.1 Product Process & Analysis Flow
**Use Case:** Track the process from idea to spec and analyze project health.
**Workflow:**
-> Business Idea Step
-> Feature Intake Step
-> Business Summary Step
-> Product Spec Step
-> Project Analysis Step (Analyze current progress and bottlenecks)
-> Analytics Review Step (Review user impact)
