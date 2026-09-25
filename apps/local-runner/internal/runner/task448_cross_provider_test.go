package runner

import (
	"context"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// Task-448 (CP-87 P-4): cross-provider candidate enumeration — pure resolver,
// provider-neutral (registry + settings bindings only), every rejection
// explainable. No execution side effects.

// task448Service builds a service whose registry carries the CURRENT provider
// plus the given extra providers, all marked available.
func task448Service(t *testing.T, current ProviderKey, extra ...ProviderRegistration) *InteractiveService {
	t.Helper()
	// Account sync scans $HOME for provider credential dirs — isolate it so a
	// developer machine's real accounts can't leak into candidate buckets.
	t.Setenv("HOME", t.TempDir())
	s := task447Service(t)
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: current, DisplayName: string(current), Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true, Resume: true, ApprovalEvents: true, FileEvents: true, Mcp: true, Interrupt: true},
		newAdapter:   func() ProviderRuntimeAdapter { return newFakeProviderAdapter(current) },
	})
	for _, e := range extra {
		reg.register(e)
	}
	s.registry = reg
	return s
}

func task448Provider(key ProviderKey) ProviderRegistration {
	return ProviderRegistration{
		Key: key, DisplayName: string(key), Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true, Resume: true, ApprovalEvents: true, FileEvents: true, Mcp: true, Interrupt: true},
		newAdapter:   func() ProviderRuntimeAdapter { return newFakeProviderAdapter(key) },
	}
}

func task448Settings(classes map[string]string) QuotaRoutingSettings {
	bindings := []ModelClassBinding{}
	for _, kv := range [][3]string{
		{"claude", "coding", "claude-sonnet"},
		{"grok", "coding", "grok-4.5"},
		{"gemini", "coding", "g-1"},
	} {
		if _, wanted := classes[kv[0]]; wanted {
			bindings = append(bindings, ModelClassBinding{
				ProviderKey:   ProviderKey(kv[0]),
				WorkloadClass: agentpack.WorkloadClass(kv[1]),
				Model:         kv[2],
			})
		}
	}
	return QuotaRoutingSettings{
		Mode:                        QuotaRotationManual,
		ModelBindings:               bindings,
		HeadroomLowPercent:          defaultHeadroomLowPercent,
		TelemetryTTLSeconds:         defaultQuotaTelemetryTTLSeconds,
		SameProviderCooldownSeconds: defaultSameProviderCooldownSeconds,
	}
}

func task448Demand(provider ProviderKey, class agentpack.WorkloadClass) ExecutionDemand {
	return ExecutionDemand{
		RunID: "run-1", RequestedProvider: provider,
		RequestedModel: "gpt-5.4", RequestedAccountID: "cx-0",
		WorkloadClass: class,
		RequiredCaps:  ProviderCapabilities{Streaming: true, ApprovalEvents: true, Mcp: true},
	}
}

func candByProvider(list []RouteCandidate, key ProviderKey) *RouteCandidate {
	for i := range list {
		if list[i].ProviderKey == key {
			return &list[i]
		}
	}
	return nil
}

func hasReason(c RouteCandidate, reason string) bool {
	for _, r := range c.RejectionReasons {
		if r == reason {
			return true
		}
	}
	return false
}

func TestTask448_EnumeratesRegistryExcludingCurrent(t *testing.T) {
	now := time.Now().UTC()
	writeTask447Accounts(t, []ProviderAccount{
		task447Account("cx-0", "codex", 0, true),
		task447Account("cl-0", "claude", 0, true),
		task447Account("gk-0", "grok", 0, true),
	})
	s := task448Service(t, ProviderKeyCodex, task448Provider(ProviderKeyClaude), task448Provider(ProviderKeyGrok))
	s.quotaTelemetryFn = task447Telemetry(map[string]int{"cl-0": 80, "gk-0": 70}, now.Format(time.RFC3339))

	set, err := s.CrossProviderCandidates(context.Background(), task448Demand(ProviderKeyCodex, agentpack.WorkloadCoding),
		task448Settings(map[string]string{"claude": "x", "grok": "x"}))
	if err != nil {
		t.Fatalf("CrossProviderCandidates: %v", err)
	}
	if candByProvider(set.Eligible, ProviderKeyCodex) != nil ||
		candByProvider(set.ManualOnly, ProviderKeyCodex) != nil ||
		candByProvider(set.Rejected, ProviderKeyCodex) != nil {
		t.Fatal("current provider must never appear in the cross-provider set")
	}
	if candByProvider(set.Eligible, ProviderKeyClaude) == nil || candByProvider(set.Eligible, ProviderKeyGrok) == nil {
		t.Fatalf("both registered providers expected eligible, got %+v", set)
	}
}

