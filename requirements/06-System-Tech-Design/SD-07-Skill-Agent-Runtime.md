# FlowPilot Tech Design - Skill & Agent Runtime

This document translates `SS-06-Workflow-Skill-Agent` into technical implementation details.

## 1. File System Storage
- The Go-runner creates and manages hidden folders inside the project's `directory_path`: `.claude/`, `.codex/`, `.gemini/`.
- Built-in skills are synced from the backend/cloud to these folders.
- Custom skills are stored here as Markdown files (e.g., `SKILL.md`) and can be committed to Git.

## 2. Go-Runner Process Isolation
- When a step requires a heavy-duty Agent (like the Coder Agent), the main Go-runner spawns a separate background process (`os/exec.Command`) to isolate the execution.
- This allows the Coder Agent to autonomously compile code and run tests without blocking the main orchestrator loop.
- The sub-process streams its stdout/stderr back to the main runner to be logged in Supabase.

## 3. Syncing Custom Skills
- If the "Google Drive MCP" is enabled for the project, the Go-runner will execute a sync routine that uploads the contents of the `.claude/`, `.codex/`, and `.gemini/` skill folders to the configured Google Drive folder for backup and cross-project sharing.
