// Package client provides a thin HTTP+SSE client for the FlowPilot runner
// /client/* API. It mirrors the TypeScript HttpWsRunnerClient contract
// (apps/desktop-flowpilot/src/client/HttpWsRunnerClient.ts) but is
// provider-agnostic: no runner internals are imported (CP-56 boundary rule).
//
// Skill: cli-tui (CP-56)
package client

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	neturl "net/url"
	"strings"
	"time"
)

// HealthResponse mirrors GET /health.
type HealthResponse struct {
	Status        string `json:"status"`
	RunnerVersion string `json:"runnerVersion"`
	Cwd           string `json:"cwd"`
	OS            string `json:"os"`
	StartedAt     string `json:"startedAt"`
}

// Project mirrors the /client/projects catalog entry.
type Project struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Path  string `json:"path"`
	Model string `json:"model,omitempty"`
}

// Workflow mirrors the /client/workflows catalog entry.
type Workflow struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	ProjectID string `json:"projectId"`
}

// Step mirrors the /client/steps catalog entry.
type Step struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	WorkflowID string `json:"workflowId"`
	ProjectID  string `json:"projectId"`
}

// ProviderAccountSummary mirrors /client/provider-accounts (snake_case JSON).
type ProviderAccountSummary struct {
	ID                 string                     `json:"id"`
	ProviderKey        string                     `json:"provider_key"`
	DisplayLabel       string                     `json:"display_label"`
	HomePath           string                     `json:"home_path"`
	IsActive           bool                       `json:"is_active"`
	AuthStatus         string                     `json:"auth_status"`
	UsageSummary       *string                    `json:"usage_summary"`
	Remaining5hPercent *int                       `json:"remaining_5h_percent"`
	Remaining7dPercent *int                       `json:"remaining_7d_percent"`
	Remaining5hResetAt *string                    `json:"remaining_5h_reset_at"`
	Remaining7dResetAt *string                    `json:"remaining_7d_reset_at"`
	UsageDetailLines   []ProviderAccountUsageLine `json:"usage_detail_lines,omitempty"`
}

// ProviderAccountUsageLine is one quota meter from GET /client/provider-accounts.
type ProviderAccountUsageLine struct {
	Label            string `json:"label"`
	RemainingPercent int    `json:"remaining_percent"`
	ResetAt          string `json:"reset_at,omitempty"`
}

// ProviderModel mirrors runner.ProviderModel from GET /providers.
type ProviderModel struct {
	ID                        string   `json:"id"`
	DisplayName               string   `json:"display_name,omitempty"`
	Available                 bool     `json:"available"`
	SupportedReasoningEfforts []string `json:"supported_reasoning_efforts,omitempty"`
	DefaultReasoningEffort    string   `json:"default_reasoning_effort,omitempty"`
	ContextWindowTokens       int64    `json:"context_window_tokens,omitempty"`
	// Name is a legacy/test alias; prefer ID via ModelID().
	Name string `json:"name,omitempty"`
}

// ModelID returns the canonical model identifier.
func (m ProviderModel) ModelID() string {
	if strings.TrimSpace(m.ID) != "" {
		return m.ID
	}
	return strings.TrimSpace(m.Name)
}

// Provider mirrors GET /providers list entry (Desktop localProviders / ChatInput readiness).
type Provider struct {
	Key             string          `json:"key"`
	Name            string          `json:"name,omitempty"`
	Label           string          `json:"label,omitempty"`
	Installed       bool            `json:"installed"`
	InstallStatus   string          `json:"install_status,omitempty"`
	AuthStatus      string          `json:"auth_status,omitempty"`
	DetectedVersion string          `json:"detected_version,omitempty"`
	Version         string          `json:"version,omitempty"`
	InstallHint     string          `json:"installHint,omitempty"`
	Models          []ProviderModel `json:"models,omitempty"`
}