func TestTask448_WorkloadBindingSelectsModel(t *testing.T) {
	now := time.Now().UTC()
	writeTask447Accounts(t, []ProviderAccount{
		task447Account("cx-0", "codex", 0, true),
		task447Account("cl-0", "claude", 0, true),
	})
	s := task448Service(t, ProviderKeyCodex, task448Provider(ProviderKeyClaude))
	s.quotaTelemetryFn = task447Telemetry(map[string]int{"cl-0": 90}, now.Format(time.RFC3339))

	set, err := s.CrossProviderCandidates(context.Background(), task448Demand(ProviderKeyCodex, agentpack.WorkloadCoding),
		task448Settings(map[string]string{"claude": "x"}))
	if err != nil {
		t.Fatalf("CrossProviderCandidates: %v", err)
	}
	c := candByProvider(set.Eligible, ProviderKeyClaude)
	if c == nil {
		t.Fatalf("claude candidate missing, got %+v", set)
	}
	if c.Model != "claude-sonnet" {
		t.Fatalf("explicit (provider,class) binding selects the model, got %q", c.Model)
	}
}

func TestTask448_MissingBindingGates(t *testing.T) {
	now := time.Now().UTC()
	writeTask447Accounts(t, []ProviderAccount{
		task447Account("cx-0", "codex", 0, true),
		task447Account("gk-0", "grok", 0, true),
	})
	s := task448Service(t, ProviderKeyCodex, task448Provider(ProviderKeyGrok))
	s.quotaTelemetryFn = task447Telemetry(map[string]int{"gk-0": 90}, now.Format(time.RFC3339))

	// No (grok,coding) binding — healthy account but no quality mapping.
	set, err := s.CrossProviderCandidates(context.Background(), task448Demand(ProviderKeyCodex, agentpack.WorkloadCoding),
		task448Settings(map[string]string{}))
	if err != nil {
		t.Fatalf("CrossProviderCandidates: %v", err)
	}
	if candByProvider(set.Eligible, ProviderKeyGrok) != nil {
		t.Fatal("missing class binding must never be auto-eligible")
	}
	c := candByProvider(set.ManualOnly, ProviderKeyGrok)
	if c == nil {
		t.Fatalf("missing binding stays a manual candidate, got %+v", set)
	}
	if !hasReason(*c, QuotaRejectMissingModelBind) {
		t.Fatalf("missing binding reason expected, got %v", c.RejectionReasons)
	}
}

func TestTask448_CapabilityMismatchRejected(t *testing.T) {
	now := time.Now().UTC()
	writeTask447Accounts(t, []ProviderAccount{
		task447Account("cx-0", "codex", 0, true),
		task447Account("gk-0", "grok", 0, true),
	})
	noTool := task448Provider(ProviderKeyGrok)
	noTool.Capabilities = ProviderCapabilities{Streaming: true} // no approval/mcp bridge
	s := task448Service(t, ProviderKeyCodex, noTool)
	s.quotaTelemetryFn = task447Telemetry(map[string]int{"gk-0": 90}, now.Format(time.RFC3339))

	set, err := s.CrossProviderCandidates(context.Background(), task448Demand(ProviderKeyCodex, agentpack.WorkloadCoding),
		task448Settings(map[string]string{"grok": "x"}))
	if err != nil {
		t.Fatalf("CrossProviderCandidates: %v", err)
	}
	c := candByProvider(set.Rejected, ProviderKeyGrok)
	if c == nil {
		t.Fatalf("capability-mismatch provider must be rejected, got %+v", set)
	}
	if !hasReason(*c, "missing_capability:mcp") && !hasReason(*c, "missing_capability:approvalEvents") {
		t.Fatalf("expected missing_capability reasons, got %v", c.RejectionReasons)
	}
}

func TestTask448_ContextWindowTooSmallRejected(t *testing.T) {
	now := time.Now().UTC()
	writeTask447Accounts(t, []ProviderAccount{
		task447Account("cx-0", "codex", 0, true),
		task447Account("cl-0", "claude", 0, true),
	})
	s := task448Service(t, ProviderKeyCodex, task448Provider(ProviderKeyClaude))
	s.quotaTelemetryFn = task447Telemetry(map[string]int{"cl-0": 90}, now.Format(time.RFC3339))

	demand := task448Demand(ProviderKeyCodex, agentpack.WorkloadCoding)
	demand.MinContextTokens = 9_000_000 // bigger than any catalog window
	set, err := s.CrossProviderCandidates(context.Background(), demand,
		task448Settings(map[string]string{"claude": "x"}))
	if err != nil {
		t.Fatalf("CrossProviderCandidates: %v", err)
	}
	c := candByProvider(set.Rejected, ProviderKeyClaude)
	if c == nil {
		t.Fatalf("too-small window must reject, got %+v", set)
	}
	if !hasReason(*c, "context_window_too_small") {
		t.Fatalf("expected context_window_too_small, got %v", c.RejectionReasons)
	}
}

