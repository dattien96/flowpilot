package app

import (
	"path/filepath"
	"testing"

	"flowpilot-runner/internal/tui/client"
)

// F-1 regression: the first-load onboarding chain must open the Supabase Setup
// modal whenever supabase is unconfigured — even when the offline fake catalog
// reports preloaded projects (len(m.projects) > 0), which previously made the
// `!SupabaseConfigured && len(projects) == 0` guard unreachable.
func TestMaybeAutoOnboardSetupTakesPrecedenceOverLogin(t *testing.T) {
	m := newTestAppModel(t)
	m.authNeedLogin = true
	m.projects = []client.Project{{ID: "proj-web", Name: "Fake"}} // fake catalog shape

	m.maybeAutoOnboard(false)

	if !m.supabaseSetupModalOpen {
		t.Fatal("supabase setup modal should open when supabase is unconfigured")
	}
	if m.loginModalOpen {
		t.Fatal("login modal must NOT open while supabase is unconfigured (setup takes precedence)")
	}
	if m.projectWizardOpen {
		t.Fatal("project wizard must not open while supabase is unconfigured")
	}
}

// F-1 guard: an established session that already has a bound project must not
// be interrupted by the setup modal (CA-633 typing/unlock contract — locked by
// the pre-existing TestTypingAfterSessionDefaultsGoesToInput).
func TestMaybeAutoOnboardSkipsSetupWhenProjectBound(t *testing.T) {
	m := newTestAppModel(t)
	m.project = &client.Project{ID: "p1", Name: "Bound"}

	m.maybeAutoOnboard(false)

	if m.hasModalOpen() {
		t.Fatal("no modal may interrupt a session that already has a bound project")
	}
}

func TestMaybeAutoOnboardLoginWhenUnauthenticated(t *testing.T) {
	m := newTestAppModel(t)
	m.authNeedLogin = true
	m.project = &client.Project{ID: "p1", Name: "Bound"}

	m.maybeAutoOnboard(true)

	if !m.loginModalOpen {
		t.Fatal("login modal should open when supabase configured but unauthenticated")
	}
	if m.supabaseSetupModalOpen {
		t.Fatal("supabase setup modal must not open when supabase is configured")
	}
}

func TestMaybeAutoOnboardWizardWhenProjectUnbound(t *testing.T) {
	m := newTestAppModel(t)
	m.authNeedLogin = false
	m.project = nil
	m.cfg.ProjectPath = filepath.Join(t.TempDir(), "unbound")

	m.maybeAutoOnboard(true)

	if !m.projectWizardOpen {
		t.Fatal("project wizard should open when authenticated but project path is unbound")
	}
	if m.loginModalOpen || m.supabaseSetupModalOpen {
		t.Fatal("no other modal should open on the wizard step")
	}
}

func TestMaybeAutoOnboardNoopWhenEverythingReady(t *testing.T) {
	m := newTestAppModel(t)
	m.authNeedLogin = false
	m.project = &client.Project{ID: "p1", Name: "Bound"}

	m.maybeAutoOnboard(true)

	if m.hasModalOpen() {
		t.Fatal("no modal should open when authenticated and project is bound")
	}
}

func TestMaybeAutoOnboardIdempotentWhenModalAlreadyOpen(t *testing.T) {
	m := newTestAppModel(t)
	m.openLoginModal()
	m.authNeedLogin = true

	m.maybeAutoOnboard(true)

	if !m.loginModalOpen {
		t.Fatal("an already-open modal must stay open (no re-open reset)")
	}
}

// F-1 integration: SessionDefaultsMsg on cold start with supabase unconfigured
// must end with the Supabase Setup modal open, regardless of catalog contents.
func TestSessionDefaultsFirstLoadOpensSetupWhenUnconfigured(t *testing.T) {
	m := newTestAppModel(t)
	m.sessionDefaultsLoaded = false
	m.cfg.ProjectPath = filepath.Join(t.TempDir(), "proj")

	_, _ = m.Update(SessionDefaultsMsg{
		Projects: []client.Project{
			{ID: "proj-web", Name: "Acme Web"},
			{ID: "proj-android", Name: "Acme Android"},
		},
		SupabaseConfigured: false,
	})

	if !m.supabaseSetupModalOpen {
		t.Fatal("first load with unconfigured supabase must open the setup modal (fake catalog projects must not block it)")
	}
}

// F-2b: after a first-run Supabase save the follow-up (non-firstLoad)
// SessionDefaultsMsg must re-arm the chain — here login — and consume the flag.
func TestSessionDefaultsSupabaseJustConfiguredRearmsChain(t *testing.T) {
	m := newTestAppModel(t)
	m.sessionDefaultsLoaded = true
	m.supabaseJustConfigured = true
	m.authNeedLogin = true
	m.project = nil

	m.handleOnboardingAfterSession(SessionDefaultsMsg{SupabaseConfigured: true}, false)

	if !m.loginModalOpen {
		t.Fatal("login modal should re-arm after supabase save")
	}
	if m.supabaseJustConfigured {
		t.Fatal("supabaseJustConfigured flag must be consumed after one re-arm")
	}
}

// F-2b negative: a plain non-firstLoad SessionDefaultsMsg (background refresh)
// must NOT auto-open modals — only the post-save re-arm path does.
func TestSessionDefaultsRefreshDoesNotOpenModals(t *testing.T) {
	m := newTestAppModel(t)
	m.sessionDefaultsLoaded = true
	m.supabaseJustConfigured = false
	m.authNeedLogin = true

	m.handleOnboardingAfterSession(SessionDefaultsMsg{SupabaseConfigured: true}, false)

	if m.hasModalOpen() {
		t.Fatal("background refresh must not auto-open any modal")
	}
}

// F-2a: after a successful login the D-2 chain continues to the project wizard
// when the workspace path is still unbound.
func TestLoginResultOpensProjectWizardWhenUnbound(t *testing.T) {
	m := newTestAppModel(t)
	m.project = nil
	m.cfg.ProjectPath = filepath.Join(t.TempDir(), "unbound")

	_, _ = m.Update(LoginResultMsg{Email: "dev@flowpilot.ai"})

	if !m.projectWizardOpen {
		t.Fatal("project wizard should open after login when project path is unbound")
	}
}

func TestLoginResultNoWizardWhenProjectBound(t *testing.T) {
	m := newTestAppModel(t)
	m.project = &client.Project{ID: "p1", Name: "Bound"}

	_, _ = m.Update(LoginResultMsg{Email: "dev@flowpilot.ai"})

	if m.projectWizardOpen {
		t.Fatal("project wizard must not open when a project is already bound")
	}
}
