package flowgate

import "testing"

// Task-339 (CP-62 P-3): or-explained gains a structured field
// (TurnResult.DodExplanation) alongside the legacy text-match heuristics.

func dodOpenTurn() TurnResult {
	return TurnResult{
		DodTransitionedToDone: true,
		DodStatus:             DodStatus{Total: 2, Checked: 1, OpenItems: []string{"AC-3: unicode round-trip"}},
	}
}

// Scenario: Giải trình DOD còn dở dang bằng struct có schema -> Cho phép hoàn thành kèm cảnh báo
func TestRDodComplete_StructuredExplanation_Pass(t *testing.T) {
	tr := dodOpenTurn()
	tr.DodExplanation = &DodExplanation{Explanation: "Deferred to sprint 2: unicode collation needs a dependency bump", ReferencingAC: "AC-3"}
	v := checkRule(Rule{ID: "r-dod-complete", Trigger: "marked_done_with_open_dod", Action: "block"}, tr)
	if v == nil {
		t.Fatalf("expected violation (downgraded to warn)")
	}
	if v.Rule.Action != "warn" {
		t.Fatalf("action=%q want warn (explained)", v.Rule.Action)
	}
}

// Scenario: Trường giải trình rỗng hoặc sai format -> Chặn không cho phép hoàn thành
func TestRDodComplete_MalformedExplanation_Block(t *testing.T) {
	tr := dodOpenTurn()
	tr.DodExplanation = &DodExplanation{Explanation: "   "}
	v := checkRule(Rule{ID: "r-dod-complete", Trigger: "marked_done_with_open_dod", Action: "block"}, tr)
	if v == nil {
		t.Fatalf("expected block violation")
	}
	if v.Rule.Action != "block" {
		t.Fatalf("action=%q want block (blank explanation)", v.Rule.Action)
	}
}

// Scenario: Tương thích ngược: Chuỗi giải trình kiểu cũ vẫn được chấp nhận trong giai đoạn chuyển tiếp
func TestRDodComplete_LegacyTextMatch_BackwardCompatible(t *testing.T) {
	tr := dodOpenTurn()
	tr.FinalMessage = "Item deferred to the next sprint because the dependency is not merged yet."
	v := checkRule(Rule{ID: "r-dod-complete", Trigger: "marked_done_with_open_dod", Action: "block"}, tr)
	if v == nil || v.Rule.Action != "warn" {
		t.Fatalf("legacy text-match must still explain: %+v", v)
	}
}

// Edge: structured explanation wins even when the prose path would miss.
func TestRDodComplete_StructuredBeatsSilentProse(t *testing.T) {
	tr := dodOpenTurn()
	tr.DodExplanation = &DodExplanation{Explanation: "Open item intentionally parked; see CP-62 handoff.", ReferencingAC: "AC-3"}
	if !hasValidDodExplanation(tr) {
		t.Fatalf("structured explanation must validate")
	}
}
