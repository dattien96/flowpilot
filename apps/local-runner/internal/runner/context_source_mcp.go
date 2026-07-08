package runner

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ContextSourceMCPDriver is the mcp.driver context source id (CP-44 P-5 /
// Task-195): the first non-ledger, external context source, proving the
// registry extends past the built-in 3 without touching FlowContextPackage.
const ContextSourceMCPDriver ContextSourceID = "mcp.driver"

// mcpDriverTimeout bounds how long mcpDriverSource waits for its adapter
// before treating the call as failed. CP-44 P-10: the MCP call is live/synchronous
// at Plan-time with a hard timeout; caching is deferred past v1.
const mcpDriverTimeout = 5 * time.Second

// mcpDriverContentCap bounds the returned content, matching the byte-cap
// discipline every context source must follow (CP-41 T-3: bounded + source-referenced).
const mcpDriverContentCap = 8 * 1024 // 8 KB

// MCPDriverAdapter fetches raw content for a driver reference from an
// already-connected MCP integration. CP-44 P-7 restricts this to MCPs the
// system already supports/connects — never an arbitrary command or path, so
// the adapter implementation (not this source) is the enforcement point for
// which MCPs are reachable at all.
//
// The production adapter that calls the real Chat-mode MCP driver path is a
// separate integration (tracked as a Task-195 follow-up); this interface is
// the seam it plugs into. Tests exercise this source with a fake adapter,
// matching Task-195's own DOD-1 ("chạy end-to-end với fake adapter").
type MCPDriverAdapter interface {
	Fetch(ctx context.Context, driverRef string) (content string, err error)
}

// mcpDriverSource is registered on the default registry but deliberately
// excluded from defaultContextSourceIDs — a flow must opt in via
// `contexts.<name>.sources` (Task-194) to enable it.
type mcpDriverSource struct {
	priority int
	adapter  MCPDriverAdapter
	timeout  time.Duration
}

func (s *mcpDriverSource) ID() string    { return string(ContextSourceMCPDriver) }
func (s *mcpDriverSource) Priority() int { return s.priority }

// Deterministic returns true: this source does an explicit lookup by
// driverRef, not a similarity/semantic search (SD-22 D-2), so it satisfies
// the registry's no-vector invariant even though it calls an external system.
func (s *mcpDriverSource) Deterministic() bool { return true }

// Fetch calls the adapter for hints.MCPDriverRef under a hard timeout. A
// missing driver ref is a normal "not configured" case (empty section, no
// error/warning); an adapter error or timeout is returned as an error so
// ContextSourceRegistry.Collect degrades it to a warning rather than failing
// the Plan step (CP-44 D-5, AC-9/AC-13).
func (s *mcpDriverSource) Fetch(ctx context.Context, hints FlowContextHints) (FlowContextSection, error) {
	section := FlowContextSection{SourceType: s.ID(), Priority: s.priority}
	driverRef := strings.TrimSpace(hints.MCPDriverRef)
	if driverRef == "" {
		return section, nil
	}
	if s.adapter == nil {
		return section, fmt.Errorf("mcp.driver: no adapter configured")
	}
	section.SourceRef = "mcp:" + driverRef

	timeout := s.timeout
	if timeout <= 0 {
		timeout = mcpDriverTimeout
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	content, err := s.adapter.Fetch(callCtx, driverRef)
	if err != nil {
		return section, fmt.Errorf("mcp.driver: fetch %q: %w", driverRef, err)
	}
	if len(content) > mcpDriverContentCap {
		content = content[:mcpDriverContentCap]
	}
	section.Body = content
	return section, nil
}
