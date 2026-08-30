package app

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
	"flowpilot-runner/internal/tui/prefs"
)

// CA-685 (operator decision): chat postures gain a fourth value "non" — the
// no-mode posture. Tab cycles Plan → Code → Non → Plan; in "non" NO pins are
// applied, so the session keeps the user's last provider/model choice across
// restarts. Scan/plan/code keep the strict pin semantics (CA-685: the pin
// always re-applies on restore). Chat-only — flow mode never touches postures.
// Additive tests; the rewritten CA-679-era file now encodes the same spec.

func nonPostureServer(t *testing.T, active string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/client/chat-posture" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"active":"` + active + `","profiles":{"scan":{},"plan":{},"code":{}}}`))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestTabCycleIncludesNon(t *testing.T) {
	srv := nonPostureServer(t, "code")
	m := New(config.ChatConfig{}, srv.URL)
	m.mode = ModeChat
	m.sessionDefaultsLoaded = true
	m.chatPosture = "code"
	m.providers = nil

	// code → non
	m.chatPosturePending = "apply:code"
	if next := tabPostureNext(m.activePosture()); next != "non" {
		t.Fatalf("Tab from code must reach non, got %q", next)
	}
	// non → plan
	if next := tabPostureNext("non"); next != "plan" {
		t.Fatalf("Tab from non must reach plan, got %q", next)
	}
	// plan → code
	if next := tabPostureNext("plan"); next != "code" {
		t.Fatalf("Tab from plan must reach code, got %q", next)
	}
	// scan is NOT in the Tab cycle (opt-in only, unchanged)
	if next := tabPostureNext("scan"); next != "plan" {
		t.Fatalf("Tab from scan (out of cycle) must default to plan, got %q", next)
	}
}

// tabPostureNext mirrors the inline Tab-handler lookup (app.go) so the cycle
// stays covered by tests.
func tabPostureNext(current string) string {
	for i, p := range tabPostureOrder {
		if p == current {
			return tabPostureOrder[(i+1)%len(tabPostureOrder)]
		}
	}
	return tabPostureOrder[0]
}

func TestApplyNonPostureAppliesNoPins(t *testing.T) {
	srv := nonPostureServer(t, "code")
	m := New(config.ChatConfig{Provider: "opencode"}, srv.URL)
	m.mode = ModeChat
	m.provider = "opencode"
	m.model = "opencode-go/muse-spark-1.2-contributor"
	m.reasoningEffort = "low"

	cmd := m.cmdLoadChatPosture()
	cp := cmd().(chatPostureMsg)
	m.chatPosturePending = "apply:non"
	m.chatPostureCmdFromPending(cp.Cfg)

	if m.chatPosture != "non" {
		t.Fatalf("posture = %q, want non", m.chatPosture)
	}
	if m.provider != "opencode" || m.model != "opencode-go/muse-spark-1.2-contributor" || m.reasoningEffort != "low" {
		t.Fatalf("non must apply no pins, got provider=%q model=%q reasoning=%q", m.provider, m.model, m.reasoningEffort)
	}
	if !m.chatPostureDirty {
		t.Fatal("apply must persist the new active posture back to the runner")
	}
}

func TestRestoreNonKeepsUserModelAcrossRestart(t *testing.T) {
	srv := nonPostureServer(t, "non")

	prefFile := filepath.Join(t.TempDir(), "tui-session.json")
	t.Setenv("FLOWPILOT_TUI_SESSION_FILE", prefFile)

	m := New(config.ChatConfig{Provider: "opencode"}, srv.URL)
	m.mode = ModeChat
	m.provider = "opencode"
	m.model = "opencode-go/muse-spark-1.2-contributor"
	m.persistSessionPrefs()

	m.chatPosturePending = "restore"
	cmd := m.cmdLoadChatPosture()
	cp := cmd().(chatPostureMsg)
	m.chatPostureCmdFromPending(cp.Cfg)

	if m.chatPosture != "non" {
		t.Fatalf("posture after restore = %q, want non", m.chatPosture)
	}
	if m.model != "opencode-go/muse-spark-1.2-contributor" {
		t.Fatalf("non restore must keep the user's model, got %q", m.model)
	}
	saved, _, err := prefs.Load()
	if err != nil {
		t.Fatalf("load prefs: %v", err)
	}
	if saved.Model != "opencode-go/muse-spark-1.2-contributor" {
		t.Fatalf("non restore must keep the persisted model, got %q", saved.Model)
	}
}

