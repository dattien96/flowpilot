# CP-27: FlowPilot Self-Hosted MCP Server For Google Drive Write Mode

## Metadata

- Document ID: `CP-27`
- Title: `FlowPilot Self-Hosted MCP Server For Google Drive Write Mode`
- Phase: `coding_plan`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-09`
- Last Updated: `2026-06-09`
- Parent Documents: [CP-05-03: Google Drive MCP Current Implementation Notes](../priority/CP-05-03-Driver-Mcp.md), [SD-11: MCP Connection Flows](../../06-System-Tech-Design/SD-11-MCP-Connection-Flows.md), [SS-02: Project Context](../../05-System-Specs/SS-02-Project-Context.md), [SS-04: Workflow](../../05-System-Specs/SS-04-Workflow.md)
- Child Documents: `TBD`
- Related Documents: [Task-025: Drive MCP Auth Flow](../../08-Task/Task-025-Drive-MCP-Auth-Flow.md), [CP-27: Google Cloud Setting](../done/CP-27-Google-Cloud-Setting-Manually.md), [CP-28: Google Cloud Config With UI Auto](../done/CP-28-Google-Cloud-Config-With-Ui-Auto.md)
- Replaces: `None`
- Tags: `mcp, google-drive, workflow-runtime, write-approval, provider-config`

## AI Quick View

### Summary

- CP-05-03 now supports Phase A and Phase B read-only Google Drive MCP behavior only.
- CP-27 owns the future design for FlowPilot's own MCP server so write-mode steps can be observed and approved safely.
- The key architecture change is moving provider config from direct `AI provider -> raw Google Drive MCP` to `AI provider -> FlowPilot MCP server -> Google Drive`.
- Default step mode remains `read_only`; `read_write` is an explicit workflow-step configuration.
- True per-request approval is only possible when the write tool call crosses a FlowPilot-controlled MCP server boundary.

### Current Ask

- Explain how a self-hosted MCP server works in terms a non-MCP expert can understand.
- Define the future implementation plan in enough detail that another AI agent can implement it with minimal ambiguity.

### Key Decisions

- `P-1` Build a FlowPilot-owned MCP server/proxy instead of trying to inspect provider stdout for tool calls.
- `P-2` Use `stdio` transport first because provider clients already support local command-based MCP servers.
- `P-3` Configure AI providers to connect to FlowPilot's MCP server when write-mode observability is required.
- `P-4` Keep `read_only` as the default for all workflow steps.
- `P-5` Require explicit user approval before executing write tools.
- `P-6` Keep destructive and permission-changing Google Drive tools disabled until a separate policy is approved.

### Constraints

- FlowPilot currently sends prompts to provider CLIs; provider CLIs own MCP client behavior.
- FlowPilot cannot reliably see exact provider-side Google Drive write tool calls when providers connect directly to the raw Google Drive MCP package.
- Supabase must not store Google OAuth tokens, Desktop OAuth JSON contents, or MCP token contents.
- The first implementation should stay local-runner owned and machine-local.
- Provider account-home isolation must continue to work.

### Open Questions

- Should the FlowPilot MCP server call Google APIs directly, or proxy to the existing `@piotr-agier/google-drive-mcp` package?
- Should first write-mode support be limited to `createGoogleDoc` only?
- Should approved write requests expire after one tool call, one step, or one run?
- Should rejected write requests fail the step immediately or return a tool error that lets the model produce a non-write final answer?

### Source Refs

- CP-05-03 Section 11.15 Phase C handoff
- CP-05-03 Phase A and Phase B read-only provider-MCP behavior
- SD-11 MCP connection flows
- SS-02 MCP context
- SS-04 workflow step MCP requirement model

## 1. Goal

Implement a future FlowPilot-owned MCP server for Google Drive so workflow steps can safely support:

- default `read_only` MCP access
- explicit `read_write` MCP access
- exact write tool call capture
- user approval before write execution
- write audit metadata after execution

This plan is intentionally split into two parts:

- Part 1 explains the design and terminology without code.
- Part 2 gives an implementation plan detailed enough for a future AI agent to execute.

## 2. Input Documents

- [CP-05-03: Google Drive MCP Current Implementation Notes](../priority/CP-05-03-Driver-Mcp.md)
- [SD-11: MCP Connection Flows](../../06-System-Tech-Design/SD-11-MCP-Connection-Flows.md)
- [SS-02: Project Context](../../05-System-Specs/SS-02-Project-Context.md)
- [SS-04: Workflow](../../05-System-Specs/SS-04-Workflow.md)
- [Task-025: Drive MCP Auth Flow](../../08-Task/Task-025-Drive-MCP-Auth-Flow.md)

## 3. Implementation Strategy

### Part 1: Design Explanation Without Code

#### 3.1 Current FlowPilot MCP Architecture

Today FlowPilot does not call Google Drive MCP tools directly during normal workflow execution.

Current runtime shape:

```text
FlowPilot
-> builds the workflow prompt
-> injects "this step requires google_drive" instructions
-> launches Codex, Claude, or Gemini
-> provider client reads its own MCP config
-> provider client connects directly to Google Drive MCP
-> provider client calls Drive tools
-> provider client returns final output to FlowPilot
```

This works for read-only mode because FlowPilot only needs to prove:

- Google Drive MCP auth exists
- provider config exists
- provider can use read tools
- prompt tells the provider to use the right MCP server

This does not work for exact write approval because the exact write request happens inside the provider client after FlowPilot has already launched the provider.

#### 3.2 Terms

`MCP` means Model Context Protocol.

It is a protocol that lets an AI client use external tools and data sources. A provider such as Codex, Claude, or Gemini can connect to an MCP server and ask it to run a tool.

`AI provider` means the local AI client process FlowPilot launches. Examples:

- Codex CLI
- Claude Code CLI
- Gemini CLI

`MCP client` means the part that connects to an MCP server and asks for tools. In the current architecture, the AI provider is the MCP client.

`MCP server` means a process that exposes tools. Example: a Google Drive MCP server exposes tools such as search, read document, create document, and update document.

`Tool` means one callable operation exposed by an MCP server. A tool has:

- a name, such as `search` or `createGoogleDoc`
- an input schema, such as "query is required"
- a handler that executes the work
- a result, such as matching files or a created document ID

`Read tool` means a tool that only reads or diagnoses state. Examples:

- search Drive files
- list a folder
- read a Google Doc
- check auth status

`Write tool` means a tool that creates, updates, deletes, moves, shares, or uploads something. Examples:

- create Google Doc
- update Google Doc
- create folder
- delete item
- add permission

`Transport` means how the AI provider talks to the MCP server.

`stdio` means standard input and standard output. The provider starts the MCP server as a local child process and sends JSON messages through the process input/output streams. No HTTP port is required.

Example idea:

```text
Codex starts command:
  flowpilot-google-drive-mcp --workspace C:/working/flowpilot

