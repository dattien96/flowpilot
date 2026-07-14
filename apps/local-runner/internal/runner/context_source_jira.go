package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// Task-229 (CP-05-06 P-3/P-4): jira.issue and jira.sprint are two
// independent context sources (Q-3 resolved 2026-07-13 → separate sources,
// not a shared "target mode") mirroring mcp.driver's shape — each is its own
// ContextSource with its own adapter, registered but opt-in, and excluded
// from the live AI-turn collect path in favor of a runtime-target prompt
// note (see resolveJiraIssueTargetForRun/resolveJiraSprintTargetForRun and
// appendJiraIssueTargetPrompt/appendJiraSprintTargetPrompt in
// flow_executor.go).
const (
	ContextSourceJiraIssue  ContextSourceID = "jira.issue"
	ContextSourceJiraSprint ContextSourceID = "jira.sprint"
)

// JiraIssueAdapter fetches a single Jira issue's bounded content by issue
// key. Mirrors MCPDriverAdapter's shape (context_source_mcp.go) — CP-44 P-7's
// "only MCPs the system already supports/connects" applies here too: the
// production adapter only reaches Jira through the already-connected
// credential (resolveConnectedJiraCredential), never an arbitrary command.
type JiraIssueAdapter interface {
	Fetch(ctx context.Context, issueKey string) (content string, err error)
}

// JiraSprintAdapter fetches a Jira sprint's issue list as bounded content.
// sprintRef is either "active" (the connected board's current active
// sprint) or a specific sprint id/name.
type JiraSprintAdapter interface {
	Fetch(ctx context.Context, sprintRef string) (content string, err error)
}

// jiraIssueSource reads hints.JiraIssueRef. Deliberately excluded from
// defaultContextSourceIDs — a flow must opt in (Task-194 pattern).
type jiraIssueSource struct {
	priority int
	adapter  JiraIssueAdapter
}

func (s *jiraIssueSource) ID() string          { return string(ContextSourceJiraIssue) }
func (s *jiraIssueSource) Priority() int       { return s.priority }
func (s *jiraIssueSource) Deterministic() bool { return true }

func (s *jiraIssueSource) Fetch(ctx context.Context, hints FlowContextHints) (FlowContextSection, error) {
	section := FlowContextSection{SourceType: s.ID(), Priority: s.priority}
	issueKey := strings.TrimSpace(hints.JiraIssueRef)
	if issueKey == "" {
		return section, nil
	}
	if s.adapter == nil {
		return section, fmt.Errorf("jira.issue: no adapter configured")
	}
	section.SourceRef = "jira:" + issueKey
	content, err := mcpBoundedFetch(ctx, 0, mcpDriverTimeout, mcpDriverContentCap, s.ID(), issueKey, s.adapter.Fetch)
	if err != nil {
		return section, err
	}
	section.Body = content
	return section, nil
}

// jiraSprintSource reads hints.JiraSprintRef.
type jiraSprintSource struct {
	priority int
	adapter  JiraSprintAdapter
}

func (s *jiraSprintSource) ID() string          { return string(ContextSourceJiraSprint) }
func (s *jiraSprintSource) Priority() int       { return s.priority }
func (s *jiraSprintSource) Deterministic() bool { return true }

func (s *jiraSprintSource) Fetch(ctx context.Context, hints FlowContextHints) (FlowContextSection, error) {
	section := FlowContextSection{SourceType: s.ID(), Priority: s.priority}
	sprintRef := strings.TrimSpace(hints.JiraSprintRef)
	if sprintRef == "" {
		return section, nil
	}
	if s.adapter == nil {
		return section, fmt.Errorf("jira.sprint: no adapter configured")
	}
	section.SourceRef = "jira-sprint:" + sprintRef
	content, err := mcpBoundedFetch(ctx, 0, mcpDriverTimeout, mcpDriverContentCap, s.ID(), sprintRef, s.adapter.Fetch)
	if err != nil {
		return section, err
	}
	section.Body = content
	return section, nil
}

