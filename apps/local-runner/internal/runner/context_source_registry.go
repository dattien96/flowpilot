package runner

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// ContextSourceID identifies a registered context source (e.g. "feature.history",
// "chat.summary", "source.excerpt", "mcp.driver"). Pack/flow data selects a
// source by this ID; it never supplies or alters the implementation (mirrors
// BehaviorID / CP-42 P-1).
type ContextSourceID string

// FlowContextSection is the generic unit of output every ContextSource
// produces. BuildFlowContextPackage composes a package from a slice of these
// instead of a fixed set of hardcoded blocks (SD-22 D-3, CP-42 P-4).
type FlowContextSection struct {
	SourceType string                // = ContextSource.ID()
	Priority   int                   // packing order (Task-168 T-4); lower runs/packs first
	SourceRef  string                // required: where this content came from (CP-41 T-3)
	Body       string                // bounded, already-rendered text content (e.g. history/discussion prose)
	Excerpts   []FlowContextExcerpt  // structured per-file content (e.g. source.excerpt); parallel to Body
	Omitted    []string              // reasons content was skipped (outside_workspace, too_large, ...)
	Warnings   []string              // source-level warnings (e.g. "no history found"); merged into pkg.Warnings by the builder
	Confidence FlowContextConfidence // zero value means "not applicable" for this source
}

// ContextSource produces one FlowContextSection deterministically from
// FlowContextHints. Implementations must not perform vector/embedding/
// similarity-search retrieval (SD-22 D-2, CP-41 no-vector invariant).
type ContextSource interface {
	// ID returns the canonical, stable identifier for this source.
	ID() string
	// Priority returns this source's default packing order.
	Priority() int
	// Deterministic must return true. The registry rejects any source that
	// returns false at Register time — pack/config data can never introduce
	// a non-deterministic (e.g. similarity-search-backed) source.
	Deterministic() bool
	// Fetch produces this source's section for the given hints. A non-nil
	// error is treated as a degrade-to-warning by Collect, never a hard
	// failure of the Plan step.
	Fetch(ctx context.Context, hints FlowContextHints) (FlowContextSection, error)
}

// ContextSourceRegistry resolves context-source IDs to their implementations
// and collects sections from a chosen subset of enabled sources. It mirrors
// BehaviorRegistry (behavior_registry.go) at the context-source layer.
type ContextSourceRegistry struct {
	sources map[ContextSourceID]ContextSource
}

// NewContextSourceRegistry returns an empty registry with no sources
// registered.
func NewContextSourceRegistry() *ContextSourceRegistry {
	return &ContextSourceRegistry{sources: make(map[ContextSourceID]ContextSource)}
}

// Register adds a context source. Registering an empty ID, a duplicate ID, or
// a source that reports itself non-deterministic is an error — a context
// source can never be silently shadowed, and a non-deterministic source can
// never enter the registry (SD-22 D-2).
func (r *ContextSourceRegistry) Register(src ContextSource) error {
	if src == nil {
		return fmt.Errorf("context source registry: nil source")
	}
	id := src.ID()
	if id == "" {
		return fmt.Errorf("context source registry: source missing id")
	}
	if !src.Deterministic() {
		return fmt.Errorf("context source registry: source %q is not deterministic; only deterministic sources may be registered (CP-41 no-vector invariant)", id)
	}
	key := ContextSourceID(id)
	if _, exists := r.sources[key]; exists {
		return fmt.Errorf("context source registry: id %q already registered", id)
	}
	r.sources[key] = src
	return nil
}

// Resolve returns the source registered under id. Unknown IDs fail fast
// rather than degrading to a no-op, so a pack referencing a source that does
// not exist can never silently run without it (SD-22 D-4).
func (r *ContextSourceRegistry) Resolve(id string) (ContextSource, error) {
	src, ok := r.sources[ContextSourceID(id)]
	if !ok {
		return nil, fmt.Errorf("context source registry: no source registered for %q", id)
	}
	return src, nil
}

// Collect runs Fetch for every enabled source ID against hints and returns
// the resulting sections plus any degrade warnings. A source that errors, or
// an unknown source ID, never fails Collect as a whole — it is recorded as a
// warning and the remaining sources still run (SD-22 D-5/F-1, Task-168 T-4
// graceful degradation).
//
// Results are sorted by Priority ascending, then by SourceType, so ordering
// is stable across calls regardless of enabledIDs order (CP-44 P-8: import
// completeness matters, not declaration order).
func (r *ContextSourceRegistry) Collect(ctx context.Context, enabledIDs []string, hints FlowContextHints) ([]FlowContextSection, []string) {
	var sections []FlowContextSection
	var warnings []string

	for _, id := range enabledIDs {
		src, err := r.Resolve(id)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", id, err))
			continue
		}
		section, err := src.Fetch(ctx, hints)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", id, err))
			continue
		}
		if len(section.Warnings) > 0 {
			warnings = append(warnings, section.Warnings...)
		}
		sections = append(sections, section)
	}

	sort.SliceStable(sections, func(i, j int) bool {
		if sections[i].Priority != sections[j].Priority {
			return sections[i].Priority < sections[j].Priority
		}
		return sections[i].SourceType < sections[j].SourceType
	})

	return sections, warnings
}

var (
	defaultContextSourceRegistryOnce sync.Once
	defaultContextSourceRegistry     *ContextSourceRegistry
)

// DefaultContextSourceRegistry returns the process-wide registry of built-in
// context sources, built once on first use. Task-191 leaves this empty
// (NewContextSourceRegistry only); Task-192 populates it with the migrated
// built-in sources via NewDefaultContextSourceRegistry.
func DefaultContextSourceRegistry() *ContextSourceRegistry {
	defaultContextSourceRegistryOnce.Do(func() {
		defaultContextSourceRegistry = NewDefaultContextSourceRegistry()
	})
	return defaultContextSourceRegistry
}

// NewDefaultContextSourceRegistry returns a registry pre-populated with the
// core built-in context sources (feature.history, chat.summary,
// source.excerpt — CP-44 P-2/Task-192).
func NewDefaultContextSourceRegistry() *ContextSourceRegistry {
	r := NewContextSourceRegistry()
	registerBuiltinContextSources(r)
	return r
}
