package lsp

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	// diagnosticsTimeout caps the wait for publishDiagnostics per file.
	diagnosticsTimeout = 5 * time.Second
	// serverReadyTimeout caps the initialize handshake on cold start.
	serverReadyTimeout = 5 * time.Second
	// gradleValidationTimeout caps the Android fallback compile.
	gradleValidationTimeout = 120 * time.Second
	// idleShutdownAfter reaps servers nobody asked about for this long.
	idleShutdownAfter = 5 * time.Minute
	// maxParallelChecks bounds concurrent per-file diagnostics queries.
	maxParallelChecks = 4
	// startCooldown suppresses respawn attempts for a repeatedly failing
	// server so a broken binary cannot stall every turn.
	startCooldown = 60 * time.Second
)

// PostWriteDiagnosticsHook queries one LSP server about one file and
// renders any compiler errors for an agent reprompt. The zero value is
// inert; a nil *PostWriteDiagnosticsHook is also safe to call (skips).
type PostWriteDiagnosticsHook struct {
	Manager       *ServerManager
	DocSync       *DocumentSyncManager
	Collector     *DiagnosticsCollector
	WorkspaceRoot string
	// DiagnosticsTimeout caps the per-file wait; <=0 means diagnosticsTimeout.
	DiagnosticsTimeout time.Duration
}

func (h *PostWriteDiagnosticsHook) timeout() time.Duration {
	if h != nil && h.DiagnosticsTimeout > 0 {
		return h.DiagnosticsTimeout
	}
	return diagnosticsTimeout
}

// ShouldRun reports whether the hook can do anything useful for relPath.
func (h *PostWriteDiagnosticsHook) ShouldRun(relPath string) bool {
	if h == nil || h.Manager == nil || !h.Manager.IsRunning() {
		return false
	}
	if h.DocSync == nil || h.Collector == nil {
		return false
	}
	return LanguageID(relPath) != ""
}

// AfterFileWrite syncs the file to the server, waits for diagnostics and
// returns them formatted for an agent reprompt, or "" when clean or when
// the server is unavailable. An empty newContent reads the file from disk.
// It never returns an error for graceful-skip situations; the error return
// is reserved for unexpected failures and callers treat any error as skip.
func (h *PostWriteDiagnosticsHook) AfterFileWrite(relPath, newContent string) (string, error) {
	errs, err := h.AfterFileWriteErrors(relPath, newContent)
	if err != nil {
		return "", err
	}
	if len(errs) == 0 {
		return "", nil
	}
	return FormatDiagnosticsForAgent(errs), nil
}

// AfterFileWriteErrors is AfterFileWrite in structured form: the errors for
// exactly this file (not the whole collector), so callers can decide
// follow-up validation (e.g. the Gradle fallback) before formatting.
func (h *PostWriteDiagnosticsHook) AfterFileWriteErrors(relPath, newContent string) ([]FileDiagnostic, error) {
	if !h.ShouldRun(relPath) {
		return nil, nil
	}
	abs := relPath
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(h.WorkspaceRoot, relPath)
	}
	content := newContent
	if content == "" {
		b, err := os.ReadFile(abs)
		if err != nil {
			// Deleted or moved mid-turn: nothing to diagnose.
			return nil, nil
		}
		content = string(b)
	}
	uri := FileURI(abs)
	if err := h.DocSync.ChangeDocument(uri, content); err != nil {
		log.Printf("[lsp] doc sync failed for %s: %v (skipping diagnostics)", abs, err)
		return nil, nil
	}
	if err := h.Collector.WaitForDiagnostics(context.Background(), h.timeout()); err != nil {
		return nil, nil // timeout or cancel: skip gracefully
	}
	var errs []FileDiagnostic
	for _, d := range h.Collector.GetDiagnostics(uri) {
		if d.Severity == DiagnosticSeverityError {
			errs = append(errs, FileDiagnostic{URI: uri, Diagnostic: d})
		}
	}
	return errs, nil
}

// Server bundles one workspace+language LSP session.
type Server struct {
	Config  PlatformLSPConfig
	Root    string
	Manager *ServerManager
	Docs    *DocumentSyncManager
	Diags   *DiagnosticsCollector

	lastUsed time.Time // guarded by ServerSet.mu
}

func (s *Server) hook() *PostWriteDiagnosticsHook {
	return &PostWriteDiagnosticsHook{
		Manager: s.Manager, DocSync: s.Docs, Collector: s.Diags,
		WorkspaceRoot: s.Root,
	}
}

// serverKey identifies one server: a workspace may host several languages.
type serverKey struct {
	root     string
	platform string
}

