package runner

// ============================================================================
// CP-89 live harness — L-1..L-13 from CP-89-Test-Steps §3, executed through
// the REAL HTTP entry points of a real `flowpilot runner serve` process (never
// the in-process service). Guarded by LIVE=1; skipped-by-default in CI.
//
// Env knobs:
//   LIVE=1                      required opt-in
//   CP89_LIVE_PROVIDER          provider to drive (default: first binary found
//                               of devin, claude, codex, grok, opencode)
//   CP89_LIVE_PROVIDER_ALT      second provider for L-11 leg switch (default:
//                               second binary found; skips with reason if none)
//   CP89_LIVE_WORKSPACE         reuse a workspace dir (default: t.TempDir())
//
// Skip matrix (per §3): providers without local binaries are NAMED in the skip
// reason — parity claims are limited to the provider actually executed.
// L-12 skips everywhere: pack flows are go:embed'd so a live corrupt-definition
// cannot be staged through the binary; the path is unit-covered.
// ============================================================================

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const cp89PollTurnTimeout = 4 * time.Minute
const cp89FlowWaitTimeout = 5 * time.Minute

type cp89LiveEnv struct {
	base      string
	workspace string
	provider  string
	alt       string
	http      *http.Client
	proc      *exec.Cmd
	port      int
}

func cp89ProviderBinaries() []string {
	return []string{"devin", "claude", "codex", "grok", "opencode"}
}

func cp89DetectProvider(preferred string, exclude ...string) (string, []string) {
	var missing []string
	if preferred != "" {
		if _, err := exec.LookPath(preferred); err == nil {
			return preferred, nil
		}
		missing = append(missing, preferred)
	}
	ex := map[string]bool{}
	for _, e := range exclude {
		ex[e] = true
	}
	for _, b := range cp89ProviderBinaries() {
		if ex[b] {
			continue
		}
		if _, err := exec.LookPath(b); err == nil {
			return b, missing
		}
		missing = append(missing, b)
	}
	return "", missing
}

func cp89FreePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("pick free port: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// cp89BuildBinary compiles the real CLI once per run — the serve path must be
// the shipped entry point, not the test harness.
func cp89BuildBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "fp-cp89")
	out, err := exec.Command("go", "build", "-o", bin, "flowpilot-runner/cmd/flowpilot").CombinedOutput()
	if err != nil {
		t.Fatalf("go build flowpilot-runner/cmd/flowpilot: %v\n%s", err, out)
	}
	return bin
}

func (e *cp89LiveEnv) startServer(t *testing.T, bin string) {
	t.Helper()
	e.port = cp89FreePort(t)
	e.base = fmt.Sprintf("http://127.0.0.1:%d", e.port)
	cmd := exec.Command(bin, "runner", "serve",
		"--host", "127.0.0.1", "--port", fmt.Sprint(e.port), "--workspace", e.workspace)
	cmd.Dir = e.workspace
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("runner serve start: %v", err)
	}
	e.proc = cmd
	deadline := time.Now().Add(30 * time.Second)
	for {
		resp, err := e.http.Get(e.base + "/health")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("runner did not reach /health within 30s")
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func (e *cp89LiveEnv) stopServer(t *testing.T) {
	t.Helper()
	if e.proc == nil {
		return
	}
	_ = e.proc.Process.Kill()
	_ = e.proc.Wait()
	e.proc = nil
	// Wait for the port to actually free before a restart binds it.
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", e.port), 200*time.Millisecond)
		if err != nil {
			return
		}
		conn.Close()
		time.Sleep(200 * time.Millisecond)
	}
}

// cp89Req performs one JSON call and returns (status, decoded body).
func (e *cp89LiveEnv) cp89Req(t *testing.T, method, path string, body any) (int, map[string]any) {
	t.Helper()
	var rdr *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		rdr = bytes.NewReader(raw)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, e.base+path, rdr)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	// Desktop/TUI client marker — the vibe working-mode gate (L-7/L-8) only
	// serves Desktop/TUI clients; dev-mode rows accept it harmlessly.
	req.Header.Set("X-Client", "tui")
	resp, err := e.http.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	var out map[string]any
	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(&out); err != nil {
		out = map[string]any{}
	}
	return resp.StatusCode, out
}

