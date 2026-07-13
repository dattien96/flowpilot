package runner

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
)

type memorySecretStore struct {
	values map[string]string
}

func newMemorySecretStore() *memorySecretStore {
	return &memorySecretStore{values: map[string]string{}}
}

func containsEnvValue(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func (s *memorySecretStore) Set(key, value string) error {
	s.values[key] = value
	return nil
}

func (s *memorySecretStore) Get(key string) (string, error) {
	value, ok := s.values[key]
	if !ok {
		return "", errors.New("not found")
	}
	return value, nil
}

func (s *memorySecretStore) Delete(key string) error {
	delete(s.values, key)
	return nil
}

func TestTriggerIntegrationConnectionAcceptsValidRequest(t *testing.T) {
	instance := &Runner{workspace: t.TempDir()}

	// Task-232 gave "telegram" real credential validation (bot token/channel
	// id required, status becomes "connected"/"failed" instead of a bare
	// placeholder "pending" acknowledgement) — this test now exercises
	// "figma", the one remaining provider still on the generic no-op
	// placeholder path this test was originally written to cover.
	result, err := instance.TriggerIntegrationConnection(context.Background(), "integration-1", IntegrationConnectionRequest{
		ProjectID:    "project-alpha",
		ProviderType: "figma",
		Action:       "test",
	})
	if err != nil {
		t.Fatalf("trigger integration connection: %v", err)
	}

	if result.RequestStatus != "accepted" {
		t.Fatalf("expected accepted request status, got %q", result.RequestStatus)
	}
	if result.IntegrationID != "integration-1" {
		t.Fatalf("expected integration id to round-trip, got %q", result.IntegrationID)
	}
	if result.IntegrationStatus != "pending" {
		t.Fatalf("expected pending integration status, got %q", result.IntegrationStatus)
	}
	if result.Message == nil || !strings.Contains(*result.Message, "project-alpha") {
		t.Fatalf("expected message to mention project id, got %#v", result.Message)
	}
}

func TestPickDirectoryReturnsSelectedPath(t *testing.T) {
	instance := &Runner{workspace: t.TempDir()}

	originalRunCommand := runCommandFn
	t.Cleanup(func() {
		runCommandFn = originalRunCommand
	})

	var observedName string
	var observedArgs []string
	runCommandFn = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		observedName = name
		observedArgs = append([]string(nil), args...)
		return []byte("  /tmp/project-alpha \n"), nil
	}

	result, err := instance.PickDirectory(context.Background())
	if err != nil {
		t.Fatalf("pick directory: %v", err)
	}

	if result.Path != filepath.Clean("/tmp/project-alpha") {
		t.Fatalf("expected cleaned selected path, got %q", result.Path)
	}

	switch runtime.GOOS {
	case "darwin":
		if observedName != "osascript" {
			t.Fatalf("expected osascript on darwin, got %q", observedName)
		}
		if len(observedArgs) != 2 || observedArgs[0] != "-e" {
			t.Fatalf("expected AppleScript arguments, got %v", observedArgs)
		}
	case "linux":
		if observedName != "zenity" {
			t.Fatalf("expected zenity on linux, got %q", observedName)
		}
		if !slices.Equal(observedArgs, []string{"--file-selection", "--directory", "--title=Select project folder"}) {
			t.Fatalf("unexpected zenity args: %v", observedArgs)
		}
	case "windows":
		if observedName != "powershell" {
			t.Fatalf("expected powershell on windows, got %q", observedName)
		}
		if len(observedArgs) == 0 {
			t.Fatal("expected powershell arguments for folder browser")
		}
	default:
		t.Fatalf("unexpected runtime.GOOS in test: %s", runtime.GOOS)
	}
}

func TestPickDirectoryRejectsEmptySelection(t *testing.T) {
	instance := &Runner{workspace: t.TempDir()}

	originalRunCommand := runCommandFn
	t.Cleanup(func() {
		runCommandFn = originalRunCommand
	})

	runCommandFn = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return []byte(" \n "), nil
	}

	_, err := instance.PickDirectory(context.Background())
	if err == nil {
		t.Fatal("expected empty selection to return an error")
	}
	if !strings.Contains(err.Error(), "no directory was selected") {
		t.Fatalf("expected no-selection error, got %v", err)
	}
}

func TestValidateDirectoryReportsUsableDirectory(t *testing.T) {
	instance := &Runner{workspace: t.TempDir()}
	projectDir := filepath.Join(t.TempDir(), "project-alpha")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("mkdir project dir: %v", err)
	}

	result := instance.ValidateDirectory(projectDir)
	if !result.Usable {
		t.Fatalf("expected directory to be usable, got %#v", result)
	}
	if result.Reason != "" {
		t.Fatalf("expected empty reason for usable directory, got %q", result.Reason)
	}
}

func TestValidateDirectoryReportsMissingOrInvalidPath(t *testing.T) {
	instance := &Runner{workspace: t.TempDir()}

	missing := instance.ValidateDirectory(filepath.Join(t.TempDir(), "missing-project"))
	if missing.Usable {
		t.Fatalf("expected missing path to be unusable, got %#v", missing)
	}
	if missing.Reason != "path does not exist" {
		t.Fatalf("expected missing-path reason, got %q", missing.Reason)
	}

	filePath := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(filePath, []byte("demo"), 0o644); err != nil {
		t.Fatalf("write file path: %v", err)
	}

	notDirectory := instance.ValidateDirectory(filePath)
	if notDirectory.Usable {
		t.Fatalf("expected file path to be unusable, got %#v", notDirectory)
	}
	if notDirectory.Reason != "path is not a directory" {
		t.Fatalf("expected not-a-directory reason, got %q", notDirectory.Reason)
	}
}

func TestExecutePromptPrefersStderrSummaryOverGenericExitStatus(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script fixture is unix-only")
	}

	workspace := t.TempDir()
	binDir := filepath.Join(workspace, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin dir: %v", err)
	}

	binaryPath := filepath.Join(binDir, "codex")
	script := "#!/bin/sh\n" +
		"echo 'provider validation failed: missing API token' >&2\n" +
		"exit 1\n"
	if err := os.WriteFile(binaryPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake codex binary: %v", err)
	}

	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+originalPath)

	instance := &Runner{workspace: workspace}
	result, err := instance.ExecutePrompt(context.Background(), PromptExecutionRequest{
		ProviderKey: "codex",
		Prompt:      "Test prompt",
	})
	if err != nil {
		t.Fatalf("execute prompt: %v", err)
	}

	if result.Status != "failed" {
		t.Fatalf("expected failed status, got %q", result.Status)
	}
	if result.ExitCode != 1 {
		t.Fatalf("expected exit code 1, got %d", result.ExitCode)
	}
	if result.StderrSummary != "provider validation failed: missing API token" {
		t.Fatalf("expected stderr summary to be captured, got %q", result.StderrSummary)
	}
	if result.ErrorMessage != result.StderrSummary {
		t.Fatalf("expected error message to prefer stderr summary, got %q", result.ErrorMessage)
	}
}

func TestExecutePromptCapturesAbsoluteProviderCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script fixture is unix-only")
	}

	workspace := t.TempDir()
	binDir := filepath.Join(workspace, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin dir: %v", err)
	}

	binaryPath := filepath.Join(binDir, "codex")
	script := "#!/bin/sh\n" +
		"output=''\n" +
		"while [ \"$#\" -gt 0 ]; do\n" +
		"  if [ \"$1\" = \"--output-last-message\" ]; then\n" +
		"    shift\n" +
		"    output=\"$1\"\n" +
		"  fi\n" +
		"  shift\n" +
		"done\n" +
		"cat >/dev/null\n" +
		"printf 'updated artifact' > \"$output\"\n"
	if err := os.WriteFile(binaryPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake codex binary: %v", err)
	}

	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+originalPath)

	instance := &Runner{workspace: workspace}
	result, err := instance.ExecutePrompt(context.Background(), PromptExecutionRequest{
		ProviderKey: "codex",
		ModelName:   "gpt-5.4-mini",
		Prompt:      "Tighten the artifact wording.",
		AllowWrite:  true,
	})
	if err != nil {
		t.Fatalf("execute prompt: %v", err)
	}

	if result.Status != "success" {
		t.Fatalf("expected success status, got %q", result.Status)
	}
	if strings.Contains(result.Command, "< prompt.txt") {
		t.Fatalf("expected absolute prompt path in command, got %q", result.Command)
	}
	if !strings.Contains(result.Command, "--output-last-message") {
		t.Fatalf("expected command to include output flag, got %q", result.Command)
	}
	if !strings.Contains(result.Command, ".flowpilot/runs/") || !strings.Contains(result.Command, "/prompt.txt") {
		t.Fatalf("expected command to point at the real run prompt file, got %q", result.Command)
	}
}

func TestExecutePromptMarksResultFailedWhenProviderReportsMcpFailureCode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script fixture is unix-only")
	}

	workspace := t.TempDir()

	accountHomePath := t.TempDir()
	instance := &Runner{workspace: workspace, secretStore: newMemorySecretStore()}
	writeValidGoogleDriveWorkspaceConfig(t, instance)
	_, err := instance.EnsureGoogleDriveMcpProviderConfig(GoogleDriveMcpProviderConfigRequest{
		ProviderKey:     "codex",
		AccountHomePath: accountHomePath,
		Scope:           "account",
		Mode:            "read_only",
	})
	if err != nil {
		t.Fatalf("ensure provider config: %v", err)
	}

	binDir := filepath.Join(workspace, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin dir: %v", err)
	}

	binaryPath := filepath.Join(binDir, "codex")
	script := "#!/bin/sh\n" +
		"output=''\n" +
		"while [ \"$#\" -gt 0 ]; do\n" +
		"  if [ \"$1\" = \"--output-last-message\" ]; then\n" +
		"    shift\n" +
		"    output=\"$1\"\n" +
		"  fi\n" +
		"  shift\n" +
		"done\n" +
		"cat >/dev/null\n" +
		"printf 'MCP_AUTH_REQUIRED' > \"$output\"\n"
	if err := os.WriteFile(binaryPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake codex binary: %v", err)
	}

	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+originalPath)

	result, err := instance.ExecutePrompt(context.Background(), PromptExecutionRequest{
		ProviderKey:     "codex",
		Prompt:          "Summarize the roadmap doc.",
		RequiredMcps:    []string{"google_drive"},
		AccountHomePath: accountHomePath,
	})
	if err != nil {
		t.Fatalf("execute prompt: %v", err)
	}

	if result.Status != "failed" {
		t.Fatalf("expected failed status, got %q", result.Status)
	}
	if !strings.Contains(result.ErrorMessage, "mcp_auth_required") {
		t.Fatalf("expected MCP failure code in error message, got %q", result.ErrorMessage)
	}
	if !strings.Contains(result.OutputMarkdown, "MCP_AUTH_REQUIRED") {
		t.Fatalf("expected provider output to be captured, got %q", result.OutputMarkdown)
	}
	if !strings.Contains(result.ActualPromptText, "## Required MCP Usage") {
		t.Fatalf("expected injected MCP section in actual prompt, got %q", result.ActualPromptText)
	}
}