func TestRestoreNonIgnoresStaleProfilePins(t *testing.T) {
	// Even when the doc still carries pins for scan/plan/code, active=non
	// applies nothing.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/client/chat-posture" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"active":"non","profiles":{"scan":{"model":"grok-4.5"},"plan":{"model":"grok-4.5"},"code":{"model":"grok-4.5"}}}`))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	m := New(config.ChatConfig{Provider: "opencode"}, srv.URL)
	m.mode = ModeChat
	m.provider = "opencode"
	m.model = "opencode-go/deepseek-v4-flash"

	m.chatPosturePending = "restore"
	cmd := m.cmdLoadChatPosture()
	cp := cmd().(chatPostureMsg)
	m.chatPostureCmdFromPending(cp.Cfg)

	if m.model != "opencode-go/deepseek-v4-flash" {
		t.Fatalf("active=non must ignore stale pins, got model %q", m.model)
	}
}

func TestChatFrameTitleNonShowsBareChat(t *testing.T) {
	// CA-685: the no-mode default renders just "Chat:" — no "Chat: Non".
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.mode = ModeChat
	m.chatPosture = "non"
	nonTitle := stripANSI(m.chatFrameTitle())
	if !strings.HasPrefix(nonTitle, "Chat:") || strings.Contains(nonTitle, "Non") {
		t.Fatalf("non posture title = %q, want prefix %q without Non", nonTitle, "Chat:")
	}
	m.chatPosture = "code"
	if got := stripANSI(m.chatFrameTitle()); !strings.HasPrefix(got, "Chat: Code") {
		t.Fatalf("code posture title = %q, want prefix %q", got, "Chat: Code")
	}
	m.chatPosture = "scan"
	if got := stripANSI(m.chatFrameTitle()); !strings.HasPrefix(got, "Chat: Scan") {
		t.Fatalf("scan posture title = %q, want prefix %q", got, "Chat: Scan")
	}
}

func TestDefaultPostureIsNon(t *testing.T) {
	// CA-685: an unset posture resolves to "non" (the no-mode default).
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	if m.activePosture() != "non" {
		t.Fatalf("default activePosture = %q, want non", m.activePosture())
	}
}

func TestRestoreAppliesModelAndReasoningPinsOpencodeXhigh(t *testing.T) {
	// Operator report (2026-08-29): the scan posture pins muse-spark + xhigh —
	// after restart the TUI must show reasoning xhigh, not the stale prefs value.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/client/chat-posture" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"active":"scan","profiles":{"scan":{"provider":"opencode","model":"opencode-go/muse-spark-1.2-contributor","reasoningEffort":"xhigh","yolo":true},"plan":{},"code":{}}}`))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	m := New(config.ChatConfig{Provider: "opencode"}, srv.URL)
	m.mode = ModeChat
	m.chatPosture = "code"
	m.provider = "opencode"
	m.model = "opencode/muse-spark-1.2-contributor-free"
	m.reasoningEffort = "medium"
	m.sessionDefaultsLoaded = false

	m.chatPosturePending = "restore"
	cmd := m.cmdLoadChatPosture()
	cp := cmd().(chatPostureMsg)
	m.chatPostureCmdFromPending(cp.Cfg)

	if m.model != "opencode-go/muse-spark-1.2-contributor" {
		t.Fatalf("pin model must load, got %q", m.model)
	}
	if m.reasoningEffort != "xhigh" {
		t.Fatalf("pin reasoning xhigh must load, got %q", m.reasoningEffort)
	}
}

