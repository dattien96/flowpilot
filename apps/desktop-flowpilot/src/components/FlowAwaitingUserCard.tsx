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
  const [submitting, setSubmitting] = useState(false);

  if (!loopState || loopState.status !== "blocked") return null;
  // CP-51 A1 dual-UI: when a regression/gate decision modal is open, hide this
  // escalate card so operators are not offered Retry/Stop/Allow behind a second modal.
  if (gateBlock) return null;

  const { stalled, isCap, driftedPaths, isDrift, retryIsPrimary } = awaitingUserDriftState(loopState);
  const reasonLabel =
    isCap
      ? "Round limit reached"
      : stalled
        ? "Member stalled"
        : "Needs your decision";
  const detail =
    loopState.gateReason ||
    (isCap
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
              className={`btn ${retryIsPrimary ? "btn-primary" : "btn-ghost"}`}
              onClick={() => void handleRetry()}
              disabled={submitting}
              title="run again with old scope"
            >
              Retry <span className="btn-desc">run again with old scope</span>
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
