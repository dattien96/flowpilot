package runner

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"flowpilot-runner/internal/promptpacker"
)

// Task-453 (CP-89): forward entry-prompt packaging. When a pending run's
// explicit forwardFlow turn starts the pinned flow, the entry child must see
// BOTH the user's forward text and the settled chat transcript that preceded
// it — packed under the entry node's hard context budget, oldest transcript
// material degrading first, the forward text pinned intact.

// forwardPromptBudgetTokens is the fallback budget when the resolved flow's
// entry node declares no contextProfile budget (or the def can't be read at
// pack time — fences already passed, so this is a rare degrade). Matches the
// promptpacker default ceiling (DefaultPackerOptions: 8000 tokens).
const forwardPromptBudgetTokens = int64(8000)

// forwardPromptReserveBytes covers section headers and joiners PackPrompt adds
// on top of section content, so the assembled prompt stays under the byte cap.
const forwardPromptReserveBytes = 512

// ForwardPromptPackage is the deterministic bundle handed to the flow's entry
// child on a forward turn (Task-453 §Code Guide).
type ForwardPromptPackage struct {
	Prompt        string
	TurnsIncluded int
	Bytes         int
	Degraded      bool
}

// settledChatTurnsForRun returns the run's settled user/assistant transcript
// pairs oldest→newest. The runner's own event store is the transcript source
// of truth — transcriptTurnsFromRun already reduces rs.events to visible
// prompt/reply pairs, which inherently drops system/tool frames (they never
// produce a TurnStarted-with-prompt/MessageCompleted pair). User text that IS
// a system prompt (gate reprompts, flow-engine notes) is excluded by the same
// isSystemPrompt contract the rest of the transcript pipeline uses.
//
// Deviation from the suggested signature: returns []transcriptTurn — the
// existing settled-turn shape (User/Assistant + seq range) rather than a new
// ChatTurn type; transcriptTurn already carries exactly the roles required.
func settledChatTurnsForRun(rs *interactiveRun) []transcriptTurn {
	if rs == nil {
		return nil
	}
	turns := transcriptTurnsFromRun(rs)
	// transcriptTurnsFromRun flushes a trailing TurnStarted even when no
	// completion ever arrived — that tail is IN FLIGHT, not settled (a turn
	// that closed via TurnFailed/TurnCompleted or by the next TurnStarted
	// already flushed correctly). Drop it: a forward must never smuggle the
	// half-written turn into the entry child.
	if n := len(rs.events); n > 0 {
		openTail := false
		for _, ev := range rs.events {
			switch ev.Type {
			case EventTurnStarted:
				if strings.TrimSpace(ev.Prompt) != "" {
					openTail = true
				}
			case EventTurnCompleted, EventTurnFailed:
				openTail = false
			}
		}
		if openTail && len(turns) > 0 {
			turns = turns[:len(turns)-1]
		}
	}
	out := make([]transcriptTurn, 0, len(turns))
	for _, tr := range turns {
		if isSystemPrompt(tr.User) {
			continue
		}
		// R4-4: a turn that closed via TurnFailed is not settled — its prompt
		// and partial reply must never reach the entry child as context.
		if tr.Failed {
			continue
		}
		out = append(out, tr)
	}
	return out
}

// forwardChatTranscriptTurns reads the settled discussion the forward must
// carry from the durable CHAT transcript (CP-59 SSOT). A chat that reattached
// a leg or switched providers mints a fresh run whose rs.events start empty,
// so a leg-scoped read would silently drop every pre-switch turn the user
// discussed (SS/SD/CP review context); the transcript store spans all legs.
//
// Caller must NOT hold s.mu — this is chat-store I/O and the writer mutex
// exists precisely so such I/O never extends the service lock (SD26-S-2).
// Returns nil when no usable transcript exists; the caller then falls back
// to the leg's own settled events.
func (s *InteractiveService) forwardChatTranscriptTurns(ctx context.Context, chatID string) []transcriptTurn {
	if chatID == "" {
		return nil
	}
	w := s.ensureChatTranscriptWriter()
	if w == nil || w.store == nil {
		return nil
	}
	records, err := w.store.ReadChatRecords(ctx, chatID, 0, 0)
	if err != nil || len(records) == 0 {
		return nil
	}
	turns, _ := chatTurnsAndActions(records)
	out := make([]transcriptTurn, 0, len(turns))
	for _, tr := range turns {
		if isSystemPrompt(tr.User) {
			continue
		}
		out = append(out, tr)
	}
	// A trailing unanswered turn is never settled context (in-flight or
	// interrupted) — and this forward's own turn_started, when already
	// recorded, would otherwise ship the forward text twice (it is the
	// mandatory "Forward request" section). Mid-chat unanswered turns stay:
	// the prompt still happened and the discussion continued after it.
	if n := len(out); n > 0 && strings.TrimSpace(out[n-1].Assistant) == "" {
		out = out[:n-1]
	}
	return out
}

