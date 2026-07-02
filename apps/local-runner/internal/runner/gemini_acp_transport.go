package runner

import "strings"

func geminiACPInitializeParams() map[string]interface{} {
	return map[string]interface{}{
		"protocolVersion": 1,
		"capabilities":    map[string]interface{}{},
		"clientInfo": map[string]interface{}{
			"name":    "flowpilot",
			"version": "1.0",
		},
	}
}

func geminiACPSessionNewParams(cwd string) map[string]interface{} {
	return geminiACPSessionNewParamsWithMCP(cwd, nil)
}

func geminiACPSessionNewParamsWithMCP(cwd string, mcpServers []interface{}) map[string]interface{} {
	if mcpServers == nil {
		mcpServers = []interface{}{}
	}
	return map[string]interface{}{
		"cwd":        cwd,
		"mcpServers": mcpServers,
	}
}

func geminiACPSessionLoadParams(sessionID, cwd string, mcpServers []interface{}) map[string]interface{} {
	if mcpServers == nil {
		mcpServers = []interface{}{}
	}
	return map[string]interface{}{
		"sessionId":  strings.TrimSpace(sessionID),
		"cwd":        cwd,
		"mcpServers": mcpServers,
	}
}

func geminiACPPromptParams(sessionID string, prompt string) map[string]interface{} {
	return map[string]interface{}{
		"sessionId": sessionID,
		"prompt": []map[string]string{
			{
				"type": "text",
				"text": prompt,
			},
		},
	}
}

func geminiACPResponseSessionID(message map[string]interface{}) string {
	result, ok := message["result"].(map[string]interface{})
	if !ok {
		return ""
	}
	if sessionID, ok := result["sessionId"].(string); ok {
		return strings.TrimSpace(sessionID)
	}
	return ""
}

func extractGeminiACPText(message map[string]interface{}) string {
	method, _ := message["method"].(string)
	if method != "session/update" {
		return ""
	}

	params, ok := message["params"].(map[string]interface{})
	if !ok {
		return ""
	}

	update, ok := params["update"].(map[string]interface{})
	if !ok {
		return ""
	}

	if updateType, _ := update["sessionUpdate"].(string); updateType != "agent_message_chunk" {
		return ""
	}

	content, ok := update["content"].(map[string]interface{})
	if !ok {
		return ""
	}

	if contentType, _ := content["type"].(string); contentType != "text" {
		return ""
	}

	if text, ok := content["text"].(string); ok {
		return text
	}

	return ""
}

func geminiACPResultText(result map[string]interface{}) string {
	if text, ok := result["text"].(string); ok && text != "" {
		return text
	}

	if content, ok := result["content"].([]interface{}); ok {
		var output strings.Builder
		for _, entry := range content {
			block, ok := entry.(map[string]interface{})
			if !ok {
				continue
			}
			if blockType, _ := block["type"].(string); blockType != "text" {
				continue
			}
			if text, ok := block["text"].(string); ok {
				output.WriteString(text)
			}
		}
		return output.String()
	}

	return ""
}

func geminiACPFlowPilotMCPServers(baseURL, token string) []interface{} {
	base := strings.TrimSpace(baseURL)
	tok := strings.TrimSpace(token)
	if base == "" || tok == "" {
		return nil
	}
	return []interface{}{
		map[string]interface{}{
			"type":    "http",
			"name":    claudeMCPServerName,
			"url":     strings.TrimRight(base, "/") + ClaudeMCPPath + "?token=" + tok,
			"headers": []interface{}{},
		},
	}
}
