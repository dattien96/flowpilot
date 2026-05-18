# CP-08: Go-Runner Implementation — CLI, Providers, Skills & Process Isolation

**Maps from:** SD-03 (Util Tools), SD-05 §3–5 (Prompt Assembly, Provider Execution, LLM Files), SD-06 (AI Provider Integration), SD-07 (Skill & Agent Runtime), SD-10 (Context Resolver & RAG)
**Phase:** Cross-cutting (built alongside Phases 4–6)
**Depends on:** CP-04 (workflow engine schema must exist first), CP-09 (artifact memory schema and retrieval policy)

---

## 1. Core Concept

This plan covers everything the **Go-Runner** (Cobra CLI) must implement that is NOT captured in the Admin Web coding plans. The Go-Runner is the execution engine that:
1. Pulls pending workflow steps from Supabase
2. Resolves prompt memory from workflow context, MCP context, user context, and artifact working memory
3. Assembles prompts from skills + selected context
4. Invokes AI provider CLIs
5. Parses output, saves artifacts, and generates working memory
6. Manages approval gates and retry loops

---

## 2. Cobra CLI Command Structure

```go
// cmd/root.go
flowpilot
├── run           <workflow_run_id>    // Execute a workflow run
├── install-provider <provider_name>   // Install Claude/Codex/Gemini CLI
├── install-tools                      // Install RTK + GitNexus utilities
├── init-project  <project_id>        // Generate CLAUDE.md/AGENTS.md/GEMINI.md + sync skills
├── sync-skills   <project_id>        // Sync skill folders to/from Google Drive
├── cache-clear   <workflow_id>       // Invalidate all prompt cache for a workflow
├── status                             // Show runner status and active workflow runs
└── version                            // Print version info
```

---

## 3. Utility Tool Installation (from SD-03)

### 3.1 GitNexus
```go
// internal/tools/gitnexus.go
func installGitNexus() error {
    if _, err := exec.LookPath("gitnexus"); err == nil {
        log.Info("GitNexus already installed")
        return nil
    }
    // Prompt user for confirmation
    if !promptUserConfirmation("GitNexus is required for code intelligence. Install now?") {
        return ErrUserDeclined
    }
    cmd := exec.Command("npm", "install", "-g", "gitnexus")
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    if err := cmd.Run(); err != nil {
        return fmt.Errorf("failed to install GitNexus: %w", err)
    }
    // Verify
    return verifyCommand("gitnexus", "--version")
}
```

### 3.2 RTK (Rust Token Killer)
```go
// internal/tools/rtk.go
func installRTK() error {
    if _, err := exec.LookPath("rtk"); err == nil {
        log.Info("RTK already installed")
        return nil
    }
    if !promptUserConfirmation("RTK is recommended for token-optimized CLI usage. Install now?") {
        return ErrUserDeclined
    }
    switch runtime.GOOS {
    case "darwin":
        cmd := exec.Command("brew", "install", "rtk")
        return cmd.Run()
    case "linux":
        cmd := exec.Command("sh", "-c",
            `curl -fsSL https://raw.githubusercontent.com/rtk-ai/rtk/refs/heads/master/install.sh | sh`)
        return cmd.Run()
    default:
        return fmt.Errorf("RTK install not supported on %s — install manually", runtime.GOOS)
    }
}
```

### 3.3 Verification
```go
func verifyCommand(name string, args ...string) error {
    cmd := exec.Command(name, args...)
    out, err := cmd.Output()
    if err != nil {
        return fmt.Errorf("%s verification failed: %w", name, err)
    }
    log.Infof("%s installed: %s", name, strings.TrimSpace(string(out)))
    return nil
}
```

---

## 4. AI Provider Installation (from SD-06 §2–5)

### 4.1 Provider Detection & Installation
```go
// internal/provider/installer.go
type ProviderInstaller struct {
    Name    string
    Detect  func() bool
    Install func() error
    Verify  func() error
    Auth    func() error
}

