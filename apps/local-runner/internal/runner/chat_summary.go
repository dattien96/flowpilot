package runner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"flowpilot-runner/internal/changeledger"
	"flowpilot-runner/internal/featurecatalog"
	"flowpilot-runner/internal/promptblock"
)

const chatSummaryMaxBytes = 900
const defaultSummaryIdleWindow = 5 * time.Minute

// summaryIdleWindow is how long a chat must be idle before its rolling summary is
// generated. Overridable via FLOWPILOT_SUMMARY_IDLE_MS (milliseconds) for tests.
func summaryIdleWindow() time.Duration {
	if ms := strings.TrimSpace(os.Getenv("FLOWPILOT_SUMMARY_IDLE_MS")); ms != "" {
		if n, err := strconv.Atoi(ms); err == nil && n > 0 {
			return time.Duration(n) * time.Millisecond
		}
	}
	return defaultSummaryIdleWindow
}

// scheduleChatSummary (re)arms the per-run idle timer: after the idle window with
// no new turn, the rolling summary is generated. A new turn calls
// cancelChatSummary, so the window restarts from zero.
func (s *InteractiveService) scheduleChatSummary(runID string) {
	if runID == "" {
		return
	}
	s.summaryMu.Lock()
	defer s.summaryMu.Unlock()
	if t := s.summaryTimers[runID]; t != nil {
		t.Stop()
	}
	s.summaryTimers[runID] = time.AfterFunc(summaryIdleWindow(), func() {
		s.summaryMu.Lock()
		delete(s.summaryTimers, runID)
		s.summaryMu.Unlock()
		s.summarizeRunIfIdle(runID)
	})
}

// cancelChatSummary stops and clears any pending idle-summary timer for a run.
func (s *InteractiveService) cancelChatSummary(runID string) {
	s.summaryMu.Lock()
	defer s.summaryMu.Unlock()
	if t := s.summaryTimers[runID]; t != nil {
		t.Stop()
		delete(s.summaryTimers, runID)
	}
}

// summarizeRunIfIdle generates the rolling summary for a live run, but only when
// the run is still present and idle (not mid-turn or awaiting approval/question).
func (s *InteractiveService) summarizeRunIfIdle(runID string) bool {
	s.mu.Lock()
	rs := s.runs[runID]
	var turnID string
	idle := false
	if rs != nil {
		idle = !rs.turnInFlight && rs.pendingApprovalID == "" && rs.pendingQuestionID == ""
		turnID = rs.lastTurnID
	}
	s.mu.Unlock()
	if rs == nil || !idle {
		return false
	}
	job, ok := s.newChatSummaryJob(rs, turnID)
	if !ok {
		return false
	}
	return s.runChatSummaryJob(job)
}

// chatSummaryJob is a self-contained snapshot of a completed turn so the summary
// can be produced off the turn-finalization path without touching the live run.
type chatSummaryJob struct {
	runID     string
	projectID string
	cwd       string
	turnID    string
	provider  ProviderKey
	turns     []transcriptTurn
}

type generateChatSummaryResponse struct {
	RunID     string `json:"runId"`
	Generated bool   `json:"generated"`
	Skipped   bool   `json:"skipped"`
	Reason    string `json:"reason,omitempty"`
}

// handleGenerateChatSummary serves the manual "Gen summary" button: it generates
// the rolling summary immediately (bypassing the idle wait), errors if the chat
// is mid-turn, and is a no-op when the stored summary already matches.
func (s *InteractiveService) handleGenerateChatSummary(w http.ResponseWriter, r *http.Request) {
	result, apiErr := s.generateChatSummaryNow(r.PathValue("runId"))
	if apiErr != nil {
		writeInteractiveError(w, apiErr)
		return
	}
	writeInteractiveJSON(w, http.StatusOK, result)
}

func (s *InteractiveService) generateChatSummaryNow(runID string) (generateChatSummaryResponse, *apiErr) {
	s.mu.Lock()
	rs := s.runs[runID]
	var busy bool
	var turnID string
	if rs != nil {
		busy = rs.turnInFlight || rs.pendingApprovalID != "" || rs.pendingQuestionID != ""
		turnID = rs.lastTurnID
	}
	s.mu.Unlock()

	if rs == nil {
		return generateChatSummaryResponse{}, newAPIErr(http.StatusNotFound, "run_not_found", "run not found")
	}
	if busy {
		return generateChatSummaryResponse{}, newAPIErr(http.StatusConflict, "chat_summary_run_busy", "cannot summarize while the chat is active")
	}

	// Manual trigger bypasses the idle wait and supersedes any pending timer.
	s.cancelChatSummary(runID)
	generated := s.recordChatSummarySync(rs, turnID)

	resp := generateChatSummaryResponse{RunID: runID, Generated: generated, Skipped: !generated}
	if !generated {
		resp.Reason = "summary already current or no feature resolved"
	}
	return resp, nil
}

