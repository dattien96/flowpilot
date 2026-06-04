# CP-27: Google Cloud Setting for Google Drive MCP and Artifact Sync

## Status

Planning and setup reference.

This document is the shared Google Cloud setup model for:

- [CP-05-03: Google Drive MCP](../priority/CP-05-03-Driver-Mcp.md)
- [Task-023: Sync Artifact With Google](../../08-Task/Task-023-Sync-Artifact-With-Google.md)

CP-27 owns the explanation of Google Cloud project, OAuth, API key, user consent, and local token storage. Feature-specific implementation details belong in the linked plans.

---

## 1. Goal

Use one Google Cloud project as the shared FlowPilot app registration for both:

- Google Drive MCP
- Google Drive artifact sync

Users still connect their own real Google Drive accounts. The Google Cloud project identifies FlowPilot as an app; it does not make all users use the project owner's Drive.

---

## 2. Core Model

### 2.1 Google Cloud project

The Google Cloud project is shared app infrastructure.

It contains:

- OAuth consent screen
- OAuth client ID
- OAuth client secret
- authorized redirect URIs
- optional browser API key for Google Picker
- enabled Google APIs

The project can be owned by the FlowPilot maintainer for the MVP.

### 2.2 User Google account

Each user signs in with their own Google account and grants permission to their own Drive.

Example:

- FlowPilot owner creates the Google Cloud project.
- Friend A signs in with `friend-a@gmail.com`.
- Friend B signs in with `friend-b@gmail.com`.
- Both users authorize on the same FlowPilot app registration (admin gg acount that created GG cloud console project).
- FlowPilot receives tokens for each user's own account, not for the project owner's account.

### 2.3 OAuth token

An OAuth token is local permission granted by a user.

The separation is:

- Google Cloud project defines the app.
- User login grants that app permission to one Google account.
- Access token allows short-lived API access.
- Refresh token allows the runner to request new access tokens later.

Current artifact sync design stores refresh tokens only in the local runner secret store on the current PC.

---

## 3. OAuth vs API Key

OAuth and API keys are different and cannot be treated as interchangeable.

### 3.1 OAuth

OAuth is required when FlowPilot needs **private user data**.

OAuth is used to:

- **redirect the user** to Google's consent screen
- **ask the user to grant** Drive access
- receive an authorization code
- exchange that code for access and refresh tokens
- read or write files in the user's Drive

OAuth is required for both Google Drive MCP and Google Drive artifact sync.

This flow triggered and controlled by GG cloud project of admin

### 3.2 API key

An API key identifies the app to browser-side Google APIs.

**An API key does not prove user consent and does not grant permission to private Drive data.**

In FlowPilot, the API key is currently needed only because artifact sync uses Google Picker for folder selection.

### 3.3 Feature requirements

Current requirement matrix:

| Feature | OAuth | API key | Why |
| --- | --- | --- | --- |
| Google Drive MCP | Required | Not normally required | MCP needs user-authorized Drive access. |
| Artifact sync with Google Picker | Required | Required | Sync needs user-authorized Drive access; Picker needs a browser developer key. |


MVP decision for current code:

- Keep OAuth for both features.
- Keep API key for artifact sync because the implemented flow uses Google Picker.

---

## 4. Google Cloud Project Setup

### 4.1 Create or choose project

Create one Google Cloud project for FlowPilot.

Recommended MVP naming:

```text
FlowPilot Local MVP
```

This one project can serve both MCP and artifact sync.

### 4.2 Configure OAuth consent screen

Configure the OAuth consent screen for the FlowPilot app.

Minimum values:

- App name: `FlowPilot`
- User support email: maintainer email
- Developer contact email: maintainer email
- User type: external for friends outside your Google Workspace

#### **Console guide after the project already exists:**
https://developers.google.com/workspace/guides/configure-oauth-consent

