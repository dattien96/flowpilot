# CA-038: Fix Terminal Thinking Stream Rendering

## Scope

- Workflow run detail thinking panel and Run Logs rendering in `apps/admin-web/src/routes/_authenticated/workflow-runs/$runId.tsx`.
- Provider stream transcript normalization in `apps/admin-web/src/features/workflow-engine/workflow-run-thinking.ts`.
- Focused regression coverage in `apps/admin-web/src/features/workflow-engine/workflow-run-thinking.test.ts`.

## Completed

- Reconstructed buffered provider chunks before parsing so JSON protocol envelopes split across database log rows can be handled as one stream.
- Extracted only nested provider `content` text from concatenated JSON protocol events instead of exposing raw JSON-RPC payloads in the thinking panel.
- Reused the same display-text normalization for provider rows in the Run Logs panel.
- Preserved ordinary non-JSON terminal stdout and stderr text.
- Marked the latest prompt in an active running session as live without relying on the absence of output attempts, keeping the rolling three-line thinking window active until step completion.
- Added a collapsible `SKILL AUDIT` panel below each thinking panel. It extracts operational reads of `.../skills/<name>/SKILL.md` files, named `*-skill` actions such as `Read git-commit-skill` or `opening the local git-commit-skill instructions`, and clear skill-use announcements from the normalized transcript. It ignores available-skill catalogs, deduplicates detected calls, preserves call order, and shows a live waiting state before the first detected skill call.
- Scoped thinking and skill-audit transcripts to the next prompt across the entire selected step, including prompts in later replay sessions. This prevents completed prompts from inheriting skills used only by a follow-up session.
- Kept `SKILL AUDIT` visible for every prompt, including completed prompts with no detected skill calls. Empty completed audits render a `0` count and an explicit empty-state message when expanded.

## Verification

- Passed `npm test -- src/features/workflow-engine/workflow-run-thinking.test.ts src/features/workflow-engine/workflow-run-detail-timeline.test.ts src/features/workflow-engine/workflow-run-log-fallback.test.ts` from `apps/admin-web` with 23 passing tests.
- Passed `npm run build` from `apps/admin-web`.
- Passed `git diff --check`.
- Ran GitNexus impact analysis before edits and `gitnexus_detect_changes()` after edits. The final change report identified the expected workflow-run detail page scope with MEDIUM risk and no HIGH or CRITICAL warnings.

## Residual Notes

- Targeted ESLint could not start because the workspace is missing the `eslint-config-next` package imported by `apps/admin-web/eslint.config.mjs`.
- The production build retains existing browser-compatibility warnings for `node:path` and `node:fs`, plus the existing large-chunk warning.

# ---8<--- flowpilot:change-ledger
feature_key: terminal-session
source_doc_id: CA-038
change_type: fix
summary: Fix Terminal Thinking Stream Rendering
# --->8---