// ScanPersistedChatsForSummaries runs once at startup: it walks every persisted
// root chat run and generates the rolling summary for any whose stored summary is
// missing or stale (an up-to-date one is a no-op via the transcript-hash check).
// Best-effort, throttled, abortable via ctx, and skips runs already live
// in-memory (their idle timer / manual button owns them).
func (s *InteractiveService) ScanPersistedChatsForSummaries(ctx context.Context) {
	reader, ok := s.workflowStore.(SessionIndexReader)
	if !ok {
		return
	}
	sessions, err := reader.ListAllProviderSessions(ctx)
	if err != nil {
		return
	}
	for _, sess := range sessions {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if sess.RunKind != "chat" || sess.ParentRunID != "" || strings.TrimSpace(sess.RunID) == "" {
			continue
		}
		s.mu.Lock()
		_, live := s.runs[sess.RunID]
		s.mu.Unlock()
		if live {
			continue
		}
		rs, apiErr := s.loadPersistedRun(sess.RunID)
		if apiErr != nil || rs == nil {
			continue
		}
		s.seedTranscriptFromDisk(rs)
		job, ok := s.newChatSummaryJob(rs, rs.lastTurnID)
		if !ok {
			continue
		}
		s.runChatSummaryJob(job)
		time.Sleep(50 * time.Millisecond) // gentle at boot
	}
}

// recordChatSummarySync runs the recorder synchronously over a snapshot of the
// completed turn. Used by the manual-trigger endpoint and by tests that assert
// the ledger immediately; the idle timer and startup scan call the core directly.
func (s *InteractiveService) recordChatSummarySync(rs *interactiveRun, turnID string) bool {
	job, ok := s.newChatSummaryJob(rs, turnID)
	if !ok {
		return false
	}
	return s.runChatSummaryJob(job)
}

func (s *InteractiveService) newChatSummaryJob(rs *interactiveRun, turnID string) (chatSummaryJob, bool) {
	if rs == nil || rs.runKind != "chat" || rs.workspaceCwd == "" {
		return chatSummaryJob{}, false
	}
	turns := transcriptTurnsFromRun(rs)
	if len(turns) == 0 {
		return chatSummaryJob{}, false
	}
	return chatSummaryJob{
		runID:     rs.id,
		projectID: rs.projectID,
		cwd:       rs.workspaceCwd,
		turnID:    turnID,
		provider:  rs.providerKey,
		turns:     turns,
	}, true
}

