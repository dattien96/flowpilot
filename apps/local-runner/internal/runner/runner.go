package runner

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

const Version = "dev"

var (
	lookPathFn   = exec.LookPath
	runCommandFn = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return exec.CommandContext(ctx, name, args...).CombinedOutput()
	}
	httpRequestFn = func(
		ctx context.Context,
		method string,
		endpoint string,
		headers map[string]string,
		body []byte,
	) (int, []byte, error) {
		var requestBody *strings.Reader
		if body != nil {
			requestBody = strings.NewReader(string(body))
		} else {
			requestBody = strings.NewReader("")
		}

		request, err := http.NewRequestWithContext(ctx, method, endpoint, requestBody)
		if err != nil {
			return 0, nil, err
		}

		for key, value := range headers {
			if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
				continue
			}
			request.Header.Set(key, value)
		}

		response, err := http.DefaultClient.Do(request)
		if err != nil {
			return 0, nil, err
		}
		defer response.Body.Close()

		responseBody, err := io.ReadAll(response.Body)
		if err != nil {
			return 0, nil, err
		}

		return response.StatusCode, responseBody, nil
	}
	probeRemoteBackendFn = func(ctx context.Context, endpoint string, headers map[string]string) error {
		statusCode, _, err := httpRequestFn(ctx, http.MethodGet, endpoint, headers, nil)
		if err != nil {
			return err
		}

		if statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden {
			return fmt.Errorf("remote backend authorization failed: %d", statusCode)
		}

		if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
			return fmt.Errorf("remote backend check failed: %d", statusCode)
		}

		return nil
	}
	executeJiraRequestFn = func(
		ctx context.Context,
		method string,
		endpoint string,
		creds jiraCredential,
		payload []byte,
	) ([]byte, error) {
		auth := base64.StdEncoding.EncodeToString([]byte(creds.Email + ":" + creds.ApiToken))
		headers := map[string]string{
			"Authorization": "Basic " + auth,
			"Accept":        "application/json",
		}
		if payload != nil {
			headers["Content-Type"] = "application/json"
		}

		statusCode, responseBody, err := httpRequestFn(ctx, method, endpoint, headers, payload)
		if err != nil {
			return nil, err
		}
		if statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden {
			return nil, fmt.Errorf("remote backend authorization failed: %d", statusCode)
		}
		if statusCode < http.StatusOK || statusCode >= http.StatusMultipleChoices {
			return nil, fmt.Errorf("jira request failed: %d %s", statusCode, strings.TrimSpace(string(responseBody)))
		}

		return responseBody, nil
	}
	safeShellTokenPattern = regexp.MustCompile(`^[A-Za-z0-9_./:=+-]+$`)
)

type Runner struct {
	workspace   string
	startedAt   time.Time
	secretStore SecretStore
	sessionsMu  sync.Mutex
	sessions    map[string]*LiveSession

	// codexAppServer is the single shared `codex app-server` process (04-03/04-06),
	// bound to the active provider account scope. nil until first ensure.
	codexAppServerMu sync.Mutex
	codexAppServer   *codexAppServerHandle

	// claudePool owns the per-(account,cwd,session) `claude` CLI processes (07 plan).
	// Claude has no shared multi-thread process like Codex app-server, so the Go runner
	// is the multiplexer: concurrent sessions = concurrent processes.
	claudePool *claudeProcessPool

	// geminiSessions tracks FlowPilot synthetic session ids to Gemini ACP session ids
	// across per-turn adapter instances.
	geminiSessions *geminiSessionMap

	// claudeMCP is the runner-hosted MCP server for the Claude permission/ask_user tools
	// (07); mcpBaseURL is the runner's own base URL ("http://host:port"), set at startup
	// so the adapter can build per-turn --mcp-config URLs.
	claudeMCP    *claudeMCPServer
	mcpBaseURLMu sync.RWMutex
	mcpBaseURL   string

	// grokProcessMu guards grokProcesses.
	grokProcessMu sync.Mutex
	// grokProcesses holds live `grok agent stdio` processes keyed by
	// scope+model+reasoningEffort+alwaysApprove (see grokProcessKey). Grok exposes
	// model/effort/approve only as `grok agent` LAUNCH flags — there is no ACP
	// session-level switch — so each distinct combination needs its own OS process.
	// Unlike the original single-handle design (CP-46/Task-206), these now COEXIST
	// instead of tearing each other down: a parent turn and a concurrently-spawned
	// child turn with different launch flags each keep their own process, so the
	// child no longer kills the parent's in-flight process ("grok agent process
	// torn down"). Only an account/scope change reclaims processes. nil until first ensure.
	grokProcesses map[string]*grokProcessHandle
	// grokDesiredAlwaysApprove is the current YOLO posture for Grok (Task-218),
	// set explicitly by ApplyGrokYoloPosture rather than threaded through the
	// shared ProviderRegistration/Adapter() call chain (which Claude/Codex/Gemini
	// would also have to accept and ignore). ensureGrokProcess reads this to
	// decide whether to pass --always-approve at launch. Guarded by grokProcessMu.
	grokDesiredAlwaysApprove bool
}

func New(workspace string) (*Runner, error) {
	resolved, err := ResolveWorkspace(workspace)
	if err != nil {
		return nil, err
	}
	if err := loadWorkspaceEnvFile(resolved); err != nil {
		return nil, err
	}

	return &Runner{
		workspace:      resolved,
		startedAt:      time.Now().UTC(),
		secretStore:    newDefaultSecretStore(),
		sessions:       make(map[string]*LiveSession),
		claudePool:     newClaudeProcessPool(),
		geminiSessions: newGeminiSessionMap(),
		claudeMCP:      newClaudeMCPServer(),
	}, nil
}

func loadWorkspaceEnvFile(workspace string) error {
	envPath := filepath.Join(workspace, ".env")
	file, err := os.Open(envPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}

		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}

		value = strings.TrimSpace(value)
		if len(value) >= 2 {
			if (strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"")) ||
				(strings.HasPrefix(value, "'") && strings.HasSuffix(value, "'")) {
				value = value[1 : len(value)-1]
			}
		}

		if err := os.Setenv(key, value); err != nil {
			return err
		}
	}

	return scanner.Err()
}

func (r *Runner) Health() Health {
	return Health{
		Status:        "online",
		RunnerVersion: Version,
		Cwd:           r.workspace,
		Os:            runtime.GOOS,
		StartedAt:     r.startedAt.Format(time.RFC3339Nano),
	}
}

func (r *Runner) PickDirectory(ctx context.Context) (DirectorySelection, error) {
	var command string
	var args []string

	switch runtime.GOOS {
	case "darwin":
		command = "osascript"
		args = []string{
			"-e",
			`POSIX path of (choose folder with prompt "Select project folder")`,
		}
	case "linux":
		command = "zenity"
		args = []string{
			"--file-selection",
			"--directory",
			"--title=Select project folder",
		}
	case "windows":
		command = "powershell"
		args = []string{
			"-NoProfile",
			"-Command",
			`Add-Type -AssemblyName System.Windows.Forms; $dialog = New-Object System.Windows.Forms.FolderBrowserDialog; $dialog.Description = 'Select project folder'; if ($dialog.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) { Write-Output $dialog.SelectedPath }`,
		}
	default:
		return DirectorySelection{}, fmt.Errorf("directory picker is not supported on %s", runtime.GOOS)
	}

	output, err := runCommandFn(ctx, command, args...)
	if err != nil {
		return DirectorySelection{}, err
	}

	selectedPath := filepath.Clean(strings.TrimSpace(string(output)))
	if selectedPath == "." || selectedPath == "" {
		return DirectorySelection{}, errors.New("no directory was selected")
	}

	return DirectorySelection{Path: selectedPath}, nil
}

func (r *Runner) ValidateDirectory(path string) DirectoryValidationResult {
	candidate := strings.TrimSpace(path)
	if candidate == "" {
		return DirectoryValidationResult{
			Path:   candidate,
			Usable: false,
			Reason: "path is empty",
		}
	}

	resolved, err := filepath.Abs(candidate)
	if err != nil {
		return DirectoryValidationResult{
			Path:   candidate,
			Usable: false,
			Reason: fmt.Sprintf("could not resolve path: %v", err),
		}
	}

	info, err := os.Stat(resolved)
	if err != nil {
		reason := err.Error()
		if errors.Is(err, os.ErrNotExist) {
			reason = "path does not exist"
		}
		return DirectoryValidationResult{
			Path:   resolved,
			Usable: false,
			Reason: reason,
		}
	}

	if !info.IsDir() {
		return DirectoryValidationResult{
			Path:   resolved,
			Usable: false,
			Reason: "path is not a directory",
		}
	}

	return DirectoryValidationResult{
		Path:   resolved,
		Usable: true,
		Reason: "",
	}
}

func (r *Runner) DetectProviders(ctx context.Context) ([]Provider, error) {
	providers := make([]Provider, 0, len(providerSpecs()))
	for _, spec := range providerSpecs() {
		providers = append(providers, detectProvider(ctx, spec))
	}

	return providers, nil
}

func (r *Runner) ListProviders(ctx context.Context) (ProviderInventory, error) {
	providers, err := r.DetectProviders(ctx)
	if err != nil {
		return ProviderInventory{}, err
	}

	return ProviderInventory{Providers: providers}, nil
}

func (r *Runner) InstallProvider(ctx context.Context, providerName string) (ProviderInventory, error) {
	spec, ok := lookupProviderSpec(providerName)
	if !ok {
		return ProviderInventory{}, fmt.Errorf("unsupported AI provider %q", providerName)
	}

	current := detectProvider(ctx, spec)
	if current.InstallStatus == "INSTALLED" {
		return r.ListProviders(ctx)
	}

	if current.InstallStatus == "UNSUPPORTED_OS" {
		inventory, err := r.ListProviders(ctx)
		if err != nil {
			return ProviderInventory{}, err
		}
		message := stringValueOrFallback(current.LastError, "provider installation is not supported on this operating system")
		annotateProviderInventory(&inventory, spec.Key, "UNSUPPORTED_OS", &message, false, "UNKNOWN")
		return inventory, errors.New(message)
	}

	if err := runProviderInstallCommand(ctx, spec); err != nil {
		inventory, listErr := r.ListProviders(ctx)
		if listErr != nil {
			return ProviderInventory{}, listErr
		}
		message := err.Error()
		status := "FAILED"
		if strings.Contains(strings.ToLower(message), "not supported") {
			status = "UNSUPPORTED_OS"
		}
		annotateProviderInventory(&inventory, spec.Key, status, &message, false, "UNKNOWN")
		return inventory, err
	}

	inventory, err := r.ListProviders(ctx)
	if err != nil {
		return ProviderInventory{}, err
	}

	refreshed := findProvider(inventory.Providers, spec.Key)
	if refreshed == nil {
		return inventory, fmt.Errorf("provider %q was not found after installation refresh", spec.Key)
	}
	if refreshed.InstallStatus != "INSTALLED" {
		message := stringValueOrFallback(refreshed.LastError, fmt.Sprintf("%s install did not complete", spec.Label))
		annotateProviderInventory(&inventory, spec.Key, "FAILED", &message, false, "UNKNOWN")
		return inventory, errors.New(message)
	}

	return inventory, nil
}

func (r *Runner) ListMcpBackends(ctx context.Context) ([]McpBackend, error) {
	specs := mcpBackendSpecs()
	records, err := r.loadMcpBackendRecords()
	if err != nil {
		return nil, err
	}

	backends := make([]McpBackend, 0, len(specs))

	for _, spec := range specs {
		var backend McpBackend
		if spec.Key == "jira" {
			backend = r.detectJiraBackend(records[spec.Key])
		} else {
			backend = spec.detect(records[spec.Key])
		}
		backends = append(backends, backend)
	}

	return backends, nil
}

func (r *Runner) InstallMcpBackend(ctx context.Context, backendKey string) (McpBackend, error) {
	spec, ok := lookupMcpBackendSpec(backendKey)
	if !ok {
		return McpBackend{}, fmt.Errorf("unsupported MCP backend %q", backendKey)
	}

	if backendKey == "jira" {
		return r.detectJiraBackend(mcpBackendRecord{}), errors.New("jira MCP uses the project form and Atlassian API token flow")
	}

	records, err := r.loadMcpBackendRecords()
	if err != nil {
		return McpBackend{}, err
	}

	backend := spec.detect(records[backendKey])
	if strings.TrimSpace(backend.BinaryPath) == "" {
		return backend, errors.New(nonEmptyOrFallback(backend.LastError, "mcp backend launcher is not available"))
	}

	if backendKey == "google_drive" {
		if err := r.executeGoogleDriveMcpCommand(ctx, backend.BinaryPath, spec.InstallArgs); err != nil {
			backend.Installed = false
			backend.State = "missing"
			backend.Action = "install"
			backend.ActionLabel = "Install"
			backend.LastError = backendErrorMessage(err, nil)
			_ = r.saveMcpBackendRecord(backend)
			return backend, err
		}
		backend.Installed = true
		backend.State = "installed"
		backend.LastError = ""
		if err := r.saveMcpBackendRecord(backend); err != nil {
			return backend, err
		}
		backend.Action = "verify"
		backend.ActionLabel = "Verify"
		return backend, nil
	}

	if err := spec.execute(ctx, &backend, spec.InstallArgs); err != nil {
		_ = r.saveMcpBackendRecord(backend)
		return backend, err
	}

	if err := r.saveMcpBackendRecord(backend); err != nil {
		return backend, err
	}

	backend.Action = "verify"
	backend.ActionLabel = "Verify"
	return backend, nil
}

func (r *Runner) VerifyMcpBackend(
	ctx context.Context,
	backendKey string,
	projectID string,
	integrationID string,
) (McpBackend, error) {
	spec, ok := lookupMcpBackendSpec(backendKey)
	if !ok {
		return McpBackend{}, fmt.Errorf("unsupported MCP backend %q", backendKey)
	}

	if backendKey == "jira" {
		records, err := r.loadMcpBackendRecords()
		if err != nil {
			return McpBackend{}, err
		}
		backend := r.detectJiraBackend(records[backendKey])
		secretKey := strings.TrimSpace(backend.SecretKey)
		if strings.TrimSpace(integrationID) != "" {
			secretKey = jiraCredentialKey(integrationID)
			backend.SecretKey = secretKey
		}
		creds, err := r.loadJiraCredential(secretKey)
		if err != nil {
			return backend, err
		}
		requestProjectID := strings.TrimSpace(projectID)
		if strings.TrimSpace(creds.ProjectID) == "" && requestProjectID != "" {
			creds.ProjectID = requestProjectID
			if strings.TrimSpace(integrationID) != "" {
				if saveErr := r.saveJiraCredential(integrationID, creds); saveErr != nil {
					return backend, saveErr
				}
			}
		}
		if err := r.verifyJiraCredential(ctx, creds); err != nil {
			backend.LastError = err.Error()
			backend.LastCheckedAt = time.Now().UTC().Format(time.RFC3339Nano)
			_ = r.saveMcpBackendRecord(backend)
			return backend, err
		}
		backend.Installed = true
		backend.State = "installed"
		backend.Action = "verify"
		backend.ActionLabel = "Verify"
		backend.LastError = ""
		backend.LastCheckedAt = time.Now().UTC().Format(time.RFC3339Nano)
		if err := r.saveMcpBackendRecord(backend); err != nil {
			return backend, err
		}
		return backend, nil
	}

	records, err := r.loadMcpBackendRecords()
	if err != nil {
		return McpBackend{}, err
	}

	backend := spec.detect(records[backendKey])
	if !backend.Installed {
		return backend, errors.New(nonEmptyOrFallback(backend.LastError, "mcp backend is not installed"))
	}

	if backendKey == "google_drive" {
		if err := r.executeGoogleDriveMcpCommand(ctx, backend.BinaryPath, spec.VerifyArgs); err != nil {
			backend.Installed = false
			backend.State = "missing"
			backend.Action = "install"
			backend.ActionLabel = "Install"
			backend.LastError = backendErrorMessage(err, nil)
			_ = r.saveMcpBackendRecord(backend)
			return backend, err
		}
		backend.Installed = true
		backend.State = "installed"
		backend.LastError = ""
		if err := r.saveMcpBackendRecord(backend); err != nil {
			return backend, err
		}
		backend.Action = "verify"
		backend.ActionLabel = "Verify"
		return backend, nil
	}

	if err := spec.execute(ctx, &backend, spec.VerifyArgs); err != nil {
		_ = r.saveMcpBackendRecord(backend)
		return backend, err
	}

	if err := r.saveMcpBackendRecord(backend); err != nil {
		return backend, err
	}

	backend.Action = "verify"
	backend.ActionLabel = "Verify"
	return backend, nil
}

