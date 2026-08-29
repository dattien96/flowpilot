# CA-681 — CP-57 DOD gap closure: opencode YOLO route, approval/YOLO/summarizer/install/MCP/tooling tests

## Problem

CP-57 DOD audit (2026-08-29) found landed-but-unverified surfaces:

- `ApplyOpencodeYoloPosture` had zero production callers — the desktop YOLO
  toggle only called the Grok endpoint, so the runner-side auto SSOT
  (`opencodeDesiredAuto`) was unreachable over HTTP.
- Adapter-level approval channel untested: deny path, YOLO auto-approve,
  `question`-never-auto-approved (spec Q-3), missing-bridge fail-closed.
- Summarizer cheap tier (`opencode/gpt-5.4-nano`), one-shot
  `opencode run --format json` prompt-execution adapter, opencode install
  command, `opencode.json` MCP one-click writer, and `CheckTool("opencode")`:
  complete code, zero tests.

## Fix / Change

- `cli/root.go`: new `POST /provider-accounts/opencode-yolo-posture` route →
  `ApplyOpencodeYoloPosture` (mirrors the Grok toggle endpoint). Desktop/TUI
  stay on the CP-57 §5.2 row-13 "sync per-turn default" — per-turn YOLO already
  rides `TurnRequest.YoloMode` through the adapter permission auto-approve.
- `CP-57` doc: synthetic `thread-*` spec reconciled (2026-08-29) to shipped
  Grok-BUG-324-parity semantics — never `session/load`; a fresh session starts;
  cross-account mismatch still refuses via the run-owner guard.

## Tests (all additive; no pre-existing test edited)

- `opencode_yolo_approval_test.go` — posture flips auto + closes live processes;
  deny → bridge + `reject` optionId with raw command/kind; YOLO auto-approves
  tool permissions without the bridge; `question` never auto-approved; missing
  bridge fails closed.
- `opencode_summarizer_promptexec_test.go` — cheap-tier model + claude/gemini/
  codex/grok parity; one-shot args (`--model/--variant/--auto`, omission rules);
  `opencode/`+`opencode-go/` prefix wins over supplied provider; codex/grok
  branch shape drift guards.
- `opencode_install_mcp_test.go` — install command `npm i -g
  opencode-ai@latest`; `opencode.json` MCP one-click write (google-drive + jira,
  stdio shape, `CheckOpencodeMcpConfig`, idempotent no-duplicate rewrite).
- `tooling/tooling_opencode_check_test.go` — `CheckTool("opencode")` ok via
  `FLOWPILOT_OPENCODE_BIN` fake + missing-binary path (OC-18).

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: CP-57
change_type: bugfix
summary: CP-57 DOD gap closure — opencode YOLO posture route plus approval, summarizer, prompt-exec, install, MCP and tooling tests; thread-* spec reconciled
# --->8---
