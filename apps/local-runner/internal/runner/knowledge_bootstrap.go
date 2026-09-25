package runner

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"flowpilot-runner/internal/knowledge"
	"flowpilot-runner/internal/structure"
)

// knowledgeBootstrapOnce guards one distill scan per runner process per
// workspace (mirrors ensureGitNexusIndexAsync's guard map, but lock-free via
// sync.Map since the bootstrap has no failure-counter state). A failed scan
// clears the guard so a later bind can retry.
var knowledgeBootstrapOnce sync.Map // workspace -> true

// knowledgeUpdateLocks serializes incremental updates per workspace so two
// consecutive audit completions cannot interleave section merges.
var knowledgeUpdateLocks sync.Map // workspace -> *sync.Mutex

// ensureKnowledgeBaseAsync distills the living knowledge base once per
// process when a run binds a workspace. Non-blocking: the scan runs in a
// background goroutine (CP-66 constraint: never add >1s to runner startup;
// R-1: huge-repo scans stay off the UI path). distill is injected so tests
// use a counting closure instead of the GitNexus CLI.
func ensureKnowledgeBaseAsync(workspace string, distill func(ctx context.Context) error) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" || distill == nil {
		return
	}
	if knowledge.Missing(workspace) == false {
		return // already bootstrapped
	}
	if _, loaded := knowledgeBootstrapOnce.LoadOrStore(workspace, true); loaded {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		if err := distill(ctx); err != nil {
			log.Printf("[knowledge] bootstrap distill failed workspace=%q: %v", workspace, err)
			knowledgeBootstrapOnce.Delete(workspace)
			return
		}
		log.Printf("[knowledge] bootstrap distill done workspace=%q", workspace)
	}()
}

// UpdateAsync incrementally re-distills the flows affected by changedPaths.
// Synchronous and serialized per workspace; callers MUST invoke it from a
// background goroutine (`go UpdateAsync(...)`) — it can shell GitNexus and
// must never block a flow step. Missing knowledge base, empty path set, or
// any error degrades to log-only (CP-66 §8 fallback): knowledge freshness
// never fails or delays a flow.
func UpdateAsync(workspace string, changedPaths []string, redistill func(ctx context.Context) (*knowledge.KnowledgeBase, error)) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" || redistill == nil {
		return
	}
	if knowledge.Missing(workspace) {
		return // not bootstrapped — never full-rebuild mid-flow
	}
	paths := filterKnowledgePaths(changedPaths)
	if len(paths) == 0 {
		return
	}
	mu, _ := knowledgeUpdateLocks.LoadOrStore(workspace, &sync.Mutex{})
	lock := mu.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()
	if err := knowledge.IncrementalUpdate(workspace, paths, redistill); err != nil {
		log.Printf("[knowledge] incremental update failed workspace=%q: %v", workspace, err)
	}
}

