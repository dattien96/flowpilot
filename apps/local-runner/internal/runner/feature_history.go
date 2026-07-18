package runner

import (
	"path/filepath"
	"strings"

	"flowpilot-runner/internal/changecontract"
	"flowpilot-runner/internal/changeledger"
	"flowpilot-runner/internal/featurecatalog"
	"flowpilot-runner/internal/flowgate"
)

func injectFeatureHistoryPrompt(workspace string, prompt string, priorTurns []transcriptTurn) string {
	// Legacy package-level helper: multi-secret / active-secret verify (tests and
	// one-shot callers). InteractiveService must use injectFeatureHistoryPromptCtx.
	return injectFeatureHistoryPromptWithSecret(workspace, prompt, priorTurns, nil)
}

// MarkerVerificationContext is REQUIRED on the service path (SD-24 §6.6 / Task-252).
// Empty AllowedMarkerIDs fails closed (no suppression → history injected).
type MarkerVerificationContext struct {
	Secret           []byte
	AllowedMarkerIDs []string
}

// injectFeatureHistoryPromptCtx is the service-path helper (CP-51 Task-252).
// Both Secret and non-empty AllowedMarkerIDs are required to suppress history.
func injectFeatureHistoryPromptCtx(workspace, prompt string, priorTurns []transcriptTurn, mv MarkerVerificationContext) string {
	handoff := false
	if len(mv.Secret) > 0 && len(mv.AllowedMarkerIDs) > 0 {
		handoff = isFlowContextHandoffWithSecret(mv.Secret, prompt, mv.AllowedMarkerIDs...)
	}
	// empty allowed set ⇒ fail closed (no handoff suppression)
	if isHandoffPrompt(prompt) || handoff ||
		isFlowEnginePrompt(prompt) || isFlowReviewHandoffPrompt(prompt) {
		return prompt
	}
	return injectFeatureHistoryBody(workspace, prompt, priorTurns)
}

// injectFeatureHistoryPromptWithSecret is kept for package tests / one-shot callers.
// The InteractiveService path must use injectFeatureHistoryPromptCtx.
func injectFeatureHistoryPromptWithSecret(workspace string, prompt string, priorTurns []transcriptTurn, secret []byte) string {
	// A handoff envelope already carries its (correctly source-resolved) feature
	// block, prepended at build time. Never re-inject from the flat envelope text —
	// it embeds gate-reprompt lines naming feature keys and would mis-resolve.
	// A flow context package (Task-169) carries its own history block; skip to
	// avoid duplicating the same feature history in the Coding prompt.
	// Task-224 / BUG-277: skip full Prior work block for flow-engine synthesis
	// and flow review handoffs — history bulk belongs to hub user turns and the
	// first post-context.produce consumer (via package), not every late node.
	handoff := false
	if len(secret) > 0 {
		// Legacy: secret only — tests may omit IDs. Service path never uses this.
		handoff = isFlowContextHandoffWithSecret(secret, prompt)
	} else {
		handoff = isFlowContextHandoff(prompt)
	}
	if isHandoffPrompt(prompt) || handoff ||
		isFlowEnginePrompt(prompt) || isFlowReviewHandoffPrompt(prompt) {
		return prompt
	}
	return injectFeatureHistoryBody(workspace, prompt, priorTurns)
}

func injectFeatureHistoryBody(workspace, prompt string, priorTurns []transcriptTurn) string {
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

// isFlowReviewHandoffPrompt reports a flow-executor auto-advance review brief
// (tryAdvanceFlowFromNode). Those nodes are deliverable-centric (Task-224);
// ledger history must not be re-injected on top of the package already given
// to the first post-context consumer.
func isFlowReviewHandoffPrompt(prompt string) bool {
	p := strings.TrimSpace(prompt)
	return strings.Contains(p, "[flow-engine] Review this result from node")
}

// composeFeatureBlocks returns Canonical Head + prior-work (+ prior-discussion)
// for a known feature key, or "" when the feature has neither a Head nor
// history/discussion. Shared by per-turn injection and cross-provider handoff.
// Task-245: head-only inject when history is empty is intentional.
func composeFeatureBlocks(dotFlowpilotDir string, featureKey string) string {
	var parts []string
	workspace := dotFlowpilotDir
	if filepath.Base(dotFlowpilotDir) == ".flowpilot" {
		workspace = filepath.Dir(dotFlowpilotDir)
	}
	if head, found, err := changecontract.LoadHead(workspace, featureKey); err == nil && found {
		if block := strings.TrimSpace(changecontract.RenderHeadBlock(head)); block != "" {
			parts = append(parts, block)
		}
	}
	ledger, err := changeledger.New(dotFlowpilotDir)
	if err == nil {
		if history := strings.TrimSpace(featurecatalog.HistorySlot(featureKey, ledger)); history != "" {
			parts = append(parts, history)
		}
	}
	if summaryLedger, err := changeledger.NewChatSummaryLedger(dotFlowpilotDir); err == nil {
		if discussion := strings.TrimSpace(featurecatalog.ChatSummarySlot(featureKey, summaryLedger)); discussion != "" {
			parts = append(parts, discussion)
		}
	}
	return strings.Join(parts, "\n\n")
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

// isFlowEnginePrompt reports whether a prompt is an internal flow-engine /
// hub-orchestration message (cohort join notes, synthesis reinvoke, agent
// results ready, UI sub-agent system notes). These embed child/reviewer text
// and must not drive feature resolution (BUG-275 — hub drifted
// calc-core → calc-format → sandbox-meta on synthesis turns).
func isFlowEnginePrompt(prompt string) bool {
	p := strings.TrimSpace(prompt)
	if p == "" {
		return false
	}
	if strings.HasPrefix(p, "[flow-engine]") ||
		strings.HasPrefix(p, "[flow-engine joined result note]") ||
		strings.HasPrefix(p, "[FlowPilot system note — sub-agents") ||
		strings.HasPrefix(p, "[FlowPilot system note - sub-agents") {
		return true
	}
	// Embedded join note + synthesis instruction (maybeAutoReinvokeHubWithNote).
	if strings.Contains(p, "[flow-engine joined result note]") {
		return true
	}
	if strings.Contains(p, "[flow-engine] Agent results ready") ||
		strings.Contains(p, "submit_review_outcome") && strings.Contains(p, "[flow-engine]") {
		return true
	}
	// spawnChildRun composes the child role prompt and delegated work into one
	// provider turn. It is internal orchestration input, never a user message.
	if strings.Contains(p, "[FlowPilot sub-agent — ") || strings.Contains(p, "[FlowPilot sub-agent - ") {
		return true
	}
	return false
}

// isSystemPrompt reports whether a prompt is system-generated (gate reprompt,
// handoff envelope, or flow-engine/hub orchestration). Such prompts describe
// process / embed prior conversation, so their text must not drive feature
// resolution, recording, or bucketing.
func isSystemPrompt(prompt string) bool {
	return isGateReprompt(prompt) || isHandoffPrompt(prompt) || isFlowEnginePrompt(prompt)
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
