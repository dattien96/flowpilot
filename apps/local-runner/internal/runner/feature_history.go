package runner

import (
	"path/filepath"
	"strings"

	"flowpilot-runner/internal/changeledger"
	"flowpilot-runner/internal/featurecatalog"
	"flowpilot-runner/internal/flowgate"
)

func injectFeatureHistoryPrompt(workspace string, prompt string, priorTurns []transcriptTurn) string {
	// A handoff envelope already carries its (correctly source-resolved) feature
	// block, prepended at build time. Never re-inject from the flat envelope text —
	// it embeds gate-reprompt lines naming feature keys and would mis-resolve.
	if isHandoffPrompt(prompt) {
		return prompt
	}
	dotFlowpilotDir := filepath.Join(workspace, ".flowpilot")
	catalog, err := featurecatalog.LoadCatalog(dotFlowpilotDir)
	if err != nil {
		return prompt
	}
	top, ok := resolveInjectionFeature(prompt, priorTurns, catalog)
	if !ok {
		return prompt
	}
	block := composeFeatureBlocks(dotFlowpilotDir, top.Key)
	if strings.TrimSpace(block) == "" {
		return prompt
	}
	return block + "\n\n---\n\n" + prompt
}

// composeFeatureBlocks returns the prior-work (+ prior-discussion) blocks for a
// known feature key, or "" when there is no committed history. Shared by per-turn
// injection and the cross-provider handoff (which resolves its feature from the
// clean source transcript rather than the envelope text).
func composeFeatureBlocks(dotFlowpilotDir string, featureKey string) string {
	ledger, err := changeledger.New(dotFlowpilotDir)
	if err != nil {
		return ""
	}
	history := featurecatalog.HistorySlot(featureKey, ledger)
	if strings.TrimSpace(history) == "" {
		return ""
	}
	combined := history
	if summaryLedger, err := changeledger.NewChatSummaryLedger(dotFlowpilotDir); err == nil {
		if discussion := featurecatalog.ChatSummarySlot(featureKey, summaryLedger); strings.TrimSpace(discussion) != "" {
			combined += "\n\n" + discussion
		}
	}
	return combined
}

