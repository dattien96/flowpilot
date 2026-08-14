---
id: CA-504
feature_key: cli-tui
title: Highlight focused agent; resume child transcript on /agent open
date: 2026-08-14
status: COMPLETE
---

## Change

1. **Status agents chip**: focused agent (viewing child or main) is accent-bold
   (`styleStatusHi` + `*`); status line0 no longer re-wraps the whole row so
   nested highlight survives.

2. **Empty child transcript (run-98158)**: `/agent` / step open only called
   `StreamLive` without `ResumeRun`, so cold children had an empty event buffer.
   `cmdFocusAgent` now ResumeRun → seed → collect history →
   `replayChildHistoryMessages` → StreamLive from lastSeq. Spawn prompts are
   reduced to the human task line via `childFacingUserPrompt`.

Does not undo CA-503 hydrate agents, CA-502 open flow chrome, CA-501 seed filter.

## Provider impact

Case 1 (agnostic). Resume + SSE replay path shared by all providers.

## Tests

New `tui_agent_focus_transcript_test.go`. Old tests untouched.

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: Highlight focused agent chip; ResumeRun+replay child transcript on /agent open
# --->8---
