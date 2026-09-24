// CP-84 (Task-431): per-kind inline controls for the attention inbox.
// `decisionControlModel` is a pure descriptor (unit-testable without a DOM);
// `DecisionControls` renders it, `DecisionPreview` renders the expandable
// heavy-payload details (T-2). Anything without a usable payload returns null
// so the caller renders Open-only (existing rule — never guess).

import { useState } from "react";
import type { AttentionItem } from "@/state/attentionQueue";
import type { DecisionPayload } from "@/types/contract";

export interface ControlChoice {
  value: string;
  label: string;
  /** Task-435 T-1: one-line consequence text shown under the action so the
   *  user understands what each choice does before clicking. */
  description?: string;
}

/** Task-435 T-1/T-5: worktree resolve modes with their consequence text.
 *  Descriptions must stay honest about destructive outcomes — keep_branch
 *  keeps only committed work; discard throws everything away. */
export const WORKTREE_MODES: ControlChoice[] = [
  {
    value: "apply_patch",
    label: "Apply patch",
    description: "Merge all worktree changes into your project, then clean up.",
  },
  {
    value: "keep_branch",
    label: "Keep branch",
    description: "Remove the worktree, keep the fp/* branch. Only committed work survives.",
  },
  {
    value: "discard",
    label: "Discard",
    description: "Delete the worktree and its branch — all changes are thrown away.",
  },
];

export type ControlModel =
  | { type: "choices"; choices: ControlChoice[]; prompt?: string }
  | { type: "gate"; options: ControlChoice[]; allowCustom: boolean }
  | { type: "ss_lock" }
  | { type: "worktree"; modes: ControlChoice[] }
  | { type: "worktree_confirm"; mode: string; modeLabel: string; files: string[]; message: string }
  | { type: "quota"; candidateLabel: string };

/** Kinds that may participate in "Approve all eligible" batch — safe,
 *  no-reading-required decisions only (T-3). */
export function isBatchEligible(item: AttentionItem): boolean {
  const d = item.decision;
  if (!d || !d.actionable) return false;
  if (d.kind === "approval") return true;
  if (d.kind === "question") {
    return !d.question?.multiSelect && (d.question?.options?.length ?? 0) > 0;
  }
  return false;
}

/** Default choice a batch submit sends for an eligible item (first approval
 *  option / first question option). */
export function batchChoiceFor(item: AttentionItem): string | undefined {
  const d = item.decision;
  if (!d) return undefined;
  if (d.kind === "approval") {
    return d.approval?.decisions?.find((x) => /approve|allow|yes/i.test(x.value + x.label))?.value
      ?? d.approval?.decisions?.[0]?.value
      ?? "approved";
  }
  if (d.kind === "question") {
    const opt = d.question?.options?.[0];
    return opt ? opt.value ?? opt.label : undefined;
  }
  return undefined;
}

/** Pure per-kind descriptor — null → caller renders Open-only. */
export function decisionControlModel(item: AttentionItem): ControlModel | null {
  const d = item.decision;
  if (!d || !d.actionable) return null;
  switch (d.kind) {
    case "approval":
      return {
        type: "choices",
        choices: d.approval?.decisions?.length
          ? d.approval.decisions
          : [
              { value: "approved", label: "Approve" },
              { value: "denied", label: "Deny" },
            ],
        prompt: d.approval?.command ?? d.prompt,
      };
    case "question":
      if (d.question?.multiSelect || !d.question?.options?.length) return null;
      return {
        type: "choices",
        choices: d.question.options.map((o) => ({ value: o.value ?? o.label, label: o.label })),
        prompt: d.prompt,
      };
    case "gate":
    case "r_requirement":
      return {
        type: "gate",
        options: [
          { value: "keep-test-fix-code", label: "Fix the code" },
          { value: "suggest-requirement-change", label: "Suggest requirement change" },
        ],
        allowCustom: true,
      };
    case "ss_lock":
      return { type: "ss_lock" };
    case "worktree_merge":
      // Task-435 T-5: a pending server-side confirm replaces the mode buttons
      // with an explicit Cancel/Proceed step listing the files at stake.
      if (item.worktreeConfirm) {
        const c = item.worktreeConfirm;
        return {
          type: "worktree_confirm",
          mode: c.mode,
          modeLabel: WORKTREE_MODES.find((m) => m.value === c.mode)?.label ?? c.mode,
          files: c.files,
          message: c.message,
        };
      }
      return { type: "worktree", modes: WORKTREE_MODES };
    case "quota":
      return {
        type: "quota",
        candidateLabel: d.quota?.candidateLabel ?? "the suggested account",
      };
    default:
      return null;
  }
}