func TestTask448_UnknownQuotaManualOnly(t *testing.T) {
	writeTask447Accounts(t, []ProviderAccount{
		task447Account("cx-0", "codex", 0, true),
		task447Account("cl-0", "claude", 0, true),
	})
	s := task448Service(t, ProviderKeyCodex, task448Provider(ProviderKeyClaude))
	s.quotaTelemetryFn = task447Telemetry(map[string]int{}, time.Now().UTC().Format(time.RFC3339))

	set, err := s.CrossProviderCandidates(context.Background(), task448Demand(ProviderKeyCodex, agentpack.WorkloadCoding),
		task448Settings(map[string]string{"claude": "x"}))
	if err != nil {
		t.Fatalf("CrossProviderCandidates: %v", err)
	}
	if candByProvider(set.Eligible, ProviderKeyClaude) != nil {
		t.Fatal("unknown quota is never auto-eligible")
	}
	c := candByProvider(set.ManualOnly, ProviderKeyClaude)
	if c == nil || !hasReason(*c, QuotaRejectUnknownQuota) {
		t.Fatalf("expected manual-only unknown_quota candidate, got %+v", set)
	}
}

func TestTask448_DeterministicPriorityTieBreak(t *testing.T) {
	now := time.Now().UTC()
	writeTask447Accounts(t, []ProviderAccount{
		task447Account("cx-0", "codex", 0, true),
		task447Account("cl-0", "claude", 0, true),
		task447Account("gk-0", "grok", 0, true),
	})
	s := task448Service(t, ProviderKeyCodex, task448Provider(ProviderKeyClaude), task448Provider(ProviderKeyGrok))
	// claude has MORE headroom but grok wins on configured priority.
	s.quotaTelemetryFn = task447Telemetry(map[string]int{"cl-0": 90, "gk-0": 70}, now.Format(time.RFC3339))
	settings := task448Settings(map[string]string{"claude": "x", "grok": "x"})
	settings.ProviderPriority = []ProviderKey{ProviderKeyGrok, ProviderKeyClaude}

	set, err := s.CrossProviderCandidates(context.Background(), task448Demand(ProviderKeyCodex, agentpack.WorkloadCoding), settings)
	if err != nil {
		t.Fatalf("CrossProviderCandidates: %v", err)
	}
	if len(set.Eligible) != 2 {
		t.Fatalf("expected 2 eligible, got %+v", set)
	}
	if set.Eligible[0].ProviderKey != ProviderKeyGrok || set.Eligible[1].ProviderKey != ProviderKeyClaude {
		t.Fatalf("provider priority must beat raw headroom, got order %s,%s",
			set.Eligible[0].ProviderKey, set.Eligible[1].ProviderKey)
	}
	if set.Eligible[0].Score <= set.Eligible[1].Score {
		t.Fatal("score must reflect the priority ordering")
	}
}

func TestTask448_NewProviderNeedsNoRouterCodeChange(t *testing.T) {
	now := time.Now().UTC()
	// A provider the router never names: gemini is registered on the service's
	// test registry only — a settings binding + connected account must make it
	// eligible with zero router source changes.
	writeTask447Accounts(t, []ProviderAccount{
		task447Account("cx-0", "codex", 0, true),
		task447Account("gm-0", "gemini", 0, true),
	})
	s := task448Service(t, ProviderKeyCodex, task448Provider("gemini"))
	s.quotaTelemetryFn = task447Telemetry(map[string]int{"gm-0": 88}, now.Format(time.RFC3339))

	set, err := s.CrossProviderCandidates(context.Background(), task448Demand(ProviderKeyCodex, agentpack.WorkloadCoding),
		task448Settings(map[string]string{"gemini": "x"}))
	if err != nil {
		t.Fatalf("CrossProviderCandidates: %v", err)
	}
	c := candByProvider(set.Eligible, "gemini")
	if c == nil || !c.AutoEligible {
		t.Fatalf("new provider must be eligible purely from registry+config, got %+v", set)
	}
	if c.AccountID != "gm-0" || c.Model != "g-1" {
		t.Fatalf("candidate binds the configured model + connected account, got %+v", c)
	}
}
