package app

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/config"
)

func newTestAppModel(t *testing.T) *AppModel {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	m := New(config.ChatConfig{Provider: "codex"}, srv.URL)
	m.mode = ModeChat
	m.width = 100
	m.height = 30
	return m
}

func TestProjectPlatformDetection(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Golang
	goDir := filepath.Join(tempDir, "go-app")
	_ = os.MkdirAll(goDir, 0755)
	_ = os.WriteFile(filepath.Join(goDir, "go.mod"), []byte("module test"), 0644)
	if p := detectProjectPlatform(goDir); p != "golang" {
		t.Fatalf("detectProjectPlatform = %q, want 'golang'", p)
	}

	// 2. Android
	androidDir := filepath.Join(tempDir, "android-app")
	_ = os.MkdirAll(androidDir, 0755)
	_ = os.WriteFile(filepath.Join(androidDir, "build.gradle.kts"), []byte("plugins {}"), 0644)
	if p := detectProjectPlatform(androidDir); p != "android" {
		t.Fatalf("detectProjectPlatform = %q, want 'android'", p)
	}

	// 3. Next.js
	nextDir := filepath.Join(tempDir, "next-app")
	_ = os.MkdirAll(nextDir, 0755)
	_ = os.WriteFile(filepath.Join(nextDir, "package.json"), []byte(`{"dependencies": {"next": "14.0.0"}}`), 0644)
	if p := detectProjectPlatform(nextDir); p != "nextjs" {
		t.Fatalf("detectProjectPlatform = %q, want 'nextjs'", p)
	}

	// 4. React.js
	reactDir := filepath.Join(tempDir, "react-app")
	_ = os.MkdirAll(reactDir, 0755)
	_ = os.WriteFile(filepath.Join(reactDir, "package.json"), []byte(`{"dependencies": {"react": "18.0.0"}}`), 0644)
	if p := detectProjectPlatform(reactDir); p != "reactjs" {
		t.Fatalf("detectProjectPlatform = %q, want 'reactjs'", p)
	}

	// 5. Python
	pyDir := filepath.Join(tempDir, "py-app")
	_ = os.MkdirAll(pyDir, 0755)
	_ = os.WriteFile(filepath.Join(pyDir, "requirements.txt"), []byte("fastapi"), 0644)
	if p := detectProjectPlatform(pyDir); p != "python" {
		t.Fatalf("detectProjectPlatform = %q, want 'python'", p)
	}

	// 6. Rust
	rustDir := filepath.Join(tempDir, "rust-app")
	_ = os.MkdirAll(rustDir, 0755)
	_ = os.WriteFile(filepath.Join(rustDir, "Cargo.toml"), []byte("[package]"), 0644)
	if p := detectProjectPlatform(rustDir); p != "rust" {
		t.Fatalf("detectProjectPlatform = %q, want 'rust'", p)
	}
}

func TestProjectWizardNavigationAndCycle(t *testing.T) {
	m := newTestAppModel(t)
	m.openProjectWizard(filepath.Join(t.TempDir(), "test-project"))

	if !m.projectWizardOpen {
		t.Fatal("projectWizardOpen should be true")
	}
	if !m.hasModalOpen() {
		t.Fatal("hasModalOpen should return true")
	}
	if m.projectWizardName != "test-project" {
		t.Fatalf("projectWizardName = %q, want 'test-project'", m.projectWizardName)
	}

	// Tab advances field: Name (0) -> Platform (1)
	m.handleProjectWizardKey(tea.KeyMsg{Type: tea.KeyTab})
	if m.projectWizardField != wizardFieldPlatform {
		t.Fatalf("field = %d, want %d (Platform)", m.projectWizardField, wizardFieldPlatform)
	}

	// Right key cycles platform
	initialPlatform := m.projectWizardPlatform
	m.handleProjectWizardKey(tea.KeyMsg{Type: tea.KeyRight})
	if m.projectWizardPlatform == initialPlatform {
		t.Fatal("Right arrow should cycle platform to a different option")
	}

	// Tab to Model (2)
	m.handleProjectWizardKey(tea.KeyMsg{Type: tea.KeyTab})
	if m.projectWizardField != wizardFieldModel {
		t.Fatalf("field = %d, want %d (Model)", m.projectWizardField, wizardFieldModel)
	}

	// Tab to Submit (3)
	m.handleProjectWizardKey(tea.KeyMsg{Type: tea.KeyTab})
	if m.projectWizardField != wizardFieldSubmit {
		t.Fatalf("field = %d, want %d (Submit)", m.projectWizardField, wizardFieldSubmit)
	}

	// Render modal and check title
	rendered := m.renderProjectWizard(m.width)
	if !strings.Contains(rendered, "Onboard Project") {
		t.Fatalf("rendered wizard missing title: %s", rendered)
	}

	// Esc closes wizard
	m.handleProjectWizardKey(tea.KeyMsg{Type: tea.KeyEsc})
	if m.projectWizardOpen {
		t.Fatal("Esc should close wizard")
	}
}

