import type { ApprovalDetails, FlowAuditDraftDTO, ProviderEventDTO, QuestionOption, RunStatus } from "../types/contract";

/** BUG-243 F-3: condenses a FlowAuditDraft into the inline timeline card's
 *  markdown text. Mirrors RenderAuditDraftText's section order
 *  (flow_audit_draft.go) but trimmed for a chat-sized card rather than a
 *  full standalone document. */
function renderAuditDraftSummary(draft: FlowAuditDraftDTO): string {
  const lines: string[] = [];
  const heading = draft.status === "ready" ? "📝 Audit draft ready" : `📝 Audit draft — ${draft.status}`;
  lines.push(`**${heading}**`);
  if (draft.featureKey) lines.push(`Feature: \`${draft.featureKey}\``);
  lines.push(`Validation: ${draft.validationResult}`);
  if (draft.whatChanged) lines.push(`\n${draft.whatChanged}`);
  if (draft.commitMessage) lines.push(`\nSuggested commit:\n\`\`\`\n${draft.commitMessage}\n\`\`\``);
  if (draft.changeLedgerBlock) lines.push(`\n${draft.changeLedgerBlock}`);
  if (draft.status !== "ready") {
    lines.push(
      "\n_This is a draft only — no file was written and no commit was created._",
    );
  }
  return lines.join("\n");
}

/** Lightweight image-attachment view for a sent prompt bubble (Task-052). Holds a
 *  preview thumbnail (when available) and the filename, never the full payload. */
export interface PromptAttachmentView {
  id: string;
  originalName: string;
  mimeType: string;
  previewUrl?: string;
}

export type TimelineItem =
  | { kind: "assistant"; id: string; text: string; finalized: boolean }
  | { kind: "prompt"; id: string; text: string; selectedSkills?: string[]; attachments?: PromptAttachmentView[] }
  | { kind: "thinking"; id: string; text: string }
  | { kind: "tool"; id: string; toolName: string; status: "running" | "success" | "failed" | "cancelled"; input?: unknown; output?: unknown }
  | { kind: "file"; id: string; path: string; changeType?: string }
  | { kind: "approval"; id: string; approvalId: string; details: ApprovalDetails; decision?: string }
  | { kind: "question"; id: string; questionId: string; prompt: string; options: QuestionOption[]; multiSelect?: boolean; answer?: string | string[] }
  | { kind: "system"; id: string; text: string; tone: "info" | "error" | "warn" };

export interface PendingApproval {
  approvalId: string;
  details: ApprovalDetails;
}

export interface PendingQuestion {
  questionId: string;
  prompt: string;
  options: QuestionOption[];
  multiSelect?: boolean;
}

export interface TimelineState {
  status: RunStatus;
  timeline: TimelineItem[];
  recoverable: boolean;
  /** All approvals currently awaiting a decision. A provider turn can fan out several
   *  parallel tool calls that each request approval — this must stay a collection, not a
   *  single value, or a second permission_required silently orphans the first (BUG-157). */
  pendingApprovals: PendingApproval[];
  /** Same rationale as pendingApprovals — a provider can surface more than one question
   *  in a live turn (e.g. via the grouped ask UI), so this must be a collection. */
  pendingQuestions: PendingQuestion[];
  _streamingAssistantId?: string;
}

export function shouldApplyRunEvent(activeRunId: string | undefined, streamRunId: string): boolean {
  return activeRunId === streamRunId;
}

