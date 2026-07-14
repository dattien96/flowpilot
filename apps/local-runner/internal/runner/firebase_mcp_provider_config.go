package runner

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Task-230 (CP-05-04 P-1/P-2): Firebase connects through the OFFICIAL
// firebase-tools MCP (`npx -y firebase-tools mcp --only crashlytics`), a
// local stdio launcher like Google Drive's — unlike Jira's remote HTTP
// endpoint. Auth is a Google Cloud service-account JSON (ADC), not a browser
// OAuth handshake, so — unlike Task-228's Jira gap — the full connection
// flow (credential storage, structural validation, provider-config
// injection) is buildable and testable without any live infrastructure this
// sandbox lacks. What remains unverifiable here is only the *live*
// Crashlytics API call itself (needs a real GCP project + real credentials).
//
// firebaseMcpServerName intentionally does not pin a firebase-tools version
// (`npx -y firebase-tools mcp ...`, floating), matching this codebase's own
// existing convention for Google Drive's `npx -y @piotr-agier/google-drive-mcp`
// (also unpinned) — the CP-05-04 survey recommendation to pin a specific
// version remains a follow-up hardening item, not a deviation to invent here.
const firebaseMcpServerName = "flowpilot_firebase"

// firebaseCredential is the runner-managed secret for a connected Firebase
// integration — mirrors jiraCredential's shape/lifecycle exactly.
// ServiceAccountJSON is the raw contents of the downloaded GCP service
// account key file; it is never written to Supabase, only the runner
// keyring (SD-11 §6).
type firebaseCredential struct {
	ProjectID          string `json:"projectId"`
	Environment        string `json:"environment"`
	ServiceAccountJSON string `json:"serviceAccountJson"`
}

func firebaseCredentialKey(integrationID string) string {
	return normalizeSecretKey("firebase", integrationID)
}

func (r *Runner) saveFirebaseCredential(integrationID string, creds firebaseCredential) error {
	raw, err := json.Marshal(creds)
	if err != nil {
		return err
	}
	return r.ensureSecretStore().Set(firebaseCredentialKey(integrationID), string(raw))
}

func (r *Runner) loadFirebaseCredential(secretKey string) (firebaseCredential, error) {
	if strings.TrimSpace(secretKey) == "" {
		return firebaseCredential{}, errors.New("firebase service account is not configured")
	}
	raw, err := r.ensureSecretStore().Get(secretKey)
	if err != nil {
		return firebaseCredential{}, err
	}
	var creds firebaseCredential
	if err := json.Unmarshal([]byte(raw), &creds); err != nil {
		return firebaseCredential{}, err
	}
	return creds, nil
}

func (r *Runner) deleteFirebaseCredential(integrationID string) error {
	return r.ensureSecretStore().Delete(firebaseCredentialKey(integrationID))
}

// parsedFirebaseServiceAccount is the minimal shape validated out of an
// uploaded service-account JSON — mirrors the structural (not live) JSON
// validation the Google Drive Desktop-OAuth-JSON upload flow already does
// (CP-05-03 §9): confirm the required fields exist before treating the
// credential as usable, without making a live network call.
type parsedFirebaseServiceAccount struct {
	Type        string `json:"type"`
	ProjectID   string `json:"project_id"`
	PrivateKey  string `json:"private_key"`
	ClientEmail string `json:"client_email"`
}

// validateFirebaseServiceAccountJSON structurally validates raw as a GCP
// service-account key file. It never calls Google — only shape/field checks.
func validateFirebaseServiceAccountJSON(raw string) (parsedFirebaseServiceAccount, error) {
	var parsed parsedFirebaseServiceAccount
	if strings.TrimSpace(raw) == "" {
		return parsed, errors.New("a service account JSON is required for Firebase")
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return parsed, fmt.Errorf("service account JSON is not valid JSON: %w", err)
	}
	if parsed.Type != "service_account" {
		return parsed, errors.New("service account JSON must have \"type\": \"service_account\"")
	}
	if strings.TrimSpace(parsed.ProjectID) == "" || strings.TrimSpace(parsed.PrivateKey) == "" || strings.TrimSpace(parsed.ClientEmail) == "" {
		return parsed, errors.New("service account JSON is missing project_id, private_key, or client_email")
	}
	return parsed, nil
}