func TestProjectWizardPlatformOptionsCoverSkillPackGroups(t *testing.T) {
	// Every platform that owns an embedded flow-pack group must be selectable
	// in the wizard; a missing option silently installs common-only skills.
	for _, p := range []string{
		"android", "angularjs", "flutter", "golang", "ios", "java",
		"kmm", "nodejs", "python", "react-native", "reactjs", "vuejs",
	} {
		found := false
		for _, opt := range wizardPlatformOptions {
			if opt == p {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("wizardPlatformOptions missing skill-pack platform %q", p)
		}
	}
}

func TestLoginModalNavigationAndValidation(t *testing.T) {
	m := newTestAppModel(t)
	m.openLoginModal()

	if !m.loginModalOpen {
		t.Fatal("loginModalOpen should be true")
	}
	if !m.hasModalOpen() {
		t.Fatal("hasModalOpen should return true")
	}

	// Type email
	m.handleLoginModalKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("test@flowpilot.ai")})
	if m.loginModalEmail != "test@flowpilot.ai" {
		t.Fatalf("loginModalEmail = %q, want 'test@flowpilot.ai'", m.loginModalEmail)
	}

	// Enter moves to Password field
	m.handleLoginModalKey(tea.KeyMsg{Type: tea.KeyEnter})
	if m.loginModalField != loginFieldPassword {
		t.Fatalf("field = %d, want %d (Password)", m.loginModalField, loginFieldPassword)
	}

	// Render modal and check title
	rendered := m.renderLoginModal(m.width)
	if !strings.Contains(rendered, "Supabase Sign In") {
		t.Fatalf("rendered login missing title: %s", rendered)
	}

	// Tab to Submit without password -> Enter -> should show error
	m.handleLoginModalKey(tea.KeyMsg{Type: tea.KeyTab})
	if m.loginModalField != loginFieldSubmit {
		t.Fatalf("field = %d, want %d (Submit)", m.loginModalField, loginFieldSubmit)
	}
	m.handleLoginModalKey(tea.KeyMsg{Type: tea.KeyEnter})
	if m.loginModalErr == "" {
		t.Fatal("Submit without password should set loginModalErr")
	}

	// Esc closes modal
	m.handleLoginModalKey(tea.KeyMsg{Type: tea.KeyEsc})
	if m.loginModalOpen {
		t.Fatal("Esc should close login modal")
	}
}

func TestSupabaseSetupModalNavigationAndValidation(t *testing.T) {
	m := newTestAppModel(t)
	m.openSupabaseSetupModal()

	if !m.supabaseSetupModalOpen {
		t.Fatal("supabaseSetupModalOpen should be true")
	}
	if !m.hasModalOpen() {
		t.Fatal("hasModalOpen should return true")
	}

	// Invalid URL validation test
	m.supabaseSetupModalURL = "invalid-url"
	m.supabaseSetupModalAnonKey = "dummy-key"
	m.supabaseSetupModalField = supabaseFieldSave
	m.handleSupabaseSetupModalKey(tea.KeyMsg{Type: tea.KeyEnter})
	if !strings.Contains(m.supabaseSetupModalErr, "http") {
		t.Fatalf("expected URL error, got %q", m.supabaseSetupModalErr)
	}

	// Valid URL, missing anon key
	m.supabaseSetupModalURL = "https://example.supabase.co"
	m.supabaseSetupModalAnonKey = ""
	m.handleSupabaseSetupModalKey(tea.KeyMsg{Type: tea.KeyEnter})
	if !strings.Contains(m.supabaseSetupModalErr, "Anon Key") {
		t.Fatalf("expected Anon Key error, got %q", m.supabaseSetupModalErr)
	}

	// Render modal and check title
	rendered := m.renderSupabaseSetupModal(m.width)
	if !strings.Contains(rendered, "Supabase Workspace Setup") {
		t.Fatalf("rendered supabase setup missing title: %s", rendered)
	}

	// Esc closes modal
	m.handleSupabaseSetupModalKey(tea.KeyMsg{Type: tea.KeyEsc})
	if m.supabaseSetupModalOpen {
		t.Fatal("Esc should close supabase setup modal")
	}
}
