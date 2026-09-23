package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"flowpilot-runner/internal/agentpack"
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
	// Accepted commits a provider acceptance receipt (SD-24 §6.3a / CP-51 Task-249).
	// Only evidence-backed adapters may call this; Codex/Grok have no call site.
	Accepted(receipt ReceiptEvidence)
	// Terminal commits provider-backed terminal proof; sole automatic post-send
	// terminal seam. A SendTurn error is never proof.
	Terminal(proof TerminalEvidence)
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
	// ForceShellBridge (V9-21): flow coding child under YOLO must still surface
	// shell approvals to the runner bridge so git-commit denylist can fire.
	// Adapters map this to non-bypass permission modes while RunnerAutoApprove
	// stays true for ordinary commands.
	ForceShellBridge bool
	ReasoningEffort  string
	// ChatPosture is the per-turn posture ("scan"/"plan"/"code", empty = code).
	// Scan/Plan are read-only: the adapter must use gated permission modes so
	// every tool reaches the bridge, where the read-only policy auto-approves
	// reads and auto-denies writes (never asks).
	ChatPosture string
	// FlowNodePosture is the CP-62 P-4 (Task-340/349) posture declared on the
	// flow node this child turn executes ("read_only" | "verdict_only", empty
	// = standard). Like scan/plan chat postures, a gated flow-node posture
	// must NEVER run under provider bypass modes: the adapters force gated
	// permission modes and clear runner auto-approve so every tool call
	// reaches turnBridge.RequestApproval, where the posture matrix decides.
	FlowNodePosture string
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
	// OfferVibeRequirementTool gates vibe-requirement-outcome (vibe-sprint synthesis only).
	OfferVibeRequirementTool bool
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
	// newAdapterForTurn, when set, is preferred over newAdapter and receives the
	// turn's resolved model/reasoningEffort. Only Grok sets this: Codex/Claude
	// forward model/reasoningEffort as a per-call/per-thread runtime param
	// (thread/start, --model), but Grok's CLI only accepts them as `grok agent`
	// LAUNCH flags (verified via `grok agent --help`) -- there is no ACP
	// session-level way to switch model mid-process. Constructing the adapter
	// for THIS turn must know the model before the shared process is
	// ensured/respawned, so newAdapter's zero-arg shape can't carry it.
	// newAdapterForTurn receives the resolved model/effort plus an optional
	// child-process scope hint (BUG-334): non-empty only for CHILD run turns
	// (rs.parentRunID != ""), asking the factory to isolate its runtime
	// process so a child session/new cannot reset a parent turn's in-flight
	// MCP connection (opencode keys MCP clients by server NAME per process).
	// Grok ignores the hint (its process keying is model-based).
	newAdapterForTurn func(model, reasoningEffort, childScope string) ProviderRuntimeAdapter
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
// the provider is disabled/placeholder (no adapter factory). model/reasoningEffort
// are this turn's resolved values; only a registration with newAdapterForTurn set
// (Grok) actually uses them -- see ProviderRegistration.newAdapterForTurn.
func (r *ProviderRegistry) Adapter(key ProviderKey, model, reasoningEffort string) (ProviderRuntimeAdapter, error) {
	return r.AdapterWithScope(key, model, reasoningEffort, "")
}

