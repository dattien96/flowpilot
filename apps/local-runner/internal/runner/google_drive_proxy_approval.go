package runner

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	googleDriveProxyWorkflowRunIDEnv  = "FLOWPILOT_WORKFLOW_RUN_ID"
	googleDriveProxyWorkflowStepIDEnv = "FLOWPILOT_WORKFLOW_STEP_RUN_ID"
	googleDriveProxyProcessKeyEnv     = "FLOWPILOT_PROCESS_KEY"
)

type googleDriveProxyApprovalState struct {
	Version int                                       `json:"version"`
	Records map[string]googleDriveProxyApprovalRecord `json:"records"`
}

type googleDriveProxyApprovalRecord struct {
	ID                string `json:"id"`
	WorkflowRunID     string `json:"workflowRunId,omitempty"`
	WorkflowStepRunID string `json:"workflowStepRunId,omitempty"`
	ProcessKey        string `json:"processKey,omitempty"`
	AccountHomePath   string `json:"accountHomePath,omitempty"`
	ToolName          string `json:"toolName"`
	CanonicalArgsJSON string `json:"canonicalArgsJson"`
	ArgumentsHash     string `json:"argumentsHash"`
	TargetSummary     string `json:"targetSummary,omitempty"`
	Status            string `json:"status"`
	DecisionMode      string `json:"decisionMode"`
	DecisionComment   string `json:"decisionComment,omitempty"`
	RequestedAt       string `json:"requestedAt"`
	DecidedAt         string `json:"decidedAt,omitempty"`
	ExpiresAt         string `json:"expiresAt"`
	ResultJSON        string `json:"resultJson,omitempty"`
	ResultDriveID     string `json:"resultDriveId,omitempty"`
	ResultDriveURL    string `json:"resultDriveUrl,omitempty"`
	ErrorMessage      string `json:"errorMessage,omitempty"`
}

type googleDriveProxyApprovalDecisionRequest struct {
	Decision string `json:"decision"`
	Comment  string `json:"comment,omitempty"`
}

type GoogleDriveProxyApprovalRecord struct {
	ID                string `json:"id"`
	WorkflowRunID     string `json:"workflowRunId,omitempty"`
	WorkflowStepRunID string `json:"workflowStepRunId,omitempty"`
	ProcessKey        string `json:"processKey,omitempty"`
	AccountHomePath   string `json:"accountHomePath,omitempty"`
	ToolName          string `json:"toolName"`
	CanonicalArgsJSON string `json:"canonicalArgsJson"`
	ArgumentsHash     string `json:"argumentsHash"`
	TargetSummary     string `json:"targetSummary,omitempty"`
	Status            string `json:"status"`
	DecisionMode      string `json:"decisionMode"`
	DecisionComment   string `json:"decisionComment,omitempty"`
	RequestedAt       string `json:"requestedAt"`
	DecidedAt         string `json:"decidedAt,omitempty"`
	ExpiresAt         string `json:"expiresAt"`
	ResultJSON        string `json:"resultJson,omitempty"`
	ResultDriveID     string `json:"resultDriveId,omitempty"`
	ResultDriveURL    string `json:"resultDriveUrl,omitempty"`
	ErrorMessage      string `json:"errorMessage,omitempty"`
}

type GoogleDriveProxyApprovalDecisionRequest struct {
	Decision string `json:"decision"`
	Comment  string `json:"comment,omitempty"`
}

func toGoogleDriveProxyApprovalRecord(record googleDriveProxyApprovalRecord) GoogleDriveProxyApprovalRecord {
	return GoogleDriveProxyApprovalRecord{
		ID:                record.ID,
		WorkflowRunID:     record.WorkflowRunID,
		WorkflowStepRunID: record.WorkflowStepRunID,
		ProcessKey:        record.ProcessKey,
		AccountHomePath:   record.AccountHomePath,
		ToolName:          record.ToolName,
		CanonicalArgsJSON: record.CanonicalArgsJSON,
		ArgumentsHash:     record.ArgumentsHash,
		TargetSummary:     record.TargetSummary,
		Status:            record.Status,
		DecisionMode:      record.DecisionMode,
		DecisionComment:   record.DecisionComment,
		RequestedAt:       record.RequestedAt,
		DecidedAt:         record.DecidedAt,
		ExpiresAt:         record.ExpiresAt,
		ResultJSON:        record.ResultJSON,
		ResultDriveID:     record.ResultDriveID,
		ResultDriveURL:    record.ResultDriveURL,
		ErrorMessage:      record.ErrorMessage,
	}
}