func (r *Runner) PrepareMcpBackend(ctx context.Context, backendKey string) (McpBackend, error) {
	records, err := r.loadMcpBackendRecords()
	if err != nil {
		return McpBackend{}, err
	}

	spec, ok := lookupMcpBackendSpec(backendKey)
	if !ok {
		return McpBackend{}, fmt.Errorf("unsupported MCP backend %q", backendKey)
	}

	backend := spec.detect(records[backendKey])
	if backend.Installed {
		return r.VerifyMcpBackend(ctx, backendKey, "", "")
	}

	return r.InstallMcpBackend(ctx, backendKey)
}

func (r *Runner) ListSkills() ([]Skill, error) {
	entries, err := discoverMarkdownEntries(filepath.Join(r.workspace, ".agents", "skills"), true)
	if err != nil {
		return nil, err
	}

	skills := make([]Skill, 0, len(entries))
	for _, entry := range entries {
		skills = append(skills, Skill{
			ID:          entry.ID,
			Name:        entry.Name,
			FilePath:    entry.FilePath,
			Description: entry.Description,
			Tags:        entry.Tags,
		})
	}

	return skills, nil
}

func (r *Runner) ListFlows() ([]Flow, error) {
	entries, err := discoverMarkdownEntries(filepath.Join(r.workspace, ".agents", "flows"), false)
	if err != nil {
		return nil, err
	}

	flows := make([]Flow, 0, len(entries))
	for _, entry := range entries {
		flows = append(flows, Flow{
			ID:          entry.ID,
			Name:        entry.Name,
			FilePath:    entry.FilePath,
			Description: entry.Description,
			Steps:       entry.Steps,
		})
	}

	return flows, nil
}

func (r *Runner) TriggerIntegrationConnection(
	ctx context.Context,
	integrationID string,
	request IntegrationConnectionRequest,
) (IntegrationConnectionResult, error) {
	if strings.TrimSpace(integrationID) == "" {
		return IntegrationConnectionResult{}, errors.New("integrationId is required")
	}
	if strings.TrimSpace(request.ProjectID) == "" {
		return IntegrationConnectionResult{}, errors.New("projectId is required")
	}
	if strings.TrimSpace(request.ProviderType) == "" {
		return IntegrationConnectionResult{}, errors.New("providerType is required")
	}
	switch strings.TrimSpace(request.Action) {
	case "test", "retry":
	default:
		return IntegrationConnectionResult{}, fmt.Errorf("unsupported action %q", request.Action)
	}

	providerType := strings.TrimSpace(request.ProviderType)
	providerLabel := providerLabelForType(providerType)
	message := fmt.Sprintf("Connection request accepted for project %s.", request.ProjectID)
	status := "pending"

	switch providerType {
	case "google_drive":
		backend, err := r.PrepareMcpBackend(ctx, providerType)
		if err != nil {
			message = backend.InstallHint
			if message == "" {
				message = backend.LastError
			}
			if message == "" {
				message = fmt.Sprintf("Unable to prepare %s MCP backend.", backend.Label)
			}
			return IntegrationConnectionResult{
				RequestStatus:     "rejected",
				IntegrationID:     integrationID,
				IntegrationStatus: "failed",
				RunID:             nil,
				Message:           &message,
			}, nil
		}
		status = "awaiting_oauth"
		message = fmt.Sprintf(
			"%s backend is ready for project %s. Complete provider auth in the browser if prompted.",
			backend.Label,
			request.ProjectID,
		)
	case "jira":
		creds, err := r.resolveJiraCredential(integrationID, request)
		if err != nil {
			message = err.Error()
			return IntegrationConnectionResult{
				RequestStatus:     "rejected",
				IntegrationID:     integrationID,
				IntegrationStatus: "failed",
				RunID:             nil,
				Message:           &message,
			}, nil
		}
		if err := r.verifyJiraCredential(ctx, creds); err != nil {
			message = err.Error()
			return IntegrationConnectionResult{
				RequestStatus:     "rejected",
				IntegrationID:     integrationID,
				IntegrationStatus: "failed",
				RunID:             nil,
				Message:           &message,
			}, nil
		}
		if err := r.saveJiraCredential(integrationID, creds); err != nil {
			message = err.Error()
			return IntegrationConnectionResult{
				RequestStatus:     "rejected",
				IntegrationID:     integrationID,
				IntegrationStatus: "failed",
				RunID:             nil,
				Message:           &message,
			}, nil
		}
		backend := r.detectJiraBackend(mcpBackendRecord{
			Installed:     true,
			LastCheckedAt: time.Now().UTC().Format(time.RFC3339Nano),
			LastError:     "",
			SecretKey:     jiraCredentialKey(integrationID),
		})
		_ = r.saveMcpBackendRecord(backend)
		status = "connected"
		message = fmt.Sprintf(
			"%s API-token connection is ready for project %s.",
			backend.Label,
			request.ProjectID,
		)
	case "firebase", "telegram", "figma":
		message = fmt.Sprintf(
			"%s connection request accepted for project %s.",
			providerLabel,
			request.ProjectID,
		)
	default:
		return IntegrationConnectionResult{}, fmt.Errorf("unsupported provider type %q", providerType)
	}

	return IntegrationConnectionResult{
		RequestStatus:     "accepted",
		IntegrationID:     integrationID,
		IntegrationStatus: status,
		RunID:             nil,
		Message:           &message,
	}, nil
}

func (r *Runner) DeleteIntegrationConnection(ctx context.Context, integrationID string) error {
	if strings.TrimSpace(integrationID) == "" {
		return errors.New("integrationId is required")
	}

	if err := r.deleteJiraCredential(integrationID); err != nil && !strings.Contains(err.Error(), "not found") {
		return err
	}

	records, err := r.loadMcpBackendRecords()
	if err != nil {
		return err
	}

	backend := records["jira"]
	if backend.SecretKey == jiraCredentialKey(integrationID) {
		backend.Installed = false
		backend.LastCheckedAt = time.Now().UTC().Format(time.RFC3339Nano)
		backend.LastError = "No Jira API token is stored for the runner yet."
		backend.SecretKey = ""
		if err := r.saveMcpBackendRecord(McpBackend{
			Key:           "jira",
			ProviderType:  "jira",
			Label:         "Atlassian MCP",
			Transport:     "remote",
			Launcher:      "remote",
			InstallHint:   "Open the Jira MCP form, read the Atlassian guide, and paste an API token so the runner can connect to the Atlassian remote MCP server.",
			State:         "missing",
			Action:        "install",
			ActionLabel:   "Create MCP",
			LastCheckedAt: backend.LastCheckedAt,
			LastError:     backend.LastError,
		}); err != nil {
			return err
		}
	}

	return nil
}

func resolvePromptExecutionAdapter(request PromptExecutionRequest, outputPath, workspace string) (string, []string, string, error) {
	modelName := strings.TrimSpace(request.ModelName)
	providerKey := strings.TrimSpace(request.ProviderKey)
	lowerModel := strings.ToLower(modelName)

	resolvedProvider := providerKey
	switch {
	case strings.HasPrefix(lowerModel, "gpt-"):
		resolvedProvider = "codex"
	case strings.HasPrefix(lowerModel, "gemini-"):
		resolvedProvider = "gemini"
	case strings.HasPrefix(lowerModel, "claude-"):
		resolvedProvider = "claude"
	case strings.HasPrefix(lowerModel, "grok-"), lowerModel == "grok-build":
		// Appended last (CP-46 P-0/Task-212 T-3): codex/gemini/claude cases above unchanged.
		resolvedProvider = "grok"
	case resolvedProvider == "":
		return "", nil, "", errors.New("model or provider is required")
	}

	switch resolvedProvider {
	case "codex":
		sandboxMode := "read-only"
		if request.AllowWrite {
			sandboxMode = "workspace-write"
		}
		args := []string{"--sandbox", sandboxMode, "exec"}
		if modelName != "" {
			args = append(args, "-c", fmt.Sprintf("model=%q", modelName))
		}
		if request.ReasoningEffort != "" {
			args = append(args, "-c", fmt.Sprintf("model_reasoning_effort=%s", strings.ToLower(strings.TrimSpace(request.ReasoningEffort))))
		}
		args = append(args, "--output-last-message", outputPath, "-")
		return "codex", args, resolvedProvider, nil
	case "claude":
		args := []string{"--print"}
		if modelName != "" {
			cliModel := modelName
			if strings.HasPrefix(lowerModel, "claude-") {
				cliModel = strings.TrimPrefix(lowerModel, "claude-")
			}
			args = append(args, "--model", cliModel)
		}
		if request.ReasoningEffort != "" {
			// No xhigh->max remap here: the claude CLI's --effort validator
			// accepts both as distinct values (live-verified against the
			// installed @anthropic-ai/claude-code binary, Task-215 follow-up)
			// — see normalizeClaudeEffort's doc comment for the same finding.
			args = append(args, "--effort", strings.ToLower(request.ReasoningEffort))
		}
		return "claude", args, resolvedProvider, nil
	case "gemini":
		projectID, resume, err := geminiSessionProjectID("", workspace, geminiProjectEnvHints(request.AccountHomePath, request.CustomEnv))
		if err != nil {
			return "", nil, "", err
		}
		args := geminiCLIArgs(workspace, projectID, normalizeGeminiModelName(modelName), request.AllowWrite || request.YoloMode, resume, false)
		return geminiBinaryName(), args, resolvedProvider, nil
	case "grok":
		// Appended last (CP-46 P-0/P-13, Task-212 T-3): the ONLY place Grok uses
		// one-shot `grok -p/--single` instead of the ACP `agent stdio` turn loop
		// — the summarizer/prompt-execution path, which has no session/tool/
		// approval state to preserve. `-p, --single <PROMPT>` takes the prompt
		// as the flag's own value (live-verified: `grok --help`), so the actual
		// prompt text is appended by the caller (ExecutePrompt's usesPromptArg),
		// same as Gemini's `--print <prompt>` — NOT via stdin like codex/claude.
		// `--effort` accepts the full canonical vocabulary directly here
		// (live-verified in docs, unlike the ACP session path which has no
		// reasoningEffort field at all).
		args := []string{"--output-format", "json"}
		if modelName != "" {
			args = append(args, "--model", modelName)
		}
		if request.ReasoningEffort != "" {
			args = append(args, "--effort", strings.ToLower(strings.TrimSpace(request.ReasoningEffort)))
		}
		return grokBinaryName(), args, resolvedProvider, nil
	default:
		return "", nil, "", fmt.Errorf("provider %q is not supported", resolvedProvider)
	}
}

func normalizeGeminiModelName(modelName string) string {
	trimmed := strings.TrimSpace(modelName)
	lowerModel := strings.ToLower(trimmed)

	switch lowerModel {
	case "flash", "gemini-flash":
		return "gemini-3.5-flash-medium"
	case "pro", "gemini-pro":
		return "gemini-3.1-pro-high"
	case "auto-gemini-3", "auto-gemini-2.5":
		return "gemini-3.5-flash-medium"
	case "gemini-3-pro-preview", "gemini-3.1-pro-preview", "gemini-3.1-pro-preview-customtools", "gemini-2.5-pro":
		return "gemini-3.1-pro-high"
	case "gemini-3-flash-preview", "gemini-2.5-flash":
		return "gemini-3.5-flash-medium"
	case "gemini-3.1-flash-lite-preview", "gemini-2.5-flash-lite":
		return "gemini-3.5-flash-low"
	default:
		return trimmed
	}
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	if safeShellTokenPattern.MatchString(value) {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func formatProviderCommand(binary string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, shellQuote(binary))
	for _, arg := range args {
		parts = append(parts, shellQuote(arg))
	}
	return strings.Join(parts, " ")
}

func formatShellCommand(binary string, args []string, stdinPath string) string {
	return formatProviderCommand(binary, args) + " < " + shellQuote(stdinPath)
}

func (r *Runner) ExecutePrompt(ctx context.Context, request PromptExecutionRequest) (PromptExecutionResult, error) {
	if strings.TrimSpace(request.Prompt) == "" {
		return PromptExecutionResult{}, errors.New("prompt is required")
	}

	workspace := r.workspace
	if strings.TrimSpace(request.WorkingDirectory) != "" {
		resolved, err := filepath.Abs(request.WorkingDirectory)
		if err != nil {
			return PromptExecutionResult{}, err
		}
		workspace = resolved
	}

	timeout := 10 * time.Minute
	if request.TimeoutMs > 0 {
		timeout = time.Duration(request.TimeoutMs) * time.Millisecond
	}

	runID := newRunID()
	runDir := filepath.Join(r.workspace, ".flowpilot", "runs", runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return PromptExecutionResult{}, err
	}

	promptPath := filepath.Join(runDir, "prompt.txt")
	stdoutPath := filepath.Join(runDir, "stdout.txt")
	stderrPath := filepath.Join(runDir, "stderr.txt")
	outputPath := filepath.Join(runDir, "output.md")
	commandPath := filepath.Join(runDir, "command.txt")
	metadataPath := filepath.Join(runDir, "metadata.json")

	promptWithHistory := r.injectFeatureHistory(workspace, request.Prompt)
	promptWithSkills := r.injectSkillContent(workspace, promptWithHistory, request.SkillIds)
	actualPrompt, err := r.preparePromptForRequiredMcps(
		promptWithSkills,
		request.RequiredMcps,
		request.ProviderKey,
		request.AccountHomePath,
		request.AllowWrite,
		request.YoloMode,
	)
	if err != nil {
		return PromptExecutionResult{}, err
	}

	if err := os.WriteFile(promptPath, []byte(actualPrompt), 0o644); err != nil {
		return PromptExecutionResult{}, err
	}

	binary, args, resolvedProvider, err := resolvePromptExecutionAdapter(request, outputPath, workspace)
	if err != nil {
		return PromptExecutionResult{}, err
	}
	usesPromptArg := resolvedProvider == string(ProviderKeyGemini) || resolvedProvider == string(ProviderKeyGrok)
	if usesPromptArg {
		if resolvedProvider == string(ProviderKeyGrok) {
			// Appended last (CP-46 P-0/Task-212 T-3): -p/--single takes the
			// prompt as its own value, unlike Gemini's --print <prompt>.
			args = append(args, "-p", actualPrompt)
		} else {
			args = append(args, "--print", actualPrompt)
		}
	}
	if resolvedProvider == string(ProviderKeyGemini) {
		projectID := ""
		for i, arg := range args {
			if arg == "--project" && i+1 < len(args) {
				projectID = args[i+1]
				break
			}
		}
		logGeminiAgyLaunch("prompt", binary, args, geminiLaunchDebug{
			Cwd:       workspace,
			ProjectID: projectID,
			Env:       geminiProjectEnvHints(request.AccountHomePath, request.CustomEnv),
		})
	}

	command := formatShellCommand(binary, args, promptPath)
	if usesPromptArg {
		command = formatProviderCommand(binary, args)
	}
	if err := os.WriteFile(commandPath, []byte(command), 0o644); err != nil {
		return PromptExecutionResult{}, err
	}

	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(execCtx, binary, args...)
	cmd.Env = r.getEnvForExecution(request.ProviderKey, request.AccountHomePath, request.CustomEnv, request.ProxyURL)
	stdoutFile, err := os.Create(stdoutPath)
	if err != nil {
		return PromptExecutionResult{}, err
	}
	defer stdoutFile.Close()

	stderrFile, err := os.Create(stderrPath)
	if err != nil {
		return PromptExecutionResult{}, err
	}
	defer stderrFile.Close()

	if !usesPromptArg {
		promptFile, err := os.Open(promptPath)
		if err != nil {
			return PromptExecutionResult{}, err
		}
		defer promptFile.Close()
		cmd.Stdin = promptFile
	}
	if !usesPromptArg {
		cmd.Stdout = stdoutFile
		cmd.Stderr = stderrFile
	}
	cmd.Dir = workspace

	startedAt := time.Now().UTC()
	var runErr error
	if usesPromptArg {
		stdoutText, stderrText, err := captureAgyPrint(execCtx, cmd)
		if strings.TrimSpace(stdoutText) == "" {
			projectEnv := geminiProjectEnvHints(request.AccountHomePath, request.CustomEnv)
			if recovered := recoverGeminiAgyLatestMessageFn(workspace, projectEnv); recovered != "" {
				log.Printf("[gemini-agy] result stage=prompt recovery=conversation_db cwd=%q recovered_bytes=%d", workspace, len(recovered))
				stdoutText = recovered
			}
		}
		_, _ = stdoutFile.WriteString(stdoutText)
		_, _ = stderrFile.WriteString(stderrText)
		runErr = err
	} else {
		runErr = cmd.Run()
	}
	completedAt := time.Now().UTC()

	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	} else if runErr != nil {
		exitCode = 1
	}

	stdoutSummary := readTextWithLimit(stdoutPath, 4000)
	stderrSummary := readTextWithLimit(stderrPath, 4000)
	outputMarkdown := readTextWithLimit(outputPath, 12000)
	if resolvedProvider == string(ProviderKeyGemini) {
		log.Printf("[gemini-agy] result stage=prompt status=%s cwd=%q stdout_bytes=%d stderr=%q exit_code=%d err=%v", map[bool]string{true: "error", false: "ok"}[runErr != nil], workspace, len(stdoutSummary), limitLogText(stderrSummary, 1000), exitCode, runErr)
	}
	if strings.TrimSpace(outputMarkdown) == "" && strings.TrimSpace(stdoutSummary) != "" {
		outputMarkdown = stdoutSummary
		_ = os.WriteFile(outputPath, []byte(outputMarkdown), 0o644)
	}
	status := "success"
	errorMessage := ""
	var modelName *string
	if strings.TrimSpace(request.ModelName) != "" {
		name := strings.TrimSpace(request.ModelName)
		modelName = &name
	}

	if runErr != nil || exitCode != 0 {
		status = "failed"
		errorMessage = summarizeCommandFailure(runErr, stderrSummary)
	}

	result := PromptExecutionResult{
		Status:         status,
		RunID:          runID,
		ProviderKey:    resolvedProvider,
		ModelName:      modelName,
		Command:        command,
		StdoutSummary:  stdoutSummary,
		StderrSummary:  stderrSummary,
		OutputMarkdown: outputMarkdown,
		ArtifactPaths: []string{
			promptPath,
			stdoutPath,
			stderrPath,
			outputPath,
			commandPath,
			metadataPath,
		},
		StartedAt:        startedAt.Format(time.RFC3339Nano),
		CompletedAt:      completedAt.Format(time.RFC3339Nano),
		ExitCode:         exitCode,
		ErrorMessage:     errorMessage,
		ActualPromptText: actualPrompt,
	}
	applyRequiredMcpFailureStatus(&result, request.RequiredMcps)

	if artifact, err := r.SavePromptArtifact(request, result); err == nil {
		result.ArtifactPaths = []string{
			artifact.ManifestPath,
			artifact.ContentPath,
			artifact.PromptPath,
			artifact.StdoutPath,
			artifact.StderrPath,
			artifact.CommandPath,
		}
	}

	if metadataBytes, err := json.MarshalIndent(result, "", "  "); err == nil {
		_ = os.WriteFile(metadataPath, metadataBytes, 0o644)
	}

	return result, nil
}

