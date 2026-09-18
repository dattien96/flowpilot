package app

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	loginFieldEmail = iota
	loginFieldPassword
	loginFieldSubmit
	loginFieldCancel
	loginFieldCount
)

func (m *AppModel) openLoginModal() {
	m.loginModalOpen = true
	m.loginModalField = loginFieldEmail
	if m.loginModalEmail == "" && m.authEmail != "" {
		m.loginModalEmail = m.authEmail
	}
	m.loginModalPassword = ""
	m.loginModalBusy = false
	m.loginModalErr = ""
	m.clearInputValue()
}

func (m *AppModel) closeLoginModal() {
	m.loginModalOpen = false
	m.loginModalBusy = false
	m.loginModalPassword = ""
	m.loginModalErr = ""
	m.authPhase = AuthNone
}

func (m *AppModel) handleLoginModalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.loginModalBusy {
		return m, nil
	}

	switch msg.Type {
	case tea.KeyEsc:
		m.closeLoginModal()
		return m, nil

	case tea.KeyTab, tea.KeyDown:
		m.loginModalField = (m.loginModalField + 1) % loginFieldCount
		return m, nil

	case tea.KeyShiftTab, tea.KeyUp:
		m.loginModalField = (m.loginModalField + loginFieldCount - 1) % loginFieldCount
		return m, nil

	case tea.KeyEnter:
		switch m.loginModalField {
		case loginFieldEmail:
			m.loginModalField = loginFieldPassword
			return m, nil
		case loginFieldPassword:
			if strings.Contains(m.loginModalEmail, "@") && len(m.loginModalPassword) > 0 {
				m.loginModalBusy = true
				m.loginModalErr = ""
				return m, m.cmdLogin(m.loginModalEmail, m.loginModalPassword)
			}
			m.loginModalField = loginFieldSubmit
			return m, nil
		case loginFieldSubmit:
			email := strings.TrimSpace(m.loginModalEmail)
			if !strings.Contains(email, "@") {
				m.loginModalErr = "Please enter a valid email address"
				return m, nil
			}
			if len(m.loginModalPassword) == 0 {
				m.loginModalErr = "Password cannot be empty"
				return m, nil
			}
			m.loginModalBusy = true
			m.loginModalErr = ""
			return m, m.cmdLogin(email, m.loginModalPassword)
		case loginFieldCancel:
			m.closeLoginModal()
			return m, nil
		}

	case tea.KeyBackspace, tea.KeyDelete:
		switch m.loginModalField {
		case loginFieldEmail:
			if len(m.loginModalEmail) > 0 {
				r := []rune(m.loginModalEmail)
				m.loginModalEmail = string(r[:len(r)-1])
			}
		case loginFieldPassword:
			if len(m.loginModalPassword) > 0 {
				r := []rune(m.loginModalPassword)
				m.loginModalPassword = string(r[:len(r)-1])
			}
		}
		return m, nil

	case tea.KeyRunes:
		switch m.loginModalField {
		case loginFieldEmail:
			m.loginModalEmail += string(msg.Runes)
		case loginFieldPassword:
			m.loginModalPassword += string(msg.Runes)
		}
		return m, nil

	case tea.KeySpace:
		// Spaces are not allowed in email or standard password shortcuts, but append if needed
		if m.loginModalField == loginFieldPassword {
			m.loginModalPassword += " "
			return m, nil
		}
	}

	return m, nil
}

func (m *AppModel) renderLoginModal(width int) string {
	if !m.loginModalOpen {
		return ""
	}
	if width < 50 {
		width = 50
	}
	if width > 75 {
		width = 75
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
	title := " Supabase Sign In "
	sb.WriteString(border.Render("│"))
	sb.WriteString(titleStyle.Render(lipgloss.PlaceHorizontal(innerW, lipgloss.Center, title)))
	sb.WriteString(border.Render("│") + "\n")
	sb.WriteString(border.Render("├" + strings.Repeat("─", width-2) + "┤") + "\n")

	// Notice line
	sb.WriteString(border.Render("│ "))
	notice := "Sign in with your FlowPilot Supabase account"
	sb.WriteString(hintStyle.Render(notice))
	remNotice := innerW - lipgloss.Width(notice) - 1
	if remNotice > 0 {
		sb.WriteString(strings.Repeat(" ", remNotice))
	}
	sb.WriteString(border.Render(" │") + "\n")

	// Fields
	type lField struct {
		label    string
		value    string
		focusIdx int
		masked   bool
	}
	fields := []lField{
		{"Email", m.loginModalEmail, loginFieldEmail, false},
		{"Password", strings.Repeat("•", len(m.loginModalPassword)), loginFieldPassword, true},
	}

	for _, f := range fields {
		sb.WriteString(border.Render("│ "))
		lbl := fmt.Sprintf("%-11s", f.label+":")
		sb.WriteString(fieldLabel.Render(lbl))

		valStr := f.value
		isFocused := m.loginModalField == f.focusIdx
		display := " " + valStr
		if isFocused {
			display += "▏"
		}
		var styled string
		if isFocused {
			styled = fieldValueFocus.Render(display)
			styled = "▶" + styled
		} else {
			styled = fieldValue.Render(display)
			styled = " " + styled
		}
		sb.WriteString(styled)

		used := 1 + lipgloss.Width(lbl) + lipgloss.Width(valStr) + 3
		if used < innerW {
			sb.WriteString(strings.Repeat(" ", innerW-used))
		}
		sb.WriteString(border.Render(" │") + "\n")
	}

	// Busy or error status
	if m.loginModalBusy {
		sb.WriteString(border.Render("│ "))
		busyLine := " Signing in to Supabase..."
		sb.WriteString(hintStyle.Render(busyLine))
		rem := innerW - lipgloss.Width(busyLine) - 1
		if rem > 0 {
			sb.WriteString(strings.Repeat(" ", rem))
		}
		sb.WriteString(border.Render(" │") + "\n")
	} else if m.loginModalErr != "" {
		sb.WriteString(border.Render("│ "))
		errLine := " ! " + m.loginModalErr
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
	submitLbl := " Sign In "
	cancelLbl := " Cancel "
	var submitStyle, cancelStyle lipgloss.Style
	if m.loginModalField == loginFieldSubmit {
		submitStyle = btnActive
	} else {
		submitStyle = btnInactive
	}
	if m.loginModalField == loginFieldCancel {
		cancelStyle = btnActive
	} else {
		cancelStyle = btnInactive
	}

	btnRow := submitStyle.Render(submitLbl) + "  " + cancelStyle.Render(cancelLbl)
	sb.WriteString(btnRow)
	remBtns := innerW - lipgloss.Width(btnRow) - 1
	if remBtns > 0 {
		sb.WriteString(strings.Repeat(" ", remBtns))
	}
	sb.WriteString(border.Render(" │") + "\n")

	sb.WriteString(border.Render("╰" + strings.Repeat("─", width-2) + "╯"))
	return sb.String()
}
