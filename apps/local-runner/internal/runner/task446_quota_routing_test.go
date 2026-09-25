package runner

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

// Task-446 (CP-87 P-2/P-5): machine-global quota routing settings, normalized
// account headroom, and per-run settings snapshot.

func TestTask446_HeadroomNormalization(t *testing.T) {
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	settings := QuotaRoutingSettings{
		Mode:                        QuotaRotationManual,
		HeadroomLowPercent:          20,
		TelemetryTTLSeconds:         120,
		SameProviderCooldownSeconds: 20,
	}
	fresh := now.Add(-10 * time.Second).UTC().Format(time.RFC3339)
	old := now.Add(-10 * time.Minute).UTC().Format(time.RFC3339)
	pct := func(v int) *int { return &v }

	cases := []struct {
		name           string
		summary        ProviderAccountSummary
		wantState      string
		wantPercent    *int
		wantConfidence string
	}{
		{
			name: "healthy",
			summary: ProviderAccountSummary{
				Remaining5hPercent: pct(80), UsageSource: "provider_api", ObservedAt: fresh,
			},
			wantState: "healthy", wantPercent: pct(80), wantConfidence: "exact",
		},
		{
			name: "most constrained window wins",
			summary: ProviderAccountSummary{
				Remaining5hPercent: pct(80), Remaining7dPercent: pct(5), UsageSource: "provider_api", ObservedAt: fresh,
			},
			wantState: "low", wantPercent: pct(5), wantConfidence: "exact",
		},
		{
			name: "exhausted",
			summary: ProviderAccountSummary{
				Remaining5hPercent: pct(0), UsageSource: "provider_api", ObservedAt: fresh,
			},
			wantState: "exhausted", wantPercent: pct(0), wantConfidence: "exact",
		},
		{
			name: "low boundary",
			summary: ProviderAccountSummary{
				Remaining5hPercent: pct(20), UsageSource: "provider_api", ObservedAt: fresh,
			},
			wantState: "low", wantPercent: pct(20), wantConfidence: "exact",
		},
		{
			name: "unknown without telemetry",
			summary: ProviderAccountSummary{
				UsageSource: "unavailable", ObservedAt: fresh,
			},
			wantState: "unknown", wantConfidence: "none",
		},
		{
			name: "stale past ttl",
			summary: ProviderAccountSummary{
				Remaining5hPercent: pct(80), UsageSource: "provider_api", ObservedAt: old,
			},
			wantState: "stale", wantPercent: pct(80), wantConfidence: "exact",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := NormalizeAccountHeadroom(tc.summary, now, settings)
			if got.State != tc.wantState {
				t.Fatalf("state = %q, want %q (%+v)", got.State, tc.wantState, got)
			}
			if (got.RemainingPercent == nil) != (tc.wantPercent == nil) {
				t.Fatalf("remainingPercent = %v, want %v", got.RemainingPercent, tc.wantPercent)
			}
			if got.RemainingPercent != nil && *got.RemainingPercent != *tc.wantPercent {
				t.Fatalf("remainingPercent = %d, want %d", *got.RemainingPercent, *tc.wantPercent)
			}
			if got.Confidence != tc.wantConfidence {
				t.Fatalf("confidence = %q, want %q", got.Confidence, tc.wantConfidence)
			}
			if got.Source != tc.summary.UsageSource {
				t.Fatalf("source = %q, want %q", got.Source, tc.summary.UsageSource)
			}
		})
	}
}

func TestTask446_RotationModeDefaultManual(t *testing.T) {
	path := filepath.Join(t.TempDir(), "quota-routing.json")
	t.Setenv("FLOWPILOT_QUOTA_ROUTING_FILE", path)
	settings, err := loadQuotaRoutingSettings()
	if err != nil {
		t.Fatalf("loadQuotaRoutingSettings: %v", err)
	}
	if settings.Mode != QuotaRotationManual {
		t.Fatalf("default mode = %q, want manual", settings.Mode)
	}
	if settings.SameProviderCooldownSeconds != 20 {
		t.Fatalf("default cooldown = %d, want 20", settings.SameProviderCooldownSeconds)
	}
	if settings.TelemetryTTLSeconds != 120 {
		t.Fatalf("default telemetry TTL = %d, want 120", settings.TelemetryTTLSeconds)
	}
	if settings.HeadroomLowPercent <= 0 || settings.HeadroomLowPercent >= 100 {
		t.Fatalf("default low-headroom threshold = %d, want 1..99", settings.HeadroomLowPercent)
	}
}