func (r *Runner) injectSkillContent(workspace string, prompt string, skillIds []string) string {
	if len(skillIds) == 0 {
		return prompt
	}

	skills, err := r.ListSkills()
	if err != nil || len(skills) == 0 {
		return prompt
	}

	var injected []string
	injected = append(injected, prompt)
	injected = append(injected, "\n\n## Included Skills\n\nThe following skills are provided as reference to help you complete your task:\n")

	for _, reqSkill := range skillIds {
		for _, skill := range skills {
			if skill.ID == reqSkill {
				contentBytes, err := os.ReadFile(skill.FilePath)
				if err == nil {
					injected = append(injected, fmt.Sprintf("\n### Skill: %s\n```markdown\n%s\n```\n", skill.Name, string(contentBytes)))
				}
				break
			}
		}
	}

	return strings.Join(injected, "")
}

func (r *Runner) injectFeatureHistory(workspace string, prompt string) string {
	// One-shot prompt execution has no prior conversation to fall back on.
	return injectFeatureHistoryPrompt(workspace, prompt, nil)
}

// injectSelectedSkills prepends a compact skill reference block to the prompt so the model
// reads process constraints BEFORE forming a plan for the user's request. Each skill is
// represented as a file path pointer (+ one-line description from frontmatter) rather than
// the full file content — the model uses its Read tool to load whichever skill it needs,
// keeping the injected token count small regardless of skill file size.
func (r *Runner) injectSelectedSkills(workspace string, prompt string, selections []SkillSelection) string {
	if len(selections) == 0 {
		return prompt
	}
	lines := make([]string, 0, len(selections))
	for _, sel := range selections {
		path, name := r.resolveSkillPath(workspace, sel)
		if name == "" {
			continue
		}
		entry := "- /" + name
		if path != "" {
			entry += " → " + path
			if desc := readSkillFrontmatterDescription(path); desc != "" {
				entry += "\n  > " + desc
			}
		}
		lines = append(lines, entry)
	}
	if len(lines) == 0 {
		return prompt
	}
	header := "## Selected Skills\n\n" +
		"Read each skill file listed below and follow its process before responding.\n\n" +
		strings.Join(lines, "\n")
	return header + "\n\n---\n\n" + prompt
}

// resolveSkillPath returns the resolved absolute file path and display name for a
// SkillSelection. It prefers the explicit Path the desktop picker captured; falls back
// to discovering the skill by id/name under the run workspace.
func (r *Runner) resolveSkillPath(workspace string, sel SkillSelection) (path string, name string) {
	name = strings.TrimSpace(sel.Name)
	if p := strings.TrimSpace(sel.Path); p != "" {
		if _, err := os.Stat(p); err == nil {
			if name == "" {
				name = strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
			}
			return p, name
		}
	}
	for _, sk := range r.listSkillsInWorkspace(workspace) {
		if sk.ID == sel.Name || strings.EqualFold(sk.Name, sel.Name) {
			if name == "" {
				name = sk.Name
			}
			return sk.FilePath, name
		}
	}
	return "", name
}

// readSkillFrontmatterDescription extracts the description field from a skill file's YAML
// frontmatter by reading only the first 512 bytes — enough for any realistic frontmatter block.
func readSkillFrontmatterDescription(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	buf := make([]byte, 512)
	n, _ := f.Read(buf)
	for _, line := range strings.Split(string(buf[:n]), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "description:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "description:"))
		}
	}
	return ""
}

// listSkillsInWorkspace discovers skills under the given run workspace (the run's cwd), not
// the runner's default workspace, returning absolute file paths so the content is readable
// regardless of the process cwd. The path-less fallback for injectSelectedSkills.
func (r *Runner) listSkillsInWorkspace(workspace string) []Skill {
	base := strings.TrimSpace(workspace)
	if base == "" {
		base = r.workspace
	}
	skillsDir := filepath.Join(base, ".agents", "skills")
	entries, err := discoverMarkdownEntries(skillsDir, true)
	if err != nil {
		return nil
	}
	skills := make([]Skill, 0, len(entries))
	for _, entry := range entries {
		skills = append(skills, Skill{
			ID:       entry.ID,
			Name:     entry.Name,
			FilePath: filepath.Join(skillsDir, filepath.FromSlash(entry.FilePath)),
		})
	}
	return skills
}

func summarizeCommandFailure(runErr error, stderrSummary string) string {
	errorMessage := ""
	if runErr != nil {
		errorMessage = strings.TrimSpace(runErr.Error())
	}

	stderrSummary = strings.TrimSpace(stderrSummary)
	if stderrSummary == "" {
		return errorMessage
	}

	if errorMessage == "" || strings.HasPrefix(errorMessage, "exit status ") {
		return stderrSummary
	}

	return errorMessage
}

func (r *Runner) RunMcpTest(ctx context.Context, request McpTestRequest) (McpTestResult, error) {
	// Handle provider-driven tests
	if request.UseProviderCLI {
		return r.runProviderDrivenMcpTest(ctx, request)
	}

	// Handle standard backend tests
	if strings.TrimSpace(request.BackendKey) == "" {
		return McpTestResult{}, errors.New("backendKey is required")
	}
	if strings.TrimSpace(request.ProviderType) == "" {
		return McpTestResult{}, errors.New("providerType is required")
	}
	if strings.TrimSpace(request.ProjectID) == "" {
		return McpTestResult{}, errors.New("projectId is required")
	}
	if strings.TrimSpace(request.IntegrationID) == "" {
		return McpTestResult{}, errors.New("integrationId is required")
	}
	if strings.TrimSpace(request.Prompt) == "" {
		return McpTestResult{}, errors.New("prompt is required")
	}

	spec, ok := lookupMcpBackendSpec(strings.TrimSpace(request.BackendKey))
	if !ok {
		return McpTestResult{}, fmt.Errorf("unsupported MCP backend %q", request.BackendKey)
	}
	if spec.ProviderType != strings.TrimSpace(request.ProviderType) {
		return McpTestResult{}, fmt.Errorf(
			"providerType %q does not match backend %q",
			request.ProviderType,
			request.BackendKey,
		)
	}

	timeout := 30 * time.Second
	if request.TimeoutMs > 0 {
		timeout = time.Duration(request.TimeoutMs) * time.Millisecond
	}

	runID := newMcpTestRunID()
	runDir := filepath.Join(r.workspace, ".flowpilot", "mcp-tests", runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return McpTestResult{}, err
	}

	requestPath := filepath.Join(runDir, "request.json")
	resultPath := filepath.Join(runDir, "result.json")
	stdoutPath := filepath.Join(runDir, "stdout.txt")
	stderrPath := filepath.Join(runDir, "stderr.txt")
	outputPath := filepath.Join(runDir, "output.md")

	if err := writeJSONFile(requestPath, request); err != nil {
		return McpTestResult{}, err
	}

	startedAt := time.Now().UTC()
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result := McpTestResult{
		Status:        "failed",
		RunID:         runID,
		BackendKey:    strings.TrimSpace(request.BackendKey),
		ProviderType:  strings.TrimSpace(request.ProviderType),
		ProjectID:     strings.TrimSpace(request.ProjectID),
		IntegrationID: strings.TrimSpace(request.IntegrationID),
		ArtifactPaths: []string{requestPath, resultPath, stdoutPath, stderrPath, outputPath},
		StartedAt:     startedAt.Format(time.RFC3339Nano),
	}

	stdoutSummary, stderrSummary, outputMarkdown, command, runErr := r.executeMcpTest(runCtx, request)
	completedAt := time.Now().UTC()

	result.Command = command
	result.StdoutSummary = stdoutSummary
	result.StderrSummary = stderrSummary
	result.OutputMarkdown = outputMarkdown
	result.CompletedAt = completedAt.Format(time.RFC3339Nano)

	if stdoutSummary != "" {
		if err := os.WriteFile(stdoutPath, []byte(stdoutSummary), 0o644); err != nil {
			return McpTestResult{}, err
		}
	} else if err := os.WriteFile(stdoutPath, []byte(""), 0o644); err != nil {
		return McpTestResult{}, err
	}

	if stderrSummary != "" {
		if err := os.WriteFile(stderrPath, []byte(stderrSummary), 0o644); err != nil {
			return McpTestResult{}, err
		}
	} else if err := os.WriteFile(stderrPath, []byte(""), 0o644); err != nil {
		return McpTestResult{}, err
	}

	if outputMarkdown != "" {
		if err := os.WriteFile(outputPath, []byte(outputMarkdown), 0o644); err != nil {
			return McpTestResult{}, err
		}
	} else if err := os.WriteFile(outputPath, []byte(""), 0o644); err != nil {
		return McpTestResult{}, err
	}

	if runErr != nil {
		result.ErrorMessage = runErr.Error()
	} else {
		result.Status = "success"
	}

	if err := writeJSONFile(resultPath, result); err != nil {
		return McpTestResult{}, err
	}

	return result, nil
}

func (r *Runner) ListMcpTestRuns(
	ctx context.Context,
	backendKey string,
	projectID string,
	integrationID string,
	limit int,
) ([]McpTestRunSummary, error) {
	_ = ctx

	if limit <= 0 {
		limit = 10
	}

	baseDir := filepath.Join(r.workspace, ".flowpilot", "mcp-tests")
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []McpTestRunSummary{}, nil
		}
		return nil, err
	}

	filteredBackendKey := strings.TrimSpace(backendKey)
	filteredProjectID := strings.TrimSpace(projectID)
	filteredIntegrationID := strings.TrimSpace(integrationID)
	summaries := make([]McpTestRunSummary, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		runDir := filepath.Join(baseDir, entry.Name())
		resultPath := filepath.Join(runDir, "result.json")
		var result McpTestResult
		if err := readJSONFile(resultPath, &result); err != nil {
			continue
		}

		if filteredBackendKey != "" && result.BackendKey != filteredBackendKey {
			continue
		}
		if filteredProjectID != "" && result.ProjectID != filteredProjectID {
			continue
		}
		if filteredIntegrationID != "" && result.IntegrationID != filteredIntegrationID {
			continue
		}

		summaries = append(summaries, McpTestRunSummary{
			RunID:         result.RunID,
			BackendKey:    result.BackendKey,
			ProviderType:  result.ProviderType,
			ProjectID:     result.ProjectID,
			IntegrationID: result.IntegrationID,
			Status:        result.Status,
			StartedAt:     result.StartedAt,
			CompletedAt:   result.CompletedAt,
			ArtifactDir:   runDir,
		})
	}

	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].RunID > summaries[j].RunID
	})

	if len(summaries) > limit {
		summaries = summaries[:limit]
	}

	return summaries, nil
}

type providerSpec struct {
	Key         string
	Label       string
	BinaryName  string
	InstallHint string
	Models      []ProviderModel
}

type codexDebugModelsPayload struct {
	Models []codexDebugModel `json:"models"`
}

type codexDebugModel struct {
	Slug           string `json:"slug"`
	DisplayName    string `json:"display_name"`
	Visibility     string `json:"visibility"`
	SupportedInAPI bool   `json:"supported_in_api"`
	// DefaultReasoningLevel/SupportedReasoningLevels/ContextWindow/
	// MaxContextWindow (Task-215) are live-verified fields `codex debug
	// models` already returns per model (Codex Build 0.144.1: gpt-5.6-sol
	// carries supported_reasoning_levels up to max/ultra and
	// context_window/max_context_window/effective_context_window_percent) —
	// previously silently dropped because this struct didn't type them.
	DefaultReasoningLevel    string                     `json:"default_reasoning_level"`
	SupportedReasoningLevels []codexDebugReasoningLevel `json:"supported_reasoning_levels"`
	ContextWindow            int64                      `json:"context_window"`
	MaxContextWindow         int64                      `json:"max_context_window"`
}

type codexDebugReasoningLevel struct {
	Effort      string `json:"effort"`
	Description string `json:"description"`
}

type mcpBackendSpec struct {
	Key          string
	ProviderType string
	Label        string
	Transport    string
	Launcher     string
	InstallArgs  []string
	VerifyArgs   []string
	InstallHint  string
}

type markdownEntry struct {
	ID          string
	Name        string
	FilePath    string
	Description string
	Tags        []string
	Steps       []string
}

func providerSpecs() []providerSpec {
	return []providerSpec{
		{
			Key:         "codex",
			Label:       "Codex",
			BinaryName:  "codex",
			InstallHint: "Install the Codex CLI, log in, and restart the runner.",
			Models: []ProviderModel{
				{ID: "gpt-5.5", DisplayName: "gpt-5.5", Source: "registry"},
				{ID: "gpt-5.4", DisplayName: "gpt-5.4", Source: "registry"},
				{ID: "gpt-5.4-mini", DisplayName: "gpt-5.4-mini", Source: "registry"},
			},
		},
		{
			Key:         "claude",
			Label:       "Claude Code",
			BinaryName:  "claude",
			InstallHint: "Install the Claude Code CLI, log in, and restart the runner.",
			Models: []ProviderModel{
				{ID: "claude-opus", DisplayName: "claude-opus", Source: "registry"},
				{ID: "claude-sonnet", DisplayName: "claude-sonnet", Source: "registry"},
				{ID: "claude-haiku", DisplayName: "claude-haiku", Source: "registry"},
			},
		},
		{
			Key:         "gemini",
			Label:       "Antigravity CLI",
			BinaryName:  "agy",
			InstallHint: "Install Antigravity CLI, start agy to sign in, and refresh the runner inventory.",
			Models:      defaultGeminiProviderModels(),
		},
		{
			// Appended last (CP-46 P-0/Task-210 T-1): codex/claude/gemini specs above
			// unchanged. BinaryName is overridable via FLOWPILOT_GROK_BIN
			// (grokBinaryName, grok_process.go) for machines where an unrelated
			// third-party `grok` tool shadows the real xAI Grok Build CLI on PATH
			// (observed live during Task-206 authoring).
			Key:         "grok",
			Label:       "Grok",
			BinaryName:  "grok",
			InstallHint: "Install Grok Build (irm https://x.ai/cli/install.ps1 | iex on Windows, curl -fsSL https://x.ai/cli/install.sh | sh on mac/linux), log in, and restart the runner.",
			Models:      defaultGrokProviderModels(),
		},
	}
}

