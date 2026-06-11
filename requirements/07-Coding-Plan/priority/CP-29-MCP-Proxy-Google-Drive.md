# CP-29: FlowPilot Proxy MCP Server For Google Drive

## Metadata

- Document ID: `CP-29`
- Title: `FlowPilot Proxy MCP Server For Google Drive`
- Phase: `coding_plan`
- Status: `draft`
- Owner: `FlowPilot`
- Reviewers: `TBD`
- Created: `2026-06-09`
- Last Updated: `2026-06-10`
- Parent Documents: [CP-05-03: Google Drive MCP Current Implementation Notes](../priority/CP-05-03-Driver-Mcp.md), [SD-11: MCP Connection Flows](../../06-System-Tech-Design/SD-11-MCP-Connection-Flows.md), [SS-02: Project Context](../../05-System-Specs/SS-02-Project-Context.md), [SS-04: Workflow](../../05-System-Specs/SS-04-Workflow.md)
- Child Documents: `TBD`
- Related Documents: [Task-025: Drive MCP Auth Flow](../../08-Task/Task-025-Drive-MCP-Auth-Flow.md), [CP-27: Google Cloud Setting](../done/CP-27-Google-Cloud-Setting-Manually.md), [CP-28: Google Cloud Config With UI Auto](../done/CP-28-Google-Cloud-Config-With-Ui-Auto.md), [SS-08: Approval Gates & YOLO Mode](../../05-System-Specs/SS-08-Approve-Gate.md), [SD-09: Approval Gates & YOLO Mode](../../06-System-Tech-Design/SD-09-Approval-Gates.md)
- Replaces: `None`
- Tags: `mcp, google-drive, workflow-runtime, write-approval, provider-config, artifact-sync`

## AI Quick View

### Summary

- CP-05-03 now supports Phase A and Phase B Google Drive MCP configuration, but provider-side MCP tool-call approval can block both read and write calls unless configured correctly.
- CP-29 owns the future design for FlowPilot's own proxy MCP server so read/write MCP calls can be controlled, manually approved, or auto-approved in YOLO mode.
- The key architecture change is moving provider config from direct `AI provider -> raw Google Drive MCP` to `AI provider -> FlowPilot MCP server -> Google Drive`.
- The proxy MCP should reuse FlowPilot's runner-local Google OAuth/token/Google API infrastructure and explicit account selection instead of requiring the third-party MCP Desktop OAuth setup.
- Default step mode remains `read_only`; `read_write` is an explicit workflow-step configuration.
- True per-request MCP tool-call approval and auditable YOLO auto-approval are only possible when provider approval mode and FlowPilot MCP policy are controlled together.

### Current Ask

- Explain how a FlowPilot proxy MCP server works in terms a non-MCP expert can understand.
- Define the future implementation plan in enough detail that another AI agent can implement it with minimal ambiguity.

### Key Decisions

- `P-1` Build a FlowPilot-owned MCP server/proxy instead of trying to inspect provider stdout for tool calls.
- `P-2` Use `stdio` transport first because provider clients already support local command-based MCP servers.
- `P-3` Configure AI providers to connect to FlowPilot's MCP server when write-mode observability is required.
- `P-4` Keep `read_only` as the default for all workflow steps.
- `P-5` When workflow run `yolo_mode` is off, FlowPilot proxy approval must require user approval for both read and write tools.
- `P-6` Keep destructive and permission-changing Google Drive tools disabled until a separate policy is approved.
- `P-7` Use direct Google API calls inside FlowPilot MCP instead of proxying to the existing package.
- `P-8` Reuse the existing runner-local Google OAuth, token refresh, and Drive API helper path where possible, but treat proxy MCP auth as selected-account state rather than "the currently connected artifact project".
- `P-9` Bind approval expiry to the active workflow/provider session timeout; when the session is destroyed, pending, approved, or auto-approved write requests expire if they have not executed.
- `P-10` Rejected or failed write MCP calls should let the provider continue and produce a useful output artifact with a clear user-facing notice.
- `P-11` When workflow run `yolo_mode` is on, FlowPilot proxy approval must auto-approve both read and policy-allowed write tools.
- `P-12` FlowPilot write audit remains per exact write tool call; YOLO auto-approval applies only for the active run/session and policy-allowed write tools.
- `P-13` Once proxy MCP is the active Google Drive MCP path, the Drive settings page should no longer require the third-party MCP Desktop OAuth JSON step.
- `P-14` Keep the third-party MCP Desktop OAuth setup only as a legacy/raw-MCP fallback while the proxy MCP feature flag is off.

### Constraints

- FlowPilot currently sends prompts to provider CLIs; provider CLIs own MCP client behavior.
- FlowPilot cannot reliably see exact provider-side Google Drive write tool calls when providers connect directly to the raw Google Drive MCP package.
- Headless workflow runs cannot reliably surface provider-host MCP approval popups, so the proxy path must keep provider-host approval non-blocking and move exact read/write approval to the FlowPilot proxy boundary.
- Supabase must not store Google OAuth tokens, Desktop OAuth JSON contents, or MCP token contents.
- Artifact sync currently proves Google Drive upload/write capability through FlowPilot's own Google API path, but broader proxy MCP tools may need additional scopes and APIs.
- The first implementation should stay local-runner owned and machine-local.
- Provider account-home isolation must continue to work.

### Open Questions

- None for MVP.

### Source Refs

- CP-05-03 Section 11.15 Phase C handoff
- CP-05-03 Phase A and Phase B read-only provider-MCP behavior
- CP-27 Google Cloud console and OAuth setup
- CP-28 Google Drive setup UI and artifact-sync config
- SD-11 MCP connection flows
- SS-04 workflow YOLO mode behavior
- SS-08 approval gate and YOLO mode behavior
- SD-09 approval gate and YOLO mode design
- SS-02 MCP context
- SS-04 workflow step MCP requirement model

## 1. Goal

