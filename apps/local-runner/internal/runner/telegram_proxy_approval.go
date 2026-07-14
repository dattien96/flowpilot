package runner

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Task-233 (DOD-6 revisit): a real, live approval queue for the Telegram
// send_message tool, mirroring google_drive_proxy_approval.go's mechanism
// exactly (pending → user decides via HTTP → AI retries the identical tool
// call → executes once, records the result) rather than the earlier
// static-config-flag shortcut. That flag is kept as a fallback for when no
// run/step/process scope is available (mirrors hasNoApprovalScope's
// auto-execute path on the Drive proxy) — this is not a downgrade from
// Drive's own current behavior, just the same shape applied to Telegram.
type telegramProxyApprovalState struct {
	Version int                                    `json:"version"`
	Records map[string]telegramProxyApprovalRecord `json:"records"`
}

type telegramProxyApprovalRecord struct {
	ID                string `json:"id"`
	WorkflowRunID     string `json:"workflowRunId,omitempty"`
	WorkflowStepRunID string `json:"workflowStepRunId,omitempty"`
	ProcessKey        string `json:"processKey,omitempty"`
	ChatID            string `json:"chatId"`
	Text              string `json:"text"`
	ArgumentsHash     string `json:"argumentsHash"`
	Status            string `json:"status"` // pending|approved|rejected|executed
	DecisionComment   string `json:"decisionComment,omitempty"`
	RequestedAt       string `json:"requestedAt"`
	DecidedAt         string `json:"decidedAt,omitempty"`
	ExpiresAt         string `json:"expiresAt"`
	ResultMessageID   int64  `json:"resultMessageId,omitempty"`
	ErrorMessage      string `json:"errorMessage,omitempty"`
}

// TelegramProxyApprovalRecord is the public (API/UI-facing) shape — kept
// separate from the internal record only for symmetry with Drive's
// exported/internal split; currently identical fields.
type TelegramProxyApprovalRecord = telegramProxyApprovalRecord

type TelegramProxyApprovalDecisionRequest struct {
	Decision string `json:"decision"` // "approved" | "rejected"
	Comment  string `json:"comment,omitempty"`
}

func (r *Runner) telegramProxyApprovalStatePath() string {
	return filepath.Join(r.workspace, ".flowpilot", "telegram-proxy-approvals.json")
}

func (r *Runner) loadTelegramProxyApprovalState() (telegramProxyApprovalState, error) {
	path := r.telegramProxyApprovalStatePath()
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return telegramProxyApprovalState{Version: 1, Records: map[string]telegramProxyApprovalRecord{}}, nil
		}
		return telegramProxyApprovalState{}, err
	}
	var state telegramProxyApprovalState
	if err := json.Unmarshal(raw, &state); err != nil {
		return telegramProxyApprovalState{}, err
	}
	if state.Version == 0 {
		state.Version = 1
	}
	if state.Records == nil {
		state.Records = map[string]telegramProxyApprovalRecord{}
	}
	expireTelegramProxyApprovals(&state, time.Now().UTC())
	return state, nil
}