func TestResolvePromptExecutionAdapterUsesWorkspaceWriteWhenAllowed(t *testing.T) {
	binary, args, provider, err := resolvePromptExecutionAdapter(
		PromptExecutionRequest{
			ProviderKey: "codex",
			ModelName:   "gpt-5.5",
			AllowWrite:  true,
		},
		"/tmp/output.md",
		t.TempDir(),
	)
	if err != nil {
		t.Fatalf("resolve prompt execution adapter: %v", err)
	}

	if binary != "codex" {
		t.Fatalf("expected codex binary, got %q", binary)
	}
	if provider != "codex" {
		t.Fatalf("expected codex provider, got %q", provider)
	}
	if !slices.Equal(args[:3], []string{"--sandbox", "workspace-write", "exec"}) {
		t.Fatalf("expected workspace-write sandbox, got %v", args)
	}
}

func TestResolvePromptExecutionAdapterMapsModelNames(t *testing.T) {
	tests := []struct {
		provider      string
		model         string
		expectedBin   string
		expectedModel string
	}{
		{"claude", "claude-sonnet", "claude", "sonnet"},
		{"claude", "claude-opus", "claude", "opus"},
		{"claude", "sonnet", "claude", "sonnet"},
		{"gemini", "gemini-pro", "agy", "gemini-3.1-pro-high"},
		{"gemini", "gemini-flash", "agy", "gemini-3.5-flash-medium"},
		{"gemini", "flash", "agy", "gemini-3.5-flash-medium"},
		{"codex", "gpt-5.4", "codex", "gpt-5.4"},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("%s/%s", tc.provider, tc.model), func(t *testing.T) {
			workspace := t.TempDir()
			accountHome := t.TempDir()
			binary, args, _, err := resolvePromptExecutionAdapter(
				PromptExecutionRequest{
					ProviderKey:     tc.provider,
					ModelName:       tc.model,
					AccountHomePath: accountHome,
					CustomEnv:       map[string]string{"GEMINI_HOME": filepath.Join(accountHome, ".gemini")},
				},
				"/tmp/output.md",
				workspace,
			)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if binary != tc.expectedBin {
				t.Fatalf("expected binary %q, got %q", tc.expectedBin, binary)
			}

			found := false
			for i, arg := range args {
				if tc.provider == "codex" && arg == "-c" {
					if i+1 >= len(args) {
						t.Fatalf("missing value after -c flag")
					}
					if args[i+1] != fmt.Sprintf("model=%q", tc.expectedModel) {
						continue
					}
					found = true
					break
				}
				if arg == "--model" {
					if i+1 >= len(args) {
						t.Fatalf("missing value after --model flag")
					}
					if args[i+1] != tc.expectedModel {
						t.Fatalf("expected model value %q, got %q", tc.expectedModel, args[i+1])
					}
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("expected model flag in args: %v", args)
			}
		})
	}
}

func TestResolvePromptExecutionAdapterGeminiOmitsNewProjectAfterConfigBootstrap(t *testing.T) {
	workspace := t.TempDir()
	home := t.TempDir()

	binary, args, provider, err := resolvePromptExecutionAdapter(
		PromptExecutionRequest{
			ProviderKey:     "gemini",
			ModelName:       "gemini-flash",
			AccountHomePath: home,
			CustomEnv:       map[string]string{"GEMINI_HOME": filepath.Join(home, ".gemini")},
		},
		"/tmp/output.md",
		workspace,
	)
	if err != nil {
		t.Fatalf("resolve prompt execution adapter: %v", err)
	}
	if binary != "agy" || provider != "gemini" {
		t.Fatalf("adapter = (%q, %q), want (agy, gemini)", binary, provider)
	}

	got := strings.Join(args, " ")
	if !strings.Contains(got, "--project ") {
		t.Fatalf("gemini args = %q, want bootstrapped project", got)
	}
	if strings.Contains(got, "--new-project") {
		t.Fatalf("gemini args = %q, should not pass --new-project after bootstrapping config", got)
	}
	if slices.Contains(args, "--continue") {
		t.Fatalf("gemini args = %q, should not resume a newly bootstrapped project", got)
	}
}

func TestResolvePromptExecutionAdapterReasoningEffort(t *testing.T) {
	tests := []struct {
		provider      string
		model         string
		effort        string
		expectedBin   string
		expectedFlags []string
	}{
		{"codex", "gpt-5.4", "high", "codex", []string{"-c", "model_reasoning_effort=high"}},
		{"codex", "gpt-5.5", "low", "codex", []string{"-c", "model_reasoning_effort=low"}},
		{"claude", "claude-sonnet", "high", "claude", []string{"--effort", "high"}},
		{"claude", "claude-opus", "xhigh", "claude", []string{"--effort", "xhigh"}},
		{"claude", "claude-opus", "max", "claude", []string{"--effort", "max"}},
		{"gemini", "gemini-pro", "high", "agy", []string{}}, // gemini doesn't append reasoning flags
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("%s/%s/%s", tc.provider, tc.model, tc.effort), func(t *testing.T) {
			workspace := t.TempDir()
			accountHome := t.TempDir()
			binary, args, _, err := resolvePromptExecutionAdapter(
				PromptExecutionRequest{
					ProviderKey:     tc.provider,
					ModelName:       tc.model,
					ReasoningEffort: tc.effort,
					AccountHomePath: accountHome,
					CustomEnv:       map[string]string{"GEMINI_HOME": filepath.Join(accountHome, ".gemini")},
				},
				"/tmp/output.md",
				workspace,
			)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if binary != tc.expectedBin {
				t.Fatalf("expected binary %q, got %q", tc.expectedBin, binary)
			}

			for i := 0; i < len(tc.expectedFlags); i += 2 {
				flag := tc.expectedFlags[i]
				val := tc.expectedFlags[i+1]
				found := false
				for idx, arg := range args {
					if arg != flag {
						continue
					}
					if idx+1 >= len(args) {
						t.Fatalf("missing value after flag %s", flag)
					}
					if args[idx+1] != val {
						continue
					}
					found = true
					break
				}
				if !found {
					t.Fatalf("expected flag %s with value %s in args: %v", flag, val, args)
				}
			}
		})
	}
}

func TestReadArtifactDetailLoadsPromptAndDiagnostics(t *testing.T) {
	instance := &Runner{workspace: t.TempDir()}
	artifactDir := filepath.Join(instance.workspace, ".flowpilot", "artifacts", "project", "feature", "run", "step", "artifact")
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		t.Fatalf("mkdir artifact dir: %v", err)
	}

	files := map[string]string{
		filepath.Join(artifactDir, "content.md"):       "Generated business idea",
		filepath.Join(artifactDir, "prompt.md"):        "Prompt used for the run",
		filepath.Join(artifactDir, "actual-prompt.md"): "Bootstrap replay prompt sent to the provider",
		filepath.Join(artifactDir, "stdout.txt"):       "stdout summary",
		filepath.Join(artifactDir, "stderr.txt"):       "stderr summary",
		filepath.Join(artifactDir, "command.txt"):      "codex --sandbox workspace-write exec",
	}
	for path, contents := range files {
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatalf("write fixture %s: %v", path, err)
		}
	}

	manifestPath := filepath.Join(artifactDir, "manifest.json")
	manifestLocalPath := filepath.ToSlash(artifactDir)
	manifest := `{
  "artifactId": "artifact-1",
  "title": "Business Idea",
  "sourceKind": "workflow_output",
  "projectId": "project",
  "featureId": "feature",
  "workflowRunId": "run",
  "workflowStepKey": "business_idea",
  "providerKey": "codex",
  "localPath": "` + manifestLocalPath + `",
  "remotePath": "",
  "remoteUrl": "",
  "syncStatus": "local_only",
  "createdAt": "2026-05-23T00:00:00Z",
  "updatedAt": "2026-05-23T00:00:01Z"
}`
	if err := os.WriteFile(manifestPath, []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	detail, err := instance.readArtifactDetail(manifestPath)
	if err != nil {
		t.Fatalf("read artifact detail: %v", err)
	}

	if detail.ContentMarkdown != "Generated business idea" {
		t.Fatalf("expected content markdown, got %q", detail.ContentMarkdown)
	}
	if detail.PromptText != "Prompt used for the run" {
		t.Fatalf("expected prompt text, got %q", detail.PromptText)
	}
	if detail.ActualPromptText != "Bootstrap replay prompt sent to the provider" {
		t.Fatalf("expected actual prompt text, got %q", detail.ActualPromptText)
	}
	if detail.StdoutText != "stdout summary" {
		t.Fatalf("expected stdout text, got %q", detail.StdoutText)
	}
	if detail.StderrText != "stderr summary" {
		t.Fatalf("expected stderr text, got %q", detail.StderrText)
	}
	if detail.CommandText != "codex --sandbox workspace-write exec" {
		t.Fatalf("expected command text, got %q", detail.CommandText)
	}
}

func TestTriggerIntegrationConnectionUsesJiraApiToken(t *testing.T) {
	instance := &Runner{
		workspace:   t.TempDir(),
		secretStore: newMemorySecretStore(),
	}

	originalRequest := executeJiraRequestFn
	t.Cleanup(func() {
		executeJiraRequestFn = originalRequest
	})

	var observedEndpoint string
	var observedMethod string
	var observedCreds jiraCredential
	executeJiraRequestFn = func(ctx context.Context, method string, endpoint string, creds jiraCredential, payload []byte) ([]byte, error) {
		observedEndpoint = endpoint
		observedMethod = method
		observedCreds = creds
		return []byte(`{"id":"10000","key":"SCRUM"}`), nil
	}

	result, err := instance.TriggerIntegrationConnection(context.Background(), "integration-jira", IntegrationConnectionRequest{
		ProjectID:    "project-alpha",
		ProviderType: "jira",
		Action:       "test",
		WorkspaceURL: "https://flowpilot899.atlassian.net",
		ProjectKey:   "SCRUM",
		BoardID:      "1",
		Email:        "name@company.com",
		ApiToken:     "secret-token",
	})
	if err != nil {
		t.Fatalf("trigger Jira integration connection: %v", err)
	}

	if result.RequestStatus != "accepted" {
		t.Fatalf("expected accepted request status, got %q", result.RequestStatus)
	}
	if result.IntegrationStatus != "connected" {
		t.Fatalf("expected connected integration status, got %q", result.IntegrationStatus)
	}
	if observedEndpoint != "https://flowpilot899.atlassian.net/rest/api/3/project/SCRUM" {
		t.Fatalf("expected Jira project verification endpoint, got %q", observedEndpoint)
	}
	if observedMethod != http.MethodGet {
		t.Fatalf("expected Jira verify GET request, got %q", observedMethod)
	}
	if observedCreds.Email != "name@company.com" || observedCreds.ApiToken != "secret-token" {
		t.Fatalf("expected stored Jira credentials to be used, got %#v", observedCreds)
	}

	backends, err := instance.ListMcpBackends(context.Background())
	if err != nil {
		t.Fatalf("list MCP backends after Jira connect: %v", err)
	}
	for _, backend := range backends {
		if backend.ProviderType != "jira" {
			continue
		}
		if !backend.Installed {
			t.Fatal("expected Jira backend to be marked installed after connect")
		}
		if backend.State != "installed" {
			t.Fatalf("expected Jira backend state installed, got %q", backend.State)
		}
		if backend.Action != "verify" {
			t.Fatalf("expected Jira backend action verify, got %q", backend.Action)
		}
		break
	}

	verified, err := instance.VerifyMcpBackend(
		context.Background(),
		"jira",
		"project-alpha",
		"integration-jira",
	)
	if err != nil {
		t.Fatalf("verify Jira MCP backend: %v", err)
	}
	if !verified.Installed {
		t.Fatal("expected Jira backend to remain installed after verify")
	}
	if verified.Action != "verify" {
		t.Fatalf("expected Jira backend action verify, got %q", verified.Action)
	}
}

