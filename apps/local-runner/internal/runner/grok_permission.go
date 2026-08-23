package runner

import "strings"

// Task-208 (CP-46 P-5/P-6/P-9): the Grok permission decision-vocabulary mapper.
// Reuses the Codex INBOUND-REQUEST pattern (handleInbound, codex_adapter.go) —
// NOT Claude's HTTP-MCP --permission-prompt-tool server — because Grok's
// channel is already a server->client JSON-RPC request carrying options[],
// which is a closer match to Codex's approval flow (CP-46 T-1).

// grokApprovalDetailsFromRequest builds ApprovalDetails from a live
// `session/request_permission` request's params. Live-verified top-level
// shape: {sessionId, toolCall:{...}, options:[{optionId,name,kind}]}. The
// toolCall sub-shape mirrors the tool_call/tool_call_update _meta.x.ai/tool
// shape (name/kind/namespace) that Task-207's event mapper already reads.
func grokApprovalDetailsFromRequest(params map[string]any) ApprovalDetails {
	toolCall, _ := params["toolCall"].(map[string]any)
	title, _ := toolCall["title"].(string)
	command := title
	if rawInput, ok := toolCall["rawInput"].(map[string]any); ok {
		if cmd, _ := rawInput["command"].(string); cmd != "" {
			command = cmd
		}
	}
	// Reason carries the tool name/kind so the read-only posture classifier
	// (chat_posture_policy.go) can tell a Grok read tool from a write/edit.
	reason := title
	if meta, _ := toolCall["_meta"].(map[string]any); meta != nil {
		if toolMeta, ok := meta["x.ai/tool"].(map[string]any); ok {
			if name, _ := toolMeta["name"].(string); name != "" {
				reason = name
			}
		}
	}
	return ApprovalDetails{
		Command: command,
		Reason:  reason,
		Kind:    grokPermissionKind(toolCall),
		Decisions: []ApprovalDecisionOption{
			{Value: "approve", Label: "Approve"},
			{Value: "deny", Label: "Deny"},
		},
	}
}

// grokPermissionKind classifies a Grok toolCall into the shared
// exec/file/mcp/other vocabulary so the per-project "don't ask again"
// allowlist (BUG-246) only engages for shell commands (GR-04). This mapping is
// best-effort: Task-206/207's live probe only exercised a read-only
// (kind="read") tool call, never an exec-shaped one, so the "exec" branch
// below is inferred from Grok's documented tool namespaces/kinds rather than
// independently live-verified — Task-208's live acceptance pass (E2E-07,
// GR-04) must confirm it against a real shell/run_terminal_command tool_call
// before this classification is trusted in production.
func grokPermissionKind(toolCall map[string]any) string {
	if toolCall == nil {
		return "other"
	}
	meta, _ := toolCall["_meta"].(map[string]any)
	var kind, namespace, name string
	if meta != nil {
		if toolMeta, ok := meta["x.ai/tool"].(map[string]any); ok {
			kind, _ = toolMeta["kind"].(string)
			namespace, _ = toolMeta["namespace"].(string)
			name, _ = toolMeta["name"].(string)
		}
	}
	if kind == "" {
		kind, _ = toolCall["kind"].(string)
	}
	lowerKind := strings.ToLower(kind)
	lowerName := strings.ToLower(name)
	switch {
	case lowerKind == "read" || lowerKind == "edit" || lowerKind == "write" || lowerKind == "create" || lowerKind == "delete":
		return "file"
	case lowerKind == "execute" || lowerKind == "run" || lowerKind == "shell" || lowerKind == "terminal" ||
		strings.Contains(lowerName, "terminal") || strings.Contains(lowerName, "shell") || strings.Contains(lowerName, "exec") || strings.Contains(lowerName, "run_command"):
		return "exec"
	case strings.TrimSpace(namespace) != "" && namespace != "grok_build":
		return "mcp"
	default:
		return "other"
	}
}

// grokEncodePermissionDecision picks the request-specific optionId matching
// FlowPilot's internal approve/deny decision from the options this specific
// request offered (never a hardcoded string, Task-208 T-2). Live-verified
// option kinds: allow_once/allow_always/reject_once. Returns "" if no option
// in the offered set matches the requested direction — Grok's own live
// behavior for an absent/mismatched optionId is a controlled
// PermissionRejected, i.e. already deny-shaped, so failing to find a match is
// safe by construction (never silently grants access).
func grokEncodePermissionDecision(options []any, approve bool) string {
	var fallback string
	for _, raw := range options {
		opt, _ := raw.(map[string]any)
		if opt == nil {
			continue
		}
		optionID, _ := opt["optionId"].(string)
		kind := strings.ToLower(strings.TrimSpace(stringAny(opt, "kind")))
		if optionID == "" {
			continue
		}
		isAllow := strings.Contains(kind, "allow")
		isReject := strings.Contains(kind, "reject") || strings.Contains(kind, "deny")
		if approve && isAllow {
			// Prefer the narrowest allow (allow_once) if more than one allow
			// option is offered; the first allow-shaped option seen otherwise.
			if kind == "allow_once" || fallback == "" {
				fallback = optionID
			}
			if kind == "allow_once" {
				return optionID
			}
		}
		if !approve && isReject {
			return optionID
		}
	}
	if approve {
		return fallback
	}
	return ""
}

// grokPermissionOutcomeResponse builds the `session/request_permission` reply
// body. An empty optionID (no matching option found) still produces a
// well-formed reply so Grok never hangs waiting for a response — CP-46's live
// evidence shows Grok treats an unrecognized/empty optionId as a denial
// (PermissionRejected), which is the safe outcome here.
func grokPermissionOutcomeResponse(optionID string) map[string]any {
	return map[string]any{"outcome": map[string]any{"outcome": "selected", "optionId": optionID}}
}
