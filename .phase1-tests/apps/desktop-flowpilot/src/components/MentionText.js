"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.MentionText = MentionText;
const jsx_runtime_1 = require("react/jsx-runtime");
const mentionHighlight_1 = require("@/components/mentionHighlight");
function MentionText({ text, skillNames = [], }) {
    const spans = (0, mentionHighlight_1.findMentionSpans)(text, skillNames);
    if (spans.length === 0)
        return (0, jsx_runtime_1.jsx)(jsx_runtime_1.Fragment, { children: text });
    const parts = [];
    let cursor = 0;
    spans.forEach((span, i) => {
        if (span.start > cursor)
            parts.push(text.slice(cursor, span.start));
        parts.push((0, jsx_runtime_1.jsx)("mark", { className: `mention-token mention-${span.kind}`, children: text.slice(span.start, span.end) }, `${span.kind}-${span.start}-${i}`));
        cursor = span.end;
    });
    if (cursor < text.length)
        parts.push(text.slice(cursor));
    return (0, jsx_runtime_1.jsx)(jsx_runtime_1.Fragment, { children: parts });
}
