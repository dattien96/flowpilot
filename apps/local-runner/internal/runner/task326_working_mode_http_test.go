package runner

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"flowpilot-runner/internal/workingmode"
)

func httpErrCode(body []byte) string {
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &env)
	if env.Error.Code != "" {
		return env.Error.Code
	}
	return string(body)
}

func runWorkingMode(t *testing.T, svc *InteractiveService, runID string) string {
	t.Helper()
	svc.mu.Lock()
	defer svc.mu.Unlock()
	rs := svc.runs[runID]
	if rs == nil {
		t.Fatalf("run %s missing", runID)
	}
	if rs.workingMode == "" {
		return workingmode.Dev
	}
	return rs.workingMode
}

func runCount(svc *InteractiveService) int {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	return len(svc.runs)
}

func decodeHandle(t *testing.T, body []byte) RunHandle {
	t.Helper()
	var h RunHandle
	if err := json.Unmarshal(body, &h); err != nil {
		t.Fatalf("handle: %v body=%s", err, body)
	}
	if h.RunID == "" {
		t.Fatalf("empty run id body=%s", body)
	}
	return h
}

func xClient(name string) map[string]string {
	if name == "" {
		return nil
	}
	return map[string]string{"X-Client": name}
}

// Scenario: desktop vibe start vibe-ingest stamps the run.
func TestHTTPStart_VibeDesktopIngestStampsMode(t *testing.T) {
	svc, srv := newTestServer(t)
	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs", StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", FlowRef: "vibe-ingest",
	}, xClient("desktop"))
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, body)
	}
	h := decodeHandle(t, body)
	if got := runWorkingMode(t, svc, h.RunID); got != workingmode.Vibe {
		t.Fatalf("WorkingMode=%s, want vibe", got)
	}
}

// Scenario: tui omitted mode starts task-harness as dev.
func TestHTTPStart_DevTuiTaskHarnessDefaultMode(t *testing.T) {
	svc, srv := newTestServer(t)
	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs", StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		FlowRef: "task-harness",
	}, xClient("tui"))
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, body)
	}
	h := decodeHandle(t, body)
	if got := runWorkingMode(t, svc, h.RunID); got != workingmode.Dev {
		t.Fatalf("WorkingMode=%s, want dev", got)
	}
}

// Scenario: grok spot-check allow path.
func TestHTTPStart_VibeIngestGrokSpotCheck(t *testing.T) {
	svc, srv := newTestServer(t)
	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs", StartRunInput{
		ProjectID: "proj", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", FlowRef: "vibe-ingest",
	}, xClient("tui"))
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, body)
	}
	h := decodeHandle(t, body)
	if got := runWorkingMode(t, svc, h.RunID); got != workingmode.Vibe {
		t.Fatalf("WorkingMode=%s, want vibe", got)
	}
}

// Scenario: pack-prefixed ingest id is accepted in vibe.
func TestHTTPStart_VibePackPrefixedIngest(t *testing.T) {
	svc, srv := newTestServer(t)
	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs", StartRunInput{
		ProjectID: "proj", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", FlowRef: "flowpilot-core-flow-pack/vibe-ingest",
	}, xClient("tui"))
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, body)
	}
	h := decodeHandle(t, body)
	if got := runWorkingMode(t, svc, h.RunID); got != workingmode.Vibe {
		t.Fatalf("WorkingMode=%s, want vibe", got)
	}
}

// Scenario: admin + dev harness still allowed.
func TestHTTPStart_AdminDevHarnessAllowed(t *testing.T) {
	svc, srv := newTestServer(t)
	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs", StartRunInput{
		ProjectID: "proj", ProviderKey: ProviderKeyCodex, FlowRef: "task-harness",
	}, xClient("admin"))
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, body)
	}
	h := decodeHandle(t, body)
	if got := runWorkingMode(t, svc, h.RunID); got != workingmode.Dev {
		t.Fatalf("WorkingMode=%s, want dev", got)
	}
}

// Scenario: missing X-Client + dev harness still allowed.
func TestHTTPStart_MissingClientDevHarnessAllowed(t *testing.T) {
	svc, srv := newTestServer(t)
	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs", StartRunInput{
		ProjectID: "proj", ProviderKey: ProviderKeyCodex, FlowRef: "task-harness",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, body)
	}
	h := decodeHandle(t, body)
	if got := runWorkingMode(t, svc, h.RunID); got != workingmode.Dev {
		t.Fatalf("WorkingMode=%s, want dev", got)
	}
}

