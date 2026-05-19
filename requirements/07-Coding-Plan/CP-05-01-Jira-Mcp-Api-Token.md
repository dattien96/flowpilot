# CP-05-01: Jira MCP API Token Flow

**Maps from:** SS-01, SS-02, SD-04, SD-11  
**Phase:** Jira-specific follow-up to CP-05  
**Depends on:** CP-05, CP-11

---

## 1. Goal

Implement the Jira MCP onboarding flow using Atlassian's documented API token path.

This plan is Jira-only. It does not define Google Drive, Telegram, Figma, or Firebase behavior.

This phase is not finished when we only have:
- a Jira provider dropdown
- a form with Jira fields
- a runner request that accepts the data

This phase is finished only when the user can:
- open the official Atlassian guide from the app
- generate an Atlassian API token
- paste the token back into the Jira MCP form
- create the Jira MCP without storing the token in ordinary project config
- have the runner use the token for Jira API calls
- be blocked from creating a duplicate Jira MCP for the same Atlassian site URL

Official Atlassian guide:
- [Getting started with the Atlassian Remote MCP Server](https://support.atlassian.com/atlassian-rovo-mcp-server/docs/getting-started-with-the-atlassian-remote-mcp-server/)

Token generation link shown to the user:
- [Atlassian API tokens](https://id.atlassian.com/manage-profile/security/api-tokens)

---

## 2. Current Gap To Close

Jira currently lacks a safe non-interactive onboarding path for the MVP.

The missing pieces are:
- the UI does not yet force the user through the Atlassian token guide
- the UI does not yet collect Jira token material explicitly
- the runner does not yet receive Jira API token credentials as a secure secret
- duplicate Jira MCP entries for the same Atlassian site are not yet rejected at form time

The implementation must therefore close these gaps:
- no silent Jira connect attempt without an explicit token step
- no raw token persistence in `configEncrypted`
- no duplicate Jira MCP entry for the same normalized site URL within the same project
- no false `connected` state when the token or site URL is invalid

---

## 3. User Flow

### 3.1 Add Jira MCP

When the user selects `Jira` in `Add MCP`:
1. the app shows the Jira-specific onboarding guide
2. the app shows a clickable link to Atlassian's token page
3. the app asks the user to generate an API token and paste it back into the form
4. the user enters the Atlassian site URL, Jira project key, optional board id, email, and API token
5. the app validates the input locally
6. the runner stores the secret securely
7. the runner uses the token to authenticate against Jira
8. the integration becomes `connected` only after verification succeeds

### 3.2 Duplicate Prevention

Before the form can submit, the app must check whether the current project already has a Jira MCP for the same Atlassian site.

Rules:
- normalize the entered site URL to its origin, for example `https://flowpilot899.atlassian.net`
- ignore path segments such as `/jira/software/projects/SCRUM/boards/1`
- treat trailing slashes as equivalent
- compare the normalized site URL against existing Jira integrations for the same project
- reject the create action if a match already exists

Suggested duplicate message:
- `A Jira MCP for this Atlassian site already exists in this project.`

### 3.3 Retry / Edit

If the integration already exists, the user may:
- edit the label
- edit the Jira project key
- rotate the token
- retry the connection

The app must still reject a second Jira MCP for the same site URL in the same project.

---

## 4. Jira Form Fields

Do not use a raw JSON textarea as the primary Jira UX.

Required fields:
- `workspaceUrl`
- `projectKey`
- `email`
- `apiToken`

Optional fields:
- `boardId`

Validation rules:
- `workspaceUrl` must be the Atlassian site root, not a full board URL
- `projectKey` identifies the Jira project, for example `SCRUM`
- `boardId` is optional and is only used when a workflow needs board-level precision
- `email` must be the Atlassian account email that owns the token
- `apiToken` must be entered by the user and never displayed after save

UI guidance:
- show a short explanation that Atlassian uses API token auth for this flow
- show the Atlassian token page link next to the `apiToken` field
- tell the user to paste the token only after generating it from Atlassian

---

## 5. Security Boundary

The Jira API token is sensitive and must not live in normal project config.

Allowed storage:
- runner-managed OS keychain / credential store
- encrypted local secret storage owned by the Go runner

Not allowed:
- Supabase `configEncrypted`
- browser local storage
- query string
- logs
- plain text project records

Recommended split:
- Supabase stores project-scoped Jira metadata and connection state
- the local runner stores the token securely and retrieves it when performing Jira calls
- the UI only ever handles the token long enough to send it to the runner over `localhost`

---

## 6. Runner Responsibilities

The runner must:
1. receive Jira MCP create or retry requests from admin-web
2. validate the Atlassian site URL
3. validate that the Jira MCP does not already exist for that site in the same project
4. store the email and token securely
5. build the correct Jira authentication header at request time
6. call Jira or Atlassian-backed APIs to verify access
7. update Supabase with the final status and last error

Suggested auth header behavior:
- use the API-token auth mode Atlassian documents for headless or token-based scenarios
- keep the exact header construction inside the runner secret boundary

The runner must never echo the raw token back to the UI.

---

## 7. Data Model

Use the existing project-scoped `integrations` row, but with Jira-specific config semantics.

### 7.1 Jira config payload

```json
{
  "workspaceUrl": "https://flowpilot899.atlassian.net",
  "projectKey": "SCRUM",
  "boardId": "1",
  "email": "user@example.com"
}
```

### 7.2 Secret payload

The API token must be stored separately from the integration config.

Suggested secret fields:
- `email`
- `apiToken`

Suggested secret reference:
- `credentialRef`

### 7.3 Uniqueness rule

For the same project:
- one Jira integration per normalized Atlassian site URL
- label differences do not create a second Jira MCP
- different `projectKey` values still count as duplicates if the site URL is the same and the rule is site-scoped

If we later need multiple Jira integrations per site, that should be a separate CP decision, not an accidental default.

---

## 8. Frontend Scope

### 8.1 Project Settings

Project settings should:
- keep Jira as a provider option
- show the Atlassian guide link before submission
- collect email and API token explicitly
- block duplicate site URLs before create
- warn when the local runner is offline

### 8.2 Global MCP Servers Page

The global `MCP Servers` page should:
- expose the Jira backend card
- let the user create a Jira MCP directly from the menu page
- prefill the Jira form when the user starts from the Jira card
- use the same duplicate URL check before submission

### 8.3 Inline Copy

Recommended copy for Jira:
- `To connect Jira MCP, generate an Atlassian API token and paste it here.`
- `Use the Atlassian site root, not the full board URL.`
- `A Jira MCP for this site already exists in this project.`

---

## 9. Implementation Plan

### 9.1 Phase 1: Jira form and guide

Deliverables:
- show the Atlassian official guide link
- show the API token generation link
- collect `workspaceUrl`, `projectKey`, optional `boardId`, `email`, and `apiToken`
- normalize `workspaceUrl` to its site root

Acceptance:
- user can understand where to generate the token
- user can paste the token into the Jira form
- the UI does not require a full board URL

### 9.2 Phase 2: Duplicate detection

Deliverables:
- load existing Jira integrations for the project
- compare normalized Jira site URLs
- block create when a duplicate is found
- show a clear validation message

Acceptance:
- the same Jira site cannot be added twice within the same project
- duplicate detection runs before the runner call

### 9.3 Phase 3: Runner secret storage

Deliverables:
- store Jira API tokens in runner-managed secure storage
- keep only non-sensitive references in the project record
- delete the secret when the integration is removed

Acceptance:
- raw token is never persisted in normal config
- token survives runner restarts through the secure store

### 9.4 Phase 4: Jira verification

Deliverables:
- build Jira auth headers from the stored secret
- verify access against the configured Jira site and project
- update status to `connected` only after verification succeeds

Acceptance:
- invalid token fails cleanly
- invalid site URL fails cleanly
- success updates the project integration status

### 9.5 Phase 5: UI cleanup

Deliverables:
- project settings and global MCP Servers page share the same Jira semantics
- duplicate site error is visible in both places
- offline runner messaging is explicit

Acceptance:
- the user knows whether the failure is duplicate site, invalid token, or runner offline

---

## 10. Risks And Non-Goals

Primary risks:
- leaking token material into browser logs or database payloads
- accepting full board URLs without normalization
- allowing duplicate Jira integrations for the same Atlassian site

Non-goals:
- OAuth flow for Jira in this CP
- Google Drive, Telegram, Figma, or Firebase changes
- general secret rotation tooling

---

## 11. Definition Of Done

- [ ] Jira MCP form shows the official Atlassian guide link
- [ ] Jira MCP form collects `workspaceUrl`, `projectKey`, optional `boardId`, `email`, and `apiToken`
- [ ] pasted token is sent only to the local runner
- [ ] token is stored in runner-managed secure storage
- [ ] token is not written to `configEncrypted`
- [ ] duplicate Jira site URLs are rejected within the same project
- [ ] runner verifies Jira access before marking `connected`
- [ ] removing the integration removes the stored secret
- [ ] global MCP Servers page and project settings both follow the same Jira rules