func defaultGrokProviderModels() []ProviderModel {
	return []ProviderModel{
		{ID: "grok-4.5", DisplayName: "Grok 4.5", Source: "registry"},
		{ID: "grok-build", DisplayName: "Grok Build", Source: "registry"},
	}
}

func defaultGeminiProviderModels() []ProviderModel {
	return []ProviderModel{
		{ID: "gemini-3.5-flash-medium", DisplayName: "Gemini 3.5 Flash (Medium)", Source: "registry"},
		{ID: "gemini-3.5-flash-high", DisplayName: "Gemini 3.5 Flash (High)", Source: "registry"},
		{ID: "gemini-3.5-flash-low", DisplayName: "Gemini 3.5 Flash (Low)", Source: "registry"},
		{ID: "gemini-3.1-pro-low", DisplayName: "Gemini 3.1 Pro (Low)", Source: "registry"},
		{ID: "gemini-3.1-pro-high", DisplayName: "Gemini 3.1 Pro (High)", Source: "registry"},
	}
}

func lookupProviderSpec(key string) (providerSpec, bool) {
	normalized := strings.ToLower(strings.TrimSpace(key))
	for _, spec := range providerSpecs() {
		if spec.Key == normalized {
			return spec, true
		}
	}

	return providerSpec{}, false
}

func detectProvider(ctx context.Context, spec providerSpec) Provider {
	provider := Provider{
		ID:            strings.ToUpper(spec.Key),
		Key:           spec.Key,
		Label:         spec.Label,
		Supported:     true,
		AuthStatus:    "UNKNOWN",
		InstallStatus: "NOT_INSTALLED",
		InstallHint:   spec.InstallHint,
		Models:        buildProviderModels(spec.Models, false),
	}

	if accounts, err := DiscoverProviderAccounts(spec.Key); err == nil && len(accounts) > 0 {
		provider.Accounts = accounts
	}

	binaryPath, err := lookPathFn(spec.BinaryName)
	if err != nil {
		notFound := fmt.Sprintf("%s binary was not found on PATH", spec.Label)
		provider.LastError = &notFound
		return provider
	}

	version := resolveVersion(ctx, binaryPath)
	if strings.TrimSpace(version) == "" {
		provider.Installed = true
		provider.BinaryPath = binaryPath
		provider.DetectedBinary = spec.BinaryName
		provider.InstallStatus = "FAILED"
		provider.Version = ""
		provider.DetectedVersion = ""
		versionError := fmt.Sprintf("%s version check failed", spec.Label)
		provider.LastError = &versionError
		return provider
	}

	provider.Installed = true
	provider.InstallStatus = "INSTALLED"
	provider.BinaryPath = binaryPath
	provider.DetectedBinary = spec.BinaryName
	provider.Version = version
	provider.DetectedVersion = version
	provider.AuthStatus = providerAuthStatus(spec)
	provider.Models = buildProviderModels(resolveProviderModels(ctx, spec, binaryPath), provider.AuthStatus == "READY")
	if provider.AuthStatus == "AUTH_REQUIRED" {
		authError := fmt.Sprintf("%s authentication is required", spec.Label)
		provider.LastError = &authError
	}

	return provider
}

func buildProviderModels(source []ProviderModel, available bool) []ProviderModel {
	models := make([]ProviderModel, 0, len(source))
	for _, model := range source {
		model.Available = available
		models = append(models, model)
	}

	return models
}

func resolveProviderModels(ctx context.Context, spec providerSpec, binaryPath string) []ProviderModel {
	if spec.Key == "codex" {
		if models, err := detectCodexModels(ctx, binaryPath); err == nil && len(models) > 0 {
			return models
		}
	}
	if spec.Key == "gemini" {
		if models, err := detectGeminiModels(ctx, binaryPath, spec.Models); err == nil && len(models) > 0 {
			return models
		}
	}
	if spec.Key == "grok" {
		// Appended last (CP-46 P-0/Task-213 T-2): codex/gemini branches above
		// unchanged.
		if models, err := detectGrokModels(); err == nil && len(models) > 0 {
			return models
		}
	}

	return spec.Models
}

// grokModelsCacheEntry mirrors one value in ~/.grok/models_cache.json's
// "models" map (live-verified shape, Grok Build 0.2.93, Task-213 authoring;
// SupportsReasoningEffort/ReasoningEffort/ReasoningEfforts/ContextWindow
// added Task-215 — live-verified present on the same entries but previously
// untyped/dropped).
type grokModelsCacheEntry struct {
	Info struct {
		ID                      string                       `json:"id"`
		Name                    string                       `json:"name"`
		Hidden                  bool                         `json:"hidden"`
		SupportedInAPI          bool                         `json:"supported_in_api"`
		ContextWindow           int64                        `json:"context_window"`
		SupportsReasoningEffort bool                         `json:"supports_reasoning_effort"`
		ReasoningEffort         string                       `json:"reasoning_effort"`
		ReasoningEfforts        []grokModelsCacheEffortEntry `json:"reasoning_efforts"`
	} `json:"info"`
}

type grokModelsCacheEffortEntry struct {
	ID      string `json:"id"`
	Value   string `json:"value"`
	Label   string `json:"label"`
	Default bool   `json:"default"`
}

type grokModelsCachePayload struct {
	Models map[string]grokModelsCacheEntry `json:"models"`
}

// grokModelsCachePath resolves ~/.grok/models_cache.json (or $GROK_HOME/
// models_cache.json when set), the file the real Grok Build CLI itself writes
// after every successful model-list fetch (verified live: fetched_at/
// grok_version/origin/etag + the models map). Detection reads this cached
// snapshot rather than spawning the CLI, so it works offline and never
// triggers the ambient-MCP-scan hazard (CP-46 R-1).
func grokModelsCachePath() string {
	if grokHome := strings.TrimSpace(os.Getenv("GROK_HOME")); grokHome != "" {
		return filepath.Join(grokHome, "models_cache.json")
	}
	if home := preferredUserHomeDir(); home != "" {
		return filepath.Join(home, ".grok", "models_cache.json")
	}
	return ""
}

// detectGrokModels reads the live model catalog Grok Build itself cached
// (Task-213 T-5), filtering out hidden/unsupported entries. Returns an error
// (never a partial/fabricated list) when the cache is missing or unreadable
// so resolveProviderModels falls back to the static default list, exactly
// like detectCodexModels/detectGeminiModels degrade on failure.
func detectGrokModels() ([]ProviderModel, error) {
	path := grokModelsCachePath()
	if path == "" {
		return nil, fmt.Errorf("grok models cache: unable to resolve home directory")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var payload grokModelsCachePayload
	if err := json.Unmarshal(stripUTF8BOM(raw), &payload); err != nil {
		return nil, err
	}

	models := make([]ProviderModel, 0, len(payload.Models))
	for id, entry := range payload.Models {
		if entry.Info.Hidden || !entry.Info.SupportedInAPI {
			continue
		}
		modelID := strings.TrimSpace(entry.Info.ID)
		if modelID == "" {
			modelID = strings.TrimSpace(id)
		}
		if modelID == "" {
			continue
		}
		displayName := strings.TrimSpace(entry.Info.Name)
		if displayName == "" {
			displayName = modelID
		}

		model := ProviderModel{
			ID:                  modelID,
			DisplayName:         displayName,
			Source:              "grok_models_cache",
			ContextWindowTokens: entry.Info.ContextWindow,
		}
		if entry.Info.SupportsReasoningEffort {
			efforts := make([]string, 0, len(entry.Info.ReasoningEfforts))
			for _, level := range entry.Info.ReasoningEfforts {
				effort := strings.ToLower(strings.TrimSpace(level.Value))
				if effort != "" {
					efforts = append(efforts, effort)
				}
			}
			model.SupportedReasoningEfforts = efforts
			model.DefaultReasoningEffort = strings.ToLower(strings.TrimSpace(entry.Info.ReasoningEffort))
		}
		models = append(models, model)
	}
	sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })

	if len(models) == 0 {
		return nil, fmt.Errorf("grok models cache: no supported models found in %s", path)
	}
	return models, nil
}

func detectCodexModels(ctx context.Context, binaryPath string) ([]ProviderModel, error) {
	output, err := runCommandFn(ctx, binaryPath, "debug", "models")
	if err != nil {
		return nil, err
	}

	var payload codexDebugModelsPayload
	if err := json.Unmarshal(output, &payload); err != nil {
		return nil, err
	}

	models := make([]ProviderModel, 0, len(payload.Models))
	for _, model := range payload.Models {
		if strings.TrimSpace(model.Slug) == "" {
			continue
		}
		if model.Visibility != "list" || !model.SupportedInAPI {
			continue
		}

		displayName := strings.TrimSpace(model.DisplayName)
		if displayName == "" {
			displayName = model.Slug
		}

		efforts := make([]string, 0, len(model.SupportedReasoningLevels))
		for _, level := range model.SupportedReasoningLevels {
			effort := strings.ToLower(strings.TrimSpace(level.Effort))
			if effort != "" {
				efforts = append(efforts, effort)
			}
		}

		models = append(models, ProviderModel{
			ID:                        model.Slug,
			DisplayName:               displayName,
			Source:                    "codex_debug_models",
			SupportedReasoningEfforts: efforts,
			DefaultReasoningEffort:    strings.ToLower(strings.TrimSpace(model.DefaultReasoningLevel)),
			ContextWindowTokens:       model.ContextWindow,
			MaxContextWindowTokens:    model.MaxContextWindow,
		})
	}

	return models, nil
}

func detectGeminiModels(ctx context.Context, binaryPath string, fallback []ProviderModel) ([]ProviderModel, error) {
	output, err := runCommandFn(ctx, binaryPath, "models")
	if err != nil {
		return nil, err
	}

	return parseGeminiModelsFromAGYOutput(string(output), fallback)
}

func parseGeminiModelsFromAGYOutput(raw string, fallback []ProviderModel) ([]ProviderModel, error) {
	if len(fallback) == 0 {
		fallback = defaultGeminiProviderModels()
	}

	byID := make(map[string]ProviderModel, len(fallback))
	byDisplay := make(map[string]ProviderModel, len(fallback))
	for _, model := range fallback {
		byID[strings.ToLower(model.ID)] = model
		byDisplay[strings.ToLower(model.DisplayName)] = model
	}

	models := make([]ProviderModel, 0, len(fallback))
	seen := make(map[string]struct{}, len(fallback))
	for _, line := range strings.Split(raw, "\n") {
		normalized := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(line), "(current)"))
		if normalized == "" {
			continue
		}
		model, ok := byDisplay[strings.ToLower(normalized)]
		if !ok {
			model, ok = byID[strings.ToLower(normalized)]
		}
		if !ok {
			continue
		}
		if _, exists := seen[model.ID]; exists {
			continue
		}
		model.Source = "agy_models_command"
		models = append(models, model)
		seen[model.ID] = struct{}{}
	}

	if len(models) == 0 {
		return nil, errors.New("no supported gemini models found in agy output")
	}
	return models, nil
}

func getPossibleHomeDirs() []string {
	dirs := make([]string, 0, 4)
	appendUniqueHomeDir := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}

		cleanValue := canonicalPathKey(value)
		for _, existing := range dirs {
			if canonicalPathKey(existing) == cleanValue {
				return
			}
		}

		dirs = append(dirs, filepath.Clean(value))
	}

	appendUniqueHomeDir(preferredUserHomeDir())
	appendUniqueHomeDir(os.Getenv("USERPROFILE"))
	appendUniqueHomeDir(os.Getenv("HOME"))
	appendUniqueHomeDir(os.Getenv("APPDATA"))
	if home, err := os.UserHomeDir(); err == nil {
		appendUniqueHomeDir(home)
	}

	return dirs
}

func preferredUserHomeDir() string {
	candidates := make([]string, 0, 4)
	if runtime.GOOS == "windows" {
		candidates = append(candidates, os.Getenv("USERPROFILE"))
		if drive := strings.TrimSpace(os.Getenv("HOMEDRIVE")); drive != "" {
			if path := strings.TrimSpace(os.Getenv("HOMEPATH")); path != "" {
				candidates = append(candidates, drive+path)
			}
		}
		candidates = append(candidates, os.Getenv("HOME"))
	} else {
		candidates = append(candidates, os.Getenv("HOME"))
		candidates = append(candidates, os.Getenv("USERPROFILE"))
	}

	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate != "" {
			return filepath.Clean(candidate)
		}
	}

	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		return filepath.Clean(home)
	}

	return ""
}

type RunnerInstanceContext struct {
	MachineFingerprint string `json:"machineFingerprint"`
	HostName           string `json:"hostName"`
	OSName             string `json:"osName"`
}

func CurrentRunnerInstanceContext() RunnerInstanceContext {
	hostName, err := os.Hostname()
	if err != nil || strings.TrimSpace(hostName) == "" {
		hostName = "unknown-host"
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = ""
	}

	fingerprintSeed := strings.Join(
		[]string{runtime.GOOS, strings.TrimSpace(hostName), filepath.Clean(homeDir)},
		"|",
	)
	hash := sha256.Sum256([]byte(fingerprintSeed))

	return RunnerInstanceContext{
		MachineFingerprint: hex.EncodeToString(hash[:16]),
		HostName:           hostName,
		OSName:             runtime.GOOS,
	}
}

type authCandidate struct {
	homePath string
	authPath string
}

func defaultAuthCandidates(providerKey, dir string) []authCandidate {
	switch providerKey {
	case "codex":
		return []authCandidate{
			{homePath: filepath.Join(dir, ".codex"), authPath: filepath.Join(dir, ".codex", "auth.json")},
			{homePath: filepath.Join(dir, "codex"), authPath: filepath.Join(dir, "codex", "auth.json")},
		}
	case "claude":
		return []authCandidate{
			{homePath: dir, authPath: filepath.Join(dir, ".claude", ".credentials.json")},
			{homePath: dir, authPath: filepath.Join(dir, ".claude.json")},
			{homePath: dir, authPath: filepath.Join(dir, "claude", "auth.json")},
			{homePath: dir, authPath: filepath.Join(dir, ".config", "claude", "auth.json")},
		}
	case "gemini":
		return []authCandidate{
			{homePath: dir, authPath: filepath.Join(dir, ".gemini", "oauth_creds.json")},
			{homePath: dir, authPath: filepath.Join(dir, "gemini", "oauth_creds.json")},
		}
	case "grok":
		// Appended last (CP-46 P-0/Task-210 T-5): other cases above unchanged.
		// Live-verified: ~/.grok/auth.json (map keyed by "issuer::userId").
		return []authCandidate{
			{homePath: filepath.Join(dir, ".grok"), authPath: filepath.Join(dir, ".grok", "auth.json")},
			{homePath: filepath.Join(dir, "grok"), authPath: filepath.Join(dir, "grok", "auth.json")},
		}
	default:
		return nil
	}
}

func accountAuthPaths(providerKey, homePath string) []string {
	switch providerKey {
	case "codex":
		return []string{
			filepath.Join(homePath, ".codex", "auth.json"),
			filepath.Join(homePath, "codex", "auth.json"),
			filepath.Join(homePath, "auth.json"),
		}
	case "claude":
		return []string{
			filepath.Join(homePath, ".claude", ".credentials.json"),
			filepath.Join(homePath, ".claude.json"),
			filepath.Join(homePath, "claude", "auth.json"),
			filepath.Join(homePath, ".config", "claude", "auth.json"),
		}
	case "gemini":
		return []string{
			filepath.Join(homePath, ".gemini", "oauth_creds.json"),
			filepath.Join(homePath, "gemini", "oauth_creds.json"),
		}
	case "grok":
		// Appended last (CP-46 P-0/Task-210 T-5). A managed grok GROK_HOME's
		// auth.json lives directly at its root (mirrors ~/.grok/auth.json).
		return []string{
			filepath.Join(homePath, "auth.json"),
			filepath.Join(homePath, ".grok", "auth.json"),
		}
	default:
		return nil
	}
}

