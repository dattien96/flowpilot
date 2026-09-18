package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"flowpilot-runner/internal/tui/client"
)

const (
	supabaseFieldURL = iota
	supabaseFieldAnonKey
	supabaseFieldServiceKey
	supabaseFieldSave
	supabaseFieldCancel
	supabaseFieldCount
)

func (m *AppModel) openSupabaseSetupModal() {
	m.supabaseSetupModalOpen = true
	m.supabaseSetupModalField = supabaseFieldURL
	m.supabaseSetupModalBusy = false
	m.supabaseSetupModalErr = ""
	m.clearInputValue()
}

func (m *AppModel) closeSupabaseSetupModal() {
	m.supabaseSetupModalOpen = false
	m.supabaseSetupModalBusy = false
	m.supabaseSetupModalErr = ""
}

func (m *AppModel) cmdSaveSupabaseConfig(apiUrl, anonKey, serviceRoleKey string) tea.Cmd {
	runnerURL := m.runnerURL
	return func() tea.Msg {
		cl := client.New(runnerURL)
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		err := cl.SaveSupabaseConfig(ctx, client.SaveSupabaseConfigRequest{
			APIURL:         strings.TrimSpace(apiUrl),
			AnonKey:        strings.TrimSpace(anonKey),
			ServiceRoleKey: strings.TrimSpace(serviceRoleKey),
		})
		if err != nil {
			return ErrMsg{Err: fmt.Errorf("save supabase config failed: %w", err)}
		}
		return SupabaseConfigSavedMsg{}
	}
}

func (m *AppModel) handleSupabaseSetupModalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.supabaseSetupModalBusy {
		return m, nil
	}

	switch msg.Type {
	case tea.KeyEsc:
		m.closeSupabaseSetupModal()
		return m, nil

	case tea.KeyTab, tea.KeyDown:
		m.supabaseSetupModalField = (m.supabaseSetupModalField + 1) % supabaseFieldCount
		return m, nil

	case tea.KeyShiftTab, tea.KeyUp:
		m.supabaseSetupModalField = (m.supabaseSetupModalField + supabaseFieldCount - 1) % supabaseFieldCount
		return m, nil

	case tea.KeyEnter:
		switch m.supabaseSetupModalField {
		case supabaseFieldURL:
			m.supabaseSetupModalField = supabaseFieldAnonKey
			return m, nil
		case supabaseFieldAnonKey:
			m.supabaseSetupModalField = supabaseFieldServiceKey
			return m, nil
		case supabaseFieldServiceKey:
			m.supabaseSetupModalField = supabaseFieldSave
			return m, nil
		case supabaseFieldSave:
			u := strings.TrimSpace(m.supabaseSetupModalURL)
			if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
				m.supabaseSetupModalErr = "API URL must begin with http:// or https://"
				return m, nil
			}
			ak := strings.TrimSpace(m.supabaseSetupModalAnonKey)
			if len(ak) == 0 {
				m.supabaseSetupModalErr = "Anon Key is required"
				return m, nil
			}
			m.supabaseSetupModalBusy = true
			m.supabaseSetupModalErr = ""
			return m, m.cmdSaveSupabaseConfig(u, ak, m.supabaseSetupModalServiceKey)
		case supabaseFieldCancel:
			m.closeSupabaseSetupModal()
			return m, nil
		}

	case tea.KeyBackspace, tea.KeyDelete:
		switch m.supabaseSetupModalField {
		case supabaseFieldURL:
			if len(m.supabaseSetupModalURL) > 0 {
				r := []rune(m.supabaseSetupModalURL)
				m.supabaseSetupModalURL = string(r[:len(r)-1])
			}
		case supabaseFieldAnonKey:
			if len(m.supabaseSetupModalAnonKey) > 0 {
				r := []rune(m.supabaseSetupModalAnonKey)
				m.supabaseSetupModalAnonKey = string(r[:len(r)-1])
			}
		case supabaseFieldServiceKey:
			if len(m.supabaseSetupModalServiceKey) > 0 {
				r := []rune(m.supabaseSetupModalServiceKey)
				m.supabaseSetupModalServiceKey = string(r[:len(r)-1])
			}
		}
		return m, nil

	case tea.KeyRunes:
		switch m.supabaseSetupModalField {
		case supabaseFieldURL:
			m.supabaseSetupModalURL += string(msg.Runes)
		case supabaseFieldAnonKey:
			m.supabaseSetupModalAnonKey += string(msg.Runes)
		case supabaseFieldServiceKey:
			m.supabaseSetupModalServiceKey += string(msg.Runes)
		}
		return m, nil
	}

	return m, nil
}

