package runner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Phase 3 / 07 (validated against real `claude` 2.1.177, Appendix A spike): CLI arg
// assembly + the FlowPilot permission/ask_user MCP-tool handlers, plus config isolation.
//
// SPIKE FINDINGS (canonical contract):
//   - Permission interception on the Claude CLI is the MCP `--permission-prompt-tool`
//     (NOT an in-stream control_request). Claude calls our `approve` tool with
//     arguments {tool_name, input, tool_use_id}; we reply with a single text content
//     block whose text is JSON: {"behavior":"deny","message":...} or
//     {"behavior":"allow","updatedInput":{...}}. Deny BLOCKS the tool (verified: the
//     file was not created; result.permission_denials records it).
//   - Gating only engages if the launch config has NO broad allow-rules. Under the
//     user's ambient ~/.claude (allow: [Edit, Write, Bash(*)]) every tool auto-ran and
//     the approve tool was never called. FlowPilot therefore launches claude under a
//     controlled CLAUDE_CONFIG_DIR whose settings.json has defaultMode=default and no
//     allow-list (ensureClaudeConfigSettings), plus --strict-mcp-config.
//
// The handlers below are the provider-neutral core the runner-hosted MCP server calls;
// the HTTP transport that serves them to claude (per-turn URL + bridge routing) is the
// remaining wiring (07 DoD).

const (
	// claudeMCPServerName is the single source of truth for FlowPilot's MCP server name in
	// the per-turn --mcp-config. The tool names below encode it (claude's mcp__<server>__<tool>
	// convention), and writeClaudeMCPConfig guards against an injected extra server shadowing
	// it — so the name MUST be referenced via this const, never re-typed as a literal.
	claudeMCPServerName   = "flowpilot"
	claudeApproveToolName = "mcp__" + claudeMCPServerName + "__approve"
	claudeAskUserToolName = "mcp__" + claudeMCPServerName + "__ask_user"
)

// claudeArgs builds the headless stream-json invocation. YOLO drives --permission-mode
// (the SSOT, yolo_resolver): yolo=true -> bypassPermissions (no gating); yolo=false ->
// default + --permission-prompt-tool so gated tools route to FlowPilot's approve tool.
// --strict-mcp-config loads ONLY FlowPilot's MCP config (ignores the user's ambient
// servers). resumeID MUST be a real Claude session_id (never the synthetic id; finding 1).
// Skills ride the prompt (promptPrep), not args.
func claudeArgs(posture YoloPosture, resumeID, mcpConfig, modelName, reasoningEffort string, _ []SkillSelection) []string {
	args := []string{
		"-p",
		"--input-format", "stream-json",
		"--output-format", "stream-json",
		"--verbose",
		"--include-partial-messages",
		"--include-hook-events",
		"--strict-mcp-config",
	}
	if posture.ClaudePermissionMode != "" {
		args = append(args, "--permission-mode", posture.ClaudePermissionMode)
	}
	if model := strings.TrimSpace(modelName); model != "" {
		args = append(args, "--model", normalizeClaudeModelName(model))
	}
	if effort := strings.TrimSpace(reasoningEffort); effort != "" {
		args = append(args, "--effort", normalizeClaudeEffort(effort))
	}
	if strings.TrimSpace(mcpConfig) != "" {
		args = append(args, "--mcp-config", mcpConfig)
		if !posture.RunnerAutoApprove {
			args = append(args, "--permission-prompt-tool", claudeApproveToolName)
		}
	}
	if strings.TrimSpace(resumeID) != "" {
		args = append(args, "--resume", resumeID)
	}
	return args
}

// ensureClaudeConfigSettings enforces the gating posture in the FlowPilot-managed claude
// config dir's settings.json: defaultMode=default + empty allow-list (no auto-run).
//
// It ALWAYS overwrites the permissions section, even when settings.json already exists.
// Write-once was the original design ("respect an existing settings.json"), but Claude CLI
// persists allow-rules to settings.json each time the user approves a tool via the
// --permission-prompt-tool MCP route. On the next YOLO=false turn the stale allow-rules
// bypass our gating entirely — Claude auto-runs the tool without calling the approval MCP
// and no approval card appears (BUG-069). Non-permission fields (e.g. theme) are preserved
// by merging the existing file before writing.
func ensureClaudeConfigSettings(configDir string) error {
	if strings.TrimSpace(configDir) == "" {
		return nil
	}
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(configDir, "settings.json")

	// Merge with existing settings so non-permission fields survive.
	existing := map[string]any{}
	if raw, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(raw, &existing) // best-effort; ignore parse errors
	}

	// Always force the permissions section to the gating posture. An inherited
	// allow-rule (e.g. Bash(*)) silently bypasses YOLO=false gating (spike finding).
	existing["permissions"] = map[string]any{
		"defaultMode": "default",
		"allow":       []any{},
	}

	b, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// flowpilotManagedClaudeConfigDir is a FlowPilot-owned claude config dir for the
// ANTHROPIC_API_KEY path (no account home). The env key supplies auth, so a fresh dir
// with the gating posture (ensureClaudeConfigSettings) is safe and keeps ambient
// ~/.claude allow-rules from bypassing gating (review finding 2).
func flowpilotManagedClaudeConfigDir(workspace string) string {
	if strings.TrimSpace(workspace) == "" {
		return ""
	}
	return filepath.Join(workspace, ".flowpilot", "claude-config")
}