func hasValidProviderAuthFile(providerKey, path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.Size() <= 0 {
		return false
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}

	content := string(data)
	switch providerKey {
	case "codex":
		return strings.Contains(content, `"id_token"`) || strings.Contains(content, `"OPENAI_API_KEY"`)
	case "claude":
		return claudeAuthFileLooksValid(data)
	case "gemini":
		return strings.Contains(content, `"access_token"`) || strings.Contains(content, `"refresh_token"`)
	case "grok":
		// Appended last (CP-46 P-0/Task-210 T-5). Live-verified field names in
		// ~/.grok/auth.json entries: refresh_token/email.
		return strings.Contains(content, `"refresh_token"`) && strings.Contains(content, `"email"`)
	default:
		return false
	}
}

func claudeAuthFileLooksValid(data []byte) bool {
	var payload map[string]any
	if err := json.Unmarshal(stripUTF8BOM(data), &payload); err != nil {
		return false
	}
	if oauth, ok := payload["claudeAiOauth"].(map[string]any); ok {
		return hasNonEmptyJSONString(oauth, "accessToken") || hasNonEmptyJSONString(oauth, "refreshToken")
	}
	if tokens, ok := payload["tokens"].(map[string]any); ok {
		return hasNonEmptyJSONString(tokens, "access_token") || hasNonEmptyJSONString(tokens, "refresh_token")
	}
	return hasNonEmptyJSONString(payload, "accessToken") || hasNonEmptyJSONString(payload, "refreshToken")
}

func DetectDefaultAccountHomePath(providerKey string) (string, bool) {
	for _, dir := range getPossibleHomeDirs() {
		for _, candidate := range defaultAuthCandidates(providerKey, dir) {
			if hasValidProviderAuthFile(providerKey, candidate.authPath) {
				return candidate.homePath, true
			}
		}
	}

	return "", false
}

func hasLocalAuth(providerKey string) bool {
	_, ok := DetectDefaultAccountHomePath(providerKey)
	return ok
}

func hasGeminiAntigravityConfig() bool {
	for _, dir := range getPossibleHomeDirs() {
		for _, candidate := range []string{
			filepath.Join(dir, ".gemini", "antigravity-cli", "settings.json"),
			filepath.Join(dir, ".gemini", "antigravity-cli", "keybindings.json"),
		} {
			info, err := os.Stat(candidate)
			if err == nil && !info.IsDir() && info.Size() > 0 {
				return true
			}
		}
	}

	return false
}

func providerAuthStatus(spec providerSpec) string {
	switch spec.Key {
	case "codex":
		if hasAnyEnv("OPENAI_API_KEY", "OPENAI_API_BASE") || hasLocalAuth("codex") {
			return "READY"
		}
	case "claude":
		if hasAnyEnv("ANTHROPIC_API_KEY") || hasLocalAuth("claude") {
			return "READY"
		}
	case "gemini":
		if hasAnyEnv("GOOGLE_API_KEY", "GEMINI_API_KEY") || hasLocalAuth("gemini") || hasGeminiAntigravityConfig() {
			return "READY"
		}
	case "grok":
		if hasAnyEnv("XAI_API_KEY") || hasLocalAuth("grok") {
			return "READY"
		}
	}

	return "AUTH_REQUIRED"
}

func hasAnyEnv(keys ...string) bool {
	for _, key := range keys {
		if strings.TrimSpace(os.Getenv(key)) != "" {
			return true
		}
	}

	return false
}

func runProviderInstallCommand(ctx context.Context, spec providerSpec) error {
	command, args, err := providerInstallCommand(spec)
	if err != nil {
		return err
	}

	output, runErr := runCommandFn(ctx, command, args...)
	if runErr != nil {
		message := strings.TrimSpace(string(output))
		if message != "" {
			return fmt.Errorf("%s: %w", message, runErr)
		}
		return runErr
	}

	return nil
}

func providerInstallCommand(spec providerSpec) (string, []string, error) {
	switch spec.Key {
	case "claude":
		switch runtime.GOOS {
		case "darwin", "linux":
			return "sh", []string{"-c", "curl -fsSL https://claude.ai/install.sh | bash"}, nil
		case "windows":
			return "powershell", []string{"-NoProfile", "-Command", "irm https://claude.ai/install.ps1 | iex"}, nil
		default:
			return "", nil, fmt.Errorf("%s install is not supported on %s", spec.Label, runtime.GOOS)
		}
	case "codex":
		return "npm", []string{"install", "-g", "@openai/codex"}, nil
	case "gemini":
		switch runtime.GOOS {
		case "darwin", "linux":
			return "sh", []string{"-c", "curl -fsSL https://antigravity.google/cli/install.sh | bash"}, nil
		case "windows":
			return "cmd", []string{"/c", "curl -fsSL https://antigravity.google/cli/install.cmd -o install.cmd && install.cmd && del install.cmd"}, nil
		default:
			return "", nil, fmt.Errorf("%s install is not supported on %s", spec.Label, runtime.GOOS)
		}
	case "grok":
		switch runtime.GOOS {
		case "darwin", "linux":
			return "sh", []string{"-c", "curl -fsSL https://x.ai/cli/install.sh | sh"}, nil
		case "windows":
			return "powershell", []string{"-NoProfile", "-Command", "irm https://x.ai/cli/install.ps1 | iex"}, nil
		default:
			return "", nil, fmt.Errorf("%s install is not supported on %s", spec.Label, runtime.GOOS)
		}
	default:
		return "", nil, fmt.Errorf("unsupported provider %q", spec.Key)
	}
}

func findProvider(providers []Provider, key string) *Provider {
	normalized := strings.ToLower(strings.TrimSpace(key))
	for idx := range providers {
		if providers[idx].Key == normalized {
			return &providers[idx]
		}
	}

	return nil
}

func annotateProviderInventory(inventory *ProviderInventory, key string, status string, lastError *string, installed bool, authStatus string) {
	provider := findProvider(inventory.Providers, key)
	if provider == nil {
		return
	}

	models := provider.Models
	provider.InstallStatus = status
	provider.Installed = installed
	provider.AuthStatus = authStatus
	provider.Models = buildProviderModels(models, status == "INSTALLED" && authStatus == "READY")
	provider.LastError = lastError
}

func ResolveWorkspace(workspace string) (string, error) {
	if workspace != "" {
		return filepath.Abs(workspace)
	}

	if envWorkspace := os.Getenv("FLOWPILOT_WORKSPACE"); envWorkspace != "" {
		return filepath.Abs(envWorkspace)
	}

	current, err := os.Getwd()
	if err != nil {
		return "", err
	}

	resolved, err := resolveByWalkingUp(current)
	if err != nil {
		return filepath.Abs(current)
	}

	return resolved, nil
}

func resolveByWalkingUp(start string) (string, error) {
	current := start

	for {
		agentsPath := filepath.Join(current, ".agents")
		if info, err := os.Stat(agentsPath); err == nil && info.IsDir() {
			return filepath.Abs(current)
		}

		parent := filepath.Dir(current)
		if parent == current {
			return "", errors.New("unable to resolve workspace root")
		}
		current = parent
	}
}

func resolveVersion(ctx context.Context, binaryPath string) string {
	command := exec.CommandContext(ctx, binaryPath, "--version")
	output, err := command.CombinedOutput()
	if err != nil && len(output) == 0 {
		return ""
	}

	version := strings.TrimSpace(string(output))
	if version == "" && err != nil {
		return ""
	}

	return version
}

func mcpBackendSpecs() []mcpBackendSpec {
	return []mcpBackendSpec{
		{
			Key:          "google_drive",
			ProviderType: "google_drive",
			Label:        "Google Drive MCP",
			Transport:    "launcher",
			Launcher:     "npx",
			InstallArgs:  []string{"-y", "@piotr-agier/google-drive-mcp", "--version"},
			VerifyArgs:   []string{"-y", "@piotr-agier/google-drive-mcp", "--help"},
			InstallHint:  "Install Node.js, then run `npx -y @piotr-agier/google-drive-mcp --help` and complete Google Drive OAuth.",
		},
		{
			Key:          "jira",
			ProviderType: "jira",
			Label:        "Atlassian MCP",
			Transport:    "remote",
			Launcher:     "remote",
			InstallHint:  "Open the Jira MCP form, read the Atlassian guide, and paste an API token so the runner can connect to the Atlassian remote MCP server.",
		},
	}
}

func lookupMcpBackendSpec(key string) (mcpBackendSpec, bool) {
	for _, spec := range mcpBackendSpecs() {
		if spec.Key == key {
			return spec, true
		}
	}

	return mcpBackendSpec{}, false
}

type mcpBackendRecord struct {
	Installed     bool   `json:"installed"`
	LastCheckedAt string `json:"lastCheckedAt"`
	LastError     string `json:"lastError"`
	SecretKey     string `json:"secretKey,omitempty"`
}

func (r *Runner) loadMcpBackendRecords() (map[string]mcpBackendRecord, error) {
	path := r.mcpBackendStatePath()
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]mcpBackendRecord{}, nil
		}
		return nil, err
	}

	records := map[string]mcpBackendRecord{}
	if err := json.Unmarshal(raw, &records); err != nil {
		return nil, err
	}

	return records, nil
}

func (r *Runner) saveMcpBackendRecord(backend McpBackend) error {
	records, err := r.loadMcpBackendRecords()
	if err != nil {
		return err
	}

	records[backend.Key] = mcpBackendRecord{
		Installed:     backend.Installed,
		LastCheckedAt: backend.LastCheckedAt,
		LastError:     backend.LastError,
		SecretKey:     backend.SecretKey,
	}

	payload, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}

	path := r.mcpBackendStatePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	return os.WriteFile(path, payload, 0o644)
}

func (r *Runner) mcpBackendStatePath() string {
	return filepath.Join(r.workspace, ".flowpilot", "mcp-backend-state.json")
}

type jiraCredential struct {
	ProjectID    string `json:"projectId"`
	Email        string `json:"email"`
	ApiToken     string `json:"apiToken"`
	WorkspaceURL string `json:"workspaceUrl"`
	ProjectKey   string `json:"projectKey"`
	BoardID      string `json:"boardId"`
}

func (r *Runner) ensureSecretStore() SecretStore {
	if r.secretStore == nil {
		r.secretStore = newDefaultSecretStore()
	}
	return r.secretStore
}

func jiraCredentialKey(integrationID string) string {
	return normalizeSecretKey("jira", integrationID)
}

func (r *Runner) saveJiraCredential(integrationID string, creds jiraCredential) error {
	raw, err := json.Marshal(creds)
	if err != nil {
		return err
	}

	return r.ensureSecretStore().Set(jiraCredentialKey(integrationID), string(raw))
}

func (r *Runner) loadJiraCredential(secretKey string) (jiraCredential, error) {
	if strings.TrimSpace(secretKey) == "" {
		return jiraCredential{}, errors.New("jira API token is not configured")
	}

	raw, err := r.ensureSecretStore().Get(secretKey)
	if err != nil {
		return jiraCredential{}, err
	}

	var creds jiraCredential
	if err := json.Unmarshal([]byte(raw), &creds); err != nil {
		return jiraCredential{}, err
	}

	return creds, nil
}

func (r *Runner) deleteJiraCredential(integrationID string) error {
	return r.ensureSecretStore().Delete(jiraCredentialKey(integrationID))
}

func (r *Runner) resolveJiraCredential(
	integrationID string,
	request IntegrationConnectionRequest,
) (jiraCredential, error) {
	secretKey := jiraCredentialKey(integrationID)
	email := strings.TrimSpace(request.Email)
	apiToken := strings.TrimSpace(request.ApiToken)
	workspaceURL := strings.TrimSpace(request.WorkspaceURL)
	projectKey := strings.TrimSpace(request.ProjectKey)
	boardID := strings.TrimSpace(request.BoardID)

	if email != "" && apiToken != "" {
		if workspaceURL == "" || projectKey == "" {
			return jiraCredential{}, errors.New("workspaceUrl and projectKey are required for Jira")
		}
		return jiraCredential{
			ProjectID:    strings.TrimSpace(request.ProjectID),
			Email:        email,
			ApiToken:     apiToken,
			WorkspaceURL: workspaceURL,
			ProjectKey:   projectKey,
			BoardID:      boardID,
		}, nil
	}

	creds, err := r.loadJiraCredential(secretKey)
	if err != nil {
		return jiraCredential{}, errors.New("Atlassian email and API token are required")
	}

	if workspaceURL != "" {
		creds.WorkspaceURL = workspaceURL
	}
	if projectKey != "" {
		creds.ProjectKey = projectKey
	}
	if boardID != "" {
		creds.BoardID = boardID
	}

	if strings.TrimSpace(creds.Email) == "" || strings.TrimSpace(creds.ApiToken) == "" {
		return jiraCredential{}, errors.New("Atlassian email and API token are required")
	}
	if strings.TrimSpace(creds.WorkspaceURL) == "" || strings.TrimSpace(creds.ProjectKey) == "" {
		return jiraCredential{}, errors.New("workspaceUrl and projectKey are required for Jira")
	}

	return creds, nil
}

func (r *Runner) verifyJiraCredential(ctx context.Context, creds jiraCredential) error {
	workspaceURL := strings.TrimRight(strings.TrimSpace(creds.WorkspaceURL), "/")
	projectKey := url.QueryEscape(strings.TrimSpace(creds.ProjectKey))
	if workspaceURL == "" || projectKey == "" {
		return errors.New("workspaceUrl and projectKey are required for Jira")
	}

	_, err := executeJiraRequestFn(
		ctx,
		http.MethodGet,
		workspaceURL+"/rest/api/3/project/"+projectKey,
		creds,
		nil,
	)
	return err
}

func (r *Runner) detectJiraBackend(record mcpBackendRecord) McpBackend {
	backend := McpBackend{
		Key:           "jira",
		ProviderType:  "jira",
		Label:         "Atlassian MCP",
		Transport:     "remote",
		Launcher:      "remote",
		InstallHint:   "Open the Jira MCP form, read the Atlassian guide, and paste an API token so the runner can connect to the Atlassian remote MCP server.",
		State:         "missing",
		Action:        "install",
		ActionLabel:   "Create MCP",
		LastCheckedAt: time.Now().UTC().Format(time.RFC3339Nano),
		BinaryPath:    "https://mcp.atlassian.com/v1/mcp/authv2",
		Command:       "https://mcp.atlassian.com/v1/mcp/authv2",
		SecretKey:     record.SecretKey,
	}

	if strings.TrimSpace(record.SecretKey) == "" {
		backend.LastError = "No Jira API token is stored for the runner yet."
		return backend
	}

	creds, err := r.loadJiraCredential(record.SecretKey)
	if err != nil {
		backend.LastError = err.Error()
		return backend
	}

	backend.Installed = true
	backend.State = "installed"
	backend.Action = "verify"
	backend.ActionLabel = "Verify"
	backend.Command = "Authorization: Basic <email:api_token>"
	backend.LastCheckedAt = record.LastCheckedAt
	backend.LastError = record.LastError

	if strings.TrimSpace(creds.WorkspaceURL) != "" && strings.TrimSpace(creds.ProjectKey) != "" {
		backend.InstallHint = fmt.Sprintf(
			"Connected to %s for project %s. Use the Jira MCP form to rotate the API token or verify access.",
			strings.TrimSpace(creds.WorkspaceURL),
			creds.ProjectKey,
		)
	}

	return backend
}

func (spec mcpBackendSpec) detect(record mcpBackendRecord) McpBackend {
	backend := McpBackend{
		Key:           spec.Key,
		ProviderType:  spec.ProviderType,
		Label:         spec.Label,
		Transport:     spec.Transport,
		Launcher:      spec.Launcher,
		InstallHint:   spec.InstallHint,
		State:         "missing",
		Action:        "install",
		ActionLabel:   "Install",
		LastCheckedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}

	launcherPath, err := lookPathFn(spec.Launcher)
	if err != nil {
		backend.LastError = fmt.Sprintf("%s launcher is not available on PATH", spec.Launcher)
		return backend
	}

	backend.BinaryPath = launcherPath
	if record.Installed {
		backend.Installed = true
		backend.State = "installed"
		backend.Action = "verify"
		backend.ActionLabel = "Verify"
		backend.Command = spec.commandString(spec.VerifyArgs)
		backend.LastCheckedAt = record.LastCheckedAt
		backend.LastError = record.LastError
		return backend
	}

	backend.Installed = false
	backend.State = "launcher_available"
	backend.Action = "install"
	backend.ActionLabel = "Install"
	backend.Command = spec.commandString(spec.InstallArgs)
	backend.LastError = ""
	return backend
}

