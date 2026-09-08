package app

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/prefs"
)

// persistTUISessionPrefs writes latest provider/model/yolo/mode/flow so new TUI sessions keep them.
// yolo is the chat-mode toggle only (flow uses auto-on via effectiveYolo; we still
// persist the underlying chat preference so /chat after restart keeps it).
func persistTUISessionPrefs(provider, model, reasoning string, yolo bool, mode Mode, launch LaunchArm, workingMode string) {
	yoloCopy := yolo
	s := prefs.Session{
		Provider:        strings.TrimSpace(provider),
		Model:           strings.TrimSpace(model),
		ReasoningEffort: strings.TrimSpace(reasoning),
		Yolo:            &yoloCopy,
		WorkingMode:     strings.TrimSpace(workingMode),
		Mode:            mode.String(),
	}
	if mode == ModeFlow || mode == ModeStep {
		s.FlowRef = strings.TrimSpace(launch.FlowRef)
		s.WorkflowID = strings.TrimSpace(launch.WorkflowID)
		s.FlowLabel = strings.TrimSpace(launch.Label)
	}
	_, _ = prefs.Save(s)
}

func (m *AppModel) persistSessionPrefs() {
	// Always store m.yolo (chat preference), never effectiveYolo() auto-on.
	persistTUISessionPrefs(m.provider, m.model, m.reasoningEffort, m.yolo, m.mode, m.launch, m.workingMode)
}

// bindProjectIfPossible tries to bind a project from the known catalog using
// the configured project path. Returns true when a project is bound.
func (m *AppModel) bindProjectIfPossible() bool {
	if m.project != nil {
		return true
	}
	if len(m.projects) == 0 {
		return false
	}
	if p := matchProjectByPath(m.projects, m.cfg.ProjectPath); p != nil {
		m.project = p
		return true
	}
	return false
}

// applySavedModeAndFlow restores Mode + LaunchArm from disk prefs (best-effort).
// Catalog fields are refined when flow lists arrive via refineLaunchFromCatalog.
func applySavedModeAndFlow(m *AppModel, saved prefs.Session) {
	mode := strings.ToLower(strings.TrimSpace(saved.Mode))
	switch mode {
	case "flow":
		m.mode = ModeFlow
	case "step":
		m.mode = ModeStep
	case "chat", "":
		m.mode = ModeChat
		m.launch = LaunchArm{}
		m.firstTurnPending = false
		return
	default:
		return
	}
	flowRef := strings.TrimSpace(saved.FlowRef)
	workflowID := strings.TrimSpace(saved.WorkflowID)
	label := strings.TrimSpace(saved.FlowLabel)
	if flowRef == "" && workflowID == "" && label == "" {
		m.launch = LaunchArm{Mode: m.mode}
		m.firstTurnPending = false
		return
	}
	arm := LaunchArm{
		Mode:       m.mode,
		FlowRef:    flowRef,
		WorkflowID: workflowID,
		Label:      label,
	}
	if arm.IsBuiltin() {
		arm.SubMode = "bug"
		arm.ChangeType = "bugfix"
		m.firstTurnPending = true
	} else {
		m.firstTurnPending = false
	}
	if arm.Label == "" {
		if arm.FlowRef != "" {
			arm.Label = arm.FlowRef
		} else {
			arm.Label = arm.WorkflowID
		}
	}
	m.launch = arm
}

// stashPendingFlowRestore keeps last flow prefs without arming ModeFlow yet.
// Cold-start restore of flow mode (before project_id) made the TUI feel frozen.
func stashPendingFlowRestore(m *AppModel, saved prefs.Session) {
	mode := strings.ToLower(strings.TrimSpace(saved.Mode))
	if mode != "flow" && mode != "step" {
		m.pendingFlowRestore = nil
		return
	}
	cp := saved
	m.pendingFlowRestore = &cp
	// Stay in chat until project catalog binds.
	m.mode = ModeChat
	m.launch = LaunchArm{}
	m.firstTurnPending = false
}

// tryApplyPendingFlowRestore arms saved flow mode only when a project is bound.
// Returns a short system notice when restore happens or when it is abandoned.
func (m *AppModel) tryApplyPendingFlowRestore() string {
	if m.pendingFlowRestore == nil {
		return ""
	}
	if m.project == nil {
		return ""
	}
	saved := *m.pendingFlowRestore
	m.pendingFlowRestore = nil
	applySavedModeAndFlow(m, saved)
	if m.mode != ModeFlow && m.mode != ModeStep {
		return ""
	}
	m.refineLaunchFromCatalog()
	m.persistSessionPrefs()
	label := m.launch.StatusLabel()
	if label == "" {
		label = "flow"
	}
	return fmt.Sprintf("Restored flow mode: %s (from last session).", label)
}

// pendingFlowRestoreHint is shown when catalog never bound a project.
func (m *AppModel) pendingFlowRestoreHint() string {
	if m.pendingFlowRestore == nil {
		return ""
	}
	s := m.pendingFlowRestore
	name := strings.TrimSpace(s.FlowLabel)
	if name == "" {
		name = strings.TrimSpace(s.FlowRef)
	}
	if name == "" {
		name = strings.TrimSpace(s.WorkflowID)
	}
	if name == "" {
		name = "last flow"
	}
	return fmt.Sprintf(
		"Last session was flow mode (%s) — not restored until project catalog loads.\n"+
			"Stay in chat, or after project binds re-open / use /flow %s",
		name, name,
	)
}

// refineLaunchFromCatalog re-resolves a prefs-restored arm against live builtins/workflows.
func (m *AppModel) refineLaunchFromCatalog() {
	if m.mode != ModeFlow && m.mode != ModeStep {
		return
	}
	if !m.launch.IsArmed() && strings.TrimSpace(m.launch.Label) == "" {
		return
	}
	// Do not re-arm over an active run's chrome.
	if m.runHandle != nil {
		if n := m.resolveFlowDisplayName(m.launch.WorkflowID, m.launch.FlowRef); n != "" {
			m.launch.Label = n
		}
		return
	}
	projectID := ""
	if m.project != nil {
		projectID = m.project.ID
	}
	queries := []string{
		strings.TrimSpace(m.launch.WorkflowID),
		strings.TrimSpace(m.launch.FlowRef),
		strings.TrimSpace(m.launch.Label),
	}
	for _, q := range queries {
		if q == "" {
			continue
		}
		arm, err := resolveFlowLaunch(m.flowBuiltins, m.flowWorkflows, projectID, q)
		if err != nil {
			continue
		}
		if m.mode == ModeStep {
			arm.Mode = ModeStep
		}
		m.launch = arm
		m.firstTurnPending = arm.IsBuiltin()
		return
	}
	if n := m.resolveFlowDisplayName(m.launch.WorkflowID, m.launch.FlowRef); n != "" {
		m.launch.Label = n
	}
}

// parsedGate is a lightweight gate descriptor for tests and handlers.
type parsedGate struct {
	Kind       string
	ApprovalID string
	QuestionID string
	Options    []string
}

func (m *AppModel) buildTurnInput(prompt string) client.TurnInput {
	in := client.TurnInput{
		RunID:           "",
		Prompt:          prompt,
		ReasoningEffort: m.reasoningEffort,
		SelectedSkills:  append([]client.SkillSelection(nil), m.selectedSkills...),
		Attachments:     append([]client.PromptAttachment(nil), m.pendingAttach...),
		ChatPosture:     m.activePosture(),
	}
	if m.runHandle != nil {
		in.RunID = m.runHandle.RunID
	}
	yolo := m.yolo
	in.YoloMode = &yolo
	if m.model != "" {
		model := m.model
		in.Model = &model
	}
	// Builtin orchestration extras only — never send catalog UUID as flowRef.
	if m.firstTurnPending {
		if sub, fr, ct, _, ok := m.launch.FirstTurnExtras(); ok {
			in.SubMode = sub
			in.FlowRef = fr
			in.ChangeType = ct
		}
	}
	return in
}

