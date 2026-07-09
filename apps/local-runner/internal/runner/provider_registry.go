package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// UnsupportedProviderRuntimeError is returned when a provider runtime is requested
// but not implemented/enabled (Claude/Gemini in P2). Mirrors the 04 contract.
type UnsupportedProviderRuntimeError struct {
	ProviderKey ProviderKey
}

func (e *UnsupportedProviderRuntimeError) Error() string {
	return fmt.Sprintf("%s controlled runtime is not implemented yet", e.ProviderKey)
}

// TurnBridge is how an adapter emits normalized events and pauses for user
// interaction. The interactive service implements it; the adapter calls it on its
// own goroutine. RequestApproval/AskQuestion/SpawnAgent BLOCK until resolved, the
// pending record expires, or the turn context is cancelled (interrupt).
type TurnBridge interface {
	Emit(ev ProviderEvent)
	RequestApproval(details ApprovalDetails) (decision string, err error)
	AskQuestion(prompt string, options []QuestionOption, multiSelect bool) (choice []string, err error)
	// SpawnAgent creates a child agent run from the current turn. If in.Wait==true it
	// blocks until the child run's first turn completes and returns its final message.
	SpawnAgent(in SpawnAgentInput) (SpawnAgentResult, error)
	// SubmitFlowControl advances the flow engine on the hub run with a generic
	// continue|done|escalate signal (Task-090).
	SubmitFlowControl(in FlowControlInput) (FlowControlResult, error)
}

// TurnRequest is the per-turn input handed to an adapter.
type TurnRequest struct {
	RunID             string
	StepID            string
	ProjectID         string
	ProviderSessionID string
	ProviderAccountID string
	ProviderTurnID    string
	Prompt            string
	ModelName         string
	SelectedSkills    []SkillSelection
	YoloMode          bool
	ReasoningEffort   string
	// Cwd is the run's active workspace directory (04-06). The adapter binds the
	// provider thread to this cwd; it takes precedence over any adapter default.
	Cwd string
	// Scenario is a P2 fake-adapter hint (mirrors the desktop scenario switcher);
	// the real Codex adapter (P3) ignores it.
	Scenario string
	// Attachments carries chat-turn image attachments (Task-052). Vision-capable
	// adapters convert these into provider-specific multimodal payloads; others ignore them.
	Attachments []PromptAttachment
	// OfferReviewOutcomeTool gates whether the submit_review_outcome/flow_control
	// tool is exposed to the model for this turn. True only for a run actually
	// acting as a flow's hub (rs.autoOrchestrate) — a plain normal_chat run, or a
	// spawned reviewer/coder child, never sees this tool (BUG-NOTE-CP42 #24):
	// previously every turn on every provider unconditionally advertised it,
	// so a model in ordinary chat could call it and mutate that run's loop
	// state (applyFlowControl only checks the run exists, not that it's
	// actually a flow hub).
	OfferReviewOutcomeTool bool
}

// ProviderRuntimeAdapter is the provider-neutral adapter contract (03/04). The
// runner core depends only on this interface + the registry.
type ProviderRuntimeAdapter interface {
	Key() ProviderKey
	Capabilities() ProviderCapabilities
	SendTurn(ctx context.Context, req TurnRequest, bridge TurnBridge) error
}

// ProviderStatus is the registry availability of a provider.
type ProviderStatus string

const (
	ProviderStatusAvailable   ProviderStatus = "available"
	ProviderStatusDisabled    ProviderStatus = "disabled"
	ProviderStatusPlaceholder ProviderStatus = "placeholder"
)

// ProviderRegistration describes a provider in the registry.
type ProviderRegistration struct {
	Key          ProviderKey          `json:"key"`
	DisplayName  string               `json:"displayName"`
	Status       ProviderStatus       `json:"status"`
	Capabilities ProviderCapabilities `json:"capabilities"`
	newAdapter   func() ProviderRuntimeAdapter
}

// ProviderRegistry holds provider registrations in a stable order.
type ProviderRegistry struct {
	order []ProviderKey
	regs  map[ProviderKey]ProviderRegistration
}

func newProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{regs: map[ProviderKey]ProviderRegistration{}}
}

func (r *ProviderRegistry) register(reg ProviderRegistration) {
	if _, ok := r.regs[reg.Key]; !ok {
		r.order = append(r.order, reg.Key)
	}
	r.regs[reg.Key] = reg
}

