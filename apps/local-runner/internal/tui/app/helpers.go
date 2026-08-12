package app

import (
	"fmt"
	"path/filepath"
	"strings"

	"flowpilot-runner/internal/tui/client"
)

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
	m.pendingAttach = nil
}

func (m *AppModel) canSend() bool {
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

func orderAgentsMainFirst(runs []client.AgentRunSummary) []client.AgentRunSummary {
	if len(runs) <= 1 {
		return runs
	}
	out := make([]client.AgentRunSummary, 0, len(runs))
	var rest []client.AgentRunSummary
	for _, r := range runs {
		if strings.EqualFold(r.Role, "main") || strings.EqualFold(r.AgentName, "main") {
			out = append(out, r)
		} else {
			rest = append(rest, r)
		}
	}
	if len(out) == 0 {
		out = append(out, runs[0])
		rest = runs[1:]
	}
	return append(out, rest...)
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

// matchProjectByPath finds a catalog project whose Path equals target.
// Falls back to unique basename match when absolute paths differ (bindings).
func matchProjectByPath(projects []client.Project, target string) *client.Project {
	want := normalizePathKey(target)
	if want == "" {
		return nil
	}
	for i := range projects {
		if normalizePathKey(projects[i].Path) == want {
			return &projects[i]
		}
	}
	// Unique basename fallback (e.g. catalog path differs but folder name matches).
	base := strings.ToLower(filepath.Base(strings.ReplaceAll(target, `\`, `/`)))
	if base == "" || base == "." || base == "/" {
		return nil
	}
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
	return nil
}

func formatAccountLimits(acc *client.ProviderAccountSummary) string {
	if acc == nil {
		return ""
	}
	var parts []string
	if acc.Remaining5hPercent != nil {
		parts = append(parts, fmt.Sprintf("5h:%d%%", *acc.Remaining5hPercent))
	}
	if acc.Remaining7dPercent != nil {
		parts = append(parts, fmt.Sprintf("7d:%d%%", *acc.Remaining7dPercent))
	}
	if len(parts) == 0 && acc.UsageSummary != nil && strings.TrimSpace(*acc.UsageSummary) != "" {
		return strings.TrimSpace(*acc.UsageSummary)
	}
	return strings.Join(parts, " ")
}

func formatContextLimits(usage *client.TokenUsageSnapshot, fallbackWindow int64) string {
	if usage == nil {
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
		return ""
	}
	if window > 0 {
		left := window - used
		if left < 0 {
			left = 0
		}
		return fmt.Sprintf("ctx %s/%s (%s left)", formatTokenCount(used), formatTokenCount(window), formatTokenCount(left))
	}
	if usage.Last != nil {
		return fmt.Sprintf("last %s", formatTokenCount(usage.Last.TotalTokens))
	}
	return ""
}

func formatTokenCount(n int64) string {
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
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

// wrapText wraps s to width columns, preserving existing newlines.
// Long tokens without spaces are hard-broken so nothing is truncated off-screen.
func wrapText(s string, width int) []string {
	if width < 8 {
		width = 8
	}
	if s == "" {
		return []string{""}
	}
	var out []string
	for _, para := range strings.Split(s, "\n") {
		out = append(out, wrapParagraph(para, width)...)
	}
	return out
}

func wrapParagraph(para string, width int) []string {
	runes := []rune(para)
	if len(runes) == 0 {
		return []string{""}
	}
	if len(runes) <= width {
		return []string{para}
	}
	var lines []string
	for len(runes) > 0 {
		if len(runes) <= width {
			lines = append(lines, string(runes))
			break
		}
		cut := width
		// Prefer breaking on whitespace in the right half of the window.
		for i := width; i > width/2; i-- {
			if runes[i] == ' ' || runes[i] == '\t' {
				cut = i
				break
			}
		}
		line := strings.TrimRight(string(runes[:cut]), " \t")
		lines = append(lines, line)
		runes = runes[cut:]
		for len(runes) > 0 && (runes[0] == ' ' || runes[0] == '\t') {
			runes = runes[1:]
		}
	}
	return lines
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

var reasoningEffortOptions = []string{"high", "medium", "low"}

// filterProviderSuggestions returns providers matching the query after `/provider `.
func filterProviderSuggestions(input string, providers []client.Provider, current string) []suggestItem {
	ok, query := parseSlashArgPrefix(input, "/provider")
	if !ok {
		return nil
	}
	q := strings.ToLower(query)
	out := make([]suggestItem, 0, len(providers))
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
		detail := fmt.Sprintf("%d models", len(p.Models))
		if label != "" && !strings.EqualFold(label, key) {
			detail = label + " · " + detail
		}
		if strings.EqualFold(key, current) {
			detail = "current · " + detail
		}
		out = append(out, suggestItem{value: key, detail: detail, kind: "provider"})
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
func filterReasoningSuggestions(input string, current string) []suggestItem {
	ok, query := parseSlashArgPrefix(input, "/reasoning")
	if !ok {
		return nil
	}
	q := strings.ToLower(query)
	out := make([]suggestItem, 0, len(reasoningEffortOptions))
	for _, effort := range reasoningEffortOptions {
		if q != "" && !strings.HasPrefix(effort, q) && !strings.Contains(effort, q) {
			continue
		}
		detail := "effort"
		if strings.EqualFold(effort, current) {
			detail = "current"
		}
		out = append(out, suggestItem{value: effort, detail: detail, kind: "reasoning"})
	}
	return out
}

// filterHistorySuggestions returns chats matching the query after /history|/open|/resume .
func filterHistorySuggestions(input string, items []client.RunHistoryItem) []suggestItem {
	cmd, query, ok := parseChatOpenArgPrefix(input)
	if !ok {
		return nil
	}
	q := strings.ToLower(query)
	out := make([]suggestItem, 0, len(items))
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
		hay := strings.ToLower(id + " " + title + " " + it.Status + " " + it.ProviderKey + " " + it.RunKind)
		if q != "" && !strings.Contains(hay, q) && !strings.Contains(strings.ToLower(shortID(id)), q) {
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
		detail := fmt.Sprintf("#%d · %s · %s · %s", i+1, kind, it.Status, title)
		out = append(out, suggestItem{value: id, detail: detail, kind: "history", slash: cmd})
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