Codex writes MCP requests to the process stdin.
FlowPilot MCP server writes MCP responses to stdout.
```

`stdio` is best for the first implementation because:

- provider MCP configs already support command-based local MCP servers
- no port management is needed
- no extra local HTTP security model is needed
- process lifetime is naturally tied to the provider session

`HTTP transport` means the MCP server listens on a network port and providers call it over HTTP. This can be useful later, but it adds port, auth, and lifecycle complexity.

#### 3.3 Three-Layer Interaction Model

The future write-mode architecture has three important product layers:

```text
Layer 1: User
Layer 2: AI Provider
Layer 3: FlowPilot MCP Server
```

External Google Drive is a dependency behind the FlowPilot MCP server, not a user-facing layer.

High-level interaction diagram:

```text
+------------------------------------------------------------------+
| 1. User                                                          |
|                                                                  |
| - configures workflow step                                       |
| - chooses read_only or read_write                                |
| - reviews pending write requests                                 |
| - approves or rejects exact write operations                     |
+------------------------------------------------------------------+
                              |
                              | starts workflow / approves writes
                              v
+------------------------------------------------------------------+
| 2. FlowPilot workflow runtime and UI                             |
|                                                                  |
| - stores step mcpAccessMode                                      |
| - launches provider with MCP config                              |
| - stores pending approval records                                |
| - pauses/resumes/fails workflow steps                            |
+------------------------------------------------------------------+
                              |
                              | prompt + provider config
                              v
+------------------------------------------------------------------+
| 3. AI Provider                                                   |
|                                                                  |
| - Codex, Claude, or Gemini                                       |
| - acts as MCP client                                             |
| - decides which MCP tool to call                                 |
| - calls FlowPilot MCP server over stdio                          |
+------------------------------------------------------------------+
                              |
                              | MCP tool call: toolName + arguments
                              v
+------------------------------------------------------------------+
| 4. FlowPilot MCP Server                                          |
|                                                                  |
| - exposes read and allowed write tools                           |
| - receives every provider tool call                              |
| - executes read tools immediately                                |
| - creates pending approval for write tools                       |
| - executes approved write tools                                  |
| - records audit metadata                                         |
+------------------------------------------------------------------+
                              |
                              | Google API call or proxy call
                              v
