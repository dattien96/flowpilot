package app

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func lspSidebarModel(t *testing.T) *AppModel {
	t.Helper()
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:9")
	m.authPhase = AuthNone
	m.sessionDefaultsLoaded = true
	m.width = 160
	m.fullWidth = 160
	m.height = 30
	m.mode = ModeChat
	m.sessionPanel.RunnerURL = "http://127.0.0.1:9"
	return m
}

func TestLSPSidebar_ShowsWarningWhenBinaryMissing(t *testing.T) {
	m := lspSidebarModel(t)
	m.lspStatus = &client.LSPStatus{
		Platform: "golang", Binary: "gopls",
		Installed: false, Warn: true,
		InstallHint: "go install golang.org/x/tools/gopls@latest",
	}
	lines := m.renderRightSidebar(m.height)
	joined := strings.Join(lines, "\n")
	plain := stripANSI(joined)
	if !strings.Contains(plain, "gopls") {
		t.Fatalf("sidebar must name the missing binary:\n%s", plain)
	}
	if !strings.Contains(plain, "go install") {
		t.Fatalf("sidebar must show the install hint:\n%s", plain)
	}
	// Compact by contract: at most 2 warning rows, never viewport-breaking.
	count := 0
	for _, l := range strings.Split(plain, "\n") {
		if strings.Contains(l, "gopls") || strings.Contains(l, "go install") {
			count++
		}
	}
	if count == 0 || count > 2 {
		t.Fatalf("warning must occupy 1-2 rows, got %d:\n%s", count, plain)
	}
}

func TestLSPSidebar_HiddenWithoutStatus(t *testing.T) {
	m := lspSidebarModel(t)
	plain := stripANSI(strings.Join(m.renderRightSidebar(m.height), "\n"))
	if strings.Contains(plain, "lsp:") {
		t.Fatalf("sidebar must not show LSP rows without status:\n%s", plain)
	}
}

func TestLSPSidebar_HiddenWhenInstalled(t *testing.T) {
	m := lspSidebarModel(t)
	m.lspStatus = &client.LSPStatus{Platform: "golang", Binary: "gopls", Installed: true}
	plain := stripANSI(strings.Join(m.renderRightSidebar(m.height), "\n"))
	if strings.Contains(plain, "lsp:") {
		t.Fatalf("sidebar must not warn when the server is installed:\n%s", plain)
	}
}

func TestLSPSidebar_TruncatesLongHint(t *testing.T) {
	m := lspSidebarModel(t)
	m.width = 120
	m.fullWidth = 120
	m.lspStatus = &client.LSPStatus{
		Platform: "android", Binary: "kotlin-language-server",
		Installed: false, Warn: true,
		InstallHint: "Download from fwcd/kotlin-language-server releases page and unzip somewhere on PATH please",
	}
	w := m.sideWidth()
	for _, l := range m.renderRightSidebar(30) {
		if got := lipgloss.Width(stripANSI(l)); got > w {
			t.Fatalf("sidebar row wider than %d: %q", w, l)
		}
	}
}

func TestLSPStatusMsg_SetsAndDropsStale(t *testing.T) {
	m := lspSidebarModel(t)
	m.lspStatusPath = "/proj/a"
	updated, _ := m.Update(LSPStatusMsg{Path: "/proj/a", Status: &client.LSPStatus{Binary: "gopls", Warn: true}})
	if updated.(*AppModel).lspStatus == nil {
		t.Fatal("matching LSPStatusMsg must set state")
	}
	updated2, _ := m.Update(LSPStatusMsg{Path: "/proj/b", Status: &client.LSPStatus{Binary: "vtsls", Warn: true}})
	if got := updated2.(*AppModel).lspStatus; got == nil || got.Binary != "gopls" {
		t.Fatalf("stale response must not overwrite fresh state, got %+v", got)
	}
}

func TestSessionDefaults_QueuesLSPStatusFetch(t *testing.T) {
	m := lspSidebarModel(t)
	m.sessionDefaultsLoaded = false
	m.cfg.ProjectPath = "/proj/a"
	proj := &client.Project{ID: "p1", Name: "P", Path: "/proj/a"}
	updated, _ := m.Update(SessionDefaultsMsg{Projects: []client.Project{*proj}, Project: proj, SupabaseConfigured: true})
	am := updated.(*AppModel)
	if am.lspStatusPath != "/proj/a" {
		t.Fatalf("lspStatusPath = %q, want project path", am.lspStatusPath)
	}
	if am.lspStatus != nil {
		t.Fatal("lspStatus must reset while the fetch is in flight")
	}
}