// resolveConnectedFirebaseCredential mirrors resolveConnectedJiraCredential:
// one active Firebase connection per workspace/runner.
func (r *Runner) resolveConnectedFirebaseCredential() (firebaseCredential, error) {
	records, err := r.loadMcpBackendRecords()
	if err != nil {
		return firebaseCredential{}, fmt.Errorf("load MCP backend records: %w", err)
	}
	secretKey := strings.TrimSpace(records["firebase"].SecretKey)
	if secretKey == "" {
		return firebaseCredential{}, errors.New("firebase is not connected for this workspace")
	}
	return r.loadFirebaseCredential(secretKey)
}

func (r *Runner) detectFirebaseBackend(record mcpBackendRecord) McpBackend {
	backend := McpBackend{
		Key:          "firebase",
		ProviderType: "firebase",
		Label:        "Firebase MCP",
		Transport:    "launcher",
		Launcher:     "npx",
		Command:      "npx -y firebase-tools mcp --only crashlytics",
		InstallHint:  "Upload a Google Cloud service account JSON with Crashlytics access so the runner can connect to the Firebase MCP.",
		State:        "missing",
		Action:       "install",
		ActionLabel:  "Create MCP",
		SecretKey:    record.SecretKey,
	}
	if strings.TrimSpace(record.SecretKey) == "" {
		backend.LastError = "No Firebase service account is stored for the runner yet."
		return backend
	}
	creds, err := r.loadFirebaseCredential(record.SecretKey)
	if err != nil {
		backend.LastError = err.Error()
		return backend
	}
	backend.Installed = true
	backend.State = "installed"
	backend.Action = "verify"
	backend.ActionLabel = "Verify"
	backend.LastCheckedAt = record.LastCheckedAt
	backend.LastError = record.LastError
	if strings.TrimSpace(creds.ProjectID) != "" {
		backend.InstallHint = fmt.Sprintf("Connected to Firebase project %s.", creds.ProjectID)
	}
	return backend
}

// firebaseCredentialFilePath is where EnsureClaudeFirebaseMcpConfig
// materializes the keyring-stored service-account JSON to disk, because
// firebase-tools/ADC (like the Firebase CLI generally) expects
// GOOGLE_APPLICATION_CREDENTIALS to name a file, not hold JSON inline —
// mirrors Google Drive MCP's own credential-file model
// (~/.config/google-drive-mcp/gcp-oauth.keys.json).
func firebaseCredentialFilePath(workspace string) string {
	return filepath.Join(workspace, ".flowpilot", "mcp-credentials", "firebase-service-account.json")
}

// writeFirebaseCredentialFile writes serviceAccountJSON to
// firebaseCredentialFilePath(workspace), creating parent directories as
// needed, with owner-only permissions (credential material).
func writeFirebaseCredentialFile(workspace string, serviceAccountJSON string) (string, error) {
	path := firebaseCredentialFilePath(workspace)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("failed to create Firebase credential directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(serviceAccountJSON), 0o600); err != nil {
		return "", fmt.Errorf("failed to write Firebase credential file: %w", err)
	}
	return path, nil
}

// FirebaseMcpProviderConfigResponse mirrors GoogleDriveMcpProviderConfigResponse's shape.
type FirebaseMcpProviderConfigResponse struct {
	ProviderKey string `json:"providerKey"`
	ServerName  string `json:"serverName"`
	Status      string `json:"status"`
	Changed     bool   `json:"changed"`
	ConfigPath  string `json:"configPath"`
}

func expectedClaudeFirebaseMcpServer(credentialPath string) claudeMcpServer {
	return claudeMcpServer{
		Type:    "stdio",
		Command: "npx",
		Args:    []string{"-y", "firebase-tools", "mcp", "--only", "crashlytics"},
		Env:     map[string]string{"GOOGLE_APPLICATION_CREDENTIALS": credentialPath},
	}
}