func TestVerifyMcpBackendUsesRequestedJiraIntegrationScope(t *testing.T) {
	instance := &Runner{
		workspace:   t.TempDir(),
		secretStore: newMemorySecretStore(),
	}

	if err := instance.saveJiraCredential("integration-legacy", jiraCredential{
		ProjectID:    "project-legacy",
		Email:        "legacy@company.com",
		ApiToken:     "legacy-token",
		WorkspaceURL: "https://flowpilot899.atlassian.net",
		ProjectKey:   "LEGACY",
	}); err != nil {
		t.Fatalf("save legacy jira credential: %v", err)
	}
	if err := instance.saveJiraCredential("integration-target", jiraCredential{
		ProjectID:    "project-alpha",
		Email:        "target@company.com",
		ApiToken:     "target-token",
		WorkspaceURL: "https://flowpilot899.atlassian.net",
		ProjectKey:   "SCRUM",
	}); err != nil {
		t.Fatalf("save target jira credential: %v", err)
	}
	if err := instance.saveMcpBackendRecord(McpBackend{
		Key:       "jira",
		SecretKey: jiraCredentialKey("integration-legacy"),
	}); err != nil {
		t.Fatalf("save jira backend record: %v", err)
	}

	originalRequest := executeJiraRequestFn
	t.Cleanup(func() {
		executeJiraRequestFn = originalRequest
	})

	var observedCreds jiraCredential
	executeJiraRequestFn = func(ctx context.Context, method string, endpoint string, creds jiraCredential, payload []byte) ([]byte, error) {
		observedCreds = creds
		return []byte(`{"id":"10000","key":"SCRUM"}`), nil
	}

	backend, err := instance.VerifyMcpBackend(
		context.Background(),
		"jira",
		"project-alpha",
		"integration-target",
	)
	if err != nil {
		t.Fatalf("verify jira backend with scoped integration: %v", err)
	}

	if observedCreds.Email != "target@company.com" || observedCreds.ApiToken != "target-token" {
		t.Fatalf("expected verify to use requested integration credential, got %#v", observedCreds)
	}
	if backend.SecretKey != jiraCredentialKey("integration-target") {
		t.Fatalf("expected backend secret key to switch to target integration, got %q", backend.SecretKey)
	}
}

func TestTriggerIntegrationConnectionFailsWhenRequiredBackendLauncherIsMissing(t *testing.T) {
	t.Setenv("PATH", "")

	instance := &Runner{workspace: t.TempDir()}

	result, err := instance.TriggerIntegrationConnection(context.Background(), "integration-1", IntegrationConnectionRequest{
		ProjectID:    "project-alpha",
		ProviderType: "google_drive",
		Action:       "test",
	})
	if err != nil {
		t.Fatalf("trigger integration connection: %v", err)
	}

	if result.RequestStatus != "rejected" {
		t.Fatalf("expected rejected request status, got %q", result.RequestStatus)
	}
	if result.IntegrationStatus != "failed" {
		t.Fatalf("expected failed integration status, got %q", result.IntegrationStatus)
	}
	if result.Message == nil || !strings.Contains(strings.ToLower(*result.Message), "install") {
		t.Fatalf("expected install hint in message, got %#v", result.Message)
	}
}

func TestTriggerIntegrationConnectionRejectsInvalidInput(t *testing.T) {
	instance := &Runner{workspace: t.TempDir()}

	_, err := instance.TriggerIntegrationConnection(context.Background(), "", IntegrationConnectionRequest{
		ProjectID:    "project-alpha",
		ProviderType: "google_drive",
		Action:       "test",
	})
	if err == nil {
		t.Fatal("expected integration id validation error")
	}

	_, err = instance.TriggerIntegrationConnection(context.Background(), "integration-1", IntegrationConnectionRequest{
		ProjectID:    "",
		ProviderType: "google_drive",
		Action:       "test",
	})
	if err == nil {
		t.Fatal("expected project id validation error")
	}

	_, err = instance.TriggerIntegrationConnection(context.Background(), "integration-1", IntegrationConnectionRequest{
		ProjectID:    "project-alpha",
		ProviderType: "google_drive",
		Action:       "invalid",
	})
	if err == nil {
		t.Fatal("expected action validation error")
	}
}

func TestListMcpBackendsReportsMissingLaunchers(t *testing.T) {
	t.Setenv("PATH", "")

	instance := &Runner{workspace: t.TempDir()}
	backends, err := instance.ListMcpBackends(context.Background())
	if err != nil {
		t.Fatalf("list MCP backends: %v", err)
	}

	if len(backends) == 0 {
		t.Fatal("expected allowlisted MCP backends")
	}

	for _, backend := range backends {
		if backend.Installed {
			t.Fatalf("expected backend %q to be missing when PATH is empty", backend.Key)
		}
		if backend.LastError == "" {
			t.Fatalf("expected backend %q to report a missing launcher or credential error", backend.Key)
		}
	}
}

func TestListMcpBackendsDetectsAllowlistedBackends(t *testing.T) {
	originalLookPath := lookPathFn
	t.Cleanup(func() {
		lookPathFn = originalLookPath
	})

	lookPathFn = func(file string) (string, error) {
		if file == "npx" {
			return "/usr/bin/npx", nil
		}
		return "", errors.New("launcher not found")
	}
	instance := &Runner{workspace: t.TempDir()}
	backends, err := instance.ListMcpBackends(context.Background())
	if err != nil {
		t.Fatalf("list MCP backends: %v", err)
	}

	if len(backends) < 2 {
		t.Fatalf("expected allowlisted MCP backends, got %d", len(backends))
	}

	for _, backend := range backends {
		switch backend.ProviderType {
		case "google_drive":
			if backend.Installed {
				t.Fatalf("expected backend %q to be launcher-available but not installed before any backend action", backend.Key)
			}
			if backend.State != "launcher_available" {
				t.Fatalf("expected backend %q to report launcher_available state, got %q", backend.Key, backend.State)
			}
			if backend.Action != "install" {
				t.Fatalf("expected backend %q to default to install action before setup, got %q", backend.Key, backend.Action)
			}
		case "jira":
			if backend.Installed {
				t.Fatalf("expected Jira backend %q to be missing before token setup", backend.Key)
			}
			if backend.State != "missing" {
				t.Fatalf("expected Jira backend %q to report missing state, got %q", backend.Key, backend.State)
			}
			if backend.ActionLabel != "Create MCP" {
				t.Fatalf("expected Jira backend %q to default to Create MCP action before setup, got %q", backend.Key, backend.ActionLabel)
			}
		}
	}
}

func TestInstallMcpBackendRunsExplicitCommandForGoogleDrive(t *testing.T) {
	originalLookPath := lookPathFn
	originalRunCommandWithEnv := runCommandWithEnvFn
	t.Cleanup(func() {
		lookPathFn = originalLookPath
		runCommandWithEnvFn = originalRunCommandWithEnv
	})

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)
	t.Setenv("XDG_CONFIG_HOME", "")
	credentialDir := filepath.Join(homeDir, ".config", "google-drive-mcp")
	if err := os.MkdirAll(credentialDir, 0o755); err != nil {
		t.Fatalf("create google drive config dir: %v", err)
	}
	credentialPath := filepath.Join(credentialDir, "gcp-oauth.keys.json")
	credentialJSON := []byte(`{"installed":{"client_id":"client-id","client_secret":"client-secret","auth_uri":"https://accounts.google.com/o/oauth2/auth","token_uri":"https://oauth2.googleapis.com/token"}}`)
	if err := os.WriteFile(credentialPath, credentialJSON, 0o600); err != nil {
		t.Fatalf("write credential file: %v", err)
	}

	lookPathFn = func(file string) (string, error) {
		if file == "npx" {
			return "/usr/bin/npx", nil
		}
		return "", errors.New("launcher not found")
	}
	commandArgs := make([][]string, 0, 2)
	commandEnvs := make([][]string, 0, 2)
	runCommandWithEnvFn = func(ctx context.Context, name string, env []string, args ...string) ([]byte, error) {
		commandArgs = append(commandArgs, append([]string(nil), args...))
		commandEnvs = append(commandEnvs, append([]string(nil), env...))
		if name != "/usr/bin/npx" {
			t.Fatalf("expected explicit install to use resolved launcher path, got %q", name)
		}
		if len(args) == 0 {
			t.Fatal("expected explicit install to pass backend command args")
		}
		return []byte("ok"), nil
	}

	instance := &Runner{workspace: t.TempDir()}
	backend, err := instance.InstallMcpBackend(context.Background(), "google_drive")
	if err != nil {
		t.Fatalf("install MCP backend: %v", err)
	}
	if !backend.Installed {
		t.Fatal("expected backend to be installed after explicit install")
	}
	if backend.Action != "verify" {
		t.Fatalf("expected backend action to switch to verify, got %q", backend.Action)
	}
	if len(commandArgs) != 1 {
		t.Fatalf("expected one explicit install command, got %d", len(commandArgs))
	}
	if want := []string{"-y", "@piotr-agier/google-drive-mcp", "--version"}; !slices.Equal(commandArgs[0], want) {
		t.Fatalf("expected install args %v, got %v", want, commandArgs[0])
	}
	if len(commandEnvs) != 1 {
		t.Fatalf("expected one explicit install environment, got %d", len(commandEnvs))
	}
	if !containsEnvValue(commandEnvs[0], "GOOGLE_DRIVE_OAUTH_CREDENTIALS="+credentialPath) {
		t.Fatalf("expected install env to include credential path, got %v", commandEnvs[0])
	}
	if !containsEnvValue(commandEnvs[0], "GOOGLE_DRIVE_MCP_TOKEN_PATH="+filepath.Join(homeDir, ".config", "google-drive-mcp", "tokens.json")) {
		t.Fatalf("expected install env to include token path, got %v", commandEnvs[0])
	}

	verified, err := instance.VerifyMcpBackend(context.Background(), "google_drive", "", "")
	if err != nil {
		t.Fatalf("verify MCP backend: %v", err)
	}
	if !verified.Installed {
		t.Fatal("expected backend to remain installed after explicit verify")
	}
	if verified.Action != "verify" {
		t.Fatalf("expected backend action to stay verify after verification, got %q", verified.Action)
	}
	if len(commandArgs) != 2 {
		t.Fatalf("expected install and verify commands to both run, got %d", len(commandArgs))
	}
	if want := []string{"-y", "@piotr-agier/google-drive-mcp", "--help"}; !slices.Equal(commandArgs[1], want) {
		t.Fatalf("expected verify args %v, got %v", want, commandArgs[1])
	}
	if len(commandEnvs) != 2 {
		t.Fatalf("expected install and verify environments to both run, got %d", len(commandEnvs))
	}
	if !containsEnvValue(commandEnvs[1], "GOOGLE_DRIVE_OAUTH_CREDENTIALS="+credentialPath) {
		t.Fatalf("expected verify env to include credential path, got %v", commandEnvs[1])
	}

	backends, err := instance.ListMcpBackends(context.Background())
	if err != nil {
		t.Fatalf("list MCP backends after install: %v", err)
	}

	for _, listed := range backends {
		if listed.Key != "google_drive" {
			continue
		}
		if !listed.Installed {
			t.Fatal("expected installed backend state to persist for google_drive")
		}
		if listed.State != "installed" {
			t.Fatalf("expected persisted state to be installed, got %q", listed.State)
		}
		if listed.Action != "verify" {
			t.Fatalf("expected persisted action to be verify, got %q", listed.Action)
		}
		return
	}

	t.Fatal("expected google_drive backend to be present after install")
}