function statusFromEvent(e: ProviderEventDTO, prev: RunStatus): RunStatus {
  switch (e.type) {
    case "turn_started":
      return "running";
    case "permission_required":
      // A replayed already-resolved approval (BUG-ApprovalReplay-Restart,
      // carries `decision`) is not a new pending state — it must not flip a
      // settled run back to "waiting_approval" on a full server restart.
      return e.decision !== undefined ? prev : "waiting_approval";
    case "user_question_required":
      // A replayed already-resolved question (BUG-StaleQuestion, carries
      // `answer`) is not a new pending state — it must not flip a settled run
      // back to "waiting_question" on reconnect.
      return e.answer !== undefined ? prev : "waiting_question";
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

function isTurnCompletedPlaceholder(text: string): boolean {
  return text.trim().toLowerCase().replace(/\.$/, "") === "turn completed";
}

function hasPendingPrompt(timeline: TimelineItem[], prompt: string): boolean {
  for (let i = timeline.length - 1; i >= 0; i--) {
    const item = timeline[i];
    if (item.kind === "thinking") continue;
    return item.kind === "prompt" && item.text === prompt;
  }
  return false;
}

export function applyTimelineEvent(s: TimelineState, e: ProviderEventDTO): Partial<TimelineState> {
  const status = statusFromEvent(e, s.status);
  const thinkingItem = s.timeline.find((it) => it.kind === "thinking") as Extract<TimelineItem, { kind: "thinking" }> | undefined;
  // Annotation events (BUG-121): only preserve an existing thinking row — never create one.
  let shouldKeepThinking =
    e.type !== "turn_completed" &&
    e.type !== "turn_failed" &&
    e.type !== "permission_required" &&
    e.type !== "user_question_required";
  const timeline = s.timeline.filter((it) => it.kind !== "thinking");
  let streamingAssistantId = s._streamingAssistantId;

  // Stale-approval detection (BUG-074, narrowed by BUG-157): a provider turn can fan
  // out several parallel tool calls, so more than one approval can be legitimately
  // outstanding at once in a LIVE run — a new permission_required or an unrelated
  // tool_completed arriving while others are still pending is normal, not stale.
  // The one transition that genuinely cannot happen while an approval is still open
  // in a live run is the turn ending (turn_completed / turn_failed): the provider
  // cannot finish a turn with a tool call still blocked on approval. So only treat
  // pendingApprovals as stale (resolved in a prior session, no event captured — the
  // history-replay case BUG-074 was written for) when one of those two events arrives.
  const staleApprovalIds =
    e.type === "turn_completed" || e.type === "turn_failed" ? s.pendingApprovals.map((a) => a.approvalId) : [];
  if (staleApprovalIds.length > 0) {
    const remaining = new Set(staleApprovalIds);
    for (let i = 0; i < timeline.length && remaining.size > 0; i++) {
      const it = timeline[i];
      if (it.kind === "approval" && it.decision === undefined && remaining.has(it.approvalId)) {
        timeline[i] = { ...it, decision: "resolved" };
        remaining.delete(it.approvalId);
      }
    }
  }
  // Same narrowing as approvals (BUG-157): only turn_completed/turn_failed proves a
  // question was answered in a prior session without a captured event. A concurrent
  // second question, or an unrelated event, must not orphan an already-open question.
  const staleQuestionIds =
    e.type === "turn_completed" || e.type === "turn_failed" ? s.pendingQuestions.map((q) => q.questionId) : [];
  if (staleQuestionIds.length > 0) {
    const remaining = new Set(staleQuestionIds);
    for (let i = 0; i < timeline.length && remaining.size > 0; i++) {
      const it = timeline[i];
      if (it.kind === "question" && it.answer === undefined && remaining.has(it.questionId)) {
        timeline[i] = { ...it, answer: "answered" };
        remaining.delete(it.questionId);
      }
    }
  }

  const closeAssistant = () => {
    streamingAssistantId = undefined;
  };

  const finalize = (nextTimeline: TimelineItem[], extra: Partial<TimelineState> = {}): Partial<TimelineState> => {
    // With multiple approvals able to be outstanding at once, an unrelated event (e.g.
    // tool_completed for a parallel, non-gated tool call) must not flip status away from
    // "waiting_approval" while other approvals are still open (BUG-157).
    const pendingApprovals = extra.pendingApprovals ?? (staleApprovalIds.length > 0 ? [] : s.pendingApprovals);
    const pendingQuestions = extra.pendingQuestions ?? (staleQuestionIds.length > 0 ? [] : s.pendingQuestions);
    const derivedStatus =
      pendingApprovals.length > 0 ? "waiting_approval" : pendingQuestions.length > 0 ? "waiting_question" : status;
    return {
      timeline: shouldKeepThinking
        ? [
            ...nextTimeline,
            thinkingItem ?? { kind: "thinking", id: `thinking-${nextTimeline.length}`, text: "Thinking..." },
          ]
        : nextTimeline,
      status: derivedStatus,
      _streamingAssistantId: streamingAssistantId,
      // Always restated explicitly (not spread conditionally) so every returned partial
      // carries the caller's ground truth for pendingApprovals/pendingQuestions, even on
      // a no-op event.
      pendingApprovals,
      pendingQuestions,
      ...extra,
    };
  };

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
          const cur = timeline[idx] as Extract<TimelineItem, { kind: "assistant" }>;
          timeline[idx] = { ...cur, text: cur.text + e.text };
        }
      } else {
        // Idempotent re-stream guard (BUG-111): if a bubble with this event id already
        // exists, the message is being re-delivered (chat switch / replay from seq 0).
        // Resume that bubble and reset its text so the re-stream rebuilds it in place
        // instead of pushing a duplicate assistant bubble.
        const id = e.id;
        streamingAssistantId = id;
        const existingIdx = timeline.findIndex((it) => it.kind === "assistant" && it.id === id);
        if (existingIdx >= 0) {
          timeline[existingIdx] = { kind: "assistant", id, text: e.text, finalized: false };
        } else {
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
      } else {
        // Idempotent guard (BUG-111): update an existing bubble with this id in place
        // rather than pushing a duplicate on re-delivery.
        const existingIdx = timeline.findIndex((it) => it.kind === "assistant" && it.id === e.id);
        if (existingIdx >= 0) {
          timeline[existingIdx] = { kind: "assistant", id: e.id, text: e.text, finalized: true };
        } else {
          // Duplicate-emission guard (BUG-116): a single logical assistant message can reach
          // us as several message_completed events (the Codex mapper derives one from
          // agent_message AND from item/completed). They carry distinct ids, so the id guard
          // above misses them. If the last finalized assistant bubble already has this exact
          // text and nothing was streamed since, treat this as a re-emission and skip it
          // instead of stacking identical bubbles (the "CHILD_AGENT_DONE ×3" symptom).
          const lastMeaningful = timeline[timeline.length - 1];
          const isDuplicate =
            lastMeaningful?.kind === "assistant" && lastMeaningful.finalized && lastMeaningful.text === e.text;
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
      timeline.push({ kind: "approval", id: e.id, approvalId: e.approvalId, details: e.details, decision: e.decision });
      // A replayed already-resolved approval (BUG-ApprovalReplay-Restart) must
      // stay out of pendingApprovals — it renders read-only via `decision`
      // above, not as a new interactive card the run is waiting on. Mirrors the
      // user_question_required `answer` handling below.
      return finalize(timeline, {
        pendingApprovals:
          e.decision !== undefined
            ? s.pendingApprovals
            : [...s.pendingApprovals, { approvalId: e.approvalId, details: e.details }],
      });

    case "user_question_required":
      closeAssistant();
      timeline.push({
        kind: "question",
        id: e.id,
        questionId: e.questionId,
        prompt: e.prompt,
        options: e.options,
        multiSelect: e.multiSelect,
        answer: e.answer,
      });
      // A replayed already-resolved question (BUG-StaleQuestion) must stay
      // out of pendingQuestions — it renders read-only via `answer` above,
      // not as a new interactive card the run is waiting on.
      return finalize(timeline, {
        pendingQuestions:
          e.answer !== undefined
            ? s.pendingQuestions
            : [
                ...s.pendingQuestions,
                { questionId: e.questionId, prompt: e.prompt, options: e.options, multiSelect: e.multiSelect },
              ],
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

    case "flow_audit_draft":
      // BUG-243 F-3: the first UI surface for a produced audit draft (Task-171's
      // "inspectable before any write/commit" acceptance criterion — this
      // renders the draft, it never writes a file or creates a commit itself).
      // A blocked_* status is not an error, but it is not "ready" either, so
      // it gets the same warn tone as a gate violation rather than plain info.
      if (!timeline.some((it) => it.kind === "system" && it.id === e.id)) {
        timeline.push({
          kind: "system",
          id: e.id,
          text: renderAuditDraftSummary(e.flowAuditDraft),
          tone: e.flowAuditDraft.status === "ready" ? "info" : "warn",
        });
      }
      shouldKeepThinking = thinkingItem !== undefined;
      break;
  }

  return finalize(timeline);
}
