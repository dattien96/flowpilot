# FlowPilot Tech Design - Artifact Management

This document translates `SS-07-Workflow-Artifact` into technical implementation details.

## 1. UI/UX Design (Frontend - React)
- **Artifact Viewer:** A Markdown/JSON rendering component to view outputs (PRDs, Tech Specs).
- **History/Version Control:** A sidebar or dropdown to select previous versions of an artifact.
- **Annotations:** A commenting system allowing users to highlight text in the Artifact Viewer and save notes/decisions.

## 2. Database & Storage (Supabase)
- `artifacts` table: `id`, `workflow_run_id`, `step_id`, `version`, `content_url` (or raw content), `created_at`.
- `artifact_annotations` table: `id`, `artifact_id`, `user_id`, `highlight_range`, `note_text`.
- Supabase Storage Buckets will store the actual Markdown/JSON files to keep the Postgres DB lean.

## 3. Go-Runner File Sync
- The Go-runner automatically writes the artifact payload it receives from the LLM to an `.artifacts/` folder in the local project directory.
- **Storage Strategy:** Based on the `projects.artifact_storage_preference` setting (configured via the Admin UI):
  - **Default (`supabase`):** The Go-Runner uploads the file to the Supabase Storage Bucket via API.
  - **Google Drive (`google_drive`):** If the Google Drive MCP is enabled and connected for the project, the Go-Runner uploads the file to the specified Drive folder instead of Supabase.
  - The resulting `content_url` in the database will point to either the Supabase bucket or the Google Drive URL.
