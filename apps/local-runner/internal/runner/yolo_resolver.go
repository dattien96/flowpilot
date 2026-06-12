package runner

// Phase 4 (04-04): YOLO as the single source of truth.
//
// One per-run/step YOLO boolean configures BOTH layers at once — the Codex thread
// (sandbox + approval mode) and the FlowPilot runner approval bridge — so the two
// can never drift. This replaces the old habit of pinning a standalone
// `default_tools_approval_mode = "approve"` somewhere and hoping it lines up with
// runner behavior.
//
//	resolveYoloPosture(true)  -> {"full-access",     "never",      true}
//	resolveYoloPosture(false) -> {"workspace-write", "on-request", false}
//
// Input flows in via PromptExecutionRequest.YoloMode (types.go) / session-start
// ApprovalMode+AllowWrite; it is resolved per run/step and carried on the turn.

// YoloPosture is the resolved configuration for one turn. CodexSandbox and
// CodexApprovalMode are applied to the Codex thread/turn params; RunnerAutoApprove
// drives the approval bridge (YOLO=true → auto-approve + audit as gating-disabled).
type YoloPosture struct {
	CodexSandbox      string
	CodexApprovalMode string
	RunnerAutoApprove bool
}

// resolveYoloPosture is the SSOT mapping. Keep this the only place the YOLO boolean
// becomes provider/runner config.
func resolveYoloPosture(yolo bool) YoloPosture {
	if yolo {
		return YoloPosture{
			CodexSandbox:      "full-access",
			CodexApprovalMode: "never",
			RunnerAutoApprove: true,
		}
	}
	return YoloPosture{
		CodexSandbox:      "workspace-write",
		CodexApprovalMode: "on-request",
		RunnerAutoApprove: false,
	}
}
