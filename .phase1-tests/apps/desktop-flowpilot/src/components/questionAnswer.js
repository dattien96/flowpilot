"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.resolveQuestionManualSubmit = resolveQuestionManualSubmit;
function resolveQuestionManualSubmit(selected, other, multiSelect) {
    const typed = other.trim();
    if (!multiSelect) {
        return typed || undefined;
    }
    const picks = [...selected];
    if (typed)
        picks.push(typed);
    return picks.length > 0 ? picks : undefined;
}