func cp89ErrCode(body map[string]any) string {
	if e, ok := body["error"].(map[string]any); ok {
		if c, ok := e["code"].(string); ok {
			return c
		}
	}
	return ""
}

// cp89SessionRows parses the durable sessions.ndjson on the workspace.
func (e *cp89LiveEnv) cp89SessionRows(t *testing.T) []map[string]any {
	t.Helper()
	path := filepath.Join(e.workspace, ".flowpilot", "chats", "sessions.ndjson")
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var rows []map[string]any
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	for sc.Scan() {
		var row map[string]any
		if json.Unmarshal(sc.Bytes(), &row) == nil {
			rows = append(rows, row)
		}
	}
	return rows
}

// cp89LastRow returns the LAST durable row for runID (upserts append).
func (e *cp89LiveEnv) cp89LastRow(t *testing.T, runID string) map[string]any {
	t.Helper()
	var last map[string]any
	for _, r := range e.cp89SessionRows(t) {
		if r["run_id"] == runID {
			last = r
		}
	}
	return last
}

func (e *cp89LiveEnv) cp89ChildRows(t *testing.T, parentRunID string) []map[string]any {
	t.Helper()
	var out []map[string]any
	seen := map[string]bool{}
	for _, r := range e.cp89SessionRows(t) {
		if r["parent_run_id"] == parentRunID {
			if id, _ := r["run_id"].(string); id != "" && !seen[id] {
				seen[id] = true
				out = append(out, r)
			}
		}
	}
	return out
}

// cp89StartRun POSTs /client/workflow-runs and returns the run handle body.
func (e *cp89LiveEnv) cp89StartRun(t *testing.T, body map[string]any) (int, map[string]any, string) {
	t.Helper()
	if body["providerKey"] == nil {
		body["providerKey"] = e.provider
	}
	if body["projectId"] == nil {
		body["projectId"] = "cp89-live"
	}
	if body["chatMode"] == nil {
		body["chatMode"] = "normal_chat"
	}
	code, resp := e.cp89Req(t, http.MethodPost, "/client/workflow-runs", body)
	runID, _ := resp["runId"].(string)
	if sid, _ := resp["stepId"].(string); runID != "" && sid != "" {
		cp89StepIDs[runID] = sid
	}
	return code, resp, runID
}

// cp89StepID returns the run's chat step id captured at create (RunHandle.stepId,
// "chat" for normal-chat runs — the same value the desktop sends per turn).
var cp89StepIDs = map[string]string{}

func (e *cp89LiveEnv) cp89Turn(t *testing.T, runID string, body map[string]any) (int, map[string]any) {
	t.Helper()
	if body["stepId"] == nil {
		if sid := cp89StepIDs[runID]; sid != "" {
			body["stepId"] = sid
		} else {
			body["stepId"] = "chat"
		}
	}
	// Run status flips to idle the moment the provider turn completes, but the
	// post-turn gate trail can still be running — and a freshly-resumed run
	// can still hold turnInFlight while its recovery turn launches. Both
	// 409s are transient; retry them through. Every other status surfaces
	// immediately (typed 422s are never 409s).
	deadline := time.Now().Add(2 * time.Minute)
	for {
		code, resp := e.cp89Req(t, http.MethodPost, "/client/workflow-runs/"+runID+"/turns", body)
		c := cp89ErrCode(resp)
		if code != http.StatusConflict || (c != "gate_in_progress" && c != "turn_in_progress") || time.Now().After(deadline) {
			return code, resp
		}
		time.Sleep(1500 * time.Millisecond)
	}
}

