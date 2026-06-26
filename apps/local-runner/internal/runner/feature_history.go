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

	top, ok := resolveInjectionFeature(prompt, priorTurns, catalog)
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

// resolveInjectionFeature decides which feature's history to inject for the
// current prompt. The current prompt's own resolution always wins. When it does
// not resolve, the conversation's established feature is inherited ONLY if the
// current prompt is low-signal (a "continue"/"try again" continuation). A
// substantive but unrelated prompt (e.g. "write a haiku about the sea") resolves
// to nothing on its own and is NOT low-signal, so no stale prior context is
// carried forward — the block is simply omitted.
func resolveInjectionFeature(prompt string, priorTurns []transcriptTurn, catalog *featurecatalog.Catalog) (featurecatalog.Candidate, bool) {
	if top, ok := resolveOnePrompt(prompt, catalog); ok {
		return top, true
	}
	if !isLowSignalPrompt(prompt) {
		return featurecatalog.Candidate{}, false
	}
	return resolveTurnsFeature(priorTurns, catalog)
}

// resolveOnePrompt resolves a single prompt to its top feature at or above the
// confidence threshold.
func resolveOnePrompt(prompt string, catalog *featurecatalog.Catalog) (featurecatalog.Candidate, bool) {
	candidates, err := featurecatalog.ResolveFeature(strings.TrimSpace(prompt), catalog)
	if err != nil {
		return featurecatalog.Candidate{}, false
	}
	return featurecatalog.TopCandidate(candidates, 5.0)
}

// resolveTurnsFeature finds the conversation's feature by scanning user prompts
// newest → oldest and returning the first that resolves at or above the
// confidence threshold. Used to inherit the established feature for a low-signal
// latest prompt (injection), and to pick a run's feature for recording/handoff.
func resolveTurnsFeature(turns []transcriptTurn, catalog *featurecatalog.Catalog) (featurecatalog.Candidate, bool) {
	for i := len(turns) - 1; i >= 0; i-- {
		if strings.TrimSpace(turns[i].User) == "" {
			continue
		}
		if top, ok := resolveOnePrompt(turns[i].User, catalog); ok {
			return top, true
		}
	}
	return featurecatalog.Candidate{}, false
}

// lowSignalPhrases are short continuations / acknowledgements that carry no topic
// of their own; they inherit the conversation's established feature rather than
// dropping its context.
var lowSignalPhrases = map[string]struct{}{
	"continue": {}, "continue please": {}, "please continue": {}, "go on": {},
	"go ahead": {}, "keep going": {}, "carry on": {}, "resume": {}, "proceed": {},
	"next": {}, "next step": {}, "more": {}, "go": {}, "and": {}, "then": {},
	"try again": {}, "retry": {}, "again": {}, "redo": {}, "do it": {},
	"do it again": {}, "fix it": {}, "yes": {}, "yep": {}, "yeah": {}, "ok": {},
	"okay": {}, "k": {}, "sure": {},
}

// isLowSignalPrompt reports whether a prompt is a short continuation that carries
// no new topic of its own. Such a prompt inherits the conversation's established
// feature; a longer, substantive prompt that simply fails to resolve does not —
// its prior context is dropped rather than wrongly carried forward.
func isLowSignalPrompt(prompt string) bool {
	p := strings.ToLower(strings.TrimSpace(prompt))
	p = strings.Trim(p, " \t\r\n.!?,;:")
	if p == "" {
		return true
	}
	if _, ok := lowSignalPhrases[p]; ok {
		return true
	}
	return len(strings.Fields(p)) <= 3
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
			if top, ok := resolveOnePrompt(user, catalog); ok {
				current = top.Key
			} else if !isLowSignalPrompt(user) {
				// Substantive but unrelated turn: it neither starts a feature nor
				// attaches to the running one, so its content can't leak into a
				// feature's summary.
				continue
			}
		}
		if current != "" {
			buckets[current] = append(buckets[current], turn)
		}
	}
	return buckets
}