func (r *Runner) googleDriveProxyApprovalStatePath() string {
	return filepath.Join(r.workspace, ".flowpilot", "google-drive-proxy-approvals.json")
}

func (r *Runner) loadGoogleDriveProxyApprovalState() (googleDriveProxyApprovalState, error) {
	path := r.googleDriveProxyApprovalStatePath()
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return googleDriveProxyApprovalState{
				Version: 1,
				Records: map[string]googleDriveProxyApprovalRecord{},
			}, nil
		}
		return googleDriveProxyApprovalState{}, err
	}

	var state googleDriveProxyApprovalState
	if err := json.Unmarshal(raw, &state); err != nil {
		return googleDriveProxyApprovalState{}, err
	}
	if state.Version == 0 {
		state.Version = 1
	}
	if state.Records == nil {
		state.Records = map[string]googleDriveProxyApprovalRecord{}
	}

	expireGoogleDriveProxyApprovals(&state, time.Now().UTC())
	return state, nil
}

func (r *Runner) saveGoogleDriveProxyApprovalState(state googleDriveProxyApprovalState) error {
	if state.Version == 0 {
		state.Version = 1
	}
	if state.Records == nil {
		state.Records = map[string]googleDriveProxyApprovalRecord{}
	}

	path := r.googleDriveProxyApprovalStatePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

func (r *Runner) listGoogleDriveProxyApprovals(workflowRunID, workflowStepRunID, status string) ([]googleDriveProxyApprovalRecord, error) {
	state, err := r.loadGoogleDriveProxyApprovalState()
	if err != nil {
		return nil, err
	}
	filtered := make([]googleDriveProxyApprovalRecord, 0, len(state.Records))
	for _, record := range state.Records {
		if strings.TrimSpace(workflowRunID) != "" && record.WorkflowRunID != strings.TrimSpace(workflowRunID) {
			continue
		}
		if strings.TrimSpace(workflowStepRunID) != "" && record.WorkflowStepRunID != strings.TrimSpace(workflowStepRunID) {
			continue
		}
		if strings.TrimSpace(status) != "" && record.Status != strings.TrimSpace(status) {
			continue
		}
		filtered = append(filtered, record)
	}
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].RequestedAt > filtered[j].RequestedAt
	})
	return filtered, nil
}

func (r *Runner) decideGoogleDriveProxyApproval(id string, request googleDriveProxyApprovalDecisionRequest) (googleDriveProxyApprovalRecord, error) {
	state, err := r.loadGoogleDriveProxyApprovalState()
	if err != nil {
		return googleDriveProxyApprovalRecord{}, err
	}
	record, ok := state.Records[strings.TrimSpace(id)]
	if !ok {
		return googleDriveProxyApprovalRecord{}, fmt.Errorf("google drive proxy approval %q not found", id)
	}
	if record.Status != "pending" {
		return googleDriveProxyApprovalRecord{}, fmt.Errorf("google drive proxy approval %q is %s", id, record.Status)
	}

	now := time.Now().UTC()
	switch strings.TrimSpace(request.Decision) {
	case "approved":
		record.Status = "approved"
	case "rejected":
		record.Status = "rejected"
	default:
		return googleDriveProxyApprovalRecord{}, fmt.Errorf("unsupported approval decision %q", request.Decision)
	}
	record.DecisionComment = strings.TrimSpace(request.Comment)
	record.DecidedAt = now.Format(time.RFC3339Nano)
	state.Records[record.ID] = record
	if err := r.saveGoogleDriveProxyApprovalState(state); err != nil {
		return googleDriveProxyApprovalRecord{}, err
	}
	return record, nil
}

