---
id: CA-517
feature_key: cli-tui
title: Child [open] falls back to live stream when resume fails; restore main on total failure
date: 2026-08-15
status: COMPLETE
---

## Problem

After CA-516 (F2 steps render on `/open` of a completed flow), clicking
`[open]` on a sub-agent step in the F2 panel showed a red
`Open child transcript failed: … connection refused` and left the child chrome
stuck empty — only the main agent transcript was readable.

Root cause: `cmdFocusAgent` (introduced in CA-504) made `POST /resume` a hard
prerequisite for opening a child transcript:

1. `cmdFocusAgent` cleared `m.messages` / set `focusRunID` synchronously, then
   ran `ResumeRun` in the returned cmd.
2. Any resume error returned `focusStreamOpenedMsg{Err: …}` — there was **no
   fallback to the pre-CA-504 live-only `StreamLive` path**.
3. The `focusStreamOpenedMsg` Err branch only printed an error banner and kept
   `focusRunID` set with empty messages — the user was stuck in a broken child
   view with no way back to the main transcript in view.

The regression surfaced because a cold/completed child (e.g. `grok-review`
after the parent `/open`) may fail resume for a variety of reasons (runner
momentarily unreachable, session file not resolvable, run not found in the
persisted store), yet the child may still be live in the runner's in-memory
map — where the old `StreamLive` path could have served it. Before CA-504 the
TUI never hard-failed a child open; `StreamLive` on a not-found run simply
closed the channel and showed "(no transcript events for this agent yet)".

## Fix

`cmdFocusAgent` (agents_focus.go):

- **Resume-first stays** (CA-504): a successful `ResumeRun` still seeds durable
  transcript (turn log / Grok JSONL) into the runner event buffer, then tails
  `StreamLive` from `LastEventSeq`.
- **Transient dial retry**: `resumeChildRunForFocus` retries the resume POST
  once when the error is a dial-level failure (`runnerDialDeadErr`: connection
  refused / reset / dial tcp / no such host) — covers supervisor restart / port
  handoff without dropping the seed.
- **Server-side resume failure falls back**: when resume fails with a
  non-dial error (run_not_found, session_unavailable, …), `cmdFocusAgent` no
  longer returns `Err`. It falls back to the pre-CA-504 `StreamLive` path and
  returns a soft `Fallback` note — an in-memory child still renders its
  transcript (run-193749).
- **Dial failure stays total**: if resume fails at the dial level (runner
  genuinely unreachable), `Err` is returned because the live stream would fail
  the same way.

`focusStreamOpenedMsg` handler (app.go):

- `Err` now calls `restoreMainTranscript()` before printing the error banner,
  so a total failure never leaves a stuck empty child chrome.
- `Fallback` renders as a step-note when the fallback stream yields no seeded
  messages, replacing the bare "(no transcript events for this agent yet)".

## Provider impact

Agnostic. The focus/resume/fallback logic branches on transport error class and
run kind only — no provider adapter or SSE path touched. Claude / Codex / Grok
children share the same open path; tests run all three providers.

## Tests

`tui_child_open_resume_fallback_test.go` (additive, new file):

- Resume success seeds transcript + tails stream for claude/codex/grok
  (CA-504 preserved).
- Resume non-dial failure falls back to live stream with a Fallback note —
  no `Err` (run-193749 regression).
- Resume dial failure is retried once and a healthy second attempt seeds
  history.
- Dial-level total failure → `Err` → handler restores the main transcript and
  clears `focusRunID` (no stuck child chrome).
- `focusStreamOpenedMsg{Err}` handler contract restores main + error banner.
- `focusStreamOpenedMsg{Fallback}` renders the fallback note.
- `resumeChildRunForFocus` does not retry non-dial (404) errors.

Legacy suite untouched and green (`go test ./internal/tui/... ./internal/cli/...`,
build + vet clean).

## Residual / out of scope

- Runner side: no panic-recovery middleware wraps the runner `mux`, so a panic
  inside a child `resumeRun` / Grok transcript seed can still kill the whole
  process (a plausible source of the recurring `connection refused`). Out of
  scope per operator: this change is TUI-only. The runner logs should be
  checked if the dial failure recurs.
- `RunSnapshot` still carries no runKind/workflowId/flowRef (unchanged; flow
  chrome restore relies on the resume handle + history meta, CA-502).

# ---8<--- flowpilot:change-ledger
feature_key: cli-tui
source_doc_id: CP-56
change_type: bugfix
summary: Child [open] no longer hard-fails on resume error: retry dial failures once, fall back to pre-CA-504 live-only StreamLive on server-side resume failure, and restore the main transcript on total dial failure instead of leaving a stuck empty child chrome
# --->8---