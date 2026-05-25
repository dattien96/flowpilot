package runner

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
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
)

type Runner struct {
	workspace   string
	startedAt   time.Time
	secretStore SecretStore
}

func New(workspace string) (*Runner, error) {
	resolved, err := ResolveWorkspace(workspace)
	if err != nil {
		return nil, err
	}

	return &Runner{
		workspace:   resolved,
		startedAt:   time.Now().UTC(),
		secretStore: newDefaultSecretStore(),
	}, nil
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

func resolvePromptExecutionAdapter(request PromptExecutionRequest, outputPath string) (string, []string, string, error) {
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
			args = append(args, "--model", modelName)
		}
		if request.ReasoningEffort != "" {
			args = append(args, "-c", fmt.Sprintf("reasoning_effort=%s", strings.ToLower(request.ReasoningEffort)))
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
			effort := strings.ToLower(request.ReasoningEffort)
			if effort == "xhigh" {
				effort = "max"
			}
			args = append(args, "--effort", effort)
		}
		return "claude", args, resolvedProvider, nil
	case "gemini":
		args := []string{}
		if modelName != "" {
			cliModel := modelName
			if strings.HasPrefix(lowerModel, "gemini-") {
				cliModel = strings.TrimPrefix(lowerModel, "gemini-")
			}
			args = append(args, "--model", cliModel)
		}
		return "gemini", args, resolvedProvider, nil
	default:
		return "", nil, "", fmt.Errorf("provider %q is not supported", resolvedProvider)
	}
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

	finalPrompt := r.injectSkillContent(workspace, request.Prompt, request.SkillIds)
	if err := os.WriteFile(promptPath, []byte(finalPrompt), 0o644); err != nil {
		return PromptExecutionResult{}, err
	}

	binary, args, resolvedProvider, err := resolvePromptExecutionAdapter(request, outputPath)
	if err != nil {
		return PromptExecutionResult{}, err
	}

	command := binary + " " + strings.Join(args, " ") + " < prompt.txt"
	if err := os.WriteFile(commandPath, []byte(command), 0o644); err != nil {
		return PromptExecutionResult{}, err
	}

	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(execCtx, binary, args...)
	promptFile, err := os.Open(promptPath)
	if err != nil {
		return PromptExecutionResult{}, err
	}
	defer promptFile.Close()

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

	cmd.Stdin = promptFile
	cmd.Stdout = stdoutFile
	cmd.Stderr = stderrFile
	cmd.Dir = workspace

	startedAt := time.Now().UTC()
	runErr := cmd.Run()
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
		StartedAt:    startedAt.Format(time.RFC3339Nano),
		CompletedAt:  completedAt.Format(time.RFC3339Nano),
		ExitCode:     exitCode,
		ErrorMessage: errorMessage,
	}

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
			Label:       "Gemini",
			BinaryName:  "gemini",
			InstallHint: "Install the Gemini CLI, log in, and restart the runner.",
			Models: []ProviderModel{
				{ID: "gemini-pro", DisplayName: "gemini-pro", Source: "registry"},
				{ID: "gemini-flash", DisplayName: "gemini-flash", Source: "registry"},
			},
		},
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
		Models:        buildProviderModels(spec, false),
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
	provider.Models = buildProviderModels(spec, provider.AuthStatus == "READY")
	if provider.AuthStatus == "AUTH_REQUIRED" {
		authError := fmt.Sprintf("%s authentication is required", spec.Label)
		provider.LastError = &authError
	}

	return provider
}

func buildProviderModels(spec providerSpec, available bool) []ProviderModel {
	models := make([]ProviderModel, 0, len(spec.Models))
	for _, model := range spec.Models {
		model.Available = available
		models = append(models, model)
	}

	return models
}

func getPossibleHomeDirs() []string {
	var dirs []string
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		dirs = append(dirs, home)
	}
	if userProfile := os.Getenv("USERPROFILE"); userProfile != "" {
		dirs = append(dirs, userProfile)
	}
	if homeEnv := os.Getenv("HOME"); homeEnv != "" {
		dirs = append(dirs, homeEnv)
	}
	if appData := os.Getenv("APPDATA"); appData != "" {
		dirs = append(dirs, appData)
	}
	return dirs
}

func hasLocalAuth(providerKey string) bool {
	dirs := getPossibleHomeDirs()
	for _, dir := range dirs {
		var paths []string
		switch providerKey {
		case "codex":
			paths = []string{
				filepath.Join(dir, ".codex", "auth.json"),
				filepath.Join(dir, "codex", "auth.json"),
			}
		case "claude":
			paths = []string{
				filepath.Join(dir, ".claude.json"),
				filepath.Join(dir, "claude", "auth.json"),
				filepath.Join(dir, ".config", "claude", "auth.json"),
			}
		case "gemini":
			paths = []string{
				filepath.Join(dir, ".gemini", "oauth_creds.json"),
				filepath.Join(dir, "gemini", "oauth_creds.json"),
			}
		}

		for _, path := range paths {
			if info, err := os.Stat(path); err == nil && info.Size() > 0 {
				if data, err := os.ReadFile(path); err == nil {
					content := string(data)
					switch providerKey {
					case "codex":
						if strings.Contains(content, `"id_token"`) || strings.Contains(content, `"OPENAI_API_KEY"`) {
							return true
						}
					case "claude":
						if strings.Contains(content, `"emailAddress"`) {
							return true
						}
					case "gemini":
						if strings.Contains(content, `"access_token"`) || strings.Contains(content, `"refresh_token"`) {
							return true
						}
					}
				}
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
		if hasAnyEnv("GOOGLE_API_KEY", "GEMINI_API_KEY") || hasLocalAuth("gemini") {
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
		return "npm", []string{"install", "-g", "@google/gemini-cli"}, nil
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
	provider.Models = buildProviderModels(providerSpec{Models: models}, status == "INSTALLED" && authStatus == "READY")
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
	return json.Unmarshal(raw, target)
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
		authCommand = "gemini"
	default:
		return fmt.Errorf("no auth command configured for provider %q", providerName)
	}

	return LaunchTerminalWithCommand(authCommand)
}

