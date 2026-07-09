package runner

// Task-206 (CP-46 T-1): typed Go structs for the Grok ACP wire messages, frozen
// from payloads captured live against a real `grok agent stdio` process (Grok
// Build 0.2.93, live grok.com-authenticated account) during CP-46/Task-206
// authoring. See testdata/grok_acp/live_probe_raw.txt for the raw capture.
//
// These are intentionally thin: only the fields the runner actually reads are
// typed. Notification routing stays on map[string]any (matching the existing
// Codex/Gemini dispatcher style, gemini_acp_transport.go / codex_appserver.go)
// so an unrecognized/new x.ai/* field never breaks parsing — Grok Build is early
// (0.2.93) and its extension surface "may expand across releases" (CP-46 R-2).

// GrokReasoningEffortOption is one entry in initialize's per-model
// reasoningEfforts list (verified live: high/medium/low for grok-4.5 — NOT the
// full none/minimal/low/medium/high/xhigh/max set CP-46 assumed from the spec;
// unmapped FlowPilot efforts must degrade explicitly, GR-35).
type GrokReasoningEffortOption struct {
	ID      string `json:"id"`
	Value   string `json:"value"`
	Label   string `json:"label"`
	Default bool   `json:"default"`
}

// GrokAvailableModel is one entry in initialize/_meta.modelState.availableModels
// (and session/new's models.availableModels).
type GrokAvailableModel struct {
	ModelID string `json:"modelId"`
	Name    string `json:"name"`
	Meta    struct {
		TotalContextTokens      int64                       `json:"totalContextTokens"`
		SupportsReasoningEffort bool                        `json:"supportsReasoningEffort"`
		ReasoningEfforts        []GrokReasoningEffortOption `json:"reasoningEfforts,omitempty"`
	} `json:"_meta"`
}

// GrokInitializeResult is the typed subset of the `initialize` response body
// (live-verified: protocolVersion, agentCapabilities.{promptCapabilities,
// mcpCapabilities}, _meta.agentVersion, _meta.modelState.availableModels).
type GrokInitializeResult struct {
	ProtocolVersion   int `json:"protocolVersion"`
	AgentCapabilities struct {
		PromptCapabilities struct {
			Image           bool `json:"image"`
			Audio           bool `json:"audio"`
			EmbeddedContext bool `json:"embeddedContext"`
		} `json:"promptCapabilities"`
		MCPCapabilities struct {
			HTTP bool `json:"http"`
			SSE  bool `json:"sse"`
		} `json:"mcpCapabilities"`
	} `json:"agentCapabilities"`
	Meta struct {
		AgentVersion string `json:"agentVersion"`
		ModelState   struct {
			CurrentModelID  string               `json:"currentModelId"`
			AvailableModels []GrokAvailableModel `json:"availableModels"`
		} `json:"modelState"`
	} `json:"_meta"`
}

// GrokPermissionOption is one entry in a `session/request_permission` request's
// options[] (per CP-46 live evidence: optionId/name/kind, kind one of
// allow_once/allow_always/reject_once — the request-specific optionId, never a
// hardcoded string, must be echoed back verbatim, Task-208 T-2).
type GrokPermissionOption struct {
	OptionID string `json:"optionId"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
}

// GrokRequestPermissionParams is the server->client `session/request_permission`
// inbound request body.
type GrokRequestPermissionParams struct {
	SessionID string                 `json:"sessionId"`
	ToolCall  map[string]any         `json:"toolCall,omitempty"`
	Options   []GrokPermissionOption `json:"options"`
}

// GrokTokenUsageMeta is the token-usage/model shape carried in a `session/prompt`
// response's `_meta` (live-verified field names — snake_case is NOT used here,
// unlike the sessionUpdate discriminator).
type GrokTokenUsageMeta struct {
	SessionID        string `json:"sessionId"`
	RequestID        string `json:"requestId"`
	PromptID         string `json:"promptId"`
	ModelID          string `json:"modelId"`
	TotalTokens      int64  `json:"totalTokens"`
	InputTokens      int64  `json:"inputTokens"`
	OutputTokens     int64  `json:"outputTokens"`
	CachedReadTokens int64  `json:"cachedReadTokens"`
	ReasoningTokens  int64  `json:"reasoningTokens"`
}

// GrokPromptResult is the typed `session/prompt` response body. Unlike Codex's
// fire-and-forget turn/start + turn.completed notification pair, Grok's
// session/prompt call itself BLOCKS until the prompt turn ends and its own
// result carries the terminal stopReason + token usage (live-verified) — a
// `_x.ai/session_notification{update.sessionUpdate:"turn_completed"}` notification
// also fires but is redundant with this response for the adapter's purposes.
type GrokPromptResult struct {
	StopReason string             `json:"stopReason"`
	Meta       GrokTokenUsageMeta `json:"_meta"`
}
