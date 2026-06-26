# CA-100 — Delete Chat History

## Scope

Added the ability for users to delete individual chat history entries from the Navigator sidebar. A deletion removes:

1. The FlowPilot NDJSON record in `sessions.ndjson` (atomic temp-file + rename rewrite).
2. The in-memory `interactiveRun` entry in `InteractiveService`.
3. The actual provider session file(s) on disk — searched across **all registered accounts of the matching provider** because cross-account copy/relocation (CA-098) can place the file in more than one account home directory.

A chat run belongs to exactly one provider (Claude XOR Codex) — the delete never cross-searches providers.

## Completed

**Go — local runner (`apps/local-runner/internal/runner/`):**

- `workflow_store.go`: Added `DeleteProviderSession(ctx, runID) error` to `InteractiveStateStore` interface and implemented it on `fakeWorkflowStore`.
- `local_file_session_store.go`: Added `DeleteProviderSession` — removes from in-memory map, atomically rewrites `sessions.ndjson` without the deleted run_id (write-tmp + `os.Rename`). Added `errors` import.
- `interactive_resume.go`: Added `deleteChatSession(runID) *apiErr` — resolves session from store or in-memory map, iterates all provider accounts matching `ProviderKey`, calls `LocateSessionFile` + `os.Remove` for each found path (best-effort), removes from `s.runs`, calls `DeleteProviderSession` on the store.
- `interactive_handlers.go`: Registered `DELETE /client/workflow-runs/{runId}` → `handleDeleteRun`; returns `{"status":"deleted"}`.

**Desktop (`apps/desktop-flowpilot/src/`):**

- `types/contract.ts`: Added `deleteRun(runId: string): Promise<void>` to `RunnerClient`.
- `client/HttpWsRunnerClient.ts`: Implemented `deleteRun` via `DELETE /client/workflow-runs/{runId}`.
- `client/MockRunnerClient.ts`: No-op stub `deleteRun`.
- `state/store.ts`: Added `deleteHistoryRun(runId)` — optimistic local removal, API call, re-fetch history on failure.
- `components/Navigator.tsx`: Per-row `×` delete button (visible on hover), inline confirm (`✓` / `✕`) before calling `deleteHistoryRun`. New state: `pendingDeleteId`, `deletingIds`.
- `styles.css`: Added `.project-history-delete-icon`, `.project-history-delete-confirm`, `.project-history-delete-cancel`.
- `state/store.test.ts`: Added `deleteRun: async () => {}` to inline mock to satisfy updated interface.

## Verification

- `go build ./internal/runner/...` — clean (no errors).
- `npx tsc --noEmit` on `desktop-flowpilot` — clean (no errors).
- Manual UI verification not run (no browser dev server available in this session).

## Residual Notes

- **Drive copy not deleted.** If a session was synced to Google Drive (`syncStatus: "synced"`), the Drive-side manifest and provider file under `chat-sessions/runs/<machine>/<runId>/` are untouched. Follow-up task needed.
- **Supabase path.** `SupabaseWorkflowStore` does not implement `DeleteProviderSession`. When Supabase is configured the NDJSON rewrite is bypassed — a follow-up should add a Supabase delete path (SQL `DELETE FROM workflow_sessions WHERE workflow_run_id = $1`).
- **No undo.** Delete is immediate and permanent for both the local record and the provider session file.
- **Cross-provider safety.** `deleteChatSession` uses the `ProviderKey` stored on the session record to restrict account scanning — a Claude session never touches Codex directories and vice versa.

# ---8<--- flowpilot:change-ledger
feature_key: chat-history
source_doc_id: CA-100
change_type: feature
summary: Delete Chat History
# --->8---