var providers = map[string]ProviderInstaller{
    "claude": {
        Name:   "Claude Code",
        Detect: func() bool { _, err := exec.LookPath("claude"); return err == nil },
        Install: func() error {
            switch runtime.GOOS {
            case "darwin":
                return exec.Command("brew", "install", "--cask", "claude-code").Run()
            case "linux":
                return exec.Command("sh", "-c",
                    `curl -fsSL https://claude.ai/install.sh | bash`).Run()
            case "windows":
                return exec.Command("powershell", "-Command",
                    `irm https://claude.ai/install.ps1 | iex`).Run()
            }
            return ErrUnsupportedOS
        },
        Verify: func() error { return verifyCommand("claude", "--version") },
        Auth:   func() error { return exec.Command("claude", "auth").Run() },
    },
    "codex": {
        Name:   "Codex CLI",
        Detect: func() bool { _, err := exec.LookPath("codex"); return err == nil },
        Install: func() error {
            return exec.Command("npm", "install", "-g", "@openai/codex").Run()
        },
        Verify: func() error { return verifyCommand("codex", "--version") },
        Auth:   func() error { return exec.Command("codex", "auth").Run() },
    },
    "gemini": {
        Name:   "Gemini CLI",
        Detect: func() bool { _, err := exec.LookPath("gemini"); return err == nil },
        Install: func() error {
            return exec.Command("npm", "install", "-g", "@google/gemini-cli").Run()
        },
        Verify: func() error { return verifyCommand("gemini", "--version") },
        Auth:   nil, // Auth happens on first run interactively
    },
}
```

### 4.2 Go-Runner Installation Flow
```
flowpilot install-provider claude
    → exec.LookPath("claude")
    → IF found → "Claude already installed" + verify version
    → IF not found → run OS-appropriate install command
    → Verify: claude --version
    → Auth: claude auth (opens browser)
    → Update Supabase: project_settings.provider_status = 'INSTALLED' or 'FAILED'
```

### 4.3 Configuration Resolution Priority
Before making any LLM call, the Go-Runner resolves the provider/model:
```go
// internal/provider/resolver.go
func resolveProvider(step WorkflowRunStep, run WorkflowRun, project Project) (string, string) {
    // Priority 1: Step-level override (highest)
    if step.ProviderOverride != "" {
        return step.ProviderOverride, step.ModelOverride
    }
    // Priority 2: Run-level override
    if run.Provider != "" {
        return run.Provider, run.Model
    }
    // Priority 3: Project default (lowest)
    return project.DefaultProvider, project.DefaultModel
}
```

---

## 5. Prompt Assembly Template (from SD-05 §3)

### 5.1 Template Structure
Every assembled prompt follows this exact Markdown structure:

```markdown
# System Instruction
You are a [Agent Role] working on project [Project Name].
Provider: [Claude/Codex/Gemini]
Model: [model-version]

# Workflow Context
This is Step [N] of [Total] in the "[Workflow Name]" workflow.
Previous steps completed: [list of completed steps and their artifact summaries]

# Skills & Rules
[Contents of built-in SKILL.md for this step type]
[Contents of any custom SKILL.md files attached]

# MCP Context
[Jira ticket data, if loaded]
[Figma design data, if loaded]
[Other MCP data]

# User Context
[User-provided text, uploaded files, pasted links]

# Selected Working Memory
[Structured summaries, decisions, constraints, and source references selected by the Context Resolver]

# Source Artifacts
[Links/references to raw artifact files used as durable memory]

# Raw Artifact Excerpts
[Only included when required by step policy or when summary memory is insufficient]

# Reviewer Feedback (Retry)
[Only populated on retry — contains rejection_note from previous attempt]

# Task
Execute the [Step Name] according to the skills and rules above.
Output your result as a structured Markdown artifact.
```

### 5.2 Assembly Implementation
```go
// internal/workflow/assembler.go
type PromptAssembler struct {
    stepDef      StepDefinition
    skills       []SkillContent
    mcpData      map[string]string
    userContext   string
    promptMemory  PromptMemory
    rejectionNote string
}

