package runner

// CP-81 Task-415: runner-owned lifecycle HTTP surface. The lifecycle.Manager
// is the single authority for leases/phases; these handlers translate HTTP ↔
// manager calls and hand accepted drains to a process-level DrainFunc owned
// by internal/cli (the only place os.Exit/supervisor/cleanup may live).

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"flowpilot-runner/internal/lifecycle"
)

// ProtocolVersion is the CP-81 lifecycle/build contract version runnerboot
// compares to classify an existing runner (Task-416). Bump on breaking
// lifecycle contract changes.
const ProtocolVersion = 1

// BuildID identifies this build for stale-build replacement. Release builds
// override it at link time via -ldflags
// "-X flowpilot-runner/internal/runner.BuildID=<sha>"; when empty the runner
// falls back to lifecycle.ExecutableBuildID() so `go run` dev binaries are
// distinguishable (Task-416 T-3).
var BuildID = ""

// EffectiveBuildID returns the ldflags override or the executable-derived
// identity. runnerboot.CurrentBuildIdentity() computes the same value for the
// same binary, which is what makes stale-runner detection trustworthy.
func EffectiveBuildID() string {
	if BuildID != "" {
		return BuildID
	}
	return lifecycle.ExecutableBuildID()
}

// LifecycleDrainInput carries the accepted drain decision to the process owner.
type LifecycleDrainInput struct {
	Action    string // "shutdown" | "restart"
	Reason    string
	Requester string // describeRequester output for attribution logs
	RestartID string // restart only — correlates the supervisor handoff
}

// LifecycleRouteOptions wires the lifecycle endpoints. DrainFunc is invoked
// (on its own goroutine) exactly once per accepted drain — it owns durable
// stop-all, supervisor fencing, bounded cleanup, and process exit.
type LifecycleRouteOptions struct {
	Drain func(LifecycleDrainInput)
	// DescribeRequester renders CA-913 attribution (remote/ua/X-Client/pid).
	// Nil falls back to remote addr + user agent.
	DescribeRequester func(*http.Request) string
}

// AttachLifecycle installs the shared lifecycle manager on the runner.
// Called once from `runner serve`; nil managers leave /health legacy-shaped.
func (r *Runner) AttachLifecycle(m *lifecycle.Manager) {
	r.sessionsMu.Lock()
	r.lifecycleMgr = m
	r.sessionsMu.Unlock()
}

// Lifecycle returns the attached manager, or nil in unmanaged contexts
// (tests, `runner health`, direct New() consumers).
func (r *Runner) Lifecycle() *lifecycle.Manager {
	r.sessionsMu.Lock()
	defer r.sessionsMu.Unlock()
	return r.lifecycleMgr
}

