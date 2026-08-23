package runner

import "testing"

// Adapter wiring for read-only chat postures (Task-xxx/CA-xxx): scan/plan must
// force every provider into a GATED permission mode (no bypass) so the runner's
// read-only policy sees every tool and can approve reads / deny writes. The
// profile's YOLO flag is deliberately ignored for scan/plan.

func TestResolveYoloPostureForChatPosture_ScanForcesGatedModes(t *testing.T) {
	// Even with yolo=true, scan/plan must NOT use bypass/approval-never modes.
	for _, posture := range []string{ChatPostureScan, ChatPosturePlan} {
		p := resolveYoloPostureForChatPosture(true, false, posture)
		if p.ClaudePermissionMode != "default" {
			t.Fatalf("%s claude mode = %q, want default (gated)", posture, p.ClaudePermissionMode)
		}
		if p.CodexApprovalMode != "untrusted" {
			t.Fatalf("%s codex approval mode = %q, want untrusted", posture, p.CodexApprovalMode)
		}
		if p.GrokPermissionMode != "" {
			t.Fatalf("%s grok mode = %q, want empty (gated)", posture, p.GrokPermissionMode)
		}
		if p.RunnerAutoApprove {
			t.Fatalf("%s must clear RunnerAutoApprove (read-only policy owns decisions)", posture)
		}
	}
}

func TestResolveYoloPostureForChatPosture_CodeKeepsYolo(t *testing.T) {
	p := resolveYoloPostureForChatPosture(true, false, ChatPostureCode)
	if p.ClaudePermissionMode != "bypassPermissions" || !p.RunnerAutoApprove {
		t.Fatalf("code+yolo posture = %+v, want bypassPermissions + auto-approve", p)
	}
	// Empty posture (flow children / unspecified) behaves like code today.
	p2 := resolveYoloPostureForChatPosture(true, false, "")
	if p2.ClaudePermissionMode != "bypassPermissions" || !p2.RunnerAutoApprove {
		t.Fatalf("empty+yolo posture = %+v, want bypassPermissions + auto-approve", p2)
	}
}

func TestCodexYoloDeriveForChatPosture(t *testing.T) {
	// Scan with yolo=true: codex must still route approvals through the bridge.
	sandbox, mode := codexYoloDeriveForChatPosture(true, false, ChatPostureScan)
	if sandbox != "workspace-write" || mode != "untrusted" {
		t.Fatalf("codex scan derive = (%q,%q), want workspace-write/untrusted", sandbox, mode)
	}
	// Code posture keeps normal YOLO derivation.
	sandbox, mode = codexYoloDeriveForChatPosture(true, false, ChatPostureCode)
	if sandbox != "danger-full-access" || mode != "never" {
		t.Fatalf("codex code derive = (%q,%q), want danger-full-access/never", sandbox, mode)
	}
}

func TestClaudeArgs_ScanDoesNotAttachPermissionPromptToolOrBypass(t *testing.T) {
	// Claude adapter derives its args from resolveYoloPostureForChatPosture; a
	// scan posture must not attach --permission-prompt-tool bypass flags and must
	// keep --permission-mode default.
	posture := resolveYoloPostureForChatPosture(true, false, ChatPostureScan)
	args := claudeArgs(posture, "", "", "", "", nil)
	for _, a := range args {
		if a == "--permission-prompt-tool" {
			t.Fatalf("scan posture must not attach --permission-prompt-tool, args=%v", args)
		}
	}
	if posture.ClaudePermissionMode != "default" {
		t.Fatalf("scan claude permission mode = %q, want default", posture.ClaudePermissionMode)
	}
}