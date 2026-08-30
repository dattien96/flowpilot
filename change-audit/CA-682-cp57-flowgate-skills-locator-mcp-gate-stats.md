# CA-682 — CP-57 DOD closure: opencode flow gates, skill exactness, session locator, agent catalog, MCP-ready gate, stats usage lines

## Problem

Remaining CP-57 DOD gaps after CA-681 (audit 2026-08-29):

- No test proved Opencode turns reach the shared `finishTurn` / flow-gate
  machinery (`r-ca` / `r-bug` / `r-task` — OC-02/OC-10).
- Skill injection exactness vs Claude (OC-08) unproven for the REAL live-registry
  prompt hook.
- `LocateSessionFile` opencode branch (Drive restore, E2E-23) and
  `.opencode/agents` catalog discovery (row 26) had zero tests.
- OC-17 MCP-ready-before-prompt gate existed in `SendTurn` but had no opencode
  test (BUG-114 class).
- `opencode stats` cost/token usage was deliberately skipped
  (`opencode_account.go:52-53`): live CLI takes seconds and early attempts
  dumped help text into usage lines.

## Change

- `opencode_account_stats.go` (new): bounded `opencode stats` (3s, degrade to
  no lines) + box-table parser (key/value share one cell, split on padding).
  Hooked into `loadOpencodeAccountMetadataInternal` on the cheap auth.json path
  only; lines end with the DOD-required "token limit: N/A — zen proxy" note.
- `CP-57` flow/locator/gate coverage below; no production behavior change
  besides the stats lines. Claude/Codex/Grok untouched.

## Tests (all additive)

- `opencode_flow_gate_test.go` — Opencode normal chat fires `r-ca` from a
  mapped `EventFileChanged` (status reprompt/warn, provider opencode); `r-bug`
  / `r-task` fire from the declared change type. Mirrors grok_flow_gate_test.go
  over the shared interactive HTTP surface (finishTurn reached).
- `opencode_skill_prompt_test.go` — live `ProviderRegistryFor` promptPrep must
  equal `injectSelectedSkills(ws, prompt, skills) + opencodeToolReinforcements`
  byte-for-byte; skill content present; prefix identical to the Claude hook's
  shared-injector body; no-selection → prompt + reinforcement only.
- `opencode_session_locator_agents_test.go` — three session layouts
  (config-dir `sessions/<id>.json`, `storage/session/<id>/store.json`,
  HOME `.local/share/opencode/sessions/<id>.jsonl`) + `thread-*`/missing guards;
  `.opencode/agents/oc-fixer.md` frontmatter parse (provider/model/effort/role/
  tools/system prompt).
- `opencode_mcp_ready_gate_test.go` — never-ready server: prompt withheld,
  no hang (700ms ctx); positive ordering: `session/prompt` only after
  `signalReady`; stats table parser fixture + garbage degrade.

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: CP-57
change_type: bugfix
summary: CP-57 DOD closure — opencode flow-gate, skill-exactness, session-locator, agent-catalog, MCP-ready-gate coverage and bounded opencode stats usage lines
# --->8---