// List returns registrations in registration order.
func (r *ProviderRegistry) List() []ProviderRegistration {
	out := make([]ProviderRegistration, 0, len(r.order))
	for _, k := range r.order {
		out = append(out, r.regs[k])
	}
	return out
}

// Get returns a registration by key.
func (r *ProviderRegistry) Get(key ProviderKey) (ProviderRegistration, bool) {
	reg, ok := r.regs[key]
	return reg, ok
}

// Adapter returns the adapter for a provider, or UnsupportedProviderRuntimeError if
// the provider is disabled/placeholder (no adapter factory).
func (r *ProviderRegistry) Adapter(key ProviderKey) (ProviderRuntimeAdapter, error) {
	reg, ok := r.regs[key]
	// Only an available provider with a factory yields an adapter; disabled/
	// placeholder providers surface the typed error here (runner-side boundary),
	// regardless of whether a placeholder factory is registered.
	if !ok || reg.newAdapter == nil || reg.Status != ProviderStatusAvailable {
		return nil, &UnsupportedProviderRuntimeError{ProviderKey: key}
	}
	return reg.newAdapter(), nil
}

// Selectable reports whether a provider may be chosen for a controlled run. A
// disabled/placeholder provider (no adapter) is NOT selectable and returns a typed
// UnsupportedProviderRuntimeError — this is the runner-side boundary (04-07): the
// UI gate is a convenience, the runner is the enforcement point.
func (r *ProviderRegistry) Selectable(key ProviderKey) (ProviderRegistration, error) {
	reg, ok := r.regs[key]
	if !ok {
		return ProviderRegistration{}, &UnsupportedProviderRuntimeError{ProviderKey: key}
	}
	if reg.newAdapter == nil || reg.Status != ProviderStatusAvailable {
		return ProviderRegistration{}, &UnsupportedProviderRuntimeError{ProviderKey: key}
	}
	return reg, nil
}

// DefaultProviderKey returns the first available (selectable) provider — the safe
// default for a run when the client does not specify one. A disabled/placeholder
// provider can never be the default. Returns false if none are available.
func (r *ProviderRegistry) DefaultProviderKey() (ProviderKey, bool) {
	for _, k := range r.order {
		if _, err := r.Selectable(k); err == nil {
			return k, true
		}
	}
	return "", false
}

// providerKeyFromModel maps a model name to its provider — the canonical mapping used to
// auto-select the provider for a workflow/step run from its configured model. Mirrors the
// prefix logic in resolvePromptExecutionAdapter (gpt-→codex, claude-→claude,
// gemini-/auto-gemini-→gemini). Returns ("", false) for an unrecognized model.
func providerKeyFromModel(model string) (ProviderKey, bool) {
	m := strings.ToLower(strings.TrimSpace(model))
	switch {
	case strings.HasPrefix(m, "gpt-"):
		return ProviderKeyCodex, true
	case strings.HasPrefix(m, "gemini-"), strings.HasPrefix(m, "auto-gemini-"):
		return ProviderKeyGemini, true
	case strings.HasPrefix(m, "claude-"):
		return ProviderKeyClaude, true
	case strings.HasPrefix(m, "grok-"), m == "grok-build":
		// Appended last (CP-46 P-0): existing prefix cases above are unchanged.
		return ProviderKeyGrok, true
	}
	return "", false
}

// DefaultProviderRegistry builds the P2 registry: Codex backed by the fake adapter
// (the real app-server adapter lands in P3), with Claude/Gemini kept placeholder-safe
// so unsupported runtimes still surface UnsupportedProviderRuntimeError.
func DefaultProviderRegistry() *ProviderRegistry {
	r := newProviderRegistry()
	r.register(ProviderRegistration{
		Key:         ProviderKeyCodex,
		DisplayName: "Codex",
		Status:      ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{
			Streaming: true, Resume: true, ApprovalEvents: true, FileEvents: true,
			SkillSelection: true, Mcp: true, Interrupt: true,
		},
		newAdapter: func() ProviderRuntimeAdapter { return newFakeProviderAdapter(ProviderKeyCodex) },
	})
	r.register(ProviderRegistration{
		Key:          ProviderKeyClaude,
		DisplayName:  "Claude",
		Status:       ProviderStatusPlaceholder,
		Capabilities: ProviderCapabilities{},
		newAdapter:   func() ProviderRuntimeAdapter { return newPlaceholderAdapter(ProviderKeyClaude) },
	})
	r.register(ProviderRegistration{
		Key:          ProviderKeyGemini,
		DisplayName:  "Gemini",
		Status:       ProviderStatusPlaceholder,
		Capabilities: ProviderCapabilities{},
		newAdapter:   func() ProviderRuntimeAdapter { return newPlaceholderAdapter(ProviderKeyGemini) },
	})
	return r
}

