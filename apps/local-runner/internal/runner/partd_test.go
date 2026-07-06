package runner

import (
	"context"
	"encoding/json"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

// ---- T-01: live shared app-server starts + initialize captures caps ---------

// mockCodexInitProcess scripts a process that answers the initialize handshake and
// stays alive without depending on a platform-specific shell.
func mockCodexInitProcess(t *testing.T) func() {
	t.Helper()
	orig := commandContextFn
	commandContextFn = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		script := shellReadLine() +
			shellOutputLine(`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":1,"capabilities":{"threads":true}}}`) +
			"sleep 3\n"
		return testShellCommand(ctx, script)
	}
	return func() { commandContextFn = orig }
}

func TestEnsureCodexAppServerInitializes(t *testing.T) {
	defer mockCodexInitProcess(t)()
	r, _ := New(".")

	h, err := r.ensureCodexAppServer(context.Background(), "default", ".", nil)
	if err != nil {
		t.Fatalf("ensureCodexAppServer: %v", err)
	}
	defer h.close()
	if h.adapter == nil || h.dispatcher == nil {
		t.Fatal("handle missing adapter/dispatcher")
	}
	if !h.supports("threads") {
		t.Fatal("expected negotiated capability threads=true")
	}
	if h.supports("nonexistent_unknown") != true {
		t.Fatal("unknown capability should be optimistically supported")
	}

	// reuse: same scope returns the same handle (one shared process)
	h2, err := r.ensureCodexAppServer(context.Background(), "default", ".", nil)
	if err != nil || h2 != h {
		t.Fatalf("same-scope ensure should reuse the handle (h2==h=%v, err=%v)", h2 == h, err)
	}

	// account switch: a different scope tears down + recreates (new handle)
	h3, err := r.ensureCodexAppServer(context.Background(), "acct-2", ".", nil)
	if err != nil {
		t.Fatalf("recreate ensure: %v", err)
	}
	defer h3.close()
	if h3 == h {
		t.Fatal("a different scope must recreate the app-server")
	}
	if !h.dispatcher.isClosed() {
		t.Fatal("the previous-scope dispatcher should be closed after recreate")
	}
}

// ---- registry swap ---------------------------------------------------------

func TestProviderRegistryForUsesFakeWhenFlagOff(t *testing.T) {
	r, _ := New(".")
	reg := ProviderRegistryFor(r) // flag off by default
	adapter, err := reg.Adapter(ProviderKeyCodex)
	if err != nil {
		t.Fatalf("codex adapter: %v", err)
	}
	if _, ok := adapter.(*fakeProviderAdapter); !ok {
		t.Fatalf("flag off should keep the fake adapter, got %T", adapter)
	}
}

func TestProviderRegistryForUsesLiveWhenFlagOn(t *testing.T) {
	t.Setenv(codexAppServerEnvFlag, "1")
	defer mockCodexInitProcess(t)()
	r, _ := New(".")
	reg := ProviderRegistryFor(r)
	adapter, err := reg.Adapter(ProviderKeyCodex)
	if err != nil {
		t.Fatalf("codex adapter: %v", err)
	}
	if _, ok := adapter.(*codexAdapter); !ok {
		t.Fatalf("flag on should use the live codex adapter, got %T", adapter)
	}
	r.codexAppServer.close()
}

// ---- placeholder adapters --------------------------------------------------

func TestPlaceholderAdaptersReturnTypedError(t *testing.T) {
	for _, key := range []ProviderKey{ProviderKeyClaude, ProviderKeyGemini} {
		a := newPlaceholderAdapter(key)
		err := a.SendTurn(context.Background(), TurnRequest{}, &captureBridge{})
		if _, ok := err.(*UnsupportedProviderRuntimeError); !ok {
			t.Fatalf("%s SendTurn err = %T, want UnsupportedProviderRuntimeError", key, err)
		}
		if a.Capabilities().Streaming {
			t.Fatalf("%s placeholder should advertise disabled capabilities", key)
		}
	}
}

// ---- ask_user registration + per-thread cwd -------------------------------

