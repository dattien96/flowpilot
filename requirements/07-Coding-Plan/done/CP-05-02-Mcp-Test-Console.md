# CP-05-02: MCP Test Console On `/mcp-servers/mcp-connect-test`

**Maps from:** SS-02, CP-05, CP-05-01, CP-11, SD-11
**Phase:** MCP usability follow-up after real type and instance management exists
**Depends on:** CP-05, CP-05-01, CP-11

---

## 1. Goal

Add a real UI test surface on `/mcp-servers/mcp-connect-test` so users can verify that:

- an MCP type is enabled and usable on this machine
- a specific MCP instance is not only configured, but actually usable for real tasks

This phase is not finished when we only have:

- MCP type install or enable state
- an instance `connected` badge
- a verify button that only checks launcher or credential reachability

This phase is finished only when the user can:

- select a real connected MCP instance from the UI
- choose or type a test task
- send that task through the local runner
- receive a structured success or failure result in the UI
- understand whether the failure is type readiness, backend install, auth, permissions, bad prompt, or missing instance configuration

For Jira and the Atlassian remote MCP family, the UI should support example tasks such as:

- finding open bugs in a Jira project
- creating or updating a Jira issue
- summarizing a Confluence page
- listing accessible spaces
- cross-linking Jira and Confluence content

---

## 2. Why This CP Exists

After CP-05, `/mcp-servers` should already tell us:

- whether the runner is online
- whether an allowlisted MCP type is enabled or installed
- whether real MCP instances were created
- whether a specific MCP instance is connected

That is necessary, but not sufficient.

The product still needs to answer the most important operator question:

- `Can this MCP instance actually do useful work right now from this machine?`

The existing `Verify` action is too narrow:

- for a remote type like Jira, it may validate machine-level capability readiness
- for a local type, it may validate that the launcher or backend can start
- for an MCP instance, it may validate stored credentials and target-resource access

It does not prove that:

- the instance can answer task-shaped requests
- the configured instance is sufficient for the chosen action
- the returned content is understandable by the user

This CP closes that gap by adding an explicit MCP test console on a dedicated MCP test route linked from the global MCP page.

---

## 3. Scope

This phase adds:

- a dedicated MCP test route at `/mcp-servers/mcp-connect-test`
- runner endpoints for MCP test execution
- structured test result rendering
- provider-aware example prompts
- audit-friendly local artifacts for test runs

This phase does not require:

- full workflow execution through MCP
- automatic prompt injection into workflow steps
- generalized agent orchestration
- support for every provider on day one

Recommended MVP scope:

- ship the route shell for 5 MCP type sections:
  - Driver
  - Jira
  - Telegram
  - Figma
  - Firebase
- implement the real task execution flow for Jira first
- keep the route and runner contract provider-agnostic so the other sections can be activated later without redesigning the page

---

## 4. User Experience

### 4.1 Entry Point

On `/mcp-servers`, keep a single button:

- `Open Test Console`

That button should navigate to:

- `/mcp-servers/mcp-connect-test`

The test console must no longer be embedded inline on the global MCP page.

### 4.2 Route Layout

The dedicated route must render 5 top-level MCP type sections:

- `Driver`
- `Jira`
- `Telegram`
- `Figma`
- `Firebase`

Rules:

- each MCP type section is collapsible
- collapsed state should still show the MCP type name and high-level readiness
- expanded state should show the list of supported test actions for that MCP type
- only Jira needs to execute real tests in this phase
- the other sections may render `planned / not implemented yet` states as long as the shell is present

### 4.3 Jira Action Groups

Inside the Jira section, the UI should render action-level child sections.

Initial Jira actions:

- `Get a list of bugs`
- `Get a list of user stories`
- `Get the content of 1 ticket`

Rules:

- each action group is collapsible
- expanding an action group should show the exact API-style test payload or task prompt that will be sent
- the user should be able to inspect the action before executing it
- the user should be able to trigger the test from inside that expanded child section
- later write actions can be added, but this MVP should stay read-oriented

### 4.4 Required Inputs

The test panel should require:

- `MCP Type`
- `MCP Instance`
- `Task template` or `Custom prompt`

Optional input:

- `Project`

Rules:

- only show instances whose status is `connected`
- only show instances for the selected type
- disable submit when the runner is offline
- disable submit when no connected instance exists
- only ask for `Project` when the selected template or provider action needs project context beyond the instance itself

### 4.5 Task Templates

The UI should offer curated action cards, not only a blank textarea.