func TestStartGoogleDriveMcpAuthLaunchesTerminalWithManagedPaths(t *testing.T) {
	originalLookPath := lookPathFn
	originalLaunchTerminalCommandWithEnv := launchTerminalCommandWithEnvFn
	t.Cleanup(func() {
		lookPathFn = originalLookPath
		launchTerminalCommandWithEnvFn = originalLaunchTerminalCommandWithEnv
	})

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)
	t.Setenv("XDG_CONFIG_HOME", "")
	credentialDir := filepath.Join(homeDir, ".config", "google-drive-mcp")
	if err := os.MkdirAll(credentialDir, 0o755); err != nil {
		t.Fatalf("create google drive config dir: %v", err)
	}
	credentialPath := filepath.Join(credentialDir, "gcp-oauth.keys.json")
	credentialJSON := []byte(`{"installed":{"client_id":"client-id","client_secret":"client-secret","auth_uri":"https://accounts.google.com/o/oauth2/auth","token_uri":"https://oauth2.googleapis.com/token"}}`)
	if err := os.WriteFile(credentialPath, credentialJSON, 0o600); err != nil {
		t.Fatalf("write credential file: %v", err)
	}

	lookPathFn = func(file string) (string, error) {
		if file == "npx" {
			return "/usr/bin/npx", nil
		}
		return "", errors.New("launcher not found")
	}

	var observedEnv map[string]string
	var observedCommand string
	var observedKeepShellOpen bool
	launchTerminalCommandWithEnvFn = func(env map[string]string, command string, keepShellOpen bool) error {
		observedEnv = env
		observedCommand = command
		observedKeepShellOpen = keepShellOpen
		return nil
	}

	instance := &Runner{workspace: t.TempDir()}
	if err := instance.StartGoogleDriveMcpAuth(); err != nil {
		t.Fatalf("start google drive MCP auth: %v", err)
	}

	if !observedKeepShellOpen {
		t.Fatal("expected auth terminal to stay open")
	}
	if !strings.Contains(observedCommand, "@piotr-agier/google-drive-mcp auth") {
		t.Fatalf("expected auth command to run google-drive-mcp auth, got %q", observedCommand)
	}
	if !strings.Contains(observedCommand, "npx") {
		t.Fatalf("expected auth command to use npx launcher, got %q", observedCommand)
	}
	if observedEnv["GOOGLE_DRIVE_OAUTH_CREDENTIALS"] != credentialPath {
		t.Fatalf("expected credential env to match uploaded path, got %#v", observedEnv)
	}
	if observedEnv["GOOGLE_DRIVE_MCP_TOKEN_PATH"] != filepath.Join(homeDir, ".config", "google-drive-mcp", "tokens.json") {
		t.Fatalf("expected token env to use managed token path, got %#v", observedEnv)
	}
}

func TestStartInteractiveAuthLaunchesFromWorkspace(t *testing.T) {
	originalLookPath := lookPathFn
	originalLaunchProviderTerminalCommand := launchProviderTerminalCommandFn
	t.Cleanup(func() {
		lookPathFn = originalLookPath
		launchProviderTerminalCommandFn = originalLaunchProviderTerminalCommand
	})

	workspace := filepath.Join(t.TempDir(), "flowpilot")
	instance := &Runner{workspace: workspace}

	lookPathFn = func(file string) (string, error) {
		if file != "agy" {
			t.Fatalf("expected agy lookup, got %q", file)
		}
		return "/usr/local/bin/agy", nil
	}

	var observedProviderKey string
	var observedAccountHomePath string
	var observedCommand string
	var observedKeepShellOpen bool
	launchProviderTerminalCommandFn = func(providerKey, accountHomePath, command string, keepShellOpen bool) error {
		observedProviderKey = providerKey
		observedAccountHomePath = accountHomePath
		observedCommand = command
		observedKeepShellOpen = keepShellOpen
		return nil
	}

	if err := instance.StartInteractiveAuth("gemini", "/tmp/.geminiHome1"); err != nil {
		t.Fatalf("start interactive auth: %v", err)
	}

	if observedProviderKey != "gemini" {
		t.Fatalf("expected provider key gemini, got %q", observedProviderKey)
	}
	if observedAccountHomePath != "/tmp/.geminiHome1" {
		t.Fatalf("expected account home path to round-trip, got %q", observedAccountHomePath)
	}
	if !observedKeepShellOpen {
		t.Fatal("expected auth terminal to stay open")
	}

	expectedPrefix := commandWithWorkingDirectory("/usr/local/bin/agy", workspace)
	if observedCommand != expectedPrefix {
		t.Fatalf("expected workspace-scoped auth command %q, got %q", expectedPrefix, observedCommand)
	}
}

func TestDeleteIntegrationConnectionRemovesJiraSecret(t *testing.T) {
	instance := &Runner{
		workspace:   t.TempDir(),
		secretStore: newMemorySecretStore(),
	}

	originalProbe := probeRemoteBackendFn
	t.Cleanup(func() {
		probeRemoteBackendFn = originalProbe
	})

	probeRemoteBackendFn = func(ctx context.Context, endpoint string, headers map[string]string) error {
		return nil
	}

	_, err := instance.TriggerIntegrationConnection(context.Background(), "integration-jira", IntegrationConnectionRequest{
		ProjectID:    "project-alpha",
		ProviderType: "jira",
		Action:       "test",
		WorkspaceURL: "https://flowpilot899.atlassian.net",
		ProjectKey:   "SCRUM",
		Email:        "name@company.com",
		ApiToken:     "secret-token",
	})
	if err != nil {
		t.Fatalf("prime Jira connection: %v", err)
	}

	if err := instance.DeleteIntegrationConnection(context.Background(), "integration-jira"); err != nil {
		t.Fatalf("delete integration connection: %v", err)
	}

	if _, err := instance.ensureSecretStore().Get(jiraCredentialKey("integration-jira")); err == nil {
		t.Fatal("expected Jira secret to be removed from keyring store")
	}

	backends, err := instance.ListMcpBackends(context.Background())
	if err != nil {
		t.Fatalf("list MCP backends after delete: %v", err)
	}
	for _, backend := range backends {
		if backend.ProviderType != "jira" {
			continue
		}
		if backend.Installed {
			t.Fatal("expected Jira backend to be missing after delete")
		}
		if backend.State != "missing" {
			t.Fatalf("expected Jira backend state missing after delete, got %q", backend.State)
		}
		return
	}

	t.Fatal("expected Jira backend to be present after delete")
}

func TestDeleteIntegrationConnectionSucceedsWhenLocalJiraSecretIsAlreadyMissing(t *testing.T) {
	instance := &Runner{
		workspace:   t.TempDir(),
		secretStore: newMemorySecretStore(),
	}

	if err := instance.saveMcpBackendRecord(McpBackend{
		Key:           "jira",
		ProviderType:  "jira",
		Label:         "Atlassian MCP",
		Transport:     "remote",
		Launcher:      "remote",
		InstallHint:   "Open the Jira MCP form, read the Atlassian guide, and paste an API token so the runner can connect to the Atlassian remote MCP server.",
		State:         "installed",
		Action:        "verify",
		ActionLabel:   "Verify",
		LastCheckedAt: "2026-05-19T08:00:00.000Z",
		SecretKey:     jiraCredentialKey("integration-jira"),
	}); err != nil {
		t.Fatalf("save jira backend record: %v", err)
	}

	if err := instance.DeleteIntegrationConnection(context.Background(), "integration-jira"); err != nil {
		t.Fatalf("delete integration connection without local secret: %v", err)
	}

	backends, err := instance.ListMcpBackends(context.Background())
	if err != nil {
		t.Fatalf("list MCP backends after delete: %v", err)
	}
	for _, backend := range backends {
		if backend.ProviderType != "jira" {
			continue
		}
		if backend.Installed {
			t.Fatal("expected Jira backend to be missing after delete")
		}
		if backend.State != "missing" {
			t.Fatalf("expected Jira backend state missing after delete, got %q", backend.State)
		}
		return
	}

	t.Fatal("expected Jira backend to be present after delete")
}

func TestInstallMcpBackendRejectsUnsupportedBackend(t *testing.T) {
	instance := &Runner{workspace: t.TempDir()}

	_, err := instance.InstallMcpBackend(context.Background(), "unsupported")
	if err == nil {
		t.Fatal("expected unsupported backend error")
	}
}

func TestRunMcpTestRejectsInvalidInput(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}

	_, err := instance.RunMcpTest(context.Background(), McpTestRequest{})
	if err == nil || !strings.Contains(err.Error(), "backendKey") {
		t.Fatalf("expected backendKey validation error, got %v", err)
	}

	_, err = instance.RunMcpTest(context.Background(), McpTestRequest{
		BackendKey:    "jira",
		ProviderType:  "google_drive",
		ProjectID:     "project-1",
		IntegrationID: "integration-1",
		Prompt:        "List open bugs.",
	})
	if err == nil || !strings.Contains(err.Error(), "does not match backend") {
		t.Fatalf("expected provider mismatch error, got %v", err)
	}
}

