package runner

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

var geminiBinaryName = func() string {
	if b := strings.TrimSpace(os.Getenv("FLOWPILOT_GEMINI_BIN")); b != "" {
		return b
	}
	return "agy"
}

var errGeminiWorkspaceRequired = fmt.Errorf("Gemini requires a bound project workspace path; set the project's local path before starting chat")

// geminiAdapter runs Antigravity CLI (`agy`) in one-shot print mode per turn.
// FlowPilot owns continuity by reusing a stable project id for the run.
type geminiAdapter struct {
	cwd        string
	scopeKey   string
	env        map[string]string
	promptPrep func(TurnRequest) string

	sessions     *geminiSessionMap
	sessionStore ProviderSessionStore
	mcpServer    *claudeMCPServer
	mcpBaseURL   func() string
}

func newGeminiAdapter(cwd, scopeKey string, env map[string]string) *geminiAdapter {
	return &geminiAdapter{cwd: cwd, scopeKey: scopeKey, env: env}
}

func (a *geminiAdapter) Key() ProviderKey { return ProviderKeyGemini }

func (a *geminiAdapter) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{
		SkillSelection: true,
		Interrupt:      true,
	}
}

func (a *geminiAdapter) preparePrompt(req TurnRequest) string {
	prompt := req.Prompt
	if a.promptPrep != nil {
		prompt = a.promptPrep(req)
	}
	return injectGeminiProjectContext(prompt, strings.TrimSpace(req.ProjectID), firstNonEmptyString(strings.TrimSpace(req.Cwd), strings.TrimSpace(a.cwd)))
}

