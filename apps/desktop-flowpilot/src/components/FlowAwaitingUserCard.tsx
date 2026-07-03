import { useState } from "react";
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
export function FlowAwaitingUserCard(): React.ReactElement | null {
  const loopState = useStore((s) => s.agentGraphSnapshot?.loopState);
  const continueFlow = useStore((s) => s.continueFlow);
  const stop = useStore((s) => s.stop);
  const [feedback, setFeedback] = useState("");
  const [submitting, setSubmitting] = useState(false);

  if (!loopState || loopState.status !== "blocked") return null;

  const reasonLabel = loopState.blockReason === "cap" ? "Round limit reached" : "Needs your decision";
  const detail =
    loopState.gateReason ||
    (loopState.blockReason === "cap"
      ? "The review loop reached its round limit."
      : "The flow paused and is waiting for your input.");

  const handleContinue = async () => {
    if (submitting) return;
    setSubmitting(true);
    try {
      await continueFlow(feedback.trim());
      setFeedback("");
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
      <p className="card-prompt">{detail}</p>
      <textarea
        className="text-input flow-awaiting-user-feedback"
        placeholder="Optional — give the flow guidance before continuing…"
        value={feedback}
        onChange={(e) => setFeedback(e.target.value)}
        rows={4}
        disabled={submitting}
      />
      <div className="other-row">
        <button type="button" className="btn btn-ghost" onClick={() => void handleStop()} disabled={submitting}>
          Stop
        </button>
        <button type="button" className="btn btn-primary" onClick={() => void handleContinue()} disabled={submitting}>
          Continue
        </button>
      </div>
    </div>
  );
}