// lifecycleSnapshot returns the manager's current snapshot without erroring
// when unmanaged — used by /health's additive fields.
func (r *Runner) lifecycleSnapshot() (lifecycle.LifecycleSnapshot, bool) {
	m := r.Lifecycle()
	if m == nil {
		return lifecycle.LifecycleSnapshot{}, false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	snap, err := m.Snapshot(ctx)
	if err != nil {
		return lifecycle.LifecycleSnapshot{}, false
	}
	return snap, true
}

// RegisterLifecycleRoutes installs the CP-81 §6.1 lease surface: lifecycle
// snapshot plus client register/heartbeat/release. The fenced
// /system/shutdown + /system/restart handlers stay in internal/cli/root.go
// (CA-911/CA-913 pin the requester-attribution line there) and delegate here
// via HandleSystemActionRequest.
func (r *Runner) RegisterLifecycleRoutes(mux *http.ServeMux, opts LifecycleRouteOptions) {
	mux.HandleFunc("GET /system/lifecycle", func(w http.ResponseWriter, req *http.Request) {
		m := r.Lifecycle()
		if m == nil {
			writeLifecycleError(w, http.StatusNotFound, lifecycle.ErrInvalidLifecycleRequest, "lifecycle manager not attached")
			return
		}
		snap, err := m.Snapshot(req.Context())
		if err != nil {
			writeLifecycleErr(w, err)
			return
		}
		writeLifecycleJSON(w, http.StatusOK, snap)
	})

	mux.HandleFunc("POST /system/clients/register", func(w http.ResponseWriter, req *http.Request) {
		m := r.Lifecycle()
		if m == nil {
			writeLifecycleError(w, http.StatusNotFound, lifecycle.ErrInvalidLifecycleRequest, "lifecycle manager not attached")
			return
		}
		var body registerClientBody
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
			writeLifecycleError(w, http.StatusBadRequest, lifecycle.ErrInvalidLifecycleRequest, "invalid request body")
			return
		}
		res, err := m.Register(req.Context(), lifecycle.RegisterInput{
			Kind:             lifecycle.ClientKind(strings.TrimSpace(body.Kind)),
			ClientInstanceID: strings.TrimSpace(body.ClientInstanceID),
			PID:              body.PID,
			Label:            strings.TrimSpace(body.Label),
			ProtocolVersion:  body.ProtocolVersion,
			BuildID:          strings.TrimSpace(body.BuildID),
			ProjectPath:      strings.TrimSpace(body.ProjectPath),
		})
		if err != nil {
			writeLifecycleErr(w, err)
			return
		}
		status := http.StatusCreated
		if res.Idempotent {
			status = http.StatusOK
		}
		writeLifecycleJSON(w, status, registerClientResponse{
			LeaseID:             res.LeaseID,
			LeaseToken:          res.LeaseToken,
			RunnerInstanceID:    res.RunnerInstanceID,
			Generation:          res.Generation,
			HeartbeatIntervalMs: res.HeartbeatIntervalMs,
			TTLMs:               res.TTLMs,
			Snapshot:            res.Snapshot,
		})
	})

	mux.HandleFunc("POST /system/clients/{leaseId}/heartbeat", func(w http.ResponseWriter, req *http.Request) {
		r.handleLeaseMutation(w, req, func(m *lifecycle.Manager, in leaseMutationBody) (lifecycle.LifecycleSnapshot, error) {
			return m.Heartbeat(req.Context(), lifecycle.HeartbeatInput{
				LeaseID:          req.PathValue("leaseId"),
				LeaseToken:       in.LeaseToken,
				RunnerInstanceID: in.RunnerInstanceID,
				Generation:       in.Generation,
			})
		})
	})

	mux.HandleFunc("POST /system/clients/{leaseId}/release", func(w http.ResponseWriter, req *http.Request) {
		r.handleLeaseMutation(w, req, func(m *lifecycle.Manager, in leaseMutationBody) (lifecycle.LifecycleSnapshot, error) {
			return m.Release(req.Context(), lifecycle.ReleaseInput{
				LeaseID:          req.PathValue("leaseId"),
				LeaseToken:       in.LeaseToken,
				RunnerInstanceID: in.RunnerInstanceID,
				Generation:       in.Generation,
			})
		})
	})
}

// ── request bodies ──────────────────────────────────────────────────────────

type registerClientBody struct {
	Kind             string `json:"kind"`
	ClientInstanceID string `json:"clientInstanceId"`
	PID              int    `json:"pid"`
	Label            string `json:"label"`
	ProtocolVersion  int    `json:"protocolVersion"`
	BuildID          string `json:"buildId"`
	ProjectPath      string `json:"projectPath"`
}

type registerClientResponse struct {
	LeaseID             string                      `json:"leaseId"`
	LeaseToken          string                      `json:"leaseToken"`
	RunnerInstanceID    string                      `json:"runnerInstanceId"`
	Generation          int                         `json:"generation"`
	HeartbeatIntervalMs int64                       `json:"heartbeatIntervalMs"`
	TTLMs               int64                       `json:"ttlMs"`
	Snapshot            lifecycle.LifecycleSnapshot `json:"snapshot"`
}

type leaseMutationBody struct {
	LeaseToken       string `json:"leaseToken"`
	RunnerInstanceID string `json:"runnerInstanceId"`
	Generation       int    `json:"generation"`
}

type systemActionBody struct {
	RequesterLeaseID   string `json:"requesterLeaseId"`
	LeaseToken         string `json:"leaseToken"`
	ExpectedInstanceID string `json:"expectedInstanceId"`
	Reason             string `json:"reason"`
	Confirm            bool   `json:"confirm"`
	ConfirmToken       string `json:"confirmToken"`
	// Force is the operator escape hatch (second SIGINT, supervisor fenced
	// commands): skips the two-phase confirmation but never skips the
	// expectedInstanceId fence.
	Force bool `json:"force"`
}

