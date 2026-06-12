package runner

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"
)

// ---- YOLO SSOT resolver (T-25) ---------------------------------------------

func TestResolveYoloPosture(t *testing.T) {
	on := resolveYoloPosture(true)
	if on.CodexSandbox != "full-access" || on.CodexApprovalMode != "never" || !on.RunnerAutoApprove {
		t.Fatalf("yolo=true posture = %+v", on)
	}
	off := resolveYoloPosture(false)
	if off.CodexSandbox != "workspace-write" || off.CodexApprovalMode != "on-request" || off.RunnerAutoApprove {
		t.Fatalf("yolo=false posture = %+v", off)
	}

	// the Codex adapter posture mapping must come from the same resolver (no drift)
	sb, am := codexYoloDerive(true)
	if sb != on.CodexSandbox || am != on.CodexApprovalMode {
		t.Fatalf("codexYoloDerive(true)=(%q,%q), want (%q,%q)", sb, am, on.CodexSandbox, on.CodexApprovalMode)
	}

	// the retired hack: proxy approval mode is now YOLO-derived, not pinned
	if got := googleDriveProxyMcpApprovalMode(true); got != "approve" {
		t.Fatalf("proxy approval yolo=true = %q, want approve", got)
	}
	if got := googleDriveProxyMcpApprovalMode(false); got != "on-request" {
		t.Fatalf("proxy approval yolo=false = %q, want on-request (no standalone =approve)", got)
	}
}

// ---- approval policy engine (T-39) -----------------------------------------

func TestApprovalPolicyEngineDecide(t *testing.T) {
	eng := NewApprovalPolicyEngine([]string{"npm run migrate", "ls"}, []string{"rm -rf", "drop table"})

	cases := []struct {
		cmd  string
		want PolicyOutcome
	}{
		{"npm run migrate", PolicyAutoApprove},
		{"ls -la", PolicyAutoApprove},
		{"rm -rf ./dist && npm run migrate", PolicyAutoDeny}, // denylist precedence
		{"DROP TABLE users", PolicyAutoDeny},                 // case-insensitive
		{"git status", PolicyAsk},
		{"", PolicyAsk},
	}
	for _, c := range cases {
		if got := eng.Decide(ApprovalDetails{Command: c.cmd}); got != c.want {
			t.Fatalf("Decide(%q) = %s, want %s", c.cmd, got, c.want)
		}
	}

	// default engine asks for everything
	if got := DefaultApprovalPolicyEngine().Decide(ApprovalDetails{Command: "rm -rf /"}); got != PolicyAsk {
		t.Fatalf("default engine = %s, want ask", got)
	}
}

// adminApprovals fetches the audit list for a run.
func adminApprovals(t *testing.T, base, runID string) []map[string]any {
	t.Helper()
	status, body := doJSON(t, "GET", base+"/admin/workflow-runs/"+runID+"/approvals", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("admin approvals status=%d", status)
	}
	var out []map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode approvals: %v", err)
	}
	return out
}

func hasPermissionRequired(evs []ProviderEvent) bool {
	for _, e := range evs {
		if e.Type == EventPermissionRequired {
			return true
		}
	}
	return false
}

// T-39: a denylisted command auto-denies without showing a card; the auto-decision
// is replied to the adapter (deny path runs) and recorded for audit.
func TestPolicyAutoDenyNoCard(t *testing.T) {
	svc, srv := newTestServer(t)
	svc.policy = NewApprovalPolicyEngine(nil, []string{"rm -rf"})

	runID := startRun(t, srv.URL)
	sendTurn(t, srv.URL, runID, "approval-required", nil)
	evs := waitTerminal(t, srv.URL, runID)

	if hasPermissionRequired(evs) {
		t.Fatal("auto-deny should not emit permission_required (no card)")
	}
	if !hasToolStatus(evs, "shell", "cancelled") {
		t.Fatal("auto-deny: expected shell tool_completed cancelled (deny path)")
	}
	apprs := adminApprovals(t, srv.URL, runID)
	if len(apprs) != 1 || apprs[0]["decision"] != "deny" || apprs[0]["policy"] != "policy_denylist" {
		t.Fatalf("expected one denylist-audited approval, got %+v", apprs)
	}
}

// T-39: an allowlisted command auto-approves without a card; approve path runs.
func TestPolicyAutoApproveNoCard(t *testing.T) {
	svc, srv := newTestServer(t)
	svc.policy = NewApprovalPolicyEngine([]string{"npm run migrate"}, nil)

	runID := startRun(t, srv.URL)
	sendTurn(t, srv.URL, runID, "approval-required", nil)
	evs := waitTerminal(t, srv.URL, runID)

	if hasPermissionRequired(evs) {
		t.Fatal("auto-approve should not emit permission_required (no card)")
	}
	if !hasToolStatus(evs, "shell", "success") {
		t.Fatal("auto-approve: expected shell tool_completed success (approve path)")
	}
	apprs := adminApprovals(t, srv.URL, runID)
	if len(apprs) != 1 || apprs[0]["policy"] != "policy_allowlist" {
		t.Fatalf("expected one allowlist-audited approval, got %+v", apprs)
	}
}

