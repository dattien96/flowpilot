# CA-159: Flow Settings List/Clone/Update Endpoints

## Scope

Backend half of `Task-179`: HTTP endpoints a future Settings "Flows" UI needs — list (built-ins + user flows), clone, and a scoped form-editable update — plus the `ListAll` capability the store contract was missing.

## Completed

- Added `ListAll(ctx) ([]FlowDefinitionRecord, error)` to the `FlowDefinitionStore` interface, implemented in all three backends: `FileFlowDefinitionStore` (refactored its directory scan into a shared `readAll` helper reused by `GetByPackFlow`), `SupabaseFlowDefinitionStore` (unfiltered `GET /flow_definitions?select=*`), and the test fake.
- `GET /client/flows` (`handleListFlows`): merges every built-in flow from the embedded pack (so the list is never empty, even before mirror sync has run) with whatever the configured `FlowDefinitionStore` has — mirrored built-ins and user-owned clones override the pack-only defaults by `flowRef`. Returns `flowSummaryDTO`, a deliberately small projection (ref, source, editable/cloneable flags, description, cap policy) rather than the full node/edge graph.
- `POST /client/flows/clone` (`handleCloneFlow`): thin wrapper over `FlowDefinitionResolver.CloneBuiltin`.
- `PUT /client/flows` (`handleUpdateFlow`): a scoped, form-editable update — description and the four cap-policy fields (`cap`/`onCap`/`extendBy`/`extendMax`), using pointer fields to distinguish "unchanged" from "set". This is deliberately not a raw full-`FlowDefinition` JSON overwrite endpoint: matching Task-179's own "form-based editor is faster for MVP than a full graph editor" allowance, and keeping the update surface auditable rather than accepting an arbitrary blob. Rejects edits to non-editable (built-in mirror) rows via `FlowDefinitionResolver.UpdateUserFlow`'s existing check (`403`).

## Explicitly not attempted — and why

**The actual React Settings UI component was not built.** Investigating `apps/desktop-flowpilot/src/components/settings/WorkflowsSettings.tsx` (the closest existing analog) revealed it runs on a completely different data layer than everything else touched this session: `getAdminUseCases()` from `@flowpilot/client-core`, not the `RunnerClient`/Zustand-store path the Chat Mode picker (CA-152/154/156/158) was built on. That's either a separate package shared with `apps/admin-web`, or a different backend entirely — this session has zero context on it. Guessing at that integration risks either producing code that's actively wrong (wrong data layer, wrong backend) or a disconnected parallel system that doesn't match the app's real conventions. The backend endpoints above are usable by whichever layer a real Task-179 implementation turns out to need; building the frontend blind was judged worse than stopping here and saying so.

## Verification

- `go test ./internal/runner -run 'TestFileFlowDefinitionStoreListAll|TestSupabaseFlowDefinitionStoreListAll|TestHandleListFlows|TestHandleCloneFlow|TestHandleUpdateFlow'` — 9 passed.
- Full suite: no new failures beyond the known pre-existing/flaky set.

## Follow-ups

- Explore `@flowpilot/client-core` / `getAdminUseCases()` (and whether it talks to `apps/admin-web` or local-runner) before attempting the actual Settings UI screens.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: Task-179
change_type: feature
summary: add flow list/clone/update HTTP endpoints and FlowDefinitionStore.ListAll for a future Settings flows UI
# --->8---
