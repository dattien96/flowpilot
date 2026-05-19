# CP-05: Project MCP Context

**Maps from:** SS-01, SS-02, SD-04, SD-05, SD-10
**Phase:** After CP-04, once the UUID project/team baseline and project settings shell exist
**Depends on:** CP-04

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

- extending the existing simple MCP settings UI on the project page into the full add/edit/test flow
- a Go-Runner orchestration flow to install and configure each MCP server
- a validation/test flow for each provider (OAuth completion, connection check)
- a clean handoff into the runtime prompt assembly path

---

## 3. Data Model

The canonical storage for project MCP integrations is the `integrations` table defined in SD-04. **CP-05 owns this table.** Do not move MCP config storage to CP-04 or CP-10.

### 3.1 Schema

Apply the following migration in this phase:

```sql
CREATE TABLE integrations (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  type TEXT NOT NULL CHECK (type IN ('jira', 'figma', 'google_drive', 'firebase', 'telegram')),
  label TEXT NOT NULL DEFAULT '',
  config_encrypted JSONB NOT NULL DEFAULT '{}'::jsonb,
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'awaiting_oauth', 'connected', 'failed')),
  last_synced_at TIMESTAMPTZ,
  last_error TEXT,
  created_at TIMESTAMPTZ DEFAULT now(),
  updated_at TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX integrations_project_idx ON integrations(project_id);
CREATE INDEX integrations_project_type_idx ON integrations(project_id, type);

ALTER TABLE integrations ENABLE ROW LEVEL SECURITY;

-- MVP admin/member policy. CP-10 hardens this for production.
CREATE POLICY "integrations_select_all_authenticated"
  ON integrations FOR SELECT
  USING (auth.role() = 'authenticated');

CREATE POLICY "integrations_write_all_authenticated"
  ON integrations FOR ALL
  USING (auth.role() = 'authenticated')
  WITH CHECK (auth.role() = 'authenticated');
```

Full column reference after creation:

| Column | Type | Notes |
|---|---|---|
| `id` | UUID | PK |
| `project_id` | UUID | FK → projects, CASCADE DELETE |
| `type` | TEXT | `jira`, `figma`, `google_drive`, `firebase`, `telegram` |
| `label` | TEXT | User-defined display name for this integration |
| `config_encrypted` | JSONB | Provider config — **encrypted at rest** (Supabase Vault or app-layer key). Never plaintext. |
| `status` | TEXT | `pending` / `awaiting_oauth` / `connected` / `failed` |
| `last_synced_at` | TIMESTAMPTZ | Set by Go-Runner after successful connect/sync |
| `last_error` | TEXT | Last failure message from Go-Runner |

### 3.1.1 Domain Model

```typescript
// src/domain/model/entity/integration.ts
export type IntegrationType = 'jira' | 'figma' | 'google_drive' | 'firebase' | 'telegram';
export type IntegrationStatus = 'pending' | 'awaiting_oauth' | 'connected' | 'failed';

export interface Integration {
  id: string;                // UUID string
  projectId: string;         // UUID string
  type: IntegrationType;
  label: string;
  configEncrypted: Record<string, unknown>;
  status: IntegrationStatus;
  lastSyncedAt: string | null;
  lastError: string | null;
  createdAt: string;
  updatedAt: string;
}
```

### 3.2 Status State Machine

```
pending → awaiting_oauth → connected
                        ↘ failed
pending → connected  (for non-OAuth providers)
connected → failed   (if Go-Runner detects disconnection)
failed → pending     (user triggers retry)
```

- `pending`: record created, Go-Runner has not yet acted.
- `awaiting_oauth`: Go-Runner opened the OAuth browser URL; waiting for user to complete auth.
- `connected`: Go-Runner confirmed the MCP server is installed and authenticated.
- `failed`: install or auth step returned an error; `last_error` is set.

### 3.3 Rules

- Each integration belongs to exactly one project.
- A project can have multiple integrations of the same type (e.g., two Jira workspaces).
- `config_encrypted` must never be written in plaintext — encrypt before insert.
- RLS uses an authenticated admin/member policy for MVP. CP-10 hardens this into strict project-member access.
- The Go-Runner reads `integrations` using the service role key; the frontend uses the anon key scoped by RLS.

---

## 4. UI and Flow

Expose the project MCP context manager from the project settings area. There is already a simple MCP settings screen on the project page; extend that screen into the full CP-05 experience instead of creating a separate duplicate surface.

### 4.1 Settings Surface