// AdapterWithScope is Adapter with a BUG-334 child-process scope hint: pass the
// child run id when the turn belongs to a spawned child so per-provider factories
// can isolate the runtime process from concurrent parent turns.
func (r *ProviderRegistry) AdapterWithScope(key ProviderKey, model, reasoningEffort, childScope string) (ProviderRuntimeAdapter, error) {
	reg, ok := r.regs[key]
	// Only an available provider with a factory yields an adapter; disabled/
	// placeholder providers surface the typed error here (runner-side boundary),
	// regardless of whether a placeholder factory is registered.
	if !ok || (reg.newAdapter == nil && reg.newAdapterForTurn == nil) || reg.Status != ProviderStatusAvailable {
		return nil, &UnsupportedProviderRuntimeError{ProviderKey: key}
	}
	if reg.newAdapterForTurn != nil {
		return reg.newAdapterForTurn(model, reasoningEffort, childScope), nil
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
	if (reg.newAdapter == nil && reg.newAdapterForTurn == nil) || reg.Status != ProviderStatusAvailable {
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
//
// Task-320: delegates to agentpack.ModelProviderKey, the single source of truth
// shared with pack-load validation — behavior is byte-identical to the previous
// inline switch (parity pinned by TestProviderKeyFromModelMatchesPackTable).
func providerKeyFromModel(model string) (ProviderKey, bool) {
	if key, ok := agentpack.ModelProviderKey(model); ok {
		return ProviderKey(key), true
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
				// Codex has no per-turn extraMCPServers live merge (Claude/Grok do):
				// its shared app-server reads mcp_servers from config.toml only at
				// boot. Sync the FlowPilot-managed entries (jira account, google-drive
				// account/token) to the currently-selected accounts before (re)using
				// the process, and force a respawn when either changed so the app-server
				// re-reads them instead of serving values frozen by an earlier Configure
				// Providers run — the reason Codex drifted to a stale Jira/Drive account
				// while Claude/Grok (which re-resolve live) stayed correct.
				if home := strings.TrimSpace(env["CODEX_HOME"]); home != "" {
					jiraChanged, _ := r.syncCodexJiraMcpLive(home)
					driveChanged, _ := r.syncCodexGoogleDriveMcpLive(home)
					if jiraChanged || driveChanged {
						r.resetCodexAppServer()
					}
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
	// Grok Build controlled-mode adapter over ACP `grok agent stdio` (CP-46).
	// On by default (grokAgentEnabled); set FLOWPILOT_GROK_AGENT=0/false/no to
	// opt back out (e.g. test/demo environments without a real grok binary).
	if grokAgentEnabled() {
		reg.register(ProviderRegistration{
			Key:         ProviderKeyGrok,
			DisplayName: "Grok",
			Status:      ProviderStatusAvailable,
			Capabilities: ProviderCapabilities{
				Streaming: true, Resume: true, ApprovalEvents: true, FileEvents: true, Interrupt: true,
				SkillSelection: true, Mcp: true,
			},
			newAdapterForTurn: func(model, reasoningEffort, childScope string) ProviderRuntimeAdapter {
				_ = childScope // Grok keys its process by model/variant; child isolation is opencode-only (BUG-334)
				scopeKey := "default"
				env := map[string]string{}
				account, err := r.ResolveProviderAccount(string(ProviderKeyGrok), "")
				if err == nil {
					scopeKey = account.ID
					for key, value := range account.ExtraEnv {
						env[key] = value
					}
					if account.HomePath != "" {
						env["GROK_HOME"] = account.HomePath
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
				} else if grokHome := strings.TrimSpace(os.Getenv("GROK_HOME")); grokHome != "" {
					scopeKey = "env:" + grokHome
					env["GROK_HOME"] = grokHome
				} else if hasAnyEnv("XAI_API_KEY") {
					scopeKey = "env:xai-api-key"
				} else {
					return errorAdapter{key: ProviderKeyGrok, err: err}
				}
				// model/reasoningEffort are `grok agent` LAUNCH flags, not a per-call
				// param (grok agent --help has no session-level model switch) -- so
				// ensureGrokProcess tears down and respawns the shared process
				// whenever either changes from what it was last launched with, the
				// same way an account switch already forces a respawn.
				//
				// alwaysApprove (Task-218) is read here rather than threaded through
				// this function's own params: ApplyGrokYoloPosture sets it explicitly
				// when the desktop's Grok-only YOLO toggle fires, so ensureGrokProcess
				// always launches (or respawns) under the current desired posture
				// without widening newAdapterForTurn's shared signature that
				// Claude/Codex/Gemini would also have to accept and ignore.
				r.grokProcessMu.Lock()
				alwaysApprove := r.grokDesiredAlwaysApprove
				r.grokProcessMu.Unlock()
				// Task-220 (GR-35): map FlowPilot's canonical reasoning-effort value
				// to a Grok-supported effort id before it becomes the `grok agent
				// --reasoning-effort` launch flag. grok-4.5 only accepts high/medium/
				// low (live-verified), so an unsupported value (e.g. xhigh/max) must
				// degrade to the nearest supported id rather than being passed
				// verbatim and rejected by the CLI. An unmappable value yields "" so
				// ensureGrokProcess omits the flag entirely and the model's own
				// default applies. Mapping here (not in resolveTurnModelAndEffort,
				// which is provider-neutral and also feeds other providers) keeps the
				// transform Grok-local. Passing the mapped id to ensureGrokProcess
				// also makes its respawn-reuse comparison correct: two turns whose raw
				// efforts both degrade to "high" reuse one process instead of churning.
				grokEffort := ""
				if mapped, ok := grokReasoningEffortID(reasoningEffort); ok {
					grokEffort = mapped
				}
				h, ensureErr := r.ensureGrokProcess(context.Background(), scopeKey, r.workspace, env, model, grokEffort, alwaysApprove)
				if ensureErr != nil {
					return errorAdapter{key: ProviderKeyGrok, err: ensureErr}
				}
				a := h.adapter
				a.sessionStore = ProviderSessionStoreFor(r)
				// Task-209 GR-06/GR-07: append grokToolReinforcements after skill
				// injection (mirrors Claude/Codex live promptPrep). Without
				// this, the registry override of preparePrompt drops the
				// default reinforcement and the model prefers native
				// ask_user_question / spawn_subagent (no QuestionCard / agent panel).
				a.promptPrep = func(req TurnRequest) string {
					workspace := r.workspace
					if req.Cwd != "" {
						workspace = req.Cwd
					}
					return r.injectSelectedSkills(workspace, req.Prompt, req.SelectedSkills) + grokToolReinforcements
				}
				a.mcpServer = r.claudeMCP
				a.mcpBaseURL = r.mcpBaseURLValue
				accountHome := env["GROK_HOME"]
				a.extraMCPServers = func(yolo bool) map[string]claudeMcpServer {
					return r.flowpilotClaudeExtraMCPServers(accountHome, yolo)
				}
				return a
			},
		})
	}
	if opencodeAgentEnabled() {
		reg.register(ProviderRegistration{
			Key:         ProviderKeyOpencode,
			DisplayName: "Opencode",
			Status:      ProviderStatusAvailable,
			// BUG-383: registration advertises provider-level capability — a
			// zero-value adapter reports ApprovalEvents/Mcp=false because its
			// mcpServer is only wired per turn. Both are live-verified.
			Capabilities: ProviderCapabilities{
				Streaming: true, Resume: true, ApprovalEvents: true, FileEvents: true,
				Interrupt: true, SkillSelection: true, Mcp: true,
			},
			newAdapterForTurn: func(model, reasoningEffort, childScope string) ProviderRuntimeAdapter {
				// CA-689c: env/scope resolution shared with the variants prober.
				scopeKey, env, envErr := r.opencodeLaunchEnv()
				if envErr != nil {
					return errorAdapter{key: ProviderKeyOpencode, err: envErr}
				}
				// Map reasoningEffort to variant for per-turn respawn key (like Grok grokEffort)
				variant := ""
				if mapped, ok := opencodeReasoningVariantID(reasoningEffort); ok {
					variant = mapped
				}
				// Use provided model if given, else resolve via defaultModelForProvider (like other providers)
				if strings.TrimSpace(model) == "" {
					// Try to resolve default model for opencode if not provided
					if def := defaultModelForProvider(ProviderKeyOpencode); strings.TrimSpace(def) != "" {
						model = def
					}
				}
				r.opencodeProcessMu.Lock()
				auto := r.opencodeDesiredAuto
				r.opencodeProcessMu.Unlock()
				// Also consider YOLO posture's OpencodePermissionMode for --auto, but global auto is SSOT
				// Log the posture for observability (blank identifier was previous dead code)
				_ = resolveYoloPosture(auto).OpencodePermissionMode
				// BUG-334: child runs get their own opencode acp process segment.
				// opencode keys its MCP clients by server NAME per process, so a
				// child session/new (new per-turn MCP token) on the SHARED process
				// replaced the connection mid-parent-turn and killed the parent's
				// in-flight spawn_agent/ask_user call (MCP -32000 Connection
				// closed). Chat turns keep segment "" (shared process, BUG-329
				// reuse intact); child turns isolate under "|child:<runID>".
				h, ensureErr := r.ensureOpencodeProcessSegmented(context.Background(), scopeKey, childScope, r.workspace, env, model, variant, auto)
				if ensureErr != nil {
					return errorAdapter{key: ProviderKeyOpencode, err: ensureErr}
				}
				a := h.adapter
				a.onVariantsCaptured = recordOpencodeModelVariants // CA-689b: live per-model variants
				a.sessionStore = ProviderSessionStoreFor(r)
				a.promptPrep = func(req TurnRequest) string {
					workspace := r.workspace
					if req.Cwd != "" {
						workspace = req.Cwd
					}
					return r.injectSelectedSkills(workspace, req.Prompt, req.SelectedSkills) + opencodeToolReinforcements
				}
				a.mcpServer = r.claudeMCP
				a.mcpBaseURL = r.mcpBaseURLValue
				accountHome := env["HOME"]
				a.extraMCPServers = func(yolo bool) map[string]claudeMcpServer {
					return r.flowpilotClaudeExtraMCPServers(accountHome, yolo)
				}
				return a
			},
		})
	}
	// Devin controlled-mode adapter over `devin acp` (CP-70). On by default
	// (devinAgentEnabled); set FLOWPILOT_DEVIN_AGENT=0/false/no to opt out.
	// Appended last — existing registrations above are unchanged.
	if devinAgentEnabled() {
		reg.register(ProviderRegistration{
			Key:         ProviderKeyDevin,
			DisplayName: "Devin",
			Status:      ProviderStatusAvailable,
			// BUG-383: same zero-value-adapter defect as opencode — advertise
			// the wired capability set, not an unwired instance's.
			Capabilities: ProviderCapabilities{
				Streaming: true, Resume: true, ApprovalEvents: true, FileEvents: true,
				Interrupt: true, SkillSelection: true, Mcp: true,
			},
			newAdapterForTurn: func(model, reasoningEffort, childScope string) ProviderRuntimeAdapter {
				scopeKey, env, envErr := r.devinLaunchEnv()
				if envErr != nil {
					return errorAdapter{key: ProviderKeyDevin, err: envErr}
				}
				if strings.TrimSpace(model) == "" {
					if def := defaultModelForProvider(ProviderKeyDevin); strings.TrimSpace(def) != "" {
						model = def
					}
				}
				// Devin mode (YOLO/posture) is a per-SESSION config option the
				// adapter applies via set_config_option — not a launch flag —
				// so the process key's permissionMode field only keeps the
				// tuple shape; the resolved mode is applied per turn.
				h, ensureErr := r.ensureDevinProcessSegmented(context.Background(), scopeKey, childScope, r.workspace, env, model, "")
				if ensureErr != nil {
					return errorAdapter{key: ProviderKeyDevin, err: ensureErr}
				}
				a := h.adapter
				a.sessionStore = ProviderSessionStoreFor(r)
				a.promptPrep = func(req TurnRequest) string {
					workspace := r.workspace
					if req.Cwd != "" {
						workspace = req.Cwd
					}
					return r.injectSelectedSkills(workspace, req.Prompt, req.SelectedSkills) + devinToolReinforcements
				}
				a.mcpServer = r.claudeMCP
				a.mcpBaseURL = r.mcpBaseURLValue
				accountHome := env["HOME"]
				a.extraMCPServers = func(yolo bool) map[string]claudeMcpServer {
					return r.flowpilotClaudeExtraMCPServers(accountHome, yolo)
				}
				return a
			},
		})
	}
	return reg
}
