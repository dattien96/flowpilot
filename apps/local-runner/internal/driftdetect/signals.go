package driftdetect

import "strings"

// Task-335 §4 T-2 / §11: the conversation-signal analysers. All heuristics
// are deterministic string/set checks over the TurnSummary the caller already
// derived from its gate observations — 0 LLM, 0 token cost (CP-23 D-3).

// checkRepeatedTestFailures reports whether the SAME test name failed in the
// current turn AND in the immediately previous turn (>=2 consecutive failures
// — Task-335 §11). Open Question resolution (Task-335 §9): the loop must be
// the same test name; two DIFFERENT failing tests are not a loop.
func checkRepeatedTestFailures(history []TurnSummary, current TurnSummary) bool {
	if len(current.TestResults) == 0 || len(history) == 0 {
		return false
	}
	prev := history[len(history)-1]
	if len(prev.TestResults) == 0 {
		return false
	}
	failed := make(map[string]struct{}, len(current.TestResults))
	for _, t := range current.TestResults {
		if name := strings.TrimSpace(t); name != "" {
			failed[name] = struct{}{}
		}
	}
	for _, t := range prev.TestResults {
		if _, ok := failed[strings.TrimSpace(t)]; ok {
			return true
		}
	}
	return false
}

// apologyPatterns are the apology / filler phrases the Task-335 spec lists
// (§11: "Tôi rất xin lỗi", "Bạn hoàn toàn đúng", "I apologize", "Let me try
// again", ...). Matching is case-insensitive substring match on the
// whitespace-normalized message. Deliberately specific phrases, not bare
// "sorry" — a lone polite "sorry" must never read as drift.
var apologyPatterns = []string{
	// Vietnamese
	"tôi rất xin lỗi",
	"tôi xin lỗi",
	"xin lỗi vì",
	"xin lỗi bạn",
	"bạn hoàn toàn đúng",
	"bạn nói đúng",
	"rất xin lỗi",
	// English
	"i apologize",
	"i'm sorry",
	"i am sorry",
	"my apologies",
	"you're right",
	"you are right",
	"let me try again",
	"let me retry",
	"sorry for the confusion",
	"sorry for the mistake",
}

// checkApologyPatterns reports whether an apology/filler LOOP is in progress.
//
// DEVIATION from the Task-335 §11 sketch (signature took only the message) —
// REQUIRED by the §10 tests and the resolved Open Question: a loop needs >=2
// CONSECUTIVE turns ("Vòng lặp xin lỗi cần >= 2 lượt mới tính, 1 lần xin lỗi
// đơn lẻ là bình thường"), so the analyser must consider the history too. The
// signal fires only when the current turn AND the immediately previous turn
// both contain an apology/filler phrase; a single apology never triggers.
func checkApologyPatterns(history []TurnSummary, current TurnSummary) bool {
	if !messageHasApologyPattern(current.FinalMessage) {
		return false
	}
	if len(history) == 0 {
		return false
	}
	return messageHasApologyPattern(history[len(history)-1].FinalMessage)
}

// messageHasApologyPattern matches any apology/filler phrase in the message,
// case-insensitive with whitespace collapsed so line-wrapped phrases still
// match deterministically.
func messageHasApologyPattern(message string) bool {
	normalized := normalizeForMatch(message)
	if normalized == "" {
		return false
	}
	for _, p := range apologyPatterns {
		if strings.Contains(normalized, p) {
			return true
		}
	}
	return false
}

// normalizeForMatch lowercases and collapses all whitespace runs to single
// spaces (deterministic, locale-independent byte-level lowering).
func normalizeForMatch(message string) string {
	return strings.Join(strings.Fields(strings.ToLower(message)), " ")
}

// checkZeroDeltaProgress reports a zero-progress turn: heavy token burn with
// no file delta (Task-335 §11). CP-23 R-2 guard: only counts when
// TokensConsumed exceeds the minimum threshold (>2000 tokens) so legitimate
// reasoning-only turns are never flagged as drift.
func checkZeroDeltaProgress(current TurnSummary) bool {
	return current.TokensConsumed > zeroDeltaTokenGuard && len(current.FilesChanged) == 0
}

// zeroDeltaTokenGuard is the CP-23 R-2 minimum token threshold ("> 2000
// tokens") for the zero-delta signal.
const zeroDeltaTokenGuard = 2000
