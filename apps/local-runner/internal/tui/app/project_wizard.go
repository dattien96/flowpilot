package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"flowpilot-runner/internal/tui/client"
)

const (
	wizardFieldName = iota
	wizardFieldPlatform
	wizardFieldModel
	wizardFieldSubmit
	wizardFieldCancel
	wizardFieldCount
)

var wizardPlatformOptions = []string{
	"nextjs",
	"golang",
	"android",
	"reactjs",
	"python",
	"node",
	"rust",
	"general",
}

// detectProjectPlatform inspects the target folder to infer its technology stack.
func detectProjectPlatform(dir string) string {
	if dir == "" {
		return "general"
	}

	// 1. Golang
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
		return "golang"
	}

	// 2. Android
	for _, f := range []string{"build.gradle", "settings.gradle", "build.gradle.kts", "settings.gradle.kts"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err == nil {
			return "android"
		}
	}

	// 3. Rust
	if _, err := os.Stat(filepath.Join(dir, "Cargo.toml")); err == nil {
		return "rust"
	}

	// 4. Python
	for _, f := range []string{"requirements.txt", "pyproject.toml", "Pipfile", "setup.py"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err == nil {
			return "python"
		}
	}

	// 5. JavaScript / TypeScript
	pkgPath := filepath.Join(dir, "package.json")
	if data, err := os.ReadFile(pkgPath); err == nil {
		s := strings.ToLower(string(data))
		if strings.Contains(s, "\"next\"") {
			return "nextjs"
		}
		if strings.Contains(s, "\"react\"") {
			return "reactjs"
		}
		return "node"
	}

	return "general"
}

func (m *AppModel) openProjectWizard(dir string) {
	if dir == "" {
		dir = m.cfg.ProjectPath
	}
	if dir == "" {
		if cwd, err := os.Getwd(); err == nil {
			dir = cwd
		}
	}
	name := filepath.Base(dir)
	if name == "." || name == "/" || name == "\\" {
		name = "my-project"
	}

	platform := detectProjectPlatform(dir)
	model := m.model
	if model == "" {
		model = "gemini-2.5-flash"
	}

	m.projectWizardOpen = true
	m.projectWizardField = wizardFieldName
	m.projectWizardDir = dir
	m.projectWizardName = name
	m.projectWizardPlatform = platform
	m.projectWizardModel = model
	m.projectWizardBusy = false
	m.projectWizardErr = ""
	m.clearInputValue()
}

func (m *AppModel) closeProjectWizard() {
	m.projectWizardOpen = false
	m.projectWizardBusy = false
	m.projectWizardErr = ""
}

func (m *AppModel) cmdCreateAndBindProject(dir, name, platform, model string) tea.Cmd {
	runnerURL := m.runnerURL
	return func() tea.Msg {
		cl := client.New(runnerURL)
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		proj, err := cl.CreateProject(ctx, client.CreateProjectInput{
			Name:          strings.TrimSpace(name),
			DirectoryPath: strings.TrimSpace(dir),
			Platform:      strings.TrimSpace(platform),
			DefaultModel:  strings.TrimSpace(model),
		})
		if err != nil {
			return ErrMsg{Err: fmt.Errorf("create project failed: %w", err)}
		}
		return ProjectCreatedMsg{Project: proj}
	}
}