// resolveInjectionFeature decides which feature's history to inject for the
// current prompt. The current prompt's own resolution always wins. When it does
// not resolve, the conversation's established feature is inherited ONLY for an
// explicit continuation ("continue"/"try again"). Everything else that fails to
// resolve — a greeting ("hi"), an acknowledgement ("ok"/"no"), or a substantive
// off-topic prompt ("write a haiku about the sea") — injects nothing.
func resolveInjectionFeature(prompt string, priorTurns []transcriptTurn, catalog *featurecatalog.Catalog) (featurecatalog.Candidate, bool) {
	// A system prompt (gate reprompt or cross-provider handoff envelope) resolves on
	// its own process text — it names feature keys, writes change-audit files, and
	// embeds prior conversation — so it must NEVER resolve standalone. Inherit the
	// conversation's established feature instead (on a fresh run that means no block,
	// rather than a feature the envelope merely mentions).
	if isSystemPrompt(prompt) {
		return resolveTurnsFeature(priorTurns, catalog)
	}
	// Otherwise the current prompt's own resolution wins (even when short, e.g. a
	// prompt that names a feature). Only an explicit continuation inherits when it
	// fails to resolve; anything else drops the prior context.
	if top, ok := resolveOnePrompt(prompt, catalog); ok {
		return top, true
	}
	if isContinuationPrompt(prompt) {
		return resolveTurnsFeature(priorTurns, catalog)
	}
	return featurecatalog.Candidate{}, false
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
// confidence threshold. Used to inherit the established feature for a continuation
// prompt (injection), and to pick a run's feature for recording/handoff.
func resolveTurnsFeature(turns []transcriptTurn, catalog *featurecatalog.Catalog) (featurecatalog.Candidate, bool) {
	for i := len(turns) - 1; i >= 0; i-- {
		user := strings.TrimSpace(turns[i].User)
		// System prompts (gate reprompts, handoff envelopes) are transparent: their
		// process-describing text must not drive resolution, so skip them and keep
		// scanning for the real feature.
		if user == "" || isSystemPrompt(user) {
			continue
		}
		if top, ok := resolveOnePrompt(user, catalog); ok {
			return top, true
		}
	}
	return featurecatalog.Candidate{}, false
}

// isGateReprompt reports whether a prompt is a system-issued flow-gate reprompt.
func isGateReprompt(prompt string) bool {
	return strings.HasPrefix(strings.TrimSpace(prompt), flowgate.GateRepromptPrefix)
}

// isHandoffPrompt reports whether a prompt is (or contains) a cross-provider
// handoff envelope. Contains, not HasPrefix: the handoff prompt is built with its
// source feature block prepended, so the marker is no longer at the very start.
func isHandoffPrompt(prompt string) bool {
	return strings.Contains(prompt, handoffPromptPrefix)
}

// isSystemPrompt reports whether a prompt is system-generated (a gate reprompt or
// a handoff envelope). Such prompts describe process / embed prior conversation, so
// their text must not drive feature resolution, recording, or bucketing.
func isSystemPrompt(prompt string) bool {
	return isGateReprompt(prompt) || isHandoffPrompt(prompt)
}

// continuationPhrases are explicit "keep going" instructions that carry no topic
// of their own and so inherit the conversation's established feature. This set is
// deliberately narrow: greetings ("hi"), acknowledgements ("ok"/"yes"/"no"), and
// any other short prompt are NOT continuations — they do not inherit. (A prompt
// that names a feature resolves on its own; everything else drops prior context.)
var continuationPhrases = map[string]struct{}{
	"continue": {}, "continue please": {}, "please continue": {}, "go on": {},
	"go ahead": {}, "keep going": {}, "carry on": {}, "resume": {}, "proceed": {},
	"next": {}, "next step": {}, "more": {}, "go": {}, "and": {}, "then": {},
	"try again": {}, "retry": {}, "again": {}, "redo": {}, "do it": {},
	"do it again": {}, "fix it": {},
}

// isContinuationPrompt reports whether a prompt is an explicit continuation
// ("continue"/"try again"/"do it"…). Only a continuation inherits the
// conversation's established feature; greetings, acknowledgements, and any
// substantive-but-unresolved prompt do not — their prior context is dropped rather
// than wrongly carried forward.
func isContinuationPrompt(prompt string) bool {
	p := strings.ToLower(strings.TrimSpace(prompt))
	p = strings.Trim(p, " \t\r\n.!?,;:")
	if p == "" {
		return false
	}
	_, ok := continuationPhrases[p]
	return ok
}

// bucketTurnsByFeature groups turns by the feature each belongs to. A turn whose
// own prompt resolves sets the running feature; an explicit continuation
// ("continue") attaches to the running feature. This lets a chat that spans two
// features produce one summary per feature from only that feature's turns (no
// mixing).
func bucketTurnsByFeature(turns []transcriptTurn, catalog *featurecatalog.Catalog) map[string][]transcriptTurn {
	buckets := make(map[string][]transcriptTurn)
	current := ""
	for _, turn := range turns {
		if user := strings.TrimSpace(turn.User); user != "" {
			if isSystemPrompt(user) {
				// System prompt (gate reprompt / handoff envelope): attach to the
				// running feature, never start one from its process-describing text
				// (which would resolve to whatever feature key it names).
			} else if top, ok := resolveOnePrompt(user, catalog); ok {
				current = top.Key
			} else if !isContinuationPrompt(user) {
				// Not a continuation (greeting, acknowledgement, or substantive
				// off-topic): it neither starts a feature nor attaches to the running
				// one, so its content can't leak into a feature's summary.
				continue
			}
		}
		if current != "" {
			buckets[current] = append(buckets[current], turn)
		}
	}
	return buckets
}

// featureBucketTurns returns the turns belonging to featureKey — the exact set the
// chat-summary recorder summarizes — falling back to all turns when the bucket is
// empty. The recorder AND the cross-provider handoff both derive the summary
// `state_key` from this set, so they MUST agree on it; computing the key over a
// different turn set (e.g. all turns vs. bucketed turns) would make the handoff's
// state_key never match the stored one, silently disabling hybrid mode.
func featureBucketTurns(turns []transcriptTurn, catalog *featurecatalog.Catalog, featureKey string) []transcriptTurn {
	featureTurns := bucketTurnsByFeature(turns, catalog)[featureKey]
	if len(featureTurns) == 0 {
		featureTurns = turns
	}
	return featureTurns
}
