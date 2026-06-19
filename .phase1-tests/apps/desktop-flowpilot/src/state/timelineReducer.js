"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.shouldApplyRunEvent = shouldApplyRunEvent;
exports.applyTimelineEvent = applyTimelineEvent;
function shouldApplyRunEvent(activeRunId, streamRunId) {
    return activeRunId === streamRunId;
}
function statusFromEvent(e, prev) {
    switch (e.type) {
        case "turn_started":
            return "running";
        case "permission_required":
            return "waiting_approval";
        case "user_question_required":
            return "waiting_question";
        case "turn_completed":
            return "completed";
        case "turn_failed":
            return "failed";
        default:
            return prev === "waiting_approval" || prev === "waiting_question" ? "running" : prev;
    }
}
function isTurnCompletedPlaceholder(text) {
    return text.trim().toLowerCase().replace(/\.$/, "") === "turn completed";
}
function hasPendingPrompt(timeline, prompt) {
    for (let i = timeline.length - 1; i >= 0; i--) {
        const item = timeline[i];
        if (item.kind === "thinking")
            continue;
        return item.kind === "prompt" && item.text === prompt;
    }
    return false;
}
function applyTimelineEvent(s, e) {
    const status = statusFromEvent(e, s.status);
    const thinkingItem = s.timeline.find((it) => it.kind === "thinking");
    const shouldKeepThinking = e.type !== "turn_completed" &&
        e.type !== "turn_failed" &&
        e.type !== "permission_required" &&
        e.type !== "user_question_required";
    const timeline = s.timeline.filter((it) => it.kind !== "thinking");
    let streamingAssistantId = s._streamingAssistantId;
    // Stale pending-interaction detection (BUG-074): in live runs, approve() and
    // answer() clear pendingApproval/pendingQuestion synchronously before the server
    // sends any follow-up event, so these are undefined by the time the next event
    // arrives — the blocks below are no-ops during normal execution.
    //
    // During history replay the resolution was client-side only; no resolution event
    // exists in the stream. The first event that arrives after permission_required /
    // user_question_required signals that the interaction was resolved. Stamp the
    // card so it renders as resolved instead of re-showing the buttons.
    //
    // No type guards are needed: when a NEW permission_required or user_question_required
    // fires and sets a new pending value via `...extra`, it overrides the `undefined`
    // spread by finalize — so the net result is always correct. (BUG-074)
    const staleApproval = s.pendingApproval;
    if (staleApproval) {
        for (let i = timeline.length - 1; i >= 0; i--) {
            const it = timeline[i];
            if (it.kind === "approval" && it.approvalId === staleApproval.approvalId && it.decision === undefined) {
                timeline[i] = { ...it, decision: "resolved" };
                break;
            }
        }
    }
    const staleQuestion = s.pendingQuestion;
    if (staleQuestion) {
        for (let i = timeline.length - 1; i >= 0; i--) {
            const it = timeline[i];
            if (it.kind === "question" && it.questionId === staleQuestion.questionId && it.answer === undefined) {
                timeline[i] = { ...it, answer: "answered" };
                break;
            }
        }
    }
    const closeAssistant = () => {
        streamingAssistantId = undefined;
    };
    const finalize = (nextTimeline, extra = {}) => ({
        timeline: shouldKeepThinking
            ? [
                ...nextTimeline,
                thinkingItem ?? { kind: "thinking", id: `thinking-${nextTimeline.length}`, text: "Thinking..." },
            ]
            : nextTimeline,
        status,
        _streamingAssistantId: streamingAssistantId,
        ...(staleApproval ? { pendingApproval: undefined } : {}),
        ...(staleQuestion ? { pendingQuestion: undefined } : {}),
        ...extra,
    });
    switch (e.type) {
        case "turn_started":
            if (e.prompt && !hasPendingPrompt(timeline, e.prompt)) {
                timeline.push({ kind: "prompt", id: `prompt-${e.providerTurnId}`, text: e.prompt });
            }
            break;
        case "message_delta": {
            if (streamingAssistantId) {
                const idx = timeline.findIndex((it) => it.id === streamingAssistantId);
                if (idx >= 0 && timeline[idx].kind === "assistant") {
                    const cur = timeline[idx];
                    timeline[idx] = { ...cur, text: cur.text + e.text };
                }
            }
            else {
                const id = e.id;
                streamingAssistantId = id;
                timeline.push({ kind: "assistant", id, text: e.text, finalized: false });
            }
            break;
        }
        case "message_completed": {
            if (isTurnCompletedPlaceholder(e.text)) {
                closeAssistant();
                break;
            }
            if (streamingAssistantId) {
                const idx = timeline.findIndex((it) => it.id === streamingAssistantId);
                if (idx >= 0 && timeline[idx].kind === "assistant") {
                    timeline[idx] = { kind: "assistant", id: streamingAssistantId, text: e.text, finalized: true };
                }
            }
            else {
                timeline.push({ kind: "assistant", id: e.id, text: e.text, finalized: true });
            }
            closeAssistant();
            break;
        }
        case "tool_started":
            closeAssistant();
            timeline.push({ kind: "tool", id: e.id, toolName: e.toolName, status: "running", input: e.input });
            break;
        case "tool_completed": {
            closeAssistant();
            for (let i = timeline.length - 1; i >= 0; i--) {
                const it = timeline[i];
                if (it.kind === "tool" && it.toolName === e.toolName && it.status === "running") {
                    timeline[i] = { ...it, status: e.status, output: e.output };
                    return finalize(timeline);
                }
            }
            timeline.push({ kind: "tool", id: e.id, toolName: e.toolName, status: e.status, output: e.output });
            break;
        }
        case "file_changed":
            closeAssistant();
            timeline.push({ kind: "file", id: e.id, path: e.path, changeType: e.changeType });
            break;
        case "permission_required":
            closeAssistant();
            timeline.push({ kind: "approval", id: e.id, approvalId: e.approvalId, details: e.details });
            return finalize(timeline, { pendingApproval: { approvalId: e.approvalId, details: e.details } });
        case "user_question_required":
            closeAssistant();
            timeline.push({
                kind: "question",
                id: e.id,
                questionId: e.questionId,
                prompt: e.prompt,
                options: e.options,
                multiSelect: e.multiSelect,
            });
            return finalize(timeline, {
                pendingQuestion: { questionId: e.questionId, prompt: e.prompt, options: e.options, multiSelect: e.multiSelect },
            });
        case "turn_completed":
            closeAssistant();
            return finalize(timeline);
        case "turn_failed":
            closeAssistant();
            if (e.error !== "interrupted by user") {
                timeline.push({ kind: "system", id: e.id, text: e.error, tone: "error" });
            }
            return finalize(timeline, { recoverable: e.recoverable });
    }
    return finalize(timeline);
}
