# CA-024: Artifact Sync Planning And Cloud Provider Review

## Scope

This audit records the CP-06-01 planning revision after reviewing the `/artifacts` demo panel, project preference schema, local runner filesystem driver, Google Drive MCP placeholder, local artifact layouts, and official Supabase and Google Drive documentation.

## Second Review Findings

- The existing `/artifacts` form is a filesystem mirror configuration, not an online artifact-storage selector.
- The project model already defaults `artifactStoragePreference` to `supabase`, but no Supabase Storage bucket or adapter exists.
- Existing Google Drive MCP setup cannot be reused as the artifact upload provider.
- Google Drive artifact sync needs separate server-side OAuth, offline refresh-token storage, and one selected folder per project.
- `integrations.config_encrypted` is readable JSONB under authenticated RLS policies. It is not suitable for refresh tokens.
- Supabase private bucket URLs must be signed on demand rather than persisted.
- Google Picker requires browser interaction. QR UI should open a short-lived FlowPilot connect link that continues through OAuth and folder selection.
- The QR connect token and OAuth `state` must be separate, single-use hashed values.
- Picker receives a short-lived access token in memory only; it never receives or persists the refresh token.
- Runner cloud export uses a bounded streamed ZIP bundle instead of JSON/base64 payloads.

## Updated Decision

- Make private Supabase Storage the default online provider.
- Add optional project-scoped Google Drive storage with QR/deep-link OAuth and folder picker.
- Keep local runner filesystem mirroring as an independent advanced backup option.
- Orchestrate online upload in the authenticated Next.js server route.
- Keep runner responsibility limited to local artifact bundle export, local sync-result persistence, and optional filesystem mirroring.
- Use focused artifact-storage gateways and avoid broad workflow gateway changes.

## Preserved History Rules

- Support declared workflow snapshots, fallback snapshots, and standalone runner prompt artifacts.
- Upload normalized snapshot history.
- Promote canonical output only from the newest successfully synced snapshot.
- Keep older retries idempotent without regressing canonical output.
- Persist failures best-effort and keep retries possible.

## Impact Review

GitNexus reports:

- `ArtifactStoragePanel`: LOW risk
- `/api/local-runner/storage-driver`: LOW risk with one UI consumer
- `Runner.SaveStorageDriver`: LOW risk
- `Runner.ValidateStorageDriver`: LOW risk
- `Runner.SyncArtifact`: LOW risk
- `WorkflowGateway`: CRITICAL risk, intentionally avoided
- `WorkflowEngineGateway`: HIGH risk, intentionally avoided

## Verification

- Reviewed `apps/admin-web/src/presentation/components/artifacts/artifact-storage-panel.tsx`.
- Reviewed `apps/admin-web/src/app/api/local-runner/storage-driver/route.ts`.
- Reviewed `apps/admin-web/src/app/api/local-runner/artifacts/[artifactId]/sync/route.ts`.
- Reviewed `apps/admin-web/src/domain/model/entity/project.ts`.
- Reviewed `apps/local-runner/internal/runner/artifacts.go`.
- Reviewed `apps/local-runner/internal/runner/secret_store.go`.
- Reviewed Supabase migrations for project preferences and integration metadata.
- Verified official Google guidance for web-server offline OAuth, `drive.file`, Google Picker folder selection, Drive parent-folder uploads, Supabase private buckets, signed URLs, uploads, and Vault.

## Residual Notes

- This audit covers planning only. Implementation and automated tests remain pending.
- Google OAuth environment values and the public QR connect origin are prerequisites.
- The plan defaults to one Drive authorization and selected folder per project.