func (r *Runner) ListGoogleDriveProxyApprovals(workflowRunID, workflowStepRunID, status string) ([]GoogleDriveProxyApprovalRecord, error) {
	records, err := r.listGoogleDriveProxyApprovals(workflowRunID, workflowStepRunID, status)
	if err != nil {
		return nil, err
	}
	result := make([]GoogleDriveProxyApprovalRecord, 0, len(records))
	for _, record := range records {
		result = append(result, toGoogleDriveProxyApprovalRecord(record))
	}
	return result, nil
}

func (r *Runner) DecideGoogleDriveProxyApproval(id string, request GoogleDriveProxyApprovalDecisionRequest) (GoogleDriveProxyApprovalRecord, error) {
	record, err := r.decideGoogleDriveProxyApproval(id, googleDriveProxyApprovalDecisionRequest{
		Decision: request.Decision,
		Comment:  request.Comment,
	})
	if err != nil {
		return GoogleDriveProxyApprovalRecord{}, err
	}
	return toGoogleDriveProxyApprovalRecord(record), nil
}

func (r *Runner) retireGoogleDriveProxyApprovalsForProcess(processKey string, reason string) error {
	trimmedProcessKey := strings.TrimSpace(processKey)
	if trimmedProcessKey == "" {
		return nil
	}

	state, err := r.loadGoogleDriveProxyApprovalState()
	if err != nil {
		return err
	}

	changed := false
	for id, record := range state.Records {
		if record.ProcessKey != trimmedProcessKey {
			continue
		}
		if record.Status != "pending" && record.Status != "approved" && record.Status != "auto_approved" {
			continue
		}
		record.Status = "expired"
		record.ErrorMessage = strings.TrimSpace(reason)
		state.Records[id] = record
		changed = true
	}

	if !changed {
		return nil
	}

	return r.saveGoogleDriveProxyApprovalState(state)
}

func expireGoogleDriveProxyApprovals(state *googleDriveProxyApprovalState, now time.Time) {
	for id, record := range state.Records {
		if record.Status != "pending" && record.Status != "approved" && record.Status != "auto_approved" {
			continue
		}
		expiresAt, err := time.Parse(time.RFC3339Nano, record.ExpiresAt)
		if err != nil {
			continue
		}
		if now.After(expiresAt) {
			record.Status = "expired"
			record.ErrorMessage = "approval expired before execution"
			state.Records[id] = record
		}
	}
}

func canonicalizeGoogleDriveProxyArguments(args map[string]any) (string, string, error) {
	if args == nil {
		args = map[string]any{}
	}
	raw, err := json.Marshal(args)
	if err != nil {
		return "", "", err
	}
	var normalized any
	if err := json.Unmarshal(raw, &normalized); err != nil {
		return "", "", err
	}
	canonical, err := marshalCanonicalJSON(normalized)
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256(canonical)
	return string(canonical), hex.EncodeToString(sum[:]), nil
}

func marshalCanonicalJSON(value any) ([]byte, error) {
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		builder := strings.Builder{}
		builder.WriteByte('{')
		for i, key := range keys {
			if i > 0 {
				builder.WriteByte(',')
			}
			keyJSON, err := json.Marshal(key)
			if err != nil {
				return nil, err
			}
			valueJSON, err := marshalCanonicalJSON(typed[key])
			if err != nil {
				return nil, err
			}
			builder.Write(keyJSON)
			builder.WriteByte(':')
			builder.Write(valueJSON)
		}
		builder.WriteByte('}')
		return []byte(builder.String()), nil
	case []any:
		builder := strings.Builder{}
		builder.WriteByte('[')
		for i, item := range typed {
			if i > 0 {
				builder.WriteByte(',')
			}
			itemJSON, err := marshalCanonicalJSON(item)
			if err != nil {
				return nil, err
			}
			builder.Write(itemJSON)
		}
		builder.WriteByte(']')
		return []byte(builder.String()), nil
	default:
		return json.Marshal(typed)
	}
}
