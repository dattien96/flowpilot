package runner

import "strings"

func mapGeminiACPUpdate(message map[string]interface{}) []ProviderEvent {
	update, ok := geminiACPUpdate(message)
	if !ok {
		return nil
	}
	updateType, _ := update["sessionUpdate"].(string)
	switch updateType {
	case "tool_call":
		name := geminiACPToolName(update)
		return []ProviderEvent{{
			Type:     EventToolStarted,
			ToolName: name,
			Input:    update["rawInput"],
			Status:   geminiACPToolStatus(update),
		}}
	case "tool_call_update":
		name := geminiACPToolName(update)
		return []ProviderEvent{{
			Type:     EventToolCompleted,
			ToolName: name,
			Output:   geminiACPToolOutput(update),
			Status:   geminiACPToolStatus(update),
		}}
	default:
		return nil
	}
}

func geminiACPUpdate(message map[string]interface{}) (map[string]interface{}, bool) {
	method, _ := message["method"].(string)
	if method != "session/update" {
		return nil, false
	}
	params, ok := message["params"].(map[string]interface{})
	if !ok {
		return nil, false
	}
	update, ok := params["update"].(map[string]interface{})
	return update, ok
}

func geminiACPToolName(update map[string]interface{}) string {
	for _, key := range []string{"title", "toolCallId", "kind"} {
		if value, ok := update[key].(string); ok && strings.TrimSpace(value) != "" {
			return value
		}
	}
	return "gemini_tool"
}

func geminiACPToolStatus(update map[string]interface{}) string {
	if value, ok := update["status"].(string); ok {
		return value
	}
	return ""
}

func geminiACPToolOutput(update map[string]interface{}) any {
	if output, ok := update["rawOutput"]; ok {
		return output
	}
	if content, ok := update["content"]; ok {
		return content
	}
	return nil
}

func geminiACPPermissionRequest(message map[string]interface{}) (interface{}, map[string]interface{}, bool) {
	method, _ := message["method"].(string)
	if method != "session/request_permission" {
		return nil, nil, false
	}
	id, ok := message["id"]
	if !ok {
		return nil, nil, false
	}
	params, ok := message["params"].(map[string]interface{})
	return id, params, ok
}

func geminiACPApprovalDetails(params map[string]interface{}) ApprovalDetails {
	toolCall, _ := params["toolCall"].(map[string]interface{})
	details := ApprovalDetails{
		Command: geminiACPToolName(toolCall),
		Reason:  geminiACPApprovalReason(toolCall),
	}
	if locations, ok := toolCall["locations"].([]interface{}); ok && len(locations) > 0 {
		if first, ok := locations[0].(map[string]interface{}); ok {
			if path, ok := first["path"].(string); ok {
				details.Cwd = path
			}
		}
	}
	if options, ok := params["options"].([]interface{}); ok {
		for _, raw := range options {
			option, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			value, _ := option["optionId"].(string)
			label, _ := option["name"].(string)
			if strings.TrimSpace(value) == "" {
				continue
			}
			if strings.TrimSpace(label) == "" {
				label = value
			}
			details.Decisions = append(details.Decisions, ApprovalDecisionOption{Value: value, Label: label})
		}
	}
	return details
}

func geminiACPApprovalReason(toolCall map[string]interface{}) string {
	kind, _ := toolCall["kind"].(string)
	status, _ := toolCall["status"].(string)
	switch {
	case kind != "" && status != "":
		return "Gemini ACP " + kind + " tool is " + status
	case kind != "":
		return "Gemini ACP " + kind + " tool"
	default:
		return "Gemini ACP tool request"
	}
}

func geminiACPPermissionResponse(params map[string]interface{}, decision string, bridgeErr error) map[string]interface{} {
	if bridgeErr != nil {
		return map[string]interface{}{"outcome": map[string]interface{}{"outcome": "cancelled"}}
	}
	optionID := geminiACPSelectPermissionOption(params, decision)
	if optionID == "" {
		return map[string]interface{}{"outcome": map[string]interface{}{"outcome": "cancelled"}}
	}
	return map[string]interface{}{
		"outcome": map[string]interface{}{
			"outcome":  "selected",
			"optionId": optionID,
		},
	}
}

func geminiACPSelectPermissionOption(params map[string]interface{}, decision string) string {
	options, _ := params["options"].([]interface{})
	trimmed := strings.TrimSpace(decision)
	for _, raw := range options {
		option, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		optionID, _ := option["optionId"].(string)
		if optionID == trimmed && optionID != "" {
			return optionID
		}
	}

	wantReject := strings.EqualFold(trimmed, "deny") || strings.EqualFold(trimmed, "reject")
	for _, raw := range options {
		option, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		optionID, _ := option["optionId"].(string)
		kind, _ := option["kind"].(string)
		if optionID == "" {
			continue
		}
		if wantReject && strings.HasPrefix(kind, "reject") {
			return optionID
		}
		if !wantReject && strings.HasPrefix(kind, "allow") {
			return optionID
		}
	}
	return ""
}
