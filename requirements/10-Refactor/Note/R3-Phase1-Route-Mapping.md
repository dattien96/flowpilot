# R3 Phase 1 Route Mapping

This file is the desktop-side source mapping artifact for Phase 1.

## `/login`

- Desktop files:
  - `apps/desktop-flowpilot/src/App.tsx`
  - `apps/desktop-flowpilot/src/components/LoginScreen.tsx`
  - `apps/desktop-flowpilot/src/auth/desktopSupabaseAuthRepository.ts`
- Shared core:
  - `packages/flowpilot-client-core/src/domain/auth.ts`
  - `packages/flowpilot-client-core/src/domain/runtime.ts`
  - `packages/flowpilot-client-core/src/data/runnerRuntimeConfigRepository.ts`
- Runner endpoints:
  - `GET /supabase-config`
  - `POST /supabase-config/validate`
  - `PUT /supabase-config`
- Notes:
  - Login is blocked when runtime config is missing.
  - Pre-auth settings are available from the login screen.

## `/settings/supabase` and `/setup/supabase`

- Desktop files:
  - `apps/desktop-flowpilot/src/components/SupabaseSetupScreen.tsx`
  - `apps/desktop-flowpilot/src/components/SettingsShell.tsx`
- Shared core:
  - `packages/flowpilot-client-core/src/domain/runtime.ts`
  - `packages/flowpilot-client-core/src/data/runnerRuntimeConfigRepository.ts`
- Runner endpoints:
  - `GET /supabase-config`
  - `POST /supabase-config/validate`
  - `PUT /supabase-config`

## `/projects`, `/projects/$projectId`, `/projects/$projectId/directory-bindings`, `/projects/$projectId/settings`

- Desktop files:
  - `apps/desktop-flowpilot/src/components/settings/ProjectsSettings.tsx`
  - `apps/desktop-flowpilot/src/components/settings/settingsHelpers.ts`
- Shared core:
  - `packages/flowpilot-client-core/src/domain/adminLogic.ts`
  - `packages/flowpilot-client-core/src/domain/adminModels.ts`
  - `packages/flowpilot-client-core/src/domain/adminRepositories.ts`
  - `packages/flowpilot-client-core/src/data/supabaseAdminRepository.ts`
  - `packages/flowpilot-client-core/src/data/runnerAdminRepository.ts`
- Runner endpoints:
  - `POST /directories/validate`
- Supabase tables:
  - `projects`
  - `project_workspace_bindings`
  - `project_team_links`
  - `workflow_runs`

## `/workflows`, `/workflows/create`, `/workflows/$workflowId`, `/workflow-steps`, `/workflow-steps/create`, `/workflow-steps/$stepType`

- Desktop files:
  - `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx`
- Shared core:
  - `packages/flowpilot-client-core/src/domain/adminModels.ts`
  - `packages/flowpilot-client-core/src/domain/adminRepositories.ts`
  - `packages/flowpilot-client-core/src/data/supabaseAdminRepository.ts`
- Supabase tables:
  - `workflows`
  - `workflow_steps`
  - `workflow_step_definitions`

## `/teams`

- Desktop files:
  - `apps/desktop-flowpilot/src/components/settings/TeamsSettings.tsx`
- Shared core:
  - `packages/flowpilot-client-core/src/domain/adminModels.ts`
  - `packages/flowpilot-client-core/src/data/supabaseAdminRepository.ts`
- Supabase tables:
  - `teams`
  - `team_members`
  - `project_team_links`

## `/artifacts?tab=generated`, `/artifacts?tab=storage`, `/artifacts?tab=catalog`

- Desktop files:
  - `apps/desktop-flowpilot/src/components/settings/ArtifactsSettings.tsx`
- Shared core:
  - `packages/flowpilot-client-core/src/domain/adminModels.ts`
  - `packages/flowpilot-client-core/src/data/supabaseAdminRepository.ts`
  - `packages/flowpilot-client-core/src/data/runnerAdminRepository.ts`
- Runner endpoints:
  - `GET /artifacts`
  - `GET /storage-driver`
  - `PUT /storage-driver`
- Supabase tables:
  - `artifact_definitions`
  - `artifact_runs`
  - `projects`

## `/settings/ai-providers`

- Desktop files:
  - `apps/desktop-flowpilot/src/components/settings/AiProvidersSettings.tsx`
- Shared core:
  - `packages/flowpilot-client-core/src/domain/adminLogic.ts`
  - `packages/flowpilot-client-core/src/data/supabaseAdminRepository.ts`
  - `packages/flowpilot-client-core/src/data/runnerAdminRepository.ts`
- Runner endpoints:
  - `GET /providers`
  - `POST /providers/auth`
- Supabase tables:
  - `supported_models`

## `/settings/google-drive-setup`

- Desktop files:
  - `apps/desktop-flowpilot/src/components/SettingsShell.tsx`
  - `apps/desktop-flowpilot/src/components/settings/McpSettings.tsx`
- Shared core:
  - `packages/flowpilot-client-core/src/data/supabaseAdminRepository.ts`
  - `packages/flowpilot-client-core/src/data/runnerAdminRepository.ts`
- Runner endpoints:
  - `GET /mcp/backends`
  - `POST /mcp/backends/:key/install`
  - `POST /mcp/backends/:key/actions`
  - `POST /integrations/connect`
- Supabase tables:
  - `integrations`
  - `project_integrations`

## `/settings/mcp-servers/jira-link` and helper flows

- Desktop files:
  - `apps/desktop-flowpilot/src/components/SettingsShell.tsx`
  - `apps/desktop-flowpilot/src/components/settings/McpSettings.tsx`
- Shared core:
  - `packages/flowpilot-client-core/src/data/supabaseAdminRepository.ts`
  - `packages/flowpilot-client-core/src/data/runnerAdminRepository.ts`
- Runner endpoints:
  - `GET /mcp/backends`
  - `POST /mcp/backends/:key/install`
  - `POST /mcp/backends/:key/actions`
  - `POST /integrations/connect`
- Supabase tables:
  - `integrations`
  - `project_integrations`

## `/settings/runner`

- Desktop files:
  - `apps/desktop-flowpilot/src/components/RunnerHealthPanel.tsx`
  - `apps/desktop-flowpilot/src/components/RunnerStatusIndicator.tsx`
- Shared core:
  - `packages/flowpilot-client-core/src/domain/runner.ts`
  - `packages/flowpilot-client-core/src/data/runnerRepository.ts`
- Runner endpoints:
  - `GET /health`
