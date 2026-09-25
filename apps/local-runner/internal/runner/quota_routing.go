package runner

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// Quota routing settings (Task-446 / CP-87 P-2/P-5): the machine-global,
// user-owned routing policy document. Provider accounts and quota telemetry
// are machine-local, so this document lives beside chat-posture.json — never
// synced, never per-project. It is snapshotted into every new run
// (ProviderSessionState.QuotaRouting) so a mid-run settings edit cannot
// change the policy an active run executes under.

const (
	// QuotaRotationManual is the default and always-safe mode: quota events
	// surface a candidate table and wait for a user decision.
	QuotaRotationManual = "manual"
	// QuotaRotationAuto allows bounded automatic rotation — exact/high-
	// confidence candidates only, same-provider cooldown + max-switches caps
	// enforced by the router (Task-447+).
	QuotaRotationAuto = "auto"

	// QuotaRoutingPolicyVersion stamps each run snapshot; bump when the
	// snapshot's semantics change so a restored run's decisions stay
	// interpretable.
	QuotaRoutingPolicyVersion = 1

	defaultQuotaTelemetryTTLSeconds    = 120 // proposed freshness window (2m)
	defaultSameProviderCooldownSeconds = 20
	defaultHeadroomLowPercent          = 20
)

// ModelClassBinding binds a provider+workload class to the preferred model the
// router may auto-select (Task-446 T-3: auto never infers model quality — a
// missing binding means the user gate).
type ModelClassBinding struct {
	ProviderKey   ProviderKey             `json:"providerKey"`
	WorkloadClass agentpack.WorkloadClass `json:"workloadClass"`
	Model         string                  `json:"model"`
}

// QuotaRoutingSettings is the persisted machine-global routing policy.
type QuotaRoutingSettings struct {
	Mode             string              `json:"mode"` // manual | auto
	ProviderPriority []ProviderKey       `json:"providerPriority,omitempty"`
	ModelBindings    []ModelClassBinding `json:"modelBindings,omitempty"`
	// HeadroomLowPercent is the normalized-headroom "low" threshold; below it
	// an account is not a healthy auto candidate (never compared to tokens —
	// quota percentages are provider-specific, Task-446 constraint).
	HeadroomLowPercent int `json:"headroomLowPercent"`
	// TelemetryTTLSeconds bounds how old quota evidence may be before it is
	// treated as stale (and therefore never auto-routable).
	TelemetryTTLSeconds int `json:"telemetryTtlSeconds"`
	// SameProviderCooldownSeconds is the minimum dwell between automatic
	// same-provider account switches (default 20s).
	SameProviderCooldownSeconds int `json:"sameProviderCooldownSeconds"`
}

// QuotaRoutingSnapshot is the per-run frozen copy of the routing policy plus
// the schema version — persisted on ProviderSessionState so restart/replay
// resolves the same routing decisions the run was created under.
type QuotaRoutingSnapshot struct {
	QuotaRoutingSettings
	PolicyVersion int `json:"policyVersion"`
}

// AccountHeadroom is the normalized quota view the router consumes (Task-446
// T-4). Provider percentages/credits never surface as token balances.
type AccountHeadroom struct {
	State            string `json:"state"` // healthy | low | exhausted | unknown | stale
	RemainingPercent *int   `json:"remainingPercent,omitempty"`
	ResetAt          string `json:"resetAt,omitempty"`
	Source           string `json:"source,omitempty"`
	// FreshnessSeconds is the age of the telemetry at normalize time.
	FreshnessSeconds int64  `json:"freshnessSeconds"`
	Confidence       string `json:"confidence"` // exact | none
}

// ProviderAccountSummary is the runner-side quota-telemetry view of one
// account — the shape the /client/provider-accounts response (and its TUI
// mirror) already carries. The runner owns this copy so the routing engine
// normalizes headroom without importing the cli package.
type ProviderAccountSummary struct {
	ID                 string `json:"id"`
	ProviderKey        string `json:"providerKey"`
	Remaining5hPercent *int   `json:"remaining5hPercent,omitempty"`
	Remaining7dPercent *int   `json:"remaining7dPercent,omitempty"`
	Remaining5hResetAt string `json:"remaining5hResetAt,omitempty"`
	Remaining7dResetAt string `json:"remaining7dResetAt,omitempty"`
	UsageSource        string `json:"usageSource,omitempty"`
	// ObservedAt is the RFC3339 capture time of the telemetry; empty means the
	// reading's age is unknown.
	ObservedAt string `json:"observedAt,omitempty"`
}