For the Jira MVP, the initial action set should be:

- `Get a list of bugs`
- `Get a list of user stories`
- `Get the content of 1 ticket`

Each action should define:

- a stable `templateKey`
- a default task prompt
- whether extra user input is required

Example mappings:

- `jira_list_bugs`
- `jira_list_user_stories`
- `jira_get_ticket_content`

The user may still edit the task prompt before sending it.

### 4.6 Result Surface

The UI must show:

- status: `success` or `failed`
- MCP type and MCP instance used
- optional project link used, if any
- executed test prompt
- summarized output
- raw output panel
- execution duration
- actionable error message

Recommended tabs or sections:

- `Summary`
- `Raw Result`
- `Debug`

The debug view may include:

- MCP type key
- MCP instance id
- optional project id
- runner command or transport mode
- timestamp

The debug view must not include secrets.

---

## 5. Runner Contract

The local runner needs a new MCP test execution contract.

### 5.1 New Request Type

Suggested request payload:

```json
{
  "mcpType": "jira",
  "mcpInstanceId": "instance-jira-alpha",
  "projectId": "project-alpha",
  "task": "Get a list of bug tickets for project SCRUM.",
  "templateKey": "jira_list_bugs",
  "timeoutMs": 60000
}
```

Notes:

- `projectId` should be optional
- the primary execution target is the MCP instance
- project context should be supplied only when the template needs linked project state
- if `projectId` is supplied, the runner must verify that the project is currently linked to `mcpInstanceId` before executing the task

### 5.2 Suggested Endpoint

Preferred endpoint:

```text
POST /mcp-test/execute
```

Alternative acceptable endpoint:

```text
POST /mcp-instances/{mcpInstanceId}/test
```

The important part is semantic clarity:

- this is not type installation
- this is not type enablement
- this is not connection creation
- this is a real MCP task execution probe against a selected instance

### 5.3 Result Type

Suggested response:

```json
{
  "status": "success",
  "mcpType": "jira",
  "mcpInstanceId": "instance-jira-alpha",
  "projectId": "project-alpha",
  "templateKey": "jira_find_open_bugs",
  "task": "Find all open bugs in Project Alpha.",
  "summary": "Returned 7 open bug issues from project FLOW.",
  "rawOutput": "...",
  "artifactPaths": [
    ".flowpilot/runs/mcp-test-123/result.json"
  ],
  "startedAt": "2026-05-19T10:00:00.000Z",
  "completedAt": "2026-05-19T10:00:03.000Z",
  "durationMs": 3000,
  "errorMessage": null
}
```

Failure result:

- `status = failed`
- `summary` should be short and readable
- `errorMessage` should explain the real failure mode

---

## 6. Execution Model

There are 2 valid implementation models.

### 6.1 Preferred MVP: Provider-Aware Native Runner Calls

For Jira-first delivery, the runner may use the stored Jira credentials and call provider APIs directly through a provider adapter.

Pros:

- simpler than standing up a generic MCP client runtime immediately
- easier to control response shape
- easier to debug and test

Cons:

- not a pure MCP transport exercise
- later provider-specific logic may drift

This is acceptable for MVP if the UI truthfully says it is testing the selected MCP instance, not benchmarking a generic MCP protocol engine.

### 6.2 Longer-Term: Generic MCP Client Session

The runner may open a real MCP client session against the configured backend and execute tool or resource operations through a generalized MCP runtime.

Pros:

- closest to real MCP behavior
- reusable across Jira, Confluence, Compass, Drive, and future providers

Cons:

- larger implementation cost
- requires tool and resource routing plus richer response normalization

This aligns well with `CP-13-Direct-MCP-Runtime.md`, but should not block the initial Jira-focused test console.

### 6.3 Decision

Recommended path:

1. ship Jira-first MCP test console using provider-aware runner logic
2. keep request and response contracts generic
3. migrate internals toward a true direct MCP runtime later if needed

---

## 7. Jira / Atlassian MVP Semantics

For Jira-backed tests, the runner should support a small allowlist of safe task modes first.

Suggested internal template keys:

- `jira_find_open_bugs`
- `jira_create_story`
- `confluence_summarize_page`
- `confluence_list_spaces`
- `atlassian_link_content`

The runner may initially interpret these templates into provider-specific API calls rather than free-form agentic reasoning.

Example:

- `jira_find_open_bugs`
  - build a JQL query using the instance `projectKey`
  - fetch issues from Jira
  - return issue key, title, status, assignee

