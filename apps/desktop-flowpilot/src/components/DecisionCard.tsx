import { useStore } from "@/state/store";
import type { DecisionCardDTO } from "@/types/contract";

interface Props {
  itemId: string;
  card: DecisionCardDTO;
  chosenOptionId?: string;
}

// CP-62 P-3 (Task-345): the structured escalation card for the runner's
// user_decision_card_requested event (request_user_decision). One-tap options
// with stated consequences; the recommended option is highlighted. The answer
// channel is the chat prompt — the runner matches the option id back to the
// parked card (Task-346) — and the prose composer stays available as the Q-1
// fallback, so the card never blocks free-text replies.
export function DecisionCard({ itemId, card, chosenOptionId }: Props): React.ReactElement {
  const choose = useStore((s) => s.chooseDecisionOption);
  const resolved = chosenOptionId !== undefined;

  return (
    <div className={`card decision-card ${resolved ? "resolved" : ""}`}>
      <div className="card-head">
        <span className="badge badge-ask">Decision needed</span>
        {resolved && (
          <span className="badge">answered: {card.options.find((o) => o.id === chosenOptionId)?.label ?? chosenOptionId}</span>
        )}
      </div>
      <div className="decision-card-question">{card.question}</div>
      {card.detail && <div className="decision-card-detail">{card.detail}</div>}
      <div className="decision-card-options">
        {card.options.map((option) => {
          const recommended = card.recommended === option.id;
          return (
            <button
              key={option.id}
              className={`decision-card-option ${recommended ? "recommended" : ""} ${
                resolved ? (chosenOptionId === option.id ? "chosen" : "") : ""
              }`}
              disabled={resolved}
              title={option.consequence}
              onClick={() => void choose(itemId, option.id)}
            >
              <span className="decision-card-option-label">
                {option.label}
                {recommended && <span className="badge badge-warn">recommended</span>}
              </span>
              <span className="decision-card-option-consequence">{option.consequence}</span>
            </button>
          );
        })}
      </div>
      {card.evidence && card.evidence.length > 0 && (
        <div className="decision-card-evidence">
          {card.evidence.map((ev) => (
            <div key={`${ev.path}:${ev.line ?? 0}`} className="decision-card-evidence-row">
              {ev.path}
              {ev.line !== undefined ? `:${ev.line}` : ""}
              {ev.excerpt ? ` — ${ev.excerpt}` : ""}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