// EnsureClaudeFirebaseMcpConfig writes/updates Claude's `mcpServers.firebase`
// stdio entry, reusing the existing typed claudeConfig/claudeMcpServer
// structs from google_drive_mcp_provider_config.go (same package) — same
// round-trip pattern ensureClaudeGoogleDriveMcpConfig already uses for a
// local stdio-launched MCP server, just a different server name/command.
// It first materializes the connected credential to
// firebaseCredentialFilePath(r.workspace) so GOOGLE_APPLICATION_CREDENTIALS
// names a real file.
func (r *Runner) EnsureClaudeFirebaseMcpConfig(accountHomePath string) (FirebaseMcpProviderConfigResponse, error) {
	accountHomePath = strings.TrimSpace(accountHomePath)
	if accountHomePath == "" {
		return FirebaseMcpProviderConfigResponse{}, errors.New("accountHomePath is required")
	}
	creds, err := r.resolveConnectedFirebaseCredential()
	if err != nil {
		return FirebaseMcpProviderConfigResponse{}, err
	}
	credentialPath, err := writeFirebaseCredentialFile(r.workspace, creds.ServiceAccountJSON)
	if err != nil {
		return FirebaseMcpProviderConfigResponse{}, err
	}

	configPath := filepath.Join(accountHomePath, ".claude.json")
	var config claudeConfig
	changed := false

	raw, readErr := os.ReadFile(configPath)
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return FirebaseMcpProviderConfigResponse{}, fmt.Errorf("failed to read Claude config: %w", readErr)
	}
	if readErr == nil {
		if err := json.Unmarshal(raw, &config); err != nil {
			config = claudeConfig{}
			changed = true
		}
	}
	if config.McpServers == nil {
		config.McpServers = make(map[string]claudeMcpServer)
	}

	// Migration: drop the pre-rename key so old+new don't coexist.
	if legacy := legacyMcpServerName(firebaseMcpServerName); legacy != "" {
		if _, ok := config.McpServers[legacy]; ok {
			delete(config.McpServers, legacy)
			changed = true
		}
	}

	expected := expectedClaudeFirebaseMcpServer(credentialPath)
	existing, exists := config.McpServers[firebaseMcpServerName]
	if !exists || !claudeServerConfigMatches(existing, expected) {
		config.McpServers[firebaseMcpServerName] = expected
		changed = true
	}

	if changed {
		if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
			return FirebaseMcpProviderConfigResponse{}, fmt.Errorf("failed to create config directory: %w", err)
		}
		out, err := json.MarshalIndent(config, "", "  ")
		if err != nil {
			return FirebaseMcpProviderConfigResponse{}, fmt.Errorf("failed to marshal JSON: %w", err)
		}
		if err := os.WriteFile(configPath, out, 0o644); err != nil {
			return FirebaseMcpProviderConfigResponse{}, fmt.Errorf("failed to write config: %w", err)
		}
	}

	return FirebaseMcpProviderConfigResponse{
		ProviderKey: "claude",
		ServerName:  firebaseMcpServerName,
		Status:      "configured",
		Changed:     changed,
		ConfigPath:  configPath,
	}, nil
}

// FirebaseMcpProviderConfigRequest is the HTTP-facing request for
// EnsureFirebaseMcpProviderConfig, mirroring JiraMcpProviderConfigRequest.
type FirebaseMcpProviderConfigRequest struct {
	ProviderKey     string `json:"providerKey"`
	AccountHomePath string `json:"accountHomePath"`
}

