package runner

// Phase 4 (04-04): YOLO as the single source of truth.
//
// One per-run/step YOLO boolean configures BOTH layers at once — the Codex thread
// (sandbox + approval mode) and the FlowPilot runner approval bridge — so the two
// can never drift. This replaces the old habit of pinning a standalone
// `default_tools_approval_mode = "approve"` somewhere and hoping it lines up with
// runner behavior.
//
//	resolveYoloPosture(true)  -> {"danger-full-access", "never",      true}
//	resolveYoloPosture(false) -> {"workspace-write",   "untrusted",  false}
//
// Input flows in via PromptExecutionRequest.YoloMode (types.go) / session-start
// ApprovalMode+AllowWrite; it is resolved per run/step and carried on the turn.

// YoloPosture is the resolved configuration for one turn. CodexSandbox and
// CodexApprovalMode are applied to the Codex thread/turn params; RunnerAutoApprove
// drives the approval bridge (YOLO=true → auto-approve + audit as gating-disabled).
type YoloPosture struct {
	CodexSandbox      string
	CodexApprovalMode string
	// ClaudePermissionMode is the Claude Code --permission-mode derived from the same
	// YOLO value (07 plan): yolo=true -> "bypassPermissions" (full auto, no prompts),
	// yolo=false -> "default" (gated; the runner surfaces permission_required + the
	// --permission-prompt-tool / in-stream control_request route, deny blocks).
	ClaudePermissionMode string
	RunnerAutoApprove    bool
	// GrokPermissionMode is read only by the Grok adapter (CP-46/Task-208):
	// yolo=true -> "bypassPermissions" (the only mode Grok's own docs say takes
	// effect via flag/session), yolo=false -> "" (default; live per-call gating
	// via session/request_permission). Additive field — codex/claude callers
	// never read it, so resolveYoloPosture's codex/claude outputs are unchanged.
	GrokPermissionMode string
}

// resolveYoloPosture is the SSOT mapping. Keep this the only place the YOLO boolean
// becomes provider/runner config.
func resolveYoloPosture(yolo bool) YoloPosture {
	if yolo {
		return YoloPosture{
			CodexSandbox:         "danger-full-access",
			CodexApprovalMode:    "never",
			ClaudePermissionMode: "bypassPermissions",
			RunnerAutoApprove:    true,
			GrokPermissionMode:   "bypassPermissions",
		}
	}
	return YoloPosture{
		CodexSandbox:         "workspace-write",
		CodexApprovalMode:    "untrusted",
		ClaudePermissionMode: "default",
		RunnerAutoApprove:    false,
		GrokPermissionMode:   "",
	}
}
