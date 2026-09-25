import type { ApprovalDetails, DecisionCardDTO, FlowAuditDraftDTO, ProviderEventDTO, QuestionOption, QuotaRouteDecisionDTO, RunStatus } from "../types/contract";

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
  | { kind: "assistant"; id: string; text: string; finalized: boolean; pinned?: boolean }
  | { kind: "prompt"; id: string; text: string; selectedSkills?: string[]; attachments?: PromptAttachmentView[]; pinned?: boolean }
  | { kind: "thinking"; id: string; text: string }
  | { kind: "tool"; id: string; toolName: string; status: "running" | "success" | "failed" | "cancelled"; input?: unknown; output?: unknown }
  | { kind: "file"; id: string; path: string; changeType?: string }
  | { kind: "approval"; id: string; approvalId: string; details: ApprovalDetails; decision?: string }
  | { kind: "question"; id: string; questionId: string; prompt: string; options: QuestionOption[]; multiSelect?: boolean; answer?: string | string[]; quotaDecision?: QuotaRouteDecisionDTO }
  | { kind: "agent"; id: string; agentName: string; childRunId: string; finalMessage?: string }
  | { kind: "decision_card"; id: string; card: DecisionCardDTO; chosenOptionId?: string }
  | { kind: "system"; id: string; text: string; tone: "info" | "error" | "warn"; pinned?: boolean };

export interface PendingApproval {
  approvalId: string;
  details: ApprovalDetails;
}

export interface PendingQuestion {
  questionId: string;
  prompt: string;
  options: QuestionOption[];
  multiSelect?: boolean;
  /** Task-450: structured candidate table on quota_route_required cards. */
  quotaDecision?: QuotaRouteDecisionDTO;
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
  /** Task-421: ids of items evicted by the timeline window — a non-rendered
   *  dedup cache so a re-delivered event (replay/reconnect) never re-appends
   *  an evicted row. Mutated in place during eviction; capped by
   *  EVICTED_IDS_CAP. Never read by render code. */
  _timelineEvictedIds?: Set<string>;
}

/** Task-421: bound for the resident timeline array. Older items are evicted
 *  from memory (pinned interactive rows excepted) and paged back from the
 *  persisted transcript via loadEarlierTimeline. */
export const TIMELINE_WINDOW_MAX = 500;
/** Cap for the evicted-id dedup set — ids are small; 50k ≈ ~2MB worst case. */
const EVICTED_IDS_CAP = 50000;

/** Task-421: items that must never be evicted — unresolved interactive rows
 *  (approvals/questions/decision cards gate pending-state reconciliation in
 *  settleHistoryReplayPendingState), the live streaming bubble, `thinking`,
 *  and rows the user explicitly paged back (pinned flag set by
 *  loadEarlierTimeline). */
export function isPinnedTimelineItem(it: TimelineItem): boolean {
  if ("pinned" in it && it.pinned === true) return true;
  switch (it.kind) {
    case "approval": return it.decision === undefined;
    case "question": return it.answer === undefined;
    case "decision_card": return it.chosenOptionId === undefined;
    case "thinking": return true;
    case "assistant": return !it.finalized;
    default: return false;
  }
}

/** Task-421: keep at most `max` tail items, evicting oldest non-pinned rows
 *  into `evictedIds` (dedup guard). Returns the same refs when under the
 *  bound — zero allocation on the hot path. */