func (a *geminiAdapter) SendTurn(ctx context.Context, req TurnRequest, bridge TurnBridge) error {
	if strings.TrimSpace(req.Cwd) == "" {
		return errGeminiWorkspaceRequired
	}
	cwd := a.cwd
	if req.Cwd != "" {
		cwd = req.Cwd
	}

	projectID, resume, createProject := a.projectID(req, cwd)
	// V9-21 / V10R re-audit P0: ForceShellBridge under YOLO skips
	// --dangerously-skip-permissions. Gemini --print has no
	// TurnBridge.RequestApproval path, so commit deny is enforced by:
	//  1) prompt guard (soft)
	//  2) PATH git shim — fail closed if install fails (no prompt-only fallthrough)
	//  3) post-turn undo of any commits that bypassed PATH (absolute git / libgit2)
	//  4) post-turn gate as last line of defense
	yoloSkipPerms := req.YoloMode && !req.ForceShellBridge
	prompt := a.preparePrompt(req)
	envOverrides := a.env
	agentCwd := cwd
	var headCheckpoint gitHeadCheckpoint
	var haveCheckpoint bool
	var wtCleanup func()
	if req.ForceShellBridge {
		prompt = "[FlowPilot gate] Do NOT run `git commit` or any shell form that creates a git commit. " +
			"Commits are reserved for the audit/commit-prep step. Use other tools freely.\n\n" + prompt
		guardDir, cleanup, gerr := installGitCommitGuard(true)
		if gerr != nil || guardDir == "" {
			log.Printf("[gemini-agy] git commit guard install failed (fail-closed): %v", gerr)
			if gerr == nil {
				gerr = errGitCommitGuardRequired
			}
			return fmt.Errorf("%w: %v", errGitCommitGuardRequired, gerr)
		}
		defer cleanup()
		// V10R4 P0: snapshot main HEAD+refs+files; run agent in isolated sandbox.
		cp, cperr := captureGitHeadCheckpoint(cwd)
		if cperr != nil {
			return fmt.Errorf("%w: cannot snapshot git HEAD before turn: %v", errGitCommitGuardRequired, cperr)
		}
		headCheckpoint = cp
		haveCheckpoint = true
		wtPath, wtClean, wterr := prepareForceShellBridgeWorktree(cwd, cp)
		if wterr != nil {
			return fmt.Errorf("%w: isolated worktree: %v", errGitCommitGuardRequired, wterr)
		}
		wtCleanup = wtClean
		defer func() {
			if wtCleanup != nil {
				wtCleanup()
			}
		}()
		if wtPath == "" || wtPath == cwd {
			return fmt.Errorf("%w: isolation returned main cwd (unsafe)", errGitCommitGuardRequired)
		}
		agentCwd = wtPath
		// PATH shim + FLOWPILOT_GIT_SANDBOX blocks absolute git into main.
		envOverrides = envWithGitCommitGuard(a.env, guardDir, agentCwd)
	}
	args := geminiCLIArgs(agentCwd, projectID, normalizeGeminiModelName(req.ModelName), yoloSkipPerms, resume, createProject)
	args = append(args, "--print", prompt)
	logGeminiAgyLaunch("adapter", geminiBinaryName(), args, geminiLaunchDebug{
		Cwd:               agentCwd,
		ProjectID:         projectID,
		ProviderSessionID: req.ProviderSessionID,
		RunID:             req.RunID,
		Resume:            resume,
		CreateProject:     createProject,
		Env:               envOverrides,
	})

	cmd := commandContextFn(ctx, geminiBinaryName(), args...)
	cmd.Env = agyFilteredEnv(os.Environ(), envOverrides)
	if strings.TrimSpace(agentCwd) != "" {
		cmd.Dir = agentCwd
	}

	stdoutText, stderrText, err := captureAgyPrint(ctx, cmd)
	// Finalize isolation: copy dirty files to main, discard worktree commits,
	// fail closed if main HEAD moved (never rewrite shared history).
	if req.ForceShellBridge && haveCheckpoint {
		finErr := finalizeForceShellBridgeWorktree(cwd, agentCwd, headCheckpoint)
		if wtCleanup != nil {
			wtCleanup()
			wtCleanup = nil
		}
		if finErr != nil {
			log.Printf("[gemini-agy] ForceShellBridge finalize FAILED (fail-closed) main=%q wt=%q err=%v",
				cwd, agentCwd, finErr)
			if err == nil {
				return finErr
			}
			return fmt.Errorf("%w; also adapter error: %v", finErr, err)
		}
	}
	if err != nil {
		log.Printf("[gemini-agy] result stage=adapter status=error project_id=%q cwd=%q stdout_bytes=%d stderr=%q err=%v", projectID, agentCwd, len(stdoutText), limitLogText(stderrText, 1000), err)
		return geminiProcessError("print", err, stderrText)
	}
	log.Printf("[gemini-agy] result stage=adapter status=ok project_id=%q cwd=%q stdout_bytes=%d stderr_bytes=%d", projectID, agentCwd, len(stdoutText), len(stderrText))

	a.recordSession(ctx, req, projectID, cwd)
	finalMessage := strings.TrimSpace(stdoutText)
	if finalMessage == "" {
		finalMessage = recoverGeminiAgyLatestMessageFn(cwd, a.env)
		if finalMessage != "" {
			log.Printf("[gemini-agy] result stage=adapter recovery=conversation_db project_id=%q cwd=%q recovered_bytes=%d", projectID, cwd, len(finalMessage))
		}
	}
	if finalMessage != "" {
		bridge.Emit(ProviderEvent{Type: EventMessageCompleted, Text: finalMessage})
	}
	bridge.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: finalMessage})
	return nil
}

func (a *geminiAdapter) sessionScopeKey(req TurnRequest) string {
	if scope := strings.TrimSpace(req.ProviderAccountID); scope != "" {
		return scope
	}
	return strings.TrimSpace(a.scopeKey)
}