// NormalizeAccountHeadroom folds provider-specific quota telemetry into one
// comparable headroom state. Percentages stay percentages — the router's
// demand side (Task-447) never compares them to token counts directly.
//
//   - no percent evidence at all      -> unknown (never auto-routable)
//   - evidence older than the TTL     -> stale   (never auto-routable)
//   - min(short,long) == 0            -> exhausted
//   - min <= HeadroomLowPercent       -> low
//   - otherwise                       -> healthy
//
// Confidence is "exact" when a provider API supplied the number and "none"
// when there is no number — heuristic percentage inference is deliberately
// absent (T-2: evidence quality is an audit field, not a guess).
func NormalizeAccountHeadroom(summary ProviderAccountSummary, now time.Time, settings QuotaRoutingSettings) AccountHeadroom {
	head := AccountHeadroom{
		State:      "unknown",
		Source:     summary.UsageSource,
		Confidence: "none",
	}
	// Effective remaining = the most constrained (minimum) of the declared
	// quota windows — a 5h window at 0% blocks even with a full 7d window.
	var minPct *int
	for _, p := range []*int{summary.Remaining5hPercent, summary.Remaining7dPercent} {
		if p == nil {
			continue
		}
		if minPct == nil || *p < *minPct {
			minPct = p
		}
	}
	head.RemainingPercent = minPct
	head.ResetAt = summary.ResetAt5hOrEarliest()
	if minPct != nil {
		head.Confidence = "exact"
	}
	if observed := strings.TrimSpace(summary.ObservedAt); observed != "" {
		if ts, err := time.Parse(time.RFC3339, observed); err == nil {
			age := now.Sub(ts)
			if age < 0 {
				age = 0
			}
			head.FreshnessSeconds = int64(age.Seconds())
		}
	}
	if minPct == nil {
		return head // unknown
	}
	ttl := settings.TelemetryTTLSeconds
	if ttl <= 0 {
		ttl = defaultQuotaTelemetryTTLSeconds
	}
	if strings.TrimSpace(summary.ObservedAt) == "" || head.FreshnessSeconds > int64(ttl) {
		head.State = "stale"
		return head
	}
	switch {
	case *minPct <= 0:
		head.State = "exhausted"
	case *minPct <= settings.HeadroomLowPercent:
		head.State = "low"
	default:
		head.State = "healthy"
	}
	return head
}

// ResetAt5hOrEarliest returns the reset timestamp of whichever quota window is
// currently the constraining one (or the earliest known reset when no percent
// is declared). Exported for the router's audit fields.
func (s ProviderAccountSummary) ResetAt5hOrEarliest() string {
	// Constraining window's reset wins when both are declared; otherwise the
	// earliest timestamp.
	five, seven := strings.TrimSpace(s.Remaining5hResetAt), strings.TrimSpace(s.Remaining7dResetAt)
	if s.Remaining5hPercent != nil && s.Remaining7dPercent != nil {
		if *s.Remaining7dPercent < *s.Remaining5hPercent {
			return seven
		}
		return five
	}
	if five == "" {
		return seven
	}
	return five
}

// quotaRoutingSettingsPaths returns candidate machine-global file locations,
// mirroring chatPosturePaths. Tests override via FLOWPILOT_QUOTA_ROUTING_FILE.
func quotaRoutingSettingsPaths() []string {
	if envPath := strings.TrimSpace(os.Getenv("FLOWPILOT_QUOTA_ROUTING_FILE")); envPath != "" {
		return []string{envPath}
	}
	var out []string
	switch runtime.GOOS {
	case "windows":
		if appData := strings.TrimSpace(os.Getenv("APPDATA")); appData != "" {
			out = append(out, filepath.Join(appData, "FlowPilot", "quota-routing.json"))
		}
	case "darwin":
		if home, err := os.UserHomeDir(); err == nil {
			out = append(out, filepath.Join(home, "Library", "Application Support", "FlowPilot", "quota-routing.json"))
		}
	default:
		if home, err := os.UserHomeDir(); err == nil {
			out = append(out, filepath.Join(home, ".config", "FlowPilot", "quota-routing.json"))
		}
	}
	return out
}

func defaultQuotaRoutingSettings() QuotaRoutingSettings {
	return QuotaRoutingSettings{
		Mode:                        QuotaRotationManual,
		HeadroomLowPercent:          defaultHeadroomLowPercent,
		TelemetryTTLSeconds:         defaultQuotaTelemetryTTLSeconds,
		SameProviderCooldownSeconds: defaultSameProviderCooldownSeconds,
	}
}

