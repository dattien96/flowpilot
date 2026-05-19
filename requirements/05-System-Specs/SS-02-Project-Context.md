# 1. What is context?

Purpose:
- collect and normalize task-specific context
- make external project systems available to workflow runs

## Why do we need context?

Many workflow steps need project data that does not live inside FlowPilot itself.

Examples:
- Jira ticket description, comments, status, assignee
- Figma design spec or component details
- Google Drive files and folders
- Firebase project data
- Telegram notifications or messages

If a workflow requires MCP context and that context is not really connected, the workflow must fail early with a clear reason.

---

# 2. What can be a context?

For MVP now, we support 2 types of context:

## 2.1 Text-based context

Put directly in the flow definition or project/workflow inputs.

This type is used to store text-based information.
Users can:
- type text
- upload files
- paste links

## 2.2 MCP context

Belongs to the parent project.

See [SS-01-Project.md](./SS-01-Project.md) for the project relationship.

MCP must be configured once per project and shared across flows in that project.

---

# 3. Types of MCP for MVP

We want to support these MCP context types for MVP:
- Jira: read ticket description, comments, status, assignee, and related metadata
- Figma: read design spec, file structure, components, and comments
- Google Drive: read or write files in a selected folder or shared drive
- Firebase: read or write project data, crash info, analytics, and environment details
- Telegram: send notifications and optionally receive workflow-related messages

---

# 4. Business expectation for MCP connection

This is the most important expectation for MVP:

**Adding an MCP in the UI must create a real usable connection, not just save a row in Supabase.**

When the user clicks `Add MCP`:
1. user selects provider type and enters provider-specific config such as `folderId`
2. FlowPilot saves the project-scoped MCP record
3. FlowPilot asks the local Go-Runner to connect that MCP
4. the Go-Runner opens the user's local browser to the real provider auth page when auth is required
5. user logs in and approves access
6. the Go-Runner receives the callback, token, or equivalent auth proof locally
7. the Go-Runner verifies real access to the configured resource
8. FlowPilot marks the MCP as `connected` only after verification succeeds

If verification fails, the MCP must stay non-connected and surface the failure reason.

---

# 5. Google Drive example

For Google Drive, entering only `folderId` is not enough by itself.

Expected real flow:
1. user enters `folderId`
2. system saves the project MCP entry
3. Go-Runner opens Google OAuth in the user's local browser
4. user logs into Google and approves Drive access
5. Go-Runner receives the auth callback locally
6. Go-Runner stores the credential securely on the local machine
7. Go-Runner calls Google Drive API to verify it can access the configured folder
8. MCP status becomes `connected`

Later, when a workflow or project action asks to import files from Google Drive, the system uses that real connection.

---

# 6. Security expectation

Provider credentials must not be treated as plain business config.

Rules:
- stable project config may be stored in FlowPilot DB, for example `folderId`, `workspaceUrl`, `board`, `projectId`
- provider auth tokens or equivalent secrets must be handled by the Go-Runner
- auth credentials should be stored securely on the local machine or in a dedicated secure secret store
- the project MCP record should expose status and metadata, not raw provider tokens

---

# 7. Runtime expectation

After an MCP is connected, users must get real value from it.

Examples:
- import Jira tickets into business logic context
- import Google Drive docs into project context
- read Figma design data during workflow execution
- send Telegram notifications from workflow steps

The system must not pretend an MCP is available if the runner cannot actually use it.
