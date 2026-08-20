package app

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"flowpilot-runner/internal/tui/client"
)

const (
	modalFocusModel = iota
	modalFocusReasoning
	modalFocusYolo
	modalFocusSave
	modalFocusCancel
	modalFocusCount
)

func (m *AppModel) openModeSetupModal(cfg client.ChatPostureConfig, tab string) {
	draft := cfg
	if draft.Profiles == nil {
		draft.Profiles = map[string]client.ChatPostureProfile{}
	} else {
		newMap := make(map[string]client.ChatPostureProfile, len(draft.Profiles))
		for k, v := range draft.Profiles {
			newMap[k] = v
		}
		draft.Profiles = newMap
	}
	for _, p := range postureOrder {
		if _, ok := draft.Profiles[p]; !ok {
			draft.Profiles[p] = client.ChatPostureProfile{}
		}
	}
	if !validPosture(tab) {
		tab = m.activePosture()
		if !validPosture(tab) {
			tab = "code"
		}
	}
	m.modeSetupModalOpen = true
	m.modeSetupModalTab = tab
	m.modeSetupModalDraft = &draft
	m.modeSetupModalFocus = modalFocusModel
	m.modeSetupModalPickerOpen = false
	m.modeSetupModalPickerIdx = 0
	m.modeSetupModalPickerKind = ""
	m.clearInputValue()
}

func (m *AppModel) closeModeSetupModal(save bool) tea.Cmd {
	if !m.modeSetupModalOpen {
		return nil
	}
	draft := m.modeSetupModalDraft
	m.modeSetupModalOpen = false
	m.modeSetupModalPickerOpen = false
	m.modeSetupModalPickerKind = ""
	m.modeSetupModalPickerIdx = 0
	m.modeSetupModalFocus = 0
	if !save || draft == nil {
		m.modeSetupModalDraft = nil
		m.addMessage("system", "Posture setup closed.", "")
		return nil
	}
	m.modeSetupModalDraft = nil
	m.chatPostureCfg = *draft
	m.chatPostureSaving = true
	m.chatPostureSavingPosture = draft.Active
	m.addMessage("system", "Saving posture profiles…", "")
	return m.cmdSaveChatPosture(*draft)
}

func (m *AppModel) reasoningOptionsForModel(modelID string) []string {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return nil
	}
	for _, p := range m.providers {
		for _, mod := range p.Models {
			if strings.EqualFold(mod.ModelID(), modelID) && len(mod.SupportedReasoningEfforts) > 0 {
				out := make([]string, len(mod.SupportedReasoningEfforts))
				for i, e := range mod.SupportedReasoningEfforts {
					out[i] = strings.ToLower(strings.TrimSpace(e))
				}
				return out
			}
		}
	}
	return append([]string(nil), reasoningEffortOptions...)
}

func (m *AppModel) isReasoningEnabled() bool {
	if m.modeSetupModalDraft == nil {
		return false
	}
	prof := m.modeSetupModalDraft.Profiles[m.modeSetupModalTab]
	return strings.TrimSpace(prof.Model) != ""
}

var modalYoloOptions = []string{"(inherit)", "on", "off"}

func yoloLabel(v *bool) string {
	if v == nil {
		return "(inherit)"
	}
	if *v {
		return "on"
	}
	return "off"
}