Implement a future FlowPilot-owned MCP server for Google Drive so workflow steps can safely support:

- default `read_only` MCP access
- explicit `read_write` MCP access
- exact write tool call capture
- user approval before provider MCP tool calls when YOLO mode is off, for both read and write tools
- automatic provider MCP tool-call approval when YOLO mode is on, for read and policy-allowed write tools
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
- [CP-27: Google Cloud Setting](../done/CP-27-Google-Cloud-Setting-Manually.md)
- [CP-28: Google Cloud Config With UI Auto](../done/CP-28-Google-Cloud-Config-With-Ui-Auto.md)

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

This setup is not enough by itself because provider-side MCP `call_tool` approval can block both read and write tools.

Example observed correction:

- Codex config with `default_tools_approval_mode = "prompt"` can wait for interactive approval before calling even read tools.
- Changing Codex config to `default_tools_approval_mode = "approve"` lets the provider call MCP tools automatically.
- That auto-approval behavior is equivalent to YOLO mode for provider MCP tool calls.

FlowPilot still needs to prove:

- Google Drive MCP auth exists
- provider config exists
- provider can call MCP tools under the selected approval mode
- prompt tells the provider to use the right MCP server

Direct provider-to-raw-MCP still does not work for exact FlowPilot write audit/approval because the exact write request happens inside the provider client after FlowPilot has already launched the provider.

Current Google Drive setup status:

- CP-27 and CP-28 already set up the Google Cloud console and Drive settings UI for artifact sync and current MCP package support.
- Artifact sync already uses FlowPilot-owned OAuth callback, refresh-token storage, access-token refresh, and Google Drive upload helpers.
- Artifact sync upload proves FlowPilot can perform Google Drive writes through its own Google API path.
- The third-party MCP Desktop OAuth JSON path exists only because `@piotr-agier/google-drive-mcp` expects its own installed-app OAuth credentials and token file.
- If FlowPilot proxy MCP replaces the third-party MCP package, the target design should reuse artifact-sync Google API infrastructure instead of requiring a separate Desktop OAuth JSON upload.

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
| - chooses normal or YOLO run mode                                |
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
| - stores pending and auto-approved write records                 |
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
| - creates pending or auto-approved records for write tools        |
| - executes manually approved or YOLO auto-approved write tools    |
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
-> if yolo_mode is off:
     provider can reach FlowPilot MCP without a host popup
     FlowPilot MCP creates a pending read approval for the exact tool call
     workflow step moves to waiting_approval
     user approves
     provider reruns and retries the exact approved read call
-> if yolo_mode is on:
     provider can reach FlowPilot MCP without a host popup
     FlowPilot MCP auto-approves the read call and executes immediately
-> provider uses returned Drive context
-> provider returns final answer
```

FlowPilot MCP creates exact-match approval records for read tools when manual approval is required:

- YOLO off: pending approval before executing the exact read or write tool call
- YOLO on: auto-approve the exact read or policy-allowed write tool call

#### 3.6 Write-Mode Flow

Write mode is explicit per workflow step.

Interaction:

```text
User starts workflow
-> step has mcpAccessMode = read_write
-> runtime reads workflowRun.yolo_mode
-> FlowPilot launches provider with write-capable FlowPilot MCP config
-> if yolo_mode is off:
     provider can reach FlowPilot MCP without a host popup
-> if yolo_mode is on:
     provider can reach FlowPilot MCP without a host popup
-> provider calls createGoogleDoc
-> FlowPilot MCP receives the write call
-> if yolo_mode is off:
     FlowPilot MCP creates pending approval record
     workflow step moves to waiting_approval
     UI shows exact tool, operation, target summary, and expiry details
     user approves
     provider reruns and retries the exact approved write call
     FlowPilot MCP executes the stored write call
-> if yolo_mode is on:
     FlowPilot MCP creates an auto-approved audit record
     FlowPilot MCP executes the policy-allowed write call immediately
-> Google Drive returns created document ID and URL
-> FlowPilot MCP returns tool result to provider
-> provider returns final answer
-> FlowPilot records write audit metadata
```

If user rejects:

```text
User rejects
-> FlowPilot MCP returns a structured rejected-write notice
-> provider continues without executing the write
-> provider produces a final answer or artifact with a clear notice that the write was rejected
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
- `read_write`: expose read tools and selected write tools; write execution follows run approval mode.

Important rule:

- `read_write` does not mean "the AI can write freely".
- `read_write` means "the AI may request write tools; provider MCP `call_tool` approval follows YOLO mode, and FlowPilot MCP enforces write policy/audit".

#### 3.8 Recommended Write Tool Coverage

FlowPilot MCP should mirror the `@piotr-agier/google-drive-mcp` package tool surface where possible so provider prompts and expectations stay compatible.

Implement direct Google API handlers and classify each tool with access and risk metadata:

- provider MCP `call_tool` requests for read tools require user approval when YOLO mode is off
- provider MCP `call_tool` requests for read tools are auto-approved when YOLO mode is on
- provider MCP `call_tool` requests for non-destructive create/update/upload tools require user approval when YOLO mode is off
- provider MCP `call_tool` requests for non-destructive create/update/upload tools are auto-approved when YOLO mode is on
- destructive and permission-changing tools remain disabled until a separate policy is approved

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
  - write requests require manual approval when YOLO mode is off
  - write requests are auto-approved and audited when YOLO mode is on
  - destructive and permission-changing tools are disabled unless a separate policy enables them

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
- `--yolo-mode`: `true` or `false`, copied from the active workflow run
- `--approval-api-url`: optional local runner URL if the MCP server calls the runner API to create approval records

Environment:

- `FLOWPILOT_WORKSPACE`
- `FLOWPILOT_ACCOUNT_HOME`
- `FLOWPILOT_WORKFLOW_RUN_ID`
- `FLOWPILOT_WORKFLOW_STEP_RUN_ID`
- `FLOWPILOT_YOLO_MODE`
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

