import { useState } from "react";
import { awaitingUserDriftState } from "@/components/flowAwaitingUserDrift";
import { useStore } from "@/state/store";

// BUG-231: an escalate (or round-cap-reached) outcome pauses the flow's agent
// loop awaiting the human — a deliberate, non-terminal "awaiting user" state,
// not a hang. Before this card, that pause had no actionable surface in the
// main chat: the composer stayed locked ("Waiting for the current turn…")
// with no way for the user the flow is waiting on to respond. This card
// renders inline, above the composer, whenever the loop is "blocked" —
// modeled on QuestionCard's simple prompt + input + submit shape (per user
// direction: "giống form question_user vậy").
//
// Continue does NOT hard-route anywhere itself: it hands the (optional)
// feedback to the hub and lets the hub re-decide via submit_review_outcome
// (approve -> done, changes requested -> back to the coder, still stuck ->
// blocked again and this card reappears). Stop reuses the existing
// stop-the-loop + interrupt path.
//
// Task-241: blockReason=member_stalled shows Retry / Skip / Stop instead of
// the generic Continue form (I-16).

export function FlowAwaitingUserCard(): React.ReactElement | null {
  const loopState = useStore((s) => s.agentGraphSnapshot?.loopState);
  const gateBlock = useStore((s) => s.gateBlock);
  const continueFlow = useStore((s) => s.continueFlow);
  const amendFlow = useStore((s) => s.amendFlow);
  const stop = useStore((s) => s.stop);
  const [feedback, setFeedback] = useState("");
  const [amendInput, setAmendInput] = useState("");
  const [amendOpen, setAmendOpen] = useState(false);
  const [amendError, setAmendError] = useState("");
  const [submitting, setSubmitting] = useState(false);

  if (!loopState || loopState.status !== "blocked") return null;
  // CP-51 A1 dual-UI: when a regression/gate decision modal is open, hide this
  // escalate card so operators are not offered Retry/Stop/Allow behind a second modal.
  if (gateBlock) return null;

  const { stalled, isCap, isSprintBoundary, driftedPaths, isDrift, canAmend, retryIsPrimary } = awaitingUserDriftState(loopState);
  const isVibeLock = loopState.blockReason === "vibe_lock";
  const reasonLabel =
    isVibeLock
      ? "Preview & Lock"
      : isSprintBoundary
        ? "Sprint done"
        : isCap
          ? "Round limit reached"
          : stalled
            ? "Member stalled"
            : "Needs your decision";
  const detail =
    loopState.gateReason ||
    (isVibeLock
      ? "Lock the SS/CP draft to start slicing, or paste edits then Lock."
      : isSprintBoundary
        ? "Task complete — Continue starts the next task, Stop ends the run."
        : isCap
          ? "The review loop reached its round limit."
          : stalled
            ? "A cohort member stopped producing events. Retry it, skip it (mark failed and join), or stop the flow."
            : "The flow paused and is waiting for your input.");

  const handleRetry = async () => {
    if (submitting) return;
    setSubmitting(true);
    try {
      await continueFlow(feedback.trim());
      setFeedback("");
    } finally {
      setSubmitting(false);
    }
  };

  const handleAllow = async () => {
    if (submitting || !driftedPaths) return;
    setSubmitting(true);
    try {
      await amendFlow(driftedPaths);
      setFeedback("");
    } finally {
      setSubmitting(false);
    }
  };

  // BUG-638 (live run-306526): a RESCOPE/escalate park names the missing
  // paths in prose — no drift marker, so the Allow button never rendered
  // and the only action the escalation asked for was unreachable. Offer a
  // declare-paths field on every amendable park; the server validates and
  // reports unamendable entries.
  const handleAmendScope = async () => {
    if (submitting) return;
    const paths = amendInput
      .split(/[,\n]/)
      .map((p) => p.trim())
      .filter((p) => p.length > 0);
    if (paths.length === 0) return;
    setSubmitting(true);
    setAmendError("");
    try {
      await amendFlow(paths);
      setAmendInput("");
      setAmendOpen(false);
      setFeedback("");
    } catch (err) {
      // 404 no_frozen_contract / 422 amend_failed land here — keep the field
      // open and show the server's reason instead of a silent dead button.
      setAmendError(err instanceof Error ? err.message : "amend failed");
    } finally {
      setSubmitting(false);
    }
  };

  const handleMemberAction = async (action: "retry" | "skip") => {
    if (submitting) return;
    setSubmitting(true);
    try {
      await continueFlow("", { action, node: loopState.activeNode });
    } finally {
      setSubmitting(false);
    }
  };

  const handleStop = async () => {
    if (submitting) return;
    setSubmitting(true);
    try {
      await stop();
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="card question flow-awaiting-user-card" role="status" aria-live="polite">
      <div className="card-head">
        <span className="badge badge-ask">{reasonLabel}</span>
      </div>
      <p className="card-prompt flow-awaiting-user-detail">{detail}</p>
      {!stalled && (
        <textarea
          className="text-input flow-awaiting-user-feedback"
          placeholder="Optional — give the flow guidance before continuing…"
          value={feedback}
          onChange={(e) => setFeedback(e.target.value)}
          rows={4}
          disabled={submitting}
        />
      )}
      {/* BUG-638: amendable parks without a drift marker (RESCOPE/escalate
          verdicts that name missing paths in prose) get a declare-paths
          field — previously the card offered only Retry/Stop while the
          escalation explicitly asked for a contract amend. */}
      {canAmend && !isDrift && !isVibeLock && !stalled && (
        amendOpen ? (
          <>
          <div className="amend-scope-row">
            <input
              className="text-input flow-awaiting-user-amend"
              placeholder="Declare scope — path(s), comma separated"
              value={amendInput}
              onChange={(e) => setAmendInput(e.target.value)}
              disabled={submitting}
            />
            <button
              type="button"
              className="btn btn-ghost"
              onClick={() => void handleAmendScope()}
              disabled={submitting || amendInput.trim() === ""}
              title="widen the frozen contract and resume"
            >
              Amend <span className="btn-desc">declare path(s) &amp; resume</span>
            </button>
          </div>
          {amendError !== "" && (
            <p className="flow-awaiting-user-amend-error" role="alert">{amendError}</p>
          )}
          </>
        ) : (
          <button
            type="button"
            className="btn btn-ghost amend-scope-toggle"
            onClick={() => setAmendOpen(true)}
            disabled={submitting}
            title="widen the frozen contract's declared paths"
          >
            Amend scope…
          </button>
        )
      )}
      <div className="other-row">
        <button
          type="button"
          className="btn btn-ghost"
          onClick={() => void handleStop()}
          disabled={submitting}
          title="end flow"
        >
          Stop <span className="btn-desc">end flow</span>
        </button>
        {stalled ? (
          <>
            <button type="button" className="btn btn-ghost" onClick={() => void handleMemberAction("skip")} disabled={submitting}>
              Skip member
            </button>
            <button type="button" className="btn btn-primary" onClick={() => void handleMemberAction("retry")} disabled={submitting}>
              Retry member
            </button>
          </>
        ) : (
          <>
            <button
              type="button"
              className={`btn ${retryIsPrimary || isVibeLock || isSprintBoundary ? "btn-primary" : "btn-ghost"}`}
              onClick={() => void handleRetry()}
              disabled={submitting}
              title={isVibeLock ? "lock draft and continue" : isSprintBoundary ? "start the next task" : "run again with old scope"}
            >
              {isVibeLock ? "Lock" : isSprintBoundary ? "Continue" : "Retry"} <span className="btn-desc">{isVibeLock ? "write-back if edited, then slice" : isSprintBoundary ? "start the next task" : "run again with old scope"}</span>
            </button>
            {isDrift && (
              <button
                type="button"
                className="btn btn-primary"
                onClick={() => void handleAllow()}
                disabled={submitting}
                title="continue with new scope match code changed"
              >
                Allow <span className="btn-desc">continue with new scope (match code changed)</span>
              </button>
            )}
          </>
        )}
      </div>
    </div>
  );
}