+------------------------------------------------------------------+
| 5. Google Drive / Google Docs API                                |
|                                                                  |
| - stores files and docs                                          |
| - returns file IDs, URLs, and content                            |
+------------------------------------------------------------------+
```

For the user's requested three-layer explanation, collapse the runtime/UI and external Google API detail like this:

```text
User
-> configures and approves

AI Provider
-> thinks and calls MCP tools

Our MCP
-> receives tool calls, enforces read/write policy, talks to Google Drive
```

#### 3.4 Why "Our MCP" Solves Per-Request Write Approval

If the provider talks directly to raw Google Drive MCP:

```text
AI Provider -> raw Google Drive MCP -> Google Drive
```

FlowPilot only sees:

- the original prompt
- stdout/stderr chunks
- final provider output

FlowPilot does not reliably see:

- exact write tool name
- exact write arguments
- exact target document/folder
- whether the provider retried with different arguments
- whether a write happened before the final text

If the provider talks to FlowPilot MCP:

```text
AI Provider -> FlowPilot MCP -> Google Drive
```

FlowPilot sees the actual tool call before execution:

```text
toolName = createGoogleDoc
arguments = {
  title: "Sprint Plan",
  content: "...",
  parentFolderId: "folder-123"
}
workflowRunId = run-123
workflowStepRunId = step-456
```

That is the correct place to enforce approval.

#### 3.5 Read-Only Flow

Read-only mode is the default.

Interaction:

```text
User starts workflow
-> FlowPilot launches provider
-> provider connects to FlowPilot MCP
-> provider calls search/read/list tools
-> FlowPilot MCP executes immediately
-> provider uses returned Drive context
-> provider returns final answer
```

No approval is needed for read tools.

#### 3.6 Write-Mode Flow

Write mode is explicit per workflow step.

Interaction:

```text
User starts workflow
-> step has mcpAccessMode = read_write
-> FlowPilot launches provider with write-capable FlowPilot MCP config
-> provider calls createGoogleDoc
-> FlowPilot MCP receives the write call
-> FlowPilot MCP creates pending approval record
-> workflow step moves to waiting_approval
-> UI shows exact tool, target, and arguments summary
-> user approves
-> FlowPilot MCP executes the stored write call
-> Google Drive returns created document ID and URL
-> FlowPilot MCP returns tool result to provider
-> provider returns final answer
-> FlowPilot records write audit metadata
```

If user rejects:

```text
User rejects
-> FlowPilot MCP returns write rejected error
-> workflow step fails or continues without write based on policy
```

#### 3.7 What "Step Write Mode" Means

Step config should use:

```text
mcpAccessMode = read_only | read_write
```

Default:

```text
mcpAccessMode = read_only
```

Meaning:

- `read_only`: expose read tools only; write tools are unavailable.
- `read_write`: expose read tools and selected write tools; write execution still requires approval.

Important rule:

- `read_write` does not mean "the AI can write freely".
- `read_write` means "the AI may request write tools, and FlowPilot will approve or reject each write request".

#### 3.8 Recommended First Write Tools

Start with non-destructive create/update tools only:

- `createGoogleDoc`
- `updateGoogleDoc`
- `createGoogleSheet`
- `updateGoogleSheet`

Do not include these in the first release:

- `deleteItem`
- `addPermission`
- `updatePermission`
- `removePermission`
- `shareFile`
- public sharing
- calendar write tools
- move/rename unless the target policy is very clear

## 4. Work Breakdown

### Part 2: Detailed Code Implementation Plan

#### P-1: Add Step MCP Access Mode

Goal:

- Store whether a workflow step is read-only or read-write for MCP access.

Data model:

- Add `mcp_access_mode` to workflow step storage.
- Valid values:
  - `read_only`
  - `read_write`
- Default:
  - `read_only`

Apply to both template and runtime copies if the project stores workflow definitions and workflow run steps separately.

Suggested TypeScript domain shape:

```ts
export type McpAccessMode = "read_only" | "read_write";

