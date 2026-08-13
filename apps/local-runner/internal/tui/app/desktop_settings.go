package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/desktopboot"
)

// DesktopEnsureMsg carries /settings ensure-desktop results.
type DesktopEnsureMsg struct {
	URL      string
	Launched bool
	Reused   bool
	Err      string
	AuthSync string // path synced, or empty
	AuthErr  string
}

// ProviderConnectMsg carries /provider connect results.
type ProviderConnectMsg struct {
	ProviderKey string
	Err         string
}

// ProviderInstallMsg carries /provider install results.
type ProviderInstallMsg struct {
	ProviderKey string
	Providers   []client.Provider
	Err         string
}

func settingsBridgeBlurb(url string) string {
	var sb strings.Builder
	sb.WriteString("Settings live in the Desktop app (providers, projects, workflows, MCP).\n")
	sb.WriteString("Continue there after Desktop is up — this TUI does not host a settings page.\n")
	if strings.TrimSpace(url) != "" {
		sb.WriteString(fmt.Sprintf("Desktop URL: %s", url))
	}
	return sb.String()
}

func (m *AppModel) cmdEnsureDesktop() tea.Cmd {
	cfg := desktopboot.Config{
		Workspace: m.cfg.RunnerWorkspace,
		Host:      m.cfg.DesktopHost,
		Port:      m.cfg.DesktopPort,
		RunnerURL: m.runnerURL,
	}
	return func() tea.Msg {
		// Mirror TUI Supabase session into every Electron userData candidate
		// before Desktop boots (fixes Login screen after TUI /login).
		msg := DesktopEnsureMsg{}
		if path, err := client.SyncDesktopAuthSession(); err != nil {
			msg.AuthErr = err.Error()
		} else {
			msg.AuthSync = path
		}
		res, err := desktopboot.EnsureDesktop(cfg)
		msg.URL = res.URL
		if err != nil {
			msg.Err = err.Error()
			return msg
		}
		msg.Launched = res.Launched
		msg.Reused = res.Reused
		return msg
	}
}

func (m *AppModel) cmdConnectProvider(providerKey string) tea.Cmd {
	runnerURL := m.runnerURL
	key := strings.TrimSpace(providerKey)
	return func() tea.Msg {
		cl := client.New(runnerURL)
		err := cl.ConnectProviderAccount(context.Background(), key)
		if err != nil {
			return ProviderConnectMsg{ProviderKey: key, Err: err.Error()}
		}
		return ProviderConnectMsg{ProviderKey: key}
	}
}

func (m *AppModel) cmdInstallProvider(providerKey string) tea.Cmd {
	runnerURL := m.runnerURL
	key := strings.TrimSpace(providerKey)
	return func() tea.Msg {
		cl := client.New(runnerURL)
		providers, err := cl.InstallProvider(context.Background(), key)
		if err != nil {
			return ProviderInstallMsg{ProviderKey: key, Providers: providers, Err: err.Error()}
		}
		return ProviderInstallMsg{ProviderKey: key, Providers: providers}
	}
}

type ActivatedAccountMsg struct {
	Account *client.ProviderAccountSummary
	Err     error
}

func (m *AppModel) cmdActivateAccount(accountID string) tea.Cmd {
	runnerURL := m.runnerURL
	id := strings.TrimSpace(accountID)
	return func() tea.Msg {
		cl := client.New(runnerURL)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		acc, err := cl.ActivateProviderAccount(ctx, id)
		return ActivatedAccountMsg{Account: acc, Err: err}
	}
}

func findProvider(providers []client.Provider, key string) *client.Provider {
	for i := range providers {
		if strings.EqualFold(providers[i].Key, key) {
			return &providers[i]
		}
	}
	return nil
}
