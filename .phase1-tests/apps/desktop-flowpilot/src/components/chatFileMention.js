"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.findActiveAt = findActiveAt;
exports.isAgentAtMention = isAgentAtMention;
exports.filterWorkspaceFiles = filterWorkspaceFiles;
exports.insertAtMention = insertAtMention;
function findActiveAt(text, cursor) {
    for (let i = cursor - 1; i >= 0; i--) {
        if (text[i] === "@") {
            if (i === 0 || text[i - 1] === " " || text[i - 1] === "\n") {
                return { index: i, query: text.slice(i + 1, cursor) };
            }
            return null;
        }
        if (text[i] === " " || text[i] === "\n")
            return null;
    }
    return null;
}
function isAgentAtMention(fragment, agentNames) {
    if (fragment.index !== 0)
        return false;
    if (/[./\\]/.test(fragment.query))
        return false;
    const name = fragment.query.toLowerCase();
    if (name.length === 0)
        return false;
    return agentNames.some((agent) => agent.toLowerCase() === name);
}
function filterWorkspaceFiles(paths, query, limit = 40) {
    const q = query.trim().toLowerCase().replace(/\\/g, "/");
    const matched = q.length === 0
        ? paths
        : paths.filter((path) => path.toLowerCase().replace(/\\/g, "/").includes(q));
    return matched.slice(0, limit);
}
function insertAtMention(text, fragment, cursor, path) {
    const next = text.slice(0, fragment.index) + path + text.slice(cursor);
    return { text: next, cursor: fragment.index + path.length };
}
