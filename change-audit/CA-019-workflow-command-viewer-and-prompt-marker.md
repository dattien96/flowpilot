# CA-019: Workflow Command Viewer and Prompt Marker

## Scope

This audit captures the provider command diagnostics work that made the execution logs easier to inspect during workflow run debugging.

## Completed

- Added a provider command viewer to the workflow run detail page so developers can inspect the exact command used to invoke the AI provider.
- Updated the local runner command serialization to append a visible stdin redirection marker to `command.txt`.
- Improved the command display so the prompt flow is easier to understand when debugging local runs.

## Verification

- Confirmed `command.txt` shows the stdin prompt marker.
- Verified the workflow run detail page renders the command viewer with the captured command text.

