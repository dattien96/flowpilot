# 1. Project

A project is a collection of related workflows.
A project is also linked to one or more real local workspaces.

Project is created when user click new project in the UI.
Project will be stored in the database.

# 2. Project vs Workflow
See workflow in [SS-04-Workflow.md](./SS-04-Workflow.md)

Basically i can have many projects.

Each project can use many workflows.

Workflow definitions can be:
- Global workflows: reusable templates visible to many projects
- Private workflows: custom workflows owned by one specific project

Workflow run is different from workflow definition:
- Workflow = saved definition of ordered steps
- Workflow run = 1 actual execution of that workflow on a project

Example:

Go to project Backend ABC - pick a global workflow to implement API authentication
Go to project Mobile App XYZ - create a private workflow for payment gateway integration

**1 Project can use many workflow definitions and can start many workflow runs**

# 3. Project Structure

# 3.1 Mandatory fields

- Name: User input text
- Repository URL: user input text if available
- Local Directory Path: user input text or open system file dialog to choose directory path

The project create flow still requires at least one local directory path.

But this path must not be treated as one globally shared `project_path` for all devices.

Instead, the system model is:

- one shared `project`
- many local workspace paths
- bindings stored by `project_id + local_path`

Recommended shared/local separation:

- `projects`
  - shared metadata only
  - name
  - description
  - repository_url
- `project_workspace_bindings`
  - project_id
  - local_path
  - optional label

The selected local path at create time becomes the first directory binding for the project.

This resolved local path is used by the Golang Cobra runner as the execution working directory.

This path will be used by Golang cobra runner tool: CD to this path -> call LLM to execute workflow prompt.

Example:
```bash
cd /Users/tiendat/Desktop/flowpilot/backend-abc
golang-cobra-runner /path/to/prompt
```

Every workflow run must execute against one valid bound local path.
If no valid bound path can be opened, workflow execution must not start.

## 3.1.1 Binding resolution

When workflow execution is triggered:

1. load all bindings for the project
2. Golang runner checks each path
3. if one path is valid and accessible, run there
4. if no path is valid, fail the trigger and notify user the root cause

The user must not be forced to create a duplicate project just because the path is different on another machine.

# 3.2 Project Detail Screen as Workflow Entry

The project detail screen is the main execution entry point for a project.

It must provide one trigger UI that supports 3 modes:

1. Start from workflow definition
2. Start from single step
3. Create new workflow

Before starting, user must be able to enter one begin prompt.

Example:

```text
I want to implement the login feature
```

This begin prompt is passed to:

- the first step of the selected workflow definition
- or the selected single step

The project detail screen is not a chat screen.
It is a workflow launchpad.

The project detail screen must also include one tab:

- `Directory Binding`

That tab must:

- show all current bindings for the project
- allow add binding
- allow edit binding
- allow delete binding

# 3.3 Optional fields: MCP context
See [SS-02-Project-Context.md](./SS-02-Project-Context.md) for more information.

You dont need to include this one when create new project. We will add it later.

But later if you want to use any flow that contain 1 step need MCP context, you need add it to the project.

For example: you go to project mobile A, start a new flow: Fix bug jira ticket 123

That flow contain 1 step need MCP context: Read jira ticket 123. You need configure JIta MCP context for project mobile A. If not, the flow will fail at that step.

You can configure many MCP context for 1 project.

# 3.4 Optional fields: Team
See [SS-03-Project-Team.md](./SS-03-Project-Team.md) for more information.

if we assign 1 team to 1 project then we can use for the flow with break task and assign task to team member.
