import { useEffect, useRef, useState } from "react";
import { useStore } from "@/state/store";
import type { AttentionItem, AttentionKind } from "@/state/attentionQueue";
import { InboxIcon, EyeIcon } from "@/components/icons";
import {
  DecisionControls,
  DecisionPreview,
  batchChoiceFor,
  filterAttentionItems,
  isBatchEligible,
  isHeavyKind,
} from "@/components/DecisionControls";

const KIND_LABEL: Record<AttentionKind, string> = {
  approval: "Approval",
  question: "Question",
  gate: "Gate",
  ss_lock: "SS Lock",
  cp_lock: "CP Lock",
  r_requirement: "Requirement",
  decision: "Decision",
  dispatch_attention: "Dispatch",
  worktree_merge: "Merge",
  quota: "Quota",
};

function truncate(text: string, max = 72): string {
  const oneLine = text.replace(/\s+/g, " ").trim();
  return oneLine.length > max ? `${oneLine.slice(0, max - 1)}…` : oneLine;
}

function waitingLabel(iso: string): string {
  const ms = Date.now() - new Date(iso).getTime();
  if (!Number.isFinite(ms) || ms < 0) return "";
  const min = Math.floor(ms / 60_000);
  if (min < 1) return "now";
  if (min < 60) return `${min}m`;
  const hr = Math.floor(min / 60);
  if (hr < 24) return `${hr}h`;
  return `${Math.floor(hr / 24)}d`;
}

/**
 * Header inbox affordance for the attention queue (replaces the always-visible
 * Navigator panel). Badge shows the pending count; the popover lists each
 * waiting run oldest-first and opens it in place.
 *
 * Task-423: items with a pending payload expose inline actions — approve/deny
 * and question option chips — executed against the run's IDs without
 * switching workspace focus (approveAttentionItem/answerAttentionItem never
 * touch the focused run's slots). Kinds without a safe payload keep the
 * Open-only affordance.
 */
