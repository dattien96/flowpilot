package runner

import (
	"context"
	"errors"
	"testing"
)

// fakeContextSource is a minimal ContextSource test double.
type fakeContextSource struct {
	id            string
	priority      int
	deterministic bool
	section       FlowContextSection
	err           error
}

func (f *fakeContextSource) ID() string           { return f.id }
func (f *fakeContextSource) Priority() int        { return f.priority }
func (f *fakeContextSource) Deterministic() bool  { return f.deterministic }
func (f *fakeContextSource) Fetch(_ context.Context, _ FlowContextHints) (FlowContextSection, error) {
	if f.err != nil {
		return FlowContextSection{}, f.err
	}
	return f.section, nil
}

func TestContextSourceRegistryRegisterRejectsDuplicate(t *testing.T) {
	r := NewContextSourceRegistry()
	src := &fakeContextSource{id: "feature.history", deterministic: true}
	if err := r.Register(src); err != nil {
		t.Fatalf("first register: %v", err)
	}
	if err := r.Register(src); err == nil {
		t.Fatal("expected error registering duplicate context source id")
	}
}

func TestContextSourceRegistryRegisterRejectsEmptyID(t *testing.T) {
	r := NewContextSourceRegistry()
	if err := r.Register(&fakeContextSource{id: "", deterministic: true}); err == nil {
		t.Fatal("expected error for empty source id")
	}
}

func TestContextSourceRegistryRegisterRejectsNonDeterministic(t *testing.T) {
	r := NewContextSourceRegistry()
	if err := r.Register(&fakeContextSource{id: "vector.search", deterministic: false}); err == nil {
		t.Fatal("expected error registering a non-deterministic source (CP-41 no-vector invariant)")
	}
}

func TestContextSourceRegistryResolveUnknownFails(t *testing.T) {
	r := NewContextSourceRegistry()
	if _, err := r.Resolve("not_a_source"); err == nil {
		t.Fatal("expected error resolving unknown context source id")
	}
}

func TestContextSourceCollectDegradesOnSourceError(t *testing.T) {
	r := NewContextSourceRegistry()
	ok := &fakeContextSource{
		id: "feature.history", priority: 1, deterministic: true,
		section: FlowContextSection{SourceType: "feature.history", Priority: 1, SourceRef: "ledger", Body: "history body"},
	}
	failing := &fakeContextSource{id: "chat.summary", priority: 2, deterministic: true, err: errors.New("ledger unavailable")}
	if err := r.Register(ok); err != nil {
		t.Fatalf("register ok: %v", err)
	}
	if err := r.Register(failing); err != nil {
		t.Fatalf("register failing: %v", err)
	}

	sections, warnings := r.Collect(context.Background(), []string{"feature.history", "chat.summary"}, FlowContextHints{})
	if len(sections) != 1 {
		t.Fatalf("expected 1 section from the surviving source, got %d: %#v", len(sections), sections)
	}
	if sections[0].SourceType != "feature.history" {
		t.Fatalf("unexpected surviving section: %#v", sections[0])
	}
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning for the failing source, got %d: %#v", len(warnings), warnings)
	}
	if got := warnings[0]; got == "" {
		t.Fatal("expected non-empty warning text identifying the failing source")
	}
}

func TestContextSourceCollectUnknownIDDegradesToWarning(t *testing.T) {
	r := NewContextSourceRegistry()
	sections, warnings := r.Collect(context.Background(), []string{"does.not.exist"}, FlowContextHints{})
	if len(sections) != 0 {
		t.Fatalf("expected no sections, got %#v", sections)
	}
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning for unknown source id, got %#v", warnings)
	}
}

func TestContextSourceCollectStableOrderByPriorityThenID(t *testing.T) {
	r := NewContextSourceRegistry()
	// Register out of priority order to prove Collect sorts, not registration order.
	low := &fakeContextSource{id: "b.source", priority: 5, deterministic: true,
		section: FlowContextSection{SourceType: "b.source", Priority: 5}}
	high := &fakeContextSource{id: "a.source", priority: 1, deterministic: true,
		section: FlowContextSection{SourceType: "a.source", Priority: 1}}
	samePriorityA := &fakeContextSource{id: "z.source", priority: 3, deterministic: true,
		section: FlowContextSection{SourceType: "z.source", Priority: 3}}
	samePriorityB := &fakeContextSource{id: "y.source", priority: 3, deterministic: true,
		section: FlowContextSection{SourceType: "y.source", Priority: 3}}
	for _, s := range []ContextSource{low, high, samePriorityA, samePriorityB} {
		if err := r.Register(s); err != nil {
			t.Fatalf("register %s: %v", s.ID(), err)
		}
	}

	enabled := []string{"b.source", "a.source", "z.source", "y.source"}
	sections, warnings := r.Collect(context.Background(), enabled, FlowContextHints{})
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %#v", warnings)
	}
	got := make([]string, len(sections))
	for i, s := range sections {
		got[i] = s.SourceType
	}
	want := []string{"a.source", "y.source", "z.source", "b.source"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order mismatch at %d: got %v, want %v", i, got, want)
		}
	}

	// Run again to confirm determinism across calls.
	sections2, _ := r.Collect(context.Background(), enabled, FlowContextHints{})
	for i := range sections2 {
		if sections2[i].SourceType != sections[i].SourceType {
			t.Fatalf("order not stable across calls: first=%v second=%v", sections, sections2)
		}
	}
}

func TestNewContextSourceRegistryStartsEmpty(t *testing.T) {
	// A freshly constructed (non-default) registry has no sources — only
	// NewDefaultContextSourceRegistry (Task-192) pre-populates built-ins.
	r := NewContextSourceRegistry()
	if _, err := r.Resolve("feature.history"); err == nil {
		t.Fatal("expected feature.history to be unregistered on a bare registry")
	}
}

func TestDefaultContextSourceRegistryHasBuiltinSources(t *testing.T) {
	// Task-192: NewDefaultContextSourceRegistry pre-registers the 3 migrated
	// built-in sources.
	r := NewDefaultContextSourceRegistry()
	for _, id := range defaultContextSourceIDs {
		if _, err := r.Resolve(id); err != nil {
			t.Errorf("Resolve(%q) failed: %v", id, err)
		}
	}
}
