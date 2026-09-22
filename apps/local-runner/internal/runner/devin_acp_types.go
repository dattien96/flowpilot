package runner

// Task-400 (CP-70 T-1): typed structs for every Devin ACP message class,
// mirroring opencode_acp_types.go. Live-captured shapes from devin 3000.10.31
// (see testdata/devin_acp/*.json — captured 2026-09-21 against a real
// `devin acp` process). These are used for JSON round-trip validation
// (DOD-1); the dispatcher itself works with map[string]any for flexibility.

// DevinInitializeParams is the client -> server initialize request params.
type DevinInitializeParams struct {
	ProtocolVersion    int                     `json:"protocolVersion"`
	ClientCapabilities DevinClientCapabilities `json:"clientCapabilities"`
	ClientInfo         DevinClientInfo         `json:"clientInfo"`
}

type DevinClientCapabilities struct {
	FS       DevinFSCapabilities `json:"fs"`
	Terminal bool                `json:"terminal"`
}

type DevinFSCapabilities struct {
	ReadTextFile  bool `json:"readTextFile"`
	WriteTextFile bool `json:"writeTextFile"`
}

type DevinClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// DevinInitializeResult is the server -> client initialize result.
// Live (3000.10.31): authMethods=[{id:"devin-browser"}], agentInfo.name is a
// codename (e.g. "affogato"), _meta.mcpConfigPath reports the mcp_config.json
// the agent reads. mcpCapabilities.{http,sse} are both false — Devin accepts
// stdio MCP servers only (session/new mcpServers or the config file).
type DevinInitializeResult struct {
	ProtocolVersion   int                    `json:"protocolVersion"`
	AgentCapabilities DevinAgentCapabilities `json:"agentCapabilities"`
	AuthMethods       []DevinAuthMethod      `json:"authMethods,omitempty"`
	AgentInfo         DevinAgentInfo         `json:"agentInfo"`
	Meta              map[string]interface{} `json:"_meta,omitempty"`
}

type DevinAgentCapabilities struct {
	LoadSession         bool                    `json:"loadSession"`
	MCPCapabilities     DevinMCPCapabilities    `json:"mcpCapabilities"`
	PromptCapabilities  DevinPromptCapabilities `json:"promptCapabilities"`
	SessionCapabilities map[string]interface{}  `json:"sessionCapabilities,omitempty"`
	Auth                map[string]interface{}  `json:"auth,omitempty"`
	Meta                map[string]interface{}  `json:"_meta,omitempty"`
}

type DevinMCPCapabilities struct {
	HTTP bool `json:"http"`
	SSE  bool `json:"sse"`
}

type DevinPromptCapabilities struct {
	Image           bool `json:"image"`
	Audio           bool `json:"audio"`
	EmbeddedContext bool `json:"embeddedContext"`
}

type DevinAuthMethod struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type DevinAgentInfo struct {
	Name    string `json:"name"`
	Title   string `json:"title"`
	Version string `json:"version"`
}

// DevinAuthenticateParams is client -> server authenticate params. Devin ACP
// requires the host to authenticate BEFORE session/new — local CLI
// credentials (~/.local/share/devin/credentials.toml) are NOT consulted in
// ACP mode (live-verified: the agent logs "Waiting for the ACP host to call
// authenticate"). The only advertised method is "devin-browser" (PKCE); when
// the machine's browser profile is already signed in the call completes in
// ~3s with no user interaction and returns {}.
type DevinAuthenticateParams struct {
	MethodID string `json:"methodId"`
}

// DevinSessionNewParams is client -> server session/new params. mcpServers
// accepts stdio entries only — http/sse are rejected capabilities-wise.
type DevinSessionNewParams struct {
	Cwd        string        `json:"cwd"`
	McpServers []interface{} `json:"mcpServers"`
}

// DevinSessionNewResult is server -> client session/new result. Live shape:
// {sessionId:"<adjective-noun slug>", modes:{currentModeId,availableModes},
// configOptions:[{id:"mode"...},{id:"model"...}], _meta}. Session IDs are
// slug-style names (e.g. "working-pentagon") — NOT ses_* and NOT UUIDs.
type DevinSessionNewResult struct {
	SessionID     string                 `json:"sessionId"`
	Modes         *DevinSessionModes     `json:"modes,omitempty"`
	ConfigOptions []DevinConfigOption    `json:"configOptions,omitempty"`
	Meta          map[string]interface{} `json:"_meta,omitempty"`
}

type DevinSessionModes struct {
	CurrentModeID  string            `json:"currentModeId"`
	AvailableModes []DevinModeChoice `json:"availableModes,omitempty"`
}

