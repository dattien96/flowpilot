# Task-025: Drive MCP Auth Flow

## Status

Implemented.

Related prerequisite work:

- [CP-28: Google Cloud Config With UI Auto](../07-Coding-Plan/done/CP-28-Google-Cloud-Config-With-Ui-Auto.md)
- [CP-05-03: Google Drive MCP Implementation Plan](../07-Coding-Plan/priority/CP-05-03-Driver-Mcp.md)

---

## 1. Goal

Let the user start the Google Drive MCP OAuth flow directly from FlowPilot instead of manually opening a terminal and running the `google-drive-mcp auth` command.

The intended UX is:

- user completes the Google Cloud setup flow first
- user uploads the Desktop OAuth JSON in `/settings/google-drive-setup`
- user clicks `Start Auth` in the Google Drive MCP block
- FlowPilot opens a new terminal and runs the MCP auth flow with the runner-managed credential and token paths
- after Google sign-in completes, the MCP token file is created locally
- user returns to FlowPilot and clicks `Refresh MCP status`

---

## 2. Prerequisite

This button only works after the Desktop OAuth JSON has already been saved through the CP-28 setup flow.

Required prerequisite:

- [CP-28: Google Cloud Config With UI Auto](../07-Coding-Plan/done/CP-28-Google-Cloud-Config-With-Ui-Auto.md) must have already copied the Desktop OAuth JSON to the runner-managed path

Required file before auth can start:

- `~/.config/google-drive-mcp/gcp-oauth.keys.json`

Without that file, FlowPilot must reject the auth start action because the MCP package cannot complete OAuth without the Desktop client credentials.

---

## 3. Implemented Behavior

FlowPilot now exposes a `Start Auth` button inside the Google Drive MCP block on `/settings/google-drive-setup`.

When clicked, the local runner opens a new terminal and launches:

```bash
GOOGLE_DRIVE_OAUTH_CREDENTIALS="$HOME/.config/google-drive-mcp/gcp-oauth.keys.json" \
GOOGLE_DRIVE_MCP_TOKEN_PATH="$HOME/.config/google-drive-mcp/tokens.json" \
npx -y @piotr-agier/google-drive-mcp auth
```

Runner-managed behavior:

- the runner resolves the stored Desktop OAuth JSON path
- the runner resolves the managed token output path
- the runner opens a terminal window and keeps it open for the interactive auth flow
- the MCP package handles the browser sign-in and OAuth callback flow

---

## 4. Token File Outcome

Important behavior to document:

- uploading the Desktop OAuth JSON does not create the token file
- the token file is created only after the MCP auth flow completes successfully

Expected token path:

- `~/.config/google-drive-mcp/tokens.json`

This means Step 5 may still show `needs_auth` after the JSON upload, and that is expected until the auth flow finishes and `tokens.json` exists and is valid.

---

## 5. UI Notes

Current UI behavior:

- `Install` verifies that the MCP package can be launched from the runner machine
- `Start Auth` opens the interactive Google Drive MCP OAuth flow
- `Refresh MCP status` reloads MCP runtime status and should be used after the terminal/browser auth completes

The MCP block and Step 5 now work together like this:

- Step 5 stores the Desktop OAuth JSON
- the MCP block starts auth with that stored JSON
- the MCP token file is created after successful auth
- refresh then moves the status away from `needs_auth`
