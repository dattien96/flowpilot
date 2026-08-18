package app

import (
	"context"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
)

func questionOptionToken(o map[string]string) string {
	for _, k := range []string{"id", "value", "label"} {
		if v := strings.TrimSpace(o[k]); v != "" {
			return v
		}
	}
	return ""
}

func questionOptionLabel(o map[string]string) string {
	if v := strings.TrimSpace(o["label"]); v != "" {
		return v
	}
	return questionOptionToken(o)
}

func questionOptionDescription(o map[string]string) string {
	return strings.TrimSpace(o["description"])
}

func resolveQuestionChoice(input string, opts []map[string]string) string {
	raw := strings.TrimSpace(input)
	if raw == "" {
		return ""
	}
	if n, err := strconv.Atoi(raw); err == nil && n >= 1 && n <= len(opts) {
		return questionOptionToken(opts[n-1])
	}
	norm := strings.ToLower(strings.TrimPrefix(strings.ToLower(raw), "/"))
	for _, o := range opts {
		label := strings.ToLower(strings.TrimSpace(questionOptionLabel(o)))
		token := strings.ToLower(strings.TrimSpace(questionOptionToken(o)))
		if norm == label || norm == token {
			return questionOptionToken(o)
		}
	}
	if alias, ok := approvalAlias(norm); ok {
		if tok := matchQuestionAlias(opts, alias); tok != "" {
			return tok
		}
	}
	return raw
}

func approvalAlias(norm string) (string, bool) {
	switch norm {
	case "approve", "yes", "y", "allow":
		return "approve", true
	case "deny", "no", "n", "reject":
		return "deny", true
	default:
		return "", false
	}
}

func matchQuestionAlias(opts []map[string]string, alias string) string {
	for _, o := range opts {
		words := strings.FieldsFunc(strings.ToLower(questionOptionLabel(o)), func(r rune) bool {
			return r == ' ' || r == '—' || r == '-' || r == ':' || r == '/'
		})
		if len(words) == 0 {
			continue
		}
		w := words[0]
		if alias == "approve" && (w == "approve" || w == "yes" || w == "allow") {
			return questionOptionToken(o)
		}
		if alias == "deny" && (w == "deny" || w == "no" || w == "reject") {
			return questionOptionToken(o)
		}
	}
	return ""
}

func formatQuestionMessage(prompt string, opts []map[string]string, multi bool) string {
	var b strings.Builder
	b.WriteString("[QUESTION] ")
	b.WriteString(strings.TrimSpace(prompt))
	if len(opts) == 0 {
		return b.String()
	}
	b.WriteString("\n")
	for i, o := range opts {
		b.WriteString("  ")
		b.WriteString(strconv.Itoa(i + 1))
		b.WriteString(") ")
		b.WriteString(questionOptionLabel(o))
		if desc := questionOptionDescription(o); desc != "" {
			b.WriteString(" — ")
			b.WriteString(desc)
		}
		if i+1 < len(opts) {
			b.WriteString("\n")
		}
	}
	if multi {
		b.WriteString("\n  [multi-select] pick options (click or type 1/2/3), then /submit to send")
	}
	return b.String()
}

func (m *AppModel) submitQuestionAnswer(input string) (tea.Model, tea.Cmd) {
	if m.question == nil {
		m.addMessage("system", "No pending question.", "")
		return m, nil
	}
	q := m.question
	choice := resolveQuestionChoice(input, q.Options)
	if strings.TrimSpace(choice) == "" {
		if q.MultiSelect {
			m.addMessage("system", "Pick an option (click or type 1/2/3) to toggle it, or /submit to send.", "question")
		} else {
			m.addMessage("system", "Pick an option (click, type 1/2/3, or approve/deny).", "question")
		}
		return m, nil
	}
	if q.MultiSelect {
		return m.toggleQuestionSelection(choice), nil
	}
	return m, m.cmdAnswerQuestion(m.question.ID, choice)
}

// toggleQuestionSelection flips one option in the multiSelect selection set.
func (m *AppModel) toggleQuestionSelection(value string) *AppModel {
	label := value
	for _, o := range m.question.Options {
		if questionOptionToken(o) == value {
			label = questionOptionLabel(o)
			break
		}
	}
	sel := m.question.Selected
	removed := false
	for i, v := range sel {
		if v == value {
			sel = append(sel[:i], sel[i+1:]...)
			removed = true
			break
		}
	}
	if !removed {
		sel = append(sel, value)
		m.addMessage("system", "Selected "+label+". /submit to send.", "question")
	} else {
		m.addMessage("system", "Deselected "+label+".", "question")
	}
	m.question.Selected = sel
	return m
}

// submitQuestionSubmit sends the multiSelect selection set (or surfaces a hint
// for a single-select question that expects one option).
func (m *AppModel) submitQuestionSubmit() (tea.Model, tea.Cmd) {
	if m.question == nil {
		m.addMessage("system", "No pending question.", "")
		return m, nil
	}
	if !m.question.MultiSelect {
		m.addMessage("system", "This question takes one answer — click an option or type 1/2/3.", "question")
		return m, nil
	}
	sel := m.question.Selected
	if len(sel) == 0 {
		m.addMessage("system", "Select at least one option first (click or type 1/2/3), then /submit.", "question")
		return m, nil
	}
	return m, m.cmdAnswerQuestionMulti(m.question.ID, sel)
}

func (m *AppModel) cmdAnswerQuestion(questionID, choice string) tea.Cmd {
	runnerURL := m.runnerURL
	id := questionID
	ch := choice
	return func() tea.Msg {
		cl := client.New(runnerURL)
		if err := cl.AnswerQuestion(context.Background(), id, ch); err != nil {
			return ErrMsg{Err: err}
		}
		return QuestionResolvedMsg{ID: id, Choice: ch}
	}
}

func (m *AppModel) cmdAnswerQuestionMulti(questionID string, choices []string) tea.Cmd {
	runnerURL := m.runnerURL
	id := questionID
	ch := choices
	return func() tea.Msg {
		cl := client.New(runnerURL)
		if err := cl.AnswerQuestionMulti(context.Background(), id, ch); err != nil {
			return ErrMsg{Err: err}
		}
		return QuestionResolvedMsg{ID: id, Choice: strings.Join(ch, ", ")}
	}
}
