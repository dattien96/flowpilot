package runner

import "testing"

func TestOpencodeReasoningVariantID(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"minimal", "minimal", true},
		{"low", "low", true},
		{"medium", "medium", true},
		{"high", "high", true},
		{"xhigh", "xhigh", true},
		{"max", "max", true},
		{"ultra", "max", true},
		{"", "", false},
		{"unknown", "", false},
	}
	for _, tc := range cases {
		got, ok := opencodeReasoningVariantID(tc.in)
		if ok != tc.ok || got != tc.want {
			t.Fatalf("opencodeReasoningVariantID(%q)=%q,%v want %q,%v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestOpencodeReasoningDegradesToDefault(t *testing.T) {
	if _, ok := opencodeReasoningVariantID("turbo"); ok {
		t.Fatal("expected false for unknown effort")
	}
	if _, ok := opencodeReasoningVariantID(""); ok {
		t.Fatal("expected false for empty")
	}
}

func TestOpencodeInjectSelectedSkills(t *testing.T) {
	// Verify promptPrep wiring: adapter's preparePrompt appends nothing extra beyond injected skills
	// This is a smoke test that the adapter's promptPrep can be set to injectSelectedSkills.
	r, _ := New(".")
	d := newOpencodeDispatcher(nil, nil)
	a := newOpencodeAdapter(d, r.workspace)
	a.promptPrep = func(req TurnRequest) string {
		return r.injectSelectedSkills(r.workspace, req.Prompt, req.SelectedSkills)
	}
	req := TurnRequest{Prompt: "hello", SelectedSkills: []SkillSelection{{Name: "test", Source: "local"}}}
	// Should not panic
	_ = a.preparePrompt(req)
}
