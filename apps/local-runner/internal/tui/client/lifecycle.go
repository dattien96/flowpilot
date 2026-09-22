// CP-81 (SS-24/SD-28): client-side mirror of the runner lifecycle surface —
// /system/lifecycle, /system/clients/*, /system/shutdown, /system/restart.
// DTOs mirror the runner wire contract (and HttpWsRunnerClient.ts); no runner
// internals are imported (CP-56 boundary).
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ClientLeaseView mirrors the runner's lease projection (no token material).
type ClientLeaseView struct {
	LeaseID          string    `json:"leaseId"`
	ClientInstanceID string    `json:"clientInstanceId"`
	Kind             string    `json:"kind"`
	PID              int       `json:"pid"`
	Label            string    `json:"label"`
	ProjectPath      string    `json:"projectPath,omitempty"`
	ProtocolVersion  int       `json:"protocolVersion,omitempty"`
	BuildID          string    `json:"buildId,omitempty"`
	CreatedAt        time.Time `json:"createdAt"`
	LastHeartbeatAt  time.Time `json:"lastHeartbeatAt"`
	ExpiresAt        time.Time `json:"expiresAt"`
}

// WorkloadItem mirrors one unit of protected work in the lifecycle snapshot.
type WorkloadItem struct {
	Kind        string    `json:"kind"`
	RunID       string    `json:"runId,omitempty"`
	ProjectID   string    `json:"projectId,omitempty"`
	ProviderKey string    `json:"providerKey,omitempty"`
	StartedAt   time.Time `json:"startedAt,omitempty"`
	Cancellable bool      `json:"cancellable"`
	Detail      string    `json:"detail,omitempty"`
}

// WorkloadSnapshot mirrors the protected-work inventory.
type WorkloadSnapshot struct {
	Items []WorkloadItem `json:"items"`
}

// ActiveCount is the number of protected items.
func (w WorkloadSnapshot) ActiveCount() int { return len(w.Items) }

// RestartInfo rides the snapshot while phase == draining_restart.
type RestartInfo struct {
	RestartID   string    `json:"restartId"`
	RequestedAt time.Time `json:"requestedAt"`
	Deadline    time.Time `json:"deadline"`
}

// LifecycleSnapshot mirrors GET /system/lifecycle (SD-28 §5).
type LifecycleSnapshot struct {
	RunnerInstanceID  string            `json:"runnerInstanceId"`
	Generation        int               `json:"generation"`
	ProtocolVersion   int               `json:"protocolVersion"`
	BuildID           string            `json:"buildId"`
	Mode              string            `json:"lifecycleMode"`
	Phase             string            `json:"phase"`
	StartedAt         time.Time         `json:"startedAt"`
	Clients           []ClientLeaseView `json:"clients"`
	OtherClientCount  int               `json:"otherClientCount"`
	Workload          WorkloadSnapshot  `json:"workload"`
	IdleDeadline      *time.Time        `json:"idleDeadline,omitempty"`
	Restart           *RestartInfo      `json:"restart,omitempty"`
	UpdatePending     bool              `json:"updatePending"`
	InventoryRevision int64             `json:"inventoryRevision"`
	ServerNow         time.Time         `json:"serverNow"`
}

// IsDraining reports whether the runner rejects new leases/work.
func (s LifecycleSnapshot) IsDraining() bool {
	return s.Phase == "draining_restart" || s.Phase == "draining_shutdown" || s.Phase == "stopped"
}

// RegisterClientRequest is POST /system/clients/register (SD-28 §6.1).
type RegisterClientRequest struct {
	Kind             string `json:"kind"`
	ClientInstanceID string `json:"clientInstanceId"`
	PID              int    `json:"pid"`
	Label            string `json:"label"`
	ProtocolVersion  int    `json:"protocolVersion"`
	BuildID          string `json:"buildId"`
	ProjectPath      string `json:"projectPath"`
}

// RegisterClientResponse carries the raw leaseToken — returned exactly once.
type RegisterClientResponse struct {
	LeaseID             string            `json:"leaseId"`
	LeaseToken          string            `json:"leaseToken"`
	RunnerInstanceID    string            `json:"runnerInstanceId"`
	Generation          int               `json:"generation"`
	HeartbeatIntervalMs int64             `json:"heartbeatIntervalMs"`
	TTLMs               int64             `json:"ttlMs"`
	Snapshot            LifecycleSnapshot `json:"snapshot"`
}

// LeaseMutation is the heartbeat/release body (token + generation fence).
type LeaseMutation struct {
	LeaseToken       string `json:"leaseToken"`
	RunnerInstanceID string `json:"runnerInstanceId"`
	Generation       int    `json:"generation"`
}

// SystemActionRequest is POST /system/shutdown | /system/restart.
type SystemActionRequest struct {
	RequesterLeaseID   string `json:"requesterLeaseId,omitempty"`
	LeaseToken         string `json:"leaseToken,omitempty"`
	ExpectedInstanceID string `json:"expectedInstanceId,omitempty"`
	Reason             string `json:"reason,omitempty"`
	Confirm            bool   `json:"confirm,omitempty"`
	ConfirmToken       string `json:"confirmToken,omitempty"`
	Force              bool   `json:"force,omitempty"`
}