// runChatSummaryJob produces the rolling summary for the conversation's current
// feature and upserts it (one line per run+feature). It returns true when a new
// summary was written, false when nothing changed (hash match) or no feature
// resolved — so the manual-trigger endpoint can report generated vs. skipped.
func (s *InteractiveService) runChatSummaryJob(job chatSummaryJob) bool {
	dotFP := filepath.Join(job.cwd, ".flowpilot")
	catalog, err := featurecatalog.LoadCatalog(dotFP)
	if err != nil {
		return false
	}
	top, ok := resolveTurnsFeature(job.turns, catalog)
	if !ok {
		return false
	}

	// Summarize only the turns belonging to this feature (no cross-feature
	// mixing when one chat spans two features).
	featureTurns := bucketTurnsByFeature(job.turns, catalog)[top.Key]
	if len(featureTurns) == 0 {
		featureTurns = job.turns
	}

	ledger, err := changeledger.NewChatSummaryLedger(dotFP)
	if err != nil {
		return false
	}

	// Cache by transcript state: when the stored summary for this run+feature
	// already reflects the current feature turns, skip the model call entirely.
	stateKey := transcriptStateKey(job.runID, featureTurns)
	if existing, err := ledger.GetFeatureSummariesForRun(top.Key, job.runID); err == nil && len(existing) > 0 {
		if existing[len(existing)-1].StateKey == stateKey {
			return false
		}
	}

	summary := s.summarizeChatTurns(job.provider, job.cwd, featureTurns)
	if strings.TrimSpace(summary) == "" {
		return false
	}

	entry := changeledger.ChatSummaryEntry{
		RunID:      job.runID,
		TurnID:     job.turnID,
		FeatureKey: top.Key,
		StateKey:   stateKey,
		Summary:    summary,
		CreatedAt:  time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := ledger.UpsertForRun(entry); err != nil {
		return false
	}
	_, _ = s.syncContextEngineFilesBestEffort(job.projectID, dotFP)
	return true
}

// summarizeChatTurns produces a discussion summary for a chat. It prefers a
// cheap-tier model of the chat's own provider (Task-161 T-2 / Task-162 T-4) and
// falls back to a deterministic heuristic when no runner/account is available or
// the model call fails, so summarization is always best-effort and non-fatal.
func (s *InteractiveService) summarizeChatTurns(provider ProviderKey, cwd string, turns []transcriptTurn) string {
	if s.runner != nil {
		if out, err := s.runner.SummarizeChatTranscript(context.Background(), renderTranscriptForSummary(turns), cwd, provider); err == nil {
			if summary := normalizeSummaryBullets(out); summary != "" {
				return summary
			}
		}
	}
	return heuristicSummarizeTurns(turns)
}

// renderTranscriptForSummary flattens ordered turns into plain User/Assistant
// lines for the summarizer model input.
func renderTranscriptForSummary(turns []transcriptTurn) string {
	var sb strings.Builder
	for _, turn := range turns {
		if u := strings.TrimSpace(turn.User); u != "" {
			sb.WriteString("User: ")
			sb.WriteString(u)
			sb.WriteString("\n")
		}
		if a := strings.TrimSpace(turn.Assistant); a != "" {
			sb.WriteString("Assistant: ")
			sb.WriteString(a)
			sb.WriteString("\n")
		}
	}
	return strings.TrimSpace(sb.String())
}

// normalizeSummaryBullets cleans a model summary into bounded, single-line
// bullet points that are safe to embed in a prompt block.
func normalizeSummaryBullets(out string) string {
	out = strings.TrimSpace(out)
	if out == "" {
		return ""
	}
	var lines []string
	for _, raw := range strings.Split(out, "\n") {
		bullet := compactTranscriptText(raw, 300)
		if bullet == "" {
			continue
		}
		bullet = strings.TrimLeft(bullet, "-*• ")
		if bullet == "" {
			continue
		}
		lines = append(lines, "- "+bullet)
		if len(lines) >= 5 {
			break
		}
	}
	summary := strings.Join(lines, "\n")
	summary, _ = promptblock.TruncateUTF8(summary, chatSummaryMaxBytes)
	return summary
}

// transcriptStateKey fingerprints a run's whole transcript as runID + a SHA-256
// of the concatenated turn text. Hashing keeps the key O(1) in size (instead of
// embedding the entire conversation on every chat_summary line) while still
// changing whenever any turn's content changes — so it dedups an unchanged
// transcript and refreshes on a new/edited turn.
func transcriptStateKey(runID string, turns []transcriptTurn) string {
	sum := sha256.Sum256([]byte(latestTranscriptState(turns)))
	return runID + ":" + hex.EncodeToString(sum[:])
}

func latestTranscriptState(turns []transcriptTurn) string {
	var parts []string
	for _, turn := range turns {
		parts = append(parts, strings.TrimSpace(turn.User)+"\n"+strings.TrimSpace(turn.Assistant))
	}
	return strings.Join(parts, "\n---\n")
}

// heuristicSummarizeTurns is the deterministic fallback summarizer used when the
// cheap model is unavailable; it keeps summarization non-fatal and offline-safe.
func heuristicSummarizeTurns(turns []transcriptTurn) string {
	if len(turns) == 0 {
		return ""
	}
	var bullets []string

	if first := firstNonEmptyUserTurn(turns); first != "" {
		bullets = append(bullets, "Goal: "+first)
	}

	for _, turn := range turns {
		for _, sentence := range summarizeSentences(turn) {
			if sentence == "" {
				continue
			}
			bullets = append(bullets, sentence)
			if len(bullets) >= 5 {
				goto done
			}
		}
	}

done:
	if len(bullets) == 0 {
		return ""
	}
	var lines []string
	for _, bullet := range bullets {
		bullet = strings.TrimSpace(bullet)
		if bullet == "" {
			continue
		}
		if !strings.HasSuffix(bullet, ".") {
			bullet += "."
		}
		lines = append(lines, "- "+bullet)
	}
	summary := strings.Join(lines, "\n")
	summary, _ = promptblock.TruncateUTF8(summary, chatSummaryMaxBytes)
	return summary
}

func firstNonEmptyUserTurn(turns []transcriptTurn) string {
	for _, turn := range turns {
		if text := compactTranscriptText(turn.User, 180); text != "" {
			return text
		}
	}
	return ""
}

func summarizeSentences(turn transcriptTurn) []string {
	text := strings.TrimSpace(turn.Assistant)
	if text == "" {
		text = strings.TrimSpace(turn.User)
	}
	if text == "" {
		return nil
	}

	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "blocked") || strings.Contains(lower, "can't") || strings.Contains(lower, "cannot"):
		return []string{"Blocked: " + compactTranscriptText(text, 220)}
	case strings.Contains(lower, "prefer") || strings.Contains(lower, "should") || strings.Contains(lower, "want") || strings.Contains(lower, "need"):
		return []string{"Preference/decision: " + compactTranscriptText(text, 220)}
	case strings.Contains(lower, "decided") || strings.Contains(lower, "use") || strings.Contains(lower, "keep") || strings.Contains(lower, "switch"):
		return []string{"Decision: " + compactTranscriptText(text, 220)}
	case strings.Contains(text, "?"):
		return []string{"Open question: " + compactTranscriptText(text, 220)}
	default:
		return []string{compactTranscriptText(text, 180)}
	}
}

func compactTranscriptText(text string, maxBytes int) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.Join(strings.Fields(text), " ")
	text = promptblock.EscapeClosingTag(text, "previous_conversation")
	text, _ = promptblock.TruncateUTF8(text, maxBytes)
	return text
}