export interface WorkflowStep {
  requiredMcps: string[];
  mcpAccessMode: McpAccessMode;
}
```

Validation rules:

- If `requiredMcps` does not include `google_drive`, ignore or force `mcpAccessMode = read_only`.
- If `mcpAccessMode` is missing, treat it as `read_only`.
- If a step is `read_write`, it must also include `requiredMcps: ["google_drive"]`.

Admin UI:

- In workflow builder step settings, add a two-option control:
  - `Read only`
  - `Read + write`
- Default selected value is `Read only`.
- Only show this control when the step requires an MCP that supports write mode.
- If user enables `Read + write`, show a warning summary:
  - write tools can be requested by the AI
  - every write request still requires approval
  - destructive tools are not included in first release

Files likely touched:

- `apps/admin-web/src/domain/model/entity/workflow.ts` or nearest workflow model file
- `apps/admin-web/src/data/repository/supabase/workflow-engine-mappers.ts`
- `apps/admin-web/src/data/repository/supabase/supabase-workflow-engine-gateway.ts`
- workflow builder route/component files
- Supabase migration file for `workflow_steps.mcp_access_mode`
- any runtime step-plan mapper in `apps/admin-web/src/features/workflow-engine/workflow-start-runtime.ts`

Tests:

- Missing `mcp_access_mode` maps to `read_only`.
- Workflow builder saves `read_write`.
- Runtime step plan receives `read_write`.
- Invalid values are rejected or normalized.

#### P-2: Add FlowPilot MCP Server Binary Or Subcommand

Goal:

- Provide a local command that AI providers can launch as an MCP server.

Recommended location:

- Add a Go package under the local runner app.
- Preferred command name:
  - `flowpilot-google-drive-mcp`
- Acceptable alternative:
  - local runner subcommand `local-runner mcp google-drive`

The provider config should eventually point to a command like:

```text
flowpilot-google-drive-mcp --workspace <workspacePath> --account-home <accountHomePath>
```

or:

```text
flowpilot-local-runner mcp google-drive --workspace <workspacePath> --account-home <accountHomePath>
```

Arguments:

- `--workspace`: absolute FlowPilot workspace path
- `--account-home`: provider account home path
- `--workflow-run-id`: optional, used when launching run-scoped sessions
- `--workflow-step-run-id`: optional, used when launching run-scoped sessions
- `--access-mode`: `read_only` or `read_write`
- `--approval-api-url`: optional local runner URL if the MCP server calls the runner API to create approval records

Environment:

- `FLOWPILOT_WORKSPACE`
- `FLOWPILOT_ACCOUNT_HOME`
- `FLOWPILOT_WORKFLOW_RUN_ID`
- `FLOWPILOT_WORKFLOW_STEP_RUN_ID`
- `GOOGLE_DRIVE_OAUTH_CREDENTIALS`
- `GOOGLE_DRIVE_MCP_TOKEN_PATH`

Implementation rule:

- The MCP server must not print non-MCP logs to stdout.
- Logs must go to stderr or a FlowPilot log file.
- stdout is reserved for MCP protocol messages.

Tests:

- Server starts with valid args.
- Server fails fast with missing workspace.
- Server does not write logs to stdout.
- Server exposes expected tool list in `read_only`.
- Server exposes expected tool list in `read_write`.

#### P-3: Implement MCP Tool Registry

Goal:

- Define exactly which tools FlowPilot MCP exposes.

Read-only tools:

```text
authGetStatus
authListScopes
authTestFileAccess
search
listFolder
listSharedDrives
readGoogleDoc
getGoogleDocContent
```

First write tools:

```text
createGoogleDoc
updateGoogleDoc
createGoogleSheet
updateGoogleSheet
```

Tool metadata requirements:

- name
- description
- input schema
- access type:
  - `read`
  - `write`
- destructive flag:
  - `false` for first release tools
- approval requirement:
  - `none` for read tools
  - `required` for write tools

Suggested Go shape:

```go
type ToolAccess string

const (
	ToolAccessRead  ToolAccess = "read"
	ToolAccessWrite ToolAccess = "write"
)

