package runner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// InjectRequiredMcpInstructions adds MCP usage instructions to a prompt
func InjectRequiredMcpInstructions(prompt string, requiredMcps []string, providerKey string, allowWrite bool) string {
	if len(requiredMcps) == 0 {
		return prompt
	}

	// Check if google_drive is required
	hasGoogleDrive := false
	for _, mcp := range requiredMcps {
		if strings.ToLower(strings.TrimSpace(mcp)) == "google_drive" {
			hasGoogleDrive = true
			break
		}
	}

	if !hasGoogleDrive {
		return prompt
	}

	// Build MCP instructions section
	instructions := buildGoogleDriveMcpInstructions(providerKey, allowWrite)

	// Insert instructions after any existing headers or at the start
	if strings.Contains(prompt, "\n\n") {
		// Insert after first paragraph
		parts := strings.SplitN(prompt, "\n\n", 2)
		return parts[0] + "\n\n" + instructions + "\n\n" + parts[1]
	}

	// Prepend instructions
	return instructions + "\n\n" + prompt
}

// buildGoogleDriveMcpInstructions creates the MCP usage section
func buildGoogleDriveMcpInstructions(providerKey string, allowWrite bool) string {
	var sb strings.Builder

	sb.WriteString("## Required MCP Usage\n\n")
	sb.WriteString("This workflow step requires FlowPilot MCP `google_drive`.\n")
	sb.WriteString("The configured provider MCP server name is `google-drive`.\n\n")

	if allowWrite {
		sb.WriteString("This step is allowed to perform read and write operations on Google Drive.\n\n")
	} else {
		sb.WriteString("Before producing the final answer, use Google Drive MCP tools from `google-drive` when Drive context is needed for this task.\n\n")
	}

	sb.WriteString("Preferred read-only tools:\n")
	sb.WriteString("- `authGetStatus` or equivalent auth/status diagnostic, when checking availability\n")
	sb.WriteString("- `search` for locating files\n")
	sb.WriteString("- `listFolder` for folder contents\n")
	sb.WriteString("- `readGoogleDoc` or paginated document read tools for Google Docs content\n\n")

	sb.WriteString("Rules:\n")
	sb.WriteString("- Do not invent Google Drive content.\n")
	sb.WriteString("- If `google-drive` is unavailable, stop and report `MCP_UNAVAILABLE`.\n")
	sb.WriteString("- If auth is missing or expired, stop and report `MCP_AUTH_REQUIRED`.\n")
	sb.WriteString("- If the required Drive file or folder cannot be found, report `DRIVE_CONTENT_NOT_FOUND`.\n")
	sb.WriteString("- Include the file name and file ID for every Drive item used.\n")

	if !allowWrite {
		sb.WriteString("- Use read-only tools only unless this step explicitly allows writes.\n")
	}

	return sb.String()
}

// MCPPreflightCheck validates Google Drive MCP readiness
type MCPPreflightCheck struct {
	GoogleDriveReady   bool
	ProviderConfigured bool
	ErrorMessage       string
}

// PreflightGoogleDriveMcp checks if Google Drive MCP and provider are ready
func (r *Runner) PreflightGoogleDriveMcp(providerKey string, accountHomePath string) MCPPreflightCheck {
	result := MCPPreflightCheck{}

	// Check Google Drive MCP status
	mcpStatus, err := r.googleDriveMcpRuntimeConfig()
	if err != nil {
		result.ErrorMessage = fmt.Sprintf("Failed to check Google Drive MCP status: %v", err)
		return result
	}

	// Validate Google Drive MCP status
	switch mcpStatus.Status {
	case "needs_input":
		result.ErrorMessage = "Google Drive MCP credential JSON is missing. Upload the Desktop OAuth JSON first."
		return result
	case "failed":
		result.ErrorMessage = "Google Drive MCP credential JSON is invalid."
		return result
	case "needs_auth":
		result.ErrorMessage = "Google Drive MCP auth is incomplete. Run Start Auth, complete sign-in, then refresh status."
		return result
	case "reconnect_required":
		result.ErrorMessage = "Google Drive MCP token requires reconnect. Start Auth again."
		return result
	case "configured", "warning":
		result.GoogleDriveReady = true
	default:
		result.ErrorMessage = fmt.Sprintf("Google Drive MCP status is %s", mcpStatus.Status)
		return result
	}

	// Check provider config if provider key is provided
	if strings.TrimSpace(providerKey) != "" && strings.TrimSpace(accountHomePath) != "" {
		providerKey = strings.ToLower(strings.TrimSpace(providerKey))

		var configPath string
		var configExists bool

		switch providerKey {
		case "codex":
			configPath = filepath.Join(accountHomePath, "config.toml")
		case "gemini":
			configPath = filepath.Join(accountHomePath, ".gemini", "settings.json")
		case "claude":
			configPath = filepath.Join(accountHomePath, ".claude.json")
		default:
			result.ErrorMessage = fmt.Sprintf("Unsupported provider: %s", providerKey)
			return result
		}

		// Check if config file exists
		if _, err := os.Stat(configPath); err == nil {
			configExists = true
		}

		if !configExists {
			result.ErrorMessage = fmt.Sprintf("The selected AI provider is not configured with the google-drive MCP server. Config path: %s", configPath)
			return result
		}

		// Validate the actual config structure and mcpServers.google-drive presence
		providerConfigStatus, err := r.checkProviderGoogleDriveMcpConfig(providerKey, accountHomePath, configPath, mcpStatus)
		if err != nil {
			result.ErrorMessage = fmt.Sprintf("Provider config validation failed: %v", err)
			return result
		}

		if providerConfigStatus.Status == "failed" {
			result.ErrorMessage = fmt.Sprintf("Provider config is invalid: %s", providerConfigStatus.LastError)
			return result
		}

		// Check for stale config
		if providerConfigStatus.Status == "config_stale" {
			result.ErrorMessage = "Provider has stale Google Drive MCP config. Re-run Configure Providers."
			return result
		}

		result.ProviderConfigured = true
	} else {
		// If no provider specified, assume provider config is not checked
		result.ProviderConfigured = true
	}

	return result
}
