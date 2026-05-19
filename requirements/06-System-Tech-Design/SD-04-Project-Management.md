# FlowPilot Tech Design - Project Management

This document updates the project-management architecture to align with the current global MCP ownership model and the revised routing structure for MCP management, runner settings, and project creation.

## 1. Architecture Intent

Project management now separates three concerns that were previously mixed together:

- global MCP type management
- reusable MCP instance management
- project-to-MCP linking

The admin-web must no longer treat project settings as the place where MCP provider connections are created or edited. Project settings only decide which existing MCP instance is linked to a project.

---

## 2. Core Domain Model

### 2.1 MCP type

An MCP type is an allowlisted capability family defined by the product and verified by the runner.

Examples:

- `jira`
- `tele`
- `driver`

Every MCP type belongs to exactly one category:

- `remote`
- `local`

Examples by category:

- remote:
  - `jira`
  - `tele`
- local:
  - `driver`

Each MCP type exposes one machine-level enable/disable control in the UI.

Type-level state is global to the current machine and runner, not to any project.

### 2.2 MCP instance

An MCP instance is a reusable configured connection created from an enabled MCP type.

Examples:

- Jira instance for workspace A and project `OPS`
- Jira instance for workspace B and project `APP`
- another Jira instance for the same workspace but a different Jira project

Rules:

- remote enabled types may create multiple MCP instances
- local enabled types may still use install-oriented UX and may not require multi-instance creation in the same way
- each instance owns provider-specific configuration and connection status
- instance lifecycle lives on the global MCP management page

### 2.3 Project link

A project links to existing MCP instances from the global list.

Rules:

- a project must not create or edit provider connections inside project settings
- a project may link or unlink one MCP per supported type for MVP
- `Create new MCP` from project settings navigates to the global MCP page
- when a type has no linked MCP, show `Link MCP` and hide `Unlink`
- when a type already has a linked MCP, hide `Link MCP` and show `Unlink`

---

## 3. UI/UX Architecture

### 3.1 Global MCP page

Route:

- `/settings/mcp-servers`

This page must contain only two sections.

#### Section A: Runner Reachability + Allowlisted MCP inventory

This is a combined section that shows:

- runner reachability summary relevant to MCP management
- available MCP backends returned by the allowlist and runner inventory
- category for each type: `remote` or `local`
- enabled or disabled state per type
- install state or launcher state when relevant
- enable and disable control per type
- verify action when supported

Behavior by category:

- remote types:
  - expose one machine-level enable or disable control
  - do not pretend the type card is itself a project connection
  - once enabled, they allow creation of multiple MCP instances
- local types:
  - keep install-oriented UX when that is the correct backend model
  - continue to show command guidance such as `npx ...`
  - continue to expose `INSTALL` when missing

#### Section B: MCP instances list

This section lists the real reusable MCP instances and is the only place for instance lifecycle management.

Required contents:

- label
- type
- provider summary
- status
- last verified or synced information
- last error
- actions such as create, edit, verify, remove

Important rule:

- the general page must not embed MCP Test Console once the dedicated route exists

### 3.2 MCP Test Console

Route:

- `/settings/mcp-servers/mcp-connect-test`

The MCP Test Console is a dedicated page, not a section on the general MCP page.

Responsibilities:

- select an MCP backend
- select a connected MCP instance
- run smoke-test flows through the runner
- show recent runs and artifacts

Boundary:

- this route is diagnostic and operational
- it must not be mixed into the main MCP inventory and instance-management page

### 3.3 Runner settings page

Add a new settings menu page:

- `RUNNER`

This page becomes the home for system-wide runner reachability and runner-only settings.

Responsibilities:

- show system-wide runner status
- host runner-specific diagnostics or configuration that is not MCP-type-specific

Boundary:

- runner-wide settings move here
- the MCP general page retains only the runner reachability summary needed to explain MCP availability in Section A

### 3.4 Projects list

Route:

- `/projects`

Required behavior:

- show a single `ADD project` button
- remove embedded create-project form from the index page
- navigate project creation to a dedicated route

New route:

- `/projects/create`

This route owns the create-project form and related create flow.

### 3.5 Project settings

Route:

- `/projects/$projectId/settings`

Project settings must support only project-to-global-MCP linking behavior.

Allowed actions:

- view currently linked MCP per type
- choose an existing MCP from the global list
- link
- unlink
- navigate to `Create new MCP`

Disallowed actions:

- create MCP inside project settings
- edit MCP inside project settings
- run provider auth lifecycle from project settings
- manage MCP type enable or install state from project settings

---

## 4. Data Model

### 4.1 MCP type registry

MCP types should be represented as an allowlisted registry, likely code-defined and runner-aware.

Suggested shape:

```ts
type McpType = {
  key: "driver" | "jira" | "tele"
  label: string
  category: "remote" | "local"
  supportsEnable: boolean
  supportsInstall: boolean
  supportsVerify: boolean
}
```

### 4.2 MCP instances

MCP instances remain global reusable records.

Suggested table:

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
- credential material remains owned by the runner secure store
- remote types may have many rows
- local types may still have zero or more rows depending on the backend model

### 4.2a Current integration-table rollout note

The codebase still uses `integration` naming in several places during rollout.

To support machine-level enablement for remote types before full persistence normalization, the current integration storage must add a new field for remote MCP type enablement state.

Suggested staged field:

```sql
ALTER TABLE integrations
ADD COLUMN mcp_type_enabled BOOLEAN NOT NULL DEFAULT false;
```

Intent:

- this field records whether a remote MCP type such as Jira has been enabled on the current machine
- remote MCP instance creation must remain blocked until this flag is enabled
- once enabled, users may create multiple Jira MCP instances across multiple Jira projects

Migration note:

- this is a staged compatibility step for the current rollout
- longer-term type-state persistence may move to a dedicated machine-level registry, but the rollout plan must include this integration-table column now

### 4.3 Project MCP links

Projects reference global instances through a link table.

Suggested table:

```sql
CREATE TABLE project_mcp_links (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  mcp_instance_id UUID NOT NULL REFERENCES mcp_instances(id) ON DELETE CASCADE,
  type TEXT NOT NULL,
  created_at TIMESTAMPTZ DEFAULT now(),
  updated_at TIMESTAMPTZ DEFAULT now(),
  UNIQUE (project_id, type)
);
```

Rules:

- type is derived from the linked MCP instance
- a project links at most one instance per type for MVP
- unlinking removes only the link, not the instance

---

## 5. Runner Responsibilities

The runner owns truthful machine-level and instance-level state.

### 5.1 Type lifecycle

For every allowlisted type, the runner must support the appropriate combination of:

- detect
- enable
- disable
- install
- verify

Type lifecycle is machine-scoped.

### 5.2 Instance lifecycle

For instance-capable types, the runner must support:

- create or update connection
- auth or install handoff as required
- verification against the configured remote or local target
- status updates back to persisted instance records

Important distinction:

- enabling Jira is not the same as creating a Jira instance
- installing Driver is not the same as linking Driver to a project

---

## 6. Routing Summary

Required routing structure:

- `/settings/mcp-servers`
  - general MCP page with only:
    - combined Runner Reachability + Allowlisted MCP inventory
    - MCP instances list
- `/settings/mcp-servers/mcp-connect-test`
  - dedicated MCP Test Console
- `/settings/runner`
  - system-wide runner page
- `/projects`
  - project list with one `ADD project` entry point
- `/projects/create`
  - project creation form
- `/projects/$projectId/settings`
  - project settings with MCP link and unlink only

---

## 7. Implementation Notes

Current code already appears to be partially aligned:

- global MCP ownership has largely moved to `/settings/mcp-servers`
- project settings already focus on link and unlink

Remaining architecture work is primarily:

- spec alignment
- route extraction for MCP Test Console
- new `RUNNER` settings page
- project creation route split
- cleanup of any remaining project-owned MCP language in docs and UI