func (m *AppModel) clearPendingTurnPayload() {
	m.selectedSkills = nil
	// Drop pending images + temp files (do not leave orphans under flowpilot-tui-pending).
	_ = m.clearPendingAttachments()
}

// resolveTurnStepID returns the step id a new turn must send to startTurn. It
// mirrors Desktop store.sendPrompt (store.ts): normal chat reuses the synthetic
// "chat-<runId>" step minted by the runner (surfaced via RunHandle.StepID) so
// follow-ups never POST an empty stepId (which startTurn rejects with 400
// "stepId is required"); workflow/flow-mode runs fall back to the launch
// workflow id so a resumed catalog flow can still continue (CA-519).
func (m *AppModel) resolveTurnStepID() string {
	if id := strings.TrimSpace(m.stepID); id != "" {
		return id
	}
	if m.runHandle != nil {
		if id := strings.TrimSpace(m.runHandle.StepID); id != "" {
			return id
		}
	}
	if id := strings.TrimSpace(m.launch.StepID); id != "" {
		return id
	}
	if id := strings.TrimSpace(m.launch.WorkflowID); id != "" {
		return id
	}
	if m.runHandle != nil && m.runHandle.RunID != "" {
		return "chat-" + m.runHandle.RunID
	}
	return ""
}

func (m *AppModel) canSend() bool {
	if m.viewingChild() {
		return false
	}
	if !m.agentsFocus || len(m.agentRuns) == 0 {
		return true
	}
	if m.focusedAgentIdx <= 0 {
		return true
	}
	agent := m.agentRuns[m.focusedAgentIdx]
	if strings.EqualFold(agent.Role, "main") || agent.AgentName == "main" {
		return true
	}
	return false
}

// isStepCompleteStub detects synthetic finalMessage stubs from the demo/fake
// adapter (and similar step-complete closers). Desktop keeps the streamed
// deltas as the transcript and does not replace them with this text.
func isStepCompleteStub(s string) bool {
	lower := strings.ToLower(strings.TrimSpace(s))
	if lower == "" {
		return false
	}
	if lower == "turn completed" || strings.TrimSuffix(lower, ".") == "turn completed" {
		return true
	}
	return strings.Contains(lower, "the change is implemented") &&
		strings.Contains(lower, "step is complete")
}

// chooseAssistantFinal prefers streamed message_delta text over turn_completed
// finalMessage (desktop timeline parity).
func chooseAssistantFinal(deltas, finalMessage string) string {
	d := strings.TrimSpace(deltas)
	f := strings.TrimSpace(finalMessage)
	if d != "" {
		return d
	}
	if f != "" && !isStepCompleteStub(f) {
		return f
	}
	// Stub-only replies still surface something rather than a blank bubble.
	return f
}

func formatUsageLine(lastTokens, contextWindow, lastTurn int64) string {
	if contextWindow <= 0 {
		return fmt.Sprintf("last:%dk", lastTurn/1000)
	}
	return fmt.Sprintf("ctx:%dk/%dk last:%dk", lastTokens/1000, contextWindow/1000, lastTurn/1000)
}

// orderAgentsMainFirst keeps main first, then sorts the remaining child agents
// STABLY by AgentName then RunID. A stable sort prevents the /agents list and the
// Tab/picker from flickering/reordering every time a fresh agent_graph_updated
// snapshot replaces agentRuns (the SSE arrival order differs from poll order).
func orderAgentsMainFirst(runs []client.AgentRunSummary) []client.AgentRunSummary {
	if len(runs) == 0 {
		return runs
	}
	var main, rest []client.AgentRunSummary
	for _, r := range runs {
		if strings.EqualFold(r.Role, "main") || strings.EqualFold(r.AgentName, "main") {
			main = append(main, r)
		} else {
			rest = append(rest, r)
		}
	}
	sort.SliceStable(rest, func(i, j int) bool {
		ni, nj := strings.ToLower(strings.TrimSpace(rest[i].AgentName)), strings.ToLower(strings.TrimSpace(rest[j].AgentName))
		if ni != nj {
			return ni < nj
		}
		return strings.TrimSpace(rest[i].RunID) < strings.TrimSpace(rest[j].RunID)
	})
	out := append([]client.AgentRunSummary(nil), main...)
	out = append(out, rest...)
	if len(out) == 0 {
		return runs
	}
	return out
}

// pickActiveSessionDefaults chooses provider/model for the statusline.
// Explicit flag overrides win. Otherwise prefer the first active account,
// then the first listed provider; model comes from that provider's first model.
func pickActiveSessionDefaults(
	flagProvider, flagModel string,
	accounts []client.ProviderAccountSummary,
	providers []client.Provider,
) (provider, model, accountLabel string) {
	provider = strings.TrimSpace(flagProvider)
	model = strings.TrimSpace(flagModel)

	if provider == "" {
		for _, a := range accounts {
			if a.IsActive {
				provider = a.ProviderKey
				accountLabel = a.DisplayLabel
				break
			}
		}
	}
	if provider == "" && len(providers) > 0 {
		provider = providers[0].Key
	}
	if accountLabel == "" {
		for _, a := range accounts {
			if strings.EqualFold(a.ProviderKey, provider) && a.IsActive {
				accountLabel = a.DisplayLabel
				break
			}
		}
	}
	if model == "" && provider != "" {
		for _, p := range providers {
			if !strings.EqualFold(p.Key, provider) {
				continue
			}
			for _, m := range p.Models {
				id := m.ModelID()
				if id == "" {
					continue
				}
				if !m.Available && m.ID != "" {
					// Prefer available models when the flag is populated; still
					// accept the first id when Available is unset/false for all.
					continue
				}
				model = id
				break
			}
			if model == "" && len(p.Models) > 0 {
				model = p.Models[0].ModelID()
			}
			break
		}
	}
	return provider, model, accountLabel
}

