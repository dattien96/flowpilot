package runner

import (
	"path/filepath"
	"strings"

	"flowpilot-runner/internal/changeledger"
	"flowpilot-runner/internal/featurecatalog"
)

func injectFeatureHistoryPrompt(workspace string, prompt string, priorTurns []transcriptTurn) string {
	dotFlowpilotDir := filepath.Join(workspace, ".flowpilot")
	catalog, err := featurecatalog.LoadCatalog(dotFlowpilotDir)
	if err != nil {
		return prompt
	}
	ledger, err := changeledger.New(dotFlowpilotDir)
	if err != nil {
		return prompt
	}

	// Resolve the conversation's feature, not just this prompt: try the current
	// prompt first, then fall back through prior turns. A low-signal prompt
	// ("continue", "try again") thus inherits the established feature instead of
	// dropping context, while an explicit topic change still re-resolves.
	turns := append(append([]transcriptTurn{}, priorTurns...), transcriptTurn{User: prompt})
	top, ok := resolveTurnsFeature(turns, catalog)
	if !ok {
		return prompt
	}
	history := featurecatalog.HistorySlot(top.Key, ledger)
	if strings.TrimSpace(history) == "" {
		return prompt
	}
	combined := history
	if summaryLedger, err := changeledger.NewChatSummaryLedger(dotFlowpilotDir); err == nil {
		if discussion := featurecatalog.ChatSummarySlot(top.Key, summaryLedger); strings.TrimSpace(discussion) != "" {
			combined += "\n\n" + discussion
		}
	}
	return combined + "\n\n---\n\n" + prompt
}

// resolveTurnsFeature finds the conversation's feature by scanning user prompts
// newest → oldest and returning the first that resolves at or above the
// confidence threshold. This makes resolution conversation-sticky: a low-signal
// latest prompt ("continue"/"try again") falls back to the most recent
// substantive prompt's feature, while a genuine pivot resolves on its own.
func resolveTurnsFeature(turns []transcriptTurn, catalog *featurecatalog.Catalog) (featurecatalog.Candidate, bool) {
	for i := len(turns) - 1; i >= 0; i-- {
		user := strings.TrimSpace(turns[i].User)
		if user == "" {
			continue
		}
		candidates, err := featurecatalog.ResolveFeature(user, catalog)
		if err != nil {
			continue
		}
		if top, ok := featurecatalog.TopCandidate(candidates, 5.0); ok {
			return top, true
		}
	}
	return featurecatalog.Candidate{}, false
}

// bucketTurnsByFeature groups turns by the feature each belongs to. A turn whose
// own prompt resolves sets the running feature; low-signal turns ("continue")
// attach to the running feature. This lets a chat that spans two features
// produce one summary per feature from only that feature's turns (no mixing).
func bucketTurnsByFeature(turns []transcriptTurn, catalog *featurecatalog.Catalog) map[string][]transcriptTurn {
	buckets := make(map[string][]transcriptTurn)
	current := ""
	for _, turn := range turns {
		if user := strings.TrimSpace(turn.User); user != "" {
			if candidates, err := featurecatalog.ResolveFeature(user, catalog); err == nil {
				if top, ok := featurecatalog.TopCandidate(candidates, 5.0); ok {
					current = top.Key
				}
			}
		}
		if current != "" {
			buckets[current] = append(buckets[current], turn)
		}
	}
	return buckets
}