type FlowPilotMcpTool struct {
	Name             string
	Description      string
	Access           ToolAccess
	Destructive      bool
	RequiresApproval bool
}
```

Rules:

- In `read_only`, register only read tools.
- In `read_write`, register read tools and allowed non-destructive write tools.
- Do not register destructive tools in first release.

Tests:

- `read_only` mode never lists write tools.
- `read_write` mode lists first-release write tools.
- Destructive tools are absent even in `read_write`.

#### P-4: Implement Google Drive Execution Layer

Goal:

- Make the FlowPilot MCP server actually read and write Google Drive.

Two possible approaches:

Approach A: Call Google APIs directly.

- Use Google Drive, Docs, Sheets, and Slides APIs from Go.
- Pros:
  - FlowPilot owns behavior completely.
  - Easier to audit and test exact requests.
  - No second MCP process behind the proxy.
- Cons:
  - More Google API implementation work.
  - Must implement document conversion/update behavior.

Approach B: Proxy to existing `@piotr-agier/google-drive-mcp`.

- FlowPilot MCP server receives provider tool calls.
- It starts or connects to the existing Google Drive MCP package.
- It forwards allowed calls after policy checks.
- Pros:
  - Less Google API implementation at first.
  - Reuses existing package behavior.
- Cons:
  - More protocol plumbing.
  - Need robust child-process management.
  - Need to map tool names and errors.

Recommended first implementation:

- Use Approach A for a very small write set:
  - `authGetStatus`
  - `search`
  - `readGoogleDoc`
  - `createGoogleDoc`
- Expand after the policy and approval flow works.

Credential rules:

- Read credential path from existing FlowPilot Google Drive config.
- Read token path from existing FlowPilot Google Drive config.
- Never return token contents in tool results.
- Never log token contents.
- Reuse CP-05-03 auth readiness checks before exposing tools.

Tests:

- Missing credential path fails startup or tool call clearly.
- Missing token path fails with `MCP_AUTH_REQUIRED`.
- Search returns file name and ID.
- Read doc returns file name, ID, and content.
- Create doc returns file name, ID, and URL.

#### P-5: Add Approval Record Model

Goal:

- Store pending write requests created by the FlowPilot MCP server.

Suggested table:

```text
mcp_write_approvals
```

Columns:

```text
id uuid primary key
project_id uuid nullable
workflow_run_id uuid not null
workflow_run_step_id uuid not null
mcp_server text not null
tool_name text not null
tool_arguments_json jsonb not null
target_summary text nullable
target_resource_id text nullable
target_resource_type text nullable
requested_by_provider text nullable
requested_at timestamptz not null
status text not null
decision_comment text nullable
decided_by uuid nullable
decided_at timestamptz nullable
expires_at timestamptz not null
result_json jsonb nullable
error_message text nullable
```

Status values:

- `pending`
- `approved`
- `rejected`
- `expired`
- `executed`
- `failed`

Unique/safety behavior:

- One approval record represents one exact tool call.
- If the provider retries with different arguments, create a new approval record.
- Do not reuse approval across different step runs.
- Expire pending approvals after a short TTL, for example 10 minutes.

Tests:

- Creating a write request inserts `pending`.
- Approval changes status to `approved`.
- Rejection changes status to `rejected`.
- Expired requests cannot execute.
- Approval for one step cannot execute a write from another step.

#### P-6: Implement Write Tool Handling

Goal:

- Capture exact write tool calls and pause before execution.

Handler flow for every tool:

```text
Receive tool call
-> identify tool metadata
-> if read tool:
     execute immediately
-> if write tool and access mode is not read_write:
     return error DRIVE_WRITE_NOT_ALLOWED
-> if write tool and no matching approved approval:
     create pending approval record
     signal workflow waiting_approval
     return error MCP_WRITE_APPROVAL_REQUIRED with approval id
-> if write tool and matching approval exists:
     execute write
     save result_json
     mark approval executed
     return tool result
