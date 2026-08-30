"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.findMentionSpans = findMentionSpans;
const FILE_RE = /(?<![A-Za-z0-9_@])(?:[A-Za-z]:[\\/])?(?:[\w.-]+[\\/])+[\w.-]+\.[A-Za-z0-9]+/g;
const BRACKET_SKILL_RE = /\[([A-Za-z0-9_./-]+)\]/g;
function escapeRegExp(value) {
    return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}
function findMentionSpans(text, skillNames = []) {
    const spans = [];
    if (!text)
        return spans;
    for (const match of text.matchAll(new RegExp(FILE_RE, "g"))) {
        const raw = match[0];
        const start = match.index ?? 0;
        if (/^https?:/i.test(raw))
            continue;
        if (start >= 3 && text.slice(start - 3, start) === "://")
            continue;
        spans.push({ start, end: start + raw.length, kind: "file" });
    }
    for (const match of text.matchAll(new RegExp(BRACKET_SKILL_RE, "g"))) {
        const start = match.index ?? 0;
        spans.push({ start, end: start + match[0].length, kind: "skill" });
    }
    for (const name of skillNames) {
        const trimmed = name.trim();
        if (!trimmed)
            continue;
        const re = new RegExp(`(?<![A-Za-z0-9_-])${escapeRegExp(trimmed)}(?![A-Za-z0-9_-])`, "g");
        for (const match of text.matchAll(re)) {
            const start = match.index ?? 0;
            spans.push({ start, end: start + trimmed.length, kind: "skill" });
        }
    }
    spans.sort((a, b) => a.start - b.start || b.end - a.end);
    const out = [];
    let cursor = 0;
    for (const span of spans) {
        if (span.start < cursor)
            continue;
        out.push(span);
        cursor = span.end;
    }
    return out;
}