type DevinModeChoice struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// DevinConfigOption is one entry of the session's configOptions array. The
// "model" option carries the full model catalog (~380 entries live); each
// model option's _meta["cognition.ai/supportsImages"] advertises per-model
// vision capability. The "mode" option carries the five session modes
// (accept-edits/smart/ask/plan/bypass).
type DevinConfigOption struct {
	ID           string              `json:"id"`
	Name         string              `json:"name,omitempty"`
	Description  string              `json:"description,omitempty"`
	Category     string              `json:"category,omitempty"`
	Type         string              `json:"type"`
	CurrentValue string              `json:"currentValue"`
	Options      []DevinConfigChoice `json:"options,omitempty"`
}

type DevinConfigChoice struct {
	Value       string                 `json:"value"`
	Name        string                 `json:"name,omitempty"`
	Description string                 `json:"description,omitempty"`
	Meta        map[string]interface{} `json:"_meta,omitempty"`
}

// DevinSessionLoadParams is client -> server session/load params (resume).
// Live-verified: session/load {sessionId,cwd,mcpServers} replays history and
// returns the modes+configOptions shape (sessionId may be absent — adopt the
// requested id, same BUG-329 rule as OpenCode).
type DevinSessionLoadParams struct {
	SessionID  string        `json:"sessionId"`
	Cwd        string        `json:"cwd"`
	McpServers []interface{} `json:"mcpServers"`
}

// DevinSessionListResult is server -> client session/list result.
type DevinSessionListResult struct {
	Sessions []DevinSessionListEntry `json:"sessions"`
}

type DevinSessionListEntry struct {
	SessionID string                 `json:"sessionId"`
	Cwd       string                 `json:"cwd"`
	Title     string                 `json:"title"`
	UpdatedAt string                 `json:"updatedAt"`
	Meta      map[string]interface{} `json:"_meta,omitempty"`
}

// DevinPromptParams is client -> server session/prompt params.
type DevinPromptParams struct {
	SessionID string             `json:"sessionId"`
	Prompt    []DevinPromptBlock `json:"prompt"`
}

type DevinPromptBlock struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	Data     string `json:"data,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
}

// DevinSessionUpdate is a server -> client session/update params.update.
// sessionUpdate kinds observed live (3000.10.31): agent_message_chunk,
// agent_thought_chunk, user_message_chunk, tool_call, tool_call_update,
// usage_update, session_info_update, current_mode_update,
// available_commands_update, config_option_update, plan.
type DevinSessionUpdate struct {
	SessionUpdate string                 `json:"sessionUpdate"`
	Content       map[string]interface{} `json:"content,omitempty"`
	ToolCallID    string                 `json:"toolCallId,omitempty"`
	Title         string                 `json:"title,omitempty"`
	Kind          string                 `json:"kind,omitempty"`
	Status        string                 `json:"status,omitempty"`
	Locations     []DevinLocation        `json:"locations,omitempty"`
	RawInput      map[string]interface{} `json:"rawInput,omitempty"`
	RawOutput     map[string]interface{} `json:"rawOutput,omitempty"`
	Used          int64                  `json:"used,omitempty"`
	Size          int64                  `json:"size,omitempty"`
	CurrentModeID string                 `json:"currentModeId,omitempty"`
	Meta          map[string]interface{} `json:"_meta,omitempty"`
}

type DevinLocation struct {
	Path string `json:"path"`
}

// DevinPermissionRequest is server -> client session/request_permission params.
type DevinPermissionRequest struct {
	SessionID string                  `json:"sessionId"`
	ToolCall  DevinToolCall           `json:"toolCall"`
	Options   []DevinPermissionOption `json:"options"`
}

type DevinToolCall struct {
	ToolCallID string                 `json:"toolCallId"`
	Title      string                 `json:"title"`
	Kind       string                 `json:"kind"`
	Status     string                 `json:"status"`
	Locations  []DevinLocation        `json:"locations,omitempty"`
	RawInput   map[string]interface{} `json:"rawInput,omitempty"`
}

type DevinPermissionOption struct {
	OptionID string `json:"optionId"`
	Kind     string `json:"kind"`
	Name     string `json:"name"`
}

// DevinPermissionResponse is client -> server permission reply result.
type DevinPermissionResponse struct {
	Outcome DevinPermissionOutcome `json:"outcome"`
}

type DevinPermissionOutcome struct {
	Outcome  string `json:"outcome"` // "selected"
	OptionID string `json:"optionId"`
}

// DevinPromptResult is server -> client session/prompt result. Live shape:
// {stopReason:"end_turn", usage:{totalTokens,inputTokens,outputTokens},
// _meta:{"cognition.ai/userMessageId":...}}.
type DevinPromptResult struct {
	StopReason string                 `json:"stopReason"`
	Usage      *DevinUsage            `json:"usage,omitempty"`
	Meta       map[string]interface{} `json:"_meta,omitempty"`
}

type DevinUsage struct {
	InputTokens  int64 `json:"inputTokens"`
	OutputTokens int64 `json:"outputTokens"`
	TotalTokens  int64 `json:"totalTokens"`
}
