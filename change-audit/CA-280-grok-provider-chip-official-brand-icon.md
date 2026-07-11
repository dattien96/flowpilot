# CA-280: Grok Provider Chip Official Brand Icon

## Scope

Replaces the placeholder lettermark `GrokIcon` in the desktop chat provider picker with the official Grok/xAI swirl mark (Wikimedia Commons `Grok-icon.svg` paths), matching the Grok tab favicon the user referenced.

## Changes

- `apps/desktop-flowpilot/src/components/ChatInput.tsx`: `GrokIcon` inline SVG updated to the official swirl path; still uses `currentColor` + `--grok-brand` chip styling.

## Verification

- `npm run typecheck` in `apps/desktop-flowpilot`: PASS.

# ---8<--- flowpilot:change-ledger
feature_key: chat-ui
source_doc_id: Task-211
change_type: feature
summary: Grok provider chip uses official xAI swirl icon instead of placeholder lettermark
# --->8---