package runner

import (
	"context"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"flowpilot-runner/internal/tooling"
)

// ensureGitNexusIndexAsync auto-indexes a target project once per runner
// process. When <workspace> has no .gitnexus index yet, the gitnexus CLI is
// available, and the workspace is a git repo, it launches `gitnexus analyze`
// in the background (best-effort, logged, non-blocking). This is what lets
// scope-drift HighSeverity (CP-43 B14) and source.dependence (B12) use real
// dependents without the operator remembering to index the project first.
//
// Fired when a run is created against the workspace (chat / flow / workflow).
// The guard map makes it a no-op on every subsequent run in the same process;
// a failed analyze clears the guard so a later process/bind can retry.
func (s *InteractiveService) ensureGitNexusIndexAsync(workspace string) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return
	}
	if _, err := os.Stat(filepath.Join(workspace, ".gitnexus")); err == nil {
		return // already indexed
	}
	if _, err := os.Stat(filepath.Join(workspace, ".git")); err != nil {
		return // not a git workspace — gitnexus cannot index it
	}
	if tooling.CheckTool("gitnexus", workspace).Status != "ok" {
		return // CLI unavailable — leave for the operator
	}
	s.mu.Lock()
	if s.gitnexusAnalyzeOnce[workspace] {
		s.mu.Unlock()
		return
	}
	s.gitnexusAnalyzeOnce[workspace] = true
	s.mu.Unlock()

	go func() {
		log.Printf("[gitnexus] auto-index start workspace=%q (no .gitnexus index)", workspace)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, "gitnexus", "analyze")
		cmd.Dir = workspace
		out, err := cmd.CombinedOutput()
		if err != nil {
			log.Printf("[gitnexus] auto-index failed workspace=%q: %v: %s", workspace, err, strings.TrimSpace(string(out)))
			s.mu.Lock()
			delete(s.gitnexusAnalyzeOnce, workspace)
			s.mu.Unlock()
			return
		}
		log.Printf("[gitnexus] auto-index done workspace=%q", workspace)
	}()
}
