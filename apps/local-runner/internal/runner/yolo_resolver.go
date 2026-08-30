package runner

import "strings"

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
//
// Product lock (BUG-299 residual / run-35329): Flow/Workflow (and any flow-engine
// driven run) always runs YOLO=true. Normal Chat keeps the UI toggle. Session
// rehydrate must not re-open YOLO=off for those runs just because the durable
// row lacked a yolo field or still stored false from a pre-CA-378 default.

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
	// OpencodePermissionMode is read only by the Opencode adapter (CP-57):
	// yolo=true -> "bypassPermissions" via --auto flag, yolo=false -> "" (default).
	// Additive field — codex/claude/grok callers never read it.
	OpencodePermissionMode string
}

// resolveYoloPosture is the SSOT mapping. Keep this the only place the YOLO boolean
// becomes provider/runner config.
func resolveYoloPosture(yolo bool) YoloPosture {
	if yolo {
		return YoloPosture{
			CodexSandbox:           "danger-full-access",
			CodexApprovalMode:      "never",
			ClaudePermissionMode:   "bypassPermissions",
			RunnerAutoApprove:      true,
			GrokPermissionMode:     "bypassPermissions",
			OpencodePermissionMode: "bypassPermissions",
		}
	}
	return YoloPosture{
		CodexSandbox:           "workspace-write",
		CodexApprovalMode:      "untrusted",
		ClaudePermissionMode:   "default",
		RunnerAutoApprove:      false,
		GrokPermissionMode:     "",
		OpencodePermissionMode: "",
	}
}

// resolveYoloPostureForTurn applies V9-21: flow-engine coding children must still
// route shell approvals through the runner bridge under YOLO so the git-commit
// denylist can fire. Adapters that use approval-never/bypass-permissions never
// call RequestApproval, which made commit deny a no-op on all four providers.
//
// When forceBridge is true (flow coding child + YOLO): keep auto-approve on the
// bridge for ordinary commands, but force provider permission modes that still
// surface exec approvals so isFlowCodingCommitAttempt can deny commits first.
func resolveYoloPostureForTurn(yolo, forceShellBridge bool) YoloPosture {
	p := resolveYoloPosture(yolo)
	if yolo && forceShellBridge {
		p.CodexApprovalMode = "untrusted"
		p.ClaudePermissionMode = "default"
		p.GrokPermissionMode = ""
		p.OpencodePermissionMode = ""
		// RunnerAutoApprove remains true.
	}
	return p
}

// resolveYoloPostureForChatPosture extends resolveYoloPostureForTurn with the
// read-only chat postures (scan/plan). A read-only posture must route EVERY tool
// through the runner bridge so the read-only policy can approve reads / deny
// writes — provider bypass modes (approval-never, bypassPermissions) would
// silently let writes through. So scan/plan force the same gated provider modes
// as forceShellBridge, and additionally clear RunnerAutoApprove (the bridge's
// read-only policy owns the decision; the profile YOLO flag is deliberately
// ignored for scan/plan).
func resolveYoloPostureForChatPosture(yolo, forceShellBridge bool, posture string) YoloPosture {
	p := resolveYoloPostureForTurn(yolo, forceShellBridge)
	if IsReadOnlyChatPosture(posture) {
		p.CodexSandbox = "workspace-write"
		p.CodexApprovalMode = "untrusted"
		p.ClaudePermissionMode = "default"
		p.GrokPermissionMode = ""
		p.OpencodePermissionMode = ""
		p.RunnerAutoApprove = false
	}
	return p
}

// shouldForceFlowYolo reports whether product policy requires YOLO=true for this
// run (BUG-299 residual). Normal Chat (runKind=="chat", no workflow, not
// flow-engine-driven) keeps the user's toggle and is never forced here.
//
// Force when any of:
//   - flowEngineDriven (chat bug-submode Review Loop, workflow-picker flow, etc.)
//   - non-empty workflowID (workflow launch + flow children that inherit it)
//   - runKind is workflow / empty (createRun stamps "workflow"; legacy hubs used "")
func shouldForceFlowYolo(runKind, workflowID string, flowEngineDriven bool) bool {
	if flowEngineDriven {
		return true
	}
	if strings.TrimSpace(workflowID) != "" {
		return true
	}
	rk := strings.TrimSpace(runKind)
	return rk == "" || rk == "workflow"
}

// resolveEffectiveYolo applies the Flow force on top of a base (request / session /
// catalog) value. Chat mode without force keeps base unchanged.
func resolveEffectiveYolo(base bool, runKind, workflowID string, flowEngineDriven bool) bool {
	if shouldForceFlowYolo(runKind, workflowID, flowEngineDriven) {
		return true
	}
	return base
}

// resolveTurnYolo is the per-turn YOLO boolean: explicit TurnInput.YoloMode
// overrides the run default, then Flow/Workflow force (BUG-299) applies.
func resolveTurnYolo(rs *interactiveRun, in TurnInput) bool {
	if rs == nil {
		return false
	}
	yolo := rs.yolo
	if in.YoloMode != nil {
		yolo = *in.YoloMode
	}
	return resolveEffectiveYolo(yolo, rs.runKind, rs.workflowID, rs.flowEngineDriven)
}