func (spec mcpBackendSpec) execute(ctx context.Context, backend *McpBackend, args []string) error {
	launcherPath := backend.BinaryPath
	if strings.TrimSpace(launcherPath) == "" {
		var err error
		launcherPath, err = lookPathFn(spec.Launcher)
		if err != nil {
			backend.Installed = false
			backend.State = "missing"
			backend.Action = "install"
			backend.ActionLabel = "Install"
			backend.LastError = fmt.Sprintf("%s launcher is not available on PATH", spec.Launcher)
			return errors.New(backend.LastError)
		}
		backend.BinaryPath = launcherPath
	}

	if len(args) == 0 {
		backend.Installed = true
		backend.State = "installed"
		backend.LastError = ""
		return nil
	}

	output, execErr := runCommandFn(ctx, launcherPath, args...)
	if execErr != nil {
		backend.Installed = false
		backend.State = "missing"
		backend.Action = "install"
		backend.ActionLabel = "Install"
		backend.LastError = backendErrorMessage(execErr, output)
		return execErr
	}

	backend.Installed = true
	backend.State = "installed"
	backend.LastError = ""
	return nil
}

func (spec mcpBackendSpec) commandString(args []string) string {
	parts := append([]string{spec.Launcher}, args...)
	return strings.Join(parts, " ")
}

func providerLabelForType(providerType string) string {
	switch strings.TrimSpace(providerType) {
	case "google_drive":
		return "Google Drive MCP"
	case "jira":
		return "Atlassian MCP"
	case "telegram":
		return "Telegram"
	case "figma":
		return "Figma"
	case "firebase":
		return "Firebase"
	case "":
		return "MCP"
	default:
		words := strings.Fields(strings.ReplaceAll(providerType, "_", " "))
		for index, word := range words {
			if word == "" {
				continue
			}
			words[index] = strings.ToUpper(word[:1]) + strings.ToLower(word[1:])
		}
		if len(words) == 0 {
			return providerType
		}
		return strings.Join(words, " ")
	}
}

func backendErrorMessage(err error, output []byte) string {
	if len(output) > 0 {
		trimmed := strings.TrimSpace(string(output))
		if trimmed != "" {
			return trimmed
		}
	}
	return err.Error()
}

func nonEmptyOrFallback(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

func stringValueOrFallback(value *string, fallback string) string {
	if value != nil && strings.TrimSpace(*value) != "" {
		return *value
	}
	return fallback
}

func discoverMarkdownEntries(baseDir string, skills bool) ([]markdownEntry, error) {
	info, err := os.Stat(baseDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []markdownEntry{}, nil
		}
		return nil, err
	}

	if !info.IsDir() {
		return []markdownEntry{}, nil
	}

	entries := make([]markdownEntry, 0)
	walkErr := filepath.WalkDir(baseDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}

		name := strings.ToLower(d.Name())
		if skills && name != "skill.md" {
			return nil
		}
		if !skills && !strings.HasSuffix(name, ".md") {
			return nil
		}

		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		entry := parseMarkdownEntry(baseDir, path, string(raw))
		entries = append(entries, entry)
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}

	sort.Slice(entries, func(left, right int) bool {
		return entries[left].Name < entries[right].Name
	})

	return entries, nil
}

func parseMarkdownEntry(baseDir, path, contents string) markdownEntry {
	id := strings.TrimSuffix(filepath.Base(filepath.Dir(path)), filepath.Ext(filepath.Base(path)))
	if id == "" || strings.EqualFold(id, "skills") || strings.EqualFold(id, "flows") {
		id = strings.TrimSuffix(filepath.Base(path), filepath.Ext(filepath.Base(path)))
	}

	name := ""
	description := ""
	bullets := make([]string, 0, 8)

	lines := strings.Split(contents, "\n")
	inFrontMatter := false
	frontMatterDone := false
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if index == 0 && trimmed == "---" {
			inFrontMatter = true
			continue
		}
		if inFrontMatter && trimmed == "---" {
			inFrontMatter = false
			frontMatterDone = true
			continue
		}
		if inFrontMatter {
			if value, ok := strings.CutPrefix(trimmed, "name:"); ok && name == "" {
				name = strings.TrimSpace(value)
				continue
			}
			if value, ok := strings.CutPrefix(trimmed, "description:"); ok && description == "" {
				description = strings.TrimSpace(value)
				continue
			}
		}

		if !frontMatterDone {
			if strings.HasPrefix(trimmed, "# ") && name == "" {
				name = strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
				continue
			}
			if strings.HasPrefix(trimmed, "- ") && len(bullets) < 6 {
				bullets = append(bullets, strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")))
			}
			continue
		}

		if name == "" && strings.HasPrefix(trimmed, "# ") {
			name = strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
			continue
		}
		if description == "" && trimmed != "" && !strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(trimmed, "-") {
			description = trimmed
			continue
		}
		if strings.HasPrefix(trimmed, "- ") && len(bullets) < 6 {
			bullets = append(bullets, strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")))
		}
	}

	if name == "" {
		name = strings.ReplaceAll(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)), "-", " ")
	}

	relPath, err := filepath.Rel(baseDir, path)
	if err != nil {
		relPath = path
	}

	return markdownEntry{
		ID:          id,
		Name:        name,
		FilePath:    filepath.ToSlash(relPath),
		Description: description,
		Tags:        deriveTags(relPath),
		Steps:       bullets,
	}
}

func deriveTags(relPath string) []string {
	parts := strings.Split(filepath.ToSlash(relPath), "/")
	tags := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(strings.TrimSuffix(part, ".md"))
		if part == "" || strings.EqualFold(part, "skill") {
			continue
		}
		tags = append(tags, part)
	}
	return tags
}

func newRunID() string {
	return fmt.Sprintf("prompt_%s_%d", time.Now().UTC().Format("20060102_150405"), time.Now().UTC().UnixNano()%10000)
}

func newMcpTestRunID() string {
	return fmt.Sprintf("mcp_%s_%d", time.Now().UTC().Format("20060102_150405"), time.Now().UTC().UnixNano()%10000)
}

func readTextWithLimit(path string, limit int) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	trimmed := strings.TrimSpace(string(raw))
	if len(trimmed) <= limit {
		return trimmed
	}

	return trimmed[:limit] + "\n...[truncated]"
}

func (r *Runner) executeMcpTest(
	ctx context.Context,
	request McpTestRequest,
) (stdoutSummary string, stderrSummary string, outputMarkdown string, command string, err error) {
	switch request.BackendKey {
	case "jira":
		creds, loadErr := r.loadJiraCredential(jiraCredentialKey(request.IntegrationID))
		if loadErr != nil {
			return "", loadErr.Error(), "", "GET https://mcp.atlassian.com/v1/mcp/authv2", loadErr
		}
		requestProjectID := strings.TrimSpace(request.ProjectID)
		if strings.TrimSpace(creds.ProjectID) == "" {
			creds.ProjectID = requestProjectID
			if saveErr := r.saveJiraCredential(request.IntegrationID, creds); saveErr != nil {
				return "", saveErr.Error(), "", "", saveErr
			}
		}
		return executeJiraMcpPrompt(ctx, request, creds)
	default:
		err = fmt.Errorf("MCP test execution is not implemented for backend %q", request.BackendKey)
		return "", err.Error(), "", "", err
	}
}

func executeJiraMcpPrompt(
	ctx context.Context,
	request McpTestRequest,
	creds jiraCredential,
) (stdoutSummary string, stderrSummary string, outputMarkdown string, command string, err error) {
	prompt := strings.TrimSpace(request.Prompt)
	templateKey := resolveMcpTestTemplateKey(request)

	switch {
	case templateKey == "jira_create_story":
		if !request.AllowWrite {
			err = errors.New("jira write tests require explicit allowWrite confirmation")
			return "", err.Error(), "", "", err
		}

		title := extractStoryTitle(prompt)
		if title == "" {
			err = errors.New("story title is required in the prompt; use wording like \"Create a story titled 'Redesign onboarding'.\"")
			return "", err.Error(), "", "", err
		}

		payload := map[string]any{
			"fields": map[string]any{
				"project": map[string]string{
					"key": strings.TrimSpace(creds.ProjectKey),
				},
				"summary": title,
				"issuetype": map[string]string{
					"name": "Story",
				},
				"description": prompt,
			},
		}
		payloadBytes, marshalErr := json.Marshal(payload)
		if marshalErr != nil {
			return "", marshalErr.Error(), "", "", marshalErr
		}

		command = "POST " + strings.TrimRight(strings.TrimSpace(creds.WorkspaceURL), "/") + "/rest/api/3/issue"
		responseBody, requestErr := executeJiraRequestFn(
			ctx,
			http.MethodPost,
			strings.TrimRight(strings.TrimSpace(creds.WorkspaceURL), "/")+"/rest/api/3/issue",
			creds,
			payloadBytes,
		)
		if requestErr != nil {
			stderrSummary = requestErr.Error()
			outputMarkdown = renderJiraCreateStoryMarkdown(request, creds, title, "", requestErr.Error())
			return "", stderrSummary, outputMarkdown, command, requestErr
		}

		var issue struct {
			Key  string `json:"key"`
			Self string `json:"self"`
		}
		if unmarshalErr := json.Unmarshal(responseBody, &issue); unmarshalErr != nil {
			return "", unmarshalErr.Error(), "", command, unmarshalErr
		}

		if strings.TrimSpace(issue.Key) == "" {
			err = errors.New("jira create issue response did not include an issue key")
			return "", err.Error(), "", command, err
		}

		stdoutSummary = fmt.Sprintf(
			"Created Jira story %s in project %s.",
			issue.Key,
			strings.TrimSpace(creds.ProjectKey),
		)
		outputMarkdown = renderJiraCreateStoryMarkdown(request, creds, title, issue.Key, "")
		return stdoutSummary, "", outputMarkdown, command, nil

	case templateKey == "confluence_list_spaces":
		command = "GET " + strings.TrimRight(strings.TrimSpace(creds.WorkspaceURL), "/") + "/wiki/api/v2/spaces?limit=10"
		responseBody, requestErr := executeJiraRequestFn(
			ctx,
			http.MethodGet,
			strings.TrimRight(strings.TrimSpace(creds.WorkspaceURL), "/")+"/wiki/api/v2/spaces?limit=10",
			creds,
			nil,
		)
		if requestErr != nil {
			stderrSummary = requestErr.Error()
			outputMarkdown = renderConfluenceSpacesMarkdown(request, creds, nil, requestErr.Error())
			return "", stderrSummary, outputMarkdown, command, requestErr
		}

		var payload struct {
			Results []struct {
				ID   string `json:"id"`
				Key  string `json:"key"`
				Name string `json:"name"`
			} `json:"results"`
		}
		if unmarshalErr := json.Unmarshal(responseBody, &payload); unmarshalErr != nil {
			return "", unmarshalErr.Error(), "", command, unmarshalErr
		}

		stdoutSummary = fmt.Sprintf("Listed %d Confluence spaces.", len(payload.Results))
		outputMarkdown = renderConfluenceSpacesMarkdown(request, creds, payload.Results, "")
		return stdoutSummary, "", outputMarkdown, command, nil

	case templateKey == "jira_find_open_bugs" || templateKey == "jira_list_bugs":
		jql := url.QueryEscape(fmt.Sprintf(
			"project = %s AND issuetype = Bug AND statusCategory != Done ORDER BY created DESC",
			strings.TrimSpace(creds.ProjectKey),
		))
		command = "GET " + strings.TrimRight(strings.TrimSpace(creds.WorkspaceURL), "/") + "/rest/api/3/search/jql?maxResults=10&fields=summary,status,assignee&jql=" + jql
		responseBody, requestErr := executeJiraRequestFn(
			ctx,
			http.MethodGet,
			strings.TrimRight(strings.TrimSpace(creds.WorkspaceURL), "/")+"/rest/api/3/search/jql?maxResults=10&fields=summary,status,assignee&jql="+jql,
			creds,
			nil,
		)
		if requestErr != nil {
			stderrSummary = requestErr.Error()
			outputMarkdown = renderJiraIssueListMarkdown(request, creds, "bug", nil, requestErr.Error())
			return "", stderrSummary, outputMarkdown, command, requestErr
		}

		var payload struct {
			Issues []struct {
				Key    string `json:"key"`
				Fields struct {
					Summary string `json:"summary"`
					Status  struct {
						Name string `json:"name"`
					} `json:"status"`
					Assignee *struct {
						DisplayName string `json:"displayName"`
					} `json:"assignee"`
				} `json:"fields"`
			} `json:"issues"`
		}
		if unmarshalErr := json.Unmarshal(responseBody, &payload); unmarshalErr != nil {
			return "", unmarshalErr.Error(), "", command, unmarshalErr
		}

		stdoutSummary = fmt.Sprintf(
			"Found %d open bug issues in project %s.",
			len(payload.Issues),
			strings.TrimSpace(creds.ProjectKey),
		)
		outputMarkdown = renderJiraIssueListMarkdown(request, creds, "bug", payload.Issues, "")
		return stdoutSummary, "", outputMarkdown, command, nil

	case templateKey == "jira_list_user_stories":
		jql := url.QueryEscape(fmt.Sprintf(
			"project = %s AND issuetype in (Story, \"User Story\") ORDER BY created DESC",
			strings.TrimSpace(creds.ProjectKey),
		))
		command = "GET " + strings.TrimRight(strings.TrimSpace(creds.WorkspaceURL), "/") + "/rest/api/3/search/jql?maxResults=10&fields=summary,status,assignee&jql=" + jql
		responseBody, requestErr := executeJiraRequestFn(
			ctx,
			http.MethodGet,
			strings.TrimRight(strings.TrimSpace(creds.WorkspaceURL), "/")+"/rest/api/3/search/jql?maxResults=10&fields=summary,status,assignee&jql="+jql,
			creds,
			nil,
		)
		if requestErr != nil {
			stderrSummary = requestErr.Error()
			outputMarkdown = renderJiraIssueListMarkdown(request, creds, "user story", nil, requestErr.Error())
			return "", stderrSummary, outputMarkdown, command, requestErr
		}

		var payload struct {
			Issues []struct {
				Key    string `json:"key"`
				Fields struct {
					Summary string `json:"summary"`
					Status  struct {
						Name string `json:"name"`
					} `json:"status"`
					Assignee *struct {
						DisplayName string `json:"displayName"`
					} `json:"assignee"`
				} `json:"fields"`
			} `json:"issues"`
		}
		if unmarshalErr := json.Unmarshal(responseBody, &payload); unmarshalErr != nil {
			return "", unmarshalErr.Error(), "", command, unmarshalErr
		}

		stdoutSummary = fmt.Sprintf(
			"Found %d user stories in project %s.",
			len(payload.Issues),
			strings.TrimSpace(creds.ProjectKey),
		)
		outputMarkdown = renderJiraIssueListMarkdown(request, creds, "user story", payload.Issues, "")
		return stdoutSummary, "", outputMarkdown, command, nil

	case templateKey == "jira_get_ticket_content":
		issueKey := extractIssueKey(prompt, strings.TrimSpace(creds.ProjectKey))
		if issueKey == "" {
			err = errors.New("ticket key is required in the prompt; use wording like \"Get the content of Jira ticket SCRUM-1.\"")
			return "", err.Error(), "", "", err
		}

		command = "GET " + strings.TrimRight(strings.TrimSpace(creds.WorkspaceURL), "/") + "/rest/api/3/issue/" + url.PathEscape(issueKey) + "?fields=summary,status,description,assignee"
		responseBody, requestErr := executeJiraRequestFn(
			ctx,
			http.MethodGet,
			strings.TrimRight(strings.TrimSpace(creds.WorkspaceURL), "/")+"/rest/api/3/issue/"+url.PathEscape(issueKey)+"?fields=summary,status,description,assignee",
			creds,
			nil,
		)
		if requestErr != nil {
			stderrSummary = requestErr.Error()
			outputMarkdown = renderJiraIssueContentMarkdown(request, creds, issueKey, nil, requestErr.Error())
			return "", stderrSummary, outputMarkdown, command, requestErr
		}

		var issue struct {
			Key    string `json:"key"`
			Fields struct {
				Summary string `json:"summary"`
				Status  struct {
					Name string `json:"name"`
				} `json:"status"`
				Description any `json:"description"`
				Assignee    *struct {
					DisplayName string `json:"displayName"`
				} `json:"assignee"`
			} `json:"fields"`
		}
		if unmarshalErr := json.Unmarshal(responseBody, &issue); unmarshalErr != nil {
			return "", unmarshalErr.Error(), "", command, unmarshalErr
		}

		stdoutSummary = fmt.Sprintf(
			"Loaded Jira ticket %s from project %s.",
			strings.TrimSpace(issue.Key),
			strings.TrimSpace(creds.ProjectKey),
		)
		outputMarkdown = renderJiraIssueContentMarkdown(request, creds, issueKey, &issue, "")
		return stdoutSummary, "", outputMarkdown, command, nil

	default:
		err = fmt.Errorf("unsupported MCP test template %q for backend %q", templateKey, request.BackendKey)
		return "", err.Error(), "", "", err
	}
}