// normalizeQuotaRoutingSettings fails closed to the safe defaults: an unknown
// mode reads back as manual, and non-positive thresholds/TTLs fall to their
// defaults so a corrupted file can never arm auto-rotation or disable
// cooldowns.
func normalizeQuotaRoutingSettings(s *QuotaRoutingSettings) {
	if s.Mode != QuotaRotationAuto && s.Mode != QuotaRotationManual {
		s.Mode = QuotaRotationManual
	}
	if s.HeadroomLowPercent <= 0 || s.HeadroomLowPercent >= 100 {
		s.HeadroomLowPercent = defaultHeadroomLowPercent
	}
	if s.TelemetryTTLSeconds <= 0 {
		s.TelemetryTTLSeconds = defaultQuotaTelemetryTTLSeconds
	}
	if s.SameProviderCooldownSeconds <= 0 {
		s.SameProviderCooldownSeconds = defaultSameProviderCooldownSeconds
	}
	cleaned := s.ModelBindings[:0]
	for _, b := range s.ModelBindings {
		if strings.TrimSpace(string(b.ProviderKey)) == "" || strings.TrimSpace(b.Model) == "" {
			continue
		}
		b.WorkloadClass = agentpack.WorkloadClass(strings.TrimSpace(string(b.WorkloadClass)))
		cleaned = append(cleaned, b)
	}
	s.ModelBindings = cleaned
}

func loadQuotaRoutingSettings() (QuotaRoutingSettings, error) {
	for _, path := range quotaRoutingSettingsPaths() {
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var s QuotaRoutingSettings
		if err := json.Unmarshal(raw, &s); err != nil {
			continue
		}
		normalizeQuotaRoutingSettings(&s)
		return s, nil
	}
	return defaultQuotaRoutingSettings(), nil
}

// mustLoadQuotaRoutingSettings is the run-create path helper: a missing or
// unreadable file yields the safe manual-mode defaults, never an error that
// would block run creation.
func mustLoadQuotaRoutingSettings() QuotaRoutingSettings {
	s, _ := loadQuotaRoutingSettings()
	return s
}

// ---- HTTP handlers (registered in RegisterInteractiveRoutes) ----------------

// handleGetQuotaRoutingSettings handles GET /client/quota-routing-settings.
// Runner-owned SSOT — Desktop/TUI read/write through here, never the file.
func (s *InteractiveService) handleGetQuotaRoutingSettings(w http.ResponseWriter, _ *http.Request) {
	writeInteractiveJSON(w, http.StatusOK, mustLoadQuotaRoutingSettings())
}

// handleSetQuotaRoutingSettings handles PUT /client/quota-routing-settings.
// The body is a full settings document; normalization fails closed to manual
// + defaults for out-of-range fields before persisting.
func (s *InteractiveService) handleSetQuotaRoutingSettings(w http.ResponseWriter, r *http.Request) {
	var req QuotaRoutingSettings
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeInteractiveError(w, newAPIErr(http.StatusBadRequest, "invalid_request", "invalid request body"))
		return
	}
	if err := saveQuotaRoutingSettings(req); err != nil {
		writeInteractiveError(w, newAPIErr(http.StatusInternalServerError, "write_failed", err.Error()))
		return
	}
	writeInteractiveJSON(w, http.StatusOK, req)
}

func saveQuotaRoutingSettings(s QuotaRoutingSettings) error {
	normalizeQuotaRoutingSettings(&s)
	paths := quotaRoutingSettingsPaths()
	if len(paths) == 0 {
		return os.ErrNotExist
	}
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	var lastErr error
	written := false
	for _, dest := range paths {
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			lastErr = err
			continue
		}
		if err := os.WriteFile(dest, raw, 0o600); err != nil {
			lastErr = err
			continue
		}
		written = true
	}
	if !written {
		if lastErr == nil {
			lastErr = os.ErrNotExist
		}
		return lastErr
	}
	return nil
}

// snapshotQuotaRouting freezes the live settings into a run snapshot.
func snapshotQuotaRouting(s QuotaRoutingSettings) *QuotaRoutingSnapshot {
	normalizeQuotaRoutingSettings(&s)
	return &QuotaRoutingSnapshot{
		QuotaRoutingSettings: s,
		PolicyVersion:        QuotaRoutingPolicyVersion,
	}
}