func TestTask446_SettingsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "quota-routing.json")
	t.Setenv("FLOWPILOT_QUOTA_ROUTING_FILE", path)
	want := QuotaRoutingSettings{
		Mode:             QuotaRotationAuto,
		ProviderPriority: []ProviderKey{ProviderKeyDevin, ProviderKeyGrok},
		ModelBindings: []ModelClassBinding{
			{ProviderKey: ProviderKeyDevin, WorkloadClass: "coding", Model: "devin/swe-2-high"},
			{ProviderKey: ProviderKeyGrok, WorkloadClass: "high_reasoning", Model: "grok-4.5"},
		},
		HeadroomLowPercent:          15,
		TelemetryTTLSeconds:         90,
		SameProviderCooldownSeconds: 30,
	}
	if err := saveQuotaRoutingSettings(want); err != nil {
		t.Fatalf("saveQuotaRoutingSettings: %v", err)
	}
	got, err := loadQuotaRoutingSettings()
	if err != nil {
		t.Fatalf("loadQuotaRoutingSettings: %v", err)
	}
	if got.Mode != want.Mode || got.HeadroomLowPercent != want.HeadroomLowPercent ||
		got.TelemetryTTLSeconds != want.TelemetryTTLSeconds || got.SameProviderCooldownSeconds != want.SameProviderCooldownSeconds {
		t.Fatalf("round-trip settings = %+v, want %+v", got, want)
	}
	if len(got.ProviderPriority) != 2 || got.ProviderPriority[0] != ProviderKeyDevin {
		t.Fatalf("provider priority = %v", got.ProviderPriority)
	}
	if len(got.ModelBindings) != 2 || got.ModelBindings[0].Model != "devin/swe-2-high" {
		t.Fatalf("model bindings = %+v", got.ModelBindings)
	}
	// Invalid mode in a persisted file fails closed back to manual.
	if err := saveQuotaRoutingSettings(QuotaRoutingSettings{Mode: "yolo"}); err != nil {
		t.Fatalf("saveQuotaRoutingSettings: %v", err)
	}
	got, err = loadQuotaRoutingSettings()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.Mode != QuotaRotationManual {
		t.Fatalf("invalid mode did not normalize to manual: %q", got.Mode)
	}
}

func TestTask446_RunSnapshotsSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "quota-routing.json")
	t.Setenv("FLOWPILOT_QUOTA_ROUTING_FILE", path)
	if err := saveQuotaRoutingSettings(QuotaRoutingSettings{Mode: QuotaRotationAuto}); err != nil {
		t.Fatalf("save settings: %v", err)
	}

	adapter := &flakyAdapter{}
	store := newFakeWorkflowStore()
	svc := NewInteractiveServiceWithStore(registryWithAdapter(adapter), nil, store)
	handle, apiErr := svc.createRun(StartRunInput{
		ProjectID:   "p1",
		ChatMode:    "normal_chat",
		ProviderKey: ProviderKeyCodex,
		Model:       "gpt-5.4-mini",
		Cwd:         t.TempDir(),
	})
	if apiErr != nil {
		t.Fatalf("createRun: %v", apiErr)
	}
	st, ok, err := store.GetProviderSession(context.Background(), handle.RunID)
	if err != nil || !ok {
		t.Fatalf("GetProviderSession ok=%v err=%v", ok, err)
	}
	if st.QuotaRouting == nil {
		t.Fatal("run session has no quota-routing snapshot")
	}
	if st.QuotaRouting.Mode != QuotaRotationAuto {
		t.Fatalf("snapshot mode = %q, want auto", st.QuotaRouting.Mode)
	}
	if st.QuotaRouting.PolicyVersion != QuotaRoutingPolicyVersion {
		t.Fatalf("snapshot policyVersion = %d, want %d", st.QuotaRouting.PolicyVersion, QuotaRoutingPolicyVersion)
	}

	// Settings edits after run creation must not rewrite the snapshot.
	if err := saveQuotaRoutingSettings(QuotaRoutingSettings{Mode: QuotaRotationManual}); err != nil {
		t.Fatalf("resave settings: %v", err)
	}
	if st.QuotaRouting.Mode != QuotaRotationAuto {
		t.Fatalf("snapshot mutated after settings edit: %q", st.QuotaRouting.Mode)
	}
}
