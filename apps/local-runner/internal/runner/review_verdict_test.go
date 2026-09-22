package runner

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/agentpack"
)

// Task-338 (CP-62 P-2): reviewer verdicts are schema'd, per-AC, evidence-
// backed rows. All provider adapters share parseReviewOutcomeInput, so shape
// validation lands at the one shared parse point (cross-provider Case 1).

func verdictArgs() map[string]any {
	return map[string]any{
		"status":   "changes_requested",
		"feedback": "AC-2 fails",
		"verdicts": []any{
			map[string]any{
				"ac_id":   "AC-1",
				"verdict": "pass",
				"evidence": []any{
					map[string]any{"path": "stringutil/string.go", "line": 12, "excerpt": "return reversed"},
				},
			},
			map[string]any{
				"ac_id":   "AC-2",
				"verdict": "fail",
				"evidence": []any{
					map[string]any{"path": "stringutil/string_test.go", "line": 30, "excerpt": "Reverse(\"\") != \"\""},
				},
				"note": "empty input breaks",
			},
		},
	}
}

// Scenario: Reviewer trả về đầy đủ các AC kèm bằng chứng hợp lệ
func TestSubmitReviewOutcome_ValidSchema_Accepted(t *testing.T) {
	in, err := parseReviewOutcomeInput(verdictArgs())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(in.Verdicts) != 2 {
		t.Fatalf("verdicts=%d want 2", len(in.Verdicts))
	}
	if in.Verdicts[0].ACID != "AC-1" || in.Verdicts[0].Verdict != "pass" {
		t.Fatalf("row0 = %+v", in.Verdicts[0])
	}
	if len(in.Verdicts[0].Evidence) != 1 || in.Verdicts[0].Evidence[0].Path != "stringutil/string.go" || in.Verdicts[0].Evidence[0].Line != 12 {
		t.Fatalf("evidence0 = %+v", in.Verdicts[0].Evidence)
	}
	if in.Verdicts[1].Note != "empty input breaks" {
		t.Fatalf("note = %q", in.Verdicts[1].Note)
	}
}

// Scenario: Verdict sai enum / thiếu ac_id / evidence thiếu path -> tool error (reprompt in-turn)
func TestSubmitReviewOutcome_InvalidShape_Rejected(t *testing.T) {
	cases := []struct {
		name string
		mut  func(map[string]any)
		want string
	}{
		{"bad-enum", func(a map[string]any) {
			a["verdicts"] = []any{map[string]any{"ac_id": "AC-1", "verdict": "maybe"}}
		}, "verdict must be pass|fail|blocked"},
		{"missing-ac-id", func(a map[string]any) {
			a["verdicts"] = []any{map[string]any{"verdict": "pass"}}
		}, "ac_id is required"},
		{"evidence-missing-path", func(a map[string]any) {
			a["verdicts"] = []any{map[string]any{"ac_id": "AC-1", "verdict": "pass",
				"evidence": []any{map[string]any{"line": 3}}}}
		}, "evidence.path is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := verdictArgs()
			tc.mut(args)
			_, err := parseReviewOutcomeInput(args)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err=%v, want containing %q", err, tc.want)
			}
		})
	}
}

// Scenario: Reviewer thiếu 1 AC trong danh sách -> runner reprompt đúng 1 lần
// (tool error nêu đích danh AC còn thiếu; model tự sửa trong turn — fail-closed
// cuối cùng là hợp đồng hub-done-no-verdict của CP-61).
func TestSubmitReviewOutcome_MissingAC_RepromptOnce(t *testing.T) {
	in, err := parseReviewOutcomeInput(verdictArgs())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	err = ValidateReviewOutcomeVerdicts(in, []string{"AC-1", "AC-2", "AC-3"})
	if err == nil {
		t.Fatalf("expected missing-AC error")
	}
	if !strings.Contains(err.Error(), "AC-3") {
		t.Fatalf("error must name the missing AC: %v", err)
	}
	// Full coverage passes.
	if err := ValidateReviewOutcomeVerdicts(in, []string{"AC-1", "AC-2"}); err != nil {
		t.Fatalf("full coverage: %v", err)
	}
}

// Scenario: AC list được trích xuất deterministic từ artifact đã khóa (SS-13)
func TestReviewerPrompt_InjectsACListFromLockedArtifact(t *testing.T) {
	md := "# Task-904\n\n## 6. Acceptance Check\n\n- [ ] AC-1: Reverse(\"hello\") == \"olleh\"\n- [ ] AC-2: Reverse(\"\") == \"\"\n\n## AC-3 notes\n"
	got := ExtractACIDs(md)
	want := []string{"AC-1", "AC-2", "AC-3"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}

// Scenario: Hub forward raw rows nguyên vẹn sang node đích qua back-edge —
// payloadMap phải khai báo verdicts -> payload.verdicts (data-level, hub không paraphrase).
func TestHubForwarding_PreservesRawVerdictsIdentity(t *testing.T) {
	face, ok, err := agentpack.LoadBuiltinToolFace("submit_review_outcome")
	if err != nil || !ok {
		t.Fatalf("load face: %v %v", ok, err)
	}
	if face.PayloadMap["verdicts"] != "payload.verdicts" {
		t.Fatalf("payloadMap[verdicts]=%q want payload.verdicts", face.PayloadMap["verdicts"])
	}
}