func (m *AppModel) handleProjectWizardKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.projectWizardBusy {
		return m, nil
	}

	switch msg.Type {
	case tea.KeyEsc:
		m.closeProjectWizard()
		return m, nil

	case tea.KeyTab, tea.KeyDown:
		m.projectWizardField = (m.projectWizardField + 1) % wizardFieldCount
		return m, nil

	case tea.KeyShiftTab, tea.KeyUp:
		m.projectWizardField = (m.projectWizardField + wizardFieldCount - 1) % wizardFieldCount
		return m, nil

	case tea.KeyLeft:
		if m.projectWizardField == wizardFieldPlatform {
			m.cycleWizardPlatform(-1)
			return m, nil
		}

	case tea.KeyRight:
		if m.projectWizardField == wizardFieldPlatform {
			m.cycleWizardPlatform(1)
			return m, nil
		}

	case tea.KeyEnter:
		switch m.projectWizardField {
		case wizardFieldName:
			m.projectWizardField = wizardFieldPlatform
			return m, nil
		case wizardFieldPlatform:
			m.projectWizardField = wizardFieldModel
			return m, nil
		case wizardFieldModel:
			m.projectWizardField = wizardFieldSubmit
			return m, nil
		case wizardFieldSubmit:
			name := strings.TrimSpace(m.projectWizardName)
			if name == "" {
				m.projectWizardErr = "Project name cannot be empty"
				return m, nil
			}
			m.projectWizardBusy = true
			m.projectWizardErr = ""
			return m, m.cmdCreateAndBindProject(
				m.projectWizardDir,
				m.projectWizardName,
				m.projectWizardPlatform,
				m.projectWizardModel,
			)
		case wizardFieldCancel:
			m.closeProjectWizard()
			return m, nil
		}

	case tea.KeyBackspace, tea.KeyDelete:
		switch m.projectWizardField {
		case wizardFieldName:
			if len(m.projectWizardName) > 0 {
				r := []rune(m.projectWizardName)
				m.projectWizardName = string(r[:len(r)-1])
			}
		case wizardFieldModel:
			if len(m.projectWizardModel) > 0 {
				r := []rune(m.projectWizardModel)
				m.projectWizardModel = string(r[:len(r)-1])
			}
		}
		return m, nil

	case tea.KeyRunes:
		switch m.projectWizardField {
		case wizardFieldName:
			m.projectWizardName += string(msg.Runes)
		case wizardFieldModel:
			m.projectWizardModel += string(msg.Runes)
		case wizardFieldPlatform:
			// Allow typing space to advance or cycle
			if string(msg.Runes) == " " {
				m.cycleWizardPlatform(1)
			}
		}
		return m, nil

	case tea.KeySpace:
		if m.projectWizardField == wizardFieldPlatform {
			m.cycleWizardPlatform(1)
			return m, nil
		} else if m.projectWizardField == wizardFieldName {
			m.projectWizardName += " "
			return m, nil
		}
	}

	return m, nil
}

func (m *AppModel) cycleWizardPlatform(delta int) {
	currentIdx := 0
	for i, opt := range wizardPlatformOptions {
		if strings.EqualFold(opt, m.projectWizardPlatform) {
			currentIdx = i
			break
		}
	}
	nextIdx := (currentIdx + delta + len(wizardPlatformOptions)) % len(wizardPlatformOptions)
	m.projectWizardPlatform = wizardPlatformOptions[nextIdx]
}