func TestSlashReasoningAcceptsXhigh(t *testing.T) {
	// CA-685: /reasoning accepts the full vocabulary (xhigh was rejected before).
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m2, _ := m.handleSlashCommand("/reasoning xhigh")
	am := m2.(*AppModel)
	if am.reasoningEffort != "xhigh" {
		t.Fatalf("/reasoning xhigh rejected, got %q", am.reasoningEffort)
	}
	m3, _ := am.handleSlashCommand("/reasoning minimal")
	if m3.(*AppModel).reasoningEffort != "minimal" {
		t.Fatalf("/reasoning minimal rejected")
	}
	// Unknown values still fail with guidance.
	m4, _ := m3.(*AppModel).handleSlashCommand("/reasoning turbo")
	if m4.(*AppModel).reasoningEffort == "turbo" {
		t.Fatal("unknown effort must be rejected")
	}
}

func TestReasoningModelFirstValidation(t *testing.T) {
	// CA-686 operator note: not every model has xhigh (claude has max instead).
	// When the selected model advertises efforts, ONLY those are valid.
	m := New(config.ChatConfig{Provider: "claude"}, "http://127.0.0.1:4317")
	m.provider = "claude"
	m.model = "claude-opus-5"
	m.providers = []client.Provider{{
		Key: "claude",
		Models: []client.ProviderModel{{
			ID:                        "claude-opus-5",
			SupportedReasoningEfforts: []string{"low", "medium", "high", "max"},
		}},
	}}

	if !reasoningEffortAllowed(m, "max") {
		t.Fatal("claude max must be allowed")
	}
	if reasoningEffortAllowed(m, "xhigh") {
		t.Fatal("xhigh must be rejected for a model that does not advertise it")
	}
	if !reasoningEffortAllowed(m, "") {
		t.Fatal("empty (model default) must stay allowed")
	}

	// opencode advertises xhigh instead.
	m2 := New(config.ChatConfig{Provider: "opencode"}, "http://127.0.0.1:4317")
	m2.provider = "opencode"
	m2.model = "opencode-go/muse-spark-1.2-contributor"
	m2.providers = []client.Provider{{
		Key: "opencode",
		Models: []client.ProviderModel{{
			ID:                        "opencode-go/muse-spark-1.2-contributor",
			SupportedReasoningEfforts: []string{"minimal", "low", "medium", "high", "xhigh"},
		}},
	}}
	if !reasoningEffortAllowed(m2, "xhigh") {
		t.Fatal("opencode xhigh must be allowed")
	}
	if reasoningEffortAllowed(m2, "max") {
		t.Fatal("max must be rejected for opencode muse-spark")
	}

	// Picker lists exactly the model's efforts (Desktop Task-215 parity).
	items := filterReasoningSuggestions("/reasoning ", "high", m.modelReasoningEfforts())
	if len(items) != 4 {
		t.Fatalf("claude picker = %+v, want 4 model efforts", items)
	}
	for _, it := range items {
		if it.value == "xhigh" {
			t.Fatalf("claude picker must not offer xhigh: %+v", items)
		}
	}
}