func (r *Runner) saveTelegramProxyApprovalState(state telegramProxyApprovalState) error {
	if state.Version == 0 {
		state.Version = 1
	}
	if state.Records == nil {
		state.Records = map[string]telegramProxyApprovalRecord{}
	}
	path := r.telegramProxyApprovalStatePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

func expireTelegramProxyApprovals(state *telegramProxyApprovalState, now time.Time) {
	for id, record := range state.Records {
		if record.Status != "pending" && record.Status != "approved" {
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

// ListTelegramProxyApprovals lists approval records, optionally filtered.
func (r *Runner) ListTelegramProxyApprovals(workflowRunID, workflowStepRunID, status string) ([]TelegramProxyApprovalRecord, error) {
	state, err := r.loadTelegramProxyApprovalState()
	if err != nil {
		return nil, err
	}
	filtered := make([]TelegramProxyApprovalRecord, 0, len(state.Records))
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
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].RequestedAt > filtered[j].RequestedAt })
	return filtered, nil
}

// DecideTelegramProxyApproval is the user-facing approve/reject action
// (called from the desktop UI via the runner HTTP API).
func (r *Runner) DecideTelegramProxyApproval(id string, request TelegramProxyApprovalDecisionRequest) (TelegramProxyApprovalRecord, error) {
	state, err := r.loadTelegramProxyApprovalState()
	if err != nil {
		return TelegramProxyApprovalRecord{}, err
	}
	record, ok := state.Records[strings.TrimSpace(id)]
	if !ok {
		return TelegramProxyApprovalRecord{}, fmt.Errorf("telegram proxy approval %q not found", id)
	}
	if record.Status != "pending" {
		return TelegramProxyApprovalRecord{}, fmt.Errorf("telegram proxy approval %q is %s", id, record.Status)
	}
	switch strings.TrimSpace(request.Decision) {
	case "approved":
		record.Status = "approved"
	case "rejected":
		record.Status = "rejected"
	default:
		return TelegramProxyApprovalRecord{}, fmt.Errorf("unsupported approval decision %q", request.Decision)
	}
	record.DecisionComment = strings.TrimSpace(request.Comment)
	record.DecidedAt = time.Now().UTC().Format(time.RFC3339Nano)
	state.Records[record.ID] = record
	if err := r.saveTelegramProxyApprovalState(state); err != nil {
		return TelegramProxyApprovalRecord{}, err
	}
	return record, nil
}

// resolveTelegramToolApproval mirrors proxyMcpServer.resolveToolApproval:
// looks up an existing record matching the exact (run, step, process, chat,
// text) tuple; if none exists, creates a new "pending" one. The AI must
// retry the identical send_message call after the user decides.
func (s *telegramProxyMcpServer) resolveTelegramToolApproval(chatID, text string) (telegramProxyApprovalRecord, error) {
	if s.runner == nil {
		return telegramProxyApprovalRecord{}, errors.New("telegram proxy: runner not configured for approval scope")
	}
	canonicalArgsJSON, argumentsHash, err := canonicalizeGoogleDriveProxyArguments(map[string]any{"chatId": chatID, "text": text})
	if err != nil {
		return telegramProxyApprovalRecord{}, err
	}
	_ = canonicalArgsJSON

	state, err := s.runner.loadTelegramProxyApprovalState()
	if err != nil {
		return telegramProxyApprovalRecord{}, err
	}

	for _, record := range state.Records {
		if record.WorkflowRunID != s.workflowRunID || record.WorkflowStepRunID != s.workflowStepRunID || record.ProcessKey != s.processKey {
			continue
		}
		if record.ArgumentsHash != argumentsHash {
			continue
		}
		return record, nil
	}

	now := time.Now().UTC()
	expiresAt := now.Add(2 * time.Hour)
	if deadline, err := s.runner.googleDriveProxyApprovalDeadline(s.processKey); err == nil {
		expiresAt = deadline
	}
	record := telegramProxyApprovalRecord{
		ID:                newRunID(),
		WorkflowRunID:     s.workflowRunID,
		WorkflowStepRunID: s.workflowStepRunID,
		ProcessKey:        s.processKey,
		ChatID:            chatID,
		Text:              text,
		ArgumentsHash:     argumentsHash,
		Status:            "pending",
		RequestedAt:       now.Format(time.RFC3339Nano),
		ExpiresAt:         expiresAt.Format(time.RFC3339Nano),
	}
	state.Records[record.ID] = record
	if err := s.runner.saveTelegramProxyApprovalState(state); err != nil {
		return telegramProxyApprovalRecord{}, err
	}
	return record, nil
}

func (s *telegramProxyMcpServer) markTelegramApprovalExecuted(record telegramProxyApprovalRecord, messageID int64) error {
	state, err := s.runner.loadTelegramProxyApprovalState()
	if err != nil {
		return err
	}
	current, ok := state.Records[record.ID]
	if !ok {
		return fmt.Errorf("telegram proxy approval %q not found", record.ID)
	}
	current.Status = "executed"
	current.ResultMessageID = messageID
	state.Records[current.ID] = current
	return s.runner.saveTelegramProxyApprovalState(state)
}

func (s *telegramProxyMcpServer) markTelegramApprovalFailed(record telegramProxyApprovalRecord, execErr error) error {
	state, err := s.runner.loadTelegramProxyApprovalState()
	if err != nil {
		return execErr
	}
	current, ok := state.Records[record.ID]
	if !ok {
		return execErr
	}
	current.ErrorMessage = execErr.Error()
	state.Records[current.ID] = current
	_ = s.runner.saveTelegramProxyApprovalState(state)
	return execErr
}