// forwardEntryBudgetTokens resolves the pinned flow's entry-node context
// budget: the first spawnable entry node's contextProfile.MaxEstPromptTokens
// (Task-341 schema). Falls back to the packer default when the flow declares
// no profile — pack time runs after the fences, so resolution failure is not
// expected but must still bound the package.
func (s *InteractiveService) forwardEntryBudgetTokens(ctx context.Context, flowRef string) int64 {
	record, err := NewFlowDefinitionResolver(s.flowDefinitionStore).ResolveFlowRef(ctx, flowRef)
	if err != nil {
		return forwardPromptBudgetTokens
	}
	for _, node := range entryDelegateNodes(record.Definition) {
		if prof := record.Definition.ContextProfiles[node.ContextProfile]; prof.MaxEstPromptTokens > 0 {
			return int64(prof.MaxEstPromptTokens)
		}
	}
	return forwardPromptBudgetTokens
}

// buildForwardPromptPackage packs the forward text plus the run's settled
// chat transcript under a hard token budget. Ordering and degradation are
// deterministic for identical state:
//
//   - transcript turns go oldest→newest through packConversationTurns — the
//     existing newest-kept/oldest-dropped packer with omission markers, so the
//     OLDEST material degrades first exactly like chat handoff packing;
//   - the forward text is a mandatory section — pinned intact, never dropped
//     or truncated (PackPrompt's mandatory-kinds contract);
//   - PackPrompt assembles with deterministic section order + audit.
func (s *InteractiveService) buildForwardPromptPackage(rs *interactiveRun, forwardText string, budget int64, chatTurns []transcriptTurn) (ForwardPromptPackage, *apiErr) {
	if budget <= 0 {
		budget = forwardPromptBudgetTokens
	}
	fwd := strings.TrimSpace(forwardText)
	// R4-5: the forward text is a mandatory section — PackPrompt will never
	// truncate it, so a text that alone overflows the budget would ship
	// oversize while the forward still succeeds. Reject explicitly instead:
	// fail-closed, typed, and the user can shorten the message or pick a
	// flow whose entry node carries a bigger context profile.
	if fwd != "" && int64(promptpacker.EstimateTokens(fwd)) > budget {
		return ForwardPromptPackage{}, newAPIErr(http.StatusUnprocessableEntity, "forward_prompt_too_large",
			fmt.Sprintf("forward text (%d est. tokens) exceeds the entry node's prompt budget (%d tokens); shorten the message", promptpacker.EstimateTokens(fwd), budget))
	}
	turns := chatTurns
	if len(turns) == 0 {
		turns = settledChatTurnsForRun(rs)
	}
	// The transcript's byte cap = budget − forward text − envelope reserve.
	// Tokens ≈ bytes/4 (EstimateTokens), so the transcript cap in bytes is
	// budget*4 minus what the forward text and headers will occupy.
	transcriptBudget := int(budget)*4 - len(fwd) - forwardPromptReserveBytes
	if transcriptBudget < 0 {
		transcriptBudget = 0
	}
	body, included, omitted, truncated := packConversationTurns(turns, transcriptBudget)
	sections := []promptpacker.PromptSection{
		{Kind: promptpacker.SectionRawExcerpt, Title: "Prior chat transcript", Content: body, Priority: 1},
		{Kind: promptpacker.SectionCurrentTask, Title: "Forward request", Content: fwd, Priority: 2},
	}
	packed, report, perr := promptpacker.PackPrompt(sections, promptpacker.SectionBudget{
		TotalMaxTokens: int(budget),
	})
	if perr != nil {
		return ForwardPromptPackage{}, newAPIErr(http.StatusUnprocessableEntity, "forward_prompt_pack_failed", perr.Error())
	}
	// R5-4: TotalMaxTokens bounds section CONTENT only — PackPrompt adds the
	// section headers/joiners on top, and the forward section is mandatory
	// (never truncated). Measure the ASSEMBLED prompt against the entry
	// node's hard cap; an over-cap assembly is a typed rejection, not a
	// silent oversize launch.
	if int64(promptpacker.EstimateTokens(packed)) > budget {
		return ForwardPromptPackage{}, newAPIErr(http.StatusUnprocessableEntity, "forward_prompt_too_large",
			fmt.Sprintf("assembled forward prompt (%d est. tokens) exceeds the entry node's prompt budget (%d tokens); shorten the message", promptpacker.EstimateTokens(packed), budget))
	}
	return ForwardPromptPackage{
		Prompt:        packed,
		TurnsIncluded: included,
		Bytes:         len(packed),
		Degraded:      truncated || omitted > 0 || len(report.DroppedItems) > 0,
	}, nil
}
