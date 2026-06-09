package runner

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type mcpResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *mcpError       `json:"error,omitempty"`
}

type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type proxyMcpTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type proxyMcpServer struct {
	runner            *Runner
	workspace         string
	accountHome       string
	mode              string
	yoloMode          bool
	workflowRunID     string
	workflowStepRunID string
	processKey        string
	stdin             *bufio.Scanner
	stdout            io.Writer
}

func (r *Runner) RunGoogleDriveProxyMcpServer(ctx context.Context, workspace string, accountHome string, mode string, yoloMode bool) error {
	server := &proxyMcpServer{
		runner:            r,
		workspace:         strings.TrimSpace(workspace),
		accountHome:       strings.TrimSpace(accountHome),
		mode:              normalizeGoogleDriveProxyMode(mode),
		yoloMode:          yoloMode,
		workflowRunID:     strings.TrimSpace(os.Getenv(googleDriveProxyWorkflowRunIDEnv)),
		workflowStepRunID: strings.TrimSpace(os.Getenv(googleDriveProxyWorkflowStepIDEnv)),
		processKey:        strings.TrimSpace(os.Getenv(googleDriveProxyProcessKeyEnv)),
		stdin:             bufio.NewScanner(os.Stdin),
		stdout:            os.Stdout,
	}
	server.stdin.Buffer(make([]byte, 64*1024), 10*1024*1024)
	return server.serve(ctx)
}

func normalizeGoogleDriveProxyMode(mode string) string {
	trimmed := strings.ToLower(strings.TrimSpace(mode))
	if trimmed == "read_write" {
		return "read_write"
	}
	return "read_only"
}

func (s *proxyMcpServer) serve(ctx context.Context) error {
	encoder := json.NewEncoder(s.stdout)
	encoder.SetEscapeHTML(false)

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		if !s.stdin.Scan() {
			if err := s.stdin.Err(); err != nil {
				return err
			}
			return nil
		}

		line := strings.TrimSpace(s.stdin.Text())
		if line == "" {
			continue
		}

		var req mcpRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			_ = encoder.Encode(mcpResponse{
				JSONRPC: "2.0",
				Error:   &mcpError{Code: -32700, Message: "parse error", Data: err.Error()},
			})
			continue
		}

		if len(strings.TrimSpace(string(req.ID))) == 0 {
			continue
		}

		resp := s.handleRequest(ctx, req)
		if err := encoder.Encode(resp); err != nil {
			return err
		}
	}
}

func (s *proxyMcpServer) handleRequest(ctx context.Context, req mcpRequest) mcpResponse {
	switch req.Method {
	case "initialize":
		return mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"protocolVersion": "2025-06-18",
				"serverInfo": map[string]any{
					"name":    "flowpilot-google-drive-mcp",
					"version": Version,
				},
				"capabilities": map[string]any{
					"tools": map[string]any{},
				},
			},
		}
	case "notifications/initialized", "ping":
		return mcpResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{}}
	case "tools/list":
		return mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"tools": s.tools(),
			},
		}
	case "tools/call":
		result, err := s.callTool(ctx, req.Params)
		if err != nil {
			return mcpResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error: &mcpError{
					Code:    -32000,
					Message: err.Error(),
				},
			}
		}
		return mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  result,
		}
	default:
		return mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &mcpError{
				Code:    -32601,
				Message: "method not found",
			},
		}
	}
}