// flowpilotClaudeMcpConfig is the --mcp-config payload registering FlowPilot's approve +
// ask_user tools, served by a Go-hosted MCP server (command/args for stdio, or a url for
// http). The live server transport is the remaining 07 wiring.
func flowpilotClaudeMcpConfig(command string, cmdArgs []string) map[string]any {
	return map[string]any{
		"mcpServers": map[string]any{
			claudeMCPServerName: map[string]any{"command": command, "args": cmdArgs},
		},
	}
}

// ---- MCP tool handlers (the provider-neutral core; transport calls these) ----

// handleClaudeApprove maps a `--permission-prompt-tool` call to the FlowPilot bridge and
// returns the MCP tool result claude expects. args is the validated approve shape
// {tool_name, input, tool_use_id}. On bridge error (expiry/interrupt) it denies so the
// engine never hangs.
func handleClaudeApprove(args map[string]any, bridge TurnBridge) map[string]any {
	details := claudeApprovalDetails(args)
	decision, err := bridge.RequestApproval(details)
	if err != nil || decision == "deny" {
		return claudeMcpToolResult(map[string]any{
			"behavior": "deny",
			"message":  "denied by FlowPilot policy",
		})
	}
	input, _ := args["input"].(map[string]any)
	if input == nil {
		input = map[string]any{}
	}
	return claudeMcpToolResult(map[string]any{
		"behavior":     "allow",
		"updatedInput": input,
	})
}

// handleClaudeAskUser maps an ask_user tool call to the bridge's structured question and
// returns the answer as the tool result.
func handleClaudeAskUser(args map[string]any, bridge TurnBridge) map[string]any {
	prompt, options, multi := claudeAskUserParams(args)
	choice, err := bridge.AskQuestion(prompt, options, multi)
	if err != nil || len(choice) == 0 {
		return claudeMcpTextResult("No answer was provided.")
	}
	return claudeMcpTextResult(strings.Join(choice, ", "))
}

// claudeMcpToolResult wraps a decision object as the MCP tool result claude reads for
// the permission-prompt-tool: a single text content block whose text is the JSON object.
func claudeMcpToolResult(decision map[string]any) map[string]any {
	text, _ := json.Marshal(decision)
	return map[string]any{"content": []any{map[string]any{"type": "text", "text": string(text)}}}
}

func claudeMcpTextResult(text string) map[string]any {
	return map[string]any{"content": []any{map[string]any{"type": "text", "text": text}}}
}

// ---- payload parsers --------------------------------------------------------

// claudeControlRequest extracts the request id + payload from a control_request frame
// (the SDK stdio path; kept as a fallback). Tolerates id under request_id/id and the
// payload nested under "request" or inlined.
func claudeControlRequest(raw map[string]any) (any, map[string]any) {
	reqID := raw["request_id"]
	if reqID == nil {
		reqID = raw["id"]
	}
	payload, _ := raw["request"].(map[string]any)
	if payload == nil {
		payload = raw
	}
	return reqID, payload
}

// claudeApprovalDetails maps an approve/can_use_tool payload into ApprovalDetails. The
// validated approve shape is {tool_name, input{...}}; Command is the most useful label:
// Bash -> input.command, file-editing tools -> input.file_path, else the tool name.
func claudeApprovalDetails(payload map[string]any) ApprovalDetails {
	str := func(m map[string]any, k string) string {
		if m == nil {
			return ""
		}
		s, _ := m[k].(string)
		return s
	}
	toolName := str(payload, "tool_name")
	if toolName == "" {
		toolName = str(payload, "toolName")
	}
	input, _ := payload["input"].(map[string]any)

	command := str(payload, "command")
	if command == "" && input != nil {
		if c := str(input, "command"); c != "" {
			command = c
		} else if fp := str(input, "file_path"); fp != "" {
			command = fp
		}
	}
	if command == "" {
		command = toolName
	}
	cwd := str(payload, "cwd")
	if cwd == "" {
		cwd = str(input, "cwd")
	}
	return ApprovalDetails{
		Command: command,
		Cwd:     cwd,
		Reason:  toolName,
		Decisions: []ApprovalDecisionOption{
			{Value: "approve", Label: "Approve"},
			{Value: "deny", Label: "Deny"},
		},
	}
}

// claudeAskUserParams maps an ask_user payload into AskQuestion arguments. Options may be
// plain strings or {label, description} objects.
func claudeAskUserParams(payload map[string]any) (string, []QuestionOption, bool) {
	prompt, _ := payload["prompt"].(string)
	multi, _ := payload["multiSelect"].(bool)
	var opts []QuestionOption
	if rawOpts, ok := payload["options"].([]any); ok {
		for _, o := range rawOpts {
			switch v := o.(type) {
			case string:
				opts = append(opts, QuestionOption{Label: v, Value: v})
			case map[string]any:
				label, _ := v["label"].(string)
				desc, _ := v["description"].(string)
				opts = append(opts, QuestionOption{Label: label, Description: desc, Value: label})
			}
		}
	}
	return prompt, opts, multi
}
