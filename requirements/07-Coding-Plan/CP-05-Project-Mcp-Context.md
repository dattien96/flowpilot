# CP-05: Project MCP Context

**Maps from:** SS-01, SS-02, SD-04, SD-05, SD-10
**Phase:** After CP-04 and CP-03, once the UUID baseline and project settings shell exist, and before workflow features depend on MCP-backed context
**Depends on:** CP-04, CP-03

---

## 1. Goal

Align the implementation with the updated MCP ownership model while preserving the runtime guarantees expected by workflow execution.

This phase is complete only when:

- MCP types are managed as global allowlisted capabilities
- each type is either `remote` or `local`
- each type has one machine-level enable or disable surface
- remote enabled types can create multiple reusable MCP instances
- local enabled types keep install-oriented UX where appropriate
- projects only link and unlink existing MCP instances
- the runner can still validate and load the linked MCP context at execution time

---

## 2. Final Mental Model

### 2.1 MCP type

Examples:

- `jira`
- `tele`
- `driver`

Each type:

- belongs to `remote` or `local`
- appears in the allowlisted inventory
- has one enable or disable UI
- may also expose install and verify depending on backend behavior

### 2.2 MCP instance

An MCP instance is a reusable configured connection created from an enabled type.

Rules:

- remote types such as `jira` can create multiple instances across multiple Jira projects
- local types such as `driver` keep the current install-first experience with command guidance and `INSTALL`
- instances are managed globally
- provider-specific configuration lives on the instance
- credential secrets stay owned by the runner secure store where possible

### 2.3 Project link

Project settings only select one existing instance from the global list for each supported type for MVP.

Rules:

- no create form in project settings
- no edit form in project settings
- no provider auth lifecycle in project settings
- `Create new MCP` navigates to the global MCP page
- project settings are assignment only, not provider setup

---

## 3. Current State vs Remaining Gap

Already aligned in code:

- much of MCP ownership has moved to the global MCP page
- project settings already behave largely as link and unlink only

Remaining gaps to close:

- specs still describe older project-centric ownership in some places
- MCP general page still mixes inventory, instance CRUD, runner reachability, and test console in one route in older descriptions
- system-wide runner settings do not belong on the same page as instance CRUD
- project creation should live on `/projects/create` instead of being embedded on `/projects`
- project settings should show only the valid MCP action for the current linked state
- the runner and schema documentation must reflect the latest type, instance, and project-link split

---

## 4. Target Routes

### 4.1 MCP routes

- `/settings/mcp-servers`
  - only two sections:
    - combined Runner Reachability + Allowlisted MCP inventory
    - MCP instances list
- `/settings/mcp-servers/create`
  - dedicated MCP instance creation form
- `/settings/mcp-servers/mcp-connect-test`
  - dedicated MCP Test Console

### 4.2 Runner route

- `/settings/runner`
  - system-wide runner reachability and runner-specific settings

### 4.3 Project routes

- `/projects`
  - list page with a single `ADD project` button
- `/projects/create`
  - create-project form
- `/projects/$projectId/settings`
  - artifact storage plus MCP link and unlink only

---

## 5. Frontend Plan

### 5.1 Refactor `/settings/mcp-servers`

Keep only two sections.

#### Section A: Combined Runner Reachability + Allowlisted MCP inventory

Show:

- runner summary relevant to MCP availability
- allowlisted MCP backends
- category badge: `remote` or `local`
- enable or disable actions
- install or launcher state where relevant
- verify action where supported

Behavior:

- remote types:
  - enabling is machine-level only
  - type cards must not imply they are project-bound integrations
  - enabled state is a prerequisite for creating remote MCP instances
- local types:
  - keep current `npx` command hint and `INSTALL` flow if that backend model is still valid

#### Section B: MCP instances list

Show:

- existing reusable MCP instances
- create, edit, verify, remove actions
- provider-specific summary and status

Required cleanup:

- remove embedded MCP Test Console from this page
- remove any owner-project framing that implies the instance belongs to a single project

### 5.2 Add `/settings/mcp-servers/create`

Move MCP instance creation into a dedicated route.

Rules:

- provider dropdown should only show enabled MCP types
- remote instance creation should be blocked until the type is enabled
- provider-specific setup stays in this route, not in project settings

### 5.3 Add `/settings/mcp-servers/mcp-connect-test`

Move current MCP Test Console UI and loader logic into a dedicated route.

Responsibilities:

- backend selection
- connected-instance selection
- prompt or template selection
- test execution
- run history and artifacts

### 5.4 Add `/settings/runner`

Introduce a new settings menu entry:

- `RUNNER`

Move system-wide runner reachability and runner-only controls here.

The MCP page should retain only the runner summary needed for explaining backend availability inside Section A.

### 5.5 Refactor `/projects`

Change `/projects` to a list-first page.

Required behavior:

- one `ADD project` button
- remove in-page create form
- navigate to `/projects/create`

### 5.6 Keep `/projects/$projectId/settings` link-only

Required behavior:

- show currently linked MCP per type
- choose from global MCP list
- when no MCP is linked for that type, show only `Link MCP`
- when an MCP is already linked for that type, hide `Link MCP` and show only `Unlink`
- keep `Create new MCP` as navigation to `/settings/mcp-servers`

Must not add back:

- project-owned MCP create
- project-owned MCP edit
- project-owned provider auth

---

## 6. Data Model and Persistence

### 6.1 MCP type contract

The runner and admin-web need a shared understanding of type metadata:

```ts
type McpTypeDefinition = {
  key: "driver" | "jira" | "tele"
  category: "remote" | "local"
  supportsEnable: boolean
  supportsInstall: boolean
  supportsVerify: boolean
}
```

### 6.2 Final target: global MCP instances

The final persistence model should treat MCP instances as global reusable records.

Suggested shape:

```sql
CREATE TABLE mcp_instances (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  type TEXT NOT NULL,
  label TEXT NOT NULL,
  config_encrypted JSONB NOT NULL DEFAULT '{}'::jsonb,
  status TEXT NOT NULL DEFAULT 'pending'
    CHECK (status IN ('pending', 'awaiting_oauth', 'connected', 'failed')),
  last_verified_at TIMESTAMPTZ,
  last_error TEXT,
  created_at TIMESTAMPTZ DEFAULT now(),
  updated_at TIMESTAMPTZ DEFAULT now()
);
```

Rules:

- instance config stores stable provider settings
- `config_encrypted` must never be written in plaintext
- instance status supports:
  - `pending`
  - `awaiting_oauth`
  - `connected`
  - `failed`
- remote types may have many instances
- local types may still have zero or more rows depending on backend behavior

### 6.3 Final target: project link table

Project assignment should be separated from instance lifecycle.

Suggested shape:

```sql
CREATE TABLE project_mcp_links (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  mcp_instance_id UUID NOT NULL REFERENCES mcp_instances(id) ON DELETE CASCADE,
  created_at TIMESTAMPTZ DEFAULT now(),
  updated_at TIMESTAMPTZ DEFAULT now()
);
```

Rules:

- linking is a separate operation from instance creation
- for MVP, project settings should allow only one linked MCP per supported type
- project-link type should be server-derived from the instance, not client-authored

### 6.4 Current rollout bridge

The current rollout still uses `integrations` naming and persistence in several places. CP-05 owns the migration path that bridges from old naming to the new MCP model.

Current rollout requirements:

- keep integration-backed persistence where already implemented
- support machine-level enablement for remote MCP types
- support project-to-MCP linking separately from provider configuration

Required rollout step:

```sql
ALTER TABLE integrations
ADD COLUMN mcp_type_enabled BOOLEAN NOT NULL DEFAULT false;
```

Usage:

- remote MCP types such as Jira cannot create instances until their type is enabled
- after enable succeeds, users may create multiple Jira MCP instances across multiple Jira projects
- the enablement field is a staged rollout bridge and must not be confused with project linkage

---

## 7. Runner and Runtime Plan

### 7.1 Type-level behavior

- remote type:
  - can be enabled and disabled
  - may create many instances after enable succeeds
- local type:
  - may require install first
  - keeps command-driven install guidance
  - may not need the same multi-instance UX as remote types

### 7.2 Instance lifecycle behavior

- instance creation is global
- provider-specific configuration lives on the instance
- project assignment is a separate link operation
- verify and test actions operate on the instance, not on project settings

### 7.3 Runner installation and auth flow

The runner remains the installer and configurator for MCP backends. The frontend should create or update records, then the runner owns verification and provider-specific connection work.

Expected orchestration:

1. User enables a type at the machine level when required.
2. User creates or edits an MCP instance from the global MCP page.
3. Runner checks whether the backend is installed or reachable for that type.
4. If auth is required, runner starts the provider auth or token validation flow.
5. Runner updates instance status:
   - `pending`
   - `awaiting_oauth`
   - `connected`
   - `failed`
6. Runner writes `last_error` and last verification data after each attempt.

### 7.4 Runtime prompt assembly

At the start of each workflow step execution, the runner must validate and load linked MCP context before prompt assembly.

Runtime logic:

1. Load the step definition and its required MCP types.
2. Resolve the project’s linked MCP instances.
3. Verify that each required MCP is linked and `connected`.
4. If any required MCP is missing or not connected:
   - fail the workflow step
   - surface a named error such as `Missing required MCP: <type>`
5. If all required MCPs are present:
   - inject the relevant MCP context payload into the prompt assembly path
   - proceed with execution

---

## 8. Data and Copy Adjustments

Required alignment:

- replace project-scoped MCP wording with global-instance wording
- ensure labels distinguish:
  - type inventory
  - instance list
  - project links
- ensure route names and menu labels match the final structure

Copy rules:

- use `available MCP backends` or `allowlisted MCP inventory` for type-level list
- use `MCP instances` for reusable configured connections
- use `Link MCP` and `Unlink` only for project assignment

---

## 9. Implementation Sequence

1. Update route structure first.
2. Extract MCP Test Console into `/settings/mcp-servers/mcp-connect-test`.
3. Add `/settings/runner` and move runner-only settings there.
4. Simplify `/settings/mcp-servers` to the two required sections.
5. Move instance creation to `/settings/mcp-servers/create`.
6. Split project creation from `/projects` into `/projects/create`.
7. Update project settings so it shows only the valid MCP action for each type.
8. Add the integration-table migration for remote MCP type enablement.
9. Keep rollout persistence aligned while moving toward explicit instance and project-link ownership.
10. Update tests for the new routes, navigation targets, and runner behavior.

---

## 10. Acceptance Criteria

- `/settings/mcp-servers` has exactly two user-facing sections:
  - combined Runner Reachability + Allowlisted MCP inventory
  - MCP instances list
- MCP creation lives on `/settings/mcp-servers/create`
- MCP Test Console is reachable only from `/settings/mcp-servers/mcp-connect-test`
- a `RUNNER` settings page exists and owns system-wide runner settings
- `/projects` uses one `ADD project` action and no embedded create form
- `/projects/create` owns project creation
- project settings only link and unlink MCPs from the global list
- when linked, only `Unlink` is visible for that type
- when not linked, only `Link MCP` is visible for that type
- remote MCP type enablement is persisted with the required integration-table field for this rollout
- instance status supports:
  - `pending`
  - `awaiting_oauth`
  - `connected`
  - `failed`
- instance config is encrypted before persistence and not exposed as plaintext in the UI
- runner verifies linked MCP requirements before workflow prompt assembly
- spec language no longer describes project settings as the MCP creation or edit surface
