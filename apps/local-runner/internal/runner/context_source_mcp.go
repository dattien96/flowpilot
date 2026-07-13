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

// mcpBoundedFetch is the shared bounded-fetch contract every MCP-backed
// context source follows (CP-44 D-5, Task-226): call an external MCP under a
// hard timeout, cap the returned content, and wrap any failure so
// ContextSourceRegistry.Collect degrades it to a warning rather than failing
// the Plan step. Task-226 extracts this out of mcpDriverSource so new
// MCP-backed sources (jira.issue, jira.sprint, firebase.crashlytics) reuse the
// same timeout/cap/degrade discipline instead of re-deriving it — each new
// source still owns its own struct + adapter interface (mirroring
// mcpDriverSource below), it just shares this one call shape.
func mcpBoundedFetch(ctx context.Context, timeout time.Duration, defaultTimeout time.Duration, cap int, sourceID string, ref string, fetch func(context.Context, string) (string, error)) (string, error) {
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	content, err := fetch(callCtx, ref)
	if err != nil {
		return "", fmt.Errorf("%s: fetch %q: %w", sourceID, ref, err)
	}
	if cap > 0 && len(content) > cap {
		content = content[:cap]
	}
	return content, nil
}

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

	content, err := mcpBoundedFetch(ctx, s.timeout, mcpDriverTimeout, mcpDriverContentCap, s.ID(), driverRef, s.adapter.Fetch)
	if err != nil {
		return section, err
	}
	section.Body = content
	return section, nil
}

// googleDriveDriverAdapter is Task-204's production MCPDriverAdapter: it
// backs mcp.driver with a real Google Drive document read instead of the
// fake adapter Task-195's own tests use. driverRef is treated as a Google
// Drive file id directly (Task-204 Q-2) — readGoogleDriveDocument already
// takes one, and no per-step indirection layer exists to map anything else
// onto it.
//
// Account resolution deliberately reuses resolveGoogleDriveAccessTokenForRunner
// (extracted from proxyMcpServer.accessToken(), Task-204 T-3) — the exact
// same OAuth-client + active-account + credential chain the Chat-mode Google
// Drive MCP proxy already uses, per Task-204's constraint against inventing
// a second way to resolve "which account backs this workspace." That chain
// depends only on *Runner (process/workspace-scoped config), not on any
// per-run state, so there is nothing to thread through FlowContextHints for
// account identity (Task-204 Q-1) — this adapter is bound to a *Runner once,
// not re-resolved per call.
type googleDriveDriverAdapter struct {
	runner *Runner
}

// Fetch resolves a live access token for whichever Google Drive account is
// currently connected/selected for this workspace, then reads driverRef as a
// file id. Any failure (no account connected, expired/missing credentials,
// unreadable file) is returned as an error — ContextSourceRegistry.Collect
// degrades that to a warning rather than failing the Plan step (Task-195's
// existing contract, unchanged), and never falls back to a different
// account's content (Task-204's fail-closed constraint).
func (a *googleDriveDriverAdapter) Fetch(ctx context.Context, driverRef string) (string, error) {
	if a.runner == nil {
		return "", fmt.Errorf("mcp.driver: google drive runner not configured")
	}
	accessToken, err := resolveGoogleDriveAccessTokenForRunner(a.runner)
	if err != nil {
		return "", fmt.Errorf("mcp.driver: resolve google drive access token: %w", err)
	}
	content, err := readGoogleDriveDocument(accessToken, driverRef)
	if err != nil {
		return "", fmt.Errorf("mcp.driver: read google drive document %q: %w", driverRef, err)
	}
	return content, nil
}

// SetMCPDriverAdapter wires a production MCPDriverAdapter onto the already-
// registered mcp.driver source (Task-204 T-4). A no-op if mcp.driver isn't
// registered on r (shouldn't happen for DefaultContextSourceRegistry, whose
// registerBuiltinContextSources always registers it) — this never fails
// construction, it only leaves mcp.driver degrading exactly as it did before
// this method was ever called.
func (r *ContextSourceRegistry) SetMCPDriverAdapter(adapter MCPDriverAdapter) {
	src, err := r.Resolve(string(ContextSourceMCPDriver))
	if err != nil {
		return
	}
	if mcp, ok := src.(*mcpDriverSource); ok {
		mcp.adapter = adapter
	}
}