// EnsureFirebaseMcpProviderConfig is the dispatch entry point mirroring
// EnsureJiraMcpProviderConfig — the production call site that was missing
// entirely before this change (only EnsureClaudeFirebaseMcpConfig existed,
// called from tests only). All four providers are wired: Firebase is a
// local stdio launcher (unlike Jira's remote HTTP), so the same
// npx-firebase-tools shape already proven for Claude round-trips cleanly
// into Codex/Gemini/Grok's own config formats, mirroring Google Drive's
// per-provider writers exactly.
func (r *Runner) EnsureFirebaseMcpProviderConfig(req FirebaseMcpProviderConfigRequest) (FirebaseMcpProviderConfigResponse, error) {
	providerKey := strings.ToLower(strings.TrimSpace(req.ProviderKey))
	if providerKey == "" {
		return FirebaseMcpProviderConfigResponse{}, errors.New("providerKey is required")
	}
	accountHomePath := strings.TrimSpace(req.AccountHomePath)
	if accountHomePath == "" {
		return FirebaseMcpProviderConfigResponse{}, errors.New("accountHomePath is required")
	}
	switch providerKey {
	case "claude":
		return r.EnsureClaudeFirebaseMcpConfig(accountHomePath)
	case "codex":
		return r.ensureCodexFirebaseMcpConfig(accountHomePath)
	case "gemini":
		return r.ensureGeminiFirebaseMcpConfig(accountHomePath)
	case "grok":
		return r.ensureGrokFirebaseMcpConfig(accountHomePath)
	default:
		return FirebaseMcpProviderConfigResponse{}, fmt.Errorf("Firebase MCP provider config is not yet implemented for provider: %s", providerKey)
	}
}

// resolveMaterializedFirebaseCredentialPath resolves the connected Firebase
// credential and materializes it to disk (GOOGLE_APPLICATION_CREDENTIALS
// needs a file path, not inline JSON) — shared by every provider writer plus
// firebaseLiveMCPServer, so each doesn't reimplement resolve+materialize.
func (r *Runner) resolveMaterializedFirebaseCredentialPath() (string, error) {
	creds, err := r.resolveConnectedFirebaseCredential()
	if err != nil {
		return "", err
	}
	return writeFirebaseCredentialFile(r.workspace, creds.ServiceAccountJSON)
}

// firebaseLiveMCPServer builds the per-turn claudeMcpServer entry for
// Firebase when connected, for flowpilotClaudeExtraMCPServers (Task-234
// T-1). Its stdio shape (Command set) reaches Grok automatically too via
// grokACPExtraMCPServers.
func (r *Runner) firebaseLiveMCPServer() (claudeMcpServer, bool) {
	credentialPath, err := r.resolveMaterializedFirebaseCredentialPath()
	if err != nil || credentialPath == "" {
		return claudeMcpServer{}, false
	}
	return expectedClaudeFirebaseMcpServer(credentialPath), true
}

func (r *Runner) ensureCodexFirebaseMcpConfig(accountHomePath string) (FirebaseMcpProviderConfigResponse, error) {
	credentialPath, err := r.resolveMaterializedFirebaseCredentialPath()
	if err != nil {
		return FirebaseMcpProviderConfigResponse{}, err
	}
	configPath := filepath.Join(accountHomePath, "config.toml")

	var config codexConfig
	changed := false
	raw, readErr := os.ReadFile(configPath)
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return FirebaseMcpProviderConfigResponse{}, fmt.Errorf("failed to read Codex config: %w", readErr)
	}
	if readErr == nil {
		if tomlErr := toml.Unmarshal(raw, &config); tomlErr != nil {
			config = codexConfig{}
			changed = true
		}
	}
	if config.McpServers == nil {
		config.McpServers = make(map[string]codexMcpServer)
	}

	// Migration: drop the pre-rename key so old+new don't coexist.
	if legacy := legacyMcpServerName(firebaseMcpServerName); legacy != "" {
		if _, ok := config.McpServers[legacy]; ok {
			delete(config.McpServers, legacy)
			changed = true
		}
	}

	expected := codexMcpServer{
		Command:           "npx",
		Args:              []string{"-y", "firebase-tools", "mcp", "--only", "crashlytics"},
		StartupTimeoutSec: 20,
		ToolTimeoutSec:    120,
		Enabled:           true,
		Env:               map[string]string{"GOOGLE_APPLICATION_CREDENTIALS": credentialPath},
	}
	existing, exists := config.McpServers[firebaseMcpServerName]
	if !exists || !codexServerConfigMatches(existing, expected) {
		config.McpServers[firebaseMcpServerName] = expected
		changed = true
	}

	if changed {
		if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
			return FirebaseMcpProviderConfigResponse{}, fmt.Errorf("failed to create config directory: %w", err)
		}
		out, err := toml.Marshal(config)
		if err != nil {
			return FirebaseMcpProviderConfigResponse{}, fmt.Errorf("failed to marshal TOML: %w", err)
		}
		if err := os.WriteFile(configPath, out, 0o644); err != nil {
			return FirebaseMcpProviderConfigResponse{}, fmt.Errorf("failed to write config: %w", err)
		}
	}

	return FirebaseMcpProviderConfigResponse{
		ProviderKey: "codex",
		ServerName:  firebaseMcpServerName,
		Status:      "configured",
		Changed:     changed,
		ConfigPath:  configPath,
	}, nil
}

