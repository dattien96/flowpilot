package runner

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// fakeMCPDriverAdapter is a test double for MCPDriverAdapter.
type fakeMCPDriverAdapter struct {
	content string
	err     error
	delay   time.Duration
}

func (f *fakeMCPDriverAdapter) Fetch(ctx context.Context, driverRef string) (string, error) {
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	if f.err != nil {
		return "", f.err
	}
	return f.content, nil
}

// TestMcpDriverSourceProducesBoundedSectionWithSourceRef verifies the source
// returns a bounded section carrying a SourceRef derived from the driver ref
// (CP-44 DOD-5, Task-195 T-1/T-4).
func TestMcpDriverSourceProducesBoundedSectionWithSourceRef(t *testing.T) {
	src := &mcpDriverSource{priority: 6, adapter: &fakeMCPDriverAdapter{content: "driver content here"}}
	section, err := src.Fetch(context.Background(), FlowContextHints{MCPDriverRef: "driver-123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if section.Body != "driver content here" {
		t.Errorf("Body = %q, want %q", section.Body, "driver content here")
	}
	if section.SourceRef != "mcp:driver-123" {
		t.Errorf("SourceRef = %q, want %q", section.SourceRef, "mcp:driver-123")
	}
	if section.SourceType != "mcp.driver" {
		t.Errorf("SourceType = %q, want mcp.driver", section.SourceType)
	}
}

// TestMcpDriverSourceBoundsLargeContent verifies content beyond the cap is
// truncated (bounded, per CP-41 T-3).
func TestMcpDriverSourceBoundsLargeContent(t *testing.T) {
	big := strings.Repeat("x", mcpDriverContentCap+500)
	src := &mcpDriverSource{priority: 6, adapter: &fakeMCPDriverAdapter{content: big}}
	section, err := src.Fetch(context.Background(), FlowContextHints{MCPDriverRef: "driver-123"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(section.Body) != mcpDriverContentCap {
		t.Errorf("Body length = %d, want %d (capped)", len(section.Body), mcpDriverContentCap)
	}
}

// TestMcpDriverSourceNoDriverRefIsEmptyNotError verifies an unconfigured
// driver ref degrades to an empty, error-free section (a run with no MCP
// driver configured must not warn or fail).
func TestMcpDriverSourceNoDriverRefIsEmptyNotError(t *testing.T) {
	src := &mcpDriverSource{priority: 6, adapter: &fakeMCPDriverAdapter{content: "should not be used"}}
	section, err := src.Fetch(context.Background(), FlowContextHints{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if section.Body != "" || section.SourceRef != "" {
		t.Errorf("expected empty section for unconfigured driver ref, got %#v", section)
	}
}

// TestMcpDriverSourceDegradesOnMcpTimeout verifies a slow adapter call is
// bounded by the hard timeout and surfaces as an error (which Collect turns
// into a degrade-warning, not a failed Plan step — CP-44 D-5).
func TestMcpDriverSourceDegradesOnMcpTimeout(t *testing.T) {
	src := &mcpDriverSource{
		priority: 6,
		adapter:  &fakeMCPDriverAdapter{content: "too slow", delay: 50 * time.Millisecond},
		timeout:  5 * time.Millisecond,
	}
	_, err := src.Fetch(context.Background(), FlowContextHints{MCPDriverRef: "driver-123"})
	if err == nil {
		t.Fatal("expected timeout error")
	}

	// End-to-end through Collect: the timeout must degrade to a warning, not
	// propagate as a Collect-level failure.
	r := NewContextSourceRegistry()
	if err := r.Register(src); err != nil {
		t.Fatalf("register: %v", err)
	}
	sections, warnings := r.Collect(context.Background(), []string{"mcp.driver"}, FlowContextHints{MCPDriverRef: "driver-123"})
	if len(sections) != 0 {
		t.Errorf("expected no sections from a timed-out source, got %#v", sections)
	}
	if len(warnings) != 1 {
		t.Fatalf("expected 1 degrade warning, got %v", warnings)
	}
}

// TestMcpDriverSourceAdapterErrorDegrades verifies an adapter-returned error
// (e.g. MCP down) degrades the same way as a timeout.
func TestMcpDriverSourceAdapterErrorDegrades(t *testing.T) {
	src := &mcpDriverSource{priority: 6, adapter: &fakeMCPDriverAdapter{err: errors.New("mcp unreachable")}}
	r := NewContextSourceRegistry()
	if err := r.Register(src); err != nil {
		t.Fatalf("register: %v", err)
	}
	sections, warnings := r.Collect(context.Background(), []string{"mcp.driver"}, FlowContextHints{MCPDriverRef: "driver-123"})
	if len(sections) != 0 {
		t.Errorf("expected no sections, got %#v", sections)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "mcp unreachable") {
		t.Errorf("expected warning mentioning the adapter error, got %v", warnings)
	}
}

// TestMcpDriverSourceNotInDefaultSet verifies mcp.driver is registered but
// not part of the default enabled set (CP-44 P-4/Task-195: opt-in only).
func TestMcpDriverSourceNotInDefaultSet(t *testing.T) {
	for _, id := range defaultContextSourceIDs {
		if id == string(ContextSourceMCPDriver) {
			t.Fatalf("mcp.driver must not be in defaultContextSourceIDs, got %v", defaultContextSourceIDs)
		}
	}
	r := NewDefaultContextSourceRegistry()
	if _, err := r.Resolve(string(ContextSourceMCPDriver)); err != nil {
		t.Errorf("expected mcp.driver to be registered on the default registry: %v", err)
	}
}

// fakeSecondMCPSource is a minimal second MCP-backed ContextSource used only
// to prove the registry supports N independently-adapted MCP sources at once
// (Task-226): each MCP-backed source owns its own struct + adapter, mirroring
// mcpDriverSource, rather than sharing one adapter slot — so registering
// mcp.driver and a second MCP source (standing in for jira.issue /
// firebase.crashlytics, landed in later tasks) side by side must not collide.
type fakeSecondMCPSource struct {
	priority int
	adapter  MCPDriverAdapter
	ref      string
}

func (s *fakeSecondMCPSource) ID() string          { return "fake.second-mcp" }
func (s *fakeSecondMCPSource) Priority() int       { return s.priority }
func (s *fakeSecondMCPSource) Deterministic() bool { return true }

func (s *fakeSecondMCPSource) Fetch(ctx context.Context, _ FlowContextHints) (FlowContextSection, error) {
	section := FlowContextSection{SourceType: s.ID(), Priority: s.priority}
	if s.ref == "" {
		return section, nil
	}
	section.SourceRef = "fake:" + s.ref
	content, err := mcpBoundedFetch(ctx, 0, mcpDriverTimeout, mcpDriverContentCap, s.ID(), s.ref, s.adapter.Fetch)
	if err != nil {
		return section, err
	}
	section.Body = content
	return section, nil
}

// TestTwoMCPBackedSourcesCoexistIndependently verifies registering mcp.driver
// alongside a second, independently-adapted MCP source does not collide —
// each keeps its own adapter and its own fetch result (Task-226 DOD: adapter
// dispatch generalizes to N sources without a shared-adapter refactor).
func TestTwoMCPBackedSourcesCoexistIndependently(t *testing.T) {
	r := NewContextSourceRegistry()
	drive := &mcpDriverSource{priority: 6, adapter: &fakeMCPDriverAdapter{content: "drive content"}}
	second := &fakeSecondMCPSource{priority: 7, adapter: &fakeMCPDriverAdapter{content: "second content"}, ref: "second-ref"}
	if err := r.Register(drive); err != nil {
		t.Fatalf("register mcp.driver: %v", err)
	}
	if err := r.Register(second); err != nil {
		t.Fatalf("register fake.second-mcp: %v", err)
	}

	sections, warnings := r.Collect(context.Background(), []string{"mcp.driver", "fake.second-mcp"}, FlowContextHints{MCPDriverRef: "driver-ref"})
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
	if len(sections) != 2 {
		t.Fatalf("expected 2 sections, got %d: %#v", len(sections), sections)
	}
	byType := map[string]FlowContextSection{}
	for _, s := range sections {
		byType[s.SourceType] = s
	}
	if byType["mcp.driver"].Body != "drive content" {
		t.Errorf("mcp.driver Body = %q, want %q", byType["mcp.driver"].Body, "drive content")
	}
	if byType["fake.second-mcp"].Body != "second content" {
		t.Errorf("fake.second-mcp Body = %q, want %q", byType["fake.second-mcp"].Body, "second content")
	}
}

// TestFlowContextPackageStillHasNoVectorDependencyWithMcpSource verifies the
// no-vector guard holds even when a flow explicitly enables mcp.driver
// (CP-44 DOD-6).
func TestFlowContextPackageStillHasNoVectorDependencyWithMcpSource(t *testing.T) {
	workspace, _ := fcpFixture(t)
	pkg, err := BuildFlowContextPackageWithSources(context.Background(), workspace, FlowContextHints{
		WorkflowRunID: "run-195",
		PlanStepRunID: "plan-195",
		UserPrompt:    "agent-flow-engine",
		MCPDriverRef:  "", // no adapter wired on the process-wide default registry; unconfigured ref keeps this warning-free
	}, []string{"feature.history", "mcp.driver"})
	if err != nil {
		t.Fatalf("BuildFlowContextPackageWithSources: %v", err)
	}
	rendered := RenderFlowContextPackage(pkg)
	if !strings.Contains(rendered, "## Context") {
		t.Error("rendered package must contain context header with mcp.driver enabled")
	}
}