```

Important behavior:

- The write must not execute before approval.
- Approval must match:
  - workflow run ID
  - workflow step run ID
  - tool name
  - exact canonicalized arguments
- Canonicalize JSON arguments before matching to avoid whitespace/order differences.

Tool error text:

```text
MCP_WRITE_APPROVAL_REQUIRED: FlowPilot created approval request <approvalId>. Wait for user approval before retrying this exact write.
```

Runtime issue:

- Some providers may stop after receiving the approval-required error.
- First implementation may require the workflow step to pause and then re-run the provider step after approval.
- A later implementation can support a true wait/resume if the provider transport supports long-running blocked tool calls.

Recommended MVP behavior:

- On first write request, create approval and fail/pause the current provider call.
- FlowPilot marks step `waiting_approval`.
- After approval, FlowPilot reruns the step with a prompt note:
  - "The previous write request approval ID X is approved. Retry the same write operation."
- FlowPilot MCP server recognizes the approved exact call and executes it.

Tests:

- Read tool executes without approval.
- Write tool in `read_only` returns `DRIVE_WRITE_NOT_ALLOWED`.
- Write tool in `read_write` creates `pending` and does not execute.
- Approved exact write executes.
- Approved different write does not execute.
- Rejected write returns rejected error.

#### P-7: Add Workflow Runtime State Handling

Goal:

- Pause workflow runs when MCP write approval is required.

Runtime state additions:

- Add or reuse step status:
  - `waiting_approval`
- Add metadata on workflow run step:
  - `pendingMcpWriteApprovalId`
  - `mcpWriteApprovalStatus`

Flow:

```text
Provider output or MCP server signal says MCP_WRITE_APPROVAL_REQUIRED
-> runtime extracts approval id
-> workflow_run_steps.status = waiting_approval
-> workflow run remains paused
-> UI shows approval request
-> user approves or rejects
-> approved: runtime can replay/resume step
-> rejected: runtime marks step failed or changes_requested
```

How runtime receives approval requirement:

- Option 1: parse provider output for `MCP_WRITE_APPROVAL_REQUIRED`.
- Option 2: MCP server calls local runner API directly when creating approval.
- Option 3: both.

Recommended:

- Use option 2 as source of truth.
- Keep option 1 as a fallback for provider-visible errors.

Tests:

- Approval-required signal moves step to `waiting_approval`.
- Approving a request makes the step eligible to resume.
- Rejecting a request fails the step or marks it changes requested.
- Running unrelated steps is not blocked unless workflow order requires it.

#### P-8: Add Runner APIs

Goal:

- Let the MCP server and admin UI create, read, and decide approval requests.

Runner/local API endpoints:

```text
POST /mcp/write-approvals
GET /mcp/write-approvals?workflowRunId=<id>
GET /mcp/write-approvals/{approvalId}
POST /mcp/write-approvals/{approvalId}/decision
POST /mcp/write-approvals/{approvalId}/execute-result
```

Create request:

```json
{
  "projectId": "project-id",
  "workflowRunId": "run-id",
  "workflowRunStepId": "step-id",
  "mcpServer": "flowpilot-google-drive",
  "toolName": "createGoogleDoc",
  "toolArgumentsJson": {
    "title": "Sprint Plan",
    "content": "..."
  },
  "targetSummary": "Create Google Doc named Sprint Plan",
  "requestedByProvider": "codex"
}
```

Decision request:

```json
{
  "decision": "approved",
  "comment": "Create the planning doc."
}
```

Decision response:

```json
{
  "id": "approval-id",
  "status": "approved",
  "workflowRunId": "run-id",
  "workflowRunStepId": "step-id"
}
```

Validation:

- Only `pending` requests can be approved or rejected.
- Expired requests cannot be approved.
- Decision must be tied to the current authenticated user when user auth is available.

#### P-9: Update Provider MCP Config Generation

Goal:

- Point providers to FlowPilot MCP server for Google Drive when CP-27 is enabled.

Current CP-05-03 config points to:

```text
npx -y @piotr-agier/google-drive-mcp
```

Future CP-27 config should point to:

```text
flowpilot-google-drive-mcp --workspace <workspace> --account-home <accountHome>
```

or local-runner subcommand equivalent.

Codex config shape:

```toml
[mcp_servers.google-drive]
command = "flowpilot-google-drive-mcp"
args = [
  "--workspace", "C:/working/flowpilot",
  "--account-home", "C:/path/to/provider-account"
]
startup_timeout_sec = 20
tool_timeout_sec = 120
enabled = true
enabled_tools = [
  "authGetStatus",
  "search",
  "listFolder",
  "readGoogleDoc"
]
default_tools_approval_mode = "approve"
```

For `read_write`, include only first-release write tools:

```toml
enabled_tools = [
  "authGetStatus",
  "search",
  "listFolder",
  "readGoogleDoc",
  "createGoogleDoc",
  "updateGoogleDoc"
]
```

Important:

- Provider config can expose write tools only when the step/session needs write mode.
- Prefer temporary run-scoped provider config for write mode.
- Do not permanently add write tools to a normal provider account config unless product explicitly approves that.

Implementation options:

- Option A: keep persistent provider config read-only, and generate a temporary account home/config for write-mode steps.
- Option B: write persistent config with all tools but rely on FlowPilot MCP server to reject writes unless run context says `read_write`.

Recommended first implementation:

- Use Option B for implementation simplicity.
- Rely on FlowPilot MCP server access-mode enforcement.
- Still keep destructive tools unregistered.

Stronger later implementation:

- Use Option A for defense in depth.

Tests:

- Provider config points to FlowPilot MCP server, not raw package, when CP-27 feature flag is enabled.
- Read-only config excludes write tools if using tool allowlists.
- Write-mode config includes first-release write tools only.
- Existing unrelated provider config is preserved.

#### P-10: Add UI For Pending Write Approvals

Goal:

- Let the user approve exact write requests.

UI surfaces:

- Workflow run detail page
- Step detail panel
- Global approvals page if existing approval system can be reused

Approval card must show:

- workflow run name/id
- step name/id
- provider
- MCP server
- tool name
- target summary
- arguments preview
- requested time
- expiry time
- approve button
- reject button
- comment box

For `createGoogleDoc`, show:

- document title
- parent folder if available
- content preview or summary

For `updateGoogleDoc`, show:

- document ID
- document title if resolved
- update mode
- content preview or diff if available

Do not show:

- OAuth token values
- credential file contents
- full secret env vars

Tests:

- Pending request appears on workflow run page.
- Approve sends decision request.
- Reject sends decision request.
- Expired request disables approve.
- Arguments preview redacts secrets.

#### P-11: Add Prompt Contract For Write Mode

Goal:

- Make provider behavior predictable.

Read-only prompt section:

```markdown
## Required MCP Usage