func (r *Runner) ensureGeminiFirebaseMcpConfig(accountHomePath string) (FirebaseMcpProviderConfigResponse, error) {
	credentialPath, err := r.resolveMaterializedFirebaseCredentialPath()
	if err != nil {
		return FirebaseMcpProviderConfigResponse{}, err
	}
	configDir := filepath.Join(accountHomePath, ".gemini")
	configPath := filepath.Join(configDir, "settings.json")

	var config geminiSettings
	changed := false
	raw, readErr := os.ReadFile(configPath)
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return FirebaseMcpProviderConfigResponse{}, fmt.Errorf("failed to read Gemini config: %w", readErr)
	}
	if readErr == nil {
		if jsonErr := json.Unmarshal(raw, &config); jsonErr != nil {
			config = geminiSettings{}
			changed = true
		}
	}
	if config.McpServers == nil {
		config.McpServers = make(map[string]geminiMcpServer)
	}

	// Migration: drop the pre-rename key so old+new don't coexist.
	if legacy := legacyMcpServerName(firebaseMcpServerName); legacy != "" {
		if _, ok := config.McpServers[legacy]; ok {
			delete(config.McpServers, legacy)
			changed = true
		}
	}

	expected := geminiMcpServer{
		Command: "npx",
		Args:    []string{"-y", "firebase-tools", "mcp", "--only", "crashlytics"},
		Env:     map[string]string{"GOOGLE_APPLICATION_CREDENTIALS": credentialPath},
		Timeout: 600000,
		Trust:   false,
	}
	existing, exists := config.McpServers[firebaseMcpServerName]
	if !exists || !geminiServerConfigMatches(existing, expected) {
		config.McpServers[firebaseMcpServerName] = expected
		changed = true
	}

	if changed {
		if err := os.MkdirAll(configDir, 0o755); err != nil {
			return FirebaseMcpProviderConfigResponse{}, fmt.Errorf("failed to create config directory: %w", err)
		}
		out, err := json.MarshalIndent(config, "", "  ")
		if err != nil {
			return FirebaseMcpProviderConfigResponse{}, fmt.Errorf("failed to marshal JSON: %w", err)
		}
		if err := os.WriteFile(configPath, out, 0o644); err != nil {
			return FirebaseMcpProviderConfigResponse{}, fmt.Errorf("failed to write config: %w", err)
		}
	}

	return FirebaseMcpProviderConfigResponse{
		ProviderKey: "gemini",
		ServerName:  firebaseMcpServerName,
		Status:      "configured",
		Changed:     changed,
		ConfigPath:  configPath,
	}, nil
}