func (a *geminiAdapter) projectID(req TurnRequest, cwd string) (string, bool, bool) {
	scopeKey := a.sessionScopeKey(req)
	if a.sessions != nil {
		for _, candidate := range []string{strings.TrimSpace(req.ProviderSessionID), strings.TrimSpace(req.RunID)} {
			if candidate == "" {
				continue
			}
			if real := a.sessions.realSession(scopeKey, candidate); real != "" {
				return real, true, false
			}
		}
	}
	if sessionID := strings.TrimSpace(req.ProviderSessionID); sessionID != "" && !isSyntheticGeminiSessionID(sessionID) {
		return sessionID, true, false
	}
	if workspaceProjectID, err := resolveGeminiProjectIDForWorkspace(cwd, a.env); err == nil && workspaceProjectID != "" {
		return workspaceProjectID, false, false
	}
	if projectID := strings.TrimSpace(req.ProjectID); projectID != "" {
		return projectID, false, true
	}
	if runID := strings.TrimSpace(req.RunID); runID != "" {
		return "flowpilot-gemini-" + runID, false, true
	}
	if scopeKey != "" {
		return "flowpilot-gemini-" + scopeKey, false, true
	}
	return "flowpilot-gemini-default", false, true
}

func (a *geminiAdapter) recordSession(ctx context.Context, req TurnRequest, sessionID, cwd string) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	if a.sessions != nil {
		scopeKey := a.sessionScopeKey(req)
		a.sessions.setRealSession(scopeKey, req.ProviderSessionID, sessionID)
		a.sessions.setRealSession(scopeKey, req.RunID, sessionID)
		a.sessions.setRealSession(scopeKey, sessionID, sessionID)
	}
	if a.sessionStore == nil {
		return
	}
	_ = a.sessionStore.UpsertSession(ctx, ProviderSessionRecord{
		WorkflowRunID:     req.RunID,
		ProviderKey:       string(ProviderKeyGemini),
		ProviderSessionID: sessionID,
		ProviderThreadID:  sessionID,
		WorkingDirectory:  cwd,
		ModelName:         req.ModelName,
		Status:            "active",
	})
}

func geminiCLIArgs(cwd, projectID, model string, yolo, resume, createProject bool) []string {
	args := []string{"--project", strings.TrimSpace(projectID), "--print-timeout", "5m"}
	if resume {
		args = append(args, "--continue")
	} else if createProject {
		args = append(args, "--new-project")
	}
	if strings.TrimSpace(model) != "" {
		args = append(args, "--model", strings.TrimSpace(model))
	}
	if strings.TrimSpace(cwd) != "" {
		args = append(args, "--add-dir", strings.TrimSpace(cwd))
	}
	if yolo {
		args = append(args, "--dangerously-skip-permissions")
	} else {
		args = append(args, "--sandbox")
	}
	return args
}

func resolveGeminiProjectIDForWorkspace(cwd string, env map[string]string) (string, error) {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		return "", errGeminiWorkspaceRequired
	}
	return ensureGeminiProjectConfig(cwd, env)
}

func geminiProjectEnvHints(accountHomePath string, customEnv map[string]string) map[string]string {
	hints := map[string]string{}
	for key, value := range customEnv {
		if strings.TrimSpace(key) != "" {
			hints[key] = value
		}
	}
	if home := strings.TrimSpace(accountHomePath); home != "" {
		hints["HOME"] = home
	}
	return hints
}

func geminiSessionProjectID(currentProviderSessionID, cwd string, env map[string]string) (string, bool, error) {
	currentProviderSessionID = strings.TrimSpace(currentProviderSessionID)
	if currentProviderSessionID != "" && !isSyntheticGeminiSessionID(currentProviderSessionID) {
		return currentProviderSessionID, true, nil
	}
	projectID, err := resolveGeminiProjectIDForWorkspace(cwd, env)
	if err != nil {
		return "", false, err
	}
	if projectID == "" {
		return "", false, errGeminiWorkspaceRequired
	}
	return projectID, false, nil
}

type geminiLaunchDebug struct {
	Cwd               string
	ProjectID         string
	ProviderSessionID string
	RunID             string
	Resume            bool
	CreateProject     bool
	Env               map[string]string
}