func TestRunMcpTestUsesStoredJiraCredentialAndWritesArtifacts(t *testing.T) {
	instance := &Runner{
		workspace:   t.TempDir(),
		secretStore: newMemorySecretStore(),
	}

	if err := instance.saveJiraCredential("integration-jira", jiraCredential{
		ProjectID:    "project-alpha",
		Email:        "name@company.com",
		ApiToken:     "secret-token",
		WorkspaceURL: "https://flowpilot899.atlassian.net",
		ProjectKey:   "SCRUM",
		BoardID:      "1",
	}); err != nil {
		t.Fatalf("save jira credential: %v", err)
	}

	originalProbe := probeRemoteBackendFn
	originalRequest := executeJiraRequestFn
	t.Cleanup(func() {
		probeRemoteBackendFn = originalProbe
		executeJiraRequestFn = originalRequest
	})

	probeRemoteBackendFn = func(ctx context.Context, endpoint string, headers map[string]string) error {
		return nil
	}
	executeJiraRequestFn = func(ctx context.Context, method string, endpoint string, creds jiraCredential, payload []byte) ([]byte, error) {
		if method != "GET" {
			t.Fatalf("expected Jira search GET request, got %q", method)
		}
		if !strings.Contains(endpoint, "/rest/api/3/search/jql") {
			t.Fatalf("expected Jira search endpoint, got %q", endpoint)
		}
		if creds.Email != "name@company.com" || creds.ApiToken != "secret-token" {
			t.Fatalf("expected stored Jira credential to be used, got %#v", creds)
		}
		return []byte(`{"issues":[{"key":"FLOW-101","fields":{"summary":"Login fails on mobile","status":{"name":"In Progress"},"assignee":{"displayName":"Taylor"}}}]}`), nil
	}

	result, err := instance.RunMcpTest(context.Background(), McpTestRequest{
		BackendKey:    "jira",
		ProviderType:  "jira",
		ProjectID:     "project-alpha",
		IntegrationID: "integration-jira",
		TemplateKey:   "jira_find_open_bugs",
		AllowWrite:    false,
		Prompt:        "Find all open bugs in Project Alpha.",
		TimeoutMs:     1000,
	})
	if err != nil {
		t.Fatalf("run MCP test: %v", err)
	}

	if result.Status != "success" {
		t.Fatalf("expected success status, got %q", result.Status)
	}
	if result.RunID == "" || !strings.HasPrefix(result.RunID, "mcp_") {
		t.Fatalf("expected MCP run id, got %q", result.RunID)
	}
	if !strings.Contains(result.Command, "/rest/api/3/search/jql") {
		t.Fatalf("unexpected command %q", result.Command)
	}
	if !strings.Contains(result.StdoutSummary, "Found 1 open bug issues") {
		t.Fatalf("expected stdout summary, got %q", result.StdoutSummary)
	}
	if !strings.Contains(result.OutputMarkdown, "FLOW-101") {
		t.Fatalf("expected prompt in output markdown, got %q", result.OutputMarkdown)
	}
	if len(result.ArtifactPaths) != 5 {
		t.Fatalf("expected five artifact paths, got %d", len(result.ArtifactPaths))
	}

	for _, path := range result.ArtifactPaths {
		if _, statErr := os.Stat(path); statErr != nil {
			t.Fatalf("expected artifact %q to exist: %v", path, statErr)
		}
	}

	runs, err := instance.ListMcpTestRuns(
		context.Background(),
		"jira",
		"project-alpha",
		"integration-jira",
		10,
	)
	if err != nil {
		t.Fatalf("list MCP test runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected one run summary, got %d", len(runs))
	}
	if runs[0].RunID != result.RunID {
		t.Fatalf("expected run id %q, got %q", result.RunID, runs[0].RunID)
	}
	if runs[0].ProjectID != "project-alpha" {
		t.Fatalf("expected project id to round-trip, got %q", runs[0].ProjectID)
	}
	if runs[0].ArtifactDir != filepath.Dir(result.ArtifactPaths[0]) {
		t.Fatalf("expected artifact dir %q, got %q", filepath.Dir(result.ArtifactPaths[0]), runs[0].ArtifactDir)
	}
}

func TestRunMcpTestListsJiraUserStories(t *testing.T) {
	instance := &Runner{
		workspace:   t.TempDir(),
		secretStore: newMemorySecretStore(),
	}

	if err := instance.saveJiraCredential("integration-jira", jiraCredential{
		ProjectID:    "project-alpha",
		Email:        "name@company.com",
		ApiToken:     "secret-token",
		WorkspaceURL: "https://flowpilot899.atlassian.net",
		ProjectKey:   "SCRUM",
	}); err != nil {
		t.Fatalf("save jira credential: %v", err)
	}

	originalRequest := executeJiraRequestFn
	t.Cleanup(func() {
		executeJiraRequestFn = originalRequest
	})

	executeJiraRequestFn = func(ctx context.Context, method string, endpoint string, creds jiraCredential, payload []byte) ([]byte, error) {
		if method != "GET" {
			t.Fatalf("expected Jira user story GET request, got %q", method)
		}
		if !strings.Contains(endpoint, "issuetype+in+%28Story%2C+%22User+Story%22%29") {
			t.Fatalf("expected Jira user story endpoint, got %q", endpoint)
		}
		return []byte(`{"issues":[{"key":"SCRUM-12","fields":{"summary":"Onboarding wizard","status":{"name":"To Do"},"assignee":{"displayName":"Avery"}}}]}`), nil
	}

	result, err := instance.RunMcpTest(context.Background(), McpTestRequest{
		BackendKey:    "jira",
		ProviderType:  "jira",
		ProjectID:     "project-alpha",
		IntegrationID: "integration-jira",
		TemplateKey:   "jira_list_user_stories",
		Prompt:        "Get a list of user stories in project SCRUM.",
	})
	if err != nil {
		t.Fatalf("run MCP test: %v", err)
	}
	if result.Status != "success" {
		t.Fatalf("expected success status, got %q", result.Status)
	}
	if !strings.Contains(result.StdoutSummary, "Found 1 user stories") {
		t.Fatalf("expected stdout summary, got %q", result.StdoutSummary)
	}
	if !strings.Contains(result.OutputMarkdown, "SCRUM-12") {
		t.Fatalf("expected story key in output markdown, got %q", result.OutputMarkdown)
	}
}

func TestRunMcpTestLoadsJiraTicketContent(t *testing.T) {
	instance := &Runner{
		workspace:   t.TempDir(),
		secretStore: newMemorySecretStore(),
	}

	if err := instance.saveJiraCredential("integration-jira", jiraCredential{
		ProjectID:    "project-alpha",
		Email:        "name@company.com",
		ApiToken:     "secret-token",
		WorkspaceURL: "https://flowpilot899.atlassian.net",
		ProjectKey:   "SCRUM",
	}); err != nil {
		t.Fatalf("save jira credential: %v", err)
	}

	originalRequest := executeJiraRequestFn
	t.Cleanup(func() {
		executeJiraRequestFn = originalRequest
	})

	executeJiraRequestFn = func(ctx context.Context, method string, endpoint string, creds jiraCredential, payload []byte) ([]byte, error) {
		if method != "GET" {
			t.Fatalf("expected Jira issue GET request, got %q", method)
		}
		if !strings.Contains(endpoint, "/rest/api/3/issue/SCRUM-7") {
			t.Fatalf("expected Jira ticket endpoint, got %q", endpoint)
		}
		return []byte(`{"key":"SCRUM-7","fields":{"summary":"Broken login button","status":{"name":"In Progress"},"description":"Investigate mobile web regression","assignee":{"displayName":"Jordan"}}}`), nil
	}

	result, err := instance.RunMcpTest(context.Background(), McpTestRequest{
		BackendKey:    "jira",
		ProviderType:  "jira",
		ProjectID:     "project-alpha",
		IntegrationID: "integration-jira",
		TemplateKey:   "jira_get_ticket_content",
		Prompt:        "Get the content of Jira ticket SCRUM-7.",
	})
	if err != nil {
		t.Fatalf("run MCP test: %v", err)
	}
	if result.Status != "success" {
		t.Fatalf("expected success status, got %q", result.Status)
	}
	if !strings.Contains(result.StdoutSummary, "Loaded Jira ticket SCRUM-7") {
		t.Fatalf("expected stdout summary, got %q", result.StdoutSummary)
	}
	if !strings.Contains(result.OutputMarkdown, "Broken login button") {
		t.Fatalf("expected ticket content in output markdown, got %q", result.OutputMarkdown)
	}
}

func TestRunMcpTestBackfillsLegacyJiraProjectScope(t *testing.T) {
	instance := &Runner{
		workspace:   t.TempDir(),
		secretStore: newMemorySecretStore(),
	}

	if err := instance.saveJiraCredential("integration-jira", jiraCredential{
		Email:        "name@company.com",
		ApiToken:     "secret-token",
		WorkspaceURL: "https://flowpilot899.atlassian.net",
		ProjectKey:   "SCRUM",
	}); err != nil {
		t.Fatalf("save jira credential: %v", err)
	}

	originalRequest := executeJiraRequestFn
	t.Cleanup(func() {
		executeJiraRequestFn = originalRequest
	})

	executeJiraRequestFn = func(ctx context.Context, method string, endpoint string, creds jiraCredential, payload []byte) ([]byte, error) {
		if creds.ProjectID != "project-alpha" {
			t.Fatalf("expected legacy credential to be backfilled with project scope, got %#v", creds)
		}
		return []byte(`{"issues":[]}`), nil
	}

	result, err := instance.RunMcpTest(context.Background(), McpTestRequest{
		BackendKey:    "jira",
		ProviderType:  "jira",
		ProjectID:     "project-alpha",
		IntegrationID: "integration-jira",
		TemplateKey:   "jira_find_open_bugs",
		Prompt:        "Find all open bugs in Project Alpha.",
	})
	if err != nil {
		t.Fatalf("run MCP test: %v", err)
	}
	if result.Status != "success" {
		t.Fatalf("expected success status, got %q", result.Status)
	}

	creds, err := instance.loadJiraCredential(jiraCredentialKey("integration-jira"))
	if err != nil {
		t.Fatalf("load jira credential: %v", err)
	}
	if creds.ProjectID != "project-alpha" {
		t.Fatalf("expected saved credential project id project-alpha, got %q", creds.ProjectID)
	}
}

func TestRunMcpTestCreatesStoryWhenPromptRequestsIt(t *testing.T) {
	instance := &Runner{
		workspace:   t.TempDir(),
		secretStore: newMemorySecretStore(),
	}

	if err := instance.saveJiraCredential("integration-jira", jiraCredential{
		ProjectID:    "project-alpha",
		Email:        "name@company.com",
		ApiToken:     "secret-token",
		WorkspaceURL: "https://flowpilot899.atlassian.net",
		ProjectKey:   "SCRUM",
	}); err != nil {
		t.Fatalf("save jira credential: %v", err)
	}

	originalProbe := probeRemoteBackendFn
	originalRequest := executeJiraRequestFn
	t.Cleanup(func() {
		probeRemoteBackendFn = originalProbe
		executeJiraRequestFn = originalRequest
	})

	probeRemoteBackendFn = func(ctx context.Context, endpoint string, headers map[string]string) error {
		return nil
	}
	executeJiraRequestFn = func(ctx context.Context, method string, endpoint string, creds jiraCredential, payload []byte) ([]byte, error) {
		if method != "POST" {
			t.Fatalf("expected Jira create POST request, got %q", method)
		}
		if !strings.Contains(endpoint, "/rest/api/3/issue") {
			t.Fatalf("expected Jira issue endpoint, got %q", endpoint)
		}
		if !strings.Contains(string(payload), "Redesign onboarding") {
			t.Fatalf("expected create payload to include story title, got %s", string(payload))
		}
		return []byte(`{"key":"SCRUM-42","self":"https://flowpilot899.atlassian.net/rest/api/3/issue/SCRUM-42"}`), nil
	}

	result, err := instance.RunMcpTest(context.Background(), McpTestRequest{
		BackendKey:    "jira",
		ProviderType:  "jira",
		ProjectID:     "project-alpha",
		IntegrationID: "integration-jira",
		TemplateKey:   "jira_create_story",
		AllowWrite:    true,
		Prompt:        "Create a story titled 'Redesign onboarding'.",
		TimeoutMs:     1000,
	})
	if err != nil {
		t.Fatalf("run MCP test: %v", err)
	}

	if result.Status != "success" {
		t.Fatalf("expected success status, got %q", result.Status)
	}
	if !strings.Contains(result.StdoutSummary, "SCRUM-42") {
		t.Fatalf("expected created issue key in stdout, got %q", result.StdoutSummary)
	}
	if !strings.Contains(result.OutputMarkdown, "Redesign onboarding") {
		t.Fatalf("expected story title in markdown, got %q", result.OutputMarkdown)
	}
}