func (m *AppModel) renderProjectWizard(width int) string {
	if !m.projectWizardOpen {
		return ""
	}
	if width < 50 {
		width = 50
	}
	if width > 80 {
		width = 80
	}
	innerW := width - 4
	var sb strings.Builder

	border := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86"))
	fieldLabel := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	fieldValue := lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
	fieldValueFocus := lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Background(lipgloss.Color("237")).Bold(true)
	hintStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Italic(true)
	errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
	btnActive := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(lipgloss.Color("82"))
	btnInactive := lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Background(lipgloss.Color("238"))

	sb.WriteString(border.Render("╭" + strings.Repeat("─", width-2) + "╮") + "\n")
	title := " Onboard Project (Tab to cycle fields) "
	sb.WriteString(border.Render("│"))
	sb.WriteString(titleStyle.Render(lipgloss.PlaceHorizontal(innerW, lipgloss.Center, title)))
	sb.WriteString(border.Render("│") + "\n")
	sb.WriteString(border.Render("├" + strings.Repeat("─", width-2) + "┤") + "\n")

	// Directory Path row (read only display)
	sb.WriteString(border.Render("│ "))
	dirLabel := "Directory: "
	sb.WriteString(fieldLabel.Render(dirLabel))
	displayDir := m.projectWizardDir
	maxDirW := innerW - len(dirLabel) - 2
	if maxDirW > 10 && len(displayDir) > maxDirW {
		displayDir = "..." + displayDir[len(displayDir)-maxDirW+3:]
	}
	sb.WriteString(hintStyle.Render(displayDir))
	remDir := innerW - lipgloss.Width(dirLabel) - lipgloss.Width(displayDir) - 1
	if remDir > 0 {
		sb.WriteString(strings.Repeat(" ", remDir))
	}
	sb.WriteString(border.Render(" │") + "\n")

	// Fields
	type wizardField struct {
		label    string
		value    string
		focusIdx int
		suffix   string
	}
	fields := []wizardField{
		{"Name", m.projectWizardName, wizardFieldName, ""},
		{"Platform", "< " + m.projectWizardPlatform + " >", wizardFieldPlatform, " (←/→ or Space to cycle)"},
		{"Model", m.projectWizardModel, wizardFieldModel, ""},
	}

	for _, f := range fields {
		sb.WriteString(border.Render("│ "))
		lbl := fmt.Sprintf("%-11s", f.label+":")
		sb.WriteString(fieldLabel.Render(lbl))

		valStr := f.value
		isFocused := m.projectWizardField == f.focusIdx
		display := " " + valStr
		if isFocused && f.focusIdx != wizardFieldPlatform {
			display += "▏"
		}
		var styled string
		if isFocused {
			styled = fieldValueFocus.Render(display)
		} else {
			styled = fieldValue.Render(display)
		}
		if isFocused {
			styled = "▶" + styled
		} else {
			styled = " " + styled
		}
		sb.WriteString(styled)
		if f.suffix != "" {
			sb.WriteString(hintStyle.Render(f.suffix))
		}

		used := 1 + lipgloss.Width(lbl) + lipgloss.Width(valStr) + 3 + lipgloss.Width(f.suffix)
		if used < innerW {
			sb.WriteString(strings.Repeat(" ", innerW-used))
		}
		sb.WriteString(border.Render(" │") + "\n")
	}

	// Error or busy status
	if m.projectWizardBusy {
		sb.WriteString(border.Render("│ "))
		busyLine := " Creating project & binding in Supabase..."
		sb.WriteString(hintStyle.Render(busyLine))
		rem := innerW - lipgloss.Width(busyLine) - 1
		if rem > 0 {
			sb.WriteString(strings.Repeat(" ", rem))
		}
		sb.WriteString(border.Render(" │") + "\n")
	} else if m.projectWizardErr != "" {
		sb.WriteString(border.Render("│ "))
		errLine := " ! " + m.projectWizardErr
		if len(errLine) > innerW-2 {
			errLine = errLine[:innerW-5] + "..."
		}
		sb.WriteString(errStyle.Render(errLine))
		rem := innerW - lipgloss.Width(errLine) - 1
		if rem > 0 {
			sb.WriteString(strings.Repeat(" ", rem))
		}
		sb.WriteString(border.Render(" │") + "\n")
	}

	// Action buttons
	sb.WriteString(border.Render("├" + strings.Repeat("─", width-2) + "┤") + "\n")
	sb.WriteString(border.Render("│ "))
	createLbl := " Create & Start Chat "
	cancelLbl := " Cancel "
	var createStyle, cancelStyle lipgloss.Style
	if m.projectWizardField == wizardFieldSubmit {
		createStyle = btnActive
	} else {
		createStyle = btnInactive
	}
	if m.projectWizardField == wizardFieldCancel {
		cancelStyle = btnActive
	} else {
		cancelStyle = btnInactive
	}

	btnRow := createStyle.Render(createLbl) + "  " + cancelStyle.Render(cancelLbl)
	sb.WriteString(btnRow)
	remBtns := innerW - lipgloss.Width(btnRow) - 1
	if remBtns > 0 {
		sb.WriteString(strings.Repeat(" ", remBtns))
	}
	sb.WriteString(border.Render(" │") + "\n")

	sb.WriteString(border.Render("╰" + strings.Repeat("─", width-2) + "╯"))
	return sb.String()
}