func (m *AppModel) renderSupabaseSetupModal(width int) string {
	if !m.supabaseSetupModalOpen {
		return ""
	}
	if width < 55 {
		width = 55
	}
	if width > 85 {
		width = 85
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
	title := " Supabase Workspace Setup "
	sb.WriteString(border.Render("│"))
	sb.WriteString(titleStyle.Render(lipgloss.PlaceHorizontal(innerW, lipgloss.Center, title)))
	sb.WriteString(border.Render("│") + "\n")
	sb.WriteString(border.Render("├" + strings.Repeat("─", width-2) + "┤") + "\n")

	// Notice line
	sb.WriteString(border.Render("│ "))
	notice := "Enter Supabase credentials (saved locally in settings)"
	sb.WriteString(hintStyle.Render(notice))
	remNotice := innerW - lipgloss.Width(notice) - 1
	if remNotice > 0 {
		sb.WriteString(strings.Repeat(" ", remNotice))
	}
	sb.WriteString(border.Render(" │") + "\n")

	// Fields
	type sField struct {
		label    string
		value    string
		focusIdx int
		masked   bool
	}

	displayAnon := m.supabaseSetupModalAnonKey
	if len(displayAnon) > 24 {
		displayAnon = displayAnon[:10] + "..." + displayAnon[len(displayAnon)-8:]
	}

	fields := []sField{
		{"API URL", m.supabaseSetupModalURL, supabaseFieldURL, false},
		{"Anon Key", displayAnon, supabaseFieldAnonKey, false},
		{"Service Key", strings.Repeat("•", len(m.supabaseSetupModalServiceKey)), supabaseFieldServiceKey, true},
	}

	for _, f := range fields {
		sb.WriteString(border.Render("│ "))
		lbl := fmt.Sprintf("%-13s", f.label+":")
		sb.WriteString(fieldLabel.Render(lbl))

		valStr := f.value
		isFocused := m.supabaseSetupModalField == f.focusIdx
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
	if m.supabaseSetupModalBusy {
		sb.WriteString(border.Render("│ "))
		busyLine := " Validating & saving Supabase workspace config..."
		sb.WriteString(hintStyle.Render(busyLine))
		rem := innerW - lipgloss.Width(busyLine) - 1
		if rem > 0 {
			sb.WriteString(strings.Repeat(" ", rem))
		}
		sb.WriteString(border.Render(" │") + "\n")
	} else if m.supabaseSetupModalErr != "" {
		sb.WriteString(border.Render("│ "))
		errLine := " ! " + m.supabaseSetupModalErr
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
	saveLbl := " Save Configuration "
	cancelLbl := " Cancel "
	var saveStyle, cancelStyle lipgloss.Style
	if m.supabaseSetupModalField == supabaseFieldSave {
		saveStyle = btnActive
	} else {
		saveStyle = btnInactive
	}
	if m.supabaseSetupModalField == supabaseFieldCancel {
		cancelStyle = btnActive
	} else {
		cancelStyle = btnInactive
	}

	btnRow := saveStyle.Render(saveLbl) + "  " + cancelStyle.Render(cancelLbl)
	sb.WriteString(btnRow)
	remBtns := innerW - lipgloss.Width(btnRow) - 1
	if remBtns > 0 {
		sb.WriteString(strings.Repeat(" ", remBtns))
	}
	sb.WriteString(border.Render(" │") + "\n")

	sb.WriteString(border.Render("╰" + strings.Repeat("─", width-2) + "╯"))
	return sb.String()
}
