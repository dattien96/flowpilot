# CP-30 Logic Sync Report

## Source

- `requirements/07-Coding-Plan/priority/CP-30-Google-Drive-Account-Connection-And-Artifact-Binding.md`
- Runner, admin-web, and requirement-doc changes in this revision

## Results

- `[SYNCED]` Proxy auth no longer reports configured when the selected account token is missing or revoked.
  - Implementation: `apps/local-runner/internal/runner/google_drive_proxy_mcp.go`
  - Tests:
    - `TestProxyMcpAuthStatusText_FailsWhenSelectedAccountCredentialMissing`
    - `TestProxyMcpAuthStatusText_ReportsReconnectRequiredForArtifactBinding`

- `[SYNCED]` Artifact-sync folder binding still wins when present, while proxy auth remains selected-account based.
  - Implementation:
    - `apps/local-runner/internal/runner/google_drive_proxy_mcp.go`
    - `apps/local-runner/internal/runner/artifact_google_drive_connection.go`
  - Tests:
    - `TestProxyMcpAuthStatusText_PrefersArtifactSyncBindingOverPlainSelectedAccount`
    - `TestListGoogleDriveAccountsAggregatesMultipleProjectBindingsForSameAccount`

- `[SYNCED]` Proxy-mode launcher and readiness checks now reflect FlowPilot or Go launcher availability, not `npx`.
  - Implementation:
    - `apps/local-runner/internal/runner/google_drive_config.go`
    - `apps/local-runner/internal/runner/google_drive_mcp_provider_config.go`
  - Tests:
    - `TestResolveGoogleDriveProxyMcpStatus_UsesGoRunLauncherWhenNpxIsMissing`
    - `TestValidateGoogleDriveWorkspaceConfig_ProxyPathReportsFlowPilotLauncherWhenUnavailable`

- `[SYNCED]` Proxy validation wording now references FlowPilot or Go instead of Node.js or `npx`.
  - Implementation:
    - `apps/local-runner/internal/runner/google_drive_mcp_provider_config.go`
    - `apps/admin-web/src/routes/_authenticated/settings/components/-GoogleDriveProviderConfigCard.tsx`
  - Tests:
    - `TestValidateGoogleDriveWorkspaceConfig_ProxyPathReportsFlowPilotLauncherWhenUnavailable`
    - `GoogleDriveProviderConfigCard` missing-scope and disabled-state tests

- `[SYNCED]` CP-30 account model, artifact binding flow, capability matrix, and explicit account-management surfaces are implemented across runner and admin-web.
  - Runner coverage:
    - account connect, disconnect, status, migration, proxy selection, reconnect, and scope readiness
  - Admin-web coverage:
    - account connect/disconnect routes
    - artifact account selection before folder binding
    - ready, missing-scope, missing-folder, and reconnect-required UI states

- `[UPDATED]` `SS-02`, `SD-11`, and `CP-29` now describe account-scoped proxy reads and separate project folder binding.

## Conclusion

- `CP-30` logic is aligned with the current implementation and scoped verification suites.
- Repo-wide baseline failures still exist outside this Google Drive slice, but no logic drift remains in the CP-30 implementation itself.