func (s *proxyMcpServer) tools() []proxyMcpTool {
	base := []proxyMcpTool{
		{
			Name:        "authGetStatus",
			Description: "Return the current Google Drive auth state for this proxy server.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			Name:        "authListScopes",
			Description: "Return the Google OAuth scopes expected by the proxy server.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			Name:        "authTestFileAccess",
			Description: "Verify that a Drive file can be fetched by id.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"fileId": map[string]any{"type": "string"},
				},
				"required": []string{"fileId"},
			},
		},
		{
			Name:        "search",
			Description: "Search Google Drive files by query.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query":    map[string]any{"type": "string"},
					"pageSize": map[string]any{"type": "integer"},
				},
				"required": []string{"query"},
			},
		},
		{
			Name:        "listFolder",
			Description: "List the direct children of a Drive folder.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"folderId": map[string]any{"type": "string"},
				},
				"required": []string{"folderId"},
			},
		},
		{
			Name:        "listSharedDrives",
			Description: "List shared drives visible to the current Google account.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			Name:        "readGoogleDoc",
			Description: "Read a Google Doc as text.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"fileId": map[string]any{"type": "string"},
				},
				"required": []string{"fileId"},
			},
		},
		{
			Name:        "readGoogleDocPaginated",
			Description: "Read a Google Doc as text. Pagination is not currently modeled separately.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"fileId": map[string]any{"type": "string"},
				},
				"required": []string{"fileId"},
			},
		},
		{
			Name:        "getGoogleDocContent",
			Description: "Return the content of a Google Doc.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"fileId": map[string]any{"type": "string"},
				},
				"required": []string{"fileId"},
			},
		},
		{
			Name:        "getGoogleDocContentPaginated",
			Description: "Return the content of a Google Doc. Pagination is not currently modeled separately.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"fileId": map[string]any{"type": "string"},
				},
				"required": []string{"fileId"},
			},
		},
	}

	if s.mode == "read_write" {
		base = append(base,
			proxyMcpTool{
				Name:        "createGoogleDoc",
				Description: "Create a Google Doc with the supplied title and content.",
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"title":          map[string]any{"type": "string"},
						"content":        map[string]any{"type": "string"},
						"parentFolderId": map[string]any{"type": "string"},
					},
					"required": []string{"title"},
				},
			},
			proxyMcpTool{
				Name:        "updateGoogleDoc",
				Description: "Update a Google Doc title/content by file id.",
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"fileId":  map[string]any{"type": "string"},
						"title":   map[string]any{"type": "string"},
						"content": map[string]any{"type": "string"},
					},
					"required": []string{"fileId"},
				},
			},
			proxyMcpTool{
				Name:        "createFolder",
				Description: "Create a Drive folder.",
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name":           map[string]any{"type": "string"},
						"parentFolderId": map[string]any{"type": "string"},
					},
					"required": []string{"name"},
				},
			},
		)
	}

	return base
}

