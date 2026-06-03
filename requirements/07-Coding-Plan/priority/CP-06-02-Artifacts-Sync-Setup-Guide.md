# CP-06-02: Artifacts Sync Setup Guide

## 1. Purpose

This guide lists the required setup before testing CP-06 artifact sync.

Use it before testing either shared provider:

- `supabase`
- `google_drive`

Local artifacts already live in `.flowpilot/artifacts/...`. This guide is only about shared sync and provider-specific prerequisites.

## 2. What You Are Testing

CP-06 artifact sync has two layers:

1. Local artifact persistence on the current machine.
2. Shared sync from the local runner to the selected project storage provider.

The local runner is always required. The selected project provider determines whether shared sync goes to Supabase Storage or Google Drive.

## 3. Common Prerequisites

Before testing either provider, make sure all of these are true:

- The local runner is running.
- `apps/admin-web` is running.
- You can log in to admin-web with a valid Supabase-authenticated user.
- The target project exists.
- The target project has `artifact_storage_preference` set to the provider you want to test.
- The runner can already create local artifacts under `.flowpilot/artifacts`.

## 4. Supabase Setup

### 4.1 Required Environment

The runner needs these environment variables:

```env
SUPABASE_API_URL=https://your-project.supabase.co
SUPABASE_SERVICE_ROLE_KEY=your-service-role-key
```

Admin-web also needs its normal Supabase app configuration so users can log in.

### 4.2 Required Database and Storage State

The database and storage side should already include:

- `projects.artifact_storage_preference`
- `artifact_runs.storage_provider`
- `artifact_runs.remote_object_id`
- private storage bucket `flowpilot-artifacts`

The project should use:

```text
artifact_storage_preference = supabase
```

### 4.3 Minimal Test Flow

1. Start the local runner.
2. Start admin-web.
3. Log in.
4. Open `/artifacts` or a project artifact screen.
5. Ensure the selected project shows `supabase` as the active provider.
6. Sync an artifact.
7. Verify the artifact uploads to `flowpilot-artifacts`.
8. Open the artifact and verify a signed URL is generated on demand.

### 4.4 Expected Failure Modes

If Supabase is not configured correctly, typical errors are:

- `missing required environment variable: SUPABASE_API_URL`
- `missing required environment variable: SUPABASE_SERVICE_ROLE_KEY`
- storage upload or signed URL errors from the runner

## 5. Google Drive Setup

### 5.1 Required Environment

The local runner needs all of these variables before the Google Drive connect flow can start:

```env
GOOGLE_DRIVE_CLIENT_ID=your-google-oauth-client-id
GOOGLE_DRIVE_CLIENT_SECRET=your-google-oauth-client-secret
GOOGLE_DRIVE_REDIRECT_URI=http://127.0.0.1:3000/artifact-storage/google-drive/oauth/callback
GOOGLE_PICKER_API_KEY=your-google-picker-browser-api-key
```

The exact redirect URI must match the runner callback route and must also be registered in the Google Cloud OAuth client.

### 5.2 Required Google Cloud Setup

In Google Cloud Console, configure:

1. An OAuth client for the runner callback.
2. The Google Drive API.
3. A browser API key for Google Picker.
4. An authorized redirect URI matching `GOOGLE_DRIVE_REDIRECT_URI`.

The OAuth client and API key must belong to the same Google Cloud project used for Drive testing.

### 5.3 Required Product State

The target project should use:

```text
artifact_storage_preference = google_drive
```

The current runner host must complete its own Drive connection flow. Google Drive auth is runner-local, not shared automatically across PCs.

### 5.4 Minimal Test Flow

1. Start the local runner with the Google env vars above.
2. Start admin-web.
3. Log in.
4. Open `/artifacts`.
5. Select the target project.
6. Confirm the panel shows `google_drive` as the active provider.
7. Press `Connect Google Drive`.
8. Complete Google OAuth in the opened browser tab.
9. Select a Drive folder in Google Picker.
10. Return to the artifacts page and confirm the folder/account metadata appears.
11. Sync an artifact.
12. Open the artifact and verify it resolves to the Drive-backed destination.

### 5.5 Expected Failure Modes

If Google Drive is not configured correctly, typical errors are:

- `missing required environment variable: GOOGLE_DRIVE_CLIENT_ID`
- `missing required environment variable: GOOGLE_DRIVE_CLIENT_SECRET`
- `missing required environment variable: GOOGLE_DRIVE_REDIRECT_URI`
- `missing required environment variable: GOOGLE_PICKER_API_KEY`
- OAuth redirect mismatch errors from Google
- Picker load failures caused by a bad or missing browser API key

## 6. Multi-PC Notes

Supabase:

- Shared provider.
- Any PC with the same deployment config can sync immediately.

Google Drive:

- Shared folder is possible.
- Each PC must complete its own local Google Drive connection flow.
- Each runner stores its own refresh token locally.

## 7. Recommended Test Order

Use this order when validating CP-06 from scratch:

1. Verify local artifact generation under `.flowpilot/artifacts`.
2. Verify Supabase sync.
3. Verify Supabase open flow.
4. Switch one project to Google Drive.
5. Complete Google Drive connect flow.
6. Verify Google Drive sync.
7. Verify Google Drive open flow.

## 8. Quick Checklist

- Runner is running.
- Admin-web is running.
- User can log in.
- Project exists.
- Project provider is set correctly.
- Supabase env vars are present when testing `supabase`.
- Google env vars are present when testing `google_drive`.
- Google redirect URI is registered correctly.
- Artifact sync creates or updates shared metadata after upload.