- `jira_create_story`
  - create an issue in the configured Jira project from the selected instance
  - return created issue key and URL

- `confluence_summarize_page`
  - fetch page content using the same Atlassian credential
  - return a concise summary plus page title

This approach gives the UI a real useful test surface while keeping scope bounded.

---

## 8. Frontend Scope

### 8.1 `/mcp-servers` Panel

Add a dedicated component under the global MCP page that:

- loads MCP types
- loads connected MCP instances
- filters by selected type
- offers template buttons
- offers custom text input
- submits to the runner
- renders the latest test result inline

### 8.2 State Model

Local UI state should track:

- selected MCP type
- selected MCP instance id
- optional selected project id
- selected template key
- editable prompt text
- pending state
- latest execution result

### 8.3 UX Guardrails

The UI should block or warn when:

- runner is offline
- MCP type is disabled or missing
- MCP instance is not connected
- prompt is empty
- selected template needs config that is missing

### 8.4 Copy

Recommended helper copy:

- `Run a real task against the selected MCP instance to verify it is usable.`
- `This test uses the local runner and the stored provider credential on this machine.`
- `Do not paste secrets into the prompt.`

---

## 9. Backend / Runner Scope

### 9.1 New Runner Types

Add:

- `McpTestExecutionRequest`
- `McpTestExecutionResult`

### 9.2 New Runner Method

Suggested method:

```go
ExecuteMcpTest(ctx context.Context, request McpTestExecutionRequest) (McpTestExecutionResult, error)
```

### 9.3 Artifacts

Test runs should be persisted locally under `.flowpilot` so the UI can debug failures later.

Suggested artifacts:

- request payload
- normalized provider action
- raw provider response
- summarized result
- error output

### 9.4 Error Classes

The runner should normalize errors into clear user-facing categories:

- MCP type missing
- MCP type disabled
- MCP instance not connected
- project link mismatch
- credential missing
- permission denied
- provider resource not found
- invalid instance config
- timeout
- unsupported template

---

## 10. Implementation Plan

### 10.1 Phase 1: Contract and UI shell

Deliverables:

- add MCP test panel to `/mcp-servers`
- add request and result types in admin-web and runner
- add submit mutation and loading and error states

Acceptance:

- user can choose MCP type, MCP instance, and a template
- UI can submit to runner and render mocked or stubbed result shape

### 10.2 Phase 2: Jira-first runner implementation

Deliverables:

- implement Jira test execution in the local runner
- support at least one read test and one write test
- persist local run artifacts

Acceptance:

- user can run `find open bugs`
- user can run `create story`
- success and failure are clearly visible

### 10.3 Phase 3: Confluence support on same Atlassian credential

Deliverables:

- add Confluence read test templates
- summarize page content
- list accessible spaces

Acceptance:

- a connected Atlassian MCP instance can also exercise Confluence-capable tests when permissions allow

### 10.4 Phase 4: Result hardening

Deliverables:

- raw result viewer
- structured summary
- timing and artifact links
- normalized error handling

Acceptance:

- operators can distinguish type failure, instance failure, and provider failure

### 10.5 Phase 5: Provider expansion

Deliverables:

- reusable template registry
- pluggable provider-specific execution layer
- optional Google Drive read test as second provider

Acceptance:

- the UI shell supports multiple providers without redesign

---

## 11. Risks And Non-Goals

Primary risks:

- trying to build a full generic MCP runtime too early
- mixing free-form AI prompt execution with deterministic provider probes
- allowing destructive write templates without enough guardrails
- accidentally conflating type verification with instance task execution

Recommended write-safety rules:

- start with read-only templates by default
- gate write templates behind explicit labels such as `Creates data`
- show confirmation before create or update operations

Non-goals:

- full workflow-step integration in this CP
- background automation around MCP tests
- unrestricted natural-language execution against any provider action

---

## 12. Definition Of Done

- [ ] `/mcp-servers` includes an MCP test panel
- [ ] user can select a connected MCP instance to test
- [ ] user can run curated Jira and Atlassian example tasks from the UI
- [ ] local runner exposes a real MCP test execution endpoint
- [ ] runner returns structured success and failure results
- [ ] UI shows summary, raw result, timing, and actionable errors
- [ ] test execution writes local artifacts for debugging
- [ ] secrets are never shown in UI results or stored in plain text artifacts
- [ ] at least one read test and one write test work end to end for Jira-first delivery
- [ ] the console clearly separates MCP type readiness from MCP instance task execution
