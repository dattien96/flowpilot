package runner

// Run/chat worktree isolation (CP-71 / SS-23 / SD-27). An opted-in run executes
// inside a git worktree owned by a worktreeOwnerID — the chatId for chat runs
// (every provider-switch leg shares it, D-8) or the flow run id for harness/vibe
// flow runs. Isolation is purely a working-directory concern: the provider cwd
// is pointed at the worktree and no engine/gate/prompt behavior changes.
//
// Provider-agnostic by construction (Case-1): nothing here branches on
// providerKey; the binding rides the existing workspaceCwd channel.

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"strings"

	"flowpilot-runner/internal/worktree"
)

// worktreeResolution is the durable intent for one destructive merge
// decision (BUG-481). It is persisted on the session binding BEFORE external
// effects run and advanced monotonically; Phase is cleared together with the
// intent at finalize. A kill/restart resumes from the durable phase so only
// missing idempotent effects re-run.
type worktreeResolution struct {
	ID    string `json:"id,omitempty"`
	Mode  string `json:"mode,omitempty"`
	Phase string `json:"phase,omitempty"` // resolution_requested|repository_effect_committed|cleanup_committed
}

// worktreeBinding is the persisted run-record binding (SD-27 §5). All fields
// are omitempty so toggle-off runs serialize byte-identically to before.
type worktreeBinding struct {
	OwnerID    string `json:"ownerId,omitempty"`
	Path       string `json:"path,omitempty"`
	Branch     string `json:"branch,omitempty"`
	BaseCommit string `json:"baseCommit,omitempty"`
	Slug       string `json:"slug,omitempty"`
	// State: none|active|merge_pending|merged|kept_branch|discarded|lost
	State      string              `json:"state,omitempty"`
	Enabled    bool                `json:"enabled,omitempty"`
	Resolution *worktreeResolution `json:"resolution,omitempty"`
}

// worktreeView is the client-facing projection on runSnapshotView.
type worktreeView struct {
	OwnerID    string `json:"ownerId,omitempty"`
	Path       string `json:"path,omitempty"`
	Branch     string `json:"branch,omitempty"`
	BaseCommit string `json:"baseCommit,omitempty"`
	Slug       string `json:"slug,omitempty"`
	State      string `json:"state,omitempty"`
	Enabled    bool   `json:"enabled,omitempty"`
	// Resolution exposes an in-flight destructive merge decision (BUG-481)
	// so clients can render "resolution in progress" instead of a stale
	// card. Additive — nil when no intent is recorded.
	Resolution *worktreeResolution `json:"resolution,omitempty"`
}

// worktreeAllowedClients mirrors the working_mode client gate (SD-27 §6):
// only desktop|tui may opt a run into worktree isolation.
func worktreeAllowedClient(client string) bool {
	return client == "desktop" || client == "tui"
}

// enforceWorktreeStart rejects worktree:true from disallowed clients
// (403 worktree_client_forbidden). Called in handleStartRun next to
// enforceWorkingModeStart.
func (s *InteractiveService) enforceWorktreeStart(in *StartRunInput) *apiErr {
	if in == nil || !in.Worktree {
		return nil
	}
	if !worktreeAllowedClient(in.Client) {
		return newAPIErr(http.StatusForbidden, "worktree_client_forbidden",
			"worktree isolation is only available from desktop|tui clients")
	}
	return nil
}

// worktreeOwnerIDFor resolves the owner key (SD-27 D-8): chatId for chat runs,
// the run's own id for flow runs.
func worktreeOwnerIDFor(chatID, runID, runKind string) string {
	if runKind == "chat" && chatID != "" {
		return chatID
	}
	return runID
}

// worktreeRepoDir resolves the repository dir the worktree is rooted in: the
// run's requested cwd, else the runner's default workspace.
func (s *InteractiveService) worktreeRepoDir(in StartRunInput) string {
	if cwd := strings.TrimSpace(in.Cwd); cwd != "" {
		return cwd
	}
	if s.runner != nil {
		return strings.TrimSpace(s.runner.workspace)
	}
	return ""
}