func (r *Runner) ensureGrokFirebaseMcpConfig(accountHomePath string) (FirebaseMcpProviderConfigResponse, error) {
	credentialPath, err := r.resolveMaterializedFirebaseCredentialPath()
	if err != nil {
		return FirebaseMcpProviderConfigResponse{}, err
	}
	configPath := filepath.Join(accountHomePath, "config.toml")

	doc := map[string]interface{}{}
	changed := false
	raw, readErr := os.ReadFile(configPath)
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return FirebaseMcpProviderConfigResponse{}, fmt.Errorf("failed to read Grok config: %w", readErr)
	}
	if readErr == nil {
		if tomlErr := toml.Unmarshal(raw, &doc); tomlErr != nil {
			doc = map[string]interface{}{}
			changed = true
		}
	}

	mcpServers, _ := doc["mcp_servers"].(map[string]interface{})
	if mcpServers == nil {
		mcpServers = map[string]interface{}{}
	}

	// Migration: drop the pre-rename key so old+new don't coexist.
	if legacy := legacyMcpServerName(firebaseMcpServerName); legacy != "" {
		if _, ok := mcpServers[legacy]; ok {
			delete(mcpServers, legacy)
			doc["mcp_servers"] = mcpServers
			changed = true
		}
	}

	expected := grokMcpServer{
		Command: "npx",
		Args:    []string{"-y", "firebase-tools", "mcp", "--only", "crashlytics"},
		Enabled: true,
		Env:     map[string]string{"GOOGLE_APPLICATION_CREDENTIALS": credentialPath},
	}
	existingMatches := false
	if existingRaw, exists := mcpServers[firebaseMcpServerName]; exists {
		if existingMap, ok := existingRaw.(map[string]interface{}); ok {
			if existingServer, ok := grokServerFromMap(existingMap); ok {
				existingMatches = grokServerConfigMatches(existingServer, expected)
			}
		}
	}
	if !existingMatches {
		expectedMap, mapErr := grokServerToMap(expected)
		if mapErr != nil {
			return FirebaseMcpProviderConfigResponse{}, fmt.Errorf("failed to build Grok MCP server entry: %w", mapErr)
		}
		mcpServers[firebaseMcpServerName] = expectedMap
		doc["mcp_servers"] = mcpServers
		changed = true
	}

	if changed {
		if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
			return FirebaseMcpProviderConfigResponse{}, fmt.Errorf("failed to create config directory: %w", err)
		}
		out, err := toml.Marshal(doc)
		if err != nil {
			return FirebaseMcpProviderConfigResponse{}, fmt.Errorf("failed to marshal TOML: %w", err)
		}
		if err := os.WriteFile(configPath, out, 0o644); err != nil {
			return FirebaseMcpProviderConfigResponse{}, fmt.Errorf("failed to write config: %w", err)
		}
	}

	return FirebaseMcpProviderConfigResponse{
		ProviderKey: "grok",
		ServerName:  firebaseMcpServerName,
		Status:      "configured",
		Changed:     changed,
		ConfigPath:  configPath,
	}, nil
}

// checkClaudeFirebaseMcpConfig is a read-only staleness check, mirroring
// checkClaudeJiraMcpConfig — used by PreflightFirebaseMcp so preflight never
// has side effects.
func (r *Runner) checkClaudeFirebaseMcpConfig(accountHomePath string) (configured bool, stale bool, err error) {
	configPath := filepath.Join(strings.TrimSpace(accountHomePath), ".claude.json")
	raw, readErr := os.ReadFile(configPath)
	if readErr != nil {
		if errors.Is(readErr, os.ErrNotExist) {
			return false, false, nil
		}
		return false, false, fmt.Errorf("failed to read Claude config: %w", readErr)
	}
	var config claudeConfig
	if err := json.Unmarshal(raw, &config); err != nil {
		return false, false, fmt.Errorf("failed to parse Claude config: %w", err)
	}
	existing, ok := config.McpServers[firebaseMcpServerName]
	if !ok {
		return false, false, nil
	}
	if existing.Type != "stdio" || existing.Command != "npx" || len(existing.Args) == 0 {
		return true, true, nil
	}
	return true, false, nil
}