// SystemActionResponse is the 202-accepted body.
type SystemActionResponse struct {
	Status    string            `json:"status"`
	RestartID string            `json:"restartId,omitempty"`
	Snapshot  LifecycleSnapshot `json:"snapshot"`
}

// LifecycleError is the typed error surface for lifecycle calls (SD-28 §6.1).
// When Code is lifecycle_confirmation_required, ConfirmToken + Snapshot carry
// the D-4 revalidation material.
type LifecycleError struct {
	Status       int
	Code         string
	Message      string
	ConfirmToken string
	ExpiresAt    *time.Time
	Snapshot     *LifecycleSnapshot
}

func (e *LifecycleError) Error() string { return e.Code + ": " + e.Message }

// GetLifecycleSnapshot fetches GET /system/lifecycle (Task-417 T-1).
func (c *Client) GetLifecycleSnapshot(ctx context.Context) (LifecycleSnapshot, error) {
	var snap LifecycleSnapshot
	err := c.getJSON(ctx, "/system/lifecycle", &snap)
	return snap, err
}

// RegisterLifecycleClient calls POST /system/clients/register.
func (c *Client) RegisterLifecycleClient(ctx context.Context, in RegisterClientRequest) (RegisterClientResponse, error) {
	var out RegisterClientResponse
	err := c.postJSON(ctx, "/system/clients/register", in, &out)
	return out, err
}

// HeartbeatLifecycleClient refreshes a lease:
// POST /system/clients/{leaseId}/heartbeat.
func (c *Client) HeartbeatLifecycleClient(ctx context.Context, leaseID string, m LeaseMutation) (LifecycleSnapshot, error) {
	var snap LifecycleSnapshot
	err := c.postJSON(ctx, "/system/clients/"+leaseID+"/heartbeat", m, &snap)
	return snap, err
}

// ReleaseLifecycleClient drops a lease: POST /system/clients/{leaseId}/release.
// lease_unknown on an already-gone lease is treated as success — release is
// idempotent by contract.
func (c *Client) ReleaseLifecycleClient(ctx context.Context, leaseID string, m LeaseMutation) (LifecycleSnapshot, error) {
	var snap LifecycleSnapshot
	err := c.postJSON(ctx, "/system/clients/"+leaseID+"/release", m, &snap)
	var ae *APIError
	if errors.As(err, &ae) && ae.Code == "lease_unknown" {
		return snap, nil
	}
	return snap, err
}

// RequestRunnerShutdown requests POST /system/shutdown (two-phase).
func (c *Client) RequestRunnerShutdown(ctx context.Context, in SystemActionRequest) (SystemActionResponse, error) {
	return c.systemAction(ctx, "/system/shutdown", in)
}

// RequestRunnerRestart requests POST /system/restart (two-phase).
func (c *Client) RequestRunnerRestart(ctx context.Context, in SystemActionRequest) (SystemActionResponse, error) {
	return c.systemAction(ctx, "/system/restart", in)
}

func (c *Client) systemAction(ctx context.Context, path string, in SystemActionRequest) (SystemActionResponse, error) {
	var out SystemActionResponse
	body, err := json.Marshal(in)
	if err != nil {
		return out, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+path,
		bytes.NewReader(body))
	if err != nil {
		return out, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	// CA-913: mark TUI-originated system actions for requester attribution.
	req.Header.Set("X-Client", "tui")
	resp, err := c.http.Do(req)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return out, err
	}
	if resp.StatusCode == http.StatusConflict {
		return out, parseLifecycleConflict(raw)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return out, parseLifecycleError(resp.StatusCode, raw)
	}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out)
	}
	return out, nil
}

// parseLifecycleConflict decodes the 409 lifecycle_confirmation_required body
// into a LifecycleError carrying the confirm token + inventory snapshot.
func parseLifecycleConflict(body []byte) error {
	var env struct {
		Error struct {
			Code         string     `json:"code"`
			Message      string     `json:"message"`
			ConfirmToken string     `json:"confirmToken"`
			ExpiresAt    *time.Time `json:"expiresAt"`
		} `json:"error"`
		Snapshot *LifecycleSnapshot `json:"snapshot"`
	}
	if err := json.Unmarshal(body, &env); err != nil || env.Error.Code == "" {
		return &LifecycleError{Status: http.StatusConflict, Code: "conflict", Message: string(body)}
	}
	return &LifecycleError{
		Status:       http.StatusConflict,
		Code:         env.Error.Code,
		Message:      env.Error.Message,
		ConfirmToken: env.Error.ConfirmToken,
		ExpiresAt:    env.Error.ExpiresAt,
		Snapshot:     env.Snapshot,
	}
}

// parseLifecycleError decodes the {error:{code,message}} envelope into a
// LifecycleError so callers can branch on typed codes.
func parseLifecycleError(status int, body []byte) error {
	var env struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &env); err != nil || env.Error.Code == "" {
		return &LifecycleError{Status: status, Code: fmt.Sprintf("http_%d", status), Message: string(body)}
	}
	return &LifecycleError{Status: status, Code: env.Error.Code, Message: env.Error.Message}
}