// ServerSet owns the Hub & Spoke fan-out: one child LSP process per
// (workspace, language), each spoken to over its own stdio pipes. Servers
// start lazily on first use and are reaped after idleShutdownAfter.
type ServerSet struct {
	mu       sync.Mutex
	registry Registry
	servers  map[serverKey]*Server
	cooldown map[serverKey]time.Time
	// disabled remembers (root, platform) pairs whose crash budget was spent
	// earlier in this session (CP-63 R-1): a later check must not respawn a
	// fresh manager with a reset budget — the session degrades to build/test
	// validation instead. Cleared only when the process exits.
	disabled map[serverKey]bool
	// disabledNotified remembers keys already logged as session-disabled so
	// the per-turn hook does not spam the gate log.
	disabledNotified map[serverKey]bool
	// warned remembers binaries already reported missing so the gate log
	// warns once per session instead of every turn.
	warned map[string]bool
	// InitOptionsFor optionally supplies per-workspace initialization
	// options per platform (e.g. KotlinInitOptionsFor for android). It must
	// be fast and non-blocking; nil disables it.
	InitOptionsFor func(platform, workspaceRoot string) map[string]any
}

// NewServerSet builds a set over reg (DefaultRegistry when nil).
func NewServerSet(reg Registry) *ServerSet {
	if reg == nil {
		reg = DefaultRegistry()
	}
	return &ServerSet{registry: reg, servers: make(map[serverKey]*Server), cooldown: make(map[serverKey]time.Time), disabled: make(map[serverKey]bool), disabledNotified: make(map[serverKey]bool), warned: make(map[string]bool)}
}

var defaultSet = NewServerSet(nil)

// DefaultSet is the process-wide set the runner gate consults.
func DefaultSet() *ServerSet { return defaultSet }

// CheckFiles returns formatted compiler errors for the owned files in
// relPaths, or "" when clean, unsupported or unavailable. It never fails
// the caller: every degradation path logs and yields "".
func (s *ServerSet) CheckFiles(ctx context.Context, workspaceRoot string, relPaths []string) string {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(workspaceRoot) == "" || len(relPaths) == 0 {
		return ""
	}
	root, err := filepath.Abs(workspaceRoot)
	if err != nil {
		root = workspaceRoot
	}
	platform := DetectPlatform(root)
	cfg, ok := s.registry.Lookup(platform)
	if !ok {
		return ""
	}
	var owned []string
	seen := make(map[string]struct{})
	for _, p := range relPaths {
		if _, ok := s.registry.ConfigForFile(platform, p); !ok {
			continue
		}
		if _, dup := seen[p]; dup {
			continue
		}
		seen[p] = struct{}{}
		owned = append(owned, p)
	}
	if len(owned) == 0 {
		return ""
	}
	srv, err := s.getOrStart(ctx, root, cfg)
	if err != nil {
		// Silent skip: the first occurrence of each cause already warned
		// inside getOrStart (missing binary warns once; broken servers are
		// cooldown-gated). Per-turn logging here would spam the gate log.
		return ""
	}
	hook := srv.hook()
	perFile := make([][]FileDiagnostic, len(owned))
	var wg sync.WaitGroup
	sem := make(chan struct{}, maxParallelChecks)
	for i, rel := range owned {
		wg.Add(1)
		go func(i int, rel string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			errs, _ := hook.AfterFileWriteErrors(rel, "")
			perFile[i] = errs
		}(i, rel)
	}
	wg.Wait()
	s.touch(srv)
	s.stopIdle(idleShutdownAfter)
	var lspErrs []FileDiagnostic
	for _, errs := range perFile {
		lspErrs = append(lspErrs, errs...)
	}
	var out []string
	if text := FormatDiagnosticsForAgent(lspErrs); text != "" {
		out = append(out, text)
	}
	// Android deeper validation: kotlin-language-server misses R-class and
	// Compose generated-type errors, so a clean LSP result triggers the
	// Gradle fallback (Task-361). Runs only when the LSP server actually
	// ran (unavailable server => unknown state, not "clean").
	if ShouldRunGradleFallback(lspErrs, platform) {
		if gradleText := s.runGradleFallback(ctx, root); gradleText != "" {
			out = append(out, gradleText)
		}
	}
	return strings.Join(out, "\n")
}

// runGradleFallback validates an Android workspace with compileDebugKotlin
// and formats any errors. All failures degrade to "" (caller proceeds).
func (s *ServerSet) runGradleFallback(ctx context.Context, root string) string {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, gradleValidationTimeout)
	defer cancel()
	errs, err := RunGradleValidation(ctx, root)
	if err != nil || len(errs) == 0 {
		return ""
	}
	return FormatGradleErrorsForAgent(errs)
}