func (m *AppModel) handleModeSetupModalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.modeSetupModalPickerOpen {
		switch msg.Type {
		case tea.KeyEsc:
			m.modeSetupModalPickerOpen = false
			m.modeSetupModalPickerKind = ""
			return m, nil
		case tea.KeyUp:
			if m.modeSetupModalPickerIdx > 0 {
				m.modeSetupModalPickerIdx--
			}
			return m, nil
		case tea.KeyDown:
			opts := m.modalPickerOptions()
			if m.modeSetupModalPickerIdx < len(opts)-1 {
				m.modeSetupModalPickerIdx++
			}
			return m, nil
		case tea.KeyEnter:
			opts := m.modalPickerOptions()
			if len(opts) == 0 {
				m.modeSetupModalPickerOpen = false
				return m, nil
			}
			sel := opts[m.modeSetupModalPickerIdx%len(opts)]
			m.applyModalPickerSelection(sel)
			m.modeSetupModalPickerOpen = false
			m.modeSetupModalPickerKind = ""
			return m, nil
		case tea.KeyTab, tea.KeyShiftTab:
			return m, nil
		}
		return m, nil
	}
	switch msg.Type {
	case tea.KeyEsc:
		return m, m.closeModeSetupModal(false)
	case tea.KeyLeft:
		for i, p := range postureOrder {
			if p == m.modeSetupModalTab {
				if i > 0 {
					m.modeSetupModalTab = postureOrder[i-1]
				} else {
					m.modeSetupModalTab = postureOrder[len(postureOrder)-1]
				}
				m.modeSetupModalFocus = modalFocusModel
				break
			}
		}
		return m, nil
	case tea.KeyRight:
		for i, p := range postureOrder {
			if p == m.modeSetupModalTab {
				m.modeSetupModalTab = postureOrder[(i+1)%len(postureOrder)]
				m.modeSetupModalFocus = modalFocusModel
				break
			}
		}
		return m, nil
	case tea.KeyTab:
		if msg.String() == "shift+tab" {
			for {
				if m.modeSetupModalFocus == 0 {
					m.modeSetupModalFocus = modalFocusCancel
				} else {
					m.modeSetupModalFocus--
				}
				if m.modeSetupModalFocus == modalFocusReasoning && !m.isReasoningEnabled() {
					continue
				}
				break
			}
		} else {
			for {
				m.modeSetupModalFocus = (m.modeSetupModalFocus + 1) % modalFocusCount
				if m.modeSetupModalFocus == modalFocusReasoning && !m.isReasoningEnabled() {
					continue
				}
				break
			}
		}
		return m, nil
	case tea.KeyUp:
		if m.modeSetupModalFocus >= modalFocusSave {
			m.modeSetupModalFocus = modalFocusYolo
		} else if m.modeSetupModalFocus > modalFocusModel {
			for {
				m.modeSetupModalFocus--
				if m.modeSetupModalFocus == modalFocusReasoning && !m.isReasoningEnabled() {
					if m.modeSetupModalFocus == modalFocusModel {
						break
					}
					continue
				}
				break
			}
		}
		return m, nil
	case tea.KeyDown:
		if m.modeSetupModalFocus >= modalFocusSave {
			m.modeSetupModalFocus = modalFocusModel
		} else if m.modeSetupModalFocus < modalFocusYolo {
			for {
				m.modeSetupModalFocus++
				if m.modeSetupModalFocus == modalFocusReasoning && !m.isReasoningEnabled() {
					if m.modeSetupModalFocus == modalFocusYolo {
						break
					}
					continue
				}
				break
			}
			if m.modeSetupModalFocus > modalFocusYolo {
				m.modeSetupModalFocus = modalFocusSave
			}
		} else if m.modeSetupModalFocus == modalFocusYolo {
			m.modeSetupModalFocus = modalFocusSave
		}
		return m, nil
	case tea.KeyEnter:
		switch m.modeSetupModalFocus {
		case modalFocusModel:
			m.modeSetupModalPickerKind = "model"
			m.modeSetupModalPickerOpen = true
			m.modeSetupModalPickerIdx = 0
			prof := m.modeSetupModalDraft.Profiles[m.modeSetupModalTab]
			opts := m.modalPickerOptions()
			for i, o := range opts {
				if strings.EqualFold(o, prof.Model) {
					m.modeSetupModalPickerIdx = i
					break
				}
			}
			return m, nil
		case modalFocusReasoning:
			if !m.isReasoningEnabled() {
				m.addMessage("system", "Select a model first to enable reasoning.", "error")
				return m, nil
			}
			m.modeSetupModalPickerKind = "reasoning"
			m.modeSetupModalPickerOpen = true
			m.modeSetupModalPickerIdx = 0
			prof := m.modeSetupModalDraft.Profiles[m.modeSetupModalTab]
			opts := m.modalPickerOptions()
			for i, o := range opts {
				if strings.EqualFold(o, prof.ReasoningEffort) {
					m.modeSetupModalPickerIdx = i
					break
				}
			}
			return m, nil
		case modalFocusYolo:
			m.modeSetupModalPickerKind = "yolo"
			m.modeSetupModalPickerOpen = true
			m.modeSetupModalPickerIdx = 0
			prof := m.modeSetupModalDraft.Profiles[m.modeSetupModalTab]
			lbl := yoloLabel(prof.Yolo)
			opts := m.modalPickerOptions()
			for i, o := range opts {
				if o == lbl {
					m.modeSetupModalPickerIdx = i
					break
				}
			}
			return m, nil
		case modalFocusSave:
			return m, m.closeModeSetupModal(true)
		case modalFocusCancel:
			return m, m.closeModeSetupModal(false)
		}
		return m, nil
	case tea.KeyRunes:
		s := string(msg.Runes)
		switch strings.ToLower(s) {
		case "1":
			m.modeSetupModalTab = postureOrder[0]
			m.modeSetupModalFocus = modalFocusModel
			return m, nil
		case "2":
			if len(postureOrder) > 1 {
				m.modeSetupModalTab = postureOrder[1]
				m.modeSetupModalFocus = modalFocusModel
			}
			return m, nil
		case "3":
			if len(postureOrder) > 2 {
				m.modeSetupModalTab = postureOrder[2]
				m.modeSetupModalFocus = modalFocusModel
			}
			return m, nil
		case "s", "S":
			return m, m.closeModeSetupModal(true)
		case "q", "Q":
			return m, m.closeModeSetupModal(false)
		}
	}
	if msg.String() == "shift+tab" {
		for {
			if m.modeSetupModalFocus == 0 {
				m.modeSetupModalFocus = modalFocusCancel
			} else {
				m.modeSetupModalFocus--
			}
			if m.modeSetupModalFocus == modalFocusReasoning && !m.isReasoningEnabled() {
				continue
			}
			break
		}
		return m, nil
	}
	return m, nil
}