// findChatWorktreeBindingLocked returns the chat's existing live binding.
// Caller must hold s.mu. Resident runs win; the persisted session store is
// consulted for legs that survived a restart (SD-27 D-8 lookup order).
//
// BUG-490: a store read error propagates — "no persisted binding" is only a
// truthful answer when the store actually read clean. Answering nil on fault
// let run-start provision a second worktree for a chat with a live binding.
func (s *InteractiveService) findChatWorktreeBindingLocked(chatID string) (*worktreeBinding, error) {
	for _, rs := range s.runs {
		if rs.chatID == chatID && rs.worktree != nil && rs.worktree.State != "lost" {
			return rs.worktree, nil
		}
	}
	if reader, ok := s.workflowStore.(ChatSessionReader); ok {
		rows, err := reader.ListProviderSessionsByChat(context.Background(), chatID)
		if err != nil {
			return nil, fmt.Errorf("findChatWorktreeBindingLocked: leg scan failed for chat %s: %w", chatID, err)
		}
		for i := len(rows) - 1; i >= 0; i-- { // newest leg first
			if rows[i].WorktreeState != "" && rows[i].WorktreeState != "lost" && rows[i].WorktreePath != "" {
				return &worktreeBinding{
					OwnerID:    rows[i].WorktreeOwnerID,
					Path:       rows[i].WorktreePath,
					Branch:     rows[i].WorktreeBranch,
					BaseCommit: rows[i].WorktreeBaseCommit,
					Slug:       rows[i].WorktreeSlug,
					State:      rows[i].WorktreeState,
					Enabled:    rows[i].WorktreeEnabled,
				}, nil
			}
		}
	}
	return nil, nil
}

// provisionRunWorktree creates or inherits the worktree for rs and points the
// run's cwd at it. Caller must hold s.mu. On any failure the run record is
// left unbound and the caller aborts the start.
func (s *InteractiveService) provisionRunWorktree(rs *interactiveRun, repoDir, ownerID string, inherited *worktreeBinding) *apiErr {
	repoDir = strings.TrimSpace(repoDir)
	if repoDir == "" {
		return newAPIErr(http.StatusBadRequest, "worktree_unavailable",
			"worktree requires a resolvable workspace directory")
	}
	mgr := worktree.NewManager()
	ctx := context.Background()

	if inherited != nil {
		// Leg inheritance (D-8): validate before reuse (D-7) — never recreate
		// or duplicate; a broken binding blocks the leg (SS-23 E-8).
		if err := mgr.Validate(ctx, repoDir, inherited.OwnerID, ""); err != nil {
			return newAPIErr(http.StatusConflict, "worktree_lost",
				fmt.Sprintf("worktree for this chat is missing or unregistered (%v); merge/discard decisions remain available in the owning run", err))
		}
		inherited.State = "active"
		rs.worktree = inherited
		rs.workspaceCwd = inherited.Path
		return nil
	}

	if _, err := exec.Command("git", "-C", repoDir, "rev-parse", "--git-dir").CombinedOutput(); err != nil {
		return newAPIErr(http.StatusBadRequest, "worktree_unavailable",
			"worktree requires a git repository: "+repoDir)
	}
	head, err := gitHeadAt(repoDir)
	if err != nil {
		return newAPIErr(http.StatusBadRequest, "worktree_unavailable", err.Error())
	}
	slug := worktree.Slugify(runSlugSource(rs))
	if slug == "" {
		slug = "run"
	}
	info, err := mgr.Create(ctx, repoDir, ownerID, "", head, slug)
	if err != nil {
		return newAPIErr(http.StatusConflict, "worktree_create_failed", err.Error())
	}
	rs.worktree = &worktreeBinding{
		OwnerID:    ownerID,
		Path:       info.Path,
		Branch:     info.Branch,
		BaseCommit: info.BaseCommit,
		Slug:       slug,
		State:      "active",
		Enabled:    true,
	}
	rs.workspaceCwd = info.Path
	return nil
}