func logGeminiAgyLaunch(stage, binary string, args []string, debug geminiLaunchDebug) {
	log.Printf(
		"[gemini-agy] launch stage=%s binary=%q cwd=%q project_id=%q provider_session_id=%q run_id=%q resume=%t create_project=%t config_path=%q home=%q gemini_home=%q args=%q",
		stage,
		binary,
		strings.TrimSpace(debug.Cwd),
		strings.TrimSpace(debug.ProjectID),
		strings.TrimSpace(debug.ProviderSessionID),
		strings.TrimSpace(debug.RunID),
		debug.Resume,
		debug.CreateProject,
		geminiProjectConfigPath(debug.ProjectID, debug.Env),
		strings.TrimSpace(debug.Env["HOME"]),
		strings.TrimSpace(debug.Env["GEMINI_HOME"]),
		sanitizeGeminiLogArgs(args),
	)
}

func sanitizeGeminiLogArgs(args []string) []string {
	sanitized := append([]string{}, args...)
	for i, arg := range sanitized {
		if arg == "--print" && i+1 < len(sanitized) {
			sanitized[i+1] = fmt.Sprintf("<prompt bytes=%d>", len(sanitized[i+1]))
			break
		}
	}
	return sanitized
}

func geminiProjectConfigPath(projectID string, env map[string]string) string {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return ""
	}
	projectsDir := geminiProjectsConfigDir(env)
	if projectsDir == "" {
		projectsDir = geminiProjectsConfigDirOrDefault(env)
	}
	if strings.TrimSpace(projectsDir) == "" {
		return ""
	}
	return filepath.Join(projectsDir, projectID+".json")
}

func limitLogText(text string, max int) string {
	text = strings.TrimSpace(text)
	if max <= 0 || len(text) <= max {
		return text
	}
	return text[:max] + "...(truncated)"
}

func geminiProcessError(stage string, err error, stderr string) error {
	msg := strings.TrimSpace(stderr)
	if msg != "" {
		return fmt.Errorf("gemini %s: %w: %s", stage, err, msg)
	}
	return fmt.Errorf("gemini %s: %w", stage, err)
}

func injectGeminiProjectContext(prompt, projectID, cwd string) string {
	projectID = strings.TrimSpace(projectID)
	cwd = strings.TrimSpace(cwd)
	if projectID == "" && cwd == "" {
		return prompt
	}

	var b strings.Builder
	b.WriteString("[FlowPilot active project context]\n")
	if projectID != "" {
		b.WriteString("Active project id: ")
		b.WriteString(projectID)
		b.WriteString("\n")
	}
	if cwd != "" {
		b.WriteString("Attached workspace cwd: ")
		b.WriteString(cwd)
		b.WriteString("\n")
	}
	b.WriteString("This FlowPilot run is already attached to the workspace above. For questions about this project, inspect that working directory and answer from its files. Do not inspect provider project registry files or claim there is no active workspace unless the attached directory itself is missing or unreadable.\n\n")
	b.WriteString(prompt)
	return b.String()
}

type geminiProjectConfig struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	ProjectResources struct {
		Resources []struct {
			GitFolder struct {
				FolderURI string `json:"folderUri"`
			} `json:"gitFolder"`
		} `json:"resources"`
	} `json:"projectResources"`
}

func resolveGeminiConfiguredProjectID(cwd string, env map[string]string) string {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		return ""
	}
	absCwd, err := filepath.Abs(cwd)
	if err != nil {
		return ""
	}
	projectsDir := geminiProjectsConfigDir(env)
	if projectsDir == "" {
		return ""
	}
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			continue
		}
		configPath := filepath.Join(projectsDir, entry.Name())
		raw, err := os.ReadFile(configPath)
		if err != nil {
			continue
		}
		var cfg geminiProjectConfig
		if err := json.Unmarshal(raw, &cfg); err != nil {
			continue
		}
		if strings.TrimSpace(cfg.ID) == "" {
			continue
		}
		for _, resource := range cfg.ProjectResources.Resources {
			folderURI := strings.TrimSpace(resource.GitFolder.FolderURI)
			if folderURI == "" {
				continue
			}
			matchedPath, err := geminiFolderURIPath(folderURI)
			if err != nil {
				continue
			}
			if sameFilePath(absCwd, matchedPath) {
				return strings.TrimSpace(cfg.ID)
			}
		}
	}
	return ""
}

