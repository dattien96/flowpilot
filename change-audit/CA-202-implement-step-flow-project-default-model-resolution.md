# CA-202: Implement Step > Flow > Project > Default Model Resolution

## Summary

Fixed `BUG-165`, a follow-up to `BUG-164`: workflow/step-mode runs never resolved a model at all (the desktop client sends none, and `createRun` had no fallback), so execution silently fell back to whatever the underlying CLI defaulted to instead of FlowPilot's project/workflow/step configuration. Implemented real Step > Flow > Project > default resolution in the Go runner; `normal_chat` is unaffected and keeps using the explicitly-selected model.

## What Changed

- `apps/local-runner/internal/runner/provider_event.go`: `Project`/`Workflow`/`Step` gain a `Model` field.
- `apps/local-runner/internal/runner/supabase_catalog_store.go`: `ListProjects`/`ListWorkflows`/`ListSteps`/`ListWorkflowSteps` extended to select/embed `default_model`/`model_override`/`model` respectively.
- `apps/local-runner/internal/runner/interactive_handlers.go`: `createRun` resolves `resolvedModel` once at run start for workflow/step-mode runs (Step > Flow > Project > `"gpt-5.4"`); chat-mode runs pass `in.Model` through unchanged.
- `apps/local-runner/internal/runner/workflow_model_resolution_test.go` (new): 5 tests covering each tier winning and the chat-mode exemption.
- `apps/local-runner/internal/runner/partd_test.go`: updated for the new catalog select columns.
- `requirements/05-System-Specs/SS-05-Workflow-Ai-Provider.md` §1.2/§2.2/§3, `requirements/06-System-Tech-Design/SD-06-AI-Provider-Integration.md` §6/§7/§8: updated to describe the corrected architecture and the new resolution logic, including an explicit "known limitation" note that this resolves once at run start, not per-step during execution.

## Verification

- `go build ./...` + `go test ./internal/runner/...` in `apps/local-runner` — pass (same 16 pre-existing unrelated failures as before this change, none touching this code).
- `npm run typecheck` in `apps/desktop-flowpilot` — clean (no frontend changes needed; confirmed `ChatInput.tsx:737` already hides the chat controller outside `normal_chat`).
- Not verified live — no backend/Supabase in this environment, flagged in `BUG-165` (`V-6`).

## Notes

- Direct user follow-up after discovering (during `BUG-164`'s investigation) that workflow-mode runs never actually resolved a model at all.
- Genuine per-step re-resolution during a single run's execution (not just once at start) remains unimplemented — explicitly documented as a known limitation, not silently glossed over.

# ---8<--- flowpilot:change-ledger
feature_key: agent-flow-engine
source_doc_id: BUG-165
change_type: feature
summary: Resolve a workflow/step-mode run's model via Step > Flow > Project > default at run start instead of never resolving one at all, and update SS-05/SD-06 to match
# --->8---
