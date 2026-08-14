package runner

import (
	"testing"
)

func TestGrokRunSessionIndex_RefusesSiblingSteal(t *testing.T) {
	var idx grokRunSessionIndex
	idx.remember("run-98477", "019ffe5a-shared")
	idx.remember("run-98485", "019ffe5a-shared") // sibling must not bind same ACP id
	if got := idx.lookup("run-98485"); got != "" {
		t.Fatalf("sibling steal bound session %q", got)
	}
	if got := idx.ownerOf("019ffe5a-shared"); got != "run-98477" {
		t.Fatalf("owner=%q want run-98477", got)
	}
	if got := idx.lookup("run-98477"); got != "019ffe5a-shared" {
		t.Fatalf("owner lost binding: %q", got)
	}
}

func TestGrokRunSessionIndex_SameRunRebindOK(t *testing.T) {
	var idx grokRunSessionIndex
	idx.remember("run-1", "sess-a")
	idx.remember("run-1", "sess-b")
	if got := idx.lookup("run-1"); got != "sess-b" {
		t.Fatalf("rebind = %q want sess-b", got)
	}
	if idx.ownerOf("sess-a") != "" {
		t.Fatal("old session should be released")
	}
	if idx.ownerOf("sess-b") != "run-1" {
		t.Fatal("new owner missing")
	}
}

func TestIsForeignProviderSessionID_GrokSibling(t *testing.T) {
	svc := NewInteractiveService()
	const shared = "019ffe5a-sibling-shared"
	parent := &interactiveRun{id: "run-hub", providerKey: ProviderKeyGrok, projectID: "p"}
	rsA := &interactiveRun{
		id: "run-a", parentRunID: "run-hub", providerKey: ProviderKeyGrok, projectID: "p",
		realProviderSessionID: shared, providerSessionID: shared,
	}
	rsB := &interactiveRun{
		id: "run-b", parentRunID: "run-hub", providerKey: ProviderKeyGrok, projectID: "p",
		providerSessionID: "thread-b",
	}
	svc.mu.Lock()
	svc.runs[parent.id] = parent
	svc.runs[rsA.id] = rsA
	svc.runs[rsB.id] = rsB
	svc.mu.Unlock()

	if !svc.isForeignProviderSessionID(rsB, shared) {
		t.Fatal("expected sibling Grok session to be foreign")
	}
	if svc.isForeignProviderSessionID(rsA, shared) {
		t.Fatal("owner must not treat own session as foreign")
	}
}

func TestRefreshResumeHandleGrokRefusesForeignSiblingSession(t *testing.T) {
	svc := NewInteractiveService()
	const shared = "019ffe5a-sibling-shared"
	parent := &interactiveRun{id: "run-hub", providerKey: ProviderKeyGrok, projectID: "p"}
	childA := &interactiveRun{
		id: "run-a", parentRunID: "run-hub", providerKey: ProviderKeyGrok,
		realProviderSessionID: shared, providerSessionID: shared, projectID: "p",
	}
	childB := &interactiveRun{
		id: "run-b", parentRunID: "run-hub", providerKey: ProviderKeyGrok,
		providerSessionID: "thread-b", projectID: "p",
	}
	svc.mu.Lock()
	svc.runs[parent.id] = parent
	svc.runs[childA.id] = childA
	svc.runs[childB.id] = childB
	svc.mu.Unlock()

	adapter := &keyedFakeAdapter{key: ProviderKeyGrok, lastGrokSessionID: shared}
	svc.mu.Lock()
	_ = svc.refreshResumeHandleLocked(childB, adapter)
	got := childB.realProviderSessionID
	svc.mu.Unlock()
	if got == shared {
		t.Fatal("must not promote sibling's Grok ACP id onto childB")
	}
}