1. Open Google Cloud Console and make sure the correct project is selected.
2. Go to `Google Auth Platform`.
3. Open the consent-screen area. Depending on the current console layout, this may appear under `Branding`, `Audience`, and `Data Access`.
4. In the app branding step, fill in:
   - App name: `FlowPilot`
   - User support email: your email
   - Developer contact email: your email
5. In the audience step, choose `External` if friends outside your Google Workspace will use the tool.
6. In the test-users step, add the Google accounts that should be allowed to log in while the app is still in `Testing` mode.
7. In the scopes or data-access step, keep the requested scopes as small as possible. Drive access is required for both MCP and artifact sync.
8. Save the consent-screen configuration.

Recommended practical setup for this MVP:

1. Start with `External` + `Testing`.
2. Add your own Google account and any friend test accounts as test users.
3. Confirm the OAuth flow works end to end.
4. Move the app toward `Production` when you want longer-lived, stable refresh-token behavior.

MVP testing note:

- If the app stays in `Testing` mode and the user type is `External`, Google can issue refresh tokens that expire after 7 days for non-trivial scopes such as Drive access.
- That means the first connection may work, but the user can later look "randomly disconnected" because the refresh token is no longer valid.
- `Testing` mode is fine for short local experiments and first-pass setup.
- For repeated personal use across multiple days or for friend testing, move the consent screen toward `Production` so token behavior is stable.
- `Production` here does not mean "public SaaS launch"; it mainly means the OAuth app is no longer treated as a short-lived test app.
- Keep scopes minimal so the app stays easier to operate and verification burden stays lower.
- In `Testing` mode, external users may also need to be added explicitly as test users on the consent screen.

### 4.3 Enable APIs

Enable APIs needed by the features you actually run.

#### **Console guide after the project already exists:**

1. In Google Cloud Console, go to `APIs & Services`.
2. Open `Library`.
3. Search each API by name.
4. Open the API detail page.
5. Press `Enable`.

Minimum for artifact sync:

- Google Drive API
- Google Picker API

For the current Google Drive MCP package:

- Google Drive API
- Google Docs API
- Google Sheets API
- Google Slides API
- Google Calendar API

The MCP package exposes Drive, Docs, Sheets, Slides, and Calendar capabilities, so enable the full set unless FlowPilot intentionally constrains MCP scopes later.

Recommended enable order:

1. Enable `Google Drive API`.
2. Enable `Google Picker API` because the current artifact sync flow uses Picker.
3. Enable `Google Docs API`.
4. Enable `Google Sheets API`.
5. Enable `Google Slides API`.
6. Enable `Google Calendar API` for the current MCP package capability set.

Verification:

- After enabling, each API page should show `Manage` instead of `Enable`.
- If the Picker page or Drive page is not enabled, artifact sync setup will be incomplete.

### 4.4 Create OAuth clients

Use the same Google Cloud project, but allow separate OAuth clients if the flows need different client types.

What an OAuth client is:

- The Google Cloud project is the container.
- Inside that project, you create one or more OAuth client entries.
- Each client entry has its own client ID, client secret or metadata, redirect rules, and intended application type.

So "one Google Cloud project" does not mean "one single client ID forever".

You can have, for example:

- one Web application client
- one Desktop app client
- later another Web application client for a different local callback or environment

All of those can belong to the same Google Cloud project.

Recommended MVP split:

- Artifact sync: OAuth client for local runner callback.
- Google Drive MCP: Desktop app OAuth client expected by `@piotr-agier/google-drive-mcp`.

Why use different client types:

- Artifact sync is implemented by FlowPilot's own runner callback flow.
- Google Drive MCP is implemented by a separate MCP package that already expects Desktop app credentials and manages its own local auth/token files.

So the current split is driven by implementation shape, not by Google forcing two completely different products.

Artifact sync client requirements:

- Application type: `Web application`
- It must allow the runner callback URI.
- Current default runner URL is `http://127.0.0.1:4317`.
- Current callback path is `/artifact-storage/google-drive/oauth/callback`.

#### **Console guide to create the artifact sync OAuth client:**
https://developers.google.com/workspace/guides/create-credentials