// Scenario: desktop and tui vibe ingest are equivalent.
func TestHTTPStart_DesktopTuiVibeIngestEquivalent(t *testing.T) {
	svc, srv := newTestServer(t)
	in := StartRunInput{ProjectID: "proj", ProviderKey: ProviderKeyCodex, WorkingMode: "vibe", FlowRef: "vibe-ingest"}
	for _, client := range []string{"desktop", "tui"} {
		status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs", in, xClient(client))
		if status != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", client, status, body)
		}
		h := decodeHandle(t, body)
		if got := runWorkingMode(t, svc, h.RunID); got != workingmode.Vibe {
			t.Fatalf("%s WorkingMode=%s", client, got)
		}
	}
}

func assertNoNewRun(t *testing.T, svc *InteractiveService, before int, status int, body []byte, wantCode string) {
	t.Helper()
	if status == http.StatusOK {
		t.Fatalf("want error %s, got 200 body=%s", wantCode, body)
	}
	if !strings.Contains(httpErrCode(body), wantCode) && !strings.Contains(string(body), wantCode) {
		t.Fatalf("body=%s want %s", body, wantCode)
	}
	if got := runCount(svc); got != before {
		t.Fatalf("run count %d → %d, want no new run", before, got)
	}
}

// Scenario: vibe + task-harness does not create a run.
func TestHTTPStart_VibeTaskHarnessNoRunCreated(t *testing.T) {
	svc, srv := newTestServer(t)
	before := runCount(svc)
	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs", StartRunInput{
		ProjectID: "proj", ProviderKey: ProviderKeyCodex, WorkingMode: "vibe", FlowRef: "task-harness",
	}, xClient("desktop"))
	assertNoNewRun(t, svc, before, status, body, workingmode.CodeFlowForbidden)
}

// Scenario: grok spot-check deny path.
func TestHTTPStart_VibeTaskHarnessGrokSpotCheck(t *testing.T) {
	svc, srv := newTestServer(t)
	before := runCount(svc)
	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs", StartRunInput{
		ProjectID: "proj", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", FlowRef: "task-harness",
	}, xClient("desktop"))
	assertNoNewRun(t, svc, before, status, body, workingmode.CodeFlowForbidden)
}

// Scenario: pack-prefixed harness forbidden in vibe.
func TestHTTPStart_VibePackPrefixedHarnessForbidden(t *testing.T) {
	svc, srv := newTestServer(t)
	before := runCount(svc)
	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs", StartRunInput{
		ProjectID: "proj", ProviderKey: ProviderKeyCodex, WorkingMode: "vibe",
		FlowRef: "flowpilot-core-flow-pack/task-harness",
	}, xClient("tui"))
	assertNoNewRun(t, svc, before, status, body, workingmode.CodeFlowForbidden)
}

// Scenario: dev + vibe-ingest forbidden and creates no run.
func TestHTTPStart_DevVibeIngestForbidden(t *testing.T) {
	svc, srv := newTestServer(t)
	before := runCount(svc)
	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs", StartRunInput{
		ProjectID: "proj", ProviderKey: ProviderKeyCodex, WorkingMode: "dev", FlowRef: "vibe-ingest",
	}, xClient("tui"))
	assertNoNewRun(t, svc, before, status, body, workingmode.CodeFlowForbidden)
}

// Scenario: user vibe-owner-debate forbidden even in vibe.
func TestHTTPStart_UserOwnerDebateForbidden(t *testing.T) {
	svc, srv := newTestServer(t)
	before := runCount(svc)
	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs", StartRunInput{
		ProjectID: "proj", ProviderKey: ProviderKeyCodex, WorkingMode: "vibe", FlowRef: "vibe-owner-debate",
	}, xClient("desktop"))
	assertNoNewRun(t, svc, before, status, body, workingmode.CodeFlowForbidden)
}

// Scenario: user vibe-sprint forbidden even in vibe.
func TestHTTPStart_UserSprintForbidden(t *testing.T) {
	svc, srv := newTestServer(t)
	before := runCount(svc)
	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs", StartRunInput{
		ProjectID: "proj", ProviderKey: ProviderKeyCodex, WorkingMode: "vibe", FlowRef: "vibe-sprint",
	}, xClient("tui"))
	assertNoNewRun(t, svc, before, status, body, workingmode.CodeFlowForbidden)
}