Write tools:

```text
Mirror the @piotr-agier/google-drive-mcp package tool surface where possible.
```

Implementation requirement:

- Discover and document the current upstream package tool list during implementation.
- Keep FlowPilot tool names compatible with the package names unless there is a safety reason to rename.
- Implement direct Google API handlers for supported tools.
- Mark every tool as read, non-destructive write, destructive write, or permission-changing write.
- Register only read tools in `read_only`.
- Register read tools and policy-allowed write tools in `read_write`.

Tool metadata requirements:

- name
- description
- input schema
- access type:
  - `read`
  - `write`
- destructive flag:
  - `true` for destructive tools
  - `false` for non-destructive tools
- FlowPilot proxy approval:
  - `required_when_yolo_off` for read tools
  - `required_when_yolo_off` for write tools
  - `auto_approved_when_yolo_on` for read and policy-allowed write tools

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
	RequiresProviderCallApprovalWhenYoloOff bool
}
```

Rules:

- In `read_only`, register only read tools.
- In `read_write`, register read tools and policy-allowed write tools.
- Do not register destructive tools in first release.

Tests:

- `read_only` mode never lists write tools.
- `read_write` mode lists package-equivalent write tools allowed by policy.
- Destructive tools are absent even in `read_write`.

#### P-4: Implement Google Drive Execution Layer

Goal:

- Make the FlowPilot MCP server actually read and write Google Drive.

Two possible approaches were considered:

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

Selected implementation:

- Use Approach A.
- FlowPilot MCP calls Google APIs directly.
- Do not proxy write execution to `@piotr-agier/google-drive-mcp`.
- Match the package tool surface where possible so existing provider behavior remains compatible.
- Reuse runner-local Google OAuth connection, refresh-token storage, access-token refresh, and Drive API helpers where possible.
- Implement tools in priority order if needed, but the CP-29 target scope is package-equivalent support subject to FlowPilot safety policy and available OAuth scopes.

Credential rules:

- Use the selected FlowPilot Google account as the primary OAuth source for proxy MCP.
- Treat artifact sync as a separate `projectId -> accountId -> folderId` binding under that selected account.
- Refresh access tokens through the existing runner token refresh path.
- Keep MCP Desktop OAuth credential/token paths only for legacy/raw-MCP fallback while the third-party package remains available.
- Never return token contents in tool results.
- Never log token contents.
- Reuse artifact-sync reconnect handling when Google returns invalid-grant or authorization failures.
- Add scope readiness checks before exposing proxy MCP tools that need Docs, Sheets, Slides, permissions, or broader Drive access.

Tests:

- Missing selected Google account fails startup or tool call clearly.
- Missing required scopes on the selected account fail with reconnect-required or missing-scope status.
- Expired/revoked selected-account refresh token fails with `MCP_AUTH_REQUIRED` or reconnect-required status.
- Search returns file name and ID.
- Read doc returns file name, ID, and content.
- Write tools return stable IDs, URLs when available, and enough result metadata for audit artifacts.

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
decision_mode text not null
expires_at timestamptz not null
result_json jsonb nullable
error_message text nullable
```

Status values:

- `pending`
- `approved`
- `auto_approved`
- `rejected`
- `expired`
- `executed`
- `failed`

Unique/safety behavior:

- One approval record represents one exact tool call.
- If the provider retries with different arguments, create a new approval record.
- Do not reuse approval across different step runs.
- In normal mode, write requests start as `pending`.
- In YOLO mode, policy-allowed write requests start as `auto_approved` with `decision_mode = "yolo"`.
- Set `expires_at` from the active workflow/provider session timeout.
- Expire pending, approved, or auto-approved-but-not-executed approvals when the session is destroyed or times out.

Tests:

- Creating a write request inserts `pending`.
- Creating a write request in YOLO mode inserts `auto_approved` and does not wait for user input.
- Approval changes status to `approved`.
- Rejection changes status to `rejected`.
- Expired requests cannot execute.
- Session timeout expires pending, approved, and auto-approved-but-not-executed requests.
- Approval for one step cannot execute a write from another step.

#### P-6: Implement Write Tool Handling

Goal:

- Capture exact write tool calls and pause before execution.

Handler flow for every tool:

```text
Receive tool call
-> identify tool metadata
-> if read tool:
     if yolo_mode is on:
         create auto_approved approval record
         execute immediately
     if yolo_mode is off and no matching approved approval:
         create pending approval record
         signal workflow waiting_approval
         return error MCP_TOOL_APPROVAL_REQUIRED with approval id
     if yolo_mode is off and matching approval exists:
         execute read
         save result_json
         mark approval executed
         return tool result
-> if write tool and access mode is not read_write:
     return error DRIVE_WRITE_NOT_ALLOWED
-> if write tool and yolo_mode is on and tool is policy-allowed:
     create auto_approved approval/audit record
     execute write immediately
     save result_json
     mark approval executed
     return tool result
-> if write tool and yolo_mode is off and no matching approved approval:
     create pending approval record
     signal workflow waiting_approval
     return error MCP_WRITE_APPROVAL_REQUIRED with approval id
-> if write tool and yolo_mode is off and matching approval exists:
     execute write
     save result_json
     mark approval executed
     return tool result
-> if write tool was rejected or write execution fails:
     return a structured non-executed result that tells the provider to continue
     include a user-facing notice for the final output artifact
```

Important behavior:

- When YOLO mode is off, FlowPilot proxy approval must block exact read or write tool execution until the user approves.
- When YOLO mode is off, a write must not execute before FlowPilot write approval.
- When YOLO mode is on, policy-allowed writes are auto-approved before execution and must still create audit records.
- Approval must match:
  - workflow run ID
  - workflow step run ID
  - tool name
  - exact canonicalized arguments
- Approval must also belong to the active workflow/provider session.
- Canonicalize JSON arguments before matching to avoid whitespace/order differences.