func geminiProjectsConfigDir(env map[string]string) string {
	candidates := []string{}
	if env != nil {
		if geminiHome := strings.TrimSpace(env["GEMINI_HOME"]); geminiHome != "" {
			candidates = append(candidates, filepath.Join(geminiHome, "config", "projects"))
			candidates = append(candidates, filepath.Join(geminiHome, ".gemini", "config", "projects"))
		}
		if home := strings.TrimSpace(env["HOME"]); home != "" {
			candidates = append(candidates, filepath.Join(home, ".gemini", "config", "projects"))
		}
	}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		candidates = append(candidates, filepath.Join(home, ".gemini", "config", "projects"))
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}
	return ""
}

func geminiFolderURIPath(folderURI string) (string, error) {
	parsed, err := url.Parse(folderURI)
	if err != nil {
		return "", err
	}
	path := strings.TrimSpace(parsed.Path)
	// Go's url.URL.String() serialises Windows absolute paths as file://C:/path
	// (no third slash), so url.Parse sees "C:" as the host and "/path" as the
	// path — the drive letter is lost.  Detect this case and reconstruct.
	if h := parsed.Host; len(h) == 2 && h[1] == ':' {
		path = h + path
	}
	if path == "" {
		return "", fmt.Errorf("empty path")
	}
	unescaped, err := url.PathUnescape(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(unescaped), nil
}

func sameFilePath(left, right string) bool {
	left = filepath.Clean(strings.TrimSpace(left))
	right = filepath.Clean(strings.TrimSpace(right))
	if left == "" || right == "" {
		return false
	}
	return left == right
}

func ensureGeminiProjectConfig(cwd string, env map[string]string) (string, error) {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		return "", nil
	}
	absCwd, err := filepath.Abs(cwd)
	if err != nil {
		return "", err
	}
	if configuredProjectID := resolveGeminiConfiguredProjectID(absCwd, env); configuredProjectID != "" {
		log.Printf("[gemini-agy] project_config action=found project_id=%q cwd=%q config_path=%q", configuredProjectID, absCwd, geminiProjectConfigPath(configuredProjectID, env))
		return configuredProjectID, nil
	}
	projectsDir := geminiProjectsConfigDirOrDefault(env)
	if projectsDir == "" {
		log.Printf("[gemini-agy] project_config action=missing_projects_dir cwd=%q", absCwd)
		return "", nil
	}
	if err := os.MkdirAll(projectsDir, 0o755); err != nil {
		log.Printf("[gemini-agy] project_config action=mkdir_failed cwd=%q projects_dir=%q err=%v", absCwd, projectsDir, err)
		return "", err
	}
	projectID, err := newGeminiProjectUUID()
	if err != nil {
		return "", err
	}
	configPath := filepath.Join(projectsDir, projectID+".json")
	payload, err := buildGeminiProjectConfigPayload(projectID, filepath.Base(absCwd), absCwd)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(configPath, payload, 0o644); err != nil {
		log.Printf("[gemini-agy] project_config action=write_failed project_id=%q cwd=%q config_path=%q err=%v", projectID, absCwd, configPath, err)
		return "", err
	}
	log.Printf("[gemini-agy] project_config action=created project_id=%q cwd=%q config_path=%q", projectID, absCwd, configPath)
	return projectID, nil
}