func TestReasoningClampsToNewModelDefault(t *testing.T) {
	// CA-686 operator point: reasoning is dynamic per model. Switching from an
	// xhigh opencode model to a claude model without xhigh must land the
	// reasoning on the new model's catalog default — not keep xhigh.
	m := New(config.ChatConfig{Provider: "opencode"}, "http://127.0.0.1:4317")
	m.provider = "opencode"
	m.model = "opencode-go/muse-spark-1.2-contributor"
	m.reasoningEffort = "xhigh"
	m.providers = []client.Provider{
		{
			Key: "opencode",
			Models: []client.ProviderModel{{
				ID:                        "opencode-go/muse-spark-1.2-contributor",
				SupportedReasoningEfforts: []string{"minimal", "low", "medium", "high", "xhigh"},
			}},
		},
		{
			Key: "claude",
			Models: []client.ProviderModel{{
				ID:                        "claude-opus-5",
				SupportedReasoningEfforts: []string{"low", "medium", "high", "max"},
				DefaultReasoningEffort:    "medium",
			}},
		},
	}

	// Simulate /model claude-opus-5: model changes, effort goes stale.
	m.provider = "claude"
	m.model = "claude-opus-5"
	m.clampReasoningForCurrentModel()
	if m.reasoningEffort != "medium" {
		t.Fatalf("stale xhigh must clamp to the claude default medium, got %q", m.reasoningEffort)
	}

	// Consistent effort survives.
	m.reasoningEffort = "max"
	m.clampReasoningForCurrentModel()
	if m.reasoningEffort != "max" {
		t.Fatalf("supported effort must survive, got %q", m.reasoningEffort)
	}

	// Model with no default advertised: stale effort clears to model default.
	m.providers[1].Models[0].DefaultReasoningEffort = ""
	m.reasoningEffort = "xhigh"
	m.clampReasoningForCurrentModel()
	if m.reasoningEffort != "" {
		t.Fatalf("stale effort without a default must clear, got %q", m.reasoningEffort)
	}

	// No catalog data → no clamp (cannot know).
	m.providers = nil
	m.reasoningEffort = "xhigh"
	m.clampReasoningForCurrentModel()
	if m.reasoningEffort != "xhigh" {
		t.Fatalf("no catalog data must not clamp, got %q", m.reasoningEffort)
	}
}

func TestModelReasoningEffortsFromRealCatalogWire(t *testing.T) {
	// The exact /providers wire shape (grok-4.5 advertises high/medium/low —
	// matching the official grok CLI's Low/Medium/High list) must drive the
	// /reasoning picker, not the fallback vocabulary.
	wire := []client.Provider{{
		Key: "grok",
		Models: []client.ProviderModel{
			{ID: "grok-4.5", SupportedReasoningEfforts: []string{"high", "medium", "low"}},
			{ID: "grok-4.6", SupportedReasoningEfforts: []string{"xhigh", "high", "medium", "low"}},
		},
	}}
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.provider = "grok"
	m.model = "grok-4.5"
	m.providers = wire

	efforts := m.modelReasoningEfforts()
	if len(efforts) != 3 || efforts[0] != "high" {
		t.Fatalf("grok-4.5 efforts = %v, want [high medium low]", efforts)
	}
	items := filterReasoningSuggestions("/reasoning ", "medium", m.modelReasoningEfforts())
	if len(items) != 3 {
		t.Fatalf("grok-4.5 picker = %+v, want exactly 3 model efforts", items)
	}
	var mediumDetail string
	for _, it := range items {
		if it.value == "medium" {
			mediumDetail = it.detail
		}
		if it.value == "xhigh" || it.value == "minimal" {
			t.Fatalf("grok-4.5 picker must not offer %s", it.value)
		}
	}
	if !strings.Contains(mediumDetail, "Medium effort") || !strings.Contains(mediumDetail, "active") {
		t.Fatalf("active row must carry a human description + active marker, got %q", mediumDetail)
	}

	// Cross-provider fallback: provider key renamed in prefs but the model id
	// still resolves through another catalog entry.
	m.provider = "opencode"
	if efforts := m.modelReasoningEfforts(); len(efforts) != 3 {
		t.Fatalf("cross-provider model lookup = %v, want grok-4.5 efforts", efforts)
	}
}

func TestReasoningEffortDetailWording(t *testing.T) {
	if d := reasoningEffortDetail("xhigh"); !strings.Contains(d, "Extra high effort") {
		t.Fatalf("xhigh detail = %q", d)
	}
	if d := reasoningEffortDetail("low"); !strings.Contains(d, "quick, fast implementations") {
		t.Fatalf("low detail = %q", d)
	}
}