func (s *proxyMcpServer) callTool(ctx context.Context, rawParams json.RawMessage) (map[string]any, error) {
	var params struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(rawParams, &params); err != nil {
		return nil, err
	}

	switch strings.TrimSpace(params.Name) {
	case "authGetStatus":
		return map[string]any{"content": []any{map[string]any{"type": "text", "text": s.authStatusText()}}, "isError": false}, nil
	case "authListScopes":
		return map[string]any{"content": []any{map[string]any{"type": "text", "text": strings.Join(s.authScopes(), ", ")}}, "isError": false}, nil
	case "authTestFileAccess":
		accessToken, err := s.accessToken()
		if err != nil {
			return nil, err
		}
		fileID, _ := params.Arguments["fileId"].(string)
		file, err := fetchGoogleDriveFileByID(accessToken, fileID)
		if err != nil {
			return nil, err
		}
		return textToolResult("File is accessible: " + formatGoogleDriveFile(file)), nil
	case "search":
		accessToken, err := s.accessToken()
		if err != nil {
			return nil, err
		}
		query, _ := params.Arguments["query"].(string)
		pageSize := intFromAny(params.Arguments["pageSize"], 10)
		files, err := searchGoogleDriveFiles(accessToken, query, pageSize)
		if err != nil {
			return nil, err
		}
		return textToolResult(mustJSON(files)), nil
	case "listFolder":
		accessToken, err := s.accessToken()
		if err != nil {
			return nil, err
		}
		folderID, _ := params.Arguments["folderId"].(string)
		files, err := listGoogleDriveFolderChildren(accessToken, folderID)
		if err != nil {
			return nil, err
		}
		return textToolResult(mustJSON(files)), nil
	case "listSharedDrives":
		accessToken, err := s.accessToken()
		if err != nil {
			return nil, err
		}
		drives, err := listGoogleSharedDrives(accessToken)
		if err != nil {
			return nil, err
		}
		return textToolResult(mustJSON(drives)), nil
	case "readGoogleDoc", "readGoogleDocPaginated", "getGoogleDocContent", "getGoogleDocContentPaginated":
		accessToken, err := s.accessToken()
		if err != nil {
			return nil, err
		}
		fileID, _ := params.Arguments["fileId"].(string)
		content, err := readGoogleDriveDocument(accessToken, fileID)
		if err != nil {
			return nil, err
		}
		return textToolResult(content), nil
	case "createGoogleDoc":
		title, _ := params.Arguments["title"].(string)
		content, _ := params.Arguments["content"].(string)
		parentFolderID, _ := params.Arguments["parentFolderId"].(string)
		return s.handleWriteTool("createGoogleDoc", params.Arguments, func(accessToken string) (string, string, string, error) {
			file, err := createGoogleDoc(accessToken, title, content, parentFolderID)
			if err != nil {
				return "", "", "", err
			}
			return mustJSON(file), file.ID, file.WebViewLink, nil
		})
	case "updateGoogleDoc":
		fileID, _ := params.Arguments["fileId"].(string)
		title, _ := params.Arguments["title"].(string)
		content, _ := params.Arguments["content"].(string)
		return s.handleWriteTool("updateGoogleDoc", params.Arguments, func(accessToken string) (string, string, string, error) {
			file, err := updateGoogleDoc(accessToken, fileID, title, content)
			if err != nil {
				return "", "", "", err
			}
			return mustJSON(file), file.ID, file.WebViewLink, nil
		})
	case "createFolder":
		name, _ := params.Arguments["name"].(string)
		parentFolderID, _ := params.Arguments["parentFolderId"].(string)
		return s.handleWriteTool("createFolder", params.Arguments, func(accessToken string) (string, string, string, error) {
			file, err := createGoogleDriveFolderWithParent(accessToken, parentFolderID, name)
			if err != nil {
				return "", "", "", err
			}
			return mustJSON(file), file.ID, file.WebViewLink, nil
		})
	default:
		return nil, fmt.Errorf("unsupported tool: %s", params.Name)
	}
}

func (s *proxyMcpServer) handleWriteTool(
	toolName string,
	args map[string]any,
	execFn func(accessToken string) (resultJSON string, driveID string, driveURL string, err error),
) (map[string]any, error) {
	if s.mode != "read_write" {
		return nil, errors.New("DRIVE_WRITE_NOT_ALLOWED: write tools are disabled for this run")
	}

	record, err := s.resolveWriteApproval(toolName, args)
	if err != nil {
		return nil, err
	}

	switch record.Status {
	case "pending":
		return nil, fmt.Errorf(
			"mcp_write_approval_required: MCP_WRITE_APPROVAL_REQUIRED: FlowPilot created approval request %s. Wait for user approval before retrying this exact write.",
			record.ID,
		)
	case "rejected":
		return textToolResult(mustJSON(map[string]any{
			"status":     "rejected",
			"approvalId": record.ID,
			"notice":     "The requested Google Drive write was rejected and was not executed.",
		})), nil
	case "executed":
		return textToolResult(record.ResultJSON), nil
	case "approved", "auto_approved":
		accessToken, err := s.accessToken()
		if err != nil {
			return nil, s.markWriteApprovalFailed(record, err)
		}
		resultJSON, driveID, driveURL, err := execFn(accessToken)
		if err != nil {
			return nil, s.markWriteApprovalFailed(record, err)
		}
		if _, err := s.markWriteApprovalExecuted(record, resultJSON, driveID, driveURL); err != nil {
			return nil, err
		}
		return textToolResult(resultJSON), nil
	default:
		return nil, fmt.Errorf("unsupported approval status %q", record.Status)
	}
}