// PreflightFirebaseMcp mirrors PreflightJiraMcp: checks Firebase is
// connected, then (when a provider/account is given) whether that
// provider's config has a non-stale firebase MCP server entry. Never writes
// config.
func (r *Runner) PreflightFirebaseMcp(providerKey string, accountHomePath string) MCPPreflightCheck {
	result := MCPPreflightCheck{}

	records, err := r.loadMcpBackendRecords()
	if err != nil {
		result.ErrorMessage = fmt.Sprintf("Failed to check Firebase MCP status: %v", err)
		return result
	}
	backend := r.detectFirebaseBackend(records["firebase"])
	if !backend.Installed {
		if strings.TrimSpace(backend.LastError) != "" {
			result.ErrorMessage = backend.LastError
		} else {
			result.ErrorMessage = "Firebase is not connected yet. Upload a service account JSON in MCP Servers settings."
		}
		return result
	}
	// MCPPreflightCheck's field name predates this generalization (Task-227).
	result.GoogleDriveReady = true

	providerKey = strings.ToLower(strings.TrimSpace(providerKey))
	accountHomePath = strings.TrimSpace(accountHomePath)
	if providerKey == "" || accountHomePath == "" {
		result.ProviderConfigured = true
		return result
	}

	switch providerKey {
	case "claude":
		configured, stale, err := r.checkClaudeFirebaseMcpConfig(accountHomePath)
		if err != nil {
			result.ErrorMessage = fmt.Sprintf("Failed to check Claude Firebase MCP config: %v", err)
			return result
		}
		if !configured {
			result.ErrorMessage = "The selected AI provider is not configured with the firebase MCP server. Run Configure Providers first."
			return result
		}
		if stale {
			result.ErrorMessage = "Provider has stale Firebase MCP config. Re-run Configure Providers."
			return result
		}
	default:
		result.ErrorMessage = fmt.Sprintf("Firebase MCP provider config is not yet implemented for provider: %s", providerKey)
		return result
	}

	result.ProviderConfigured = true
	return result
}

// buildFirebaseMcpInstructions mirrors buildJiraMcpInstructions, scoped to
// the Crashlytics read tools the official firebase-tools MCP exposes
// (CP-05-04 survey, 2026-07-13): crashlytics_get_issue,
// crashlytics_list_events, crashlytics_batch_get_events.
func buildFirebaseMcpInstructions(providerKey string, allowWrite bool, yoloMode bool) string {
	var sb strings.Builder
	sb.WriteString("## Required MCP Usage\n\n")
	sb.WriteString("This workflow step requires FlowPilot MCP `firebase`.\n")
	sb.WriteString(fmt.Sprintf("The configured provider MCP server name is `%s`.\n\n", firebaseMcpServerName))
	sb.WriteString("This step is restricted to `read_only` Firebase Crashlytics operations.\n")
	sb.WriteString(fmt.Sprintf("Before producing the final answer, use Firebase MCP tools from `%s` to fetch the crash context this run requires.\n\n", firebaseMcpServerName))
	sb.WriteString("Preferred tools:\n")
	sb.WriteString("- `crashlytics_get_issue` for the selected crash issue's metadata\n")
	sb.WriteString("- `crashlytics_list_events` or `crashlytics_batch_get_events` for stack traces / sample crash events\n\n")
	sb.WriteString("Rules:\n")
	sb.WriteString("- Do not invent crash content or stack traces.\n")
	sb.WriteString("- Only read the crash issue already selected for this run; do not run a broader Crashlytics search.\n")
	sb.WriteString(fmt.Sprintf("- If `%s` is unavailable, stop and end the response with `MCP_FAILURE_CODE: MCP_UNAVAILABLE`.\n", firebaseMcpServerName))
	sb.WriteString("- If auth is missing or invalid, stop and end the response with `MCP_FAILURE_CODE: MCP_AUTH_REQUIRED`.\n")
	sb.WriteString("- If the required crash issue cannot be found, end the response with `MCP_FAILURE_CODE: FIREBASE_CONTENT_NOT_FOUND`.\n")
	sb.WriteString("- Include the crash issue id for every crash event used.\n")
	sb.WriteString("- Write/mutation Crashlytics tools are not allowed in this MCP server (read-only v1).\n")
	return sb.String()
}