func (a *PromptAssembler) Assemble() string {
    var b strings.Builder

    // System instruction
    b.WriteString("# System Instruction\n")
    b.WriteString(fmt.Sprintf("You are a %s working on project %s.\n", a.stepDef.AgentType, a.projectName))
    b.WriteString(fmt.Sprintf("Provider: %s\nModel: %s\n\n", a.provider, a.model))

    // Workflow context
    b.WriteString("# Workflow Context\n")
    b.WriteString(fmt.Sprintf("This is Step %d of %d in the \"%s\" workflow.\n", a.stepIndex, a.totalSteps, a.workflowName))
    // ... previous step summaries

    // Skills & Rules
    b.WriteString("# Skills & Rules\n")
    for _, skill := range a.skills {
        b.WriteString(skill.Content)
        b.WriteString("\n\n")
    }

    // MCP Context (runtime — not cached)
    b.WriteString("# MCP Context\n")
    for name, data := range a.mcpData {
        b.WriteString(fmt.Sprintf("## %s\n%s\n\n", name, data))
    }

    // User Context (runtime — not cached)
    b.WriteString("# User Context\n")
    b.WriteString(a.userContext + "\n\n")

    // Selected working memory (runtime — not cached)
    b.WriteString("# Selected Working Memory\n")
    for _, item := range a.promptMemory.WorkingMemoryItems {
        b.WriteString(fmt.Sprintf("## %s\n%s\nSource: %s\n\n", item.Title, item.Summary, item.SourceRef))
    }

    // Source artifacts (runtime — audit references)
    b.WriteString("# Source Artifacts\n")
    for _, source := range a.promptMemory.SourceArtifacts {
        b.WriteString(fmt.Sprintf("- %s\n", source.Ref))
    }
    b.WriteString("\n")

    // Raw artifact excerpts (runtime — only when policy requires expansion)
    if len(a.promptMemory.RawExcerpts) > 0 {
        b.WriteString("# Raw Artifact Excerpts\n")
        for _, excerpt := range a.promptMemory.RawExcerpts {
            b.WriteString(fmt.Sprintf("## %s\n%s\n\n", excerpt.SourceRef, excerpt.Content))
        }
    }

    // Rejection feedback (runtime — only on retry)
    if a.rejectionNote != "" {
        b.WriteString("# Reviewer Feedback (Retry)\n")
        b.WriteString(fmt.Sprintf("The previous output was rejected. Reason: \"%s\"\n", a.rejectionNote))
        b.WriteString("Please revise your output to address this feedback.\n\n")
    }

    // Task
    b.WriteString("# Task\n")
    b.WriteString(fmt.Sprintf("Execute the %s according to the skills and rules above.\n", a.stepDef.Name))
    b.WriteString("Output your result as a structured Markdown artifact.\n")

    return b.String()
}
```

---

## 6. Context Resolver Integration (from SD-10)

Before the Prompt Assembler injects runtime context, the Go-Runner must call the Context Resolver from CP-09.

```text
internal/contextresolver/
  policy.go
  resolver.go
  ranking.go
  packing.go
  audit.go
```

Responsibilities:
1. Load the step retrieval policy from `step_definitions` or workflow step config.
2. Include mandatory context: current step, workflow run, user context, MCP context, retry note.
3. Include latest approved immediate previous-step artifact summary.
4. Include pinned artifact memories and pinned annotations.
5. Generate an embedding for the current step query through `generate-embedding`.
6. Search `artifact_memories` through `match_artifact_memories()`.
7. Rank by semantic similarity, approval status, recency, same workflow/run, required artifact type, and pinned boosts.
8. Pack selected context into the step token budget.
9. Load raw artifact excerpts only when policy requires full detail.
10. Insert `workflow_prompt_context_items` audit rows for every selected item.

```go
// internal/contextresolver/resolver.go
type PromptMemory struct {
    WorkingMemoryItems []WorkingMemoryItem
    SourceArtifacts    []SourceArtifactRef
    RawExcerpts        []RawArtifactExcerpt
    TokenEstimate      int
}