// cp89WaitRun polls GET /client/workflow-runs/{id} until pred(status) or timeout.
func (e *cp89LiveEnv) cp89WaitRun(t *testing.T, runID string, timeout time.Duration, pred func(status string) bool, what string) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var status string
	for time.Now().Before(deadline) {
		code, body := e.cp89Req(t, http.MethodGet, "/client/workflow-runs/"+runID, nil)
		if code == http.StatusOK {
			status, _ = body["status"].(string)
			if pred(status) {
				return status
			}
		}
		time.Sleep(1500 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s on run %s (last status %q)", what, runID, status)
	return status
}

func cp89TurnSettled(status string) bool {
	switch status {
	case "idle", "completed", "waiting_question", "waiting_approval", "waiting_user_approval", "blocked", "failed", "cancelled":
		return true
	}
	return false
}

// cp89ResolveParked drains parked user gates on a run — a provider-driven chat
// turn can end waiting_approval/waiting_question, and the next turn is then
// fenced by awaiting_user. Approving/answering through the REAL decision
// endpoints unblocks the run exactly like a desktop operator would.
func (e *cp89LiveEnv) cp89ResolveParked(t *testing.T, runID string) {
	t.Helper()
	for i := 0; i < 8; i++ {
		code, body := e.cp89Req(t, http.MethodGet, "/client/workflow-runs/"+runID, nil)
		if code != http.StatusOK {
			return
		}
		status, _ := body["status"].(string)
		if ap, ok := body["pendingApproval"].(map[string]any); ok {
			id, _ := ap["approvalId"].(string)
			if id == "" {
				id, _ = ap["approvalID"].(string)
			}
			dc, dr := e.cp89Req(t, http.MethodPost, "/client/approvals/"+id+"/decision",
				map[string]any{"decision": "approve"})
			t.Logf("resolved parked approval %s on %s: %d %v", id, runID, dc, dr)
			time.Sleep(time.Second)
			continue
		}
		if q, ok := body["pendingQuestion"].(map[string]any); ok {
			id, _ := q["questionId"].(string)
			choice := "ok"
			if opts, ok := q["options"].([]any); ok && len(opts) > 0 {
				// QuestionOption {label, value?} — pick the first option's
				// value (or label when value is absent), exactly like a
				// desktop clicking the first choice.
				switch o := opts[0].(type) {
				case map[string]any:
					if v, _ := o["value"].(string); v != "" {
						choice = v
					} else if l, _ := o["label"].(string); l != "" {
						choice = l
					}
				case string:
					if o != "" {
						choice = o
					}
				}
			}
			dc, dr := e.cp89Req(t, http.MethodPost, "/client/questions/"+id+"/answer",
				map[string]any{"choice": choice})
			t.Logf("answered parked question %s on %s with %q: %d %v", id, runID, choice, dc, dr)
			time.Sleep(time.Second)
			continue
		}
		if cp89TurnSettled(status) {
			return
		}
		return
	}
}

// cp89WaitChild waits until a durable child row appears under parentRunID.
func (e *cp89LiveEnv) cp89WaitChild(t *testing.T, parentRunID string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(cp89FlowWaitTimeout)
	for time.Now().Before(deadline) {
		if kids := e.cp89ChildRows(t, parentRunID); len(kids) > 0 {
			return kids[0]
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("no child run spawned under %s within %v", parentRunID, cp89FlowWaitTimeout)
	return nil
}

func TestCP89Live(t *testing.T) {
	if os.Getenv("LIVE") != "1" {
		t.Skip("CP-89 live harness: set LIVE=1 to run against a real runner + provider")
	}
	provider, missing := cp89DetectProvider(os.Getenv("CP89_LIVE_PROVIDER"))
	if provider == "" {
		t.Skipf("no provider binary found (checked: %s) — live parity claim requires a logged-in provider CLI", strings.Join(missing, ", "))
	}
	alt, _ := cp89DetectProvider(os.Getenv("CP89_LIVE_PROVIDER_ALT"), provider)

	ws := os.Getenv("CP89_LIVE_WORKSPACE")
	if ws == "" {
		ws = t.TempDir()
	}
	env := &cp89LiveEnv{
		workspace: ws,
		provider:  provider,
		alt:       alt,
		http:      &http.Client{Timeout: 60 * time.Second},
	}
	t.Logf("CP-89 live: provider=%s alt=%q workspace=%s", provider, alt, ws)
	bin := cp89BuildBinary(t)
	env.startServer(t, bin)
	defer env.stopServer(t)

	flowRef := "task-harness"

	// Stage a CP-shaped source doc inside the workspace for L-7/L-8 — the
	// ingest fence requires requirements/07-Coding-Plan/**/CP-*.md with a
	// `Document ID: CP-*` line, read from the run's workspace.
	cpDocRel := "requirements/07-Coding-Plan/note/CP-89-note.md"
	// Test CWD is apps/local-runner/internal/runner — repo root is four levels up.
	repoCP := filepath.Join("..", "..", "..", "..", "requirements", "07-Coding-Plan", "note", "CP-89-note.md")
	if raw, err := os.ReadFile(repoCP); err == nil {
		dst := filepath.Join(ws, filepath.FromSlash(cpDocRel))
		_ = os.MkdirAll(filepath.Dir(dst), 0o755)
		_ = os.WriteFile(dst, raw, 0o644)
	}

	// L-1: immediate is unchanged — a pinned flow starts on the first turn.
	t.Run("L-1_immediate_unchanged", func(t *testing.T) {
		code, body, runID := env.cp89StartRun(t, map[string]any{"flowRef": flowRef})
		if code != http.StatusOK && code != http.StatusCreated {
			t.Fatalf("createRun immediate: %d %v", code, body)
		}
		code, body = env.cp89Turn(t, runID, map[string]any{"prompt": "Reply with exactly one word: OK"})
		if code != http.StatusOK {
			t.Fatalf("turn 1: %d %v", code, body)
		}
		kid := env.cp89WaitChild(t, runID)
		t.Logf("L-1: run=%s entry child=%s agent=%v", runID, kid["run_id"], kid["agent_name"])
		row := env.cp89LastRow(t, runID)
		if row["flow_arm"] != nil && row["flow_arm"] != "immediate" {
			t.Fatalf("immediate run flow_arm=%v (must be absent/immediate)", row["flow_arm"])
		}
	})

	// L-2: pending stays chat — three plain turns, no child, latch durable.
	t.Run("L-2_pending_chat_is_chat", func(t *testing.T) {
		code, body, runID := env.cp89StartRun(t, map[string]any{
			"flowRef": flowRef, "flowArm": "pending",
		})
		if code != http.StatusOK && code != http.StatusCreated {
			t.Fatalf("createRun pending: %d %v", code, body)
		}
		for i := 1; i <= 3; i++ {
			code, body = env.cp89Turn(t, runID, map[string]any{
				"prompt": fmt.Sprintf("Turn %d: reply with exactly one word: OK", i),
			})
			if code != http.StatusOK {
				t.Fatalf("chat turn %d: %d %v", i, code, body)
			}
			env.cp89WaitRun(t, runID, cp89PollTurnTimeout, cp89TurnSettled, "chat turn settle")
			env.cp89ResolveParked(t, runID)
		}
		row := env.cp89LastRow(t, runID)
		if row["flow_arm"] != "pending" {
			t.Fatalf("sessions.ndjson flow_arm=%v, want pending", row["flow_arm"])
		}
		if nodes, ok := row["active_flow_nodes"]; ok && len(fmt.Sprint(nodes)) > 4 {
			t.Fatalf("pending run shows active_flow_nodes=%v — flow machinery armed", nodes)
		}
		if kids := env.cp89ChildRows(t, runID); len(kids) != 0 {
			t.Fatalf("pending run spawned %d children — no flow should be running", len(kids))
		}
		t.Logf("L-2: run=%s stayed chat through 3 turns", runID)
	})

	// L-3 + L-5: forward starts the flow; a second forward is 422.
	var l3RunID string
	t.Run("L-3_forward_starts_flow", func(t *testing.T) {
		code, body, runID := env.cp89StartRun(t, map[string]any{
			"flowRef": flowRef, "flowArm": "pending",
		})
		if code != http.StatusOK && code != http.StatusCreated {
			t.Fatalf("createRun pending: %d %v", code, body)
		}
		code, body = env.cp89Turn(t, runID, map[string]any{"prompt": "Reply with exactly one word: OK"})
		if code != http.StatusOK {
			t.Fatalf("chat turn: %d %v", code, body)
		}
		env.cp89WaitRun(t, runID, cp89PollTurnTimeout, cp89TurnSettled, "chat settle")
		env.cp89ResolveParked(t, runID)
		code, body = env.cp89Turn(t, runID, map[string]any{
			"forwardFlow": true, "prompt": "Chốt: forward-marker-cp89",
		})
		if code != http.StatusOK {
			t.Fatalf("forward turn: %d %v", code, body)
		}
		kid := env.cp89WaitChild(t, runID)
		l3RunID = runID
		deadline := time.Now().Add(15 * time.Second)
		for {
			row := env.cp89LastRow(t, runID)
			if row["flow_arm"] == "started" {
				t.Logf("L-3: run=%s flow_arm=started, entry child=%s", runID, kid["run_id"])
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("sessions.ndjson flow_arm=%v after forward, want started", row["flow_arm"])
			}
			time.Sleep(time.Second)
		}
	})

	// L-4: bare forward — no prompt — is actionable, not a 400.
	t.Run("L-4_bare_forward", func(t *testing.T) {
		code, body, runID := env.cp89StartRun(t, map[string]any{
			"flowRef": flowRef, "flowArm": "pending",
		})
		if code != http.StatusOK && code != http.StatusCreated {
			t.Fatalf("createRun pending: %d %v", code, body)
		}
		code, body = env.cp89Turn(t, runID, map[string]any{"forwardFlow": true})
		if code != http.StatusOK {
			t.Fatalf("bare forward must be a turn, not 400: %d %v", code, body)
		}
		kid := env.cp89WaitChild(t, runID)
		t.Logf("L-4: run=%s bare forward started flow, child=%s", runID, kid["run_id"])
	})

	t.Run("L-5_double_forward", func(t *testing.T) {
		if l3RunID == "" {
			t.Skip("depends on L-3 run")
		}
		// A second forward while flow children are live hits the hub-parked
		// turn gate (409) before it can reach the latch — the typed
		// flow_already_started is only reachable once the flow settles. Poll
		// (non-fatal) for a settled/blocked state first; if the flow is still
		// live at budget end the typed-422 assertion is covered by units.
		settleBudget := 4 * time.Minute
		deadline := time.Now().Add(settleBudget)
		st := ""
		for time.Now().Before(deadline) {
			code, snap := env.cp89Req(t, http.MethodGet, "/client/workflow-runs/"+l3RunID, nil)
			if code == http.StatusOK {
				st, _ = snap["status"].(string)
				if cp89TurnSettled(st) {
					break
				}
			}
			time.Sleep(3 * time.Second)
		}
		code, body := env.cp89Turn(t, l3RunID, map[string]any{"forwardFlow": true})
		if code == http.StatusConflict && cp89ErrCode(body) == "hub_parked" {
			t.Skipf("run %s still parked after %v (status=%s) — double-forward covered by TestTask452_ForwardOnStarted", l3RunID, settleBudget, st)
		}
		if code != http.StatusUnprocessableEntity || cp89ErrCode(body) != "flow_already_started" {
			t.Fatalf("second forward = %d %v, want 422 flow_already_started", code, body)
		}
		if kids := env.cp89ChildRows(t, l3RunID); len(kids) > 1 {
			seenAgents := map[string]bool{}
			dup := false
			for _, k := range kids {
				a, _ := k["agent_name"].(string)
				if seenAgents[a] {
					dup = true
				}
				seenAgents[a] = true
			}
			if dup {
				t.Fatalf("second forward spawned a duplicate entry child: %v", seenAgents)
			}
		}
		t.Logf("L-5: run=%s second forward 422 flow_already_started", l3RunID)
	})

	// L-6: forward without a pin is a typed 422.
	t.Run("L-6_forward_requires_pin", func(t *testing.T) {
		code, body, runID := env.cp89StartRun(t, map[string]any{})
		if code != http.StatusOK && code != http.StatusCreated {
			t.Fatalf("createRun chat: %d %v", code, body)
		}
		code, body = env.cp89Turn(t, runID, map[string]any{"forwardFlow": true})
		if code != http.StatusUnprocessableEntity || cp89ErrCode(body) != "forward_requires_flow_pin" {
			t.Fatalf("forward w/o pin = %d %v, want 422 forward_requires_flow_pin", code, body)
		}
		t.Logf("L-6: run=%s → 422 forward_requires_flow_pin", runID)
	})

	// L-7: the CP ingest fence runs at forward time, not pin time; a failed
	// fence leaves the latch pending so fixing the source + re-forwarding works.
	t.Run("L-7_cp_fence_at_forward", func(t *testing.T) {
		if _, err := os.Stat(filepath.Join(ws, filepath.FromSlash(cpDocRel))); err != nil {
			t.Skipf("CP source doc not staged (%v) — L-7 needs a real CP-shaped file", err)
		}
		code, body, runID := env.cp89StartRun(t, map[string]any{
			"flowRef": "vibe-cp-ingest", "flowArm": "pending", "workingMode": "vibe",
		})
		if code != http.StatusOK && code != http.StatusCreated {
			t.Fatalf("createRun vibe pending: %d %v", code, body)
		}
		code, body = env.cp89Turn(t, runID, map[string]any{"prompt": "Reply with exactly one word: OK"})
		if code != http.StatusOK {
			t.Fatalf("chat turn w/o source must pass (fence is forward-time): %d %v", code, body)
		}
		env.cp89WaitRun(t, runID, cp89PollTurnTimeout, cp89TurnSettled, "chat settle")
		env.cp89ResolveParked(t, runID)
		code, body = env.cp89Turn(t, runID, map[string]any{"forwardFlow": true})
		if code != http.StatusUnprocessableEntity || cp89ErrCode(body) != "invalid_cp_source" {
			t.Fatalf("forward w/o source = %d %v, want 422 invalid_cp_source", code, body)
		}
		row := env.cp89LastRow(t, runID)
		if row["flow_arm"] != "pending" {
			t.Fatalf("failed fence consumed the latch: flow_arm=%v", row["flow_arm"])
		}
		code, body = env.cp89Turn(t, runID, map[string]any{
			"forwardFlow": true, "sourceDocId": cpDocRel,
		})
		if code != http.StatusOK {
			t.Fatalf("forward with sourceDocId: %d %v", code, body)
		}
		kid := env.cp89WaitChild(t, runID)
		t.Logf("L-7: run=%s fence 422 → fixed → started (child=%s)", runID, kid["run_id"])
	})

	// L-8: a create-time sourceDocId pin satisfies the forward fence — no repaste.
	t.Run("L-8_pinned_source_doc", func(t *testing.T) {
		if _, err := os.Stat(filepath.Join(ws, filepath.FromSlash(cpDocRel))); err != nil {
			t.Skipf("CP source doc not staged (%v)", err)
		}
		code, body, runID := env.cp89StartRun(t, map[string]any{
			"flowRef": "vibe-cp-ingest", "flowArm": "pending", "workingMode": "vibe",
			"sourceDocId": cpDocRel,
		})
		if code != http.StatusOK && code != http.StatusCreated {
			t.Fatalf("createRun vibe pending w/ pin: %d %v", code, body)
		}
		code, body = env.cp89Turn(t, runID, map[string]any{"forwardFlow": true})
		if code != http.StatusOK {
			t.Fatalf("forward with pinned source must pass: %d %v", code, body)
		}
		kid := env.cp89WaitChild(t, runID)
		t.Logf("L-8: run=%s pinned source → started (child=%s)", runID, kid["run_id"])
	})

	// L-9: kill the process mid-pending; the latch must survive in
	// sessions.ndjson and forward must still work after restart.
	t.Run("L-9_restart_pending", func(t *testing.T) {
		code, body, runID := env.cp89StartRun(t, map[string]any{
			"flowRef": flowRef, "flowArm": "pending",
		})
		if code != http.StatusOK && code != http.StatusCreated {
			t.Fatalf("createRun pending: %d %v", code, body)
		}
		for i := 0; i < 2; i++ {
			code, body = env.cp89Turn(t, runID, map[string]any{
				"prompt": fmt.Sprintf("Turn %d: reply with exactly one word: OK", i),
			})
			if code != http.StatusOK {
				t.Fatalf("chat turn %d: %d %v", i, code, body)
			}
			env.cp89WaitRun(t, runID, cp89PollTurnTimeout, cp89TurnSettled, "chat settle")
			env.cp89ResolveParked(t, runID)
		}
		env.stopServer(t)
		env.startServer(t, bin)
		code, body = env.cp89Req(t, http.MethodPost, "/client/workflow-runs/"+runID+"/resume", nil)
		if code != http.StatusOK {
			t.Fatalf("resume after kill: %d %v", code, body)
		}
		env.cp89WaitRun(t, runID, 90*time.Second, func(s string) bool { return s != "" }, "reconstruct")
		row := env.cp89LastRow(t, runID)
		if row["flow_arm"] != "pending" {
			t.Fatalf("post-restart flow_arm=%v — latch lost (must stay pending)", row["flow_arm"])
		}
		if kids := env.cp89ChildRows(t, runID); len(kids) != 0 {
			t.Fatalf("reconstructed pending run has %d children — flow must not auto-start", len(kids))
		}
		code, body = env.cp89Turn(t, runID, map[string]any{"forwardFlow": true, "prompt": "forward after restart"})
		if code != http.StatusOK {
			t.Fatalf("forward after restart: %d %v", code, body)
		}
		kid := env.cp89WaitChild(t, runID)
		t.Logf("L-9: run=%s survived restart pending → forwarded (child=%s)", runID, kid["run_id"])
	})

	// L-10: kill after started → the run reconstructs as a normal flow run;
	// no re-arm, BUG-315 holds.
	t.Run("L-10_restart_started", func(t *testing.T) {
		code, body, runID := env.cp89StartRun(t, map[string]any{
			"flowRef": flowRef, "flowArm": "pending",
		})
		if code != http.StatusOK && code != http.StatusCreated {
			t.Fatalf("createRun pending: %d %v", code, body)
		}
		code, body = env.cp89Turn(t, runID, map[string]any{"forwardFlow": true})
		if code != http.StatusOK {
			t.Fatalf("forward: %d %v", code, body)
		}
		kid := env.cp89WaitChild(t, runID)
		env.stopServer(t)
		env.startServer(t, bin)
		code, body = env.cp89Req(t, http.MethodPost, "/client/workflow-runs/"+runID+"/resume", nil)
		if code != http.StatusOK {
			t.Fatalf("resume: %d %v", code, body)
		}
		env.cp89WaitRun(t, runID, 90*time.Second, func(s string) bool { return s != "" }, "reconstruct")
		row := env.cp89LastRow(t, runID)
		if row["flow_arm"] == "pending" {
			t.Fatalf("post-restart flow_arm reverted to pending — started latch must ride")
		}
		if kids := env.cp89ChildRows(t, runID); len(kids) == 0 {
			t.Fatalf("flow children lost after restart — expected child %s rows", kid["run_id"])
		}
		t.Logf("L-10: run=%s reconstructed as started flow run", runID)
	})

	// L-11: a provider switch mints a new leg that keeps pending; forward then
	// starts the pinned flow on the new leg.
	t.Run("L-11_provider_switch_pending", func(t *testing.T) {
		if env.alt == "" {
			var checked []string
			for _, b := range cp89ProviderBinaries() {
				if b != env.provider {
					checked = append(checked, b)
				}
			}
			t.Skipf("no second provider binary for leg switch (checked: %s) — set CP89_LIVE_PROVIDER_ALT", strings.Join(checked, ", "))
		}
		code, body, runID := env.cp89StartRun(t, map[string]any{
			"flowRef": flowRef, "flowArm": "pending",
		})
		if code != http.StatusOK && code != http.StatusCreated {
			t.Fatalf("createRun pending: %d %v", code, body)
		}
		chatID, _ := body["chatId"].(string)
		if chatID == "" {
			t.Fatalf("run %s create response exposes no chatId for switch (%v)", runID, body)
		}
		code, body = env.cp89Req(t, http.MethodPost, "/client/chats/"+chatID+"/switch-provider",
			map[string]any{"targetProviderKey": env.alt})
		if code != http.StatusOK {
			if code == http.StatusBadGateway || code == http.StatusUnprocessableEntity {
				t.Skipf("alt provider %s unavailable at runtime (account/quota): %d %v — leg-switch path unit-covered", env.alt, code, body)
			}
			t.Fatalf("switch-provider → %s: %d %v", env.alt, code, body)
		}
		handle, _ := body["handle"].(map[string]any)
		newRunID, _ := handle["runId"].(string)
		if newRunID == "" || newRunID == runID {
			t.Fatalf("switch must mint a new leg runId, got %v", handle)
		}
		row := env.cp89LastRow(t, newRunID)
		if row["flow_arm"] != "pending" {
			t.Fatalf("new leg flow_arm=%v — latch must ride the leg switch", row["flow_arm"])
		}
		code, body = env.cp89Turn(t, newRunID, map[string]any{"forwardFlow": true})
		if code != http.StatusOK {
			if c := cp89ErrCode(body); strings.HasPrefix(c, "quota_") || c == "account_unavailable" || c == "account_not_signed_in" {
				t.Skipf("latch rode the leg switch (flow_arm=pending on %s) but alt provider %s cannot execute the forward: %d %s", newRunID, env.alt, code, c)
			}
			t.Fatalf("forward on new leg: %d %v", code, body)
		}
		kid := env.cp89WaitChild(t, newRunID)
		t.Logf("L-11: %s→%s leg=%s kept pending → forwarded (child=%s)", env.provider, env.alt, newRunID, kid["run_id"])
	})

	// L-12: pack flow definitions are go:embed'd into the binary — there is no
	// live seam to corrupt one under a running server. Unit-covered by
	// TestTask452_CorruptDefinitionFailsClosed.
	t.Run("L-12_corrupt_definition", func(t *testing.T) {
		t.Skip("flow defs are go:embed'd — no runtime file to corrupt; covered by TestTask452_CorruptDefinitionFailsClosed")
	})

	// L-13: a pending run that is never forwarded ends as a plain chat run —
	// no flow rows, no children.
	t.Run("L-13_never_forwarded", func(t *testing.T) {
		code, body, runID := env.cp89StartRun(t, map[string]any{
			"flowRef": flowRef, "flowArm": "pending",
		})
		if code != http.StatusOK && code != http.StatusCreated {
			t.Fatalf("createRun pending: %d %v", code, body)
		}
		code, body = env.cp89Turn(t, runID, map[string]any{"prompt": "Reply with exactly one word: OK"})
		if code != http.StatusOK {
			t.Fatalf("chat turn: %d %v", code, body)
		}
		env.cp89WaitRun(t, runID, cp89PollTurnTimeout, cp89TurnSettled, "chat settle")
		code, body = env.cp89Req(t, http.MethodDelete, "/client/workflow-runs/"+runID, nil)
		if code != http.StatusOK && code != http.StatusNoContent && code != http.StatusAccepted {
			t.Fatalf("stop run: %d %v", code, body)
		}
		if kids := env.cp89ChildRows(t, runID); len(kids) != 0 {
			t.Fatalf("never-forwarded run spawned %d children", len(kids))
		}
		row := env.cp89LastRow(t, runID)
		if nodes, ok := row["active_flow_nodes"]; ok && len(fmt.Sprint(nodes)) > 4 {
			t.Fatalf("never-forwarded run carries active_flow_nodes=%v", nodes)
		}
		t.Logf("L-13: run=%s ended as chat run, flow_arm=%v", runID, row["flow_arm"])
	})
}