// SetJiraIssueAdapter wires a production JiraIssueAdapter onto the
// already-registered jira.issue source, mirroring SetMCPDriverAdapter
// (context_source_mcp.go). A no-op if jira.issue isn't registered.
func (r *ContextSourceRegistry) SetJiraIssueAdapter(adapter JiraIssueAdapter) {
	src, err := r.Resolve(string(ContextSourceJiraIssue))
	if err != nil {
		return
	}
	if s, ok := src.(*jiraIssueSource); ok {
		s.adapter = adapter
	}
}

// SetJiraSprintAdapter wires a production JiraSprintAdapter onto the
// already-registered jira.sprint source.
func (r *ContextSourceRegistry) SetJiraSprintAdapter(adapter JiraSprintAdapter) {
	src, err := r.Resolve(string(ContextSourceJiraSprint))
	if err != nil {
		return
	}
	if s, ok := src.(*jiraSprintSource); ok {
		s.adapter = adapter
	}
}

// resolveConnectedJiraCredential loads the credential for whichever Jira
// integration is currently connected for this workspace/runner — the same
// "one active connection per runner process" model googleDriveDriverAdapter
// uses (resolveGoogleDriveAccessTokenForRunner), rather than threading a
// per-call integration id through the ContextSource interface.
func (r *Runner) resolveConnectedJiraCredential() (jiraCredential, error) {
	records, err := r.loadMcpBackendRecords()
	if err != nil {
		return jiraCredential{}, fmt.Errorf("load MCP backend records: %w", err)
	}
	secretKey := strings.TrimSpace(records["jira"].SecretKey)
	if secretKey == "" {
		return jiraCredential{}, errors.New("jira is not connected for this workspace")
	}
	return r.loadJiraCredential(secretKey)
}

// jiraRestIssueAdapter is the production JiraIssueAdapter. It reuses the
// already-shipped, already-tested Jira REST credential/request path
// (executeJiraRequestFn, jiraCredential — CP-05-01/CP-05-02) rather than the
// Atlassian remote MCP transport: CP-05-06 Q-2 resolved "reuse REST for
// pick/list, remote-MCP for the live AI-turn tool calls" — this adapter
// backs the registry/validation path and any direct-collect usage, while the
// live AI turn instead gets a target-note prompt (flow_executor.go) telling
// the provider CLI to use the real jira MCP tools itself.
type jiraRestIssueAdapter struct {
	runner *Runner
}