func resolveMcpTestTemplateKey(request McpTestRequest) string {
	if strings.TrimSpace(request.TemplateKey) != "" {
		return strings.TrimSpace(request.TemplateKey)
	}

	normalizedPrompt := strings.ToLower(strings.TrimSpace(request.Prompt))
	switch {
	case strings.Contains(normalizedPrompt, "create a story"):
		return "jira_create_story"
	case strings.Contains(normalizedPrompt, "spaces do i have access") || strings.Contains(normalizedPrompt, "list confluence spaces"):
		return "confluence_list_spaces"
	case strings.Contains(normalizedPrompt, "user stor"):
		return "jira_list_user_stories"
	case strings.Contains(normalizedPrompt, "ticket") || strings.Contains(normalizedPrompt, "issue content"):
		return "jira_get_ticket_content"
	default:
		return "jira_list_bugs"
	}
}

func extractIssueKey(prompt string, defaultProjectKey string) string {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b([A-Z][A-Z0-9_]+-\d+)\b`),
	}

	for _, pattern := range patterns {
		matches := pattern.FindStringSubmatch(strings.ToUpper(prompt))
		if len(matches) >= 2 && strings.TrimSpace(matches[1]) != "" {
			return strings.TrimSpace(matches[1])
		}
	}

	defaultProjectKey = strings.TrimSpace(strings.ToUpper(defaultProjectKey))
	if defaultProjectKey != "" {
		fallback := regexp.MustCompile(`(?i)\bticket\s+(\d+)\b`).FindStringSubmatch(prompt)
		if len(fallback) >= 2 {
			return fmt.Sprintf("%s-%s", defaultProjectKey, strings.TrimSpace(fallback[1]))
		}
	}

	return ""
}

func extractStoryTitle(prompt string) string {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)create a story titled ['"]([^'"]+)['"]`),
		regexp.MustCompile(`(?i)create a story titled ([^.\n]+)`),
	}

	for _, pattern := range patterns {
		matches := pattern.FindStringSubmatch(prompt)
		if len(matches) < 2 {
			continue
		}

		title := strings.TrimSpace(matches[1])
		title = strings.Trim(title, " .")
		if title != "" {
			return title
		}
	}

	return ""
}

func renderJiraIssueListMarkdown(
	request McpTestRequest,
	creds jiraCredential,
	issueKind string,
	issues []struct {
		Key    string `json:"key"`
		Fields struct {
			Summary string `json:"summary"`
			Status  struct {
				Name string `json:"name"`
			} `json:"status"`
			Assignee *struct {
				DisplayName string `json:"displayName"`
			} `json:"assignee"`
		} `json:"fields"`
	},
	errorMessage string,
) string {
	lines := []string{
		"# Jira MCP Test Run",
		"",
		fmt.Sprintf("- Workspace: %s", strings.TrimSpace(creds.WorkspaceURL)),
		fmt.Sprintf("- Project key: %s", strings.TrimSpace(creds.ProjectKey)),
		fmt.Sprintf("- Integration: %s", strings.TrimSpace(request.IntegrationID)),
		"",
		"## Prompt",
		"",
		request.Prompt,
		"",
		"## Result",
		"",
	}

	if errorMessage != "" {
		lines = append(lines,
			fmt.Sprintf("The runner attempted a Jira %s search and the provider returned an error.", issueKind),
			"",
			fmt.Sprintf("Error: %s", errorMessage),
		)
		return strings.Join(lines, "\n")
	}

	if len(issues) == 0 {
		lines = append(lines, fmt.Sprintf("No %s issues were returned for this Jira project.", issueKind))
		return strings.Join(lines, "\n")
	}

	lines = append(lines, fmt.Sprintf("%s issues returned from Jira:", strings.Title(issueKind)))
	for _, issue := range issues {
		assignee := "Unassigned"
		if issue.Fields.Assignee != nil && strings.TrimSpace(issue.Fields.Assignee.DisplayName) != "" {
			assignee = issue.Fields.Assignee.DisplayName
		}
		lines = append(lines, fmt.Sprintf(
			"- %s: %s (%s, %s)",
			issue.Key,
			issue.Fields.Summary,
			issue.Fields.Status.Name,
			assignee,
		))
	}

	return strings.Join(lines, "\n")
}

func renderJiraIssueContentMarkdown(
	request McpTestRequest,
	creds jiraCredential,
	issueKey string,
	issue *struct {
		Key    string `json:"key"`
		Fields struct {
			Summary string `json:"summary"`
			Status  struct {
				Name string `json:"name"`
			} `json:"status"`
			Description any `json:"description"`
			Assignee    *struct {
				DisplayName string `json:"displayName"`
			} `json:"assignee"`
		} `json:"fields"`
	},
	errorMessage string,
) string {
	lines := []string{
		"# Jira MCP Test Run",
		"",
		fmt.Sprintf("- Workspace: %s", strings.TrimSpace(creds.WorkspaceURL)),
		fmt.Sprintf("- Project key: %s", strings.TrimSpace(creds.ProjectKey)),
		fmt.Sprintf("- Integration: %s", strings.TrimSpace(request.IntegrationID)),
		"",
		"## Prompt",
		"",
		request.Prompt,
		"",
		"## Result",
		"",
	}

	if errorMessage != "" {
		lines = append(lines,
			fmt.Sprintf("The runner could not load Jira ticket %s.", issueKey),
			"",
			fmt.Sprintf("Error: %s", errorMessage),
		)
		return strings.Join(lines, "\n")
	}

	if issue == nil {
		lines = append(lines, "No Jira ticket content was returned.")
		return strings.Join(lines, "\n")
	}

	assignee := "Unassigned"
	if issue.Fields.Assignee != nil && strings.TrimSpace(issue.Fields.Assignee.DisplayName) != "" {
		assignee = issue.Fields.Assignee.DisplayName
	}

	lines = append(lines,
		fmt.Sprintf("Loaded Jira ticket `%s`.", issue.Key),
		fmt.Sprintf("- Summary: %s", issue.Fields.Summary),
		fmt.Sprintf("- Status: %s", issue.Fields.Status.Name),
		fmt.Sprintf("- Assignee: %s", assignee),
		fmt.Sprintf("- Description: %v", issue.Fields.Description),
	)
	return strings.Join(lines, "\n")
}

func renderJiraCreateStoryMarkdown(
	request McpTestRequest,
	creds jiraCredential,
	title string,
	issueKey string,
	errorMessage string,
) string {
	lines := []string{
		"# Jira MCP Test Run",
		"",
		fmt.Sprintf("- Workspace: %s", strings.TrimSpace(creds.WorkspaceURL)),
		fmt.Sprintf("- Project key: %s", strings.TrimSpace(creds.ProjectKey)),
		fmt.Sprintf("- Integration: %s", strings.TrimSpace(request.IntegrationID)),
		"",
		"## Prompt",
		"",
		request.Prompt,
		"",
		"## Result",
		"",
	}

	if errorMessage != "" {
		lines = append(lines,
			fmt.Sprintf("The runner could not create the Jira story %q.", title),
			"",
			fmt.Sprintf("Error: %s", errorMessage),
		)
		return strings.Join(lines, "\n")
	}

	lines = append(lines,
		fmt.Sprintf("Created Jira story `%s` with title %q.", issueKey, title),
		"This confirms the stored MCP credential can perform a write operation for the selected project.",
	)
	return strings.Join(lines, "\n")
}

func renderConfluenceSpacesMarkdown(
	request McpTestRequest,
	creds jiraCredential,
	spaces []struct {
		ID   string `json:"id"`
		Key  string `json:"key"`
		Name string `json:"name"`
	},
	errorMessage string,
) string {
	lines := []string{
		"# Atlassian MCP Test Run",
		"",
		fmt.Sprintf("- Workspace: %s", strings.TrimSpace(creds.WorkspaceURL)),
		fmt.Sprintf("- Integration: %s", strings.TrimSpace(request.IntegrationID)),
		"",
		"## Prompt",
		"",
		request.Prompt,
		"",
		"## Result",
		"",
	}

	if errorMessage != "" {
		lines = append(lines,
			"The runner could not list Confluence spaces for this Atlassian connection.",
			"",
			fmt.Sprintf("Error: %s", errorMessage),
		)
		return strings.Join(lines, "\n")
	}

	if len(spaces) == 0 {
		lines = append(lines, "No Confluence spaces were returned for this credential.")
		return strings.Join(lines, "\n")
	}

	lines = append(lines, "Confluence spaces returned from Atlassian:")
	for _, space := range spaces {
		lines = append(lines, fmt.Sprintf("- %s (%s)", space.Name, space.Key))
	}

	return strings.Join(lines, "\n")
}

func writeJSONFile(path string, value any) error {
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, payload, 0o644)
}

func readJSONFile(path string, target any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(stripUTF8BOM(raw), target)
}

func stripUTF8BOM(raw []byte) []byte {
	if len(raw) >= 3 && raw[0] == 0xef && raw[1] == 0xbb && raw[2] == 0xbf {
		return raw[3:]
	}
	return raw
}

func LaunchTerminalWithCommand(command string) error {
	osType := runtime.GOOS
	switch osType {
	case "windows":
		cmd := exec.Command("cmd.exe", "/c", "start", "cmd.exe", "/k", command)
		return cmd.Run()
	case "darwin":
		script := fmt.Sprintf(`tell app "Terminal" to do script "%s"`, command)
		cmd := exec.Command("osascript", "-e", script)
		return cmd.Run()
	case "linux":
		terminals := []struct {
			name string
			args []string
		}{
			{"x-terminal-emulator", []string{"-e", command}},
			{"gnome-terminal", []string{"--", "sh", "-c", command}},
			{"konsole", []string{"-e", command}},
			{"xfce4-terminal", []string{"-e", command}},
			{"alacritty", []string{"-e", "sh", "-c", command}},
		}

		for _, t := range terminals {
			if path, err := exec.LookPath(t.name); err == nil {
				cmd := exec.Command(path, t.args...)
				if err := cmd.Start(); err == nil {
					return nil
				}
			}
		}
		return fmt.Errorf("no supported terminal emulator found")
	default:
		return fmt.Errorf("unsupported operating system %q for terminal spawning", osType)
	}
}

func launchProviderTerminalCommand(providerKey, accountHomePath, command string, keepShellOpen bool) error {
	if strings.TrimSpace(command) == "" {
		return errors.New("terminal command is empty")
	}

	switch runtime.GOOS {
	case "windows":
		scriptPath, err := writeWindowsProviderTerminalScript(providerKey, accountHomePath, command)
		if err != nil {
			return err
		}
		cmd := exec.Command("cmd.exe", "/c", "start", "", "cmd.exe", "/k", scriptPath)
		if err := cmd.Start(); err != nil {
			return err
		}
		return cmd.Process.Release()
	case "darwin":
		envExport := providerEnvSetCommand(providerKey, accountHomePath, "posix")
		shellCommand := fmt.Sprintf("%s && %s", envExport, command)
		if keepShellOpen {
			shellCommand += `; exec "$SHELL" -l`
		}
		escaped := escapeAppleScriptString(shellCommand)
		script := fmt.Sprintf(
			`tell application "Terminal"
activate
do script "%s"
end tell`,
			escaped,
		)
		cmd := exec.Command("osascript", "-e", script)
		cmd.Env = (&Runner{}).getEnvForExecution(providerKey, accountHomePath, nil, "")
		if err := cmd.Start(); err != nil {
			return err
		}
		return cmd.Process.Release()
	case "linux":
		envExport := providerEnvSetCommand(providerKey, accountHomePath, "posix")
		shellCommand := fmt.Sprintf("%s && %s", envExport, command)
		if keepShellOpen {
			shellCommand += "; exec bash"
		}
		launchCommand := fmt.Sprintf("bash -lc %s", singleQuoteForShell(shellCommand))
		terminals := []struct {
			name string
			args []string
		}{
			{"x-terminal-emulator", []string{"-e", launchCommand}},
			{"gnome-terminal", []string{"--", "bash", "-lc", shellCommand}},
			{"konsole", []string{"-e", "bash", "-lc", shellCommand}},
			{"xfce4-terminal", []string{"-e", launchCommand}},
			{"alacritty", []string{"-e", "bash", "-lc", shellCommand}},
		}

		for _, terminal := range terminals {
			path, err := exec.LookPath(terminal.name)
			if err != nil {
				continue
			}
			cmd := exec.Command(path, terminal.args...)
			cmd.Env = (&Runner{}).getEnvForExecution(providerKey, accountHomePath, nil, "")
			if err := cmd.Start(); err == nil {
				return cmd.Process.Release()
			}
		}
		return fmt.Errorf("no supported terminal emulator found")
	default:
		return fmt.Errorf("unsupported operating system %q for terminal spawning", runtime.GOOS)
	}
}

func terminalEnvSetCommand(env map[string]string, shellType string) string {
	if len(env) == 0 {
		return ""
	}

	keys := make([]string, 0, len(env))
	for key := range env {
		if strings.TrimSpace(key) == "" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)

	switch shellType {
	case "posix":
		lines := make([]string, 0, len(keys))
		for _, key := range keys {
			lines = append(lines, fmt.Sprintf("export %s=%s", key, singleQuoteForShell(env[key])))
		}
		return strings.Join(lines, " && ")
	case "windows":
		lines := make([]string, 0, len(keys))
		for _, key := range keys {
			lines = append(lines, fmt.Sprintf(`set "%s=%s"`, key, strings.ReplaceAll(env[key], `"`, `""`)))
		}
		return strings.Join(lines, "\r\n")
	default:
		return ""
	}
}

func writeWindowsTerminalScript(env map[string]string, command string) (string, error) {
	scriptFile, err := os.CreateTemp("", "flowpilot-terminal-*.cmd")
	if err != nil {
		return "", err
	}
	defer scriptFile.Close()

	lines := []string{"@echo off"}
	if envCommand := terminalEnvSetCommand(env, "windows"); envCommand != "" {
		lines = append(lines, envCommand)
	}
	lines = append(lines, command, "")

	if _, err := scriptFile.WriteString(strings.Join(lines, "\r\n")); err != nil {
		return "", err
	}

	return scriptFile.Name(), nil
}

func launchTerminalCommandWithEnv(env map[string]string, command string, keepShellOpen bool) error {
	if strings.TrimSpace(command) == "" {
		return errors.New("terminal command is empty")
	}

	switch runtime.GOOS {
	case "windows":
		scriptPath, err := writeWindowsTerminalScript(env, command)
		if err != nil {
			return err
		}
		cmd := exec.Command("cmd.exe", "/c", "start", "", "cmd.exe", "/k", scriptPath)
		if err := cmd.Start(); err != nil {
			return err
		}
		return cmd.Process.Release()
	case "darwin":
		shellCommand := command
		if envCommand := terminalEnvSetCommand(env, "posix"); envCommand != "" {
			shellCommand = envCommand + " && " + shellCommand
		}
		if keepShellOpen {
			shellCommand += `; exec "$SHELL" -l`
		}
		script := fmt.Sprintf(
			`tell application "Terminal"
activate
do script "%s"
end tell`,
			escapeAppleScriptString(shellCommand),
		)
		cmd := exec.Command("osascript", "-e", script)
		if err := cmd.Start(); err != nil {
			return err
		}
		return cmd.Process.Release()
	case "linux":
		shellCommand := command
		if envCommand := terminalEnvSetCommand(env, "posix"); envCommand != "" {
			shellCommand = envCommand + " && " + shellCommand
		}
		if keepShellOpen {
			shellCommand += "; exec bash"
		}
		launchCommand := fmt.Sprintf("bash -lc %s", singleQuoteForShell(shellCommand))
		terminals := []struct {
			name string
			args []string
		}{
			{"x-terminal-emulator", []string{"-e", launchCommand}},
			{"gnome-terminal", []string{"--", "bash", "-lc", shellCommand}},
			{"konsole", []string{"-e", "bash", "-lc", shellCommand}},
			{"xfce4-terminal", []string{"-e", launchCommand}},
			{"alacritty", []string{"-e", "bash", "-lc", shellCommand}},
		}

		for _, terminal := range terminals {
			path, err := exec.LookPath(terminal.name)
			if err != nil {
				continue
			}
			cmd := exec.Command(path, terminal.args...)
			if err := cmd.Start(); err == nil {
				return cmd.Process.Release()
			}
		}
		return fmt.Errorf("no supported terminal emulator found")
	default:
		return fmt.Errorf("unsupported operating system %q for terminal spawning", runtime.GOOS)
	}
}

