# 1. Project

A project is a collection of related workflows.

Project is created when user click new project in the UI.
Project will be stored in the database.

# 2. Project vs Workflow
See workflow in [SS-04-Workflow.md](./SS-04-Workflow.md)

Basically i can have many projects.

Each project i can have many workflows.

Example:

Go to project Backend ABC - create a workflow to implement API authentication
Go to project Mobile App XYZ - create a workflow to implement payment gateway integration

**1 Project can has many workflows**

# 3. Project Structure

# 3.1 Mandatory fields

- Name: User input text
- Directory Path: User input text Or open system file dialog to choose directory path

This path will be used by Golang cobra runner tool: CD to this path -> call LLM to execute workflow prompt.

Example:
```bash
cd /Users/tiendat/Desktop/flowpilot/backend-abc
golang-cobra-runner /path/to/prompt
```

# 3.2 Optional fields: MCP context
See [SS-02-Project-Context.md](./SS-02-Project-Context.md) for more information.

You dont need to include this one when create new project. We will add it later.

But later if you want to use any flow that contain 1 step need MCP context, you need add it to the project.

For example: you go to project mobile A, start a new flow: Fix bug jira ticket 123

That flow contain 1 step need MCP context: Read jira ticket 123. You need configure JIta MCP context for project mobile A. If not, the flow will fail at that step.

You can configure many MCP context for 1 project.

# 3.3 Optional fields: Team
See [SS-03-Project-Team.md](./SS-03-Project-Team.md) for more information.

if we assign 1 team to 1 project then we can use for the flow with break task and assign task to team member.