func TestRunMcpTestReturnsFailedResultWhenJiraVerificationFails(t *testing.T) {
	instance := &Runner{
		workspace:   t.TempDir(),
		secretStore: newMemorySecretStore(),
	}

	if err := instance.saveJiraCredential("integration-jira", jiraCredential{
		ProjectID:    "project-alpha",
		Email:        "name@company.com",
		ApiToken:     "secret-token",
		WorkspaceURL: "https://flowpilot899.atlassian.net",
		ProjectKey:   "SCRUM",
	}); err != nil {
		t.Fatalf("save jira credential: %v", err)
	}

	originalProbe := probeRemoteBackendFn
	originalRequest := executeJiraRequestFn
	t.Cleanup(func() {
		probeRemoteBackendFn = originalProbe
		executeJiraRequestFn = originalRequest
	})

	probeRemoteBackendFn = func(ctx context.Context, endpoint string, headers map[string]string) error {
		return nil
	}
	executeJiraRequestFn = func(ctx context.Context, method string, endpoint string, creds jiraCredential, payload []byte) ([]byte, error) {
		return nil, errors.New("remote backend authorization failed: 401 Unauthorized")
	}

	result, err := instance.RunMcpTest(context.Background(), McpTestRequest{
		BackendKey:    "jira",
		ProviderType:  "jira",
		ProjectID:     "project-alpha",
		IntegrationID: "integration-jira",
		TemplateKey:   "jira_create_story",
		AllowWrite:    true,
		Prompt:        "Create a story titled Redesign onboarding.",
	})
	if err != nil {
		t.Fatalf("run MCP test: %v", err)
	}

	if result.Status != "failed" {
		t.Fatalf("expected failed status, got %q", result.Status)
	}
	if !strings.Contains(result.ErrorMessage, "401 Unauthorized") {
		t.Fatalf("expected error message to mention failure, got %q", result.ErrorMessage)
	}
	if !strings.Contains(result.StderrSummary, "401 Unauthorized") {
		t.Fatalf("expected stderr summary to mention failure, got %q", result.StderrSummary)
	}
	if !strings.Contains(result.OutputMarkdown, "could not create the Jira story") {
		t.Fatalf("expected failure markdown, got %q", result.OutputMarkdown)
	}
}

func TestVerifyJiraCredentialRejectsNon2xxStatuses(t *testing.T) {
	originalRequest := executeJiraRequestFn
	t.Cleanup(func() {
		executeJiraRequestFn = originalRequest
	})

	executeJiraRequestFn = func(ctx context.Context, method string, endpoint string, creds jiraCredential, payload []byte) ([]byte, error) {
		return nil, errors.New("jira request failed: 429 rate limited")
	}

	err := (&Runner{}).verifyJiraCredential(context.Background(), jiraCredential{
		Email:        "name@company.com",
		ApiToken:     "secret-token",
		WorkspaceURL: "https://flowpilot899.atlassian.net",
		ProjectKey:   "SCRUM",
	})
	if err == nil || !strings.Contains(err.Error(), "429") {
		t.Fatalf("expected non-2xx verification failure, got %v", err)
	}
}

func TestRunMcpTestRejectsWriteTemplateWithoutAllowWrite(t *testing.T) {
	instance := &Runner{
		workspace:   t.TempDir(),
		secretStore: newMemorySecretStore(),
	}

	if err := instance.saveJiraCredential("integration-jira", jiraCredential{
		ProjectID:    "project-alpha",
		Email:        "name@company.com",
		ApiToken:     "secret-token",
		WorkspaceURL: "https://flowpilot899.atlassian.net",
		ProjectKey:   "SCRUM",
	}); err != nil {
		t.Fatalf("save jira credential: %v", err)
	}

	result, err := instance.RunMcpTest(context.Background(), McpTestRequest{
		BackendKey:    "jira",
		ProviderType:  "jira",
		ProjectID:     "project-alpha",
		IntegrationID: "integration-jira",
		TemplateKey:   "jira_create_story",
		AllowWrite:    false,
		Prompt:        "Create a story titled 'Redesign onboarding'.",
	})
	if err != nil {
		t.Fatalf("run MCP test: %v", err)
	}
	if result.Status != "failed" {
		t.Fatalf("expected failed status, got %q", result.Status)
	}
	if !strings.Contains(result.ErrorMessage, "allowWrite") {
		t.Fatalf("expected allowWrite failure, got %q", result.ErrorMessage)
	}
}

func TestRunMcpTestAllowsProjectLinkReuse(t *testing.T) {
	instance := &Runner{
		workspace:   t.TempDir(),
		secretStore: newMemorySecretStore(),
	}

	if err := instance.saveJiraCredential("integration-jira", jiraCredential{
		ProjectID:    "project-alpha",
		Email:        "name@company.com",
		ApiToken:     "secret-token",
		WorkspaceURL: "https://flowpilot899.atlassian.net",
		ProjectKey:   "SCRUM",
	}); err != nil {
		t.Fatalf("save jira credential: %v", err)
	}

	executeJiraRequestFn = func(
		_ context.Context,
		_ string,
		_ string,
		_ jiraCredential,
		_ []byte,
	) ([]byte, error) {
		return []byte(`{"issues":[]}`), nil
	}
	t.Cleanup(func() {
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
	})

	result, err := instance.RunMcpTest(context.Background(), McpTestRequest{
		BackendKey:    "jira",
		ProviderType:  "jira",
		ProjectID:     "project-beta",
		IntegrationID: "integration-jira",
		TemplateKey:   "jira_find_open_bugs",
		AllowWrite:    false,
		Prompt:        "Find all open bugs in Project Alpha.",
	})
	if err != nil {
		t.Fatalf("run MCP test: %v", err)
	}
	if result.Status != "success" {
		t.Fatalf("expected success status, got %q (%s)", result.Status, result.ErrorMessage)
	}
}

