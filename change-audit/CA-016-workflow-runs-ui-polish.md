# CA-016: Workflow Runs UI Polish

## Scope

This audit captures several UI/UX and backend enhancements made to improve the experience of executing, viewing, and diagnosing workflow runs in the local runner integration. It includes updates to the Project Detail launcher, the Workflow Run execution panel, and the Go local-runner API.

## Completed

- **Project Launcher Loading Layer:**
  - Implemented an absolute-positioned blurred backdrop overlay with a loading spinner inside the `ProjectExecutionLauncher` component (`$projectId.tsx`). This prevents double clicks and signals to the user that the background execution context is starting.

- **Workflow Run Output Polish:**
  - Implemented output text truncation for the Latest Execution Output panel, cutting off text at 300 characters to prevent overwhelming the UI.
  - Added a "Show more" toggle for truncated text.

- **Inline Artifact Content Viewer:**
  - Added a new endpoint `/files/read` to the Go `local-runner` backend (`apps/local-runner/internal/cli/root.go`) which allows local file reading for generated artifacts.
  - Added `readFile` capability to the frontend `LocalRunnerGateway`.
  - Built an inline `ArtifactContentViewer` component that parses `[ArtifactName.md](Path)` syntax out of the CLI output text. When detected, it renders a button that, upon click, fetches and renders the raw markdown artifact inline.
  - Implemented a fix to strip out invalid `/abs/path/` URL prefixes erroneously outputted by the local CLI to resolve `500 Internal Server Error`s.

- **Provider Command Diagnostics:**
  - Added a new `CollapsibleSection` to the Workflow Run detail view to directly inspect `latestOutput?.commandText`.
  - Updated the Go `local-runner` to explicitly append ` < prompt.txt` to the serialized `command.txt` output, making it visually clear to developers that the prompt is being streamed in via `stdin`, avoiding confusion about missing prompt arguments.

## Verification

- Verified the `/files/read` endpoint using local UI fetches after stripping `/abs/path/`.
- Visually tested the blurred overlay and `RefreshCw` spinner behavior.
- Verified the `command.txt` accurately reflects the new layout logic and the frontend collapsible block renders it appropriately.

## Residual Notes

- The `/files/read` endpoint currently relies on the user restarting their `local-runner` to recompile the Go backend. Future iterations may want to build automatic version enforcement or backend reloading.
- The path matching logic is somewhat naive (`/abs/path/` and `file:///` stripping). We may want a more robust URI parser if the `local-runner` CLI alters its output format in the future.