Tool error text:

```text
MCP_TOOL_APPROVAL_REQUIRED: FlowPilot created approval request <approvalId>. Wait for user approval before retrying this exact Google Drive tool call.
MCP_WRITE_APPROVAL_REQUIRED: FlowPilot created approval request <approvalId>. Wait for user approval before retrying this exact write.
```

Runtime issue:

- Some providers may stop after receiving the approval-required error when YOLO mode is off.
- First normal-mode implementation may require the workflow step to pause and then re-run the provider step after approval.
- A later implementation can support a true wait/resume if the provider transport supports long-running blocked tool calls.

Recommended MVP behavior:

- If YOLO mode is off, on first write request, create approval and fail/pause the current provider call.
- FlowPilot marks step `waiting_approval`.
- After approval, FlowPilot reruns the step with a prompt note:
  - "The previous write request approval ID X is approved. Retry the same write operation."
- FlowPilot MCP server recognizes the approved exact call and executes it.
- If YOLO mode is on, FlowPilot MCP auto-approves policy-allowed write requests and does not pause the provider call.

Tests:

- Read tool executes after provider MCP `call_tool` approval mode allows the call.
- Write tool in `read_only` returns `DRIVE_WRITE_NOT_ALLOWED`.
- Write tool in `read_write` with YOLO off creates `pending` and does not execute.
- Write tool in `read_write` with YOLO on creates `auto_approved` and executes immediately.
- Approved exact write executes.
- Approved different write does not execute.
- Rejected write returns a structured notice and lets the provider continue without executing the write.

#### P-7: Add Workflow Runtime State Handling

Goal:

- Pause workflow runs when MCP write approval is required and YOLO mode is off.
- Skip the pause for policy-allowed write requests when YOLO mode is on.

Runtime state additions:

- Add or reuse step status:
  - `waiting_approval`
- Add metadata on workflow run step:
  - `pendingMcpWriteApprovalId`
  - `mcpWriteApprovalStatus`

Flow:

```text
If yolo_mode is off and provider output or MCP server signal says MCP_WRITE_APPROVAL_REQUIRED
-> runtime extracts approval id
-> workflow_run_steps.status = waiting_approval
-> workflow run remains paused
-> UI shows approval request
-> user approves or rejects
-> approved: runtime can replay/resume step
-> rejected: runtime resumes or completes the step with a user-facing notice and no write execution

If yolo_mode is on and the MCP server auto-approves a policy-allowed write
-> workflow_run_steps.status remains running
-> runtime receives audit metadata after execution
-> workflow continues without waiting for user input
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
- Rejecting a request resumes or completes the step with a user-facing notice and no write execution.
- YOLO-mode auto-approval does not move the step to `waiting_approval`.
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
  "requestedByProvider": "codex",
  "yoloMode": false
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
- YOLO-mode requests are created as `auto_approved`; they do not accept manual approve/reject decisions.

#### P-9: Update Provider MCP Config Generation

Goal:

- Point providers to FlowPilot proxy MCP server for Google Drive when CP-29 is enabled.

Current CP-05-03 config points to:

```text
npx -y @piotr-agier/google-drive-mcp
```

Future CP-29 config should point to:

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
default_tools_approval_mode = "<approve>"
```

Approval mode mapping:

- `yolo_mode = false` -> `default_tools_approval_mode = "approve"`
- `yolo_mode = true` -> `default_tools_approval_mode = "approve"`

For the proxy path, host-side provider approval stays non-blocking so the provider can always reach FlowPilot MCP. The active run's `--yolo-mode` flag tells FlowPilot MCP whether to create manual approvals or auto-approve exact tool calls.

For `read_write`, include package-equivalent write tools allowed by policy:

```toml
enabled_tools = [
  "authGetStatus",
  "search",
  "listFolder",
  "readGoogleDoc",
  "createGoogleDoc",
  "updateGoogleDoc",
  "...additional package-equivalent policy-allowed write tools"
]
```

Important:

- Provider host approval must not block the proxy path during headless workflow execution.
- In non-YOLO runs, FlowPilot MCP must create manual approvals before executing exact read or write tool calls.
- In YOLO runs, FlowPilot MCP must auto-approve exact read and policy-allowed write tool calls.
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

- Provider config points to FlowPilot MCP server, not raw package, when CP-29 feature flag is enabled.
- Read-only config excludes write tools if using tool allowlists.
- Write-mode config includes package-equivalent policy-allowed write tools.
- Codex proxy config uses `default_tools_approval_mode = "approve"` so the host does not hide approvals from FlowPilot.
- The proxy server arguments still carry `--yolo-mode=true|false` so FlowPilot owns the final approval behavior.
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
Provider MCP tool-call approval mode for this run: `<manual | yolo_auto_approve>`.

If approval mode is `manual`, call Google Drive tools through FlowPilot MCP and wait for FlowPilot to return the next approval-required status when a tool needs approval.
If approval mode is `yolo_auto_approve`, Google Drive MCP read tools can be called without waiting for user approval.
If FlowPilot returns `MCP_TOOL_APPROVAL_REQUIRED`, stop and return that code with the approval ID.

Use read-only tools only.
Do not create, update, delete, move, or share Drive files.
If a write is needed, stop and report `DRIVE_WRITE_NOT_ALLOWED`.
```

Write-mode prompt section:

```markdown
## Required MCP Usage

This workflow step requires FlowPilot MCP `google_drive`.
This step may request Google Drive write tools through FlowPilot MCP.
Provider MCP tool-call approval mode for this run: `<manual | yolo_auto_approve>`.

If approval mode is `manual`, call Google Drive tools through FlowPilot MCP and wait for FlowPilot approval-required responses for exact read or write tool calls.
If approval mode is `yolo_auto_approve`, read and policy-allowed write tools can be called without waiting for user approval.
FlowPilot MCP still records policy-allowed write executions for audit.