/** Triage filters — kind and project, show-all by default (local state). */
export function filterAttentionItems(
  items: AttentionItem[],
  kind: string | undefined,
  projectId: string | undefined,
): AttentionItem[] {
  return items.filter(
    (i) => (!kind || i.kind === kind) && (!projectId || i.projectId === projectId),
  );
}

/** Kinds whose payload is heavy enough to hide behind ▸ expand by default. */
export function isHeavyKind(item: AttentionItem): boolean {
  return item.decision?.kind === "gate"
    || item.decision?.kind === "r_requirement"
    || item.decision?.kind === "ss_lock"
    || item.decision?.kind === "worktree_merge";
}

export function DecisionPreview(props: { item: AttentionItem }): React.ReactElement | null {
  const d = props.item.decision;
  if (!d) return null;
  const blocks: React.ReactNode[] = [];
  if (d.prompt) {
    blocks.push(<div key="p" className="decision-preview-line">{d.prompt}</div>);
  }
  if (d.gate?.regressedTests?.length) {
    blocks.push(
      <div key="t" className="decision-preview-line">
        Regressed: {d.gate.regressedTests.slice(0, 5).join(", ")}
        {d.gate.regressedTests.length > 5 ? ` (+${d.gate.regressedTests.length - 5})` : ""}
      </div>,
    );
  }
  if (d.ssLock?.quickView) {
    blocks.push(
      <div key="q" className="decision-preview-quickview">
        {d.ssLock.quickView}
      </div>,
    );
  }
  if (d.worktree?.conflictPaths?.length) {
    blocks.push(
      <div key="c" className="decision-preview-line">
        Conflicts: {d.worktree.conflictPaths.join(", ")}
      </div>,
    );
  }
  if (d.worktree?.branch) {
    blocks.push(
      <div key="b" className="decision-preview-line">
        Branch: {d.worktree.branch}
      </div>,
    );
  }
  if (d.quota?.candidateLabel || d.quota?.remainingPct !== undefined) {
    blocks.push(
      <div key="quota" className="decision-preview-line">
        {d.quota.candidateLabel ?? "Candidate account"}
        {d.quota.remainingPct !== undefined ? ` — ${d.quota.remainingPct}% remaining` : ""}
      </div>,
    );
  }
  if (d.question?.options?.length) {
    blocks.push(
      <div key="opts" className="decision-preview-line">
        Options: {d.question.options.map((o) => o.label).join(" · ")}
      </div>,
    );
  }
  if (blocks.length === 0) return null;
  return <div className="decision-preview">{blocks}</div>;
}