1. In Google Cloud Console, go to `Google Auth Platform > Clients`.
2. Press `Create client`.
3. Choose application type `Web application`.
4. Name it something obvious, for example:
   - `FlowPilot Artifact Sync Local Runner`
5. Under redirect URIs, add:

```text
http://127.0.0.1:4317/artifact-storage/google-drive/oauth/callback
```

6. Save the client.
7. Copy the generated `Client ID` and `Client secret`.
8. Put those values into:
   - `GOOGLE_DRIVE_CLIENT_ID`
   - `GOOGLE_DRIVE_CLIENT_SECRET`

Register this redirect URI:

```text
http://127.0.0.1:4317/artifact-storage/google-drive/oauth/callback
```

If `FLOWPILOT_RUNNER_URL` changes, `GOOGLE_DRIVE_REDIRECT_URI` and the Google Cloud authorized redirect URI must change to match.

Artifact sync verification checklist:

1. The redirect URI in Google Cloud exactly matches the value you place in `GOOGLE_DRIVE_REDIRECT_URI`.
2. The port matches the actual runner port.
3. The host matches the actual runner URL, usually `127.0.0.1`.
4. There are no extra slashes, missing path segments, or `localhost`/`127.0.0.1` mismatches.

How `127.0.0.1` works across multiple PCs:

- `127.0.0.1` always means "this same machine".
- PC A uses its own local runner at `127.0.0.1:4317`.
- PC B also uses its own local runner at `127.0.0.1:4317`.
- When the browser on PC A completes OAuth, Google redirects back to PC A's local runner.
- When the browser on PC B completes OAuth, Google redirects back to PC B's local runner.

So yes, the same redirect URI can be reused across many PCs as long as:

- each PC is running its own local runner on that loopback address and port
- the OAuth flow is opened in the browser on that same machine

This is not a shared network callback. It is one local callback per machine.

Important limitation:

- this does not work if the browser runs on one machine but the runner is only running on another machine
- in that case, the redirect would land on the wrong localhost

Why artifact sync uses runner callback instead of Desktop app client:

- FlowPilot artifact sync already implements a Web-style authorization-code flow in the runner
- it expects an exact callback URL
- it performs its own code exchange and then continues into the Picker flow

Why MCP uses Desktop app client:

- the `google-drive-mcp` package already expects Desktop app credential JSON
- it manages its own local auth flow and token files
- changing it to reuse FlowPilot's runner callback would mean wrapping or rewriting the MCP package contract

Could one client type be used for both features?

- In theory, yes, if both flows were redesigned to use the same auth contract.
- In the current implementation, no practical simplification comes from forcing that.
- The safer MVP is one Google Cloud project with two OAuth clients under it:
  - one Web application client for artifact sync
  - one Desktop app client for Google Drive MCP

Google Drive MCP client requirements:

- Application type: Desktop app
- Download the credential JSON
- Rename it to `gcp-oauth.keys.json`
- Place it in the MCP config directory or point the package to it with `GOOGLE_DRIVE_OAUTH_CREDENTIALS`

#### **Console guide to create the Google Drive MCP OAuth client: for desktop app**

1. In Google Cloud Console, go to `Google Auth Platform > Clients`.
2. Press `Create client`.
3. Choose application type `Desktop app`.
4. Name it something obvious, for example:
   - `FlowPilot Google Drive MCP`
5. Save the client.
6. Download the OAuth client JSON file from Google Cloud.
7. Rename that file to:

```text
gcp-oauth.keys.json
```

8. Store it at the default MCP config path or set `GOOGLE_DRIVE_OAUTH_CREDENTIALS` to its absolute path.

Recommended MCP credential path:

```text
~/.config/google-drive-mcp/gcp-oauth.keys.json
```

Then in the runtime, The MCP package stores tokens separately, by default at:

```text
~/.config/google-drive-mcp/tokens.json
```

Examples:

- On Linux/macOS:

```text
~/.config/google-drive-mcp/tokens.json
```