func TestCodexAdapterRegistersAskUserAndUsesReqCwd(t *testing.T) {
	d, fc := startFakeCodex(t, nil)
	adapter := newCodexAdapter(d, "/default-cwd")

	startCh := make(chan map[string]any, 1)
	fc.serve(func(fc *fakeCodex, m map[string]any) {
		method, _ := m["method"].(string)
		switch method {
		case "thread/start":
			select {
			case startCh <- m:
			default:
			}
			fc.reply(m["id"], map[string]any{"threadId": "th1"})
		case "turn/start":
			fc.reply(m["id"], map[string]any{"turnId": "ct1"})
			fc.notify("turn.completed", map[string]any{"threadId": "th1", "finalMessage": "done"})
		}
	})

	bridge := &captureBridge{}
	err := adapter.SendTurn(context.Background(), TurnRequest{RunID: "r1", Prompt: "hi", Cwd: "/run-cwd"}, bridge)
	if err != nil {
		t.Fatalf("SendTurn: %v", err)
	}

	// inspect the captured thread/start: per-run cwd wins, ask_user is registered
	var start map[string]any
	select {
	case start = <-startCh:
	case <-time.After(time.Second):
		t.Fatal("did not observe thread/start")
	}
	params, _ := start["params"].(map[string]any)
	if params["cwd"] != "/run-cwd" {
		t.Fatalf("thread/start cwd = %v, want /run-cwd (per-run cwd authoritative)", params["cwd"])
	}
	// ask_user is registered as a thread dynamicTool (DynamicToolSpec), the real app-server
	// registration channel — not the old (ignored) inline mcpServers shape.
	raw, _ := json.Marshal(params["dynamicTools"])
	if !strings.Contains(string(raw), "ask_user") {
		t.Fatalf("thread/start dynamicTools should register ask_user: %s", raw)
	}
}

// ---- finalize summary/RAG shaping (T-14 body) ------------------------------

func TestFinalizeLocalSnapshotShapesSummaryAndRag(t *testing.T) {
	f := newFinalizer()
	arts, err := f.localSnapshot(finalizeInput{
		RunID: "r1", TurnID: "t1", FinalMessage: "shipped it",
		ChangedFiles: []string{"a.go", "b.go"},
	})
	if err != nil {
		t.Fatalf("localSnapshot: %v", err)
	}
	byKind := map[string]Artifact{}
	for _, a := range arts {
		byKind[a.Kind] = a
	}
	for _, kind := range []string{"final_response", "diff_snapshot", "summary", "rag_document"} {
		if _, ok := byKind[kind]; !ok {
			t.Fatalf("missing %s artifact", kind)
		}
	}
	if !strings.Contains(byKind["summary"].Preview, "shipped it") || !strings.Contains(byKind["summary"].Preview, "2 file(s) changed") {
		t.Fatalf("summary preview = %q", byKind["summary"].Preview)
	}
	var rag map[string]any
	if err := json.Unmarshal([]byte(byKind["rag_document"].Preview), &rag); err != nil {
		t.Fatalf("rag document is not valid JSON: %v", err)
	}
	if rag["runId"] != "r1" || rag["finalMessage"] != "shipped it" {
		t.Fatalf("rag document = %+v", rag)
	}
}

// ---- Supabase catalog read-path shaping ------------------------------------