// ---- YOLO=true gating-disabled (T-21/T-24) ---------------------------------

func startRunYolo(t *testing.T, base string) string {
	t.Helper()
	status, body := doJSON(t, "POST", base+"/client/workflow-runs",
		StartRunInput{ProjectID: "proj-web", WorkflowID: "wf-feature", StepID: "step-plan", YoloMode: true}, nil)
	if status != http.StatusOK {
		t.Fatalf("start yolo run status=%d body=%s", status, body)
	}
	var h RunHandle
	if err := json.Unmarshal(body, &h); err != nil {
		t.Fatalf("decode handle: %v", err)
	}
	return h.RunID
}

// T-21/T-24: YOLO=true → no permission_required; the request that still arrives is
// auto-approved and audited as gating-disabled; approve path runs.
func TestYoloAutoApproveGatingDisabled(t *testing.T) {
	_, srv := newTestServer(t)
	runID := startRunYolo(t, srv.URL)
	sendTurn(t, srv.URL, runID, "approval-required", nil)
	evs := waitTerminal(t, srv.URL, runID)

	if hasPermissionRequired(evs) {
		t.Fatal("yolo=true must not emit permission_required")
	}
	if !hasToolStatus(evs, "shell", "success") {
		t.Fatal("yolo=true: expected shell tool_completed success (auto-approved)")
	}
	apprs := adminApprovals(t, srv.URL, runID)
	if len(apprs) != 1 || apprs[0]["decision"] != "approve" || apprs[0]["policy"] != "yolo_gating_disabled" {
		t.Fatalf("expected gating-disabled audit, got %+v", apprs)
	}
}

// ---- finalizer (T-27/T-28/T-41) --------------------------------------------

// T-41: a retried finalize does not double-write artifacts.
func TestFinalizerIdempotent(t *testing.T) {
	f := newFinalizer()
	calls := 0
	f.run = func(in finalizeInput) ([]Artifact, error) {
		calls++
		return []Artifact{{ID: in.RunID + ":a", RunID: in.RunID}}, nil
	}
	in := finalizeInput{RunID: "r1", TurnID: "t1", FinalMessage: "done"}
	for i := 0; i < 3; i++ {
		if err := f.Finalize(in); err != nil {
			t.Fatalf("finalize %d: %v", i, err)
		}
	}
	if calls != 1 {
		t.Fatalf("finalize ran %d times, want 1 (idempotent)", calls)
	}
	st, ok := f.state("r1", "t1")
	if !ok || st.status != finalizeDone || len(st.artifacts) != 1 {
		t.Fatalf("state = %+v ok=%v", st, ok)
	}
}

// T-27/T-28: a finalize failure is retryable and the completed turn is untouched; a
// later retry succeeds without double-writing.
func TestFinalizerFailureThenRetry(t *testing.T) {
	f := newFinalizer()
	fail := true
	calls := 0
	f.run = func(in finalizeInput) ([]Artifact, error) {
		calls++
		if fail {
			return nil, errors.New("rag unavailable")
		}
		return []Artifact{{ID: "ok"}}, nil
	}
	in := finalizeInput{RunID: "r1", TurnID: "t1"}

	if err := f.Finalize(in); err == nil {
		t.Fatal("expected finalize failure")
	}
	if st, _ := f.state("r1", "t1"); st.status != finalizeFailed || st.attempts != 1 {
		t.Fatalf("after failure: %+v", st)
	}

	fail = false
	if err := f.Finalize(in); err != nil {
		t.Fatalf("retry: %v", err)
	}
	st, _ := f.state("r1", "t1")
	if st.status != finalizeDone || st.attempts != 2 || len(st.artifacts) != 1 {
		t.Fatalf("after retry: %+v", st)
	}

	// further retries are no-ops (idempotent once done)
	_ = f.Finalize(in)
	if calls != 2 {
		t.Fatalf("run called %d times, want 2", calls)
	}
}

// the finalizer hook fires on turn_completed and its artifacts are surfaced.
func TestFinalizerHookSurfacesArtifacts(t *testing.T) {
	_, srv := newTestServer(t)
	runID := startRun(t, srv.URL)
	sendTurn(t, srv.URL, runID, "file-changes", nil)
	waitTerminal(t, srv.URL, runID)

	var arts []Artifact
	waitFor(t, func() bool {
		status, body := doJSON(t, "GET", srv.URL+"/client/workflow-runs/"+runID+"/artifacts", nil, nil)
		if status != http.StatusOK {
			return false
		}
		_ = json.Unmarshal(body, &arts)
		// finalizer artifact ids are run-scoped (run:turn:kind); fake ids are run-final
		for _, a := range arts {
			if a.Kind == "diff_snapshot" && a.RunID == runID {
				return true
			}
		}
		return false
	}, "finalizer artifacts")

	var diff *Artifact
	for i := range arts {
		if arts[i].Kind == "diff_snapshot" {
			diff = &arts[i]
		}
	}
	if diff == nil {
		t.Fatal("expected a diff_snapshot artifact from the finalizer")
	}
	// file-changes scenario touches 3 files
	if diff.Preview != "3 file(s) changed" {
		t.Fatalf("diff preview = %q, want \"3 file(s) changed\"", diff.Preview)
	}
}