var (
	launchTerminalCommandWithEnvFn  = launchTerminalCommandWithEnv
	launchProviderTerminalCommandFn = launchProviderTerminalCommand
)

func escapeAppleScriptString(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return replacer.Replace(value)
}

func singleQuoteForShell(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func doubleQuoteForCmd(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func writeWindowsProviderTerminalScript(providerKey, accountHomePath, command string) (string, error) {
	scriptFile, err := os.CreateTemp("", "flowpilot-provider-terminal-*.cmd")
	if err != nil {
		return "", err
	}
	defer scriptFile.Close()

	script := strings.Join([]string{
		"@echo off",
		providerEnvSetCommand(providerKey, accountHomePath, "windows"),
		command,
		"",
	}, "\r\n")

	if _, err := scriptFile.WriteString(script); err != nil {
		return "", err
	}

	return scriptFile.Name(), nil
}

func commandWithWindowsWorkingDirectory(command, workingDir string) string {
	trimmedDir := strings.TrimSpace(workingDir)
	if trimmedDir == "" {
		return command
	}

	return fmt.Sprintf("cd /d %s && %s", doubleQuoteForCmd(trimmedDir), command)
}

func commandWithWorkingDirectory(command, workingDir string) string {
	trimmedDir := strings.TrimSpace(workingDir)
	if trimmedDir == "" {
		return command
	}

	switch runtime.GOOS {
	case "windows":
		return commandWithWindowsWorkingDirectory(command, trimmedDir)
	default:
		return fmt.Sprintf("cd %s && %s", singleQuoteForShell(trimmedDir), command)
	}
}

func (r *Runner) AuthenticateProvider(ctx context.Context, providerName string) error {
	spec, ok := lookupProviderSpec(providerName)
	if !ok {
		return fmt.Errorf("unsupported AI provider %q", providerName)
	}

	var authCommand string
	switch spec.Key {
	case "codex":
		authCommand = "codex login"
	case "claude":
		authCommand = "claude auth login"
	case "gemini":
		authCommand = "agy"
	case "grok":
		// Appended last (CP-46 P-0/Task-210 T-4). `grok login --device-auth` is
		// the headless/managed-home variant; StartInteractiveAuth below routes
		// through that for non-default-slot accounts.
		authCommand = "grok login"
	default:
		return fmt.Errorf("no auth command configured for provider %q", providerName)
	}

	return LaunchTerminalWithCommand(authCommand)
}

func (r *Runner) StartGoogleDriveMcpAuth() error {
	config, err := r.googleDriveMcpRuntimeConfig()
	if err != nil {
		return err
	}
	if !config.CredentialExists || !config.CredentialValid {
		return errors.New("google drive MCP OAuth credentials are not configured")
	}
	if err := os.MkdirAll(filepath.Dir(config.TokenPath), 0o755); err != nil {
		return err
	}

	launcherPath, err := lookPathFn("npx")
	if err != nil {
		return fmt.Errorf("google drive MCP launcher \"npx\" is not available: %w", err)
	}

	authInvocation := launcherPath
	if runtime.GOOS == "windows" {
		authInvocation = doubleQuoteForCmd(launcherPath)
		lowerPath := strings.ToLower(launcherPath)
		if strings.HasSuffix(lowerPath, ".cmd") || strings.HasSuffix(lowerPath, ".bat") {
			authInvocation = "call " + authInvocation
		}
	} else {
		authInvocation = singleQuoteForShell(launcherPath)
	}

	authCommand := fmt.Sprintf("%s -y @piotr-agier/google-drive-mcp auth", authInvocation)
	env := map[string]string{
		"GOOGLE_DRIVE_MCP_TOKEN_PATH":    config.TokenPath,
		"GOOGLE_DRIVE_OAUTH_CREDENTIALS": config.CredentialPath,
	}

	return launchTerminalCommandWithEnvFn(env, authCommand, true)
}

func (r *Runner) getEnvForExecution(
	providerKey string,
	accountHomePath string,
	customEnv map[string]string,
	proxyURL string,
) []string {
	rawEnv := os.Environ()
	// agy hangs when it inherits CLAUDECODE=1 or AI_AGENT=claude-code from the
	// Claude Code parent process.  Strip those vars for the Gemini provider.
	baseEnv := rawEnv
	if strings.EqualFold(providerKey, "gemini") {
		baseEnv = agyFilteredEnv(rawEnv, nil)
	}
	trimmedAccountHomePath := strings.TrimSpace(accountHomePath)

	if trimmedAccountHomePath == "" {
		newEnv := make([]string, 0, len(baseEnv)+len(customEnv)+2)
		for _, envVar := range baseEnv {
			parts := strings.SplitN(envVar, "=", 2)
			if len(parts) == 0 {
				continue
			}
			key := parts[0]
			if _, exists := customEnv[key]; exists {
				continue
			}
			if strings.TrimSpace(proxyURL) != "" && (key == "HTTP_PROXY" || key == "HTTPS_PROXY") {
				continue
			}
			newEnv = append(newEnv, envVar)
		}
		if strings.TrimSpace(proxyURL) != "" {
			newEnv = append(newEnv, fmt.Sprintf("HTTP_PROXY=%s", proxyURL))
			newEnv = append(newEnv, fmt.Sprintf("HTTPS_PROXY=%s", proxyURL))
		}
		for k, v := range customEnv {
			newEnv = append(newEnv, fmt.Sprintf("%s=%s", k, v))
		}
		return newEnv
	}

	var newEnv []string
	for _, envVar := range baseEnv {
		parts := strings.SplitN(envVar, "=", 2)
		if len(parts) == 0 {
			continue
		}
		key := parts[0]
		if key == "HOME" || key == "USERPROFILE" || key == "APPDATA" || key == "LOCALAPPDATA" || key == "HOMEPATH" || key == "HOMEDRIVE" || key == "XDG_CONFIG_HOME" || key == "CODEX_HOME" || key == "GROK_HOME" || key == "HTTP_PROXY" || key == "HTTPS_PROXY" {
			continue
		}
		if _, exists := customEnv[key]; exists {
			continue
		}
		newEnv = append(newEnv, envVar)
	}

	_ = os.MkdirAll(filepath.Join(trimmedAccountHomePath, "AppData", "Roaming"), 0755)
	_ = os.MkdirAll(filepath.Join(trimmedAccountHomePath, "AppData", "Local"), 0755)
	_ = os.MkdirAll(filepath.Join(trimmedAccountHomePath, ".config"), 0755)

	switch strings.ToLower(providerKey) {
	case "codex":
		newEnv = append(newEnv, fmt.Sprintf("CODEX_HOME=%s", trimmedAccountHomePath))
		newEnv = append(newEnv, fmt.Sprintf("HOME=%s", trimmedAccountHomePath))
		newEnv = append(newEnv, fmt.Sprintf("XDG_CONFIG_HOME=%s/.config", trimmedAccountHomePath))
	case "grok":
		// Appended last (CP-46 P-0/Task-210 T-3): codex case above unchanged.
		newEnv = append(newEnv, fmt.Sprintf("GROK_HOME=%s", trimmedAccountHomePath))
		newEnv = append(newEnv, fmt.Sprintf("HOME=%s", trimmedAccountHomePath))
		newEnv = append(newEnv, fmt.Sprintf("XDG_CONFIG_HOME=%s/.config", trimmedAccountHomePath))
	default:
		newEnv = append(newEnv, fmt.Sprintf("HOME=%s", trimmedAccountHomePath))
		newEnv = append(newEnv, fmt.Sprintf("XDG_CONFIG_HOME=%s/.config", trimmedAccountHomePath))
	}

	if runtime.GOOS == "windows" {
		newEnv = append(newEnv, fmt.Sprintf("USERPROFILE=%s", trimmedAccountHomePath))
		newEnv = append(newEnv, fmt.Sprintf("APPDATA=%s\\AppData\\Roaming", trimmedAccountHomePath))
		newEnv = append(newEnv, fmt.Sprintf("LOCALAPPDATA=%s\\AppData\\Local", trimmedAccountHomePath))

		drive := "C:"
		path := strings.TrimPrefix(trimmedAccountHomePath, "C:")
		if strings.Contains(trimmedAccountHomePath, ":") {
			parts := strings.SplitN(trimmedAccountHomePath, ":", 2)
			drive = parts[0] + ":"
			path = parts[1]
		}
		newEnv = append(newEnv, fmt.Sprintf("HOMEDRIVE=%s", drive))
		newEnv = append(newEnv, fmt.Sprintf("HOMEPATH=%s", path))
	}

	if strings.TrimSpace(proxyURL) != "" {
		newEnv = append(newEnv, fmt.Sprintf("HTTP_PROXY=%s", proxyURL))
		newEnv = append(newEnv, fmt.Sprintf("HTTPS_PROXY=%s", proxyURL))
	}

	for k, v := range customEnv {
		newEnv = append(newEnv, fmt.Sprintf("%s=%s", k, v))
	}

	return newEnv
}

func windowsHomeDriveAndPath(homePath string) (string, string, bool) {
	if runtime.GOOS != "windows" {
		return "", "", false
	}
	trimmed := strings.TrimSpace(homePath)
	if trimmed == "" {
		return "", "", false
	}
	drive := "C:"
	path := strings.TrimPrefix(trimmed, "C:")
	if strings.Contains(trimmed, ":") {
		parts := strings.SplitN(trimmed, ":", 2)
		drive = parts[0] + ":"
		path = parts[1]
	}
	return drive, path, true
}

func NextAccountHomePath(providerKey string, existing []string) (string, int, error) {
	home := preferredUserHomeDir()
	if home == "" {
		return "", 0, errors.New("unable to resolve user home directory")
	}

	prefix := ""
	switch strings.ToLower(providerKey) {
	case "codex":
		prefix = ".codexHome"
	case "claude":
		prefix = ".claudeHome"
	case "gemini":
		prefix = ".geminiHome"
	case "grok":
		// Appended last (CP-46 P-0/Task-210 T-2).
		prefix = ".grokHome"
	default:
		return "", 0, fmt.Errorf("unsupported provider %q", providerKey)
	}

	existingPaths := make(map[string]bool)
	for _, p := range existing {
		existingPaths[canonicalPathKey(p)] = true
	}

	for i := 1; i < 1000; i++ {
		path := filepath.Join(home, fmt.Sprintf("%s%d", prefix, i))
		if existingPaths[canonicalPathKey(path)] {
			continue
		}
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return path, i, nil
		}
	}
	return "", 0, errors.New("no free slot found")
}

func (r *Runner) StartInteractiveAuth(providerKey string, accountHomePath string) error {
	spec, ok := lookupProviderSpec(providerKey)
	if !ok {
		return fmt.Errorf("unsupported provider %q", providerKey)
	}

	binaryPath, err := lookPathFn(spec.BinaryName)
	if err != nil {
		return fmt.Errorf("provider binary %q not found: %w", spec.BinaryName, err)
	}

	authInvocation := binaryPath
	if runtime.GOOS == "windows" {
		authInvocation = doubleQuoteForCmd(binaryPath)
		lowerPath := strings.ToLower(binaryPath)
		if strings.HasSuffix(lowerPath, ".cmd") || strings.HasSuffix(lowerPath, ".bat") {
			authInvocation = "call " + authInvocation
		}
	}

	var authCommand string
	switch strings.ToLower(providerKey) {
	case "claude":
		authCommand = fmt.Sprintf("%s login", authInvocation)
	case "codex":
		authCommand = fmt.Sprintf("%s login", authInvocation)
	case "gemini":
		authCommand = authInvocation
	case "grok":
		// Appended last (CP-46 P-0/Task-210 T-4). A managed (.grokHomeN) slot
		// uses the headless device-auth flow so the fresh GROK_HOME isolates
		// cleanly without borrowing the default home's browser session
		// (CP-46 Q-6); slot 0 (~/.grok) uses the normal interactive login.
		if prefix, ok := managedProviderHomePrefix("grok"); ok && strings.HasPrefix(filepath.Base(accountHomePath), prefix) {
			authCommand = fmt.Sprintf("%s login --device-auth", authInvocation)
		} else {
			authCommand = fmt.Sprintf("%s login", authInvocation)
		}
	default:
		return fmt.Errorf("provider %s does not support interactive CLI login", providerKey)
	}

	authCommand = commandWithWorkingDirectory(authCommand, r.workspace)
	return launchProviderTerminalCommandFn(providerKey, accountHomePath, authCommand, true)
}

func (r *Runner) StartInteractiveTest(providerKey string, accountHomePath string) error {
	spec, ok := lookupProviderSpec(providerKey)
	if !ok {
		return fmt.Errorf("unsupported provider %q", providerKey)
	}

	binaryPath, err := lookPathFn(spec.BinaryName)
	if err != nil {
		return fmt.Errorf("provider binary %q not found: %w", spec.BinaryName, err)
	}

	testInvocation := binaryPath
	if runtime.GOOS == "windows" {
		testInvocation = doubleQuoteForCmd(binaryPath)
		lowerPath := strings.ToLower(binaryPath)
		if strings.HasSuffix(lowerPath, ".cmd") || strings.HasSuffix(lowerPath, ".bat") {
			testInvocation = "call " + testInvocation
		}
	}

	testCommand := commandWithWorkingDirectory(testInvocation, r.workspace)
	return launchProviderTerminalCommandFn(providerKey, accountHomePath, testCommand, true)
}

func providerEnvSetCommand(providerKey, homePath, shellType string) string {
	switch shellType {
	case "posix":
		switch strings.ToLower(providerKey) {
		case "codex":
			return fmt.Sprintf("export CODEX_HOME='%s' && export HOME='%s' && export XDG_CONFIG_HOME='%s/.config'", homePath, homePath, homePath)
		case "grok":
			// Appended last (CP-46 P-0/Task-210 T-3).
			return fmt.Sprintf("export GROK_HOME='%s' && export HOME='%s' && export XDG_CONFIG_HOME='%s/.config'", homePath, homePath, homePath)
		default:
			return fmt.Sprintf("export HOME='%s' && export XDG_CONFIG_HOME='%s/.config'", homePath, homePath)
		}
	case "windows":
		switch strings.ToLower(providerKey) {
		case "codex":
			return strings.Join([]string{
				fmt.Sprintf("set CODEX_HOME=%s", homePath),
				fmt.Sprintf("set HOME=%s", homePath),
				fmt.Sprintf("set USERPROFILE=%s", homePath),
			}, "\r\n")
		case "grok":
			// Appended last (CP-46 P-0/Task-210 T-3).
			return strings.Join([]string{
				fmt.Sprintf("set GROK_HOME=%s", homePath),
				fmt.Sprintf("set HOME=%s", homePath),
				fmt.Sprintf("set USERPROFILE=%s", homePath),
			}, "\r\n")
		default:
			return strings.Join([]string{
				fmt.Sprintf("set HOME=%s", homePath),
				fmt.Sprintf("set USERPROFILE=%s", homePath),
				fmt.Sprintf("set APPDATA=%s\\AppData\\Roaming", homePath),
				fmt.Sprintf("set LOCALAPPDATA=%s\\AppData\\Local", homePath),
				fmt.Sprintf("set XDG_CONFIG_HOME=%s\\.config", homePath),
			}, "\r\n")
		}
	default:
		return ""
	}
}

func HasLocalAuthAtPath(providerKey string, homePath string) bool {
	for _, path := range accountAuthPaths(providerKey, homePath) {
		if hasValidProviderAuthFile(providerKey, path) {
			return true
		}
	}

	return false
}