func TestListMcpTestRunsAppliesFilterAndLimit(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}

	baseDir := filepath.Join(instance.workspace, ".flowpilot", "mcp-tests")
	if err := os.MkdirAll(filepath.Join(baseDir, "mcp_20260519_101010_0001"), 0o755); err != nil {
		t.Fatalf("mkdir run dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(baseDir, "mcp_20260519_101011_0002"), 0o755); err != nil {
		t.Fatalf("mkdir run dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(baseDir, "mcp_20260519_101012_0003"), 0o755); err != nil {
		t.Fatalf("mkdir run dir: %v", err)
	}

	writeResult := func(dir string, result McpTestResult) {
		t.Helper()
		if err := writeJSONFile(filepath.Join(dir, "result.json"), result); err != nil {
			t.Fatalf("write result.json: %v", err)
		}
	}

	writeResult(filepath.Join(baseDir, "mcp_20260519_101010_0001"), McpTestResult{
		Status:        "success",
		RunID:         "mcp_20260519_101010_0001",
		BackendKey:    "jira",
		ProviderType:  "jira",
		ProjectID:     "project-alpha",
		IntegrationID: "integration-a",
		StartedAt:     "2026-05-19T10:10:10Z",
		CompletedAt:   "2026-05-19T10:10:11Z",
	})
	writeResult(filepath.Join(baseDir, "mcp_20260519_101011_0002"), McpTestResult{
		Status:        "failed",
		RunID:         "mcp_20260519_101011_0002",
		BackendKey:    "jira",
		ProviderType:  "jira",
		ProjectID:     "project-alpha",
		IntegrationID: "integration-b",
		StartedAt:     "2026-05-19T10:10:11Z",
		CompletedAt:   "2026-05-19T10:10:12Z",
	})
	writeResult(filepath.Join(baseDir, "mcp_20260519_101012_0003"), McpTestResult{
		Status:        "success",
		RunID:         "mcp_20260519_101012_0003",
		BackendKey:    "google_drive",
		ProviderType:  "google_drive",
		ProjectID:     "project-beta",
		IntegrationID: "integration-c",
		StartedAt:     "2026-05-19T10:10:12Z",
		CompletedAt:   "2026-05-19T10:10:13Z",
	})

	runs, err := instance.ListMcpTestRuns(context.Background(), "jira", "project-alpha", "", 1)
	if err != nil {
		t.Fatalf("list MCP test runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected one Jira run after limit, got %d", len(runs))
	}
	if runs[0].RunID != "mcp_20260519_101011_0002" {
		t.Fatalf("expected most recent Jira run, got %q", runs[0].RunID)
	}
	if !strings.HasSuffix(runs[0].ArtifactDir, "mcp_20260519_101011_0002") {
		t.Fatalf("expected artifact dir to point at latest Jira run, got %q", runs[0].ArtifactDir)
	}
}

func TestDetectProvidersPopulatesInventoryShape(t *testing.T) {
	workspace := t.TempDir()
	binDir := filepath.Join(workspace, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin dir: %v", err)
	}

	originalRunCommand := runCommandFn
	t.Cleanup(func() {
		runCommandFn = originalRunCommand
	})
	runCommandFn = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if strings.Contains(strings.ToLower(filepath.Base(name)), "codex") && len(args) == 2 && args[0] == "debug" && args[1] == "models" {
			return []byte(`{"models":[
				{"slug":"gpt-5.5","display_name":"GPT-5.5","visibility":"list","supported_in_api":true},
				{"slug":"gpt-5.4","display_name":"GPT-5.4","visibility":"list","supported_in_api":true},
				{"slug":"gpt-5.4-mini","display_name":"GPT-5.4-Mini","visibility":"list","supported_in_api":true},
				{"slug":"gpt-5.3-codex","display_name":"GPT-5.3-Codex","visibility":"list","supported_in_api":true},
				{"slug":"gpt-5.2","display_name":"GPT-5.2","visibility":"list","supported_in_api":true},
				{"slug":"codex-auto-review","display_name":"Codex Auto Review","visibility":"hide","supported_in_api":true}
			]}`), nil
		}
		if strings.Contains(strings.ToLower(filepath.Base(name)), "agy") && len(args) == 1 && args[0] == "models" {
			return []byte("Gemini 3.5 Flash (Medium)    (current)\nGemini 3.5 Flash (High)\nGemini 3.5 Flash (Low)\nGemini 3.1 Pro (Low)\nGemini 3.1 Pro (High)\nClaude Sonnet 4.6 (Thinking)\nGPT-OSS 120B (Medium)\n"), nil
		}
		return originalRunCommand(ctx, name, args...)
	}

	t.Setenv("PATH", binDir)
	t.Setenv("OPENAI_API_KEY", "test-openai-key")
	t.Setenv("ANTHROPIC_API_KEY", "test-anthropic-key")
	t.Setenv("GEMINI_API_KEY", "test-gemini-key")

	writeMockProviderBinary(t, binDir, "codex", "codex 1.2.3")
	writeMockProviderBinary(t, binDir, "claude", "claude 4.5.6")
	writeMockProviderBinary(t, binDir, "agy", "agy 7.8.9")

	instance := &Runner{workspace: workspace}
	providers, err := instance.DetectProviders(context.Background())
	if err != nil {
		t.Fatalf("detect providers: %v", err)
	}

	raw, err := json.Marshal(struct {
		Providers []Provider `json:"providers"`
	}{Providers: providers})
	if err != nil {
		t.Fatalf("marshal provider inventory: %v", err)
	}

	var payload struct {
		Providers []map[string]any `json:"providers"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal provider inventory: %v", err)
	}

	cases := []struct {
		key     string
		version string
		models  []string
	}{
		{key: "codex", version: "codex 1.2.3", models: []string{"gpt-5.5", "gpt-5.4", "gpt-5.4-mini", "gpt-5.3-codex", "gpt-5.2"}},
		{key: "claude", version: "claude 4.5.6", models: []string{"claude-opus", "claude-sonnet", "claude-haiku"}},
		{
			key:     "gemini",
			version: "agy 7.8.9",
			models: []string{
				"gemini-3.5-flash-medium",
				"gemini-3.5-flash-high",
				"gemini-3.5-flash-low",
				"gemini-3.1-pro-low",
				"gemini-3.1-pro-high",
			},
		},
	}

	// CP-46/Task-210 added a grok providerSpec; this test doesn't mock a grok
	// binary (out of scope for this fixture), so it must appear as a fourth,
	// not-installed entry rather than changing the expectations for the three
	// providers this test does mock.
	if len(payload.Providers) != len(cases)+1 {
		t.Fatalf("expected %d providers in inventory, got %d", len(cases)+1, len(payload.Providers))
	}
	if grok := findProviderJSON(payload.Providers, "grok"); grok == nil {
		t.Fatal("expected a grok entry in the provider inventory")
	} else if grok["install_status"] == "INSTALLED" {
		t.Fatalf("expected grok to be not-installed (no mock binary on PATH), got %#v", grok["install_status"])
	}

	for _, want := range cases {
		provider := findProviderJSON(payload.Providers, want.key)
		if provider == nil {
			t.Fatalf("expected provider %q in inventory", want.key)
		}
		if provider["supported"] != true {
			t.Fatalf("expected provider %q to be supported, got %#v", want.key, provider["supported"])
		}
		if provider["install_status"] != "INSTALLED" {
			t.Fatalf("expected provider %q to be installed, got %#v", want.key, provider["install_status"])
		}
		if provider["auth_status"] != "READY" {
			t.Fatalf("expected provider %q to be ready, got %#v", want.key, provider["auth_status"])
		}
		expectedBinary := want.key
		if want.key == "gemini" {
			expectedBinary = "agy"
		}
		if provider["detected_binary"] != expectedBinary {
			t.Fatalf("expected provider %q binary %q, got %#v", want.key, expectedBinary, provider["detected_binary"])
		}
		if provider["detected_version"] != want.version {
			t.Fatalf("expected provider %q version %q, got %#v", want.key, want.version, provider["detected_version"])
		}
		if _, ok := provider["last_error"]; ok {
			t.Fatalf("expected provider %q to omit last_error when ready", want.key)
		}

		models, ok := provider["models"].([]any)
		if !ok {
			t.Fatalf("expected provider %q models array, got %#v", want.key, provider["models"])
		}
		if len(models) != len(want.models) {
			t.Fatalf("expected provider %q to expose %d models, got %d", want.key, len(want.models), len(models))
		}
		for idx, modelID := range want.models {
			model, ok := models[idx].(map[string]any)
			if !ok {
				t.Fatalf("expected provider %q model %d to be an object, got %#v", want.key, idx, models[idx])
			}
			if model["id"] != modelID {
				t.Fatalf("expected provider %q model %d id %q, got %#v", want.key, idx, modelID, model["id"])
			}
			if model["display_name"] != modelID {
				if want.key != "codex" && want.key != "gemini" {
					t.Fatalf("expected provider %q model %d display name %q, got %#v", want.key, idx, modelID, model["display_name"])
				}
			}
			if model["available"] != true {
				t.Fatalf("expected provider %q model %d to be available, got %#v", want.key, idx, model["available"])
			}
			expectedSource := "registry"
			if want.key == "codex" {
				expectedSource = "codex_debug_models"
			}
			if want.key == "gemini" {
				expectedSource = "agy_models_command"
			}
			if model["source"] != expectedSource {
				t.Fatalf("expected provider %q model %d source %s, got %#v", want.key, idx, expectedSource, model["source"])
			}
		}
	}
}

// TestDetectCodexModelsCapturesReasoningAndContextWindow (Task-215): codex
// debug models already returns default_reasoning_level/
// supported_reasoning_levels/context_window/max_context_window per model
// (live-verified, Codex Build 0.144.1, gpt-5.6-sol) — this asserts
// detectCodexModels maps them onto ProviderModel instead of silently
// dropping them (the pre-Task-215 behavior).
func TestDetectCodexModelsCapturesReasoningAndContextWindow(t *testing.T) {
	originalRunCommand := runCommandFn
	t.Cleanup(func() { runCommandFn = originalRunCommand })
	runCommandFn = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return []byte(`{"models":[
			{"slug":"gpt-5.6-sol","display_name":"GPT-5.6-Sol","visibility":"list","supported_in_api":true,
			 "default_reasoning_level":"low",
			 "supported_reasoning_levels":[
				{"effort":"low","description":"Fast responses with lighter reasoning"},
				{"effort":"medium","description":"Balances speed and reasoning depth for everyday tasks"},
				{"effort":"high","description":"Greater reasoning depth for complex problems"},
				{"effort":"xhigh","description":"Extra high reasoning depth for complex problems"},
				{"effort":"max","description":"Maximum reasoning depth for the hardest problems"},
				{"effort":"ultra","description":"Maximum reasoning with automatic task delegation"}
			 ],
			 "context_window":272000,"max_context_window":1000000,"effective_context_window_percent":95}
		]}`), nil
	}

	models, err := detectCodexModels(context.Background(), "codex")
	if err != nil {
		t.Fatalf("detectCodexModels: %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("expected exactly 1 model, got %d: %+v", len(models), models)
	}
	model := models[0]
	if model.DefaultReasoningEffort != "low" {
		t.Fatalf("expected default_reasoning_effort=low, got %q", model.DefaultReasoningEffort)
	}
	wantEfforts := []string{"low", "medium", "high", "xhigh", "max", "ultra"}
	if !reflect.DeepEqual(model.SupportedReasoningEfforts, wantEfforts) {
		t.Fatalf("expected supported_reasoning_efforts=%v, got %v", wantEfforts, model.SupportedReasoningEfforts)
	}
	if model.ContextWindowTokens != 272000 {
		t.Fatalf("expected context_window_tokens=272000, got %d", model.ContextWindowTokens)
	}
	if model.MaxContextWindowTokens != 1000000 {
		t.Fatalf("expected max_context_window_tokens=1000000, got %d", model.MaxContextWindowTokens)
	}
}

func TestDetectProvidersFallsBackToStaticCodexModelsWhenDebugCatalogFails(t *testing.T) {
	workspace := t.TempDir()
	binDir := filepath.Join(workspace, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin dir: %v", err)
	}

	originalRunCommand := runCommandFn
	t.Cleanup(func() {
		runCommandFn = originalRunCommand
	})
	runCommandFn = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if strings.Contains(strings.ToLower(filepath.Base(name)), "codex") && len(args) == 2 && args[0] == "debug" && args[1] == "models" {
			return nil, errors.New("catalog unavailable")
		}
		return originalRunCommand(ctx, name, args...)
	}

	t.Setenv("PATH", binDir)
	t.Setenv("OPENAI_API_KEY", "test-openai-key")

	writeMockProviderBinary(t, binDir, "codex", "codex 1.2.3")

	instance := &Runner{workspace: workspace}
	providers, err := instance.DetectProviders(context.Background())
	if err != nil {
		t.Fatalf("detect providers: %v", err)
	}

	codex := findProviderInventory(providers, "codex")
	if codex == nil {
		t.Fatal("expected codex provider in inventory")
	}

	got := make([]string, 0, len(codex.Models))
	for _, model := range codex.Models {
		got = append(got, model.ID)
		if model.Source != "registry" {
			t.Fatalf("expected fallback codex model %q source registry, got %q", model.ID, model.Source)
		}
	}

	want := []string{"gpt-5.5", "gpt-5.4", "gpt-5.4-mini"}
	if !slices.Equal(got, want) {
		t.Fatalf("expected fallback codex models %v, got %v", want, got)
	}
}

func TestInstallProviderUsesOSAwareInstallAndRefreshesInventory(t *testing.T) {
	workspace := t.TempDir()
	binDir := filepath.Join(workspace, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin dir: %v", err)
	}

	t.Setenv("PATH", binDir)
	t.Setenv("OPENAI_API_KEY", "test-openai-key")
	t.Setenv("ANTHROPIC_API_KEY", "test-anthropic-key")
	t.Setenv("GEMINI_API_KEY", "test-gemini-key")

	installTriggered := false

	originalRunCommand := runCommandFn
	t.Cleanup(func() {
		runCommandFn = originalRunCommand
	})
	runCommandFn = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if strings.Contains(strings.ToLower(filepath.Base(name)), "codex") && len(args) == 2 && args[0] == "debug" && args[1] == "models" {
			return []byte(`{"models":[
				{"slug":"gpt-5.5","display_name":"GPT-5.5","visibility":"list","supported_in_api":true},
				{"slug":"gpt-5.4","display_name":"GPT-5.4","visibility":"list","supported_in_api":true},
				{"slug":"gpt-5.4-mini","display_name":"GPT-5.4-Mini","visibility":"list","supported_in_api":true}
			]}`), nil
		}

		switch runtime.GOOS {
		case "windows":
			if name != "npm" || !slices.Equal(args, []string{"install", "-g", "@openai/codex"}) {
				t.Fatalf("unexpected codex install command on windows: %s %v", name, args)
			}
		default:
			if name != "npm" || !slices.Equal(args, []string{"install", "-g", "@openai/codex"}) {
				t.Fatalf("unexpected codex install command: %s %v", name, args)
			}
		}
		installTriggered = true
		createdBinary := writeMockProviderBinary(t, binDir, "codex", "codex 1.2.3")
		if _, err := os.Stat(createdBinary); err != nil {
			t.Fatalf("expected codex binary to be created during install: %v", err)
		}
		return []byte("installed"), nil
	}

	instance := &Runner{workspace: workspace}
	inventory, err := instance.InstallProvider(context.Background(), "codex")
	if err != nil {
		t.Fatalf("install provider: %v", err)
	}
	if !installTriggered {
		t.Fatal("expected codex install command to run")
	}

	provider := findProviderInventory(inventory.Providers, "codex")
	if provider == nil {
		t.Fatal("expected codex provider in refreshed inventory")
	}
	if provider.InstallStatus != "INSTALLED" {
		t.Fatalf("expected installed status after refresh, got %q", provider.InstallStatus)
	}
	if provider.AuthStatus != "READY" {
		t.Fatalf("expected ready auth status after refresh, got %q", provider.AuthStatus)
	}
	if provider.DetectedBinary != "codex" {
		t.Fatalf("expected detected binary codex, got %q", provider.DetectedBinary)
	}
	if provider.DetectedVersion != "codex 1.2.3" {
		t.Fatalf("expected detected version from installed binary, got %q", provider.DetectedVersion)
	}
	if provider.LastError != nil {
		t.Fatalf("expected last error to be nil after successful refresh, got %q", *provider.LastError)
	}
}

func TestGetEnvForExecutionPreservesBaseEnvWithoutAccountHomeOverride(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("HOME", filepath.Join(workspace, "base-home"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(workspace, "base-config"))
	t.Setenv("HTTP_PROXY", "http://base-proxy")

	instance := &Runner{workspace: workspace}
	env := instance.getEnvForExecution("codex", "", map[string]string{
		googleDriveProxyProcessKeyEnv: "proc-123",
	}, "")

	if !containsEnvValue(env, "HOME="+filepath.Join(workspace, "base-home")) {
		t.Fatalf("expected HOME to be preserved when accountHomePath is empty, got %v", env)
	}
	if !containsEnvValue(env, "XDG_CONFIG_HOME="+filepath.Join(workspace, "base-config")) {
		t.Fatalf("expected XDG_CONFIG_HOME to be preserved when accountHomePath is empty, got %v", env)
	}
	if !containsEnvValue(env, "HTTP_PROXY=http://base-proxy") {
		t.Fatalf("expected HTTP_PROXY to be preserved when accountHomePath is empty, got %v", env)
	}
	if !containsEnvValue(env, googleDriveProxyProcessKeyEnv+"=proc-123") {
		t.Fatalf("expected custom env to be appended when accountHomePath is empty, got %v", env)
	}
}

func TestGetEnvForExecutionRewritesAccountHomeAndProxyWhenOverrideProvided(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("HOME", filepath.Join(workspace, "base-home"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(workspace, "base-config"))
	t.Setenv("HTTP_PROXY", "http://base-proxy")
	t.Setenv("HTTPS_PROXY", "http://base-proxy-secure")

	accountHomePath := filepath.Join(workspace, "codex-home")
	instance := &Runner{workspace: workspace}
	env := instance.getEnvForExecution("codex", accountHomePath, map[string]string{
		googleDriveProxyProcessKeyEnv: "proc-456",
	}, "http://override-proxy")

	if !containsEnvValue(env, "CODEX_HOME="+accountHomePath) {
		t.Fatalf("expected CODEX_HOME to be rewritten for account-scoped execution, got %v", env)
	}
	if !containsEnvValue(env, "HOME="+accountHomePath) {
		t.Fatalf("expected HOME to be rewritten for account-scoped execution, got %v", env)
	}
	if !containsEnvValue(env, "XDG_CONFIG_HOME="+accountHomePath+"/.config") {
		t.Fatalf("expected XDG_CONFIG_HOME to be rewritten for account-scoped execution, got %v", env)
	}
	if !containsEnvValue(env, "HTTP_PROXY=http://override-proxy") {
		t.Fatalf("expected HTTP_PROXY to be replaced for account-scoped execution, got %v", env)
	}
	if !containsEnvValue(env, "HTTPS_PROXY=http://override-proxy") {
		t.Fatalf("expected HTTPS_PROXY to be replaced for account-scoped execution, got %v", env)
	}
	if !containsEnvValue(env, googleDriveProxyProcessKeyEnv+"=proc-456") {
		t.Fatalf("expected custom env to be appended for account-scoped execution, got %v", env)
	}
}

func TestProviderInstallCommandMatrix(t *testing.T) {
	cases := []struct {
		key         string
		wantCommand string
		wantArgs    []string
	}{
		{key: "codex", wantCommand: "npm", wantArgs: []string{"install", "-g", "@openai/codex"}},
	}

	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			spec, ok := lookupProviderSpec(tc.key)
			if !ok {
				t.Fatalf("expected provider spec for %q", tc.key)
			}

			command, args, err := providerInstallCommand(spec)
			if err != nil {
				t.Fatalf("provider install command: %v", err)
			}
			if command != tc.wantCommand {
				t.Fatalf("expected command %q, got %q", tc.wantCommand, command)
			}
			if !slices.Equal(args, tc.wantArgs) {
				t.Fatalf("expected args %v, got %v", tc.wantArgs, args)
			}
		})
	}

	t.Run("gemini", func(t *testing.T) {
		spec, ok := lookupProviderSpec("gemini")
		if !ok {
			t.Fatal("expected provider spec for gemini")
		}

		command, args, err := providerInstallCommand(spec)
		if err != nil {
			t.Fatalf("provider install command: %v", err)
		}

		switch runtime.GOOS {
		case "darwin", "linux":
			if command != "sh" {
				t.Fatalf("expected sh install command on %s, got %q", runtime.GOOS, command)
			}
			if !slices.Equal(args, []string{"-c", "curl -fsSL https://antigravity.google/cli/install.sh | bash"}) {
				t.Fatalf("unexpected gemini/antigravity args on %s: %v", runtime.GOOS, args)
			}
		case "windows":
			if command != "cmd" {
				t.Fatalf("expected cmd install command on windows, got %q", command)
			}
			if !slices.Equal(args, []string{"/c", "curl -fsSL https://antigravity.google/cli/install.cmd -o install.cmd && install.cmd && del install.cmd"}) {
				t.Fatalf("unexpected gemini/antigravity args on windows: %v", args)
			}
		default:
			if err == nil {
				t.Fatalf("expected unsupported OS error for gemini on %s", runtime.GOOS)
			}
		}
	})

	t.Run("claude", func(t *testing.T) {
		spec, ok := lookupProviderSpec("claude")
		if !ok {
			t.Fatal("expected provider spec for claude")
		}

		command, args, err := providerInstallCommand(spec)
		if err != nil {
			t.Fatalf("provider install command: %v", err)
		}

		switch runtime.GOOS {
		case "darwin", "linux":
			if command != "sh" {
				t.Fatalf("expected sh install command on %s, got %q", runtime.GOOS, command)
			}
			if !slices.Equal(args, []string{"-c", "curl -fsSL https://claude.ai/install.sh | bash"}) {
				t.Fatalf("unexpected claude args on %s: %v", runtime.GOOS, args)
			}
		case "windows":
			if command != "powershell" {
				t.Fatalf("expected powershell install command on windows, got %q", command)
			}
			if !slices.Equal(args, []string{"-NoProfile", "-Command", "irm https://claude.ai/install.ps1 | iex"}) {
				t.Fatalf("unexpected claude args on windows: %v", args)
			}
		default:
			if err == nil {
				t.Fatalf("expected unsupported OS error for claude on %s", runtime.GOOS)
			}
		}
	})
}

func TestProviderAuthStatusGeminiUsesAntigravityConfigMarker(t *testing.T) {
	workspace := t.TempDir()
	homeDir := filepath.Join(workspace, "home")
	configDir := filepath.Join(homeDir, ".gemini", "antigravity-cli")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "settings.json"), []byte(`{"theme":"dark"}`), 0o644); err != nil {
		t.Fatalf("write settings.json: %v", err)
	}

	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)
	t.Setenv("GOOGLE_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "")

	spec, ok := lookupProviderSpec("gemini")
	if !ok {
		t.Fatal("expected provider spec for gemini")
	}

	if got := providerAuthStatus(spec); got != "READY" {
		t.Fatalf("expected READY from antigravity config marker, got %q", got)
	}
}

func TestCommandWithWindowsWorkingDirectoryQuotesPath(t *testing.T) {
	command := commandWithWindowsWorkingDirectory(
		`call "C:\Users\Test User\AppData\Roaming\npm\claude.cmd"`,
		`C:\working dir\flowpilot`,
	)

	want := `cd /d "C:\working dir\flowpilot" && call "C:\Users\Test User\AppData\Roaming\npm\claude.cmd"`
	if command != want {
		t.Fatalf("expected %q, got %q", want, command)
	}
}

func TestProviderEnvSetCommandWindowsClaudeIncludesHomeStyleEnv(t *testing.T) {
	command := providerEnvSetCommand("claude", `C:\Users\Test\.claudeHome1`, "windows")

	for _, expected := range []string{
		`set HOME=C:\Users\Test\.claudeHome1`,
		`set USERPROFILE=C:\Users\Test\.claudeHome1`,
		`set APPDATA=C:\Users\Test\.claudeHome1\AppData\Roaming`,
		`set LOCALAPPDATA=C:\Users\Test\.claudeHome1\AppData\Local`,
		`set XDG_CONFIG_HOME=C:\Users\Test\.claudeHome1\.config`,
	} {
		if !strings.Contains(command, expected) {
			t.Fatalf("expected command to contain %q, got %q", expected, command)
		}
	}
}

func writeMockProviderBinary(t *testing.T, dir, name, version string) string {
	t.Helper()

	switch runtime.GOOS {
	case "windows":
		path := filepath.Join(dir, name+".cmd")
		script := fmt.Sprintf("@echo off\r\nif \"%%~1\"==\"--version\" (\r\n  echo %s\r\n  exit /b 0\r\n)\r\nif \"%%~1\"==\"auth\" (\r\n  exit /b 0\r\n)\r\nexit /b 0\r\n", version)
		if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
			t.Fatalf("write mock provider binary: %v", err)
		}
		return path
	default:
		path := filepath.Join(dir, name)
		script := fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then\n  echo \"%s\"\n  exit 0\nfi\nif [ \"$1\" = \"auth\" ]; then\n  exit 0\nfi\nexit 0\n", version)
		if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
			t.Fatalf("write mock provider binary: %v", err)
		}
		return path
	}
}

func writeMockGeminiBundleCatalog(t *testing.T, workspace string) string {
	t.Helper()

	dir := filepath.Join(workspace, "libexec", "lib", "node_modules", "@google", "gemini-cli", "bundle")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir gemini bundle dir: %v", err)
	}

	path := filepath.Join(dir, "chunk-test.js")
	content := `var PREVIEW_GEMINI_MODEL = "gemini-3-pro-preview";
var PREVIEW_GEMINI_3_1_MODEL = "gemini-3.1-pro-high";
var PREVIEW_GEMINI_3_1_CUSTOM_TOOLS_MODEL = "gemini-3.1-pro-high-customtools";
var PREVIEW_GEMINI_FLASH_MODEL = "gemini-3.5-flash-high";
var PREVIEW_GEMINI_3_1_FLASH_LITE_MODEL = "gemini-3.5-flash-low";
var DEFAULT_GEMINI_MODEL = "gemini-3.1-pro-low";
var DEFAULT_GEMINI_FLASH_MODEL = "gemini-3.5-flash-medium";
var DEFAULT_GEMINI_FLASH_LITE_MODEL = "gemini-3.5-flash-low";
var VALID_GEMINI_MODELS = /* @__PURE__ */ new Set([
  PREVIEW_GEMINI_MODEL,
  PREVIEW_GEMINI_3_1_MODEL,
  PREVIEW_GEMINI_3_1_CUSTOM_TOOLS_MODEL,
  PREVIEW_GEMINI_FLASH_MODEL,
  PREVIEW_GEMINI_3_1_FLASH_LITE_MODEL,
  DEFAULT_GEMINI_MODEL,
  DEFAULT_GEMINI_FLASH_MODEL,
  DEFAULT_GEMINI_FLASH_LITE_MODEL
]);
var PREVIEW_GEMINI_MODEL_AUTO = "gemini-3.5-flash-high";
var DEFAULT_GEMINI_MODEL_AUTO = "gemini-3.5-flash-medium";
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write gemini catalog: %v", err)
	}

	return path
}

func findProviderJSON(providers []map[string]any, key string) map[string]any {
	for _, provider := range providers {
		if provider["key"] == key {
			return provider
		}
	}
	return nil
}

func findProviderInventory(providers []Provider, key string) *Provider {
	for idx := range providers {
		if providers[idx].Key == key {
			return &providers[idx]
		}
	}
	return nil
}