// ProviderSkill mirrors a skill available for a given provider.
type ProviderSkill struct {
	Name        string `json:"name"`
	Path        string `json:"path,omitempty"`
	Description string `json:"description,omitempty"`
	Source      string `json:"source"`
}

// SkillSelection mirrors the runner SkillSelection DTO sent per turn.
type SkillSelection struct {
	Name   string `json:"name"`
	Path   string `json:"path,omitempty"`
	Source string `json:"source"`
}

// PromptAttachment is a base64-encoded image attached to a chat turn (Task-052).
// Data carries raw bytes with no "data:" prefix. MimeType is image/png or image/jpeg.
type PromptAttachment struct {
	ID           string `json:"id"`
	Kind         string `json:"kind"` // "image"
	OriginalName string `json:"originalName"`
	MimeType     string `json:"mimeType"`
	Data         string `json:"data"` // base64, no data: prefix
	SizeBytes    int64  `json:"sizeBytes"`
	Width        int    `json:"width,omitempty"`
	Height       int    `json:"height,omitempty"`
}

// TokenUsageBreakdown carries per-category token counts for a turn.
type TokenUsageBreakdown struct {
	CachedInputTokens     int64 `json:"cachedInputTokens,omitempty"`
	InputTokens           int64 `json:"inputTokens,omitempty"`
	OutputTokens          int64 `json:"outputTokens,omitempty"`
	ReasoningOutputTokens int64 `json:"reasoningOutputTokens,omitempty"`
	TotalTokens           int64 `json:"totalTokens,omitempty"`
}

// TokenUsageSnapshot carries running token usage totals for a run.
type TokenUsageSnapshot struct {
	Last               *TokenUsageBreakdown `json:"last,omitempty"`
	Total              *TokenUsageBreakdown `json:"total,omitempty"`
	ModelContextWindow *int64               `json:"modelContextWindow,omitempty"`
}

// AgentRunSummary summarises a single agent run inside an agent graph.
type AgentRunSummary struct {
	RunID       string `json:"runId"`
	AgentName   string `json:"agentName"`
	Label       string `json:"label,omitempty"` // flow node id e.g. my-reviewer
	Status      string `json:"status"`
	Role        string `json:"role,omitempty"`
	ProviderKey string `json:"providerKey,omitempty"`
}

// AgentLoopState carries loop progress metadata from the orchestrator.
type AgentLoopState struct {
	Status   string `json:"status"`
	Round    int    `json:"round"`
	RoundCap int    `json:"roundCap"`
}

// AgentGraphSnapshot carries the current agent graph for an agent_graph_updated event.
type AgentGraphSnapshot struct {
	ParentRunID string            `json:"parentRunId"`
	Runs        []AgentRunSummary `json:"runs"`
	LoopState   AgentLoopState    `json:"loopState"`
}

