# CA-718 — Scan/plan posture auto-approves the FlowPilot `ask_user` gate (question, not a write)

# ---8<--- flowpilot:change-ledger
feature_key: mcp-tools
source_doc_id: BUG-344
change_type: bugfix
summary: approve the read-only posture decision for the FlowPilot ask_user tool-call gate (Grok's gated use_tool / MCP mcp__flowpilot__ask_user) so the AskQuestion card renders in scan/plan instead of a fail-close deny; spawn_agent, native ask_user_question, and other MCP/writes still deny
# --->8---

## Problem

- Live `run-477800` (gate-sandbox, Grok `grok-4.5`, scan posture, prompt
  `test B4-inplace, toi la Nam`): the compound bash exploration is now
  APPROVED (BUG-344 working), but the model's `flowpilot__ask_user` question
  was auto-denied by the posture bridge (`posture_read_only_scan` →
  `reject-once`). Grok routes the MCP `ask_user` as a gated `use_tool`
  request whose `grokPermissionKind` maps to Kind `"other"`; the shared
  read-only classifier then fails closed (`mcp/other` → deny). The question
  card never rendered and the turn ended without the B4-inplace decision.
- Same deny existed on `run-464841` (`appr-464986`) — this is a second hole
  in the same classifier, not a regression from recent commits.
- The classifier (`readOnlyApprovalDecision`) is the single choke point;
  recent BUG-344/CA-715+717 work only widened the `exec` path and never
  covered the `ask_user` tool-call gate.

## What changed (`chat_posture_policy.go`, additive tests)

- `readOnlyApprovalDecision` approves when `isAskUserTool(details)` before the
  Kind switch. Safe by construction: approving the gate only lets the MCP
  `AskQuestion` bridge render the card — it still blocks for the user's
  answer, so it can never auto-answer a question.
- `isAskUserTool` matches `ask_user`, `*__ask_user`, `mcp__flowpilot__ask_user`
  on Command/Reason. Deliberately NOT matching the native Grok
  `ask_user_question` — the reinforcement steers onto the FlowPilot MCP tool,
  and the native one must keep being denied in scan.
- No change to `isReadOnlyCommand` / exec allowlist; `spawn_agent`, generic
  MCP, and writes keep failing closed.

## R1 — old-suite regression evidence

- All legacy posture tests PASS unchanged (`TestReadOnlyApprovalDecision_*`,
  `TestIsReadOnlyToolName_*`, `TestBug344*`).
- Full targeted run (`Posture|Grok|MCP|Yolo|Bug341..344`): `ok 40.7s`, zero
  failures.
- Additive only — no existing assertion edited.

## R2 — provider parity

- The classifier is shared across Claude/Codex/Grok (`ApprovalDetails` only).
  The approve matrix covers the live Grok `use_tool` shape plus the MCP
  `mcp__flowpilot__ask_user` and bare `ask_user` tool-name shapes, so a
  provider that routes `ask_user` through `RequestApproval` gets the same
  decision.

## R3 — new coverage (`chat_posture_policy_test.go`, additive)

- **Approve:** `{Kind:other, Command:flowpilot__ask_user, Reason:use_tool}`
  (exact run-477800/run-464841 wire), bare `ask_user`, `mcp__flowpilot__ask_user`
  via mcp-kind, `ask_user` as Reason.
- **Deny:** `flowpilot__spawn_agent`, native `ask_user_question`, generic
  `mcp_thing`/`some_random_mcp`, and a false-positive guard `echo x >
  ask_user.txt` (exec write whose arg contains the name).

## Honest gaps

- Live rerun of run-477800 post-fix still pending (needs the desktop app +
  Grok): expected outcome is the compound read bash approved + the B4-inplace
  `ask_user` question card rendered, no deny.
- No dedicated BUG doc filed yet — `source_doc_id` points at BUG-344 (the
  classifier this extends); the deny→card path should be captured in a BUG
  doc if it recurs on another provider shape.

## Prior CA not undone

- CA-715/CA-717 (BUG-344 compositional exec + `2>&1`/go/curl) intact — exec
  path untouched.
- CA-713/CA-716 (no-reply notices) intact — a genuine deny still renders the
  notice; this change removes the false-positive deny for `ask_user`.
- CA-714 (BUG-343 always-ask env) intact.