func (s *proxyMcpServer) resolveWriteApproval(toolName string, args map[string]any) (googleDriveProxyApprovalRecord, error) {
	if strings.TrimSpace(s.workflowRunID) == "" {
		return googleDriveProxyApprovalRecord{}, errors.New("workflow run id is required for Google Drive proxy write approvals")
	}
	if strings.TrimSpace(s.workflowStepRunID) == "" {
		return googleDriveProxyApprovalRecord{}, errors.New("workflow step run id is required for Google Drive proxy write approvals")
	}
	if strings.TrimSpace(s.processKey) == "" {
		return googleDriveProxyApprovalRecord{}, errors.New("process key is required for Google Drive proxy write approvals")
	}

	canonicalArgsJSON, argumentsHash, err := canonicalizeGoogleDriveProxyArguments(args)
	if err != nil {
		return googleDriveProxyApprovalRecord{}, err
	}
	state, err := s.runner.loadGoogleDriveProxyApprovalState()
	if err != nil {
		return googleDriveProxyApprovalRecord{}, err
	}

	for _, record := range state.Records {
		if record.WorkflowRunID != s.workflowRunID {
			continue
		}
		if record.WorkflowStepRunID != s.workflowStepRunID {
			continue
		}
		if record.ProcessKey != s.processKey {
			continue
		}
		if record.ToolName != toolName {
			continue
		}
		if record.ArgumentsHash != argumentsHash || record.CanonicalArgsJSON != canonicalArgsJSON {
			continue
		}
		return record, nil
	}

	now := time.Now().UTC()
	expiresAt, err := s.runner.googleDriveProxyApprovalDeadline(s.processKey)
	if err != nil {
		return googleDriveProxyApprovalRecord{}, err
	}
	record := googleDriveProxyApprovalRecord{
		ID:                newRunID(),
		WorkflowRunID:     s.workflowRunID,
		WorkflowStepRunID: s.workflowStepRunID,
		ProcessKey:        s.processKey,
		AccountHomePath:   s.accountHome,
		ToolName:          toolName,
		CanonicalArgsJSON: canonicalArgsJSON,
		ArgumentsHash:     argumentsHash,
		TargetSummary:     summarizeGoogleDriveProxyWrite(toolName, args),
		RequestedAt:       now.Format(time.RFC3339Nano),
		ExpiresAt:         expiresAt.Format(time.RFC3339Nano),
	}
	if s.yoloMode {
		record.Status = "auto_approved"
		record.DecisionMode = "yolo"
		record.DecidedAt = record.RequestedAt
	} else {
		record.Status = "pending"
		record.DecisionMode = "manual"
	}

	state.Records[record.ID] = record
	if err := s.runner.saveGoogleDriveProxyApprovalState(state); err != nil {
		return googleDriveProxyApprovalRecord{}, err
	}
	return record, nil
}

func (s *proxyMcpServer) markWriteApprovalExecuted(record googleDriveProxyApprovalRecord, resultJSON, driveID, driveURL string) (googleDriveProxyApprovalRecord, error) {
	state, err := s.runner.loadGoogleDriveProxyApprovalState()
	if err != nil {
		return googleDriveProxyApprovalRecord{}, err
	}
	current, ok := state.Records[record.ID]
	if !ok {
		return googleDriveProxyApprovalRecord{}, fmt.Errorf("google drive proxy approval %q not found", record.ID)
	}
	current.Status = "executed"
	current.ResultJSON = resultJSON
	current.ResultDriveID = driveID
	current.ResultDriveURL = driveURL
	state.Records[current.ID] = current
	if err := s.runner.saveGoogleDriveProxyApprovalState(state); err != nil {
		return googleDriveProxyApprovalRecord{}, err
	}
	return current, nil
}

