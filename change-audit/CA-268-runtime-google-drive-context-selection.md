# CA-268: Runtime Google Drive Context Selection

## Scope

Removed the need to keep editing a saved `mcp.driver` file id in Workflow Settings. Enabling Google Drive context on a `context_artifact.v1` instance now only turns the source on; when a run actually needs Drive context and no legacy default file id is stored, the runner pauses and lets the user choose a Google Drive file or folder at runtime through the existing workflow question card plus a Google Picker popup.

## Changes

- `flow_executor.go`: `startInlineEntryChain` now resolves the active source set once, strips `mcp.driver` out of the `FlowContextPackage` collection path, and resolves a runtime Drive target separately before the delegate step starts.
- `flow_executor.go`: added `resolveMCPDriverTargetForRun` and `appendGoogleDriveTargetPrompt`. The selected target becomes a small prompt note telling the AI to use Google Drive MCP tools against the chosen file/folder, instead of injecting the file contents into the context package.
- `interactive_handlers.go` + new `interactive_google_drive_picker.go`: added runner-served Google Picker routes for pending workflow questions. The picker reuses the existing Google Drive setup's access token + Picker API key and posts the selected `file:<id>` or `folder:<id>` target back to the desktop.
- `QuestionCard.tsx`: special-cases the runtime Google Drive picker option so clicking it opens the popup and answers the underlying workflow question when the popup posts its selected target back.
- `WorkflowsSettings.tsx`: removed the always-visible `Google Drive File ID` editor field from the artifact settings UI and replaced it with copy clarifying that the connected Google Drive account comes from Google Drive setup and the file is chosen at runtime.
- `artifact_type_registry.go`, `behavior_registry.go`, `flow_context_package.go`: updated comments/contracts so `mcpDriverFileId` is treated as a legacy optional default, not the primary UX.
- Tests: added runner coverage for runtime target normalization, prompt target injection, runtime prompting, and the legacy-default fallback path.

## Verification

- `rtk go test ./internal/runner -run 'Test(NormalizeGoogleDriveTarget|AppendGoogleDriveTargetPrompt|ResolveMCPDriverTargetForRunPromptsUserWhenSourceEnabled|ResolveMCPDriverTargetForRunUsesLegacyConfiguredDefault|WorkflowDrivenQuestion|StartResolvedFlow)' -count=1`
- `rtk npm run typecheck` in `apps/desktop-flowpilot`

# ---8<--- flowpilot:change-ledger
feature_key: context-regression-engine
source_doc_id: Task-204
change_type: bugfix
summary: switch mcp.driver from saved file-id editing to runtime google drive picker selection and inject the chosen target into MCP prompt instructions instead of file contents
# --->8---