// ProviderRegistryFor builds the registry for a live runner. When the Codex
// app-server path is enabled (FLOWPILOT_CODEX_APPSERVER) the Codex registration is
// backed by the real shared-process adapter (ensureCodexAppServer); otherwise the
// fake adapter is kept (demo/tests stay green without a codex binary). Claude is
// backed by its live adapter in the live runner; Gemini is registered with
// conservative ACP capabilities and any unproven parity flags remain disabled.
func ProviderRegistryFor(r *Runner) *ProviderRegistry {
	reg := DefaultProviderRegistry()
	if r == nil {
		return reg
	}
	if codexAppServerEnabled() {
		reg.register(ProviderRegistration{
			Key:         ProviderKeyCodex,
			DisplayName: "Codex",
			Status:      ProviderStatusAvailable,
			Capabilities: ProviderCapabilities{
				Streaming: true, Resume: true, ApprovalEvents: true, FileEvents: true,
				SkillSelection: true, Mcp: true, Interrupt: true,
			},
			newAdapter: func() ProviderRuntimeAdapter {
				scopeKey := "default"
				env := map[string]string{}
				account, err := r.ResolveProviderAccount(string(ProviderKeyCodex), "")
				if err == nil {
					scopeKey = account.ID
					for key, value := range account.ExtraEnv {
						env[key] = value
					}
					if account.HomePath != "" {
						env["CODEX_HOME"] = account.HomePath
					}
				} else if codexHome := strings.TrimSpace(os.Getenv("CODEX_HOME")); codexHome != "" {
					scopeKey = "env:" + codexHome
					env["CODEX_HOME"] = codexHome
				} else {
					return errorAdapter{key: ProviderKeyCodex, err: err}
				}
				h, err := r.ensureCodexAppServer(context.Background(), scopeKey, r.workspace, env)
				if err != nil {
					return errorAdapter{key: ProviderKeyCodex, err: err}
				}
				return h.adapter
			},
		})
	}
	// Claude controlled-mode adapter (07 plan). The factory resolves the active
	// account lazily, so a missing account surfaces as a typed errorAdapter at turn
	// time, not at registration.
	reg.register(ProviderRegistration{
		Key:         ProviderKeyClaude,
		DisplayName: "Claude",
		Status:      ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{
			Streaming: true, Resume: true, ApprovalEvents: true, FileEvents: true,
			SkillSelection: true, Mcp: true, Interrupt: true,
		},
		newAdapter: func() ProviderRuntimeAdapter {
			scopeKey := "default"
			env := map[string]string{}
			account, err := r.ResolveProviderAccount(string(ProviderKeyClaude), "")
			if err == nil {
				scopeKey = account.ID
				for key, value := range account.ExtraEnv {
					env[key] = value
				}
				if account.HomePath != "" {
					env["HOME"] = account.HomePath
					env["XDG_CONFIG_HOME"] = filepath.Join(account.HomePath, ".config")
					env["CLAUDE_CONFIG_DIR"] = filepath.Join(account.HomePath, ".claude")
					if drive, path, ok := windowsHomeDriveAndPath(account.HomePath); ok {
						env["USERPROFILE"] = account.HomePath
						env["APPDATA"] = filepath.Join(account.HomePath, "AppData", "Roaming")
						env["LOCALAPPDATA"] = filepath.Join(account.HomePath, "AppData", "Local")
						env["HOMEDRIVE"] = drive
						env["HOMEPATH"] = path
					}
					// Gating only engages if the config dir has no broad allow-rules
					// (spike finding). Seed a gating posture into FlowPilot's managed
					// dir (best-effort; never clobbers an existing settings.json).
					_ = ensureClaudeConfigSettings(env["CLAUDE_CONFIG_DIR"])
				}
			} else if apiKey := strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY")); apiKey != "" {
				scopeKey = "env:anthropic"
				// Isolate config so ambient ~/.claude allow-rules can't silently bypass
				// gating (review finding 2): the env-key path authenticates via the API key,
				// so a fresh FlowPilot-managed dir (gating posture, no allow-list) is safe.
				if cfgDir := flowpilotManagedClaudeConfigDir(r.workspace); cfgDir != "" {
					env["CLAUDE_CONFIG_DIR"] = cfgDir
					_ = ensureClaudeConfigSettings(cfgDir)
				}
			} else {
				return errorAdapter{key: ProviderKeyClaude, err: err}
			}
			pool := r.claudePool
			if pool == nil {
				pool = newClaudeProcessPool()
			}
			// mcpConfig is empty until the live FlowPilot MCP server is wired (07
			// Appendix A spike); the adapter handles the in-stream control_request
			// route meanwhile. promptPrep injects skill content + ask_user reinforcement.
			a := newClaudeAdapter(pool, r.workspace, scopeKey, env, "")
			// Durable (run, cwd, session) persistence for cross-restart resume (07);
			// no-op when Supabase is unconfigured.
			a.sessionStore = ProviderSessionStoreFor(r)
			// Per-turn permission MCP (07): the adapter registers the turn's bridge on
			// the runner-hosted MCP server and points claude's --mcp-config at it.
			a.mcpServer = r.claudeMCP
			a.mcpBaseURL = r.mcpBaseURLValue
			// Merge FlowPilot-managed servers (google-drive) into the per-turn --mcp-config
			// so --strict-mcp-config doesn't hide them. accountHome is "" for the API-key
			// path (no managed account home) → no extras, which is correct.
			accountHome := env["HOME"]
			a.extraMCPServers = func(yolo bool) map[string]claudeMcpServer {
				return r.flowpilotClaudeExtraMCPServers(accountHome, yolo)
			}
			a.promptPrep = func(req TurnRequest) string {
				workspace := r.workspace
				if req.Cwd != "" {
					workspace = req.Cwd
				}
				return r.injectSelectedSkills(workspace, req.Prompt, req.SelectedSkills) + claudeAskUserReinforcement
			}
			return a
		},
	})
	// Gemini controlled-mode fallback runs `agy --print` per turn with a stable
	// FlowPilot project id. This restores chat continuity after the Gemini ACP
	// prototype became incompatible with Antigravity CLI, but it does not expose
	// provider-owned streaming/approval/MCP events.
	reg.register(ProviderRegistration{
		Key:         ProviderKeyGemini,
		DisplayName: "Gemini",
		Status:      ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{
			SkillSelection: true, Interrupt: true,
		},
		newAdapter: func() ProviderRuntimeAdapter {
			scopeKey := "default"
			env := map[string]string{}
			account, err := r.ResolveProviderAccount(string(ProviderKeyGemini), "")
			if err == nil {
				scopeKey = account.ID
				for key, value := range account.ExtraEnv {
					env[key] = value
				}
				if account.HomePath != "" {
					env["GEMINI_HOME"] = account.HomePath
					env["HOME"] = account.HomePath
					env["XDG_CONFIG_HOME"] = filepath.Join(account.HomePath, ".config")
					if drive, path, ok := windowsHomeDriveAndPath(account.HomePath); ok {
						env["USERPROFILE"] = account.HomePath
						env["APPDATA"] = filepath.Join(account.HomePath, "AppData", "Roaming")
						env["LOCALAPPDATA"] = filepath.Join(account.HomePath, "AppData", "Local")
						env["HOMEDRIVE"] = drive
						env["HOMEPATH"] = path
					}
				}
			} else if geminiHome := strings.TrimSpace(os.Getenv("GEMINI_HOME")); geminiHome != "" {
				scopeKey = "env:" + geminiHome
				env["GEMINI_HOME"] = geminiHome
				env["HOME"] = geminiHome
				env["XDG_CONFIG_HOME"] = filepath.Join(geminiHome, ".config")
			} else if hasAnyEnv("GOOGLE_API_KEY", "GEMINI_API_KEY") {
				scopeKey = "env:gemini-api-key"
			} else {
				return errorAdapter{key: ProviderKeyGemini, err: err}
			}
			sessions := r.geminiSessions
			if sessions == nil {
				sessions = newGeminiSessionMap()
			}
			a := newGeminiAdapter(r.workspace, scopeKey, env)
			a.sessions = sessions
			a.sessionStore = ProviderSessionStoreFor(r)
			a.promptPrep = func(req TurnRequest) string {
				workspace := r.workspace
				if req.Cwd != "" {
					workspace = req.Cwd
				}
				return r.injectSelectedSkills(workspace, req.Prompt, req.SelectedSkills)
			}
			a.mcpServer = r.claudeMCP
			a.mcpBaseURL = r.mcpBaseURLValue
			return a
		},
	})
	return reg
}