func (s *proxyMcpServer) markWriteApprovalFailed(record googleDriveProxyApprovalRecord, execErr error) error {
	state, err := s.runner.loadGoogleDriveProxyApprovalState()
	if err != nil {
		return err
	}
	current, ok := state.Records[record.ID]
	if !ok {
		return execErr
	}
	current.ErrorMessage = execErr.Error()
	state.Records[current.ID] = current
	if err := s.runner.saveGoogleDriveProxyApprovalState(state); err != nil {
		return err
	}
	return execErr
}

func summarizeGoogleDriveProxyWrite(toolName string, args map[string]any) string {
	switch toolName {
	case "createGoogleDoc":
		title, _ := args["title"].(string)
		return fmt.Sprintf("Create Google Doc %q", strings.TrimSpace(title))
	case "updateGoogleDoc":
		fileID, _ := args["fileId"].(string)
		title, _ := args["title"].(string)
		if strings.TrimSpace(title) != "" {
			return fmt.Sprintf("Update Google Doc %s (%q)", strings.TrimSpace(fileID), strings.TrimSpace(title))
		}
		return fmt.Sprintf("Update Google Doc %s", strings.TrimSpace(fileID))
	case "createFolder":
		name, _ := args["name"].(string)
		return fmt.Sprintf("Create Drive folder %q", strings.TrimSpace(name))
	default:
		return toolName
	}
}

func (s *proxyMcpServer) authStatusText() string {
	if flowpilotGoogleDriveProxyMcpEnabled() {
		projectID, connection, err := s.proxyArtifactConnection()
		if err != nil {
			return "status=failed source=artifact_sync error=" + err.Error()
		}
		return fmt.Sprintf(
			"status=%s source=artifact_sync projectId=%s accountEmail=%s folderId=%s",
			strings.TrimSpace(connection.Status),
			projectID,
			strings.TrimSpace(connection.AccountEmail),
			strings.TrimSpace(connection.FolderID),
		)
	}

	status, err := s.runner.googleDriveMcpRuntimeConfig()
	if err != nil {
		return "status=failed error=" + err.Error()
	}
	return fmt.Sprintf("status=%s credentialPath=%s tokenPath=%s", status.Status, status.CredentialPath, status.TokenPath)
}

func (s *proxyMcpServer) authScopes() []string {
	return []string{
		"https://www.googleapis.com/auth/drive.file",
		"https://www.googleapis.com/auth/drive.readonly",
		"https://www.googleapis.com/auth/documents",
	}
}

func (s *proxyMcpServer) accessToken() (string, error) {
	if flowpilotGoogleDriveProxyMcpEnabled() {
		clientID, clientSecret, err := s.proxyOAuthClient()
		if err != nil {
			return "", err
		}
		projectID, _, err := s.proxyArtifactConnection()
		if err != nil {
			return "", err
		}
		creds, err := s.runner.loadGoogleDriveCredentialByProject(projectID)
		if err != nil {
			return "", err
		}
		return googleDriveRefreshAccessToken(clientID, clientSecret, creds.RefreshToken)
	}

	status, err := s.runner.googleDriveMcpRuntimeConfig()
	if err != nil {
		return "", err
	}
	if !status.CredentialExists || !status.CredentialValid {
		return "", errors.New("google drive MCP credential JSON is not configured")
	}
	if !status.TokenExists {
		return "", errors.New("google drive MCP token file is missing")
	}
	credentialData, err := loadGoogleDriveOAuthCredentialJSONFile(status.CredentialPath)
	if err != nil {
		return "", err
	}
	tokenFile, err := readGoogleDriveMcpTokenFile(status.TokenPath)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(tokenFile.RefreshToken) == "" {
		return "", errors.New("google drive MCP token file is missing a refresh token")
	}
	return googleDriveRefreshAccessToken(credentialData.ClientID, credentialData.ClientSecret, tokenFile.RefreshToken)
}