export function applyTimelineWindow(
  timeline: TimelineItem[],
  evictedIds: Set<string> | undefined,
  max: number = TIMELINE_WINDOW_MAX,
): { timeline: TimelineItem[]; evictedIds: Set<string> } {
  const evicted = evictedIds ?? new Set<string>();
  if (timeline.length <= max) return { timeline, evictedIds: evicted };
  let overflow = timeline.length - max;
  const kept: TimelineItem[] = [];
  for (const it of timeline) {
    if (overflow > 0 && !isPinnedTimelineItem(it)) {
      evicted.add(it.id);
      overflow--;
    } else {
      kept.push(it);
    }
  }
  while (evicted.size > EVICTED_IDS_CAP) {
    const oldest = evicted.values().next().value;
    if (oldest === undefined) break;
    evicted.delete(oldest);
  }
  return { timeline: kept, evictedIds: evicted };
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
    case "user_decision_card_requested":
      // CP-62 P-3 (Task-345/350): the run parks on the escalation card.
      // waiting_question keeps the run out of the "running" spinner; the
      // answer channel is the card buttons (POST agent-loop/continue) and
      // the FlowAwaitingUserCard feedback box — the chat composer stays
      // blocked while the run is parked (Q-1 prose fallback rides the
      // feedback box, not the composer).
      return "waiting_question";
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
  // Task-421: dedup guard against events whose timeline rows were evicted by
  // the window — a replayed/re-delivered event must not re-append a dropped
  // row at the tail.
  const evictedIds = s._timelineEvictedIds ?? new Set<string>();
  const wasEvicted = (id: string): boolean => evictedIds.has(id);
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
    // Task-421: bound the resident array — evict oldest non-pinned rows.
    const win = applyTimelineWindow(nextTimeline, evictedIds);
    return {
      timeline: shouldKeepThinking
        ? [
            ...win.timeline,
            thinkingItem ?? { kind: "thinking", id: `thinking-${win.timeline.length}`, text: "Thinking..." },
          ]
        : win.timeline,
      _timelineEvictedIds: win.evictedIds,
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
        const alreadyPresent = timeline.some((it) => it.kind === "prompt" && it.id === promptId) || wasEvicted(promptId);
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
        } else if (!wasEvicted(id)) {
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
        } else if (wasEvicted(e.id)) {
          // Task-421: row was evicted — a re-delivered completion must not
          // re-append it at the tail.
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
      if (!timeline.some((it) => it.kind === "tool" && it.id === e.id) && !wasEvicted(e.id)) {
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
      if (timeline.some((it) => it.kind === "tool" && it.toolName === e.toolName) || wasEvicted(e.id)) {
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
      if (!timeline.some((it) => it.kind === "file" && it.id === e.id) && !wasEvicted(e.id)) {
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
        ...(e.quotaDecision ? { quotaDecision: e.quotaDecision } : {}),
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
                // quotaDecision rides along only when present — an always-set
                // undefined key changes the pendingQuestions shape deep-equal'd
                // by existing question tests.
                { questionId: e.questionId, prompt: e.prompt, options: e.options, multiSelect: e.multiSelect, ...(e.quotaDecision ? { quotaDecision: e.quotaDecision } : {}) },
              ],
      });

    case "quota_route_committed": {
      // Task-449/450: informational requested→resolved rotation notice.
      const q = e.quotaRoute;
      timeline.push({
        kind: "system",
        id: e.id,
        text: `quota route: ${q.fromProvider ?? "?"}/${q.fromAccount ?? "?"} → ${q.toProvider ?? "?"}/${q.toAccount ?? "?"}${q.toModel ? ` ${q.toModel}` : ""}`,
        tone: "info",
      });
      return finalize(timeline, {});
    }
    case "quota_route_stopped":
      timeline.push({ kind: "system", id: e.id, text: `quota route stopped${e.quotaRoute?.reason ? ` (${e.quotaRoute.reason})` : ""}`, tone: "warn" });
      return finalize(timeline, {});
    case "quota_route_blocked":
      timeline.push({ kind: "system", id: e.id, text: `quota route blocked: no eligible candidate${e.quotaRoute?.reason ? ` (${e.quotaRoute.reason})` : ""}`, tone: "error" });
      return finalize(timeline, {});

    case "user_decision_card_requested":
      // CP-62 P-3 (Task-345/350): render the structured escalation card.
      // The answer channel is chooseDecisionOption → POST agent-loop/continue
      // (a parked run seals POST /turns with 409); no pendingQuestions entry
      // is registered — the Q-1 prose fallback is the FlowAwaitingUserCard
      // feedback box, not the composer (which stays blocked while parked).
      closeAssistant();
      timeline.push({ kind: "decision_card", id: e.id, card: e.input });
      return finalize(timeline);

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
      // Agent lifecycle belongs in an agent card, not a prose transcript row.
      // Idempotent by *event id*, not childRunId: lifecycle:reinvoke reuses the
      // same child run across Review Loop rounds and re-emits spawn with a new
      // event id (BUG-Rnd2). Dedupe-by-childRunId / "open card" heuristics both
      // fail on live wait:false flow children, which historically never received
      // agent_result_injected until settle (run-9034) — so the round-2 coder card
      // never appeared on the main chat timeline.
      if (!timeline.some((it) => it.kind === "agent" && it.id === e.id) && !wasEvicted(e.id)) {
        timeline.push({ kind: "agent", id: e.id, agentName: e.agentName, childRunId: e.childRunId });
      }
      // Only keep an existing thinking row — never create a new one for annotation events.
      shouldKeepThinking = thinkingItem !== undefined;
      break;

    case "agent_result_injected":
      {
        // Prefer the latest open activation for this child (reinvoke rounds);
        // fall back to the latest card of any status so resume dumps still bind.
        let index = -1;
        for (let i = timeline.length - 1; i >= 0; i--) {
          const it = timeline[i];
          if (it.kind === "agent" && it.childRunId === e.childRunId && !it.finalMessage) {
            index = i;
            break;
          }
        }
        if (index < 0) {
          for (let i = timeline.length - 1; i >= 0; i--) {
            const it = timeline[i];
            if (it.kind === "agent" && it.childRunId === e.childRunId) {
              index = i;
              break;
            }
          }
        }
        if (index >= 0) {
          const agent = timeline[index] as Extract<TimelineItem, { kind: "agent" }>;
          timeline[index] = { ...agent, finalMessage: e.finalMessage };
        } else if (!wasEvicted(e.id)) {
          timeline.push({ kind: "agent", id: e.id, agentName: e.agentName, childRunId: e.childRunId, finalMessage: e.finalMessage });
        }
      }
      shouldKeepThinking = thinkingItem !== undefined;
      break;

    case "flow_gate_violation":
      // CP-35: the gate fired. For a reprompt rule a turn_started follows; for a
      // block rule this card is the only signal. Render it and never spawn a
      // thinking row, so block rules don't leave a dangling "Thinking..." line.
      if (!timeline.some((it) => it.kind === "system" && it.id === e.id) && !wasEvicted(e.id)) {
        timeline.push({ kind: "system", id: e.id, text: `⚠ ${e.error}`, tone: "warn" });
      }
      shouldKeepThinking = thinkingItem !== undefined;
      break;

    case "provider_status": {
      // Task-439: one row per run for the provider cold-start lifecycle —
      // "connecting" pushes it, "ready"/"failed" resolve it in place so the
      // timeline shows progress instead of stacking a row per stage. The row
      // is keyed off workflowRunId (the event carries no providerTurnId — it
      // precedes turn_started).
      const rowId = `provider-status-${e.workflowRunId}`;
      const tone = e.status === "failed" ? "error" : "info";
      const idx = timeline.findIndex((it) => it.kind === "system" && it.id === rowId);
      if (idx >= 0) {
        timeline[idx] = { kind: "system", id: rowId, text: e.text ?? "", tone };
      } else if (!wasEvicted(rowId)) {
        timeline.push({ kind: "system", id: rowId, text: e.text ?? "", tone });
      }
      shouldKeepThinking = thinkingItem !== undefined;
      break;
    }

    case "flow_audit_draft":
      // BUG-243 F-3: the first UI surface for a produced audit draft (Task-171's
      // "inspectable before any write/commit" acceptance criterion — this
      // renders the draft, it never writes a file or creates a commit itself).
      // A blocked_* status is not an error, but it is not "ready" either, so
      // it gets the same warn tone as a gate violation rather than plain info.
      if (!timeline.some((it) => it.kind === "system" && it.id === e.id) && !wasEvicted(e.id)) {
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