export function DecisionControls(props: {
  item: AttentionItem;
  acting: boolean;
  onAct: (choice: string, customText?: string, confirm?: boolean) => void;
  /** Task-435: cancel a pending confirm step (back to the mode buttons). */
  onDismiss?: () => void;
  onOpen: () => void;
}): React.ReactElement | null {
  const { item, acting, onAct, onDismiss, onOpen } = props;
  const model = decisionControlModel(item);
  const [gateChoice, setGateChoice] = useState("");
  const [customText, setCustomText] = useState("");
  if (!model) return null;

  switch (model.type) {
    case "choices":
      return (
        <div className="inbox-act-group">
          {model.prompt && (
            <span className="inbox-act-cmd" title={model.prompt}>
              {model.prompt.length > 48 ? `${model.prompt.slice(0, 47)}…` : model.prompt}
            </span>
          )}
          {model.choices.map((c) => (
            <button
              key={c.value}
              type="button"
              className={`inbox-act${/deny|reject|discard/i.test(c.value) ? " inbox-act--deny" : " inbox-act--approve"}`}
              disabled={acting}
              onClick={() => onAct(c.value)}
            >
              {c.label}
            </button>
          ))}
        </div>
      );
    case "gate": {
      const effective = customText.trim() ? "custom" : gateChoice;
      return (
        <div className="inbox-act-group inbox-act-group--gate">
          {model.options.map((o) => (
            <label key={o.value} className="inbox-gate-radio">
              <input
                type="radio"
                name={`gate-${item.runId}`}
                value={o.value}
                checked={gateChoice === o.value}
                disabled={acting}
                onChange={() => setGateChoice(o.value)}
              />
              <span>{o.label}</span>
            </label>
          ))}
          <input
            type="text"
            className="inbox-gate-custom"
            placeholder="Custom instruction…"
            value={customText}
            disabled={acting}
            onChange={(e) => setCustomText(e.target.value)}
          />
          <button
            type="button"
            className="inbox-act inbox-act--approve"
            disabled={acting || !effective}
            onClick={() => onAct(effective, effective === "custom" ? customText : undefined)}
          >
            Submit
          </button>
          <button type="button" className="inbox-act" disabled={acting} onClick={onOpen}>
            Open
          </button>
        </div>
      );
    }
    case "ss_lock":
      return (
        <div className="inbox-act-group">
          <button
            type="button"
            className="inbox-act inbox-act--approve"
            disabled={acting}
            onClick={() => onAct("approve")}
          >
            Approve spec
          </button>
          <button
            type="button"
            className="inbox-act inbox-act--deny"
            disabled={acting}
            onClick={() => onAct("reject")}
          >
            Reject
          </button>
          <button type="button" className="inbox-act" disabled={acting} onClick={onOpen}>
            Open
          </button>
        </div>
      );
    case "worktree":
      return (
        <div className="inbox-act-group inbox-act-group--worktree">
          {model.modes.map((m) => (
            <div key={m.value} className="inbox-worktree-mode">
              <button
                type="button"
                className={`inbox-act${m.value === "discard" ? " inbox-act--deny" : " inbox-act--approve"}`}
                disabled={acting}
                onClick={() => onAct(m.value)}
              >
                {m.label}
              </button>
              {m.description && <span className="inbox-act-desc">{m.description}</span>}
            </div>
          ))}
          <button type="button" className="inbox-act" disabled={acting} onClick={onOpen}>
            Open
          </button>
        </div>
      );
    case "worktree_confirm":
      return (
        <div className="inbox-act-group inbox-act-group--confirm">
          <span className="inbox-act-cmd">{model.message}</span>
          {model.files.length > 0 && (
            <span className="inbox-act-desc">
              Will be lost: {model.files.slice(0, 5).join(", ")}
              {model.files.length > 5 ? ` (+${model.files.length - 5})` : ""}
            </span>
          )}
          <button type="button" className="inbox-act" disabled={acting} onClick={onDismiss}>
            Cancel
          </button>
          <button
            type="button"
            className="inbox-act inbox-act--deny"
            disabled={acting}
            onClick={() => onAct(model.mode, undefined, true)}
          >
            {model.modeLabel} anyway
          </button>
        </div>
      );
    case "quota":
      return (
        <div className="inbox-act-group">
          <button
            type="button"
            className="inbox-act inbox-act--approve"
            disabled={acting}
            onClick={() => onAct("switch")}
          >
            Switch to {model.candidateLabel}
          </button>
          <button type="button" className="inbox-act" disabled={acting} onClick={onOpen}>
            Open
          </button>
        </div>
      );
  }
}