- On Windows, the equivalent is usually under the user profile, for example:

```text
%USERPROFILE%\.config\google-drive-mcp\tokens.json
```

- Or fully:

```text
C:\Users\<your-user>\.config\google-drive-mcp\tokens.json
```

### 4.5 Create API key for Picker

Only needed while artifact sync uses Google Picker.

Create a browser API key and store it as:

```env
GOOGLE_PICKER_API_KEY=...
```

This key does not replace OAuth.

Console guide after the project already exists:

1. In Google Cloud Console, go to `APIs & Services > Credentials`.
2. Press `Create credentials`.
3. Choose `API key`.
4. Fill the form fields like this:

   - `Name`
     Use a clear name, for example:

```text
FlowPilot Artifact Picker Key
```

   - `APIs that can be accessed using this key` or `Select API restrictions`
     Select `Google Picker API`.

     If `Google Picker API` is not available in the list, go back to `APIs & Services > Library`, enable `Google Picker API`, then return to the key form.

   - `Authenticate API calls through a service account`
     Leave this unchecked.

     Reason:
     this Picker key is for browser-side app identification, not for service-account-based server auth.

   - `Application restrictions`
     For the very first local test, you can leave this as `None` so you can confirm the Picker flow works.

     After that first successful test, tighten it.

     Recommended restriction for the current runner-hosted Picker page:
     choose `Websites`.

     Add website/referrer entries that match the runner page origin. For the default local runner URL, use:

```text
http://127.0.0.1:4317/*
```

     If you also access the runner through `localhost`, add:

```text
http://localhost:4317/*
```

     If you change `FLOWPILOT_RUNNER_URL`, the website restriction values must change to match that origin.

5. Create the key.
6. Copy the created key.
7. Save it as:

```env
GOOGLE_PICKER_API_KEY=...
```

8. Open the API key detail page again and confirm:

   - the key is restricted to `Google Picker API`
   - the service-account checkbox is still off
   - website restrictions match the actual runner origin if you decided to tighten them

Practical MVP note:

- For the first local setup, create the key and confirm the Picker flow works.
- If the key works with `None` application restriction, tighten it to `Websites`.
- If Picker starts failing after applying `Websites`, check that the origin exactly matches how the browser loads the runner page.
- The API key only supports Picker app identification; it does not authorize access to private Drive data.

---

## 5. Runner Configuration Outputs

This section is the quick reference for all current Google-related variables across both features.

### 5.1 Artifact sync variables

Artifact sync currently expects:

```env
GOOGLE_DRIVE_CLIENT_ID=...
GOOGLE_DRIVE_CLIENT_SECRET=...
GOOGLE_DRIVE_REDIRECT_URI=http://127.0.0.1:4317/artifact-storage/google-drive/oauth/callback
GOOGLE_PICKER_API_KEY=...
```

Meaning:

- `GOOGLE_DRIVE_CLIENT_ID`
  Artifact sync OAuth client ID from the Web application OAuth client.
- `GOOGLE_DRIVE_CLIENT_SECRET`
  Artifact sync OAuth client secret from the same Web application OAuth client.
- `GOOGLE_DRIVE_REDIRECT_URI`
  Exact runner callback URL registered in Google Cloud for artifact sync.
- `GOOGLE_PICKER_API_KEY`
  Browser API key used only because artifact sync currently launches Google Picker.

Required today:

- all four are required for the current artifact sync implementation

### 5.2 Google Drive MCP variables

MCP setup depends on the selected MCP package. The current runner prepares:

```text
npx -y @piotr-agier/google-drive-mcp --help
```

The current MCP package expects:

```env
GOOGLE_DRIVE_OAUTH_CREDENTIALS=/absolute/path/to/gcp-oauth.keys.json
GOOGLE_DRIVE_MCP_TOKEN_PATH=/absolute/path/to/tokens.json
```

Meaning:

- `GOOGLE_DRIVE_OAUTH_CREDENTIALS`
  Absolute path to the Desktop app OAuth credential JSON used by the MCP package.