func (a *jiraRestIssueAdapter) Fetch(ctx context.Context, issueKey string) (string, error) {
	if a.runner == nil {
		return "", errors.New("jira.issue: runner not configured")
	}
	creds, err := a.runner.resolveConnectedJiraCredential()
	if err != nil {
		return "", fmt.Errorf("jira.issue: %w", err)
	}
	if strings.TrimSpace(creds.WorkspaceURL) == "" {
		return "", errors.New("jira.issue: workspaceUrl is not configured for the connected Jira integration")
	}

	endpoint := strings.TrimRight(strings.TrimSpace(creds.WorkspaceURL), "/") +
		"/rest/api/3/issue/" + url.PathEscape(issueKey) + "?fields=summary,status,description,assignee"
	raw, err := executeJiraRequestFn(ctx, http.MethodGet, endpoint, creds, nil)
	if err != nil {
		return "", fmt.Errorf("jira.issue: fetch %q: %w", issueKey, err)
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
	if err := json.Unmarshal(raw, &issue); err != nil {
		return "", fmt.Errorf("jira.issue: parse response for %q: %w", issueKey, err)
	}

	assignee := "Unassigned"
	if issue.Fields.Assignee != nil && strings.TrimSpace(issue.Fields.Assignee.DisplayName) != "" {
		assignee = issue.Fields.Assignee.DisplayName
	}
	var body strings.Builder
	fmt.Fprintf(&body, "Issue: %s\n", firstNonEmpty(issue.Key, issueKey))
	fmt.Fprintf(&body, "Summary: %s\n", issue.Fields.Summary)
	fmt.Fprintf(&body, "Status: %s\n", issue.Fields.Status.Name)
	fmt.Fprintf(&body, "Assignee: %s\n", assignee)
	if issue.Fields.Description != nil {
		fmt.Fprintf(&body, "Description: %v\n", issue.Fields.Description)
	}
	return body.String(), nil
}

// jiraRestSprintAdapter is the production JiraSprintAdapter, reusing the same
// REST credential path. sprintRef "active" (or empty) resolves the connected
// board's current active sprint via the Jira Agile API; any other value is
// treated as an explicit sprint id.
type jiraRestSprintAdapter struct {
	runner *Runner
}

func (a *jiraRestSprintAdapter) Fetch(ctx context.Context, sprintRef string) (string, error) {
	if a.runner == nil {
		return "", errors.New("jira.sprint: runner not configured")
	}
	creds, err := a.runner.resolveConnectedJiraCredential()
	if err != nil {
		return "", fmt.Errorf("jira.sprint: %w", err)
	}
	boardID := strings.TrimSpace(creds.BoardID)
	if boardID == "" {
		return "", errors.New("jira.sprint: no board id configured for the connected Jira integration")
	}
	base := strings.TrimRight(strings.TrimSpace(creds.WorkspaceURL), "/")

	sprintID := strings.TrimSpace(sprintRef)
	sprintName := ""
	if sprintID == "" || strings.EqualFold(sprintID, "active") {
		sprintID, sprintName, err = a.resolveActiveSprintID(ctx, base, boardID, creds)
		if err != nil {
			return "", err
		}
	}

	issuesEndpoint := base + "/rest/agile/1.0/sprint/" + url.PathEscape(sprintID) + "/issue?fields=summary,status"
	raw, err := executeJiraRequestFn(ctx, http.MethodGet, issuesEndpoint, creds, nil)
	if err != nil {
		return "", fmt.Errorf("jira.sprint: fetch sprint %q issues: %w", sprintID, err)
	}

	var payload struct {
		Issues []struct {
			Key    string `json:"key"`
			Fields struct {
				Summary string `json:"summary"`
				Status  struct {
					Name string `json:"name"`
				} `json:"status"`
			} `json:"fields"`
		} `json:"issues"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", fmt.Errorf("jira.sprint: parse sprint %q issues: %w", sprintID, err)
	}

	var body strings.Builder
	if sprintName != "" {
		fmt.Fprintf(&body, "Sprint: %s (id %s)\n", sprintName, sprintID)
	} else {
		fmt.Fprintf(&body, "Sprint: %s\n", sprintID)
	}
	if len(payload.Issues) == 0 {
		body.WriteString("No issues found in this sprint.\n")
		return body.String(), nil
	}
	for _, issue := range payload.Issues {
		fmt.Fprintf(&body, "- %s: %s [%s]\n", issue.Key, issue.Fields.Summary, issue.Fields.Status.Name)
	}
	return body.String(), nil
}

func (a *jiraRestSprintAdapter) resolveActiveSprintID(ctx context.Context, base string, boardID string, creds jiraCredential) (id string, name string, err error) {
	endpoint := base + "/rest/agile/1.0/board/" + url.PathEscape(boardID) + "/sprint?state=active"
	raw, err := executeJiraRequestFn(ctx, http.MethodGet, endpoint, creds, nil)
	if err != nil {
		return "", "", fmt.Errorf("jira.sprint: list active sprints for board %q: %w", boardID, err)
	}
	var payload struct {
		Values []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"values"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", "", fmt.Errorf("jira.sprint: parse active sprint list: %w", err)
	}
	if len(payload.Values) == 0 {
		return "", "", fmt.Errorf("jira.sprint: no active sprint found for board %q", boardID)
	}
	return strconv.Itoa(payload.Values[0].ID), payload.Values[0].Name, nil
}

