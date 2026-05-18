# FlowPilot Tech Design - Project Management

This document maps the business specs from `SS-01`, `SS-02`, and `SS-03` into technical implementation details.

## 1. UI/UX Design (Frontend - React)
- **Project Listing Screen:** A dashboard displaying all projects, with options to Create, Delete, and Update.
- **Project Creation Modal/Screen:**
  - **Inputs:** Name, Local Directory Path (using browser file picker/system dialog integration if possible, or manual text input).
  - **Context Setup:** UI to add/configure MCP Contexts (e.g., Jira URL, Figma Link, Google Drive Auth).
  - **Team Assignment:** Dropdown to select an existing Team or create a new one.
- **Team Management Screen:** CRUD operations for Teams and Members (Name, Role: android/ios/backend/frontend/qa, Level: junior/mid/senior, Skills list).

## 2. Database Design (Supabase)
- `projects` table: `id`, `name`, `directory_path`, `created_at`.
- `project_teams` table (join table, N:N): `id`, `project_id`, `team_id`.
- `integrations` table: `id`, `project_id`, `type` (jira, figma, google_drive, firebase, telegram), `config_encrypted` (JSONB), `status` (pending/connected/failed), `last_synced_at`.
- `teams` table: `id`, `name`.
- `team_members` table: `id`, `team_id`, `name`, `role`, `level`, `skills` (JSON array).

## 3. Go-Runner Integration
- When the Go Cobra runner receives a task for a project, it queries the `directory_path` from Supabase and executes an `os.Chdir(directory_path)` before running LLM prompts or local commands.
- The runner fetches the `integrations` table to initialize the required MCP servers for the project.

---

## 4. MCP Server Installation & Configuration

This section documents how FlowPilot's Go-Runner will install and configure each MCP server when a user adds it to a project.

### 4.1 Jira MCP (Atlassian Official Remote Server)
- **Type:** Remote MCP Server (managed by Atlassian).
- **Setup:** No local install required. The Go-runner registers the Atlassian remote MCP endpoint URL in the AI provider's config.
- **Auth:** OAuth flow — user completes browser-based authentication when first connecting.
- **Config for Claude:**
  ```bash
  claude mcp add atlassian --transport http <atlassian-remote-mcp-url>
  ```
- **Config for Gemini/Codex:** Add the remote server URL to the provider's `settings.json` or `mcp.json`.

### 4.2 Figma MCP (Official Remote Server)
- **Type:** Remote MCP Server (managed by Figma).
- **Docs:** https://mcp.figma.com/mcp
- **Setup:** Use the official deep link for each AI provider. The Go-runner opens the link or registers the remote endpoint.
- **Auth:** OAuth flow via browser.
- **Config for Claude:**
  ```bash
  claude mcp add figma --transport http <figma-remote-mcp-url>
  ```

### 4.3 Google Drive MCP (Google Managed Remote Server)
- **Type:** Remote MCP Server (managed by Google).
- **Setup:** Register the Google remote MCP server URL in the provider config.
- **Auth:** Standard Google OAuth flow.
- **Config for Claude:**
  ```bash
  claude mcp add google-drive --transport http <google-drive-mcp-url>
  ```

### 4.4 Firebase MCP (Official via Firebase CLI)
- **Type:** Local MCP Server (runs via `npx firebase-tools`).
- **Prerequisites:** Node.js + npm installed. User must be logged in: `npx -y firebase-tools@latest login`.
- **Install command (Claude):**
  ```bash
  claude mcp add firebase -- npx -y firebase-tools@latest experimental:mcp
  ```
- **Install command (Gemini/Codex):** Add to `settings.json` / `mcp.json`:
  ```json
  {
    "command": "npx",
    "args": ["-y", "firebase-tools@latest", "experimental:mcp"]
  }
  ```
- **Verification:**
  ```bash
  claude mcp list
  ```

### 4.5 Telegram MCP
- **Option A: Official Anthropic Plugin (Claude only):**
  1. User creates a bot via @BotFather on Telegram → gets API token.
  2. Go-runner executes:
     ```bash
     claude plugin install telegram@claude-plugins-official
     ```
  3. Configure token: `/telegram:configure <token>`
  4. Restart with channels: `claude --channels`
- **Option B: Custom MCP Server (provider-agnostic):**
  1. Use Composio or community `telegram-mcp` server.
  2. User provides `API_ID`, `API_HASH` from https://my.telegram.org/apps.
  3. Go-runner registers the server in the provider's config.

### 4.6 Go-Runner MCP Installation Flow
1. User selects an MCP type in the React UI → Frontend writes `type` + `config_encrypted` to `integrations` table.
2. Go-runner detects the new context record.
3. Go-runner checks if the MCP is already configured for the active AI provider (e.g., `claude mcp list`).
4. If not installed → Go-runner runs the appropriate install/register command (see above).
5. If auth is required → Go-runner opens a browser URL for OAuth and waits for user confirmation.
6. Go-runner updates `integrations.status` to `connected` or `failed`.