// BuiltinFlowOption mirrors /client/chat/builtin-orchestration-options.
type BuiltinFlowOption struct {
	FlowRef     string `json:"flowRef"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// RunHandle mirrors the RunHandle DTO returned by startRun / resumeRun.
type RunHandle struct {
	RunID             string `json:"runId"`
	ProviderSessionID string `json:"providerSessionId"`
	ProviderKey       string `json:"providerKey"`
	Status            string `json:"status"`
	StepID            string `json:"stepId,omitempty"`
	LastEventSeq      int64  `json:"lastEventSeq,omitempty"`
	// RunKind / WorkflowID / FlowRef restore flow chrome after /open (CA-502).
	RunKind    string `json:"runKind,omitempty"`
	WorkflowID string `json:"workflowId,omitempty"`
	FlowRef    string `json:"flowRef,omitempty"`
}

// RunHistoryItem mirrors GET /client/projects/{id}/workflow-runs (desktop listRunHistory).
type RunHistoryItem struct {
	RunID       string `json:"runId"`
	ProjectID   string `json:"projectId"`
	WorkflowID  string `json:"workflowId,omitempty"`
	ProviderKey string `json:"providerKey"`
	Status      string `json:"status"`
	StartedAt   string `json:"startedAt"`
	UpdatedAt   string `json:"updatedAt"`
	LastPrompt  string `json:"lastPrompt,omitempty"`
	LastMessage string `json:"lastMessage,omitempty"`
	RunKind     string `json:"runKind,omitempty"`
	ParentRunID string `json:"parentRunId,omitempty"`
	AgentName   string `json:"agentName,omitempty"`
	SubMode     string `json:"subMode,omitempty"`
	FlowRef     string `json:"flowRef,omitempty"`
}

// RunSnapshot mirrors GET /client/workflow-runs/{runId}.
type RunSnapshot struct {
	RunID           string        `json:"runId"`
	Status          string        `json:"status"`
	PendingGate     *GateInfo     `json:"pendingGate,omitempty"`
	PendingApproval *ApprovalInfo `json:"pendingApproval,omitempty"`
	PendingQuestion *QuestionInfo `json:"pendingQuestion,omitempty"`
}

// GateInfo minimal gate state from a run snapshot.
type GateInfo struct {
	Options        []string `json:"gateOptions"`
	RegressedTests []string `json:"gateRegressedTests,omitempty"`
}

// ApprovalInfo minimal approval state from a run snapshot.
type ApprovalInfo struct {
	ID      string         `json:"approvalId"`
	Details map[string]any `json:"details"`
}

// QuestionInfo minimal question state from a run snapshot.
type QuestionInfo struct {
	ID      string              `json:"questionId"`
	Prompt  string              `json:"prompt"`
	Options []map[string]string `json:"options"`
}

// StartRunInput mirrors the runner StartRunInput DTO.
type StartRunInput struct {
	ProjectID       string `json:"projectId"`
	WorkflowID      string `json:"workflowId,omitempty"`
	StepID          string `json:"stepId,omitempty"`
	ProviderKey     string `json:"providerKey,omitempty"`
	Model           string `json:"model,omitempty"`
	YoloMode        bool   `json:"yoloMode,omitempty"`
	ReasoningEffort string `json:"reasoningEffort,omitempty"`
	// ChatMode is "normal_chat" for provider-chat mode (CP-56 §3.2).
	ChatMode string `json:"chatMode,omitempty"`
	Cwd      string `json:"cwd,omitempty"`
}

// TurnInput mirrors the runner TurnInput DTO (CP-56/BUG-063).
// Model and YoloMode use pointer types so nil means "inherit run-level default".
type TurnInput struct {
	RunID           string             `json:"runId"`
	StepID          string             `json:"stepId,omitempty"`
	Prompt          string             `json:"prompt"`
	ChangeType      string             `json:"changeType,omitempty"`
	SelectedSkills  []SkillSelection   `json:"selectedSkills,omitempty"`
	Attachments     []PromptAttachment `json:"attachments,omitempty"`
	ReasoningEffort string             `json:"reasoningEffort,omitempty"`
	// Model is nil = use run default; empty string = explicit default.
	Model *string `json:"model,omitempty"`
	// YoloMode is nil = inherit run-level; non-nil = per-turn override.
	YoloMode       *bool  `json:"yoloMode,omitempty"`
	SubMode        string `json:"subMode,omitempty"`
	FlowRef        string `json:"flowRef,omitempty"`
	IdempotencyKey string `json:"idempotencyKey,omitempty"`
}

// ProviderEvent mirrors ProviderEvent from provider_event.go (camelCase JSON).
type ProviderEvent struct {
	ID                 string              `json:"id"`
	Seq                int64               `json:"seq"`
	Type               string              `json:"type"`
	WorkflowRunID      string              `json:"workflowRunId"`
	ProviderTurnID     string              `json:"providerTurnId,omitempty"`
	ProviderKey        string              `json:"providerKey"`
	Text               string              `json:"text,omitempty"`
	FinalMessage       string              `json:"finalMessage,omitempty"`
	ToolName           string              `json:"toolName,omitempty"`
	ApprovalID         string              `json:"approvalId,omitempty"`
	QuestionID         string              `json:"questionId,omitempty"`
	Prompt             string              `json:"prompt,omitempty"`
	GateOptions        []string            `json:"gateOptions,omitempty"`
	GateRegressedTests []string            `json:"gateRegressedTests,omitempty"`
	Error              string              `json:"error,omitempty"`
	Recoverable        bool                `json:"recoverable,omitempty"`
	Options            []map[string]string `json:"options,omitempty"`
	MultiSelect        bool                `json:"multiSelect,omitempty"`
	OccurredAt         string              `json:"occurredAt"`
	// TokenUsage is present on token_usage_updated events.
	TokenUsage *TokenUsageSnapshot `json:"tokenUsage,omitempty"`
	// AgentGraph is present on agent_graph_updated events.
	AgentGraph *AgentGraphSnapshot `json:"agentGraph,omitempty"`
}

// APIError represents an error response from the runner API.
type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("runner API error %d (%s): %s", e.Status, e.Code, e.Message)
}

// RetryableCode returns true when the error code should trigger a retry.
func RetryableCode(code string) bool {
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "turn_in_progress", "gate_in_progress", "hub_parked":
		return true
	}
	return false
}

// IsRetryableAPIError checks if an error represents a temporary 409 gate/turn lock.
func IsRetryableAPIError(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		if apiErr.Status == http.StatusConflict {
			return true
		}
		if RetryableCode(apiErr.Code) {
			return true
		}
		msg := strings.ToLower(apiErr.Message)
		return strings.Contains(msg, "gate_in_progress") || strings.Contains(msg, "turn_in_progress")
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "gate_in_progress") || strings.Contains(s, "turn_in_progress")
}

const (
	maxTurnRetries = 15
	turnRetryDelay = 800 * time.Millisecond
	// rpcTimeout bounds non-SSE runner calls so the TUI cannot hang forever on start/post.
	rpcTimeout = 60 * time.Second
)

// Client is a thin HTTP+SSE client for the FlowPilot runner.
type Client struct {
	base    string
	http    *http.Client
	lastSeq map[string]int64
}

// New creates a new Client targeting baseURL (e.g. "http://127.0.0.1:4317").
func New(baseURL string) *Client {
	// Timeout: 0 so SSE streams stay open. Bound dial + response headers so a
	// dead/blackholed runner cannot hang TUI cmds forever (Windows connectex /
	// silent drop previously left sessionLoading + key handling unusable).
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   3 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          32,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		// Long enough to not cut the 30s/45s catalog budgets while still guarding
		// a hung-but-accepting runner; per-call ctx deadlines do the real capping.
		ResponseHeaderTimeout: 60 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	return &Client{
		base: strings.TrimRight(baseURL, "/"),
		http: &http.Client{
			Timeout:   0,
			Transport: transport,
		},
		lastSeq: make(map[string]int64),
	}
}

// Health checks GET /health.
func (c *Client) Health(ctx context.Context) (HealthResponse, error) {
	var h HealthResponse
	err := c.getJSON(ctx, "/health", &h)
	return h, err
}

// ListProjects fetches GET /client/projects.
func (c *Client) ListProjects(ctx context.Context) ([]Project, error) {
	var ps []Project
	err := c.getJSON(ctx, "/client/projects", &ps)
	return ps, err
}

// ShutdownStack sends POST /system/shutdown to terminate the local runner process.
func (c *Client) ShutdownStack(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/system/shutdown", nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// ListWorkflows fetches GET /client/workflows (all; caller filters by projectId).
func (c *Client) ListWorkflows(ctx context.Context) ([]Workflow, error) {
	var ws []Workflow
	err := c.getJSON(ctx, "/client/workflows", &ws)
	return ws, err
}

// ListSteps fetches GET /client/steps (all; caller filters by workflowId).
func (c *Client) ListSteps(ctx context.Context) ([]Step, error) {
	var ss []Step
	err := c.getJSON(ctx, "/client/steps", &ss)
	return ss, err
}

// ListProviderAccounts fetches GET /client/provider-accounts.
func (c *Client) ListProviderAccounts(ctx context.Context) ([]ProviderAccountSummary, error) {
	var accounts []ProviderAccountSummary
	err := c.getJSON(ctx, "/client/provider-accounts", &accounts)
	return accounts, err
}

// ActivateProviderAccount calls POST /provider-accounts/activate to switch the active account for a provider.
func (c *Client) ActivateProviderAccount(ctx context.Context, accountID string) (*ProviderAccountSummary, error) {
	var wrap struct {
		Account struct {
			ProviderAccountSummary
			DisplayName string `json:"display_name"`
		} `json:"account"`
	}
	err := c.postJSON(ctx, "/provider-accounts/activate", map[string]any{
		"accountId": strings.TrimSpace(accountID),
	}, &wrap)
	if err != nil {
		return nil, err
	}
	acc := wrap.Account.ProviderAccountSummary
	if strings.TrimSpace(acc.DisplayLabel) == "" {
		acc.DisplayLabel = strings.TrimSpace(wrap.Account.DisplayName)
	}
	if strings.TrimSpace(acc.ID) == "" {
		return nil, fmt.Errorf("activate returned empty account")
	}
	return &acc, nil
}

// ListProviders fetches GET /providers — returns all detected providers with their models.
func (c *Client) ListProviders(ctx context.Context) ([]Provider, error) {
	var ps []Provider
	err := c.getJSON(ctx, "/providers", &ps)
	return ps, err
}

// ConnectProviderAccount calls POST /provider-accounts/connect (Desktop Settings parity).
// Opens the provider login terminal/browser flow on the runner host.
func (c *Client) ConnectProviderAccount(ctx context.Context, providerKey string) error {
	return c.postJSON(ctx, "/provider-accounts/connect", map[string]any{
		"providerKey": strings.TrimSpace(providerKey),
	}, nil)
}

// InstallProvider calls POST /providers/install (Desktop Settings installLocalProvider parity).
// Runner runs the provider CLI installer (codex/claude/gemini/grok); Desktop UI only
// exposes the Install button for Gemini today, but the API is generic.
func (c *Client) InstallProvider(ctx context.Context, providerName string) ([]Provider, error) {
	var inv struct {
		Providers []Provider `json:"providers"`
	}
	err := c.postJSON(ctx, "/providers/install", map[string]any{
		"providerName": strings.TrimSpace(providerName),
	}, &inv)
	return inv.Providers, err
}

// ListSkills fetches GET /client/provider-skills filtered by optional providerKey and cwd.
func (c *Client) ListSkills(ctx context.Context, providerKey, cwd string) ([]ProviderSkill, error) {
	endpoint := "/client/provider-skills"
	params := make([]string, 0, 2)
	if providerKey != "" {
		params = append(params, "provider="+neturl.QueryEscape(providerKey))
	}
	if cwd != "" {
		params = append(params, "cwd="+neturl.QueryEscape(cwd))
	}
	if len(params) > 0 {
		endpoint += "?" + strings.Join(params, "&")
	}
	var skills []ProviderSkill
	err := c.getJSON(ctx, endpoint, &skills)
	return skills, err
}

// ListAgents fetches GET /client/agents for a given workspace cwd.
func (c *Client) ListAgents(ctx context.Context, cwd string) ([]AgentRunSummary, error) {
	endpoint := "/client/agents"
	if cwd != "" {
		endpoint += "?cwd=" + neturl.QueryEscape(cwd)
	}
	var agents []AgentRunSummary
	err := c.getJSON(ctx, endpoint, &agents)
	return agents, err
}

// ListAgentRuns fetches GET /client/workflow-runs/{runId}/agents (children of a parent run).
func (c *Client) ListAgentRuns(ctx context.Context, parentRunID string) ([]AgentRunSummary, error) {
	var agents []AgentRunSummary
	err := c.getJSON(ctx, "/client/workflow-runs/"+neturl.PathEscape(parentRunID)+"/agents", &agents)
	return agents, err
}

// ListBuiltinOrchestrationOptions fetches GET /client/chat/builtin-orchestration-options.
func (c *Client) ListBuiltinOrchestrationOptions(ctx context.Context, subMode string) ([]BuiltinFlowOption, error) {
	var opts []BuiltinFlowOption
	err := c.getJSON(ctx, "/client/chat/builtin-orchestration-options?subMode="+neturl.QueryEscape(subMode), &opts)
	return opts, err
}

// StartRun sends POST /client/workflow-runs and returns a RunHandle.
func (c *Client) StartRun(ctx context.Context, input StartRunInput) (RunHandle, error) {
	ctx, cancel := context.WithTimeout(ctx, rpcTimeout)
	defer cancel()
	var h RunHandle
	err := c.postJSON(ctx, "/client/workflow-runs", input, &h)
	return h, err
}

// ResumeRun sends POST /client/workflow-runs/{runId}/resume.
func (c *Client) ResumeRun(ctx context.Context, runID string) (RunHandle, error) {
	ctx, cancel := context.WithTimeout(ctx, rpcTimeout)
	defer cancel()
	var h RunHandle
	err := c.postJSON(ctx, "/client/workflow-runs/"+runID+"/resume", nil, &h)
	return h, err
}

// GetRun fetches GET /client/workflow-runs/{runId}.
func (c *Client) GetRun(ctx context.Context, runID string) (RunSnapshot, error) {
	var s RunSnapshot
	err := c.getJSON(ctx, "/client/workflow-runs/"+runID, &s)
	return s, err
}

// ListRunHistory fetches GET /client/projects/{projectId}/workflow-runs.
func (c *Client) ListRunHistory(ctx context.Context, projectID string) ([]RunHistoryItem, error) {
	var items []RunHistoryItem
	err := c.getJSON(ctx, "/client/projects/"+neturl.PathEscape(projectID)+"/workflow-runs", &items)
	return items, err
}

// SubmitApproval sends POST /client/approvals/{approvalId}/decision (Desktop parity).
func (c *Client) SubmitApproval(ctx context.Context, approvalID, decision string, forever bool) error {
	return c.postJSON(ctx, "/client/approvals/"+neturl.PathEscape(approvalID)+"/decision", map[string]any{
		"decision": decision,
		"remember": forever,
		"forever":  forever,
	}, nil)
}

// AnswerQuestion sends POST /client/questions/{questionId}/answer (Desktop parity).
func (c *Client) AnswerQuestion(ctx context.Context, questionID, answer string) error {
	return c.postJSON(ctx, "/client/questions/"+neturl.PathEscape(questionID)+"/answer", map[string]any{
		"choice": answer,
		"answer": answer,
	}, nil)
}

// Interrupt sends POST /client/workflow-runs/{runId}/interrupt.
func (c *Client) Interrupt(ctx context.Context, runID string) error {
	return c.postJSON(ctx, "/client/workflow-runs/"+neturl.PathEscape(runID)+"/interrupt", nil, nil)
}

// StopAgentLoop sends POST /client/workflow-runs/{runId}/agent-loop/stop
// (Desktop Stop: seal the hub and cancel every child).
func (c *Client) StopAgentLoop(ctx context.Context, runID string) error {
	return c.postJSON(ctx, "/client/workflow-runs/"+neturl.PathEscape(runID)+"/agent-loop/stop", nil, nil)
}

// SubmitGateDecision sends POST /client/workflow-runs/{runId}/gate-decision (Desktop parity).
func (c *Client) SubmitGateDecision(ctx context.Context, runID, decision string) error {
	return c.postJSON(ctx, "/client/workflow-runs/"+neturl.PathEscape(runID)+"/gate-decision", map[string]any{
		"option":   decision,
		"decision": decision,
	}, nil)
}

// ApplyGrokYoloPosture calls POST /provider-accounts/grok-yolo-posture.
func (c *Client) ApplyGrokYoloPosture(ctx context.Context, yolo bool) error {
	return c.postJSON(ctx, "/provider-accounts/grok-yolo-posture", map[string]any{
		"yolo": yolo,
	}, nil)
}

// SendTurn posts a turn and yields events for that turn's providerTurnId via SSE.
// Retries retryable errors up to maxTurnRetries times.
func (c *Client) SendTurn(ctx context.Context, input TurnInput) (<-chan ProviderEvent, <-chan error) {
	evCh := make(chan ProviderEvent, 32)
	errCh := make(chan error, 1)

	go func() {
		defer close(evCh)
		defer close(errCh)

		var turnID string
		for attempt := 0; attempt <= maxTurnRetries; attempt++ {
			if attempt > 0 {
				select {
				case <-ctx.Done():
					errCh <- ctx.Err()
					return
				case <-time.After(turnRetryDelay):
				}
			}

			var resp struct {
				TurnID string `json:"turnId"`
			}
			postCtx, cancelPost := context.WithTimeout(ctx, rpcTimeout)
			err := c.postJSON(postCtx, "/client/workflow-runs/"+input.RunID+"/turns", input, &resp)
			cancelPost()
			if err != nil {
				if IsRetryableAPIError(err) && attempt < maxTurnRetries {
					continue
				}
				errCh <- err
				return
			}
			turnID = resp.TurnID
			break
		}

		after := c.lastSeq[input.RunID]
		streamCtx, cancel := context.WithCancel(ctx)
		defer cancel()

		for ev := range c.openStream(streamCtx, input.RunID, after) {
			if ev.Seq > c.lastSeq[input.RunID] {
				c.lastSeq[input.RunID] = ev.Seq
			}
			// Filter to this turn only. Always pass agent_graph_updated (and
			// empty-turnId events) so flow step transitions during a long
			// context.produce / hub turn still reach the TUI.
			if ev.Type != "agent_graph_updated" && ev.ProviderTurnID != "" && ev.ProviderTurnID != turnID {
				continue
			}
			evCh <- ev
			if (ev.Type == "turn_completed" || ev.Type == "turn_failed") && ev.ProviderTurnID == turnID {
				return
			}
		}
	}()

	return evCh, errCh
}

// StreamRun attaches to the SSE event stream after afterSeq.
func (c *Client) StreamRun(ctx context.Context, runID string, afterSeq int64) <-chan ProviderEvent {
	ch := make(chan ProviderEvent, 32)
	go func() {
		defer close(ch)
		for ev := range c.openStream(ctx, runID, afterSeq) {
			if ev.Seq > c.lastSeq[runID] {
				c.lastSeq[runID] = ev.Seq
			}
			select {
			case ch <- ev:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch
}

// StreamWithReconnect attaches to the SSE stream and automatically reconnects
// on disconnect with bounded exponential backoff (max 5 retries, 500ms→8s).
// Each reconnect resumes from the last received Seq so no events are replayed.
func (c *Client) StreamWithReconnect(ctx context.Context, runID string, afterSeq int64) <-chan ProviderEvent {
	ch := make(chan ProviderEvent, 32)
	go func() {
		defer close(ch)
		const maxRetries = 5
		backoff := 500 * time.Millisecond
		for attempt := 0; attempt <= maxRetries; attempt++ {
			if attempt > 0 {
				select {
				case <-ctx.Done():
					return
				case <-time.After(backoff):
					if backoff < 8*time.Second {
						backoff *= 2
					}
				}
			}
			for ev := range c.openStream(ctx, runID, afterSeq) {
				if ev.Seq > afterSeq {
					afterSeq = ev.Seq
				}
				select {
				case ch <- ev:
				case <-ctx.Done():
					return
				}
				if ev.Type == "turn_completed" || ev.Type == "run_completed" {
					return
				}
			}
			if ctx.Err() != nil {
				return
			}
		}
	}()
	return ch
}

// StreamLive attaches to a run SSE and stays open across turn_completed so
// flow orchestration (child graph / late gates) keeps arriving. Caller cancels ctx.
func (c *Client) StreamLive(ctx context.Context, runID string, afterSeq int64) <-chan ProviderEvent {
	ch := make(chan ProviderEvent, 32)
	go func() {
		defer close(ch)
		const maxRetries = 5
		backoff := 500 * time.Millisecond
		for attempt := 0; attempt <= maxRetries; attempt++ {
			if attempt > 0 {
				select {
				case <-ctx.Done():
					return
				case <-time.After(backoff):
					if backoff < 8*time.Second {
						backoff *= 2
					}
				}
			}
			for ev := range c.openStream(ctx, runID, afterSeq) {
				if ev.Seq > afterSeq {
					afterSeq = ev.Seq
				}
				select {
				case ch <- ev:
				case <-ctx.Done():
					return
				}
			}
			if ctx.Err() != nil {
				return
			}
		}
	}()
	return ch
}

// openStream opens GET /client/workflow-runs/{runId}/events/stream and yields ProviderEvents.
func (c *Client) openStream(ctx context.Context, runID string, afterSeq int64) <-chan ProviderEvent {
	ch := make(chan ProviderEvent, 32)
	go func() {
		defer close(ch)

		url := fmt.Sprintf("%s/client/workflow-runs/%s/events/stream?afterSeq=%d", c.base, runID, afterSeq)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return
		}
		req.Header.Set("Accept", "text/event-stream")

		resp, err := c.http.Do(req)
		if err != nil {
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return
		}

		scanner := bufio.NewScanner(resp.Body)
		var dataBuf strings.Builder
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "data:") {
				dataBuf.WriteString(strings.TrimPrefix(line, "data:"))
				dataBuf.WriteString(" ")
			} else if line == "" && dataBuf.Len() > 0 {
				raw := strings.TrimSpace(dataBuf.String())
				dataBuf.Reset()
				if raw == "" || raw == "[DONE]" {
					continue
				}
				var ev ProviderEvent
				if json.Unmarshal([]byte(raw), &ev) == nil {
					select {
					case ch <- ev:
					case <-ctx.Done():
						return
					}
				}
			}
		}
	}()
	return ch
}

// ---- Internal HTTP helpers ---------------------------------------------------

func (c *Client) getJSON(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return parseAPIError(resp.StatusCode, body)
	}
	if out != nil {
		return json.Unmarshal(body, out)
	}
	return nil
}

func (c *Client) postJSON(ctx context.Context, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+path, body)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return parseAPIError(resp.StatusCode, respBody)
	}
	if out != nil {
		return json.Unmarshal(respBody, out)
	}
	return nil
}

func parseAPIError(status int, body []byte) *APIError {
	apiErr := &APIError{Status: status}
	var payload struct {
		Error   any    `json:"error"`
		Message string `json:"message"`
		Code    string `json:"code"`
	}
	if json.Unmarshal(body, &payload) == nil {
		switch v := payload.Error.(type) {
		case string:
			apiErr.Code = v
			apiErr.Message = v
		case map[string]any:
			if code, ok := v["code"].(string); ok {
				apiErr.Code = code
			}
			if msg, ok := v["message"].(string); ok {
				apiErr.Message = msg
			}
		}
		if payload.Code != "" {
			apiErr.Code = payload.Code
		}
		if payload.Message != "" && apiErr.Message == "" {
			apiErr.Message = payload.Message
		}
	}
	if apiErr.Message == "" {
		apiErr.Message = string(body)
	}
	return apiErr
}