func TestSupabaseCatalogStoreShaping(t *testing.T) {
	store := NewSupabaseCatalogStore(SupabaseWorkspaceConfig{APIURL: "https://proj.supabase.co"}, "k")

	usableBinding := t.TempDir()
	cap := withMockHTTP(t, 200, []byte(`[
		{
			"id":"p1",
			"name":"Acme",
			"directory_path":"/missing/primary",
			"default_model":"gpt-5.4",
			"project_workspace_bindings":[
				{"local_path":"/missing/binding"},
				{"local_path":`+strconv.Quote(usableBinding)+`}
			]
		}
	]`))
	projects, err := store.ListProjects(context.Background())
	if err != nil || len(projects) != 1 || projects[0].Name != "Acme" || projects[0].Path != usableBinding {
		t.Fatalf("projects = %+v err=%v", projects, err)
	}
	// BUG-165: Model (projects.default_model) is the "Project" tier of the
	// Step > Flow > Project > default resolution order.
	if projects[0].Model != "gpt-5.4" {
		t.Fatalf("project model = %q, want gpt-5.4", projects[0].Model)
	}
	if !strings.Contains((*cap)[0].endpoint, "/rest/v1/projects?select=id,name,directory_path,default_model,project_workspace_bindings(local_path)") {
		t.Fatalf("projects endpoint = %s", (*cap)[0].endpoint)
	}

	cap2 := withMockHTTP(t, 200, []byte(`[{"id":"w1","project_id":"p1","name":"Feature","description":"d","model_override":"claude-sonnet"}]`))
	wfs, err := store.ListWorkflows(context.Background())
	if err != nil || len(wfs) != 1 || wfs[0].ProjectID != "p1" {
		t.Fatalf("workflows = %+v err=%v", wfs, err)
	}
	// BUG-165: Model (workflows.model_override) is the "Flow" tier.
	if wfs[0].Model != "claude-sonnet" {
		t.Fatalf("workflow model = %q, want claude-sonnet", wfs[0].Model)
	}
	if !strings.Contains((*cap2)[0].endpoint, "workflows?created_by=neq.flowpilot-runtime") ||
		!strings.Contains((*cap2)[0].endpoint, "model_override") {
		t.Fatalf("workflows endpoint = %s", (*cap2)[0].endpoint)
	}

	cap3 := withMockHTTP(t, 200, []byte(`[{"step_type":"plan","name":"Plan","model":"gpt-5.5","node_id":"reviewer_correctness","behavior_id":"agent.delegate","agent_ref":"agents/reviewer.md"}]`))
	steps, err := store.ListSteps(context.Background())
	if err != nil || len(steps) != 1 || steps[0].Name != "Plan" {
		t.Fatalf("steps = %+v err=%v", steps, err)
	}
	// BUG-165: Model (step_definitions.model) is the "Step" tier.
	if steps[0].Model != "gpt-5.5" {
		t.Fatalf("step model = %q, want gpt-5.5", steps[0].Model)
	}
	if steps[0].NodeID != "reviewer_correctness" || steps[0].BehaviorID != "agent.delegate" || steps[0].AgentRef != "agents/reviewer.md" {
		t.Fatalf("step node metadata = node:%q behavior:%q agent:%q", steps[0].NodeID, steps[0].BehaviorID, steps[0].AgentRef)
	}
	if !strings.Contains((*cap3)[0].endpoint, "step_definitions?select=step_type,name,model") {
		t.Fatalf("steps endpoint = %s", (*cap3)[0].endpoint)
	}
	for _, want := range []string{"node_id", "behavior_id", "agent_ref"} {
		if !strings.Contains((*cap3)[0].endpoint, want) {
			t.Fatalf("steps endpoint missing %q: %s", want, (*cap3)[0].endpoint)
		}
	}

	cap4 := withMockHTTP(t, 200, []byte(`[{"id":"ws1","workflow_id":"w1","step_type":"plan","order_index":0,"step_definitions":{"name":"Plan","required_skills":["planner"],"model":"claude-haiku"}}]`))
	workflowSteps, err := store.ListWorkflowSteps(context.Background(), "w1")
	if err != nil || len(workflowSteps) != 1 || workflowSteps[0].ID != "ws1" || workflowSteps[0].DefaultSkill != "planner" {
		t.Fatalf("workflow steps = %+v err=%v", workflowSteps, err)
	}
	if workflowSteps[0].Model != "claude-haiku" {
		t.Fatalf("workflow step model = %q, want claude-haiku", workflowSteps[0].Model)
	}
	if !strings.Contains((*cap4)[0].endpoint, "workflow_steps?workflow_id=eq.w1") ||
		!strings.Contains((*cap4)[0].endpoint, "is_enabled=is.true") {
		t.Fatalf("workflow steps endpoint = %s", (*cap4)[0].endpoint)
	}
}