// gitHeadAt returns the repo's HEAD sha — the worktree base anchor (D-4b,
// captured at createRun for every run kind; never reused from flow-only
// flowStartGitHead).
func gitHeadAt(repoDir string) (string, error) {
	out, err := exec.Command("git", "-C", repoDir, "rev-parse", "HEAD").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("cannot resolve HEAD in %s: %s", repoDir, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// runSlugSource picks a human-readable slug seed for the worktree branch.
func runSlugSource(rs *interactiveRun) string {
	for _, cand := range []string{rs.lastPrompt, rs.label, rs.chatFlowRef, rs.workflowID} {
		if s := strings.TrimSpace(cand); s != "" {
			return s
		}
	}
	return ""
}

// worktreeFieldsToSession copies the binding onto the persisted session record.
func worktreeFieldsToSession(st *ProviderSessionState, b *worktreeBinding) {
	if b == nil {
		return
	}
	st.WorktreeOwnerID = b.OwnerID
	st.WorktreePath = b.Path
	st.WorktreeBranch = b.Branch
	st.WorktreeBaseCommit = b.BaseCommit
	st.WorktreeSlug = b.Slug
	st.WorktreeState = b.State
	st.WorktreeEnabled = b.Enabled
	st.WorktreeResolutionID = ""
	st.WorktreeResolutionMode = ""
	st.WorktreeResolutionPhase = ""
	if b.Resolution != nil {
		st.WorktreeResolutionID = b.Resolution.ID
		st.WorktreeResolutionMode = b.Resolution.Mode
		st.WorktreeResolutionPhase = b.Resolution.Phase
	}
}

// worktreeBindingFromSession rebuilds the binding from a persisted record.
func worktreeBindingFromSession(st ProviderSessionState) *worktreeBinding {
	if st.WorktreePath == "" && st.WorktreeOwnerID == "" {
		return nil
	}
	b := &worktreeBinding{
		OwnerID:    st.WorktreeOwnerID,
		Path:       st.WorktreePath,
		Branch:     st.WorktreeBranch,
		BaseCommit: st.WorktreeBaseCommit,
		Slug:       st.WorktreeSlug,
		State:      st.WorktreeState,
		Enabled:    st.WorktreeEnabled,
	}
	if st.WorktreeResolutionID != "" {
		b.Resolution = &worktreeResolution{
			ID:    st.WorktreeResolutionID,
			Mode:  st.WorktreeResolutionMode,
			Phase: st.WorktreeResolutionPhase,
		}
	}
	return b
}

// markChatWorktreeState updates the persisted worktree state on every session
// record of a chat so later legs read the latest lifecycle value.
//
// BUG-490: the leg list error and each row's persist error were dropped —
// a store fault silently left persisted legs on the stale lifecycle value
// that findChatWorktreeBindingLocked later adopts as authoritative. The list
// error propagates; per-row persist failures are logged and aggregated.
func (s *InteractiveService) markChatWorktreeState(chatID, state string) error {
	if chatID == "" {
		return nil
	}
	if reader, ok := s.workflowStore.(ChatSessionReader); ok {
		rows, err := reader.ListProviderSessionsByChat(context.Background(), chatID)
		if err != nil {
			return fmt.Errorf("markChatWorktreeState: leg scan failed for chat %s: %w", chatID, err)
		}
		var firstErr error
		for _, row := range rows {
			row.WorktreeState = state
			if perr := s.persistProviderSession(row); perr != nil {
				fmt.Printf("[worktree] markChatWorktreeState persist failed chat=%s run=%s state=%s: %v\n", chatID, row.RunID, state, perr)
				if firstErr == nil {
					firstErr = perr
				}
			}
		}
		if firstErr != nil {
			return fmt.Errorf("markChatWorktreeState: persist failed for chat %s: %w", chatID, firstErr)
		}
	}
	s.mu.Lock()
	for _, rs := range s.runs {
		if rs.chatID == chatID && rs.worktree != nil {
			rs.worktree.State = state
		}
	}
	s.mu.Unlock()
	return nil
}

// worktreeViewOf projects the binding for snapshots; nil-safe.
func worktreeViewOf(b *worktreeBinding) *worktreeView {
	if b == nil {
		return nil
	}
	return &worktreeView{
		OwnerID:    b.OwnerID,
		Path:       b.Path,
		Branch:     b.Branch,
		BaseCommit: b.BaseCommit,
		Slug:       b.Slug,
		State:      b.State,
		Enabled:   b.Enabled,
		Resolution: b.Resolution,
	}
}