Allowed write operations:
- package-equivalent policy-allowed Google Drive tools

Not allowed:
- delete files
- change permissions
- public sharing
- move unrelated files

If FlowPilot returns `MCP_TOOL_APPROVAL_REQUIRED` or `MCP_WRITE_APPROVAL_REQUIRED`, stop and return that code with the approval ID.
After user approval, retry only the exact approved write operation.
If FlowPilot returns a rejected-write or failed-write notice, continue without executing the write and include the notice in the final artifact.
```

Tests:

- `read_only` prompt forbids writes.
- `read_only` prompt explains manual versus YOLO provider `call_tool` approval behavior.
- `read_write` prompt explains manual provider `call_tool` approval behavior.
- `read_write` prompt explains YOLO provider `call_tool` auto-approval behavior when the run is in YOLO mode.
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
  "yoloMode": false,
  "approvalId": "approval-id",
  "toolName": "createGoogleDoc",
  "argumentsHash": "sha256...",
  "targetSummary": "Create Google Doc named Sprint Plan",
  "decision": "approved",
  "decisionMode": "manual",
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
- Store approval decision, decision mode, YOLO mode, and timestamp.

Tests:

- Approved write creates audit artifact.
- YOLO auto-approved write creates audit artifact.
- Rejected write records rejection.
- Audit artifact does not include tokens.

#### P-13: Add Feature Flag And Legacy MCP Fallback

Goal:

- Avoid changing existing CP-05-03 read-only behavior until CP-29 proxy MCP is ready.

Suggested flag:

```text
FLOWPILOT_GOOGLE_DRIVE_PROXY_MCP=true
```

Behavior:

- If disabled:
  - keep current provider config pointing to `@piotr-agier/google-drive-mcp`
  - read-only only
  - keep the current Desktop OAuth JSON upload/auth UI available for legacy raw-MCP mode
- If enabled:
  - provider config points to FlowPilot MCP server
  - manual approval and YOLO auto-approval model is active
  - proxy MCP uses selected-account Google OAuth/token/Google API infrastructure
  - Drive settings no longer require the Desktop OAuth JSON upload step for proxy MCP readiness

Tests:

- Disabled flag keeps old config.
- Enabled flag writes proxy MCP config.
- Enabled flag treats the selected Google account as the proxy MCP auth source.
- Enabled flag does not require MCP Desktop OAuth JSON for proxy MCP readiness.

## 5. Touched Areas

Files and modules likely touched:

- `apps/local-runner/internal/runner/google_drive_mcp_provider_config.go`
  - switch provider config command to FlowPilot MCP server when feature flag enabled
  - add access-mode aware config generation
- `apps/local-runner/internal/runner/mcp_prompt_instructions.go`
  - add write-mode prompt instructions
- `apps/local-runner/internal/runner/sessions.go`
  - handle approval-required runtime status
  - preserve step/run context and `yolo_mode` for MCP server launch
- `apps/local-runner/internal/runner/types.go`
  - add MCP access mode and approval DTOs
- `apps/local-runner/internal/cli/root.go`
  - add MCP server subcommand or approval APIs
- new local-runner MCP server package
  - server startup
  - tool registry
  - Google Drive execution
  - approval handling
- `apps/local-runner/internal/runner/artifact_google_drive_connection.go`
  - reuse Google OAuth connection and refresh-token ownership for proxy MCP
- `apps/local-runner/internal/runner/artifact_cloud_storage.go`
  - extract reusable Google Drive API client/upload/list helpers for proxy MCP
- `apps/local-runner/internal/runner/google_drive_config.go`
  - separate legacy raw-MCP Desktop OAuth readiness from proxy MCP readiness
- `apps/admin-web/src/features/workflow-engine/workflow-start-runtime.ts`
  - pass `mcpAccessMode` and `yoloMode` into runner/session requests
  - handle `waiting_approval`
- `apps/admin-web/src/domain/model/entity/local-runner.ts`
  - add approval request/response types
- `apps/admin-web/src/routes/_authenticated/workflow-runs/$runId.tsx`
  - show pending MCP write approval
- `apps/admin-web/src/routes/_authenticated/settings/google-drive-setup.tsx`
  - stop requiring Desktop OAuth JSON upload for proxy MCP readiness
  - keep Desktop OAuth JSON upload only for legacy raw-MCP fallback while the third-party package remains supported
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
- Add `decision_mode` or equivalent audit field for manual versus YOLO auto-approval.

Data backfill:

- Existing steps get `read_only`.
- Existing workflow run steps get `read_only`.
- Existing provider MCP config remains read-only until feature flag is enabled.

Config updates:

- Add feature flag for proxy MCP server.
- Add provider config command path for FlowPilot MCP server.
- Add tool allowlists for read-only and policy-allowed write mode.
- Mark Desktop OAuth MCP config as legacy/raw-MCP only when proxy MCP is enabled.

## 7. Validation Plan

Unit tests:

- tool registry by access mode
- approval matching by exact canonicalized arguments
- read-only write rejection
- pending approval creation
- YOLO auto-approval creation
- approved write execution
- YOLO auto-approved write execution
- rejected write notice behavior
- prompt injection for read-only and read-write
- provider config generation with feature flag disabled/enabled
- proxy MCP readiness uses artifact-sync Google Drive connection, not Desktop OAuth JSON
- legacy raw-MCP readiness still uses Desktop OAuth JSON while fallback is supported
- non-YOLO provider config uses manual MCP `call_tool` approval for read and write tools
- YOLO provider config uses automatic MCP `call_tool` approval for read and policy-allowed write tools

Integration tests:

- provider config points to proxy MCP server
- Codex can list FlowPilot MCP tools
- non-YOLO read tool call creates a pending FlowPilot approval and pauses the workflow
- YOLO provider can call a read tool through FlowPilot MCP without manual approval
- provider write call creates pending approval and does not write
- provider write call in YOLO mode creates auto-approved audit record and executes immediately
- approval allows exact write execution
- rejected approval blocks write execution but still lets the provider produce a final artifact with notice
- proxy MCP can reuse the artifact-sync Google Drive connection to perform a Drive write

Manual checks:

1. Enable feature flag.
2. Confirm artifact sync Google Drive connection is configured and can upload an artifact.
3. Configure Codex provider MCP.
4. Run read-only workflow step with YOLO off.
5. Confirm FlowPilot pauses on a pending Google Drive read approval before executing the read tool.
6. Enable YOLO mode.
7. Run read-only workflow step again.
8. Confirm the proxy executes read tools without waiting for manual approval.
9. Disable YOLO mode.
10. Configure step as read-write.
11. Run step that asks to create a Google Doc.
12. Confirm FlowPilot pauses on a pending Google Drive write approval before executing the write tool.
13. Confirm workflow pauses at pending write approval.
14. Approve request.
15. Confirm document is created.
16. Confirm audit artifact includes document ID and URL.
17. Reject another request.
18. Confirm no document is created.
19. Enable YOLO mode for the run.
20. Run the same write-mode step.
21. Confirm the document is created without waiting for provider or FlowPilot approval.
22. Confirm audit artifact marks the write as YOLO auto-approved.
23. Confirm Desktop OAuth JSON is not required for proxy MCP readiness.
24. Disable feature flag.
25. Confirm legacy raw-MCP fallback still uses Desktop OAuth JSON readiness if fallback remains supported.

Failure cases:

- provider requests write in read-only mode
- provider requests unregistered destructive tool
- approval expires before decision
- YOLO auto-approved request expires if session ends before execution
- approval arguments do not match retry arguments
- Google auth token missing
- artifact-sync Google Drive connection missing
- artifact-sync scope is insufficient for a requested proxy MCP tool
- provider account home missing
- MCP server exits unexpectedly

## 8. Rollout and Fallback

Rollout order:

1. Add data model and UI mode but keep all steps read-only.
2. Add FlowPilot MCP server behind feature flag.
3. Reuse artifact-sync Google connection and Drive API helpers for proxy MCP read-only smoke tests.
4. Route one provider, preferably Codex, to FlowPilot MCP server for read-only smoke tests.
5. Remove Desktop OAuth JSON as a proxy MCP readiness requirement.
6. Enable write-mode for package-equivalent policy-allowed tools.
7. Add approval UI.
8. Add audit artifacts.
9. Expand to Claude and Gemini.
10. Expand provider coverage after Codex behavior is stable.

Fallback path:

- Disable `FLOWPILOT_GOOGLE_DRIVE_PROXY_MCP`.
- Provider config returns to CP-05-03 raw Google Drive MCP read-only behavior.
- Existing workflows continue with read-only MCP tools.
- Write-mode steps should be blocked with a clear "write mode unavailable" message.
- Legacy raw-MCP fallback may continue to use Desktop OAuth JSON while the third-party package remains supported.

Monitoring:

- count pending approvals
- count approved/auto-approved/rejected/expired approvals
- count write execution failures
- count provider MCP server startup failures
- count Google auth failures
- count insufficient-scope failures by proxy MCP tool

## 9. Risks

- `R-1` Provider clients may not retry a write tool call cleanly after approval.
- `R-2` Long-running blocked MCP tool calls may not be supported equally across Codex, Claude, and Gemini.
- `R-3` Direct Google API implementation may take longer than proxying the existing package.
- `R-4` Provider config for temporary write sessions may be fragile across provider versions.
- `R-5` Tool arguments may contain large document content and must be summarized/redacted carefully in UI.
- `R-6` A rejected write request could leave the model confused unless the prompt explains the policy.
- `R-7` Supabase mirroring can misrepresent local machine readiness if provider config is machine-local.
- `R-8` Artifact-sync OAuth currently proves Drive file upload, but broader proxy MCP tools may require additional scopes or enabled Google APIs.

## 10. Definition of Done

- Status after follow-up fix on `2026-06-09`: proxy readiness, provider config generation, read-path proxying, pending write approval persistence, approval/rejection UI submission, proxy preflight, session-bound approval expiry, scoping validation, YOLO auto-approval behavior, workflow `mcpAccessMode`, audit artifacts, and keychain-free runner tests are implemented.

- [x] CP-05-03 remains read-only for Phase A and Phase B.
- [x] Workflow steps support `mcpAccessMode = read_only | read_write`.
- [x] Default mode is `read_only`.
- [x] FlowPilot has a local MCP server that providers can launch over stdio.
- [x] Provider config can point to FlowPilot MCP server behind a feature flag.
- [x] Proxy MCP reuses the artifact-sync Google Drive connection and shared Google API helpers.
- [x] Proxy MCP readiness does not require Desktop OAuth JSON.
- [x] Desktop OAuth JSON upload remains only for legacy raw-MCP fallback, if that fallback is still supported.
- [x] Read tools work through FlowPilot MCP server.
- [x] With YOLO mode off, FlowPilot MCP requires approval before executing both read and write tools.
- [x] With YOLO mode on, FlowPilot MCP auto-approves read and policy-allowed write tools.
- [x] Write tools are visible only for write-mode support.
- [x] With YOLO mode off, write tools do not execute before user approval.
- [x] With YOLO mode on, policy-allowed write tools auto-approve and execute without waiting for user input.
- [x] UI shows exact pending write request details.
- [x] User can approve or reject a pending write request.
- [x] Approved exact write executes and returns Drive ID/URL.
- [x] YOLO auto-approved exact write executes and returns Drive ID/URL.
- [x] Rejected write does not execute.
- [x] Audit artifact records write decision, decision mode, YOLO mode, and result.
- [x] Tests cover read-only tool-call approval, manual write approval, YOLO tool-call auto-approval, rejection, expiry, provider config, and prompt instructions.

## 11. Manually Test Guide

Use this guide to validate the current proxy MCP slice and to track the remaining CP-29 acceptance checks. Steps marked `Current expected result` should pass with the current implementation. Steps marked `Target expected result` describe the full CP-29 behavior and currently expose the remaining open work.

1. Check config file in Ai provider config
Current expected result: `/google-drive-config` reports `mcp.proxyMcpEnabled = true`, and provider config generation points to the FlowPilot `google-drive-mcp` subcommand instead of `@piotr-agier/google-drive-mcp`.

2. Save artifact-sync Google OAuth values in the Google Drive setup page or runner config.
Web test steps:
- Open `/settings/google-drive-setup`.
- In the artifact-sync section, enter `clientId`, `clientSecret`, and `redirectUri`.
- Click the save action for the artifact-sync credentials.
- Refresh the page and confirm the saved values are reflected in status, with the secret field remaining masked or empty.
Current expected result: artifact-sync status becomes configured when `clientId`, `redirectUri`, and `clientSecret` are present. Desktop OAuth JSON is not required for proxy readiness.

3. Connect at least one Google Drive account and select it for proxy MCP.
Web test steps:
- Open `/settings/google-drive-setup`.
- Start a Google account connection, finish OAuth, then save that account as the active proxy MCP account.
- Return to the status section and confirm the selected account shows proxy-read readiness.
Current expected result: proxy MCP read tools can refresh an access token through the selected-account credential path. If zero or multiple connected accounts exist and no account is selected, the proxy server fails clearly instead of guessing.

4. Legacy desktop OAuth: Leave the legacy Desktop OAuth JSON and token files absent, then refresh Google Drive status.
Web test steps:
- Do not upload the legacy Desktop OAuth JSON on `/settings/google-drive-setup`.
- Keep the proxy flag enabled from step 1.
- Click the page refresh action or reload the browser tab.
- Review the legacy MCP area and the proxy MCP area on the same page.
Current expected result: proxy provider setup still works when the proxy flag is enabled, but the legacy MCP section remains visible as fallback-only guidance.

5. Configure Codex, Gemini, and Claude provider MCP settings from the Google Drive setup page while the proxy flag is enabled.
Web test steps:
- Open `/settings/google-drive-setup`.
- Find the provider config cards for Codex, Gemini, and Claude.
- For each provider, trigger the configure or repair action shown by the card.
- Expand or inspect the rendered provider config preview in the page if available, then refresh to verify the configured state persists.
Current expected result: each provider config points to `flowpilot google-drive-mcp --workspace <workspace> --account-home <home> --mode <mode>`, includes `FLOWPILOT_GOOGLE_DRIVE_PROXY_MCP=true`, and reports configured unless the existing config is stale or invalid.

6. Run a read-only MCP smoke test with proxy mode enabled and a provider account that has a configured `google-drive` server.
Web test steps:
- Re-enable the proxy flag and confirm step 1 still passes.
- Open `/workflow-steps/create`.
- Create or reuse a step definition with `Required MCPs` including `google_drive` and set `Google Drive access` to `Read only`.
- Use that step in a workflow, start a workflow run from the project UI, then open `/workflow-runs/$runId` and inspect the step output and logs for the MCP read calls.
Current expected result: `authGetStatus`, `search`, `listFolder`, and Google Doc read tools execute through the FlowPilot proxy MCP and use the selected-account credential source.

7. Re-run the read-only smoke test after removing artifact-sync client secret, client ID, redirect URI, the selected account token, or the selected account scopes.
Web test steps:
- Open `/settings/google-drive-setup` and clear one required artifact-sync value, disconnect the selected account, or reconnect the selected account with insufficient scopes.
- Save the change and refresh the page.
- Start the same read-only workflow again.
- Open `/workflow-runs/$runId` and inspect the failed step message and logs in the run detail page.
Current expected result: preflight fails before prompt execution and returns a clear proxy-auth or connection error instead of silently falling back to Desktop OAuth JSON.

8. Generate provider config for `read_only` mode.
Web test steps:
- Open `/workflow-steps/create` or `/workflow-steps/$stepType` for a Google Drive step.
- Set `Required MCPs` to include `google_drive`.
- Set `Google Drive access` to `Read only` and save the step definition.
- Return to `/settings/google-drive-setup`, refresh provider status, and inspect the provider config shown by the page for that account.
Current expected result: only read tools are exposed in the provider config. `createGoogleDoc`, `updateGoogleDoc`, and `createFolder` are absent from the read-only allowlist.

9. Generate provider config for `read_write` mode.
Web test steps:
- Open `/workflow-steps/create` or `/workflow-steps/$stepType` for the same Google Drive step.
- Keep `Required MCPs` including `google_drive`.
- Change `Google Drive access` to `Read + write` and save.
- Return to `/settings/google-drive-setup`, refresh provider status, and inspect the provider config again.
Current expected result: read tools plus `createGoogleDoc`, `updateGoogleDoc`, and `createFolder` are exposed in the provider config. This verifies tool-surface gating only; it does not verify the approval workflow.

10. Generate provider config with YOLO off and inspect provider settings.
Web test steps:
- Open `/workflow-runs/$runId` for a run on the target workflow.
- Confirm the `YOLO` toggle in the run header is off.
- Return to `/settings/google-drive-setup` and refresh the provider config cards.
- Inspect the Codex provider config and compare it with the Gemini and Claude cards for the same account.
Current expected result: Codex uses approve-style MCP host configuration for the proxy server even with YOLO off, and Gemini and Claude remain proxy-based and account-home aware.
Target expected result: FlowPilot proxy approval is proven end-to-end for both read and write tools with YOLO off across the supported providers.

11. Generate provider config with YOLO on and inspect provider settings.
Web test steps:
- Open `/workflow-runs/$runId`.
- Toggle `YOLO` on in the run header and wait for the UI to settle.
- Return to `/settings/google-drive-setup` and refresh the provider config cards.
- Inspect the provider config again and compare the arguments and approval mode with step 11.
Current expected result: Codex still uses approve-style MCP host configuration for the proxy server, and the `--yolo-mode` flag is present in proxy server arguments.
Target expected result: FlowPilot proxy auto-approval is proven end-to-end for read and policy-allowed write tools with YOLO on across the supported providers.

12. Run a `read_write` workflow or MCP session that attempts `createGoogleDoc` while YOLO is off.
Web test steps:
- Open `/workflow-steps/$stepType` for a Google Drive step and confirm `Google Drive access` is `Read + write`.
- Start a workflow run from the project page or workflow launcher with the run `YOLO` setting left off.
- Open `/workflow-runs/$runId`.
- Wait for the Google Drive step to enter `waiting_approval`, then inspect the step card, timeline, and error message block.
Current expected result: CP-29 manual write approval is implemented. The write does not execute immediately, FlowPilot creates a pending approval record, the workflow pauses in `waiting_approval`, and the run UI shows the exact tool, operation, target summary, and expiry details.
Target expected result: the write does not execute immediately, FlowPilot creates a pending approval record, the workflow pauses in `waiting_approval`, and the UI shows the exact tool, operation, target summary, and expiry details.

13. Approve the pending write request after step 13.
Web test steps:
- Stay on `/workflow-runs/$runId` while the step is in `waiting_approval`.
- Select the waiting step in the timeline if it is not already selected.
- Click `Approve & Continue`.
- Watch the run page until the step resumes, then inspect the latest logs and outputs for the resumed step.
Current expected result: the approval API/UI is implemented. Approving the pending request resumes the step with an exact-write retry prompt, and the proxy only executes the exact approved tool and arguments. Full manual verification of the returned Drive ID/URL is still pending.
Target expected result: FlowPilot executes only the exact approved write call and returns the created or updated Drive item ID and URL.

14. Reject the pending write request after step 13.
Web test steps:
- Re-run step 13 so the run is back in `waiting_approval`.
- Open `/workflow-runs/$runId` and select the waiting step.
- Optionally enter a comment in the follow-up text box.
- Click `Reject & Retry` and inspect the resumed step output and final run status.
Current expected result: the approval API/UI is implemented. Rejecting the pending request resumes the provider with a clear rejected-write notice, and the proxy refuses to execute the rejected exact write on retry.
Target expected result: the write does not execute, the provider continues with a clear rejected-write notice, and the run remains auditable.

15. Run the same `read_write` workflow or MCP session with YOLO on.
Web test steps:
- Start the same workflow again.
- Open `/workflow-runs/$runId` immediately after creation.
- Toggle `YOLO` on in the run header before the Google Drive write step executes, or start the run from a path that already has YOLO enabled.
- Watch the run timeline and logs until the Google Drive write step finishes.
Current expected result: provider config and proxy approval state carry YOLO intent, and the proxy auto-approves policy-allowed writes for the active run/session. Successful write results are captured in proxy approval state and mirrored into write audit artifacts.
Target expected result: the policy-allowed write auto-approves for the active run, executes immediately, and returns the Drive item ID and URL without waiting for user input.

16. Inspect artifacts and logs after any successful write-path execution.
Web test steps:
- Open `/workflow-runs/$runId` for the run from step 14 or step 16.
- Inspect the step output, run timeline, and any artifact links shown in the run detail UI.
- Open the project artifact browser or artifact run detail linked from the workflow run.
- Verify the Google Drive write audit artifact exists and inspect its stored JSON content through the web UI if the artifact viewer exposes it.
Current expected result: CP-29 write audit artifacts are created from completed or rejected proxy approval records.
Target expected result: FlowPilot records audit metadata that includes `mcpAccessMode`, `yoloMode`, approval ID if applicable, tool name, argument hash, sanitized target summary, decision, decision mode, and resulting Drive ID/URL without leaking secrets.

17. Validate prompt injection for Google Drive-required steps.
Web test steps:
- Open `/workflow-runs/$runId` for a run that used a Google Drive step.
- Inspect the step session output, prompt summary, and related logs shown in the run detail view.
- Compare a `Read only` run and a `Read + write` run, then compare a YOLO-off run and a YOLO-on run.
- Confirm the displayed prompt or logs mention the MCP guidance that matches the selected step mode and run mode.
Current expected result: the runner injects `read_only` versus `read_write` guidance, manual versus YOLO MCP approval behavior, the `MCP_WRITE_APPROVAL_REQUIRED` retry rule, and preflight readiness checks.
Target expected result: the prompt contract explicitly states `read_only` versus `read_write`, manual versus YOLO MCP call approval behavior, the `MCP_WRITE_APPROVAL_REQUIRED` retry rule, and destructive-write prohibitions.

18. Validate regression behavior with the proxy flag off after completing the proxy checks.
Web test steps:
- Stop the runner and restart it with the proxy flag disabled.
- Open `/settings/google-drive-setup` and refresh status.
- Revisit a step definition that uses `google_drive`, then start a read-only run from the project workflow UI.
- Open `/workflow-runs/$runId` and confirm the legacy raw-MCP prerequisites and failure modes match the fallback design rather than the proxy path.
Current expected result: the legacy raw MCP fallback still relies on Desktop OAuth JSON plus token auth, and existing CP-05-03 read-only behavior remains intact.

19. Close the feature only after every checked item in Definition of Done can be demonstrated by this guide without using the previous `Current expected result` exceptions.
Web test steps:
- Re-run steps 1 through 19 against the current build without changing the document expectations mid-run.
- Capture one passing web evidence point for each item: setup status, provider config state, workflow run state, approval action, or audit artifact.
- Compare the observed results against Section 10 `Definition of Done`.
- Leave the feature open if any item still requires a `Target expected result` caveat instead of a fully demonstrated web outcome.
Current expected result: CP-29 DoD is complete at code/test level; end-to-end provider smoke testing can still be run as release validation.
