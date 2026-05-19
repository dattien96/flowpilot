# CP-04: Project MCP Context

**Maps from:** SS-01, SS-02, SD-05, SD-10
**Phase:** Parallel with CP-03 and CP-05, after the project and Supabase foundations exist
**Depends on:** CP-03

---

## 1. Goal

Let each project define the MCP context sources that workflow runs can load at runtime.

This plan covers the project-level registry for context providers such as:

- Jira
- Figma
- Google Drive
- Firebase
- Telegram

The output of this phase is a stable way to configure, validate, and surface project context without mixing it into generic project CRUD.

---

## 2. Current State And Gap

The repo already assumes that MCP context exists in the workflow/runtime model:

- SS-01 says project-level MCP context is optional at project creation time.
- SS-02 says MCP context belongs to the parent project and is shared across flows.
- SD-05 expects MCP context to be injected during prompt assembly.

What is still missing:

- a first-class project context data model
- a settings UI for adding and editing contexts
- a validation/test flow for each provider
- a clean handoff into the runtime prompt assembly path

---

## 3. Data Model

Use a project-scoped registry so each project can own multiple MCP context entries.

### 3.1 Suggested Table

```sql
CREATE TABLE project_mcp_contexts (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  provider_type TEXT NOT NULL, -- jira, figma, google_drive, firebase, telegram
  label TEXT NOT NULL,
  config_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  status TEXT NOT NULL DEFAULT 'pending', -- pending, connected, failed
  last_synced_at TIMESTAMPTZ,
  last_error TEXT,
  created_at TIMESTAMPTZ DEFAULT now(),
  updated_at TIMESTAMPTZ DEFAULT now()
);
```

### 3.2 Rules

- Each context belongs to exactly one project.
- A project can have multiple contexts of the same provider type.
- Connection state must be visible in the UI.
- RLS must prevent cross-project reads and writes.

---

## 4. UI And Flow

Expose the project MCP context manager from the project settings area.

### 4.1 Settings Surface

- list all configured contexts with status badges
- add a new context
- edit an existing context
- test the connection
- remove a context

### 4.2 Provider Setup

- Jira: project URL, auth, and board or ticket mapping
- Figma: file or project token mapping
- Google Drive: folder or drive mapping
- Firebase: project and environment mapping
- Telegram: bot and channel mapping

### 4.3 UX Notes

- keep provider-specific setup in a modal or drawer
- show validation errors inline
- prefer a read-only summary when a context is connected
- surface last sync time and failure state directly in the list

---

## 5. Runtime Integration

The context registry should be consumed by the workflow/prompt runtime:

1. Load the project contexts for the selected project.
2. Filter to active or connected entries.
3. Inject the relevant context payload into the runtime prompt assembly step.
4. Block or warn when a workflow requires a missing provider.

This keeps project context separate from the workflow definition while still making it available when the runner needs it.

---

## 6. Definition Of Done

- [ ] `project_mcp_contexts` table or equivalent schema exists
- [ ] project-scoped repository methods can list, add, edit, test, and remove contexts
- [ ] project settings UI can manage MCP contexts
- [ ] provider status and error state are visible in the UI
- [ ] runtime prompt assembly can load active project MCP context data
- [ ] RLS policies prevent cross-project access
- [ ] tests cover happy path and failed connection state