func ResolvePromptMemory(ctx context.Context, input ResolveInput) (PromptMemory, error) {
    policy := loadStepContextPolicy(input.StepDefinition, input.WorkflowStep)
    mandatory := collectMandatoryContext(input)
    pinned := loadPinnedMemory(input.ProjectID, input.WorkflowID)

    queryText := buildRetrievalQuery(input.StepDefinition, input.UserContext, input.MCPContext)
    queryEmbedding := callGenerateEmbedding(queryText)
    candidates := matchArtifactMemories(queryEmbedding, input.ProjectID, input.WorkflowID, policy)

    ranked := rankMemoryCandidates(candidates, mandatory, pinned, policy)
    packed := packPromptMemory(ranked, mandatory, policy.TokenBudget)

    insertPromptContextAudit(input.WorkflowRunStepID, packed)
    return packed, nil
}
```

Prompt assembly must use `PromptMemory`; it must not append all previous raw artifacts by default.

---

## 7. Provider-Specific Execution (from SD-05 §4)

### 7.1 Standard Steps
```go
// internal/provider/executor.go
func executeProvider(provider, model, promptFilePath string) (string, error) {
    switch provider {
    case "claude":
        // Pipe prompt via stdin
        cmd := exec.Command("claude", "--print", "--model", model)
        cmd.Stdin, _ = os.Open(promptFilePath)
        out, err := cmd.Output()
        return string(out), err

    case "codex":
        // Use exec mode for non-interactive execution
        content, _ := os.ReadFile(promptFilePath)
        cmd := exec.Command("codex", "exec", "--model", model, "--prompt", string(content))
        out, err := cmd.Output()
        return string(out), err

    case "gemini":
        // Pipe prompt via stdin
        cmd := exec.Command("gemini", "--model", model)
        cmd.Stdin, _ = os.Open(promptFilePath)
        out, err := cmd.Output()
        return string(out), err
    }
    return "", ErrUnknownProvider
}
```

### 7.2 Special Case: Code/Review Loop Step
The Code/Review Loop is unique — it requires a **long-running interactive agent** that autonomously codes, compiles, tests, and reviews:

```go
// internal/workflow/code_review_loop.go
func executeCodeReviewLoop(provider, model, promptFilePath, projectDir string) (string, error) {
    var cmd *exec.Cmd

    switch provider {
    case "claude":
        // Long-running autonomous mode
        cmd = exec.Command("claude", "--dangerously-skip-permissions", "--model", model)
    case "codex":
        // Full-auto mode for autonomous coding
        cmd = exec.Command("codex", "exec", "--full-auto", "--model", model)
    case "gemini":
        cmd = exec.Command("gemini", "--model", model)
    }

    cmd.Dir = projectDir  // Execute in project directory
    cmd.Stdin, _ = os.Open(promptFilePath)

    // Stream stdout/stderr for real-time logging
    stdout, _ := cmd.StdoutPipe()
    stderr, _ := cmd.StderrPipe()

    cmd.Start()

    // Stream output to Supabase workflow_run_logs in real-time
    go streamToLogs(stdout, "stdout", stepID)
    go streamToLogs(stderr, "stderr", stepID)

    err := cmd.Wait()

    // Collect the output diff and review report as artifacts
    return collectArtifacts(projectDir), err
}
```

**Exit criteria** (Go-Runner monitors):
- Build compiles successfully
- All unit tests pass
- Code coverage >= 80%

If criteria not met after provider finishes, the loop retries (up to configurable max).

---

## 8. Persistent LLM Instruction Files (from SD-05 §5)

### 8.1 Project Instruction File Generation
The `flowpilot init-project` command generates the provider-specific instruction file:

```go
// internal/project/init.go
func initProjectFiles(project Project, provider string) error {
    var filename string
    switch provider {
    case "claude":
        filename = "CLAUDE.md"
    case "codex":
        filename = "AGENTS.md"
    case "gemini":
        filename = "GEMINI.md"
    }

    content := fmt.Sprintf(`# %s

## Project Description
%s

## Directory Structure
%s

## Coding Conventions
- Follow existing patterns in the codebase
- Use TypeScript strict mode
- All new code must have unit tests

## Active Skills
%s
`, project.Name, project.Description, generateDirTree(project.DirectoryPath), listActiveSkills(project))

    path := filepath.Join(project.DirectoryPath, filename)
    return os.WriteFile(path, []byte(content), 0644)
}
```

**Important:** This file is NOT the workflow. It is the "project onboarding" context that every AI session loads automatically.

### 8.2 Skill File Management
Built-in and custom skills live permanently in provider-specific directories:
```
project-root/
├── .claude/skills/planning/SKILL.md
├── .claude/skills/architecture/SKILL.md
├── .claude/skills/tdd/SKILL.md
├── .claude/skills/coding/SKILL.md
├── .claude/skills/review/SKILL.md
├── .codex/skills/...   (same structure)
├── .gemini/skills/...  (same structure)
```

The Go-Runner:
1. Syncs built-in skills from the backend/Supabase Storage to these folders
2. Reads from these folders when assembling runtime prompts
3. Monitors for changes (used in cache hash computation)

---

## 9. Artifact Storage, Upload & Memory Generation (from SD-08 and SD-10)

When the Go-Runner receives a successful payload from the AI Provider, it must handle the output artifact locally before syncing to the cloud:

```go
// internal/workflow/artifact.go
func parseAndSaveArtifact(output string, step WorkflowRunStep, project Project) (Artifact, error) {
    // 1. Extract markdown/json from LLM output block
    content := extractContent(output)
    
    // 2. Save locally to .artifacts/ folder
    artifactDir := filepath.Join(project.DirectoryPath, ".artifacts")
    os.MkdirAll(artifactDir, 0755)
    
    filename := fmt.Sprintf("%s_%s.md", step.ID, step.StepType)
    localPath := filepath.Join(artifactDir, filename)
    os.WriteFile(localPath, []byte(content), 0644)
    
    // 3. Upload to configured storage backend (Supabase or Google Drive)
    var storageURL string
    var err error
    
    if project.ArtifactStoragePreference == "google_drive" && project.HasGoogleDriveMCP {
        // Upload via Google Drive MCP Edge Function
        storageURL, err = uploadToGoogleDrive(localPath, project.GoogleDriveFolderID)
    } else {
        // Default to Supabase Storage Bucket
        bucketPath := fmt.Sprintf("artifacts/%s/%s", project.ID, filename)
        storageURL, err = uploadToSupabaseStorage(localPath, bucketPath)
    }
    
    if err != nil {
        return Artifact{}, err
    }
    
    // 4. Create record in Supabase DB
    artifact := Artifact{
        WorkflowRunID: step.WorkflowRunID,
        StepID:        step.ID,
        ContentURL:    storageURL,
        ContentRaw:    content, // Optionally store raw content if small enough
        Version:       1,
    }
    insertArtifactRecord(artifact)

    // 5. Generate searchable working memory for later prompt retrieval.
    // Failure must not roll back the raw artifact save.
    if err := generateArtifactMemory(artifact, content, step, project); err != nil {
        log.Warnf("artifact memory generation failed for %s: %v", artifact.ID, err)
        markArtifactMemoryFailed(artifact.ID, err)
    }
    
    return artifact, nil
}
```

```go
// internal/workflow/artifact_memory.go
func generateArtifactMemory(artifact Artifact, content string, step WorkflowRunStep, project Project) error {
    memory, err := extractWorkingMemory(content, step)
    if err != nil {
        return err
    }

    memoryID, err := insertArtifactMemory(ArtifactMemory{
        AIOutputID:        artifact.ID,
        ProjectID:         project.ID,
        WorkflowRunID:     step.WorkflowRunID,
        WorkflowRunStepID: step.ID,
        ArtifactType:      step.StepType,
        ArtifactStatus:    artifact.Status,
        ArtifactVersion:   artifact.Version,
        Summary:           memory.Summary,
        KeyDecisions:      memory.KeyDecisions,
        Constraints:       memory.Constraints,
        Assumptions:       memory.Assumptions,
        OpenQuestions:     memory.OpenQuestions,
        Keywords:          memory.Keywords,
        SourceRefs:        []string{artifact.ContentURL},
        TokenEstimate:     estimateTokens(memory.Summary),
        EmbeddingStatus:   "pending",
    })
    if err != nil {
        return err
    }

    embeddingText := buildEmbeddingText(memory)
    embedding, dimensions, err := callGenerateEmbedding(embeddingText)
    if err != nil {
        markEmbeddingFailed(memoryID, err)
        return err
    }

    return updateArtifactMemoryEmbedding(memoryID, embedding, dimensions, "gte-small")
}
```

---

## 10. Process Isolation for Heavy Agents (from SD-07 §2)

When a workflow step requires a heavy-duty agent (like Code/Review Loop), the main Go-Runner spawns a **separate background process** to avoid blocking:

```go
// internal/workflow/process.go
func spawnIsolatedAgent(cmd *exec.Cmd, stepID string) (*AgentProcess, error) {
    // Set process group for clean cleanup
    cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

    stdout, _ := cmd.StdoutPipe()
    stderr, _ := cmd.StderrPipe()

    if err := cmd.Start(); err != nil {
        return nil, err
    }

    proc := &AgentProcess{
        PID:    cmd.Process.Pid,
        StepID: stepID,
        Done:   make(chan struct{}),
    }

    // Stream logs to Supabase in real-time
    go func() {
        scanner := bufio.NewScanner(io.MultiReader(stdout, stderr))
        for scanner.Scan() {
            insertRunLog(stepID, "INFO", scanner.Text())
        }
        close(proc.Done)
    }()

    return proc, nil
}
```

**Why isolation?**
- The Coder Agent can run for minutes (compile, test, iterate)
- The main Go-Runner must continue monitoring other workflows
- Clean process group management allows graceful kill on timeout or cancellation

---

## 11. Skill Storage & Sync (from SD-07 §3)

Custom skills must be synced to remote storage for backup and cross-project sharing. This follows the same tiered strategy as Artifacts:

```go
// internal/skills/sync.go
func syncSkills(project Project) error {
    skillDirs := []string{
        filepath.Join(project.DirectoryPath, ".claude", "skills"),
        filepath.Join(project.DirectoryPath, ".codex", "skills"),
        filepath.Join(project.DirectoryPath, ".gemini", "skills"),
    }

    for _, dir := range skillDirs {
        if _, err := os.Stat(dir); os.IsNotExist(err) {
            continue
        }
        if project.ArtifactStoragePreference == "google_drive" && project.HasGoogleDriveMCP {
            // Upload via Google Drive MCP Edge Function
            err = uploadDirectoryToDrive(dir, project.GoogleDriveFolderID)
        } else {
            // Default: Upload to Supabase Storage
            bucketPath := fmt.Sprintf("skills/%s/%s", project.ID, filepath.Base(dir))
            err = uploadDirectoryToSupabase(dir, bucketPath)
        }
        
        if err != nil {
            log.Warnf("Failed to sync %s: %v", dir, err)
        }
    }
    return nil
}
```

Triggered:
- After each workflow run completes
- On manual `flowpilot sync-skills <project_id>` command
- On schedule (if configured)

---

## 12. Definition of Done — CP-08

### Go-Runner CLI
- [ ] Cobra CLI with commands: `run`, `install-provider`, `install-tools`, `init-project`, `sync-skills`, `cache-clear`, `status`, `version`
- [ ] RTK + GitNexus detection and installation (`install-tools`)
- [ ] Claude/Codex/Gemini detection, installation, and auth (`install-provider`)

### Prompt Assembly
- [ ] `PromptAssembler` struct with full template structure (System Instruction → Task)
- [ ] Cache-first execution: check `workflow_prompt_cache` → reuse or assemble new
- [ ] Context Resolver integration before prompt assembly
- [ ] Runtime placeholder injection (MCP, user context, selected working memory, source artifacts, raw excerpts, rejection notes)
- [ ] `workflow_prompt_context_items` audit rows written for selected prompt memory

### Provider Execution & Artifacts
- [ ] `executeProvider()` with provider-specific CLI commands
- [ ] Code/Review Loop special case with long-running sub-process
- [ ] Exit criteria monitoring (build, tests, coverage)
- [ ] Process isolation via `os/exec.Command` with process group management
- [ ] Write artifacts locally to `.artifacts/` folder
- [ ] Upload artifacts to Supabase Storage Bucket via API (or Google Drive if configured)
- [ ] Generate `artifact_memories` records after artifact save
- [ ] Call `generate-embedding` and store embedding status

### Project Files
- [ ] `init-project` generates `CLAUDE.md` / `AGENTS.md` / `GEMINI.md`
- [ ] Built-in skill sync from backend to `.claude/skills/`, `.codex/skills/`, `.gemini/skills/`
- [ ] Sync custom skills to Supabase Storage (or Google Drive if configured)

### Configuration
- [ ] Provider/model resolution: step override → run override → project default
- [ ] `workflow_run_logs` table for real-time log streaming
- [ ] `step_definitions` seed table with 17 MVP step types