func geminiProjectsConfigDirOrDefault(env map[string]string) string {
	if dir := geminiProjectsConfigDir(env); dir != "" {
		return dir
	}
	candidates := []string{}
	if env != nil {
		if geminiHome := strings.TrimSpace(env["GEMINI_HOME"]); geminiHome != "" {
			candidates = append(candidates, filepath.Join(geminiHome, ".gemini", "config", "projects"))
			candidates = append(candidates, filepath.Join(geminiHome, "config", "projects"))
		}
		if home := strings.TrimSpace(env["HOME"]); home != "" {
			candidates = append(candidates, filepath.Join(home, ".gemini", "config", "projects"))
		}
	}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		candidates = append(candidates, filepath.Join(home, ".gemini", "config", "projects"))
	}
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate) != "" {
			return candidate
		}
	}
	return ""
}

func newGeminiProjectUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		uint32(b[0])<<24|uint32(b[1])<<16|uint32(b[2])<<8|uint32(b[3]),
		uint16(b[4])<<8|uint16(b[5]),
		uint16(b[6])<<8|uint16(b[7]),
		uint16(b[8])<<8|uint16(b[9]),
		uint64(b[10])<<40|uint64(b[11])<<32|uint64(b[12])<<24|uint64(b[13])<<16|uint64(b[14])<<8|uint64(b[15]),
	), nil
}

func buildGeminiProjectConfigPayload(projectID, projectName, workspacePath string) ([]byte, error) {
	projectID = strings.TrimSpace(projectID)
	projectName = strings.TrimSpace(projectName)
	workspacePath = strings.TrimSpace(workspacePath)
	if projectID == "" || workspacePath == "" {
		return nil, fmt.Errorf("gemini project config requires project id and workspace path")
	}
	if projectName == "" {
		projectName = "FlowPilot Project"
	}
	folderURI := (&url.URL{Scheme: "file", Path: filepath.ToSlash(workspacePath)}).String()
	payload := map[string]any{
		"id":   projectID,
		"name": projectName,
		"projectResources": map[string]any{
			"resources": []any{
				map[string]any{
					"gitFolder": map[string]any{
						"folderUri":     folderURI,
						"defaultBranch": "",
						"allowWrite":    true,
					},
				},
			},
		},
	}
	return json.MarshalIndent(payload, "", "  ")
}

// agyEnvBlocklist holds exact env var names that cause `agy --print` to hang
// when inherited from a Claude Code parent process.
var agyEnvBlocklist = map[string]bool{
	"CLAUDECODE": true,
	"AI_AGENT":   true,
}

// agyEnvBlockPrefixes holds uppercase prefixes that also cause the hang.
var agyEnvBlockPrefixes = []string{"CLAUDE_CODE_", "CLAUDE_AGENT_"}

// agyFilteredEnv builds the subprocess environment for agy, stripping the
// Claude Code env vars that cause `agy --print` to hang, then applying
// the provided overrides on top. Override keys replace base keys (no
// duplicate PATH etc.) so git-guard PATH prepend is the only PATH entry.
func agyFilteredEnv(base []string, overrides map[string]string) []string {
	overrideKeys := make(map[string]struct{}, len(overrides))
	for k := range overrides {
		if strings.TrimSpace(k) != "" {
			overrideKeys[k] = struct{}{}
		}
	}
	result := make([]string, 0, len(base)+len(overrides))
	for _, entry := range base {
		key := entry
		if idx := strings.IndexByte(entry, '='); idx >= 0 {
			key = entry[:idx]
		}
		if _, replaced := overrideKeys[key]; replaced {
			continue
		}
		if agyEnvBlocklist[key] {
			continue
		}
		upper := strings.ToUpper(key)
		blocked := false
		for _, prefix := range agyEnvBlockPrefixes {
			if strings.HasPrefix(upper, prefix) {
				blocked = true
				break
			}
		}
		if blocked {
			continue
		}
		result = append(result, entry)
	}
	for k, v := range overrides {
		if strings.TrimSpace(k) != "" {
			result = append(result, k+"="+v)
		}
	}
	return result
}
