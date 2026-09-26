package runner

// BUG-506 (live run-15708): a spawned flow child inherits the parent's
// workflowID — the built-in mirror row's UUID — whose identity lives only in
// the store. During a transient catalog outage the UUID resolves to nothing:
// ListWorkflowSteps times out and flowStepsFromDefinition's ResolveFlowRef
// scans the embedded pack for a def.ID equal to the UUID (never matches),
// so createRun failed `catalog_unavailable` and the flow dead-parked at
// `coder spawn failed after tdd` (~90s outage stranded the chain).
//
// The parent run carries the canonical flowRef (chatFlowRef), which resolves
// from the embedded pack with no store. spawnChildRun now passes it on
// StartRunInput.FlowRefFallback — internal-only, never mode-validated — and
// createRun's step fallback retries the definition synthesis against it. A
// run with no resolvable ref still fails closed — the fallback adds an
// option, never a silent pass.
// New file; no pre-existing test is modified.

import (
	"context"
	"errors"
	"testing"
)

// bug506DeadStepCatalog fails every workflow-step read like the live outage.
type bug506DeadStepCatalog struct {
	stubCatalogStore
	err error
}

func (c *bug506DeadStepCatalog) ListWorkflowSteps(context.Context, string) ([]Step, error) {
	return nil, c.err
}

// bug506DeadFlowStore fails every definition read/write like the live outage.
type bug506DeadFlowStore struct {
	err error
}

func (f *bug506DeadFlowStore) GetByPackFlow(context.Context, string, string) (FlowDefinitionRecord, bool, error) {
	return FlowDefinitionRecord{}, false, f.err
}
func (f *bug506DeadFlowStore) GetByRef(context.Context, string) (FlowDefinitionRecord, bool, error) {
	return FlowDefinitionRecord{}, false, f.err
}
func (f *bug506DeadFlowStore) ListAll(context.Context) ([]FlowDefinitionRecord, error) {
	return nil, f.err
}
func (f *bug506DeadFlowStore) Upsert(context.Context, FlowDefinitionRecord) (FlowDefinitionRecord, error) {
	return FlowDefinitionRecord{}, f.err
}

// The live repro: catalog + definition store both unreachable, child of a
// bug-harness run — WorkflowID is the mirror UUID, FlowRef the canonical ref.
func TestBug506_ChildSurvivesCatalogOutageViaFlowRef(t *testing.T) {
	outage := errors.New(`dial tcp 104.18.38.10:443: i/o timeout`)
	catalog := &bug506DeadStepCatalog{err: outage}
	svc, _ := newTestServerWith(t, DefaultProviderRegistry(), catalog, newFakeWorkflowStore())
	svc.SetFlowDefinitionStore(&bug506DeadFlowStore{err: outage})

	h, apiErr := svc.createRun(StartRunInput{
		ProjectID:       "proj",
		ChatMode:        "normal_chat",
		ProviderKey:     ProviderKeyCodex,
		Model:           "gpt-5.4-mini", // a spawned child always inherits a model
		WorkflowID:      "c69dec0d-f341-47cb-8bdf-ea7b6dca2dba",
		FlowRefFallback: "bug-harness",
		Cwd:             t.TempDir(),
	})
	if apiErr != nil {
		t.Fatalf("createRun must survive the catalog outage via flowRef, got %s: %s", apiErr.code, apiErr.msg)
	}
	steps, err := svc.workflowStore.LoadRunSteps(context.Background(), h.RunID)
	if err != nil {
		t.Fatalf("LoadRunSteps: %v", err)
	}
	if len(steps) == 0 {
		t.Fatal("expected flow steps synthesized from the embedded pack")
	}
	svc.mu.Lock()
	got := svc.runs[h.RunID].chatFlowRef
	svc.mu.Unlock()
	if got != "bug-harness" {
		t.Fatalf("child must carry the canonical flowRef, got %q", got)
	}
}

// Fail-closed preserved: a workflowID that is not a resolvable flow and has
// no flowRef still rejects during the outage — the fallback adds an option,
// never a silent pass.
func TestBug506_NoFlowRefStillFailsClosed(t *testing.T) {
	outage := errors.New("i/o timeout")
	catalog := &bug506DeadStepCatalog{err: outage}
	svc, _ := newTestServerWith(t, DefaultProviderRegistry(), catalog, newFakeWorkflowStore())
	svc.SetFlowDefinitionStore(&bug506DeadFlowStore{err: outage})

	_, apiErr := svc.createRun(StartRunInput{
		ProjectID:  "proj",
		ChatMode:   "normal_chat",
		WorkflowID: "c69dec0d-f341-47cb-8bdf-ea7b6dca2dba",
		Cwd:        t.TempDir(),
	})
	if apiErr == nil {
		t.Fatal("unresolvable workflowID with no flowRef must still fail closed")
	}
	if apiErr.code != "catalog_unavailable" {
		t.Fatalf("error code = %q, want catalog_unavailable", apiErr.code)
	}
}

// The propagation leg: spawnChildRun stamps the parent's chatFlowRef on the
// child's create input so a deeper child in the same chain also survives.
func TestBug506_SpawnInheritsParentFlowRef(t *testing.T) {
	outage := errors.New("i/o timeout")
	catalog := &bug506DeadStepCatalog{err: outage}
	svc, _ := newTestServerWith(t, DefaultProviderRegistry(), catalog, newFakeWorkflowStore())
	svc.SetFlowDefinitionStore(&bug506DeadFlowStore{err: outage})

	parent, apiErr := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex, Cwd: t.TempDir(),
	})
	if apiErr != nil {
		t.Fatalf("parent createRun: %v", apiErr)
	}
	svc.mu.Lock()
	svc.runs[parent.RunID].chatFlowRef = "bug-harness"
	svc.runs[parent.RunID].workflowID = "c69dec0d-f341-47cb-8bdf-ea7b6dca2dba"
	svc.mu.Unlock()

	res, err := svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{
		Agent:  "coder",
		Prompt: "implement the scoped change",
		Label:  "implement",
	})
	if err != nil {
		t.Fatalf("spawnChildRun must survive the catalog outage via inherited flowRef: %v", err)
	}
	svc.mu.Lock()
	got := svc.runs[res.RunID].chatFlowRef
	svc.mu.Unlock()
	if got != "bug-harness" {
		t.Fatalf("spawned child must inherit the parent's flowRef, got %q", got)
	}
}