// normalizePathKey makes OS paths comparable across Windows/macOS/Linux.
func normalizePathKey(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	if eval, err := filepath.EvalSymlinks(p); err == nil {
		p = eval
	}
	p = filepath.Clean(p)
	p = strings.ReplaceAll(p, `\`, `/`)
	return strings.ToLower(strings.TrimRight(p, "/"))
}

// matchProjectByPath finds a catalog project matching target path.
// Resolution order (TUI side only):
// 1. Exact normalised path match (projects[i].Path == target)
// 2. Folder basename match (filepath.Base(projects[i].Path) == target folder name)
// 3. Project Name match (projects[i].Name == target folder name, e.g. "Gate-sandbox")
func matchProjectByPath(projects []client.Project, target string) *client.Project {
	want := normalizePathKey(target)
	if want == "" {
		return nil
	}
	// Pass 1: exact normalised path match.
	for i := range projects {
		if normalizePathKey(projects[i].Path) == want {
			return &projects[i]
		}
	}
	base := strings.ToLower(filepath.Base(strings.ReplaceAll(target, `\`, `/`)))
	if base == "" || base == "." || base == "/" {
		return nil
	}
	// Pass 2: unique folder basename match.
	var hits []*client.Project
	for i := range projects {
		gotBase := strings.ToLower(filepath.Base(strings.ReplaceAll(projects[i].Path, `\`, `/`)))
		if gotBase == base {
			hits = append(hits, &projects[i])
		}
	}
	if len(hits) == 1 {
		return hits[0]
	}
	// Pass 3: unique project name match (e.g. project named "Gate-sandbox" or "Gate Sandbox").
	cleanBase := strings.ReplaceAll(strings.ReplaceAll(base, "-", ""), "_", "")
	hits = nil
	for i := range projects {
		nameClean := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(projects[i].Name, "-", ""), "_", ""), " ", ""))
		if nameClean == cleanBase {
			hits = append(hits, &projects[i])
		}
	}
	if len(hits) == 1 {
		return hits[0]
	}
	return nil
}

func formatAccountLimits(acc *client.ProviderAccountSummary) string {
	if acc == nil {
		return ""
	}
	var parts []string
	if acc.Remaining5hPercent != nil {
		parts = append(parts, formatQuotaChip("5h", *acc.Remaining5hPercent, acc.Remaining5hResetAt))
	}
	if acc.Remaining7dPercent != nil {
		parts = append(parts, formatQuotaChip("7d", *acc.Remaining7dPercent, acc.Remaining7dResetAt))
	}
	if len(parts) == 0 {
		for _, line := range acc.UsageDetailLines {
			label := strings.TrimSpace(line.Label)
			if label == "" {
				continue
			}
			reset := strings.TrimSpace(line.ResetAt)
			var resetPtr *string
			if reset != "" {
				resetPtr = &reset
			}
			parts = append(parts, formatQuotaChip(label, line.RemainingPercent, resetPtr))
		}
	}
	if len(parts) == 0 && acc.UsageSummary != nil {
		s := strings.TrimSpace(*acc.UsageSummary)
		if s != "" && !strings.HasPrefix(s, "Team ") && s != "Personal" {
			return s
		}
	}
	return strings.Join(parts, " ")
}

func formatQuotaChip(label string, pct int, resetAt *string) string {
	// CA-684: a usage line with no real percent and no reset is informational
	// (opencode stats rows — sessions/cost/tokens), not a quota meter. Never
	// fabricate a ":0%" suffix onto it.
	if pct <= 0 && (resetAt == nil || strings.TrimSpace(*resetAt) == "") {
		return label
	}
	chip := fmt.Sprintf("%s:%d%%", label, pct)
	if resetAt == nil {
		return chip
	}
	if when := formatQuotaResetAt(*resetAt); when != "" {
		return chip + " · resets " + when
	}
	return chip
}

func formatQuotaResetAt(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		parsed, err = time.Parse(time.RFC3339Nano, raw)
	}
	if err != nil {
		return ""
	}
	return parsed.Local().Format("Jan 2, 15:04")
}

func formatHistoryChangedAt(it client.RunHistoryItem) string {
	if when := formatQuotaResetAt(it.UpdatedAt); when != "" {
		return when
	}
	return formatQuotaResetAt(it.StartedAt)
}

func formatContextLimits(usage *client.TokenUsageSnapshot, fallbackWindow int64) string {
	if usage == nil {
		if fallbackWindow > 0 {
			return fmt.Sprintf("ctx %s window", formatTokenCount(fallbackWindow))
		}
		return ""
	}
	var window int64
	if usage.ModelContextWindow != nil && *usage.ModelContextWindow > 0 {
		window = *usage.ModelContextWindow
	} else if fallbackWindow > 0 {
		window = fallbackWindow
	}
	var used int64
	if usage.Total != nil && usage.Total.TotalTokens > 0 {
		used = usage.Total.TotalTokens
	} else if usage.Last != nil {
		used = usage.Last.TotalTokens
	}
	if window <= 0 && (usage.Last == nil || usage.Last.TotalTokens <= 0) {
		if usage.Total != nil && usage.Total.TotalTokens > 0 {
			return fmt.Sprintf("total %s", formatTokenCount(usage.Total.TotalTokens))
		}
		return ""
	}
	if window > 0 {
		left := window - used
		if left < 0 {
			left = 0
		}
		remainPct := contextRemainingPercent(used, window)
		line := fmt.Sprintf("ctx %d%% remain · %s/%s (%s left)", remainPct, formatTokenCount(used), formatTokenCount(window), formatTokenCount(left))
		if inTok := contextInputTokens(usage); inTok > 0 {
			line += fmt.Sprintf(" in:%s", formatTokenCount(inTok))
		}
		if usage.Last != nil && usage.Last.OutputTokens > 0 {
			line += fmt.Sprintf(" out:%s", formatTokenCount(usage.Last.OutputTokens))
		}
		if usage.Last != nil && usage.Last.TotalTokens > 0 {
			line += fmt.Sprintf(" · last %s", formatTokenCount(usage.Last.TotalTokens))
		}
		return line
	}
	if usage.Last != nil {
		line := fmt.Sprintf("last %s", formatTokenCount(usage.Last.TotalTokens))
		if inTok := contextInputTokens(usage); inTok > 0 {
			line += fmt.Sprintf(" in:%s", formatTokenCount(inTok))
		}
		if usage.Last.OutputTokens > 0 {
			line += fmt.Sprintf(" out:%s", formatTokenCount(usage.Last.OutputTokens))
		}
		return line
	}
	return ""
}

func formatTokenCount(n int64) string {
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}

func contextRemainingPercent(used, window int64) int {
	if window <= 0 {
		return 0
	}
	left := window - used
	if left < 0 {
		left = 0
	}
	return int(math.Round(float64(left) * 100 / float64(window)))
}

func contextInputTokens(usage *client.TokenUsageSnapshot) int64 {
	if usage == nil {
		return 0
	}
	if usage.Last != nil && usage.Last.InputTokens > 0 {
		return usage.Last.InputTokens
	}
	if usage.Total != nil {
		return usage.Total.InputTokens
	}
	return 0
}

func contextWindowForModel(providers []client.Provider, providerKey, modelID string) int64 {
	for _, p := range providers {
		if !strings.EqualFold(p.Key, providerKey) {
			continue
		}
		for _, m := range p.Models {
			if strings.EqualFold(m.ModelID(), modelID) {
				return m.ContextWindowTokens
			}
		}
	}
	return 0
}

// wrapText wraps s to width columns, preserving existing newlines (LF, CRLF, bare CR).
// Long tokens without spaces are hard-broken so nothing is truncated off-screen.
func wrapText(s string, width int) []string {
	if width < 8 {
		width = 8
	}
	if s == "" {
		return []string{""}
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	var out []string
	for _, para := range strings.Split(s, "\n") {
		out = append(out, wrapParagraph(para, width)...)
	}
	return out
}

func wrapParagraph(para string, width int) []string {
	if width < 1 {
		width = 1
	}
	if para == "" {
		return []string{""}
	}
	if lipgloss.Width(para) <= width {
		return []string{para}
	}
	runes := []rune(para)
	var lines []string
	for len(runes) > 0 {
		if lipgloss.Width(string(runes)) <= width {
			lines = append(lines, string(runes))
			break
		}
		cut := width
		if cut > len(runes) {
			cut = len(runes)
		}
		// Prefer breaking on whitespace in the right half of the window.
		for i := width; i > width/2 && i < len(runes); i-- {
			if runes[i] == ' ' || runes[i] == '\t' {
				cut = i
				break
			}
		}
		if cut <= 0 {
			cut = width
			if cut > len(runes) {
				cut = len(runes)
			}
		}
		line := strings.TrimRight(string(runes[:cut]), " \t")
		if lipgloss.Width(line) > width {
			line = truncateVisual(line, width)
		}
		lines = append(lines, line)
		runes = runes[cut:]
		for len(runes) > 0 && (runes[0] == ' ' || runes[0] == '\t') {
			runes = runes[1:]
		}
	}
	return lines
}

// isSlashBoundary is a word edge that can start a slash command (Desktop findActiveSlash).
func isSlashBoundary(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n'
}

// activeSlashLine finds the nearest word-boundary '/' at or before caret.
// Returns the suffix from that '/' through the end of input (args included).
// Word boundary: start of input, or previous rune is space/tab/newline.
// Spaces after '/' are command args — do not treat them as "no slash".
// Glued slashes (https://, path/to) are skipped so an earlier " /cmd" still wins.
func activeSlashLine(input string, caret int) (line string, start int, ok bool) {
	runes := []rune(input)
	n := len(runes)
	if caret < 0 {
		caret = 0
	}
	if caret > n {
		caret = n
	}
	for i := caret - 1; i >= 0; i-- {
		if runes[i] != '/' {
			continue
		}
		if i == 0 || isSlashBoundary(runes[i-1]) {
			return string(runes[i:]), i, true
		}
	}
	return "", 0, false
}

// slashSuggestLine is the command token used by pickers. A mid-draft
// "hello /mo" yields "/mo" so the list opens without deleting the draft first.
// Falls back to the full input so "/cmd" still matches when the caret sits
// before the slash (Home) or tests only set inputValue.
func (m *AppModel) slashSuggestLine() string {
	line, _, ok := activeSlashLine(m.inputValue, m.inputCaretIndex())
	if ok {
		return line
	}
	return m.inputValue
}

// stripActiveSlashCommand removes the active word-boundary slash token (from
// '/' through end of input) so closing /skill keeps draft text typed before it.
// "abc [coding] /skill " → "abc [coding] " (trailing space so "def" → "abc [coding] def").
// Bare "/skill " → "".
func stripActiveSlashCommand(input string, caret int) string {
	_, start, ok := activeSlashLine(input, caret)
	if !ok {
		return input
	}
	runes := []rune(input)
	if start <= 0 {
		return ""
	}
	before := strings.TrimRight(string(runes[:start]), " \t")
	if before == "" {
		return ""
	}
	return before + " "
}

// replaceActiveSlashWith keeps text before the active word-boundary '/' and
// writes replacement as the slash command portion. Used by Tab completion so
// "abc /sk" → "abc /skill " instead of wiping the draft.
// No active slash → replacement alone (bare "/pro" Tab).
func replaceActiveSlashWith(input string, caret int, replacement string) string {
	_, start, ok := activeSlashLine(input, caret)
	if !ok {
		return replacement
	}
	runes := []rune(input)
	return string(runes[:start]) + replacement
}

// filterSlashSuggestions returns slash commands matching the current input prefix.
func filterSlashSuggestions(input string) []slashCommand {
	in := strings.ToLower(strings.TrimSpace(input))
	if in == "" || !strings.HasPrefix(in, "/") {
		return nil
	}
	// Only suggest while typing the command token (no args yet).
	if strings.Contains(strings.TrimSpace(input), " ") {
		return nil
	}
	out := make([]slashCommand, 0, len(knownSlashCommands))
	for _, sc := range knownSlashCommands {
		if strings.HasPrefix(sc.name, in) {
			out = append(out, sc)
		}
	}
	return out
}

// parseSlashArgPrefix reports whether input is `<cmd> <query…>` (space after cmd).
func parseSlashArgPrefix(input, cmd string) (ok bool, query string) {
	s := strings.TrimLeft(input, " \t")
	prefix := strings.TrimSpace(cmd)
	if prefix == "" || len(s) < len(prefix) || !strings.EqualFold(s[:len(prefix)], prefix) {
		return false, ""
	}
	rest := s[len(prefix):]
	if rest == "" {
		return false, ""
	}
	if rest[0] != ' ' && rest[0] != '\t' {
		return false, "" // e.g. /flower vs /flow
	}
	return true, strings.TrimSpace(rest)
}

// parseFlowArgPrefix reports whether input is `/flow <query…>` (space after /flow).
func parseFlowArgPrefix(input string) (ok bool, query string) {
	return parseSlashArgPrefix(input, "/flow")
}

// chatOpenSlashCommands are the three ways to open an existing chat.
var chatOpenSlashCommands = []string{"/history", "/open", "/resume"}

// parseChatOpenArgPrefix reports `/history|open|resume <query…>` (space after cmd).
func parseChatOpenArgPrefix(input string) (cmd string, query string, ok bool) {
	s := strings.TrimLeft(input, " \t")
	for _, prefix := range chatOpenSlashCommands {
		if len(s) < len(prefix) || !strings.EqualFold(s[:len(prefix)], prefix) {
			continue
		}
		rest := s[len(prefix):]
		if rest == "" {
			return "", "", false
		}
		if rest[0] != ' ' && rest[0] != '\t' {
			return "", "", false
		}
		return prefix, strings.TrimSpace(rest), true
	}
	return "", "", false
}

// parseHistoryArgPrefix is kept for call sites that only care about /history.
func parseHistoryArgPrefix(input string) (ok bool, query string) {
	cmd, query, ok := parseChatOpenArgPrefix(input)
	if !ok || !strings.EqualFold(cmd, "/history") {
		return false, ""
	}
	return true, query
}

// parseDeleteArgPrefix reports whether input is `/delete <query…>` (space after /delete).
func parseDeleteArgPrefix(input string) (ok bool, query string) {
	return parseSlashArgPrefix(input, "/delete")
}

// CA-685: full canonical reasoning vocabulary — the runner mappers accept all
// of these (opencode --variant minimal..xhigh/max, see opencodeReasoningVariantID).
// When the selected model carries catalog efforts (Task-215), the picker shows
// those instead — see AppModel.modelReasoningEfforts.
var reasoningEffortOptions = []string{"minimal", "low", "medium", "high", "xhigh", "max"}

// modelReasoningEfforts returns the selected model's detected reasoning efforts
// from the provider catalog (empty when unknown). The current provider is
// searched first; if the model id is not found there (stale catalog shard,
// provider rename), any provider owning that exact model id answers.
func (m *AppModel) modelReasoningEfforts() []string {
	modelID := strings.TrimSpace(m.model)
	if modelID == "" {
		return nil
	}
	var crossProvider []string
	for _, p := range m.providers {
		foundHere := false
		for _, mod := range p.Models {
			if !strings.EqualFold(mod.ModelID(), modelID) {
				continue
			}
			foundHere = true
			if len(mod.SupportedReasoningEfforts) > 0 && len(crossProvider) == 0 {
				crossProvider = mod.SupportedReasoningEfforts
			}
		}
		if foundHere && strings.EqualFold(p.Key, m.provider) && len(crossProvider) > 0 {
			return crossProvider
		}
	}
	return crossProvider
}

// reasoningEffortDetail is the grok-CLI-style human label + description for a
// reasoning effort (shown in the /reasoning picker's right column).
func reasoningEffortDetail(effort string) string {
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "minimal":
		return "Minimal effort — fastest, minimal reasoning"
	case "low":
		return "Low effort — quick, fast implementations"
	case "medium":
		return "Medium effort — balanced implementation and testing"
	case "high":
		return "High effort — higher quality with extensive reasoning"
	case "xhigh":
		return "Extra high effort — highest effort and reasoning level"
	case "max":
		return "Max effort — maximum reasoning level"
	case "":
		return "Model default effort"
	default:
		return "Custom effort"
	}
}

// parseProviderPicker reports `/provider <query>` or `/provider connect|config|install|account|switch|activate <query>`.
func parseProviderPicker(input string) (mode, filter string, ok bool) {
	okPrefix, query := parseSlashArgPrefix(input, "/provider")
	if !okPrefix {
		return "", "", false
	}
	parts := strings.Fields(query)
	if len(parts) == 0 {
		return "select", "", true
	}
	switch strings.ToLower(parts[0]) {
	case "connect", "config":
		if len(parts) > 1 {
			return "connect", strings.Join(parts[1:], " "), true
		}
		return "connect", "", true
	case "install":
		if len(parts) > 1 {
			return "install", strings.Join(parts[1:], " "), true
		}
		return "install", "", true
	case "account", "switch", "activate", "acc":
		if len(parts) > 1 {
			return "account", strings.Join(parts[1:], " "), true
		}
		return "account", "", true
	default:
		return "select", query, true
	}
}

// providerReadiness mirrors Desktop ChatInput: ready only when CLI installed AND an
// active connected account exists for that provider key.
func providerReadiness(p client.Provider, accounts []client.ProviderAccountSummary) (code, detail string) {
	if !p.Installed || providerCLIUnusable(p) {
		return "not_installed", "not installed — /provider install " + strings.TrimSpace(p.Key)
	}
	hasAccount := false
	activeLabel := ""
	connectedCount := 0
	for _, a := range accounts {
		if !strings.EqualFold(a.ProviderKey, p.Key) {
			continue
		}
		hasAccount = true
		if accountAuthOK(a) {
			connectedCount++
			if a.IsActive {
				activeLabel = strings.TrimSpace(a.DisplayLabel)
			}
		}
	}
	if !hasAccount {
		return "no_account", "installed · no account — /provider connect " + strings.TrimSpace(p.Key)
	}
	if activeLabel == "" {
		return "no_active_account", "installed · no active account — /provider connect " + strings.TrimSpace(p.Key)
	}
	if connectedCount == 0 {
		return "no_active_account", "installed · no connected account — /provider connect " + strings.TrimSpace(p.Key)
	}
	if activeLabel != "" {
		return "ready", "ready · " + activeLabel
	}
	return "ready", "ready"
}

func providerCLIUnusable(p client.Provider) bool {
	if strings.EqualFold(strings.TrimSpace(p.InstallStatus), "FAILED") {
		return true
	}
	ver := strings.TrimSpace(p.DetectedVersion)
	if ver == "" {
		ver = strings.TrimSpace(p.Version)
	}
	return looksLikeProviderProbeError(ver)
}

func looksLikeProviderProbeError(s string) bool {
	low := strings.ToLower(strings.TrimSpace(s))
	if low == "" {
		return false
	}
	if strings.Contains(low, "cannot find module") || strings.Contains(low, "can not find module") {
		return true
	}
	if strings.Contains(low, "module_not_found") || strings.Contains(low, "module not found") {
		return true
	}
	if strings.HasPrefix(low, "error:") && (strings.Contains(low, "module") || strings.Contains(low, "enoent") || strings.Contains(low, "not found")) {
		return true
	}
	if strings.Count(s, "\n") >= 3 {
		return true
	}
	return false
}

func accountAuthOK(a client.ProviderAccountSummary) bool {
	s := strings.ToLower(strings.TrimSpace(a.AuthStatus))
	return s == "" || s == "connected" || s == "authenticated"
}

func providerSuggestionDetail(p client.Provider, accounts []client.ProviderAccountSummary, current, mode string) string {
	code, status := providerReadiness(p, accounts)
	label := strings.TrimSpace(p.Label)
	if label == "" {
		label = strings.TrimSpace(p.Name)
	}
	parts := make([]string, 0, 5)
	if strings.EqualFold(p.Key, current) {
		parts = append(parts, "current")
	}
	if label != "" && !strings.EqualFold(label, p.Key) {
		parts = append(parts, label)
	}
	switch mode {
	case "connect":
		parts = append(parts, "connect account")
	case "install":
		parts = append(parts, "install CLI")
	}
	parts = append(parts, status)
	if mode == "select" && code != "not_installed" {
		ver := strings.TrimSpace(p.DetectedVersion)
		if ver == "" {
			ver = strings.TrimSpace(p.Version)
		}
		if ver != "" && !looksLikeProviderProbeError(ver) {
			parts = append(parts, ver)
		}
		parts = append(parts, fmt.Sprintf("%d models", len(p.Models)))
	}
	if code == "not_installed" {
		// Keep "not installed" substring for existing picker tests; do not
		// append Node "Cannot find module" dumps as a fake version.
		parts[len(parts)-1] = "not installed"
	}
	return strings.Join(parts, " · ")
}

func formatAccountPathLabel(homePath string) string {
	path := strings.TrimSpace(homePath)
	if path == "" {
		return ""
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" && strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}
	return filepath.Base(path)
}

// providerImmediateAction reports provider subcommands that EXECUTE on Enter
// instead of expanding a next picker: /provider refresh re-fetches the
// provider catalog + models without a restart (CA-687); reload is its alias.
// connect/install/account/config still expand the next picker.
func providerImmediateAction(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "refresh", "reload":
		return true
	}
	return false
}

// filterProviderSuggestions returns providers matching `/provider ` / connect / install / account.
// In select mode, action rows are listed first so they are always discoverable.
func filterProviderSuggestions(input string, providers []client.Provider, accounts []client.ProviderAccountSummary, current string) []suggestItem {
	mode, query, ok := parseProviderPicker(input)
	if !ok {
		return nil
	}
	q := strings.ToLower(query)
	out := make([]suggestItem, 0, len(providers)+len(accounts)+4)
	if mode == "select" {
		for _, act := range []struct{ value, detail string }{
			{"account", "Switch active provider account (/provider account <id>)"},
			{"connect", "Connect new account (Desktop Settings parity)"},
			{"install", "Install provider CLI (runner /providers/install)"},
			{"config", "Alias for connect"},
			{"refresh", "Re-detect providers/models without restart (/provider reload alias)"},
		} {
			if q == "" || strings.HasPrefix(act.value, q) || strings.Contains(act.value, q) {
				out = append(out, suggestItem{value: act.value, detail: act.detail, kind: "provider-action"})
			}
		}
	}
	kind := "provider"
	switch mode {
	case "connect":
		kind = "provider-connect"
	case "install":
		kind = "provider-install"
	case "account":
		kind = "provider-account"
		for _, acc := range accounts {
			pathLabel := formatAccountPathLabel(acc.HomePath)
			hay := strings.ToLower(acc.ID + " " + acc.DisplayLabel + " " + acc.ProviderKey + " " + acc.HomePath + " " + pathLabel)
			if q != "" && !strings.Contains(hay, q) {
				continue
			}
			activeTag := ""
			if acc.IsActive {
				activeTag = " (active)"
			}
			pathStr := ""
			if pathLabel != "" {
				pathStr = fmt.Sprintf(" (%s)", pathLabel)
			}
			out = append(out, suggestItem{
				value:  acc.ID,
				detail: fmt.Sprintf("[%s] %s%s%s · %s", acc.ProviderKey, acc.DisplayLabel, pathStr, activeTag, acc.AuthStatus),
				kind:   "provider-account",
			})
		}
		return out
	}
	for _, p := range providers {
		key := strings.TrimSpace(p.Key)
		if key == "" {
			continue
		}
		label := strings.TrimSpace(p.Label)
		if label == "" {
			label = strings.TrimSpace(p.Name)
		}
		hay := strings.ToLower(key + " " + label)
		if q != "" && !strings.Contains(hay, q) {
			continue
		}
		out = append(out, suggestItem{
			value:  key,
			detail: providerSuggestionDetail(p, accounts, current, mode),
			kind:   kind,
		})
	}
	return out
}

// filterModelSuggestions returns models matching the query after `/model `.
func filterModelSuggestions(input string, models []string, current string) []suggestItem {
	ok, query := parseSlashArgPrefix(input, "/model")
	if !ok {
		return nil
	}
	q := strings.ToLower(query)
	out := make([]suggestItem, 0, len(models))
	for _, id := range models {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(id), q) {
			continue
		}
		detail := "model"
		if strings.EqualFold(id, current) {
			detail = "current"
		}
		out = append(out, suggestItem{value: id, detail: detail, kind: "model"})
	}
	return out
}

// filterReasoningSuggestions returns effort levels matching `/reasoning `.
func filterReasoningSuggestions(input string, current string, modelEfforts []string) []suggestItem {
	ok, query := parseSlashArgPrefix(input, "/reasoning")
	if !ok {
		return nil
	}
	// CA-686: when the selected model advertises efforts (Task-215 catalog),
	// that list IS the menu — Desktop parity. No catalog data falls back to the
	// canonical vocabulary.
	efforts := modelEfforts
	if len(efforts) == 0 {
		efforts = reasoningEffortOptions
	}
	q := strings.ToLower(query)
	out := make([]suggestItem, 0, len(efforts))
	for _, effort := range efforts {
		if q != "" && !strings.HasPrefix(effort, q) && !strings.Contains(effort, q) {
			continue
		}
		detail := reasoningEffortDetail(effort)
		if strings.EqualFold(effort, current) {
			detail += " — active"
		}
		out = append(out, suggestItem{value: effort, detail: detail, kind: "reasoning"})
	}
	return out
}

// reasoningEffortAllowed reports whether an effort may be set. Model-first
// (CA-686): when the selected model advertises efforts, ONLY those (plus
// empty = model default) are valid — claude has max but not xhigh, opencode
// has xhigh but not max. Without catalog data the canonical vocabulary applies.
func reasoningEffortAllowed(m *AppModel, effort string) bool {
	effort = strings.ToLower(strings.TrimSpace(effort))
	if effort == "" {
		return true
	}
	if efforts := m.modelReasoningEfforts(); len(efforts) > 0 {
		for _, e := range efforts {
			if strings.EqualFold(e, effort) {
				return true
			}
		}
		return false
	}
	for _, e := range reasoningEffortOptions {
		if e == effort {
			return true
		}
	}
	return false
}

// clampReasoningForCurrentModel (CA-686): reasoning is dynamic per model —
// after the model changes, a stale effort the new model does not advertise
// resets to that model's catalog default (or empty = model default). No-op
// when the catalog carries no effort data for the model.
func (m *AppModel) clampReasoningForCurrentModel() {
	efforts := m.modelReasoningEfforts()
	if len(efforts) == 0 {
		return
	}
	current := strings.ToLower(strings.TrimSpace(m.reasoningEffort))
	if current == "" {
		return
	}
	for _, e := range efforts {
		if strings.EqualFold(e, current) {
			return
		}
	}
	var def string
	for _, p := range m.providers {
		if !strings.EqualFold(p.Key, m.provider) {
			continue
		}
		for _, mod := range p.Models {
			if strings.EqualFold(mod.ModelID(), m.model) {
				def = strings.ToLower(strings.TrimSpace(mod.DefaultReasoningEffort))
			}
		}
	}
	for _, e := range efforts {
		if strings.EqualFold(e, def) {
			m.reasoningEffort = def
			return
		}
	}
	m.reasoningEffort = ""
}

// reasoningEffortHelp lists the efforts valid for the selected model.
func reasoningEffortHelp(m *AppModel) string {
	if efforts := m.modelReasoningEfforts(); len(efforts) > 0 {
		return "Options (" + m.model + "): " + strings.Join(efforts, " · ") + " · (empty = model default)"
	}
	return "Options: minimal · low · medium · high · xhigh · max · (empty = model default)"
}

// filterModeSetupSuggestions returns picker items for `/mode-setup …`
// (posture → field → value). Each stage filters by the trailing query so
// ↑↓ Tab never requires typing values by hand. Provider values fall back to
// providerAccounts and then to the known claude/codex/grok keys so the picker
// is never empty — pins are plain strings and do not require a CLI to be
// installed.
func filterModeSetupSuggestions(input string, providers []client.Provider, accounts []client.ProviderAccountSummary, currentProvider, currentModel string, draft *client.ChatPostureConfig) []suggestItem {
	// Bare "/mode-setup" without trailing space should also open the posture picker
	// (user reported "không có command scan show ra để chọn").
	if strings.EqualFold(strings.TrimSpace(input), "/mode-setup") {
		out := make([]suggestItem, 0, len(postureOrder))
		for _, p := range postureOrder {
			out = append(out, suggestItem{value: p, detail: postureLabel(p), kind: "mode-setup-posture"})
		}
		return out
	}
	ok, query := parseSlashArgPrefix(input, "/mode-setup")
	if !ok {
		return nil
	}
	parts := strings.Fields(query)

	// No args yet: show posture picker (scan/plan/code).
	if query == "" {
		out := make([]suggestItem, 0, len(postureOrder))
		for _, p := range postureOrder {
			out = append(out, suggestItem{value: p, detail: postureLabel(p), kind: "mode-setup-posture"})
		}
		return out
	}

	// One token: posture (partial or exact).
	if len(parts) == 1 {
		tok := strings.ToLower(parts[0])
		if validPosture(tok) {
			// Exact posture -> show field picker for that posture.
			// Provider removed from picker (model auto-pins provider); typing
			// "provider" still works via value stage.
			fields := []string{"model", "reasoning", "yolo", "clear"}
			out := make([]suggestItem, 0, len(fields))
			for _, f := range fields {
				out = append(out, suggestItem{value: tok + " " + f, detail: modeSetupFieldDetail(f), kind: "mode-setup-field"})
			}
			return out
		}
		// Partial posture -> filter posture list.
		out := make([]suggestItem, 0, len(postureOrder))
		for _, p := range postureOrder {
			if strings.HasPrefix(p, tok) || strings.Contains(p, tok) {
				out = append(out, suggestItem{value: p, detail: postureLabel(p), kind: "mode-setup-posture"})
			}
		}
		return out
	}

	// Two tokens: posture + field (partial or exact).
	posture := strings.ToLower(parts[0])
	if !validPosture(posture) {
		return nil
	}
	if len(parts) == 2 {
		fieldPart := strings.ToLower(parts[1])
		// Keep provider in exact-match so typed "/mode-setup scan provider" still works,
		// but don't advertise it in partial picker.
		allFields := []string{"provider", "model", "reasoning", "yolo", "clear"}
		for _, f := range allFields {
			if f == fieldPart {
				if f == "clear" {
					return nil // no value stage
				}
				return modeSetupValueSuggestions(posture, f, "", providers, accounts, currentProvider, currentModel)
			}
		}
		pickerFields := []string{"model", "reasoning", "yolo", "clear"}
		out := make([]suggestItem, 0, len(pickerFields))
		for _, f := range pickerFields {
			if strings.HasPrefix(f, fieldPart) || strings.Contains(f, fieldPart) {
				out = append(out, suggestItem{value: posture + " " + f, detail: modeSetupFieldDetail(f), kind: "mode-setup-field"})
			}
		}
		return out
	}

	// Three+ tokens: posture + field + value query.
	field := strings.ToLower(parts[1])
	if field == "clear" {
		return nil
	}
	valueQuery := strings.Join(parts[2:], " ")
	return modeSetupValueSuggestions(posture, field, valueQuery, providers, accounts, currentProvider, currentModel)
}

func modeSetupFieldDetail(field string) string {
	switch field {
	case "provider":
		return "pin provider for this posture"
	case "model":
		return "pin model for this posture"
	case "reasoning":
		return "pin reasoning effort"
	case "yolo":
		return "pin YOLO on/off/clear"
	case "clear":
		return "clear all pins for this posture"
	default:
		return field
	}
}

func modeSetupValueSuggestions(posture, field, query string, providers []client.Provider, accounts []client.ProviderAccountSummary, currentProvider, currentModel string) []suggestItem {
	q := strings.ToLower(strings.TrimSpace(query))
	switch field {
	case "provider":
		seen := make(map[string]bool, len(providers)+len(accounts)+3)
		providerMap := make(map[string]client.Provider, len(providers))
		var keys []string
		for _, p := range providers {
			k := strings.TrimSpace(p.Key)
			if k == "" {
				continue
			}
			lk := strings.ToLower(k)
			if seen[lk] {
				continue
			}
			seen[lk] = true
			keys = append(keys, k)
			providerMap[lk] = p
		}
		if len(keys) == 0 {
			for _, a := range accounts {
				k := strings.TrimSpace(a.ProviderKey)
				if k == "" {
					continue
				}
				lk := strings.ToLower(k)
				if seen[lk] {
					continue
				}
				seen[lk] = true
				keys = append(keys, k)
			}
		}
		if len(keys) == 0 {
			for _, k := range []string{"claude", "codex", "grok"} {
				lk := strings.ToLower(k)
				if seen[lk] {
					continue
				}
				seen[lk] = true
				keys = append(keys, k)
			}
		}
		out := make([]suggestItem, 0, len(keys))
		for _, k := range keys {
			if q != "" && !strings.Contains(strings.ToLower(k), q) {
				continue
			}
			var detail string
			if p, ok := providerMap[strings.ToLower(k)]; ok {
				detail = providerSuggestionDetail(p, nil, currentProvider, "select")
			} else {
				detail = "provider"
				if strings.EqualFold(k, currentProvider) {
					detail = "current · provider"
				}
			}
			out = append(out, suggestItem{value: posture + " provider " + k, detail: detail, kind: "mode-setup-value"})
		}
		return out
	case "model":
		// FlowPilot rule: picking a model auto-pins its provider, and /model
		// lists all models across providers. So /mode-setup model must also
		// show every model with its provider as detail, not just the current
		// provider's models (which is empty for grok in the report).
		all := allModelsAcrossProviders(providers, currentProvider, currentModel)
		out := make([]suggestItem, 0, len(all))
		for _, e := range all {
			if q != "" && !strings.Contains(strings.ToLower(e.id), q) && !strings.Contains(strings.ToLower(e.provider), q) {
				continue
			}
			detail := e.provider
			if detail == "" {
				detail = "model"
			}
			out = append(out, suggestItem{value: posture + " model " + e.id, detail: detail, kind: "mode-setup-value"})
		}
		return out
	case "reasoning":
		out := make([]suggestItem, 0, len(reasoningEffortOptions))
		for _, eff := range reasoningEffortOptions {
			if q != "" && !strings.HasPrefix(eff, q) && !strings.Contains(eff, q) {
				continue
			}
			out = append(out, suggestItem{value: posture + " reasoning " + eff, detail: eff, kind: "mode-setup-value"})
		}
		return out
	case "yolo":
		opts := []string{"on", "off", "clear"}
		out := make([]suggestItem, 0, len(opts))
		for _, o := range opts {
			if q != "" && !strings.HasPrefix(o, q) && !strings.Contains(o, q) {
				continue
			}
			out = append(out, suggestItem{value: posture + " yolo " + o, detail: "yolo " + o, kind: "mode-setup-value"})
		}
		return out
	default:
		return nil
	}
}

// filterHistorySuggestions returns chats matching the query after /history|/open|/resume .
func filterHistorySuggestions(input string, items []client.RunHistoryItem) []suggestItem {
	return filterHistorySuggestionsWithRemote(input, items, nil)
}

// filterHistorySuggestionsWithRemote is filterHistorySuggestions plus Drive-index
// reconciliation (CA-552). The badge is placed right after the #N index (CA-553)
// so it stays visible even when the detail line is truncated to chatWidth.
// Grouped chats (CP-59 Q-4): one row per logical chat (Desktop parity). The
// haystack includes every leg's runId and the chatId so filtering by an older
// leg or chatId still surfaces the chat. Detail shows "N legs" when grouped.
func filterHistorySuggestionsWithRemote(input string, items []client.RunHistoryItem, remote []client.RemoteChatSessionSummary) []suggestItem {
	cmd, query, ok := parseChatOpenArgPrefix(input)
	if !ok {
		return nil
	}
	q := strings.ToLower(query)
	rows := groupRunsByChatId(items)
	out := make([]suggestItem, 0, len(rows))
	for i, row := range rows {
		it := row.item
		id := strings.TrimSpace(it.RunID)
		if id == "" {
			continue
		}
		title := strings.TrimSpace(it.LastPrompt)
		if title == "" {
			title = strings.TrimSpace(it.LastMessage)
		}
		if title == "" {
			title = "(no prompt)"
		}
		// Haystack spans head + all legs (older leg runId / chatId lookup).
		hayBase := id + " " + title + " " + it.Status + " " + it.ProviderKey + " " + it.RunKind + " " + it.SyncStatus + " " + it.ChatID
		if row.group != nil {
			for _, leg := range row.group.legs {
				hayBase += " " + leg.RunID + " " + leg.ChatID + " " + leg.ProviderKey
			}
		}
		hay := strings.ToLower(hayBase)
		if q != "" && !strings.Contains(hay, q) && !strings.Contains(strings.ToLower(shortID(id)), q) {
			// Also allow chatId / leg short id direct match
			matched := false
			if row.group != nil {
				for _, leg := range row.group.legs {
					if strings.Contains(strings.ToLower(leg.RunID), q) || strings.Contains(strings.ToLower(shortID(leg.RunID)), q) || strings.Contains(strings.ToLower(leg.ChatID), q) {
						matched = true
						break
					}
				}
				if strings.Contains(strings.ToLower(it.ChatID), q) {
					matched = true
				}
			}
			if !matched {
				continue
			}
		}
		kind := it.RunKind
		if kind == "" {
			if it.WorkflowID != "" {
				kind = "workflow"
			} else {
				kind = "chat"
			}
		}
		title = collapseWS(title)
		if len([]rune(title)) > 42 {
			r := []rune(title)
			title = string(r[:39]) + "…"
		}
		when := formatHistoryChangedAt(it)
		if when == "" {
			when = "—"
		}
		detail := fmt.Sprintf("#%d", i+1)
		if row.group != nil && len(row.group.legs) > 1 {
			detail += fmt.Sprintf(" · %d legs", len(row.group.legs))
		}
		if badge := syncBadgeWithRemote(it, remote); badge != "" {
			detail += " · " + badge
		}
		detail += " · " + kind + " · " + it.Status + " · " + when
		detail += " · " + title
		out = append(out, suggestItem{value: id, detail: detail, kind: "history", slash: cmd})
	}
	return out
}

// filterDeleteSuggestions returns chats matching the query after `/delete `.
// Task-318 v2: skill-like multi-select — Tab ticks, Enter deletes. Includes an
// `all` row plus filtered chats. Tick state is caller-owned; this pure helper
// defaults to unticked (use filterDeleteSuggestionsWithSelected for ticks).
func filterDeleteSuggestions(input string, items []client.RunHistoryItem) []suggestItem {
	return filterDeleteSuggestionsWithRemote(input, items, nil)
}

func filterDeleteSuggestionsWithRemote(input string, items []client.RunHistoryItem, remote []client.RemoteChatSessionSummary) []suggestItem {
	return filterDeleteSuggestionsWithSelected(input, items, remote, nil)
}

// filterDeleteSuggestionsWithSelected is filterDeleteSuggestions plus tick state.
// selected[runID]==true → "[*]" else "[ ]". The "all" row reflects all-ticked.
func filterDeleteSuggestionsWithSelected(input string, items []client.RunHistoryItem, remote []client.RemoteChatSessionSummary, selected map[string]bool) []suggestItem {
	ok, query := parseDeleteArgPrefix(input)
	if !ok {
		return nil
	}
	if len(items) == 0 {
		return nil
	}
	q := strings.ToLower(query)
	// all-ticked when every run is selected and at least one exists
	allTicked := len(selected) > 0
	for _, it := range items {
		if !selected[strings.TrimSpace(it.RunID)] {
			allTicked = false
			break
		}
	}
	out := make([]suggestItem, 0, len(items)+1)
	tickAll := "[ ]"
	if allTicked && len(items) > 0 {
		tickAll = "[*]"
	}
	out = append(out, suggestItem{
		value:  "all",
		detail: fmt.Sprintf("%s all · delete all %d chats", tickAll, len(items)),
		kind:   "delete",
		slash:  "/delete",
	})
	for i, it := range items {
		id := strings.TrimSpace(it.RunID)
		if id == "" {
			continue
		}
		title := strings.TrimSpace(it.LastPrompt)
		if title == "" {
			title = strings.TrimSpace(it.LastMessage)
		}
		if title == "" {
			title = "(no prompt)"
		}
		hay := strings.ToLower(id + " " + title + " " + it.Status + " " + it.ProviderKey + " " + it.RunKind + " " + it.SyncStatus)
		if q != "" && q != "all" && !strings.Contains(hay, q) && !strings.Contains(strings.ToLower(shortID(id)), q) {
			continue
		}
		kind := it.RunKind
		if kind == "" {
			if it.WorkflowID != "" {
				kind = "workflow"
			} else {
				kind = "chat"
			}
		}
		title = collapseWS(title)
		if len([]rune(title)) > 42 {
			r := []rune(title)
			title = string(r[:39]) + "…"
		}
		when := formatHistoryChangedAt(it)
		if when == "" {
			when = "—"
		}
		tick := "[ ]"
		if selected[id] {
			tick = "[*]"
		}
		detail := fmt.Sprintf("%s #%d", tick, i+1)
		if badge := syncBadgeWithRemote(it, remote); badge != "" {
			detail += " · " + badge
		}
		detail += " · " + kind + " · " + it.Status + " · " + when
		detail += " · " + title
		out = append(out, suggestItem{value: id, detail: detail, kind: "delete", slash: "/delete"})
	}
	return out
}

// filterFlowSuggestions returns flow refs matching the query after `/flow `.
func filterFlowSuggestions(
	input string,
	builtins []client.BuiltinFlowOption,
	workflows []client.Workflow,
	projectID string,
) []suggestItem {
	ok, query := parseFlowArgPrefix(input)
	if !ok {
		return nil
	}
	q := strings.ToLower(query)
	out := make([]suggestItem, 0, len(builtins)+len(workflows))
	for _, opt := range builtins {
		ref := strings.TrimSpace(opt.FlowRef)
		if ref == "" {
			continue
		}
		if q != "" &&
			!strings.Contains(strings.ToLower(ref), q) &&
			!strings.Contains(strings.ToLower(opt.Label), q) {
			continue
		}
		detail := opt.Label
		if detail == "" {
			detail = "builtin"
		} else {
			detail = detail + " · builtin"
		}
		out = append(out, suggestItem{value: ref, detail: detail, kind: "flow"})
	}
	for _, wf := range workflows {
		if projectID != "" && wf.ProjectID != "" && wf.ProjectID != projectID {
			continue
		}
		id := strings.TrimSpace(wf.ID)
		name := strings.TrimSpace(wf.Name)
		if id == "" {
			continue
		}
		if q != "" &&
			!strings.Contains(strings.ToLower(id), q) &&
			!strings.Contains(strings.ToLower(name), q) {
			continue
		}
		detail := name
		if detail == "" {
			detail = "catalog"
		} else {
			detail = detail + " · catalog"
		}
		out = append(out, suggestItem{value: id, detail: detail, kind: "flow"})
	}
	return out
}

// modelsForProvider returns model ids for a provider key.
func modelsForProvider(providers []client.Provider, providerKey string) []string {
	for _, p := range providers {
		if !strings.EqualFold(p.Key, providerKey) {
			continue
		}
		out := make([]string, 0, len(p.Models))
		for _, m := range p.Models {
			if id := m.ModelID(); id != "" {
				out = append(out, id)
			}
		}
		return out
	}
	return nil
}

const minOpencodeCatalogModels = 10

func opencodeCatalogModelCount(providers []client.Provider) int {
	for _, p := range providers {
		if strings.EqualFold(p.Key, "opencode") {
			return len(p.Models)
		}
	}
	return 0
}

func needsOpencodeCatalogWarmRetry(providers []client.Provider) bool {
	for _, p := range providers {
		if !strings.EqualFold(p.Key, "opencode") {
			continue
		}
		return p.Installed && len(p.Models) < minOpencodeCatalogModels
	}
	return false
}

// defaultModelsForKey returns registry defaults when a provider is in m.providers
// but its Models slice is empty (live grok cache miss, codex/claude not probed).
// Mirrors runner/providerSpecs defaults so picker never empty for a known provider.
func defaultModelsForKey(key string) []string {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "codex":
		return []string{"gpt-5.5", "gpt-5.4", "gpt-5.4-mini"}
	case "claude":
		return []string{"claude-opus", "claude-sonnet", "claude-haiku"}
	case "grok":
		return []string{"grok-4.5", "grok-build"}
	case "gemini":
		return []string{"gemini-3.5-flash-medium", "gemini-3.5-flash-high", "gemini-3.5-flash-low", "gemini-3.1-pro-low", "gemini-3.1-pro-high"}
	default:
		return nil
	}
}

// allModelsAcrossProviders returns every model id across m.providers (dedup),
// filling registry defaults for providers that are present but have no Models,
// plus currentModel if not already present — so /model and /mode-setup model
// always show the full supported list, not just the current provider.
func allModelsAcrossProviders(providers []client.Provider, currentProvider, currentModel string) []modelEntry {
	seen := make(map[string]bool, 32)
	seenProvider := make(map[string]bool, 8)
	var out []modelEntry
	for _, p := range providers {
		seenProvider[strings.ToLower(p.Key)] = true
		ids := make([]string, 0, len(p.Models))
		for _, m := range p.Models {
			if id := strings.TrimSpace(m.ModelID()); id != "" {
				ids = append(ids, id)
			}
		}
		if len(ids) == 0 {
			ids = defaultModelsForKey(p.Key)
		}
		for _, id := range ids {
			lk := strings.ToLower(id)
			if seen[lk] {
				continue
			}
			seen[lk] = true
			out = append(out, modelEntry{provider: p.Key, id: id})
		}
	}
	// Ensure full registry is visible even when catalog hasn't loaded or a
	// provider is missing from m.providers (live: only grok present). This
	// makes /model always the full supported list, not just the detected one.
	for _, key := range []string{"claude", "codex", "grok", "gemini"} {
		if seenProvider[strings.ToLower(key)] {
			continue
		}
		for _, id := range defaultModelsForKey(key) {
			lk := strings.ToLower(id)
			if seen[lk] {
				continue
			}
			seen[lk] = true
			out = append(out, modelEntry{provider: key, id: id})
		}
	}
	if cm := strings.TrimSpace(currentModel); cm != "" {
		lk := strings.ToLower(cm)
		if !seen[lk] {
			seen[lk] = true
			prov := providerForModel(providers, cm)
			if prov == "" {
				prov = currentProvider
			}
			out = append(out, modelEntry{provider: prov, id: cm})
		}
	}
	return out
}

type modelEntry struct {
	provider string
	id       string
}

func gateFromEvent(ev client.ProviderEvent) *parsedGate {
	switch ev.Type {
	case "permission_required":
		if ev.ApprovalID == "" {
			return nil
		}
		return &parsedGate{Kind: "approval", ApprovalID: ev.ApprovalID}
	case "user_question_required":
		if ev.QuestionID == "" {
			return nil
		}
		opts := make([]string, 0, len(ev.Options))
		for _, o := range ev.Options {
			if v := o["label"]; v != "" {
				opts = append(opts, v)
			}
		}
		return &parsedGate{Kind: "question", QuestionID: ev.QuestionID, Options: opts}
	case "flow_gate_violation":
		return &parsedGate{Kind: "gate", Options: ev.GateOptions}
	default:
		return nil
	}
}
