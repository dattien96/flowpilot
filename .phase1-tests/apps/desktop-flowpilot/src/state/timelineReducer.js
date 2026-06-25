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
        case "flow_gate_violation":
            // A block/warn gate is terminal: settle so the chat input unblocks instead of
            // appearing to load forever (status === "running" disables the composer). A
            // reprompt is immediately followed by a turn_started, so keep the prior status
            // and let that event flip back to running. (CP-35)
            return e.status === "reprompt" ? prev : "completed";
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
    // Annotation events (BUG-121): only preserve an existing thinking row — never create one.
    let shouldKeepThinking = e.type !== "turn_completed" &&
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
            closeAssistant();
            if (e.prompt) {
                // Idempotent guard (BUG-111): the prompt id is stable across replays
                // (derived from providerTurnId), so a re-delivered turn_started — e.g. when
                // switching chats in the history panel re-streams the run from seq 0 — must
                // not push a second copy of a prompt already in the timeline.
                const promptId = `prompt-${e.providerTurnId}`;
                const alreadyPresent = timeline.some((it) => it.kind === "prompt" && it.id === promptId);
                if (!alreadyPresent && !hasPendingPrompt(timeline, e.prompt)) {
                    timeline.push({ kind: "prompt", id: promptId, text: e.prompt });
                }
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
                // Idempotent re-stream guard (BUG-111): if a bubble with this event id already
                // exists, the message is being re-delivered (chat switch / replay from seq 0).
                // Resume that bubble and reset its text so the re-stream rebuilds it in place
                // instead of pushing a duplicate assistant bubble.
                const id = e.id;
                streamingAssistantId = id;
                const existingIdx = timeline.findIndex((it) => it.kind === "assistant" && it.id === id);
                if (existingIdx >= 0) {
                    timeline[existingIdx] = { kind: "assistant", id, text: e.text, finalized: false };
                }
                else {
                    timeline.push({ kind: "assistant", id, text: e.text, finalized: false });
                }
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
                // Idempotent guard (BUG-111): update an existing bubble with this id in place
                // rather than pushing a duplicate on re-delivery.
                const existingIdx = timeline.findIndex((it) => it.kind === "assistant" && it.id === e.id);
                if (existingIdx >= 0) {
                    timeline[existingIdx] = { kind: "assistant", id: e.id, text: e.text, finalized: true };
                }
                else {
                    // Duplicate-emission guard (BUG-116): a single logical assistant message can reach
                    // us as several message_completed events (the Codex mapper derives one from
                    // agent_message AND from item/completed). They carry distinct ids, so the id guard
                    // above misses them. If the last finalized assistant bubble already has this exact
                    // text and nothing was streamed since, treat this as a re-emission and skip it
                    // instead of stacking identical bubbles (the "CHILD_AGENT_DONE ×3" symptom).
                    const lastMeaningful = timeline[timeline.length - 1];
                    const isDuplicate = lastMeaningful?.kind === "assistant" && lastMeaningful.finalized && lastMeaningful.text === e.text;
                    if (!isDuplicate) {
                        timeline.push({ kind: "assistant", id: e.id, text: e.text, finalized: true });
                    }
                }
            }
            closeAssistant();
            break;
        }
        case "tool_started":
            closeAssistant();
            // Idempotent guard (BUG-111): skip a re-delivered tool_started whose row already exists.
            if (!timeline.some((it) => it.kind === "tool" && it.id === e.id)) {
                timeline.push({ kind: "tool", id: e.id, toolName: e.toolName, status: "running", input: e.input });
            }
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
            // No running tool of this name to close. In a normal forward stream every
            // tool_completed has a matching running tool, so reaching here means either a
            // re-delivery (chat switch / replay) of an already-completed tool or an orphan
            // completion. Skip if a tool of this name already exists (re-delivery); otherwise
            // record it. (BUG-111)
            if (timeline.some((it) => it.kind === "tool" && it.toolName === e.toolName)) {
                return finalize(timeline);
            }
            timeline.push({ kind: "tool", id: e.id, toolName: e.toolName, status: e.status, output: e.output });
            break;
        }
        case "file_changed":
            closeAssistant();
            // Idempotent guard (BUG-111): a chat switch re-streams the run from seq 0, so a
            // file_changed whose row already exists must not be pushed again — otherwise the
            // same "calc.go modified" / "CA-*.md created" rows duplicate after switching back.
            if (!timeline.some((it) => it.kind === "file" && it.id === e.id)) {
                timeline.push({ kind: "file", id: e.id, path: e.path, changeType: e.changeType });
            }
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
        case "agent_spawned_by_user":
            // Idempotent — replay from seq 0 must not duplicate the row. (BUG-121)
            if (!timeline.some((it) => it.kind === "system" && it.id === e.id)) {
                timeline.push({ kind: "system", id: e.id, text: `Spawned agent **${e.agentName}**`, tone: "info" });
            }
            // Only keep an existing thinking row — never create a new one for annotation events.
            shouldKeepThinking = thinkingItem !== undefined;
            break;
        case "agent_result_injected":
            // Idempotent — replay from seq 0 must not duplicate the row. (BUG-121)
            if (!timeline.some((it) => it.kind === "system" && it.id === e.id)) {
                timeline.push({ kind: "system", id: e.id, text: `**[${e.agentName}]** ${e.finalMessage}`, tone: "info" });
            }
            shouldKeepThinking = thinkingItem !== undefined;
            break;
        case "flow_gate_violation":
            // CP-35: the gate fired. For a reprompt rule a turn_started follows; for a
            // block rule this card is the only signal. Render it and never spawn a
            // thinking row, so block rules don't leave a dangling "Thinking..." line.
            if (!timeline.some((it) => it.kind === "system" && it.id === e.id)) {
                timeline.push({ kind: "system", id: e.id, text: `⚠ ${e.error}`, tone: "warn" });
            }
            shouldKeepThinking = thinkingItem !== undefined;
            break;
    }
    return finalize(timeline);
}
