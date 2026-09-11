package runner

import (
	"reflect"
	"testing"

	"flowpilot-runner/internal/agentpack"
)

func TestVibeRequirementFaceReadsFromPack(t *testing.T) {
	packFace, ok, err := agentpack.LoadBuiltinToolFace("vibe-requirement-outcome")
	if err != nil || !ok {
		t.Fatalf("LoadBuiltinToolFace: ok=%v err=%v", ok, err)
	}
	got := vibeRequirementFace()
	if got.Tool != packFace.ID {
		t.Fatalf("tool=%q want %q", got.Tool, packFace.ID)
	}
	if !reflect.DeepEqual(got.Map, packFace.StatusMap) {
		t.Fatalf("map=%v want %v", got.Map, packFace.StatusMap)
	}
}

func TestVibeRequirementToFlowControl(t *testing.T) {
	cases := map[string]string{
		"aligned":            "done",
		"drift_fixable":      "continue",
		"requirement_change": "escalate",
	}
	for verdict, want := range cases {
		fc, err := vibeRequirementToFlowControl(vibeRequirementInput{Verdict: verdict, Summary: "ss"})
		if err != nil {
			t.Fatalf("%s: %v", verdict, err)
		}
		if fc.Status != want {
			t.Fatalf("%s status=%q want %q", verdict, fc.Status, want)
		}
	}
}

func TestParseVibeRequirementInput_RejectsReviewStatus(t *testing.T) {
	if _, err := parseVibeRequirementInput(map[string]any{"verdict": "approved"}); err == nil {
		t.Fatal("approved must not map on vibe face")
	}
}