// ServerCount reports how many servers are currently owned (tests).
func (s *ServerSet) ServerCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.servers)
}

// Close stops every owned server. Safe on empty sets; used at runner
// shutdown and in tests (register after TempDir creation so helpers die
// before TempDir removal on Windows).
func (s *ServerSet) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, srv := range s.servers {
		_ = srv.Manager.Stop()
		delete(s.servers, key)
	}
}

// getOrStart returns the live server for root+platform, spawning and
// handshaking when needed. It holds s.mu throughout: spawns are rare
// (cold start only) and this prevents duplicate processes on races.
func (s *ServerSet) getOrStart(ctx context.Context, root string, cfg PlatformLSPConfig) (*Server, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := serverKey{root: root, platform: cfg.Platform}
	if srv, ok := s.servers[key]; ok {
		if srv.Manager.IsRunning() {
			srv.lastUsed = time.Now()
			return srv, nil
		}
		// CP-63 R-1: a manager whose crash budget was spent stays disabled
		// for the whole session — record it so no later check respawns a
		// fresh manager with a reset budget.
		if srv.Manager.Disabled() {
			s.disabled[key] = true
		}
		delete(s.servers, key)
	}
	if s.disabled[key] {
		if !s.disabledNotified[key] {
			s.disabledNotified[key] = true
			log.Printf("[lsp] server for %s/%s disabled for this session (crash budget spent earlier); build/test validation remains the backstop", root, cfg.Platform)
		}
		return nil, fmt.Errorf("lsp: server for %s/%s disabled for this session", root, cfg.Platform)
	}
	if until, ok := s.cooldown[key]; ok {
		if time.Now().Before(until) {
			return nil, fmt.Errorf("lsp: server for %s/%s in cooldown", root, cfg.Platform)
		}
		delete(s.cooldown, key)
	}
	if _, err := exec.LookPath(cfg.Binary); err != nil {
		if !s.warned[cfg.Binary] {
			s.warned[cfg.Binary] = true
			log.Printf("[lsp] server binary %q for platform %q not found in PATH (%s); diagnostics disabled for this session",
				cfg.Binary, cfg.Platform, cfg.InstallHint)
		}
		return nil, fmt.Errorf("lsp: server binary %q for platform %q not found in PATH: %w", cfg.Binary, cfg.Platform, err)
	}
	mgr := &ServerManager{}
	if hook := s.initOptionsFor(cfg.Platform, root); len(hook) > 0 {
		mgr.InitOptions = hook
	}
	if err := mgr.Start(ctx, cfg.Binary, cfg.Args, root); err != nil {
		s.cooldown[key] = time.Now().Add(startCooldown)
		log.Printf("[lsp] server start failed for %s/%s: %v", root, cfg.Platform, err)
		return nil, err
	}
	srv := &Server{
		Config: cfg, Root: root,
		Manager:  mgr,
		Docs:     NewDocumentSyncManager(mgr.Client()),
		Diags:    NewDiagnosticsCollector(mgr.Client()),
		lastUsed: time.Now(),
	}
	if err := mgr.WaitReady(ctx, serverReadyTimeout); err != nil {
		_ = mgr.Stop()
		s.cooldown[key] = time.Now().Add(startCooldown)
		log.Printf("[lsp] server not ready for %s/%s: %v", root, cfg.Platform, err)
		return nil, err
	}
	s.servers[key] = srv
	return srv, nil
}

func (s *ServerSet) touch(srv *Server) {
	s.mu.Lock()
	defer s.mu.Unlock()
	srv.lastUsed = time.Now()
}

// initOptionsFor invokes the InitOptionsFor hook. Caller holds s.mu; the
// hook runs under that lock, hence the fast-and-non-blocking contract.
func (s *ServerSet) initOptionsFor(platform, root string) map[string]any {
	if s.InitOptionsFor == nil {
		return nil
	}
	return s.InitOptionsFor(platform, root)
}

// stopIdle stops servers idle longer than maxIdle. Callers invoke it
// opportunistically (end of CheckFiles), so no reaper goroutine is needed.
func (s *ServerSet) stopIdle(maxIdle time.Duration) {
	if maxIdle <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for key, srv := range s.servers {
		if now.Sub(srv.lastUsed) > maxIdle {
			_ = srv.Manager.Stop()
			// Preserve an exhausted crash budget even when another workspace's
			// check reaps this server before its next getOrStart call.
			if srv.Manager.Disabled() {
				s.disabled[key] = true
			}
			delete(s.servers, key)
			log.Printf("[lsp] lsp.idle_stop root=%q platform=%q", srv.Root, srv.Config.Platform)
		}
	}
}