This workflow step requires FlowPilot MCP `google_drive`.
Use read-only tools only.
Do not create, update, delete, move, or share Drive files.
If a write is needed, stop and report `DRIVE_WRITE_NOT_ALLOWED`.
```

Write-mode prompt section:

```markdown
## Required MCP Usage

This workflow step requires FlowPilot MCP `google_drive`.
This step may request Google Drive write tools through FlowPilot MCP.
Every write request requires FlowPilot user approval before execution.

Allowed write operations:
- create Google Docs
- update Google Docs

Not allowed:
- delete files
- change permissions
- public sharing
- move unrelated files

If FlowPilot returns `MCP_WRITE_APPROVAL_REQUIRED`, stop and return that code with the approval ID.
After user approval, retry only the exact approved write operation.
```

Tests:

- `read_only` prompt forbids writes.
- `read_write` prompt explains approval behavior.
- Prompt lists exact allowed write operations.
- Prompt lists blocked destructive operations.

#### P-12: Add Audit Artifacts

Goal:

- Preserve write evidence.

Audit artifact example:

```json
{
  "mcpServer": "flowpilot-google-drive",
  "providerMcpServerName": "google-drive",
  "aiProvider": "codex",
  "workflowRunId": "run-id",
  "workflowRunStepId": "step-id",
  "mcpAccessMode": "read_write",
  "approvalId": "approval-id",
  "toolName": "createGoogleDoc",
  "argumentsHash": "sha256...",
  "targetSummary": "Create Google Doc named Sprint Plan",
  "decision": "approved",
  "result": {
    "type": "google_doc",
    "id": "doc-id",
    "url": "https://docs.google.com/document/d/doc-id"
  }
}
```

Rules:

- Store argument hash always.
- Store sanitized arguments only if they do not contain sensitive content.
- Store created/updated file ID and URL.
- Store approval decision and timestamp.

Tests:

- Approved write creates audit artifact.
- Rejected write records rejection.
- Audit artifact does not include tokens.

#### P-13: Add Feature Flag

Goal:

- Avoid changing existing CP-05-03 read-only behavior until CP-27 is ready.

Suggested flag:

```text
FLOWPILOT_GOOGLE_DRIVE_SELF_MCP=true
```

Behavior:

- If disabled:
  - keep current provider config pointing to `@piotr-agier/google-drive-mcp`
  - read-only only
- If enabled:
  - provider config points to FlowPilot MCP server
  - approval model is active

Tests:

- Disabled flag keeps old config.
- Enabled flag writes self-hosted MCP config.

## 5. Touched Areas

Files and modules likely touched:

- `apps/local-runner/internal/runner/google_drive_mcp_provider_config.go`
  - switch provider config command to FlowPilot MCP server when feature flag enabled
  - add access-mode aware config generation
- `apps/local-runner/internal/runner/mcp_prompt_instructions.go`
  - add write-mode prompt instructions
- `apps/local-runner/internal/runner/sessions.go`
  - handle approval-required runtime status
  - preserve step/run context for MCP server launch
- `apps/local-runner/internal/runner/types.go`
  - add MCP access mode and approval DTOs
- `apps/local-runner/internal/cli/root.go`
  - add MCP server subcommand or approval APIs
- new local-runner MCP server package
  - server startup
  - tool registry
  - Google Drive execution
  - approval handling
- `apps/admin-web/src/features/workflow-engine/workflow-start-runtime.ts`
  - pass `mcpAccessMode` into runner/session requests
  - handle `waiting_approval`
- `apps/admin-web/src/domain/model/entity/local-runner.ts`
  - add approval request/response types
- `apps/admin-web/src/routes/_authenticated/workflow-runs/$runId.tsx`
  - show pending MCP write approval
- workflow builder route/components
  - add read/write MCP mode UI
- Supabase migration files
  - add `mcp_access_mode`
  - add `mcp_write_approvals` if approvals are stored in Supabase

External systems:

- Google Drive API
- Google Docs API
- Google Sheets API
- local AI provider CLIs
- provider MCP config files

## 6. Data or Migration Steps

Schema:

- Add `mcp_access_mode text not null default 'read_only'` to workflow step definitions or workflow steps.
- Add runtime copy of `mcp_access_mode` if step runs are denormalized.
- Add `mcp_write_approvals` table if approval records are persisted in Supabase.

Data backfill:

- Existing steps get `read_only`.
- Existing workflow run steps get `read_only`.
- Existing provider MCP config remains read-only until feature flag is enabled.

Config updates:

- Add feature flag for self-hosted MCP server.
- Add provider config command path for FlowPilot MCP server.
- Add tool allowlists for read-only and first write-mode release.

## 7. Validation Plan

Unit tests:

- tool registry by access mode
- approval matching by exact canonicalized arguments
- read-only write rejection
- pending approval creation
- approved write execution
- rejected write behavior
- prompt injection for read-only and read-write
- provider config generation with feature flag disabled/enabled

Integration tests:

- provider config points to self-hosted MCP server
- Codex can list FlowPilot MCP tools
- provider can call a read tool through FlowPilot MCP
- provider write call creates pending approval and does not write
- approval allows exact write execution
- rejected approval blocks write

Manual checks:

1. Enable feature flag.
2. Configure Google Drive MCP auth.
3. Configure Codex provider MCP.
4. Run read-only workflow step.
5. Confirm read tools work.
6. Configure step as read-write.
7. Run step that asks to create a Google Doc.
8. Confirm workflow pauses at pending approval.
9. Approve request.
10. Confirm document is created.
11. Confirm audit artifact includes document ID and URL.
12. Reject another request.
13. Confirm no document is created.

Failure cases:

- provider requests write in read-only mode
- provider requests unregistered destructive tool
- approval expires before decision
- approval arguments do not match retry arguments
- Google auth token missing
- provider account home missing
- MCP server exits unexpectedly

## 8. Rollout and Fallback

Rollout order:

1. Add data model and UI mode but keep all steps read-only.
2. Add FlowPilot MCP server behind feature flag.
3. Route one provider, preferably Codex, to FlowPilot MCP server for read-only smoke tests.
4. Enable write-mode for `createGoogleDoc` only.
5. Add approval UI.
6. Add audit artifacts.
7. Expand to Claude and Gemini.
8. Expand to update tools after create flow is stable.

Fallback path:

- Disable `FLOWPILOT_GOOGLE_DRIVE_SELF_MCP`.
- Provider config returns to CP-05-03 raw Google Drive MCP read-only behavior.
- Existing workflows continue with read-only MCP tools.
- Write-mode steps should be blocked with a clear "write mode unavailable" message.

Monitoring:

- count pending approvals
- count approved/rejected/expired approvals
- count write execution failures
- count provider MCP server startup failures
- count Google auth failures

## 9. Risks

- `R-1` Provider clients may not retry a write tool call cleanly after approval.
- `R-2` Long-running blocked MCP tool calls may not be supported equally across Codex, Claude, and Gemini.
- `R-3` Direct Google API implementation may take longer than proxying the existing package.
- `R-4` Provider config for temporary write sessions may be fragile across provider versions.
- `R-5` Tool arguments may contain large document content and must be summarized/redacted carefully in UI.
- `R-6` A rejected write request could leave the model confused unless the prompt explains the policy.
- `R-7` Supabase mirroring can misrepresent local machine readiness if provider config is machine-local.

## 10. Definition of Done

- CP-05-03 remains read-only for Phase A and Phase B.
- Workflow steps support `mcpAccessMode = read_only | read_write`.
- Default mode is `read_only`.
- FlowPilot has a local MCP server that providers can launch over stdio.
- Provider config can point to FlowPilot MCP server behind a feature flag.
- Read tools work through FlowPilot MCP server.
- Write tools are visible only for write-mode support.
- Write tools do not execute before approval.
- UI shows exact pending write request details.
- User can approve or reject a pending write request.
- Approved exact write executes and returns Drive ID/URL.
- Rejected write does not execute.
- Audit artifact records write decision and result.
- Tests cover read-only, write approval, rejection, expiry, provider config, and prompt instructions.