func (s *proxyMcpServer) proxyOAuthClient() (string, string, error) {
	configFile, err := s.runner.loadGoogleDriveWorkspaceConfigFile()
	if err == nil {
		clientID := strings.TrimSpace(configFile.ArtifactSync.ClientID)
		clientSecret, secretErr := s.runner.ensureSecretStore().Get(googleDriveArtifactSyncClientSecretKey)
		if secretErr != nil {
			return "", "", secretErr
		}
		clientSecret = strings.TrimSpace(clientSecret)
		if clientID == "" || clientSecret == "" {
			return "", "", errors.New("FlowPilot proxy Google Drive auth is incomplete")
		}
		return clientID, clientSecret, nil
	}
	if !os.IsNotExist(err) {
		return "", "", err
	}

	clientID := strings.TrimSpace(os.Getenv("GOOGLE_DRIVE_CLIENT_ID"))
	clientSecret := strings.TrimSpace(os.Getenv("GOOGLE_DRIVE_CLIENT_SECRET"))
	if clientID == "" || clientSecret == "" {
		return "", "", errors.New("FlowPilot proxy Google Drive auth is incomplete")
	}
	return clientID, clientSecret, nil
}

func (s *proxyMcpServer) proxyArtifactConnection() (string, artifactStorageGoogleDriveConnectionRecord, error) {
	state, err := s.runner.loadArtifactStorageGoogleDriveState()
	if err != nil {
		return "", artifactStorageGoogleDriveConnectionRecord{}, err
	}

	connected := make([]artifactStorageGoogleDriveConnectionRecord, 0, len(state.Connections))
	for _, connection := range state.Connections {
		if strings.EqualFold(strings.TrimSpace(connection.Status), "connected") && strings.TrimSpace(connection.ProjectID) != "" {
			connected = append(connected, connection)
		}
	}

	switch len(connected) {
	case 0:
		return "", artifactStorageGoogleDriveConnectionRecord{}, errors.New("no artifact-sync Google Drive connection is available for the proxy MCP")
	case 1:
		return strings.TrimSpace(connected[0].ProjectID), connected[0], nil
	default:
		return "", artifactStorageGoogleDriveConnectionRecord{}, errors.New("multiple artifact-sync Google Drive connections are configured; proxy MCP project scoping is still required")
	}
}

func textToolResult(text string) map[string]any {
	return map[string]any{
		"content": []any{map[string]any{"type": "text", "text": text}},
		"isError": false,
	}
}

func mustJSON(value any) string {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Sprintf("json_marshal_error: %v", err)
	}
	return string(raw)
}

func intFromAny(value any, fallback int) int {
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case string:
		if parsed, err := strconv.Atoi(strings.TrimSpace(typed)); err == nil {
			return parsed
		}
	}
	return fallback
}

func formatGoogleDriveFile(file googleDriveFile) string {
	return fmt.Sprintf("%s (%s)", strings.TrimSpace(file.Name), strings.TrimSpace(file.ID))
}

func searchGoogleDriveFiles(accessToken, query string, pageSize int) ([]googleDriveFile, error) {
	escapedQuery := escapeGoogleDriveQueryValue(query)
	values := map[string]string{
		"Authorization": "Bearer " + accessToken,
	}
	endpoint := "https://www.googleapis.com/drive/v3/files?q=" + urlQuery(fmt.Sprintf("fullText contains '%s' and trashed = false", escapedQuery))
	if pageSize > 0 {
		endpoint += "&pageSize=" + strconv.Itoa(pageSize)
	}
	endpoint += "&fields=files(id,name,mimeType,webViewLink,appProperties)&includeItemsFromAllDrives=true&supportsAllDrives=true"

	statusCode, responseBody, err := httpRequestFn(context.Background(), "GET", endpoint, values, nil)
	if err != nil {
		return nil, err
	}
	if statusCode < 200 || statusCode >= 300 {
		return nil, fmt.Errorf("google drive search failed: %d %s", statusCode, strings.TrimSpace(string(responseBody)))
	}

	var response googleDriveListResponse
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return nil, err
	}
	return response.Files, nil
}

