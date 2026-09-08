package flowgate

import "testing"

func TestFormatRequirementCard_NamesACAndSSEdit(t *testing.T) {
	got := FormatRequirementCard([]string{"AC-9", "AC-10"})
	if got == "" {
		t.Fatal("empty")
	}
	for _, part := range []string{"AC-9", "AC-10", "SS edit", "locked SS"} {
		if !containsStr(got, part) {
			t.Fatalf("missing %q in %q", part, got)
		}
	}
}
