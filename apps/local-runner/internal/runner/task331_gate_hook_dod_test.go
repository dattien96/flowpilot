package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Task-331 (CP-47 P-5): runner wiring tests for the r-dod-complete gate.
// New file only — no pre-existing test is modified. Harness mirrors the
// established gate-path fixtures (newContractFreezeTestRepo /
// newFreezeTestService / newP4ChildRun re-keyed onto a createRun id, as in
// run200816_hub_plan_gate_and_verdict_advance_test.go).

// newTask331RootRun builds a Normal-chat root run (parentRunID empty,
// flowEngineDriven false) over dir so svc.runFlowGate evaluates the full
// default rule set, including r-dod-complete.
func newTask331RootRun(t *testing.T, svc *InteractiveService, dir, head string) *interactiveRun {
	t.Helper()
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
	rs := newP4ChildRun(svc, "dod-root", "", dir, head)
	svc.mu.Lock()
	svc.runs[parent.RunID] = rs
	rs.id = parent.RunID
	rs.workspaceCwd = dir
	svc.mu.Unlock()
	return rs
}

// writeTask331Doc writes a workspace doc at rel (slash-separated).
func writeTask331Doc(t *testing.T, dir, rel, content string) {
	t.Helper()
	abs := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// lastTask331GateViolation returns the most recent EventFlowGateViolation
// event emitted for rs, or nil.
func lastTask331GateViolation(svc *InteractiveService, rs *interactiveRun) *ProviderEvent {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	var last *ProviderEvent
	for i := range rs.events {
		if rs.events[i].Type == EventFlowGateViolation {
			ev := rs.events[i]
			last = &ev
		}
	}
	return last
}

// Scenario: Runner phát hiện file Task chuyển vào thư mục done/ và kích hoạt r-dod-complete
// Input: WrittenPaths chứa "requirements/08-Task/done/Task-001.md" có checkbox chưa tick
// Expect: runFlowGate trả về block=true
func TestGateHook_DodComplete_BlocksUnfinishedTask(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	rs := newTask331RootRun(t, svc, dir, head)

	const docRel = "requirements/08-Task/done/Task-001.md"
	writeTask331Doc(t, dir, docRel, `# Task-001: Retry path

## Metadata

- Status: done

## Definition of Done

- [x] Retry wraps transient failures
- [ ] Wire the retry path
`)

	blocked := svc.runFlowGate(context.Background(), rs, "turn-1", finalizeInput{
		FinalMessage: "wrapped up the coding step",
		ChangedFiles: []string{docRel},
	})
	if !blocked {
		t.Fatal("runFlowGate must block a turn that marks a Task done with an open DOD checkbox and no explanation")
	}
	ev := lastTask331GateViolation(svc, rs)
	if ev == nil {
		t.Fatal("expected a flow-gate violation event for the UI Decision Card")
	}
	if ev.Status != "block" {
		t.Fatalf("violation status = %q, want block", ev.Status)
	}
	if !strings.Contains(ev.Error, "Wire the retry path") {
		t.Fatalf("violation %q must list the open DOD checkbox verbatim", ev.Error)
	}
}

// Scenario: Runner bỏ qua r-dod-complete khi chỉ sửa code và không chạm file Task/Bug
// Input: WrittenPaths chứa "src/service.go"
// Expect: runFlowGate cho phép pass bình thường
func TestGateHook_DodComplete_BypassesWhenNoDocTouched(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	rs := newTask331RootRun(t, svc, dir, head)

	// A code-changing turn that satisfies the other gates: declared Change
	// Contract in the final message, change-audit note among the written
	// paths, and no on-disk code diff (so r-newtest stays quiet). No
	// Task-*/BUG-* doc is touched — r-dod-complete must bypass entirely
	// (CP-47 D-5 bypass safety).
	blocked := svc.runFlowGate(context.Background(), rs, "turn-1", finalizeInput{
		FinalMessage: "[Change Contract]\nfeature: dod-bypass\nintent: tweak service\nfiles: src/service.go\n",
		ChangedFiles: []string{"src/service.go", "change-audit/CA-900.md"},
	})
	if blocked {
		t.Fatal("runFlowGate must pass a code-only turn that never touches a Task/BUG doc")
	}
	if ev := lastTask331GateViolation(svc, rs); ev != nil {
		t.Fatalf("no gate violation expected on the bypass path, got %+v", *ev)
	}
}

// [Edge] Scenario: File Task vừa được tạo mới (chưa từng tồn tại) trong thư mục done/
// Input: WrittenPaths=["requirements/08-Task/done/Task-NEW.md"], file mới hoàn toàn
// Expect: Gate vẫn kích hoạt kiểm tra r-dod-complete
func TestGateHook_DodComplete_NewFileInDoneDir(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	rs := newTask331RootRun(t, svc, dir, head)

	// Brand-new file (never committed, no metadata): the done/ path segment
	// alone is the sufficient done signal (CP-47 D-2 backup signal).
	const docRel = "requirements/08-Task/done/Task-NEW.md"
	writeTask331Doc(t, dir, docRel, `# Task-NEW: Fresh doc

## Definition of Done

- [x] Drafted
- [ ] Still reviewing edge cases
`)

	blocked := svc.runFlowGate(context.Background(), rs, "turn-1", finalizeInput{
		FinalMessage: "all wrapped up",
		ChangedFiles: []string{docRel},
	})
	if !blocked {
		t.Fatal("runFlowGate must still activate r-dod-complete for a brand-new file in a done/ directory")
	}
	ev := lastTask331GateViolation(svc, rs)
	if ev == nil {
		t.Fatal("expected a flow-gate violation event for the new done/ doc")
	}
	if ev.Status != "block" {
		t.Fatalf("violation status = %q, want block", ev.Status)
	}
	if !strings.Contains(ev.Error, "Still reviewing edge cases") {
		t.Fatalf("violation %q must list the open DOD checkbox verbatim", ev.Error)
	}
}
