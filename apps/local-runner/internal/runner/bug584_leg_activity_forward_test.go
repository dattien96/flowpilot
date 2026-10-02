package runner

import (
	"testing"
	"time"
)

// BUG-584 (live run-100368): the desktop subscribes only the parent's SSE
// stream — a 20-minute leg turn produced zero parent-stream events, so the
// live chip read "quiet" while the leg was working. Every leg event must
// forward a throttled agent_activity stamp onto the parent stream.

func legActivityEvents(t *testing.T, parent *interactiveRun) []ProviderEvent {
	t.Helper()
	var out []ProviderEvent
	for _, e := range parent.events {
		if e.Type == EventAgentActivity {
			out = append(out, e)
		}
	}
	return out
}

func TestBug584_ChildEventForwardsActivityStamp(t *testing.T) {
	svc := bug289Service(t)
	svc.mu.Lock()
	parent := &interactiveRun{id: "run-p584", status: RunStatusRunning, subs: map[int64]chan ProviderEvent{}}
	child := &interactiveRun{
		id:          "run-c584",
		parentRunID: "run-p584",
		agentName:   "coder",
		status:      RunStatusRunning,
		subs:        map[int64]chan ProviderEvent{},
	}
	svc.runs[parent.id] = parent
	svc.runs[child.id] = child
	svc.emitLocked(child, ProviderEvent{Type: EventMessageDelta, Text: "working"})
	svc.mu.Unlock()

	marks := legActivityEvents(t, parent)
	if len(marks) != 1 {
		t.Fatalf("expected 1 agent_activity on parent stream, got %d", len(marks))
	}
	if marks[0].ChildRunID != "run-c584" {
		t.Fatalf("agent_activity ChildRunID=%q want run-c584", marks[0].ChildRunID)
	}
	if marks[0].WorkflowRunID != "run-p584" {
		t.Fatalf("agent_activity emitted on wrong stream: %q", marks[0].WorkflowRunID)
	}
}

func TestBug584_ActivityStampIsThrottledPerLeg(t *testing.T) {
	svc := bug289Service(t)
	svc.mu.Lock()
	parent := &interactiveRun{id: "run-p584b", status: RunStatusRunning, subs: map[int64]chan ProviderEvent{}}
	child := &interactiveRun{
		id:          "run-c584b",
		parentRunID: "run-p584b",
		status:      RunStatusRunning,
		subs:        map[int64]chan ProviderEvent{},
	}
	svc.runs[parent.id] = parent
	svc.runs[child.id] = child
	for i := 0; i < 5; i++ {
		svc.emitLocked(child, ProviderEvent{Type: EventMessageDelta, Text: "x"})
	}
	svc.mu.Unlock()

	if n := len(legActivityEvents(t, parent)); n != 1 {
		t.Fatalf("5 rapid child events must forward exactly 1 stamp, got %d", n)
	}

	// Past the throttle window a new event forwards again.
	svc.mu.Lock()
	parent.legActivityForwardedAt["run-c584b"] = time.Now().UTC().Add(-2 * legActivityForwardInterval)
	svc.emitLocked(child, ProviderEvent{Type: EventMessageDelta, Text: "x"})
	svc.mu.Unlock()
	if n := len(legActivityEvents(t, parent)); n != 2 {
		t.Fatalf("post-window event must forward again, got %d stamps", n)
	}
}

// A grandchild leg's events must warm the ROOT stream too — the desktop
// subscribes only the focused (root) run's stream.
func TestBug584_GrandchildEventWarmsRootStream(t *testing.T) {
	svc := bug289Service(t)
	svc.mu.Lock()
	root := &interactiveRun{id: "run-r584g", status: RunStatusRunning, subs: map[int64]chan ProviderEvent{}}
	mid := &interactiveRun{id: "run-m584g", parentRunID: "run-r584g", status: RunStatusRunning, subs: map[int64]chan ProviderEvent{}}
	leaf := &interactiveRun{id: "run-l584g", parentRunID: "run-m584g", status: RunStatusRunning, subs: map[int64]chan ProviderEvent{}}
	for _, r := range []*interactiveRun{root, mid, leaf} {
		svc.runs[r.id] = r
	}
	svc.emitLocked(leaf, ProviderEvent{Type: EventMessageDelta, Text: "deep work"})
	svc.mu.Unlock()

	if n := len(legActivityEvents(t, root)); n != 1 {
		t.Fatalf("root must receive 1 agent_activity for the grandchild, got %d", n)
	}
	if n := len(legActivityEvents(t, mid)); n != 1 {
		t.Fatalf("mid parent must receive 1 agent_activity, got %d", n)
	}
	got := legActivityEvents(t, root)[0]
	if got.ChildRunID != "run-l584g" {
		t.Fatalf("root stamp must name the producing leg, got %q", got.ChildRunID)
	}
}

// A root run (no parent) never forwards — no parent stream exists.
func TestBug584_RootEventDoesNotSelfForward(t *testing.T) {
	svc := bug289Service(t)
	svc.mu.Lock()
	root := &interactiveRun{id: "run-r584", status: RunStatusRunning, subs: map[int64]chan ProviderEvent{}}
	svc.runs[root.id] = root
	svc.emitLocked(root, ProviderEvent{Type: EventMessageDelta, Text: "work"})
	svc.mu.Unlock()
	if n := len(legActivityEvents(t, root)); n != 0 {
		t.Fatalf("root run must not emit agent_activity, got %d", n)
	}
}