func TestProviderRefreshCommandDispatchesCatalogLoad(t *testing.T) {
	// CA-687: /provider refresh is the TUI's "Detect models" button (Desktop
	// parity) — it must re-fetch the provider catalog without switching
	// provider or starting a run.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/providers" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[{"key":"grok","installed":true,"models":[{"id":"grok-4.5","supported_reasoning_efforts":["high","medium","low"]},{"id":"grok-4.6","supported_reasoning_efforts":["xhigh","high","medium","low"]}]}]`))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	m := New(config.ChatConfig{Provider: "grok"}, srv.URL)
	m.provider = "grok"
	m.model = "grok-4.5"
	before := len(m.providers)

	m2, cmd := m.handleSlashCommand("/provider refresh")
	am := m2.(*AppModel)
	if am.provider != "grok" || am.model != "grok-4.5" {
		t.Fatalf("refresh must not change provider/model, got %q/%q", am.provider, am.model)
	}
	if cmd == nil {
		t.Fatal("refresh must dispatch a catalog load cmd")
	}
	msg := cmd()
	pm, ok := msg.(ProvidersCatalogMsg)
	if !ok || pm.Err != "" {
		t.Fatalf("expected ProvidersCatalogMsg, got %T (%+v)", msg, pm)
	}
	am2, _ := am.Update(pm)
	final := am2.(*AppModel)
	if len(final.providers) != 1 {
		t.Fatalf("catalog after refresh = %d providers, want 1 (grok)", len(final.providers))
	}
	var grok *client.Provider
	for i := range final.providers {
		if final.providers[i].Key == "grok" {
			grok = &final.providers[i]
		}
	}
	if grok == nil || len(grok.Models) != 2 {
		t.Fatalf("grok models after refresh = %+v", grok)
	}
	found46 := false
	for _, mod := range grok.Models {
		if mod.ModelID() == "grok-4.6" {
			found46 = true
		}
	}
	if !found46 {
		t.Fatal("grok-4.6 must appear after /provider refresh")
	}
	if before != 0 {
		t.Fatalf("fixture expectation: catalog started empty, got %d", before)
	}
}

func TestOpencodeVariantsFetchOnModelSelection(t *testing.T) {
	// CA-689c: picking an opencode model whose catalog efforts are still the
	// guess must immediately fetch the REAL list — no chat required.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/client/providers/opencode-variants" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"modelId":"opencode-go/hy3","supportedEfforts":["none","low","high"],"defaultReasoningEffort":"none"}`))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	m := New(config.ChatConfig{}, srv.URL)
	m.provider = "opencode"
	m.model = "opencode-go/hy3"
	m.providers = []client.Provider{{
		Key: "opencode",
		Models: []client.ProviderModel{{
			ID:                        "opencode-go/hy3",
			SupportedReasoningEfforts: []string{"minimal", "low", "medium", "high", "xhigh"}, // guess
		}},
	}}
	// Sanity: before selection the guess is the menu.
	if got := m.modelReasoningEfforts(); len(got) != 5 {
		t.Fatalf("pre-selection efforts = %v", got)
	}

	// Execute /model set → a fetch cmd must come back.
	m2, cmd := m.handleSlashCommand("/model opencode-go/hy3")
	am := m2.(*AppModel)
	if cmd == nil {
		t.Fatal("model selection must dispatch the variants fetch")
	}
	msg := cmd()
	vm, ok := msg.(opencodeVariantsMsg)
	if !ok || vm.Err != "" || vm.Model != "opencode-go/hy3" {
		t.Fatalf("expected opencodeVariantsMsg, got %T %+v", msg, vm)
	}

	// Applying the message updates the catalog entry in place and clamps.
	am2, _ := am.Update(vm)
	final := am2.(*AppModel)
	efforts := final.modelReasoningEfforts()
	if len(efforts) != 3 || efforts[0] != "none" {
		t.Fatalf("post-fetch efforts = %v, want none/low/high", efforts)
	}
	// Picker now offers exactly the real list.
	items := filterReasoningSuggestions("/reasoning ", final.reasoningEffort, final.modelReasoningEfforts())
	if len(items) != 3 {
		t.Fatalf("picker = %+v, want 3 real efforts", items)
	}
}
