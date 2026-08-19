# CA-559: clamp user prompt bubbles to 4 lines with click-to-expand (Desktop + TUI)

## Fix

User prompt bubbles in the chat timeline now cap at 4 wrapped lines with a `....`
tail. Clicking the whole bubble card toggles between the clamped and full text;
no extra button or hint text is rendered. Applies to both Desktop (`bubble.prompt`)
and TUI (boxed `Role=="user"`) transcripts.

Desktop: `Timeline.tsx` `PromptCard` measures overflow with a `ResizeObserver`,
applies `-webkit-line-clamp: 4` when collapsed, shows a `....` badge, and toggles
on card click (copy button and skills summary are excluded). `styles.css` adds
`.prompt-truncatable`, `.prompt-text-clamped`, `.prompt-ellipsis`.

TUI: `chatRows` truncates boxed user messages to 4 wrapped lines and appends a
`....` tail; `expandedUserPrompts` (content-keyed, like `expandedToolGroups`)
tracks per-bubble state, `chatRow.PromptExpandKey` marks clickable rows,
`strokeChatRows` preserves it, and `hitUserPromptChrome` (after `hitCopyChrome`)
maps box clicks to a `user-prompt-expand:` toggle. Copy still wins over expand.

# ---8<--- flowpilot:change-ledger
feature_key: chat-ui
source_doc_id: CP-56
change_type: feature
summary: clamp user prompt bubbles to 4 lines with a '....' tail and click-to-expand/collapse on the whole card, for both Desktop and TUI
# --->8---