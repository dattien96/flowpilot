package runner

import (
	"fmt"
	"strings"
)

// Task-339 (CP-62 P-3): structured escalation cards. The model (or the vibe
// owner-debate hub) calls the request_user_decision tool with a schema'd
// question + options; the runner emits a user_decision_card_requested event
// the client renders as actionable buttons. Q-1 wrap-around: the prose card
// path is untouched — when no valid card payload exists, the legacy prose
// card is the fallback, so the user is never left without an ask.

// EventUserDecisionCardRequested carries the schema'd card payload in Input.
// Additive to the prose events; clients that do not know it ignore it.
const EventUserDecisionCardRequested ProviderEventType = "user_decision_card_requested"

// DecisionCardOption is one choice on the card. Consequence states what
// happens when chosen — a non-tech user decides from consequences, not prose.
type DecisionCardOption struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Consequence string `json:"consequence"`
}

// DecisionCardEvidence is one file:line citation backing the question.
type DecisionCardEvidence struct {
	Path    string `json:"path"`
	Line    int    `json:"line,omitempty"`
	Excerpt string `json:"excerpt,omitempty"`
}

// DecisionCardKindTournament marks a decision card emitted by the tournament
// arbiter's ask leg (CP-65): its option ids are candidate ids plus the
// "retry"/"ask" actions, and a captured choice routes into tournament.merge
// or a retry rollout rather than the generic blocked-resume path.
const DecisionCardKindTournament = "tournament"

// UserDecisionCard is the schema'd payload of request_user_decision.
type UserDecisionCard struct {
	// Kind is an internal routing tag (never required on the wire): "tournament"
	// cards consume their choice in resumeFlowWithFeedback's tournament branch.
	Kind        string                 `json:"kind,omitempty"`
	Question    string                 `json:"question"`
	Options     []DecisionCardOption   `json:"options"`
	Recommended string                 `json:"recommended,omitempty"`
	Evidence    []DecisionCardEvidence `json:"evidence,omitempty"`
	Detail      string                 `json:"detail,omitempty"`
}

// parseUserDecisionCard validates a request_user_decision tool-call args map.
// Strict: question required, ≥1 option with id/label/consequence each,
// recommended (when present) must reference a declared option id, evidence
// entries require path. Malformed payloads return an error — the caller keeps
// the prose fallback (never a silent drop).
func parseUserDecisionCard(args map[string]any) (UserDecisionCard, error) {
	var card UserDecisionCard
	card.Kind, _ = args["kind"].(string)
	card.Question, _ = args["question"].(string)
	if strings.TrimSpace(card.Question) == "" {
		return card, fmt.Errorf("request_user_decision: question is required")
	}
	rawOptions, _ := args["options"].([]any)
	if len(rawOptions) == 0 {
		return card, fmt.Errorf("request_user_decision: at least one option is required")
	}
	for _, item := range rawOptions {
		m, ok := item.(map[string]any)
		if !ok {
			return card, fmt.Errorf("request_user_decision: each option must be an object")
		}
		opt := DecisionCardOption{}
		opt.ID, _ = m["id"].(string)
		if strings.TrimSpace(opt.ID) == "" {
			return card, fmt.Errorf("request_user_decision: option.id is required")
		}
		opt.Label, _ = m["label"].(string)
		if strings.TrimSpace(opt.Label) == "" {
			return card, fmt.Errorf("request_user_decision: option.label is required")
		}
		opt.Consequence, _ = m["consequence"].(string)
		if strings.TrimSpace(opt.Consequence) == "" {
			return card, fmt.Errorf("request_user_decision: option.consequence is required")
		}
		card.Options = append(card.Options, opt)
	}
	card.Recommended, _ = args["recommended"].(string)
	if card.Recommended != "" {
		known := false
		for _, opt := range card.Options {
			if opt.ID == card.Recommended {
				known = true
				break
			}
		}
		if !known {
			return card, fmt.Errorf("request_user_decision: recommended must reference a declared option id, got %q", card.Recommended)
		}
	}
	if rawEvidence, ok := args["evidence"].([]any); ok {
		for _, item := range rawEvidence {
			m, ok := item.(map[string]any)
			if !ok {
				return card, fmt.Errorf("request_user_decision: evidence entries must be objects")
			}
			ev := DecisionCardEvidence{}
			ev.Path, _ = m["path"].(string)
			if strings.TrimSpace(ev.Path) == "" {
				return card, fmt.Errorf("request_user_decision: evidence.path is required")
			}
			switch line := m["line"].(type) {
			case float64:
				ev.Line = int(line)
			case int:
				ev.Line = line
			}
			ev.Excerpt, _ = m["excerpt"].(string)
			card.Evidence = append(card.Evidence, ev)
		}
	}
	card.Detail, _ = args["detail"].(string)
	return card, nil
}

// emitUserDecisionCard stamps and emits the structured card event. The prose
// park path (GateReason / EventFlowGateViolation) is untouched — this event
// is additive so the client can render actionable options when available.
func (s *InteractiveService) emitUserDecisionCard(runID string, card UserDecisionCard) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rs := s.runs[runID]
	if rs == nil {
		return
	}
	rs.decisionCard = &card
	s.emitLocked(rs, ProviderEvent{
		Type:           EventUserDecisionCardRequested,
		ProviderTurnID: rs.currentTurnID,
		Input:          card,
	})
}