// Scenario: user vibe-cp-ingest is allowed after Task-321.
func TestHTTPStart_UserCpIngestForbidden(t *testing.T) {
	svc, srv := newTestServer(t)
	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs", StartRunInput{
		ProjectID: "proj", ProviderKey: ProviderKeyCodex, WorkingMode: "vibe", FlowRef: "vibe-cp-ingest",
	}, xClient("tui"))
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, body)
	}
	h := decodeHandle(t, body)
	if got := runWorkingMode(t, svc, h.RunID); got != workingmode.Vibe {
		t.Fatalf("WorkingMode=%s, want vibe", got)
	}
}

// Scenario: admin cannot start vibe.
func TestHTTPStart_AdminVibeForbidden(t *testing.T) {
	svc, srv := newTestServer(t)
	before := runCount(svc)
	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs", StartRunInput{
		ProjectID: "proj", ProviderKey: ProviderKeyCodex, WorkingMode: "vibe", FlowRef: "vibe-ingest",
	}, xClient("admin"))
	assertNoNewRun(t, svc, before, status, body, workingmode.CodeClientForbidden)
}

// Scenario: missing X-Client cannot start vibe.
func TestHTTPStart_MissingClientVibeForbidden(t *testing.T) {
	svc, srv := newTestServer(t)
	before := runCount(svc)
	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs", StartRunInput{
		ProjectID: "proj", ProviderKey: ProviderKeyCodex, WorkingMode: "vibe", FlowRef: "vibe-ingest",
	}, nil)
	assertNoNewRun(t, svc, before, status, body, workingmode.CodeClientForbidden)
}

// Scenario: unknown working_mode rejected before start.
func TestHTTPStart_UnknownModeRejected(t *testing.T) {
	svc, srv := newTestServer(t)
	before := runCount(svc)
	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs", StartRunInput{
		ProjectID: "proj", ProviderKey: ProviderKeyCodex, WorkingMode: "prod", FlowRef: "task-harness",
	}, xClient("tui"))
	assertNoNewRun(t, svc, before, status, body, workingmode.CodeInvalidMode)
}

// Scenario: hidden flows stay invalid_flow_ref in both modes (not family-forbidden).
func TestHTTPStart_HiddenFlowsStayInvalidFlowRef(t *testing.T) {
	svc, srv := newTestServer(t)
	for _, mode := range []string{"dev", "vibe"} {
		for _, ref := range []string{"review-loop", "rag-harness", "cp-harness-smoke"} {
			before := runCount(svc)
			status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs", StartRunInput{
				ProjectID: "proj", ProviderKey: ProviderKeyCodex, WorkingMode: mode, FlowRef: ref,
			}, xClient("tui"))
			if strings.Contains(string(body), workingmode.CodeFlowForbidden) {
				t.Fatalf("mode=%s ref=%s used family-forbidden: %s", mode, ref, body)
			}
			assertNoNewRun(t, svc, before, status, body, workingmode.CodeInvalidFlowRef)
		}
	}
}

// Scenario: mutating a live run's working_mode is pinned.
func TestHTTPStart_MidRunModePinned(t *testing.T) {
	svc, srv := newTestServer(t)
	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs", StartRunInput{
		ProjectID: "proj", ProviderKey: ProviderKeyCodex, FlowRef: "task-harness",
	}, xClient("tui"))
	if status != http.StatusOK {
		t.Fatalf("seed status=%d body=%s", status, body)
	}
	h := decodeHandle(t, body)
	before := runCount(svc)
	status, body = doJSON(t, "POST", srv.URL+"/client/workflow-runs", StartRunInput{
		ProjectID: "proj", ProviderKey: ProviderKeyCodex, WorkingMode: "vibe", RunID: h.RunID,
	}, xClient("tui"))
	if status != http.StatusConflict {
		t.Fatalf("pin status=%d body=%s, want 409", status, body)
	}
	if !strings.Contains(string(body), workingmode.CodePinned) {
		t.Fatalf("body=%s want %s", body, workingmode.CodePinned)
	}
	if runCount(svc) != before {
		t.Fatal("pin request created a run")
	}
	if got := runWorkingMode(t, svc, h.RunID); got != workingmode.Dev {
		t.Fatalf("live WorkingMode=%s, want dev", got)
	}
}

// Scenario: normal chat with no flowRef is unaffected.
func TestHTTPStart_NormalChatUnaffected(t *testing.T) {
	svc, srv := newTestServer(t)
	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs", StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex, WorkingMode: "dev",
	}, xClient("tui"))
	if status != http.StatusOK {
		t.Fatalf("status=%d body=%s", status, body)
	}
	h := decodeHandle(t, body)
	if got := runWorkingMode(t, svc, h.RunID); got != workingmode.Dev {
		t.Fatalf("WorkingMode=%s, want dev", got)
	}
}