export function AttentionInbox(): React.ReactElement {
  const items = useStore((s) => s.attentionItems);
  const openRunAtAttention = useStore((s) => s.openRunAtAttention);
  const approveAttentionItem = useStore((s) => s.approveAttentionItem);
  const answerAttentionItem = useStore((s) => s.answerAttentionItem);
  const openSpectator = useStore((s) => s.openSpectator);
  const submitAttentionDecision = useStore((s) => s.submitAttentionDecision);
  const dismissWorktreeConfirm = useStore((s) => s.dismissWorktreeConfirm);
  const focusedRunId = useStore((s) => s.runId);
  const projects = useStore((s) => s.projects);
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState<Set<string>>(new Set());
  const [errors, setErrors] = useState<Record<string, string>>({});
  // CP-84 (Task-431): local triage state — never persisted to the store.
  const [kindFilter, setKindFilter] = useState<string>("");
  const [projectFilter, setProjectFilter] = useState<string>("");
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const rootRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const onPointerDown = (e: MouseEvent) => {
      if (rootRef.current && !rootRef.current.contains(e.target as Node)) setOpen(false);
    };
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false);
    };
    window.addEventListener("mousedown", onPointerDown);
    window.addEventListener("keydown", onKeyDown);
    return () => {
      window.removeEventListener("mousedown", onPointerDown);
      window.removeEventListener("keydown", onKeyDown);
    };
  }, [open]);

  // Auto-close when the queue drains (e.g. last item approved from the chat).
  useEffect(() => {
    if (items.length === 0) setOpen(false);
  }, [items.length]);

  const setBusyKey = (key: string, on: boolean) => {
    setBusy((prev) => {
      const next = new Set(prev);
      if (on) next.add(key);
      else next.delete(key);
      return next;
    });
  };

  const act = async (item: AttentionItem, key: string, run: () => Promise<boolean>) => {
    setBusyKey(key, true);
    setErrors((prev) => ({ ...prev, [item.runId]: "" }));
    const ok = await run();
    setBusyKey(key, false);
    if (!ok) {
      setErrors((prev) => ({ ...prev, [item.runId]: "Action failed — open the run to retry." }));
    }
  };

  const renderActions = (item: AttentionItem) => {
    const pending = item.pending;
    if (!pending) return null;
    const blocks: React.ReactNode[] = [];

    for (const approval of pending.approvals) {
      const key = `${item.runId}:${approval.approvalId}`;
      const decisions =
        approval.details?.decisions?.length
          ? approval.details.decisions
          : [
              { value: "approved", label: "Approve" },
              { value: "denied", label: "Deny" },
            ];
      blocks.push(
        <div className="inbox-act-group" key={approval.approvalId}>
          {approval.details?.command && (
            <span className="inbox-act-cmd" title={approval.details.command}>
              {truncate(approval.details.command, 48)}
            </span>
          )}
          {decisions.map((d) => (
            <button
              key={d.value}
              type="button"
              className={`inbox-act${d.value === "denied" || /deny|reject/i.test(d.value) ? " inbox-act--deny" : " inbox-act--approve"}`}
              disabled={busy.has(key)}
              onClick={() =>
                void act(item, key, () =>
                  approveAttentionItem(item.runId, approval.approvalId, d.value),
                )
              }
            >
              {d.label}
            </button>
          ))}
        </div>,
      );
    }

    for (const question of pending.questions) {
      // Multi-select questions need composed answers — Open-only (T-3).
      if (question.multiSelect || !question.options?.length) continue;
      const key = `${item.runId}:${question.questionId}`;
      blocks.push(
        <div className="inbox-act-group" key={question.questionId}>
          <span className="inbox-act-cmd" title={question.prompt}>
            {truncate(question.prompt, 48)}
          </span>
          {question.options.map((opt) => (
            <button
              key={opt.value ?? opt.label}
              type="button"
              className="inbox-act"
              disabled={busy.has(key)}
              onClick={() =>
                void act(item, key, () =>
                  answerAttentionItem(item.runId, question.questionId, opt.value ?? opt.label),
                )
              }
            >
              {truncate(opt.label, 24)}
            </button>
          ))}
        </div>,
      );
    }

    if (blocks.length === 0) return null;
    return <div className="inbox-actions">{blocks}</div>;
  };

  // ---- Task-431: triage + per-kind controls --------------------------------
  const visible = filterAttentionItems(items, kindFilter || undefined, projectFilter || undefined);
  const eligible = visible.filter(isBatchEligible);
  const toggleExpanded = (runId: string) =>
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(runId)) next.delete(runId);
      else next.add(runId);
      return next;
    });

  const batchApprove = async () => {
    // Sequential submits — never a bulk endpoint; partial failures reported
    // per-item and the items stay in the queue (T-3/T-4).
    for (const item of eligible) {
      const d = item.decision;
      const choice = d ? batchChoiceFor(item) : undefined;
      if (!d || !choice) continue;
      const key = `${item.runId}:${d.id}`;
      // act() records a per-item error on failure — the partial-failure report.
      await act(item, key, () => submitAttentionDecision(item.runId, d, choice));
    }
  };

  const renderDecision = (item: AttentionItem) => {
    const d = item.decision;
    if (!d) return renderActions(item);
    const acting = busy.has(`${item.runId}:${d.id}`);
    const showPreview = expanded.has(item.runId);
    return (
      <div className="inbox-actions">
        <DecisionControls
          item={item}
          acting={acting}
          onAct={(choice, customText, confirm) =>
            void act(item, `${item.runId}:${d.id}`, () =>
              submitAttentionDecision(item.runId, d, choice, customText, confirm),
            )
          }
          onDismiss={() => dismissWorktreeConfirm(item.runId)}
          onOpen={() => {
            setOpen(false);
            void openRunAtAttention(item.runId, item.chatId, item.projectId);
          }}
        />
        {showPreview && <DecisionPreview item={item} />}
      </div>
    );
  };

  return (
    <div className="attention-inbox" ref={rootRef}>
      <button
        type="button"
        className={`icon-btn attention-inbox-btn${open ? " attention-inbox-btn-open" : ""}`}
        onClick={() => setOpen((v) => !v)}
        aria-expanded={open}
        aria-label={items.length > 0 ? `${items.length} runs need attention` : "Attention inbox"}
        title={items.length > 0 ? `${items.length} run${items.length > 1 ? "s" : ""} need attention` : "Needs attention"}
      >
        <InboxIcon size={15} />
        {items.length > 0 && (
          <span className="attention-inbox-badge" aria-hidden="true">
            {items.length}
          </span>
        )}
      </button>
      {open && (
        <div className="attention-inbox-pop" role="menu" aria-label="Runs needing attention">
          <div className="attention-inbox-head">
            <span>Needs attention</span>
            {eligible.length >= 2 && (
              <button
                type="button"
                className="inbox-act inbox-act--approve inbox-batch-approve"
                onClick={() => void batchApprove()}
              >
                Approve all ({eligible.length})
              </button>
            )}
          </div>
          {items.length > 0 && (
            <div className="attention-inbox-filters">
              <select
                className="attention-filter"
                aria-label="Filter by kind"
                value={kindFilter}
                onChange={(e) => setKindFilter(e.target.value)}
              >
                <option value="">All kinds</option>
                {Object.entries(KIND_LABEL).map(([k, label]) => (
                  <option key={k} value={k}>{label}</option>
                ))}
              </select>
              <select
                className="attention-filter"
                aria-label="Filter by project"
                value={projectFilter}
                onChange={(e) => setProjectFilter(e.target.value)}
              >
                <option value="">All projects</option>
                {projects.map((p) => (
                  <option key={p.id} value={p.id}>{p.name}</option>
                ))}
              </select>
            </div>
          )}
          {items.length === 0 ? (
            <div className="attention-inbox-empty">Nothing is waiting on you.</div>
          ) : (
            <ul className="attention-inbox-list">
              {visible.map((item: AttentionItem) => (
                <li key={item.runId} className="attention-inbox-li">
                  <div className="attention-inbox-item">
                    {isHeavyKind(item) && (
                      <button
                        type="button"
                        className="attention-inbox-expand"
                        aria-expanded={expanded.has(item.runId)}
                        aria-label={`${expanded.has(item.runId) ? "Collapse" : "Expand"} details for ${item.runTitle}`}
                        onClick={() => toggleExpanded(item.runId)}
                      >
                        ▸
                      </button>
                    )}
                    {isBatchEligible(item) && (
                      <input
                        type="checkbox"
                        className="attention-inbox-check"
                        aria-label={`Eligible for batch approve`}
                        checked
                        readOnly
                        title="Eligible for batch approve"
                      />
                    )}
                    <span className={`attention-kind attention-kind--${item.kind}`}>
                      {KIND_LABEL[item.kind]}
                    </span>
                    <button
                      type="button"
                      className="attention-inbox-title"
                      title={`${item.runTitle} — ${KIND_LABEL[item.kind]}`}
                      onClick={() => {
                        setOpen(false);
                        void openRunAtAttention(item.runId, item.chatId, item.projectId);
                      }}
                    >
                      {truncate(item.runTitle)}
                    </button>
                    <span className="attention-inbox-project">
                      {projects.find((p) => p.id === item.projectId)?.name ?? ""}
                    </span>
                    <span className="attention-inbox-waiting">{waitingLabel(item.waitingSince)}</span>
                    {item.runId !== focusedRunId && (
                      <button
                        type="button"
                        className="inbox-watch"
                        aria-label={`Watch ${item.runTitle}`}
                        title="Watch in spectator pane"
                        onClick={() => openSpectator(item.runId, item.projectId)}
                      >
                        <EyeIcon size={12} />
                      </button>
                    )}
                  </div>
                  {renderDecision(item)}
                  {errors[item.runId] && (
                    <div className="inbox-error" role="alert">
                      {errors[item.runId]}
                    </div>
                  )}
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </div>
  );
}