func listGoogleSharedDrives(accessToken string) ([]map[string]any, error) {
	statusCode, responseBody, err := httpRequestFn(context.Background(), "GET", "https://www.googleapis.com/drive/v3/drives?fields=drives(id,name)&pageSize=100", map[string]string{
		"Authorization": "Bearer " + accessToken,
	}, nil)
	if err != nil {
		return nil, err
	}
	if statusCode < 200 || statusCode >= 300 {
		return nil, fmt.Errorf("google drive shared drives list failed: %d %s", statusCode, strings.TrimSpace(string(responseBody)))
	}
	var response struct {
		Drives []map[string]any `json:"drives"`
	}
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return nil, err
	}
	return response.Drives, nil
}

func readGoogleDriveDocument(accessToken, fileID string) (string, error) {
	fileID = strings.TrimSpace(fileID)
	if fileID == "" {
		return "", errors.New("fileId is required")
	}

	statusCode, responseBody, err := httpRequestFn(
		context.Background(),
		"GET",
		"https://www.googleapis.com/drive/v3/files/"+urlPathEscape(fileID)+"/export?mimeType=text/plain",
		map[string]string{"Authorization": "Bearer " + accessToken},
		nil,
	)
	if err != nil {
		return "", err
	}
	if statusCode >= 200 && statusCode < 300 {
		return string(responseBody), nil
	}

	statusCode, responseBody, err = httpRequestFn(
		context.Background(),
		"GET",
		"https://www.googleapis.com/drive/v3/files/"+urlPathEscape(fileID)+"?alt=media",
		map[string]string{"Authorization": "Bearer " + accessToken},
		nil,
	)
	if err != nil {
		return "", err
	}
	if statusCode < 200 || statusCode >= 300 {
		return "", fmt.Errorf("google drive document read failed: %d %s", statusCode, strings.TrimSpace(string(responseBody)))
	}
	return string(responseBody), nil
}

func createGoogleDoc(accessToken, title, content, parentFolderID string) (googleDriveFile, error) {
	metadata := map[string]any{
		"name":     strings.TrimSpace(title),
		"mimeType": "application/vnd.google-apps.document",
	}
	if strings.TrimSpace(parentFolderID) != "" {
		metadata["parents"] = []string{strings.TrimSpace(parentFolderID)}
	}
	return createGoogleDriveMultipartFile(
		accessToken,
		"POST",
		"https://www.googleapis.com/upload/drive/v3/files?uploadType=multipart&supportsAllDrives=true&fields=id,name,mimeType,webViewLink,appProperties",
		metadata,
		[]byte(content),
		"text/plain; charset=utf-8",
	)
}

func updateGoogleDoc(accessToken, fileID, title, content string) (googleDriveFile, error) {
	metadata := map[string]any{}
	if strings.TrimSpace(title) != "" {
		metadata["name"] = strings.TrimSpace(title)
	}
	return createGoogleDriveMultipartFile(
		accessToken,
		"PATCH",
		"https://www.googleapis.com/upload/drive/v3/files/"+urlPathEscape(strings.TrimSpace(fileID))+"?uploadType=multipart&supportsAllDrives=true&fields=id,name,mimeType,webViewLink,appProperties",
		metadata,
		[]byte(content),
		"text/plain; charset=utf-8",
	)
}

func createGoogleDriveFolderWithParent(accessToken, parentFolderID, name string) (googleDriveFile, error) {
	metadata := map[string]any{
		"name":     strings.TrimSpace(name),
		"mimeType": googleDriveFolderMimeType,
	}
	if strings.TrimSpace(parentFolderID) != "" {
		metadata["parents"] = []string{strings.TrimSpace(parentFolderID)}
	}
	return createGoogleDriveMultipartFile(
		accessToken,
		"POST",
		"https://www.googleapis.com/upload/drive/v3/files?uploadType=multipart&supportsAllDrives=true&fields=id,name,mimeType,webViewLink,appProperties",
		metadata,
		nil,
		"application/octet-stream",
	)
}

func urlQuery(value string) string {
	return url.QueryEscape(strings.TrimSpace(value))
}

func urlPathEscape(value string) string {
	return url.PathEscape(strings.TrimSpace(value))
}
