"use strict";
// Chat history grouping (CP-59 Task-316 DOD-5, SD-26 Q-4): the navigator shows
// ONE entry per logical chat; provider-switch legs collapse under the chat
// head (latest leg) with a leg count. Untagged legacy items stay 1:1.
Object.defineProperty(exports, "__esModule", { value: true });
exports.groupRunsByChatId = groupRunsByChatId;
exports.flattenGroupedHistory = flattenGroupedHistory;
/**
 * Groups consecutive chat runs by chatId (newest first). Runs without a chatId
 * (pre-CP-59 or workflow runs) pass through 1:1. Order is preserved: each
 * group takes the position of its newest leg.
 */
function groupRunsByChatId(items) {
    const rows = [];
    const indexByChat = new Map();
    for (const item of items) {
        const chatId = item.chatId;
        if (!chatId) {
            rows.push({ item });
            continue;
        }
        const existing = indexByChat.get(chatId);
        if (existing !== undefined) {
            const group = rows[existing].group;
            if (group) {
                group.legs.push(item);
                if (compareNewer(item, group.head)) {
                    // The newer leg becomes the face of the chat; keep the group slot.
                    group.head = item;
                    rows[existing] = { item, group };
                }
            }
            continue;
        }
        const group = { head: item, legs: [item] };
        indexByChat.set(chatId, rows.length);
        rows.push({ item, group });
    }
    return rows;
}
function compareNewer(a, b) {
    const la = a.legSeq ?? 0;
    const lb = b.legSeq ?? 0;
    if (la !== lb)
        return la > lb;
    return (a.updatedAt || "") > (b.updatedAt || "");
}
/** Flattens grouped rows back to renderable items with an optional leg count. */
function flattenGroupedHistory(rows) {
    return rows.map((row) => row.group ? { ...row.item, legsCount: row.group.legs.length } : row.item);
}