- `GOOGLE_DRIVE_MCP_TOKEN_PATH`
  Absolute path where the MCP package stores or reads its token file.

Optional today:

- both are optional if the package uses its default config directory
- FlowPilot should still prefer explicit paths for predictable runner validation

### 5.3 Shared setup note

Artifact sync and MCP can use the same Google Cloud project, but they do not currently use the same local variables:

- artifact sync uses FlowPilot runner OAuth env vars
- MCP uses package-specific credential and token paths

That is why CP-27 treats them as one shared Google setup with two feature-specific configuration surfaces.

---

## 6. OAuth Flow Runtime

FlowPilot does not deploy custom code into Google Cloud for OAuth.

The runtime flow is:

1. FlowPilot redirects the user to Google's OAuth URL using the configured client ID.
2. The user signs in with their own Google account.
3. The user grants FlowPilot Drive access.
4. Google redirects back to the local runner callback URI with an authorization code.
5. The runner exchanges the authorization code for tokens using the client ID and client secret.
6. The runner stores the refresh token in its local secret store.
7. Future Drive calls use fresh access tokens created from the local refresh token.

The Google Cloud project provides credentials and validates redirect URIs; FlowPilot code performs the token exchange.

---

## 7. Sharing With Friends

Friends can use the same FlowPilot Google Cloud project.

They are not using the project owner's Drive.

They use:

- FlowPilot owner's Google Cloud app registration
- their own Google login
- their own Google Drive data
- their own local runner refresh token

Current implementation does not provide a Supabase-style BYO Google Cloud setup screen.

Future enhancement:

- allow each local install or tenant to provide its own OAuth client ID, client secret, redirect URI, and Picker API key
- validate those settings like Supabase runtime config
- keep the user's chosen Google app config in local runner configuration, not in shared project data by default

---

## 8. Security Rules

Do:

- keep client secrets in runner/server-side configuration
- store refresh tokens only in the local runner secret store
- keep access tokens in memory or short-lived request scope
- register exact redirect URIs in Google Cloud
- keep scopes minimal

Do not:

- store refresh tokens in browser local storage
- put OAuth tokens in query strings
- treat API key as authorization
- share one user's refresh token across PCs
- assume MCP connection means artifact sync is ready

---

## 9. Relationship Between Features

One Google Cloud project can support both features, but the feature connection state remains separate.

Google Drive MCP needs:

- MCP backend installed or verified
- MCP-specific OAuth completed
- MCP tools usable by FlowPilot

Artifact sync needs:

- artifact storage preference set to `google_drive`
- artifact sync OAuth completed on the current runner
- one destination Drive folder selected
- runner env variables from this CP-27 setup

They can share the same Cloud project, but they should not silently reuse each other's connection state unless a future design explicitly merges token ownership and scopes.

---

## 10. Validation Checklist

CP-27 setup is ready when:

- Google Cloud project exists.
- OAuth consent screen is configured.
- Required APIs are enabled.
- Artifact sync OAuth client exists.
- Artifact sync redirect URI exactly matches the runner callback URI.
- Picker API key exists if Picker remains enabled.
- Runner has `GOOGLE_DRIVE_CLIENT_ID`.
- Runner has `GOOGLE_DRIVE_CLIENT_SECRET`.
- Runner has `GOOGLE_DRIVE_REDIRECT_URI`.
- Runner has `GOOGLE_PICKER_API_KEY` while Picker is used.
- Google Drive MCP has `gcp-oauth.keys.json` available.
- Google Drive MCP token path is known or uses the default config directory.

---

## 11. Source References

- Google OAuth web server flow: https://developers.google.com/identity/protocols/oauth2/web-server
- Google OAuth refresh token behavior: https://developers.google.com/identity/protocols/oauth2
- Google Picker overview: https://developers.google.com/drive/picker/guides/overview
- Google Drive MCP package: https://github.com/piotr-agier/google-drive-mcp
