"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.COLLAPSIBLE_TOAST_THRESHOLD = void 0;
exports.shouldCollapseToasts = shouldCollapseToasts;
exports.buildToastGroupSummary = buildToastGroupSummary;
exports.COLLAPSIBLE_TOAST_THRESHOLD = 1;
const KIND_LABELS = {
    done: "Completed",
    approval: "Approvals",
    question: "Questions",
    blocked: "Paused",
};
function shouldCollapseToasts(count) {
    return count >= exports.COLLAPSIBLE_TOAST_THRESHOLD;
}
function buildToastGroupSummary(toasts) {
    const counts = {
        done: 0,
        approval: 0,
        question: 0,
        blocked: 0,
    };
    for (const toast of toasts) {
        counts[toast.kind] += 1;
    }
    return Object.keys(counts)
        .filter((kind) => counts[kind] > 0)
        .map((kind) => `${KIND_LABELS[kind]} ${counts[kind]}`)
        .join(" · ");
}
