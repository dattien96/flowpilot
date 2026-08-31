package runner

import (
	"strings"
	"testing"
)

// Task-260: Chat Plan/Code auto-mention safe-fix-contract (pointer only).

func TestMergeChatSafeFixContractSkills_PlanAndCodeInject(t *testing.T) {
	for _, posture := range []string{ChatPosturePlan, ChatPostureCode} {
		got := mergeChatSafeFixContractSkills(posture, "chat", false, nil)
		if len(got) != 1 || !strings.EqualFold(got[0].Name, safeFixContractSkillName) {
			t.Fatalf("posture %q: got %v, want [safe-fix-contract]", posture, got)
		}
		if got[0].Source != "builtin" {
			t.Fatalf("posture %q: source = %q, want builtin", posture, got[0].Source)
		}
	}
}

func TestMergeChatSafeFixContractSkills_ScanNonAndEmptyDoNotInject(t *testing.T) {
	for _, posture := range []string{ChatPostureScan, ChatPostureNon, "", "evil"} {
		got := mergeChatSafeFixContractSkills(posture, "chat", false, nil)
		if len(got) != 0 {
			t.Fatalf("posture %q must NOT inject, got %v", posture, got)
		}
	}
}

func TestMergeChatSafeFixContractSkills_FlowEngineDrivenNeverInjects(t *testing.T) {
	for _, posture := range []string{ChatPosturePlan, ChatPostureCode} {
		got := mergeChatSafeFixContractSkills(posture, "chat", true, nil)
		if len(got) != 0 {
			t.Fatalf("flowEngineDriven true must NOT inject for posture %q, got %v", posture, got)
		}
	}
}

func TestMergeChatSafeFixContractSkills_NonChatRunNeverInjects(t *testing.T) {
	for _, runKind := range []string{"workflow", "flow", "", "agent"} {
		got := mergeChatSafeFixContractSkills(ChatPostureCode, runKind, false, nil)
		if len(got) != 0 {
			t.Fatalf("runKind %q must NOT inject, got %v", runKind, got)
		}
	}
}

func TestMergeChatSafeFixContractSkills_DedupWhenAlreadySelected(t *testing.T) {
	cases := [][]SkillSelection{
		{{Name: "safe-fix-contract", Source: "builtin"}},
		{{Name: "Safe-Fix-Contract", Source: "slash_picker"}},
		{{Name: "other", Source: "builtin"}, {Name: "safe-fix-contract", Source: "builtin"}},
		{{Name: " SAFE-FIX-CONTRACT ", Source: "builtin"}},
	}
	for i, sel := range cases {
		got := mergeChatSafeFixContractSkills(ChatPosturePlan, "chat", false, sel)
		if len(got) != len(sel) {
			t.Fatalf("case %d: dedup failed: input %v got %v (len mismatch)", i, sel, got)
		}
		// No duplicate added: count occurrences of safe-fix-contract must stay 1
		count := 0
		for _, s := range got {
			if strings.EqualFold(strings.TrimSpace(s.Name), safeFixContractSkillName) {
				count++
			}
		}
		if count != 1 {
			t.Fatalf("case %d: expected 1 safe-fix-contract, got %d in %v", i, count, got)
		}
	}
}

func TestMergeChatSafeFixContractSkills_AppendsPreservingExisting(t *testing.T) {
	sel := []SkillSelection{{Name: "other-skill", Source: "builtin"}}
	got := mergeChatSafeFixContractSkills(ChatPostureCode, "chat", false, sel)
	if len(got) != 2 {
		t.Fatalf("got len %d, want 2", len(got))
	}
	if got[0].Name != "other-skill" {
		t.Fatalf("first element must be preserved, got %v", got[0])
	}
	if !strings.EqualFold(got[1].Name, safeFixContractSkillName) {
		t.Fatalf("second element must be safe-fix-contract, got %v", got[1])
	}
}

func TestMergeChatSafeFixContractSkills_ProviderAgnostic(t *testing.T) {
	// Helper is provider-agnostic (no providerKey branch). Prove by calling same
	// helper for every provider key conceptually — result must be identical.
	sel := []SkillSelection{{Name: "x", Source: "builtin"}}
	for _, provider := range []ProviderKey{ProviderKeyClaude, ProviderKeyCodex, ProviderKeyGrok, ProviderKeyGemini, ProviderKeyOpencode} {
		_ = provider // not used by helper — proves no provider branch needed
		got := mergeChatSafeFixContractSkills(ChatPostureCode, "chat", false, sel)
		if len(got) != 2 || !strings.EqualFold(got[1].Name, safeFixContractSkillName) {
			t.Fatalf("provider %q: merge must inject safe-fix-contract agnostically, got %v", provider, got)
		}
	}
}

func TestShouldAutoInjectSafeFixContract(t *testing.T) {
	tests := []struct {
		posture          string
		runKind          string
		flowEngineDriven bool
		want             bool
	}{
		{ChatPosturePlan, "chat", false, true},
		{ChatPostureCode, "chat", false, true},
		{ChatPostureScan, "chat", false, false},
		{ChatPostureNon, "chat", false, false},
		{"", "chat", false, false},
		{ChatPosturePlan, "chat", true, false},
		{ChatPostureCode, "workflow", false, false},
	}
	for _, tc := range tests {
		got := shouldAutoInjectSafeFixContract(tc.posture, tc.runKind, tc.flowEngineDriven)
		if got != tc.want {
			t.Fatalf("shouldAutoInject(%q,%q,%v)=%v want %v", tc.posture, tc.runKind, tc.flowEngineDriven, got, tc.want)
		}
	}
}

func TestInjectSelectedSkillsWithSafeFixContractPointer(t *testing.T) {
	// End-to-end: merged selection fed into injectSelectedSkills must emit the pointer block.
	// Use a temp dir with no real SKILL.md on disk — injectSelectedSkills still emits the pointer entry.
	r := &Runner{workspace: t.TempDir()}
	sel := mergeChatSafeFixContractSkills(ChatPosturePlan, "chat", false, nil)
	if len(sel) != 1 {
		t.Fatalf("merged len=%d", len(sel))
	}
	out := r.injectSelectedSkills(r.workspace, "do the task", sel)
	if !strings.Contains(out, "Selected Skills") {
		t.Fatalf("inject must contain Selected Skills header, got %q", out)
	}
	if !strings.Contains(strings.ToLower(out), "safe-fix-contract") {
		t.Fatalf("inject must mention safe-fix-contract, got %q", out)
	}
}

func TestInjectSelectedSkills_ScanDoesNotContainPointer(t *testing.T) {
	r := &Runner{workspace: t.TempDir()}
	sel := mergeChatSafeFixContractSkills(ChatPostureScan, "chat", false, nil)
	out := r.injectSelectedSkills(r.workspace, "do the task", sel)
	if strings.Contains(strings.ToLower(out), "safe-fix-contract") {
		t.Fatalf("scan must NOT inject safe-fix-contract, got %q", out)
	}
	if out != "do the task" {
		t.Fatalf("scan with no skills must be bare prompt, got %q", out)
	}
}