func (m *AppModel) modalPickerOptions() []string {
	switch m.modeSetupModalPickerKind {
	case "model":
		all := allModelsAcrossProviders(m.providers, m.provider, m.model)
		out := make([]string, 0, len(all)+1)
		out = append(out, "(inherit)")
		for _, e := range all {
			out = append(out, e.id)
		}
		return out
	case "reasoning":
		prof := m.modeSetupModalDraft.Profiles[m.modeSetupModalTab]
		opts := m.reasoningOptionsForModel(prof.Model)
		out := make([]string, 0, len(opts)+1)
		out = append(out, "(inherit)")
		out = append(out, opts...)
		return out
	case "yolo":
		return append([]string(nil), modalYoloOptions...)
	default:
		return nil
	}
}

func (m *AppModel) applyModalPickerSelection(sel string) {
	if m.modeSetupModalDraft == nil {
		return
	}
	prof := m.modeSetupModalDraft.Profiles[m.modeSetupModalTab]
	switch m.modeSetupModalPickerKind {
	case "model":
		if sel == "(inherit)" {
			prof.Model = ""
			prof.Provider = ""
			prof.ReasoningEffort = ""
		} else {
			prof.Model = sel
			if prov := providerForModel(m.providers, sel); prov != "" {
				prof.Provider = prov
			} else {
				prof.Provider = ""
			}
		}
	case "reasoning":
		if sel == "(inherit)" {
			prof.ReasoningEffort = ""
		} else {
			prof.ReasoningEffort = strings.ToLower(sel)
		}
	case "yolo":
		switch sel {
		case "(inherit)":
			prof.Yolo = nil
		case "on":
			on := true
			prof.Yolo = &on
		case "off":
			off := false
			prof.Yolo = &off
		}
	}
	m.modeSetupModalDraft.Profiles[m.modeSetupModalTab] = prof
}