- list all configured integrations with status badges (`pending`, `awaiting_oauth`, `connected`, `failed`)
- add a new integration (select provider type, enter label and config)
- edit an existing integration
- trigger a connection test (calls backend to re-run Go-Runner install/verify step)
- remove an integration

### 4.2 Provider Setup Fields

- **Jira:** project URL, OAuth (browser flow), board or ticket mapping
- **Figma:** file or project token mapping, OAuth (browser flow)
- **Google Drive:** folder or drive mapping, Google OAuth (browser flow)
- **Firebase:** project ID and environment, `npx firebase-tools` login required
- **Telegram:** Option A — Claude plugin + BotFather token; Option B — Composio/community MCP + API_ID and API_HASH

### 4.3 UX Notes

- keep provider-specific setup in a modal or drawer
- show validation errors inline
- prefer a read-only summary when a context is `connected`; show edit affordance only on hover
- surface `last_synced_at` and `last_error` directly in the list row
- show a spinner with "Waiting for OAuth…" label while `status = 'awaiting_oauth'`

---

## 5. Go-Runner MCP Installation Flow

The **Go-Runner is the installer and configurator** — the frontend only writes the `integrations` record and polls `status`. See SD-04 §4.6 for full detail.

Orchestration steps per new or retried integration record:

1. Frontend writes `type`, `label`, `config_encrypted`, `status = 'pending'` to `integrations`.
2. Go-Runner detects the new/pending record (poll or Supabase Realtime).
3. Go-Runner checks whether the MCP is already configured for the active AI provider (e.g., `claude mcp list`).
4. If not installed → Go-Runner runs the provider-specific install command:
   - **Jira/Figma/Google Drive:** `claude mcp add <name> --transport http <remote-url>` (remote MCP, no local install)
   - **Firebase:** `claude mcp add firebase -- npx -y firebase-tools@latest experimental:mcp`
   - **Telegram (Option A):** `claude plugin install telegram@claude-plugins-official`
   - **Telegram (Option B):** register community MCP server in provider config
5. If OAuth is required → Go-Runner opens the browser URL and sets `status = 'awaiting_oauth'`. It waits for auth completion (polling or callback).
6. On success → Go-Runner sets `status = 'connected'`, `last_synced_at = NOW()`, clears `last_error`.
7. On failure → Go-Runner sets `status = 'failed'`, writes error to `last_error`.

---

## 6. Runtime Integration (Go-Runner Prompt Assembly)

At the start of each workflow step execution, the Go-Runner validates and loads MCP context (see SD-05 §8):

```sql
-- Fetch connected integrations for the project
SELECT type, config_encrypted
FROM integrations
WHERE project_id = $1
  AND status = 'connected';
```

Logic:

1. Load `step_definitions.required_mcps` for the current step type.
2. Compare against the connected integrations from the query above.
3. If any required MCP is missing or not `connected`:
   - Set `workflow_run_steps.status = 'failed'`
   - Set `workflow_run_steps.error_message = 'Missing required MCP: <type>'`
   - Do not proceed to prompt assembly.
4. If all required MCPs are present:
   - Inject the relevant MCP context payload into the `# MCP Context` section of the assembled prompt (see SD-05 §3 prompt template).
   - Proceed with execution.

---

## 7. Definition of Done

### Data Model
- [ ] `integrations` table created in CP-05 with UUID `project_id`, provider type, encrypted config, status, `last_synced_at`, and `last_error`
- [ ] `status` field supports all four states: `pending`, `awaiting_oauth`, `connected`, `failed`
- [ ] `config_encrypted` is encrypted before insert and decrypted only by the Go-Runner service role
- [ ] MVP RLS policies allow authenticated admin/member access, with production hardening deferred to CP-10

### Go-Runner
- [ ] Go-Runner detects new/pending integration records and runs the correct install command per provider type
- [ ] OAuth flow opens browser URL, sets `awaiting_oauth`, and waits for completion
- [ ] Go-Runner updates `status`, `last_synced_at`, and `last_error` after each install attempt
- [ ] Go-Runner validates required MCPs before each step and fails the step with a named error if missing

### UI
- [ ] Existing project-page MCP settings screen extended to list integrations with correct status badges
- [ ] Add / edit / remove actions write to `integrations` table correctly
- [ ] `awaiting_oauth` state shows a waiting indicator
- [ ] `last_error` and `last_synced_at` are visible in the list

### Tests
- [ ] Tests cover happy path: integration connects successfully
- [ ] Tests cover failed connection state: Go-Runner writes `failed` + `last_error`
- [ ] Tests cover runtime step validation: missing MCP fails the step with the correct error message
