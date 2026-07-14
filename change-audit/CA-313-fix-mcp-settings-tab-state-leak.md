# CA-313: Fix MCP Settings tab state leak (Jira / Firebase / Telegram)

## Context

Manual E2E for CP-05-04/05/06: Settings → switching between **Jira MCP**, **Firebase MCP**, and **Telegram MCP** showed the wrong create form and/or the wrong Existing Integrations list (e.g. Telegram create fields on Jira, Jira instances on Telegram).

## Root cause

`SettingsShell` renders the same component type for all three pages:

```tsx
<McpSettings mode="jira" | "firebase" | "telegram" />
```

React reuses the instance when only the `mode` prop changes. `McpSettings` only ran `refresh()` on mount (`useEffect([], …)`), so:

1. **Integrations / backends** stayed filtered for the previous tab’s type.
2. Editor state could flash or linger until a separate `[mode]` effect ran.
3. Input DOM nodes keyed only by `field.key` could reuse values across different field sets.
4. **Pending Telegram Approvals** gated on “any telegram integration in state”, so a stale telegram list could surface that panel on non-Telegram pages.

## Fix

1. **`SettingsShell`**: stable `key` per MCP section (`jira-mcp` / `firebase-mcp` / `telegram-mcp`) so each page remounts cleanly.
2. **`McpSettings`**: on `mode` change, reset editor + clear lists immediately, then `refresh(allowedTypes)` scoped to that mode; filter backends the same way; only fetch/show Telegram approvals on `telegram` or `all`; key form fields as `` `${type}:${field.key}` ``.

## Verification

- Open Settings → Jira MCP → form shows Jira fields; list only jira integrations.
- Switch Firebase → Firebase fields + firebase list only; no Jira rows.
- Switch Telegram → bot token / channel id fields + telegram list; approvals panel only here (when connected).
- Switch back to Jira → no Telegram form fields or Telegram-only panels.

## Source

- Live report while running `CP-05-04-05-06-Manual-E2E-Test` (Telegram OUTPUT E2E setup).

# ---8<--- flowpilot:change-ledger
feature_key: mcp-tools
source_doc_id: CP-05-04-05-06-Manual-E2E-Test
change_type: bugfix
summary: Remount + mode-scoped refresh so Jira/Firebase/Telegram MCP Settings tabs no longer leak form and integration list state
# --->8---
