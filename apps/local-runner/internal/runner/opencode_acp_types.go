package runner

// Task-300 T-1: typed structs for every Opencode ACP message class, mirroring
// grok_acp_types.go. Live-captured shapes from opencode 1.18.18 (see
// testdata/opencode_acp/*.json). These are used for JSON round-trip validation
// (DOD-1); the dispatcher itself works with map[string]any for flexibility.

// OpencodeInitializeParams is the client -> server initialize request params.
type OpencodeInitializeParams struct {
	ProtocolVersion    int                        `json:"protocolVersion"`
	ClientCapabilities OpencodeClientCapabilities `json:"clientCapabilities"`
	ClientInfo         OpencodeClientInfo         `json:"clientInfo"`
}

type OpencodeClientCapabilities struct {
	FS OpencodeFSCapabilities `json:"fs"`
}

type OpencodeFSCapabilities struct {
	ReadTextFile  bool `json:"readTextFile"`
	WriteTextFile bool `json:"writeTextFile"`
}

type OpencodeClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// OpencodeInitializeResult is the server -> client initialize result.
type OpencodeInitializeResult struct {
	ProtocolVersion   int                       `json:"protocolVersion"`
	AgentCapabilities OpencodeAgentCapabilities `json:"agentCapabilities"`
	AuthMethods       []OpencodeAuthMethod      `json:"authMethods,omitempty"`
	AgentInfo         OpencodeAgentInfo         `json:"agentInfo"`
}

type OpencodeAgentCapabilities struct {
	LoadSession         bool                        `json:"loadSession"`
	MCPCapabilities     OpencodeMCPCapabilities     `json:"mcpCapabilities"`
	PromptCapabilities  OpencodePromptCapabilities  `json:"promptCapabilities"`
	SessionCapabilities OpencodeSessionCapabilities `json:"sessionCapabilities"`
}

type OpencodeMCPCapabilities struct {
	HTTP bool `json:"http"`
	SSE  bool `json:"sse"`
}

type OpencodePromptCapabilities struct {
	EmbeddedContext bool `json:"embeddedContext"`
	Image           bool `json:"image"`
}

type OpencodeSessionCapabilities struct {
	Close  map[string]interface{} `json:"close"`
	Fork   map[string]interface{} `json:"fork"`
	List   map[string]interface{} `json:"list"`
	Resume map[string]interface{} `json:"resume"`
}

type OpencodeAuthMethod struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type OpencodeAgentInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// OpencodeSessionNewParams is client -> server session/new params.
type OpencodeSessionNewParams struct {
	Cwd        string        `json:"cwd"`
	McpServers []interface{} `json:"mcpServers"`
	Permission []interface{} `json:"permission,omitempty"`
	Model      string        `json:"model,omitempty"`
	Variant    string        `json:"variant,omitempty"`
}

// OpencodeSessionNewResult is server -> client session/new result.
type OpencodeSessionNewResult struct {
	SessionID     string                 `json:"sessionId"`
	ConfigOptions []OpencodeConfigOption `json:"configOptions,omitempty"`
}

type OpencodeConfigOption struct {
	ID           string                 `json:"id"`
	Name         string                 `json:"name"`
	Category     string                 `json:"category,omitempty"`
	Type         string                 `json:"type"`
	CurrentValue string                 `json:"currentValue"`
	Options      []OpencodeConfigChoice `json:"options,omitempty"`
}

type OpencodeConfigChoice struct {
	Value       string `json:"value"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// OpencodeSessionLoadParams is client -> server session/load params (resume).
type OpencodeSessionLoadParams struct {
	SessionID  string        `json:"sessionId"`
	Cwd        string        `json:"cwd"`
	McpServers []interface{} `json:"mcpServers"`
}

// OpencodePromptParams is client -> server session/prompt params.
type OpencodePromptParams struct {
	SessionID string                `json:"sessionId"`
	Prompt    []OpencodePromptBlock `json:"prompt"`
}

type OpencodePromptBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// OpencodeSessionUpdate is a server -> client notification params.update
type OpencodeSessionUpdate struct {
	SessionUpdate     string                     `json:"sessionUpdate"`
	Content           *OpencodeContent           `json:"content,omitempty"`
	ToolCallID        string                     `json:"toolCallId,omitempty"`
	Title             string                     `json:"title,omitempty"`
	Kind              string                     `json:"kind,omitempty"`
	Status            string                     `json:"status,omitempty"`
	Locations         []OpencodeLocation         `json:"locations,omitempty"`
	RawInput          map[string]interface{}     `json:"rawInput,omitempty"`
	RawOutput         map[string]interface{}     `json:"rawOutput,omitempty"`
	AvailableCommands []OpencodeAvailableCommand `json:"availableCommands,omitempty"`
}

type OpencodeContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type OpencodeLocation struct {
	Path string `json:"path"`
}

type OpencodeAvailableCommand struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// OpencodePermissionRequest is server -> client session/request_permission params.
type OpencodePermissionRequest struct {
	SessionID string                     `json:"sessionId"`
	ToolCall  OpencodeToolCall           `json:"toolCall"`
	Options   []OpencodePermissionOption `json:"options"`
}

type OpencodeToolCall struct {
	ToolCallID string                 `json:"toolCallId"`
	Title      string                 `json:"title"`
	Kind       string                 `json:"kind"`
	Status     string                 `json:"status"`
	Locations  []OpencodeLocation     `json:"locations,omitempty"`
	RawInput   map[string]interface{} `json:"rawInput,omitempty"`
}

type OpencodePermissionOption struct {
	OptionID string `json:"optionId"`
	Kind     string `json:"kind"`
	Name     string `json:"name"`
}

// OpencodePermissionResponse is client -> server permission reply result.
type OpencodePermissionResponse struct {
	Outcome OpencodePermissionOutcome `json:"outcome"`
}

type OpencodePermissionOutcome struct {
	Outcome  string `json:"outcome"` // "selected"
	OptionID string `json:"optionId"`
}

// OpencodePromptResult is server -> client session/prompt result.
type OpencodePromptResult struct {
	StopReason string                 `json:"stopReason"`
	Usage      *OpencodeUsage         `json:"usage,omitempty"`
	Meta       map[string]interface{} `json:"_meta,omitempty"`
}

type OpencodeUsage struct {
	InputTokens      int `json:"inputTokens"`
	OutputTokens     int `json:"outputTokens"`
	TotalTokens      int `json:"totalTokens"`
	ThoughtTokens    int `json:"thoughtTokens,omitempty"`
	CachedReadTokens int `json:"cachedReadTokens,omitempty"`
}

// OpencodeUsageUpdate is server -> client usage_update notification.
type OpencodeUsageUpdate struct {
	Used int          `json:"used"`
	Size int          `json:"size"`
	Cost OpencodeCost `json:"cost"`
}

type OpencodeCost struct {
	Amount   float64 `json:"amount"`
	Currency string  `json:"currency"`
}