func (m *AppModel) renderModeSetupModal(width int) string {
	if !m.modeSetupModalOpen || m.modeSetupModalDraft == nil {
		return ""
	}
	if width < 40 {
		width = 40
	}
	if width > 70 {
		width = 70
	}
	innerW := width - 4
	var sb strings.Builder
	border := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255"))
	tabActive := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255")).Background(lipgloss.Color("62"))
	tabInactive := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	fieldLabel := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	fieldValue := lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
	fieldValueFocus := lipgloss.NewStyle().Foreground(lipgloss.Color("255")).Background(lipgloss.Color("236")).Bold(true)
	fieldDisabled := lipgloss.NewStyle().Foreground(lipgloss.Color("238")).Italic(true)
	hintStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Italic(true)
	btnActive := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(lipgloss.Color("82"))
	btnInactive := lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Background(lipgloss.Color("238"))
	sb.WriteString(border.Render("╭" + strings.Repeat("─", width-2) + "╮") + "\n")
	title := " Configure postures "
	sb.WriteString(border.Render("│"))
	sb.WriteString(titleStyle.Render(lipgloss.PlaceHorizontal(innerW, lipgloss.Center, title)))
	sb.WriteString(border.Render("│") + "\n")
	sb.WriteString(border.Render("│ "))
	tabRow := ""
	for _, p := range postureOrder {
		isActive := p == m.modeSetupModalTab
		lbl := strings.ToUpper(p)
		if isActive {
			tabRow += tabActive.Render(" "+lbl+" ") + " "
		} else {
			tabRow += tabInactive.Render(" "+lbl+" ") + " "
		}
	}
	tabRowW := lipgloss.Width(tabRow)
	if tabRowW < innerW-2 {
		tabRow += strings.Repeat(" ", innerW-2-tabRowW)
	}
	sb.WriteString(tabRow)
	sb.WriteString(border.Render(" │") + "\n")
	sb.WriteString(border.Render("├" + strings.Repeat("─", width-2) + "┤") + "\n")
	prof := m.modeSetupModalDraft.Profiles[m.modeSetupModalTab]
	inferredProv := prof.Provider
	if inferredProv == "" && prof.Model != "" {
		if pv := providerForModel(m.providers, prof.Model); pv != "" {
			inferredProv = pv + " (auto)"
		}
	}
	if inferredProv == "" {
		inferredProv = "(inherit)"
	}
	fields := []struct {
		label    string
		value    string
		disabled bool
		focusIdx int
	}{
		{"Model", orDash(prof.Model), false, modalFocusModel},
		{"Reasoning", orDash(prof.ReasoningEffort), !m.isReasoningEnabled(), modalFocusReasoning},
		{"YOLO", yoloLabel(prof.Yolo), false, modalFocusYolo},
	}
	for _, f := range fields {
		sb.WriteString(border.Render("│ "))
		lbl := fmt.Sprintf("%-10s", f.label+":")
		sb.WriteString(fieldLabel.Render(lbl))
		valStr := f.value
		if f.disabled {
			valStr = "(select model first)"
		}
		display := fmt.Sprintf(" %-20s", valStr)
		var styled string
		if f.disabled {
			styled = fieldDisabled.Render(display)
		} else if m.modeSetupModalFocus == f.focusIdx && !m.modeSetupModalPickerOpen {
			styled = fieldValueFocus.Render(display)
		} else {
			styled = fieldValue.Render(display)
		}
		if m.modeSetupModalFocus == f.focusIdx && !m.modeSetupModalPickerOpen {
			styled = "▶" + styled
		} else {
			styled = " " + styled
		}
		sb.WriteString(styled)
		used := 1 + lipgloss.Width(lbl) + lipgloss.Width(valStr) + 4
		if used < innerW {
			sb.WriteString(strings.Repeat(" ", innerW-used))
		}
		sb.WriteString(border.Render(" │") + "\n")
	}
	sb.WriteString(border.Render("│ "))
	provLine := fmt.Sprintf("Provider: %s", inferredProv)
	sb.WriteString(hintStyle.Render(provLine))
	padding := innerW - lipgloss.Width(provLine) - 1
	if padding > 0 {
		sb.WriteString(strings.Repeat(" ", padding))
	}
	sb.WriteString(border.Render(" │") + "\n")
	if m.modeSetupModalPickerOpen {
		sb.WriteString(border.Render("├" + strings.Repeat("─", width-2) + "┤") + "\n")
		opts := m.modalPickerOptions()
		limit := 6
		start, end := 0, len(opts)
		if len(opts) > limit {
			sel := m.modeSetupModalPickerIdx
			start = sel - limit/2
			if start < 0 {
				start = 0
			}
			end = start + limit
			if end > len(opts) {
				end = len(opts)
				start = end - limit
			}
		}
		for i := start; i < end; i++ {
			sb.WriteString(border.Render("│ "))
			marker := "  "
			style := fieldValue
			if i == m.modeSetupModalPickerIdx {
				marker = "▸ "
				style = fieldValueFocus
			}
			line := marker + opts[i]
			if lipgloss.Width(line) > innerW-2 {
				line = line[:innerW-5] + "…"
			}
			sb.WriteString(style.Render(line))
			rem := innerW - 2 - lipgloss.Width(line)
			if rem > 0 {
				sb.WriteString(strings.Repeat(" ", rem))
			}
			sb.WriteString(border.Render(" │") + "\n")
		}
		if len(opts) == 0 {
			sb.WriteString(border.Render("│ "))
			sb.WriteString(hintStyle.Render("  (no options)"))
			sb.WriteString(strings.Repeat(" ", innerW-2-lipgloss.Width("  (no options)")))
			sb.WriteString(border.Render(" │") + "\n")
		}
	}
	sb.WriteString(border.Render("├" + strings.Repeat("─", width-2) + "┤") + "\n")
	sb.WriteString(border.Render("│ "))
	saveLbl := " Save "
	cancelLbl := " Cancel "
	var saveStyle, cancelStyle lipgloss.Style
	if m.modeSetupModalFocus == modalFocusSave && !m.modeSetupModalPickerOpen {
		saveStyle = btnActive
	} else {
		saveStyle = btnInactive
	}
	if m.modeSetupModalFocus == modalFocusCancel && !m.modeSetupModalPickerOpen {
		cancelStyle = btnActive
	} else {
		cancelStyle = btnInactive
	}
	btnRow := saveStyle.Render(saveLbl) + " " + cancelStyle.Render(cancelLbl)
	sb.WriteString(btnRow)
	hint := " Enter=save  Esc=cancel  ←→ tabs  1/2/3"
	remaining := innerW - lipgloss.Width(saveLbl) - lipgloss.Width(cancelLbl) - 3
	if remaining > lipgloss.Width(hint) {
		sb.WriteString(strings.Repeat(" ", remaining-lipgloss.Width(hint)))
		sb.WriteString(hintStyle.Render(hint))
	}
	usedBtn := lipgloss.Width(saveLbl) + lipgloss.Width(cancelLbl) + 1
	if remaining > 0 && remaining > lipgloss.Width(hint) {
		usedBtn += remaining
	}
	if innerW > usedBtn+1 {
		sb.WriteString(strings.Repeat(" ", innerW-usedBtn-1))
	}
	sb.WriteString(border.Render(" │") + "\n")
	sb.WriteString(border.Render("│ "))
	legend := "YOLO independent · Reasoning needs model"
	sb.WriteString(hintStyle.Render(legend))
	sb.WriteString(strings.Repeat(" ", innerW-1-lipgloss.Width(legend)))
	sb.WriteString(border.Render(" │") + "\n")
	sb.WriteString(border.Render("╰" + strings.Repeat("─", width-2) + "╯"))
	return sb.String()
}