// filterKnowledgePaths keeps concrete code targets only (doc/test noise never
// marks a flow stale — same predicate the retrieval locus uses).
func filterKnowledgePaths(paths []string) []string {
	var out []string
	seen := make(map[string]bool)
	for _, p := range paths {
		if !isConcreteCodeTarget(p) || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

// gitnexusProcessLister is the production knowledge.ProcessLister: real
// GitNexus graph data, LSP symbols absent in v1 (Task-373 T-1 — the distiller
// degrades to GitNexus-only; the LSP adapter is a follow-up).
type gitnexusProcessLister struct{ repoDir string }

func (l *gitnexusProcessLister) ListProcesses(ctx context.Context) ([]structure.FlowSummary, error) {
	return structure.Processes(ctx, l.repoDir, knowledge.DefaultProcessLimit)
}

func (l *gitnexusProcessLister) ListModelCandidates(ctx context.Context) ([]structure.ModelInfo, error) {
	return structure.ModelCandidates(ctx, l.repoDir, knowledge.DefaultModelLimit)
}

// ensureKnowledgeBaseForWorkspace builds the production distill closure and
// fires the background bootstrap. Called once per run creation (createRun),
// right after ensureGitNexusIndexAsync — the index it queries is the one the
// auto-indexer guarantees.
func (s *InteractiveService) ensureKnowledgeBaseForWorkspace(workspace string) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" || isRunnerManagedWorktreePath(workspace) {
		// Runner-managed worktrees are ephemeral scratch — never distill into
		// them (same BUG-460 pollution vector as the auto-indexer: writes land
		// in the candidate's captured diff).
		return
	}
	ensureKnowledgeBaseAsync(workspace, func(ctx context.Context) error {
		kb, err := knowledge.Distill(ctx, workspace, &gitnexusProcessLister{repoDir: workspace}, nil)
		if err != nil {
			return err
		}
		if err := knowledge.WriteFull(workspace, kb); err != nil {
			return err
		}
		// A successful full distill already covers every previously pending
		// incremental path — the ledger is clean, not silently stale (BUG-477).
		clearKnowledgePending(workspace)
		return nil
	})
	// Boot reconciliation (BUG-477): a kill can strand durable update intents
	// whose worker never finished. On bind, replay whatever is still pending —
	// cheap no-op when the ledger is empty, and a missing index stays owned by
	// the bootstrap path above.
	redistill := func(ctx context.Context) (*knowledge.KnowledgeBase, error) {
		return knowledge.Distill(ctx, workspace, &gitnexusProcessLister{repoDir: workspace}, nil)
	}
	if pending, err := loadKnowledgePending(workspace); err == nil && len(pending.Intents) > 0 {
		go replayKnowledgeUpdates(workspace, redistill)
	} else if err != nil {
		// Corrupt ledger — fail closed to a rebuild through the same worker.
		go replayKnowledgeUpdates(workspace, redistill)
	}
}

// updateKnowledgeForAudit fires the P-3 incremental update after an audit
// node completes. Always background, always best-effort.
func (s *InteractiveService) updateKnowledgeForAudit(workspace string, changedPaths []string) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return
	}
	redistill := func(ctx context.Context) (*knowledge.KnowledgeBase, error) {
		return knowledge.Distill(ctx, workspace, &gitnexusProcessLister{repoDir: workspace}, nil)
	}
	paths := filterKnowledgePaths(changedPaths)
	if len(paths) == 0 || knowledge.Missing(workspace) {
		// No bootstrapped base: the hook must stay a no-op — it never creates
		// knowledge state mid-flow (TestAuditHookSkipsUnbootstrappedWorkspace).
		// The bootstrap's full distill covers these paths anyway.
		return
	}
	// BUG-477: persist the dirty set BEFORE acknowledging the hook — a kill
	// between here and the worker's write used to lose the update forever.
	// An append failure still fires the best-effort update (never blocks the
	// flow), and the next append rewrites the ledger wholesale anyway.
	if err := appendKnowledgeUpdateIntent(workspace, paths); err != nil {
		log.Printf("[knowledge] persist update intent failed workspace=%q: %v — firing best-effort update", workspace, err)
		go UpdateAsync(workspace, changedPaths, redistill)
		return
	}
	go replayKnowledgeUpdates(workspace, redistill)
}

// onAuditNodeCompleted is the single choke point every audit completion path
// calls (CP-66 P-3 / Task-375) — and the ONLY knowledge-update trigger in any
// flow: implement/validate/reviewer nodes never reach it, so mid-flight code
// can never mark distilled knowledge stale. Fire-and-forget background;
// verdicts and flow state are untouched by construction (no return values,
// no error propagation).
func (s *InteractiveService) onAuditNodeCompleted(workspace string, changedFiles []string) {
	s.updateKnowledgeForAudit(workspace, auditKnowledgePaths(changedFiles))
}

// auditKnowledgePaths filters an audit turn's changed files down to concrete
// code targets (Task-375 T-3: doc/test noise never marks a flow stale).
// Empty output means "nothing knowledge-worthy changed" — UpdateAsync then
// no-ops without spawning work.
func auditKnowledgePaths(changedFiles []string) []string {
	return filterKnowledgePaths(changedFiles)
}