// ── handlers ─────────────────────────────────────────────────────────────────

func (r *Runner) handleLeaseMutation(w http.ResponseWriter, req *http.Request,
	call func(*lifecycle.Manager, leaseMutationBody) (lifecycle.LifecycleSnapshot, error)) {
	m := r.Lifecycle()
	if m == nil {
		writeLifecycleError(w, http.StatusNotFound, lifecycle.ErrInvalidLifecycleRequest, "lifecycle manager not attached")
		return
	}
	var body leaseMutationBody
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		writeLifecycleError(w, http.StatusBadRequest, lifecycle.ErrInvalidLifecycleRequest, "invalid request body")
		return
	}
	snap, err := call(m, body)
	if err != nil {
		writeLifecycleErr(w, err)
		return
	}
	writeLifecycleJSON(w, http.StatusOK, snap)
}

// HandleSystemActionRequest runs the D-4 two-phase destructive command:
// idle/sole requester → 202 + async drain; busy/shared → 409 with a
// single-use confirmToken bound to the current inventory revision. The
// caller supplies the requester identity string (CA-913 describeRequester).
func (r *Runner) HandleSystemActionRequest(w http.ResponseWriter, req *http.Request, opts LifecycleRouteOptions, action, requester string) {
	m := r.Lifecycle()
	if m == nil {
		writeLifecycleError(w, http.StatusNotFound, lifecycle.ErrInvalidLifecycleRequest, "lifecycle manager not attached")
		return
	}
	var body systemActionBody
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		writeLifecycleError(w, http.StatusBadRequest, lifecycle.ErrInvalidLifecycleRequest, "invalid request body")
		return
	}

	in := lifecycle.SystemActionInput{
		RequesterLeaseID:   strings.TrimSpace(body.RequesterLeaseID),
		LeaseToken:         body.LeaseToken,
		ExpectedInstanceID: strings.TrimSpace(body.ExpectedInstanceID),
		Reason:             strings.TrimSpace(body.Reason),
		Requester:          requester,
		Confirm:            body.Confirm,
		ConfirmToken:       body.ConfirmToken,
		Force:              body.Force,
	}
	var res lifecycle.SystemActionResult
	var err error
	if action == "restart" {
		res, err = m.RequestRestart(req.Context(), in)
	} else {
		res, err = m.RequestShutdown(req.Context(), in)
	}
	if err != nil {
		writeLifecycleErr(w, err)
		return
	}
	if res.ConfirmationRequired {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"code":         lifecycle.ErrConfirmationRequired,
				"message":      "Runner is shared or has active work",
				"confirmToken": res.ConfirmationToken,
				"expiresAt":    res.ConfirmationExpiresAt,
			},
			"snapshot": res.Snapshot,
		})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":    "accepted",
		"restartId": res.RestartID,
		"snapshot":  res.Snapshot,
	})
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	if opts.Drain != nil {
		go opts.Drain(LifecycleDrainInput{
			Action:    action,
			Reason:    body.Reason,
			Requester: requester,
			RestartID: res.RestartID,
		})
	}
}

// ── error mapping (SD-28 §6.1) ───────────────────────────────────────────────

func writeLifecycleErr(w http.ResponseWriter, err error) {
	var le *lifecycle.Error
	if errors.As(err, &le) {
		status := lifecycleHTTPStatus(le.Code)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{"code": le.Code, "message": le.Message},
		})
		return
	}
	writeLifecycleError(w, http.StatusInternalServerError, "internal", err.Error())
}

func lifecycleHTTPStatus(code string) int {
	switch code {
	case lifecycle.ErrLeaseUnknown:
		return http.StatusNotFound
	case lifecycle.ErrInvalidLeaseToken:
		return http.StatusForbidden
	case lifecycle.ErrStaleGeneration,
		lifecycle.ErrRunnerDraining,
		lifecycle.ErrStaleLifecycleSnapshot,
		lifecycle.ErrConfirmationRequired,
		lifecycle.ErrLeaseAlreadyReleased:
		return http.StatusConflict
	case lifecycle.ErrInvalidLifecycleRequest:
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}

func writeLifecycleError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{"code": code, "message": msg},
	})
}

func writeLifecycleJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
