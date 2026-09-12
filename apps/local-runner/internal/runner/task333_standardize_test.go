package runner

// Task-333 §10 TDD signatures — /standardize command + SS-Lock gate (CP-49).
//
// Every test runs inside a t.TempDir() sandbox pinned via
// SetStandardizeWorkspaceRoot; this repo's own requirements/ tree is never
// touched. GitNexus is stubbed through the runGitNexusCLI hook so evidence
// collection is deterministic offline (the real CLI degradation path is
// exercised in TestStandardize_GitNexusUnavailable_FallsBackToStaticScan).
//
// Provider-agnostic parity: all behavior under test is deterministic Go +
// subprocess evidence; no LLM participates in the /standardize code path, so
// these tests are identical for Claude/Codex/Grok sessions by construction.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"flowpilot-runner/internal/docscan"
)

// newStandardizeTestService builds a service pinned to a fresh sandbox and
// stubs the GitNexus CLI subprocess.
func newStandardizeTestService(t *testing.T, nexusOutput string, nexusErr error) (*InteractiveService, string) {
	t.Helper()
	root := t.TempDir()
	svc := NewInteractiveService()
	svc.SetStandardizeWorkspaceRoot(root)
	stubGitNexus(t, nexusOutput, nexusErr)
	return svc, root
}

// stubGitNexus swaps the runGitNexusCLI hook for the duration of the test.
func stubGitNexus(t *testing.T, output string, err error) {
	t.Helper()
	orig := runGitNexusCLI
	runGitNexusCLI = func(ctx context.Context, dir string, args ...string) ([]byte, error) {
		if err != nil {
			return []byte("npx gitnexus exited non-zero"), err
		}
		return []byte(output), nil
	}
	t.Cleanup(func() { runGitNexusCLI = orig })
}

// gitnexusFixtureJSON is a deterministic GitNexus query payload exercising the
// graph-parse path (processes = execution flows, definitions = symbols).
const gitnexusFixtureJSON = `{
  "processes": [
    {"name": "Standardize Command Flow"},
    {"name": "SS-Lock Gate Flow"}
  ],
  "definitions": [
    {"id": "Function:features/device/device.go:NewDeviceRunner", "name": "NewDeviceRunner", "filePath": "features/device/device.go"},
    {"id": "Struct:features/device/device.go:DeviceRunner", "name": "DeviceRunner", "filePath": "features/device/device.go"}
  ]
}`

// writeSandboxFile creates a file (with parent dirs) inside the sandbox.
func writeSandboxFile(t *testing.T, root, rel, content string) string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// sampleSSDoc is a deliberately non-conforming SS doc (no AI Quick View) so
// the conformance report has real issues to report.
const sampleSSDoc = `# SS-01: Auth

## Metadata

- Document ID: ` + "`SS-01-Auth`" + `
- Title: ` + "`Auth`" + `
- Phase: ` + "`ss`" + `
- Status: ` + "`approved`" + `
- Owner: ` + "`FlowPilot`" + `
- Reviewers: ` + "`Operator`" + `
- Created: ` + "`2026-01-01`" + `
- Last Updated: ` + "`2026-01-01`" + `
- Parent Documents: ` + "`None`" + `
- Child Documents: ` + "`None`" + `
- Related Documents: ` + "`None`" + `
- Replaces: ` + "`None`" + `
- Feature Keys: ` + "`auth`" + `
- Tags: ` + "`auth`" + `

## 1. Goal

- Login works.
`

// deviceFixtureGo is the brownfield source the static/AST fallback scans.
const deviceFixtureGo = `package device

// DeviceRunner runs the device loop.
type DeviceRunner struct{}

// Run starts the device loop.
func (r *DeviceRunner) Run() error { return nil }

// NewDeviceRunner builds a DeviceRunner.
func NewDeviceRunner() *DeviceRunner { return &DeviceRunner{} }

// registerRoutes example: mux.HandleFunc("GET /client/device/status", nil)
`

// newBrownfieldRun executes /standardize on a docs-less sandbox and returns
// the parked result plus the gate snapshot.
func newBrownfieldRun(t *testing.T, svc *InteractiveService) (*StandardizeResult, *SSLockSnapshot) {
	t.Helper()
	res, err := svc.ExecuteStandardize(context.Background(), StandardizeScope{Path: "features/device"})
	if err != nil {
		t.Fatalf("ExecuteStandardize: %v", err)
	}
	if res.RunID == "" {
		t.Fatalf("expected a run id for the SS-Lock pause, got none")
	}
	snap, ok := svc.SSLockSnapshot(res.RunID)
	if !ok {
		t.Fatalf("no SS-Lock snapshot for run %q", res.RunID)
	}
	return res, snap
}

// countMarkdownFiles walks root and counts regular .md files.
func countMarkdownFiles(t *testing.T, root string) int {
	t.Helper()
	n := 0
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.EqualFold(filepath.Ext(path), ".md") {
			n++
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// Scenario: Chạy /standardize trên scope đã có doc -> Chuyển hướng sang CP-48 Conformance
// Input: Scope="features/auth", đã tồn tại requirements/05-System-Specs/SS-01-Auth.md
// Expect: Trả về báo cáo scan của CP-48, không chạy reverse doc
func TestStandardize_ExistingDocs_RoutesToConformance(t *testing.T) {
	svc, root := newStandardizeTestService(t, gitnexusFixtureJSON, nil)
	ssDoc := writeSandboxFile(t, root, "requirements/05-System-Specs/SS-01-Auth.md", sampleSSDoc)
	writeSandboxFile(t, root, "requirements/06-System-Tech-Design/SD-01-Auth.md", sampleSSDoc)
	writeSandboxFile(t, root, "features/auth/login.go", "package auth\n")
	before, err := os.ReadFile(ssDoc)
	if err != nil {
		t.Fatal(err)
	}

	res, err := svc.ExecuteStandardize(context.Background(), StandardizeScope{Path: "features/auth"})
	if err != nil {
		t.Fatalf("ExecuteStandardize: %v", err)
	}
	if res.Mode != StandardizeModeConformance {
		t.Errorf("Mode = %q, want %q", res.Mode, StandardizeModeConformance)
	}
	if res.Status != StandardizeStatusCompleted {
		t.Errorf("Status = %q, want completed (no SS-Lock pause when docs exist)", res.Status)
	}
	if res.ScanReport == nil {
		t.Fatalf("ScanReport = nil, want a CP-48 conformance report")
	}
	if res.ScanReport.TotalFilesScanned != 2 {
		t.Errorf("TotalFilesScanned = %d, want 2 (SS + SD)", res.ScanReport.TotalFilesScanned)
	}
	if len(res.ScanReport.Issues) == 0 {
		t.Errorf("expected the non-conforming sample docs to produce issues")
	}
	if res.RunID != "" || res.DraftSSPath != "" || res.DraftSDPath != "" {
		t.Errorf("conformance branch must not reverse-doc: run=%q ss=%q sd=%q", res.RunID, res.DraftSSPath, res.DraftSDPath)
	}
	// CP-49 constraint: approved (phase-root) documents are never rewritten.
	after, err := os.ReadFile(ssDoc)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Errorf("approved SS doc was modified by the conformance branch")
	}
	// No SS-Lock gate may exist for any run of this service.
	ssLockGatesMu.Lock()
	defer ssLockGatesMu.Unlock()
	for k := range ssLockGates {
		if k.svc == svc {
			t.Errorf("unexpected SS-Lock gate registered for run %q on a conformance run", k.runID)
		}
	}
}

// Scenario: Chạy /standardize trên scope chưa có doc -> Kích hoạt reverse-doc và dừng tại SS-Lock
// Input: Scope="features/device", chưa có bất kỳ doc nào trong requirements/
// Expect: Sinh draft SD và draft SS; trạng thái run chuyển sang "waiting_user_confirm"; event SS-Lock được phát ra
func TestStandardize_BrownfieldScope_PausesAtSSLock(t *testing.T) {
	svc, root := newStandardizeTestService(t, gitnexusFixtureJSON, nil)
	writeSandboxFile(t, root, "features/device/device.go", deviceFixtureGo)

	res, snap := newBrownfieldRun(t, svc)
	if res.Mode != StandardizeModeReverseDoc {
		t.Errorf("Mode = %q, want %q", res.Mode, StandardizeModeReverseDoc)
	}
	if res.Status != StandardizeStatusWaitingSSLock {
		t.Errorf("Status = %q, want %q", res.Status, StandardizeStatusWaitingSSLock)
	}
	if snap.Status != "waiting_user_confirm" {
		t.Errorf("run status = %q, want waiting_user_confirm", snap.Status)
	}
	if !snap.State.Locked {
		t.Errorf("gate must be locked while waiting for the user")
	}
	// Drafts are written as todo/ drafts under the governed phase dirs.
	if !strings.Contains(filepath.ToSlash(res.DraftSSPath), "requirements/05-System-Specs/todo/SS-Device.md") {
		t.Errorf("DraftSSPath = %q, want under requirements/05-System-Specs/todo/", res.DraftSSPath)
	}
	if !strings.Contains(filepath.ToSlash(res.DraftSDPath), "requirements/06-System-Tech-Design/todo/SD-Device.md") {
		t.Errorf("DraftSDPath = %q, want under requirements/06-System-Tech-Design/todo/", res.DraftSDPath)
	}
	ssData, err := os.ReadFile(res.DraftSSPath)
	if err != nil {
		t.Fatalf("SS draft missing on disk: %v", err)
	}
	sdData, err := os.ReadFile(res.DraftSDPath)
	if err != nil {
		t.Fatalf("SD draft missing on disk: %v", err)
	}
	if !strings.Contains(string(ssData), "TODO: human intent needed") {
		t.Errorf("SS draft must carry TODO: human intent needed markers (anti-hallucination)")
	}
	if !strings.Contains(string(ssData), "Acceptance Criteria") {
		t.Errorf("SS draft must contain the Acceptance Criteria section")
	}
	if acIdx := strings.Index(string(ssData), "## 6. Acceptance Criteria"); acIdx < 0 ||
		!strings.Contains(string(ssData)[acIdx:], "TODO: human intent needed") {
		t.Errorf("Acceptance Criteria section must be TODO-flagged for the human owner")
	}
	if !strings.Contains(string(sdData), "NewDeviceRunner") {
		t.Errorf("SD draft must cite collected evidence symbols, got:\n%s", sdData)
	}
	// The SS-Lock event carries the draft for the client modal.
	found := false
	for _, ev := range snap.Events {
		if ev.Type == EventUserConfirmRequired {
			found = true
			if !strings.Contains(ev.DraftSSContent, "TODO: human intent needed") {
				t.Errorf("EventUserConfirmRequired payload must embed the draft SS content")
			}
		}
	}
	if !found {
		t.Errorf("EventUserConfirmRequired was not emitted")
	}
}

// Scenario: AI không thể bypass cổng SS-Lock khi chưa có confirm từ user
// Input: Cố gắng gọi turn tiếp theo trong khi run đang chờ SS-Lock
// Expect: Runner trả về lỗi 409 conflict / gate locked
func TestStandardize_SSLock_CannotBeBypassedByAI(t *testing.T) {
	svc, root := newStandardizeTestService(t, gitnexusFixtureJSON, nil)
	writeSandboxFile(t, root, "features/device/device.go", deviceFixtureGo)
	res, _ := newBrownfieldRun(t, svc)

	mux := http.NewServeMux()
	svc.RegisterInteractiveRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()

	// Bypass attempt: start a new AI/provider turn on the locked run.
	body, _ := json.Marshal(map[string]string{"prompt": "continue the workflow"})
	resp, err := http.Post(server.URL+"/client/workflow-runs/"+res.RunID+"/turns", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST turns: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("turn on SS-Locked run: status = %d, want 409 conflict", resp.StatusCode)
	}
	var errBody struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&errBody); err != nil {
		t.Fatalf("decode 409 body: %v", err)
	}
	if errBody.Error.Code != "ss_lock" {
		t.Errorf("409 code = %q, want ss_lock", errBody.Error.Code)
	}
	// Nothing was published by the bypass attempt.
	if published := countMarkdownFilesOutsideTodo(t, filepath.Join(svc.standardizeWorkspaceRoot(), "requirements")); published != 0 {
		t.Errorf("bypass attempt published %d doc(s) outside todo/", published)
	}
	// The gate stays locked.
	if snap, _ := svc.SSLockSnapshot(res.RunID); snap.Status != "waiting_user_confirm" {
		t.Errorf("gate status after bypass attempt = %q, want waiting_user_confirm", snap.Status)
	}
	// Negative control: the fence is per-run, not global.
	if e := svc.ssLockTurnFence("run-does-not-exist"); e != nil {
		t.Errorf("fence fired for an unknown run: %v", e)
	}
}

// Scenario: Người dùng xác nhận SS-Lock -> Xuất bản tài liệu chuẩn vào requirements/
// Input: Gửi POST /client/workflow-runs/{id}/confirm với nội dung SS đã chỉnh sửa
// Expect: Tài liệu chính thức được ghi vào đĩa và định dạng qua AutoFix
func TestStandardize_UserConfirm_PublishesFinalDocs(t *testing.T) {
	svc, root := newStandardizeTestService(t, gitnexusFixtureJSON, nil)
	writeSandboxFile(t, root, "features/device/device.go", deviceFixtureGo)
	res, _ := newBrownfieldRun(t, svc)

	// Human edits: raw SS content WITHOUT metadata/sections — AutoFix must
	// shape it on publication (T-5 handoff).
	edited := "# SS-Device: Device\n\n## 1. Goal\n\n- Keep devices paired and synced (human intent).\n\n## 6. Acceptance Criteria\n\n- Device pairs within 5 seconds.\n"
	mux := http.NewServeMux()
	svc.RegisterInteractiveRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()
	resp, err := http.Post(server.URL+"/client/workflow-runs/"+res.RunID+"/confirm", "application/json",
		bytes.NewBufferString(`{"edits": "`+strings.ReplaceAll(strings.ReplaceAll(edited, "\n", "\\n"), "\"", "\\\"")+`"}`))
	if err != nil {
		t.Fatalf("POST confirm: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("confirm status = %d, want 200: %s", resp.StatusCode, raw)
	}

	publishedSS := filepath.Join(root, "requirements", "05-System-Specs", "SS-Device.md")
	publishedSD := filepath.Join(root, "requirements", "06-System-Tech-Design", "SD-Device.md")
	ssData, err := os.ReadFile(publishedSS)
	if err != nil {
		t.Fatalf("official SS not published: %v", err)
	}
	if !strings.Contains(string(ssData), "Device pairs within 5 seconds.") {
		t.Errorf("published SS lost the human edits")
	}
	if !strings.Contains(string(ssData), "- Document ID:") {
		t.Errorf("published SS was not AutoFix-formatted (missing metadata skeleton)")
	}
	sdData, err := os.ReadFile(publishedSD)
	if err != nil {
		t.Fatalf("official SD not published: %v", err)
	}
	if !strings.Contains(string(sdData), "- Document ID:") {
		t.Errorf("published SD was not AutoFix-formatted")
	}
	// Drafts are consumed by publication.
	if _, err := os.Stat(res.DraftSSPath); !os.IsNotExist(err) {
		t.Errorf("SS draft still exists after publication")
	}
	if _, err := os.Stat(res.DraftSDPath); !os.IsNotExist(err) {
		t.Errorf("SD draft still exists after publication")
	}
	// Gate completed; the T-5 handoff reports a clean conformance scan.
	snap, ok := svc.SSLockSnapshot(res.RunID)
	if !ok {
		t.Fatal("gate snapshot vanished after confirm")
	}
	if snap.Status != "completed" {
		t.Errorf("gate status = %q, want completed", snap.Status)
	}
	if snap.ScanReport == nil || snap.ScanReport.TotalFilesScanned != 2 || len(snap.ScanReport.Issues) != 0 {
		t.Errorf("T-5 handoff scan = %+v, want 2 conforming files and 0 issues", snap.ScanReport)
	}
	// The gate is one-shot: a second confirm must fail.
	if err := svc.HandleSSLockConfirm(context.Background(), res.RunID, ""); err == nil {
		t.Errorf("second confirm succeeded; the SS-Lock gate must be one-shot")
	}
}

// [Edge] Scenario: /standardize không có tham số -> Chạy cho toàn bộ project
// Input: Scope rỗng
// Expect: Quét toàn bộ requirements/, mode="conformance" cho doc đã có
func TestStandardize_EmptyScope_RunsFullProject(t *testing.T) {
	svc, root := newStandardizeTestService(t, gitnexusFixtureJSON, nil)
	writeSandboxFile(t, root, "requirements/05-System-Specs/SS-01-Alpha.md", sampleSSDoc)
	writeSandboxFile(t, root, "requirements/08-Task/Task-900-Alpha.md", sampleSSDoc)

	res, err := svc.ExecuteStandardize(context.Background(), StandardizeScope{})
	if err != nil {
		t.Fatalf("ExecuteStandardize: %v", err)
	}
	if res.Mode != StandardizeModeConformance {
		t.Errorf("Mode = %q, want conformance for a documented project", res.Mode)
	}
	if res.ScanReport == nil {
		t.Fatalf("ScanReport = nil, want a full-tree CP-48 report")
	}
	if res.ScanReport.TotalFilesScanned != 2 {
		t.Errorf("TotalFilesScanned = %d, want the whole requirements/ tree (2 docs)", res.ScanReport.TotalFilesScanned)
	}
	if res.Status != StandardizeStatusCompleted {
		t.Errorf("Status = %q, want completed", res.Status)
	}
}

// [Edge] Scenario: Scope có doc lẫn lộn (SS tồn tại nhưng SD thiếu)
// Input: Scope="features/auth", tồn tại SS-01-Auth.md nhưng thiếu SD-*-Auth.md
// Expect: Chạy conformance trên SS, kích hoạt reverse-doc cho SD bị thiếu
func TestStandardize_PartialDocs_MixedMode(t *testing.T) {
	svc, root := newStandardizeTestService(t, gitnexusFixtureJSON, nil)
	writeSandboxFile(t, root, "requirements/05-System-Specs/SS-01-Auth.md", sampleSSDoc)
	writeSandboxFile(t, root, "features/auth/login.go", "package auth\n\n// Login authenticates a user.\nfunc Login(user string) bool { return user != \"\" }\n")

	res, err := svc.ExecuteStandardize(context.Background(), StandardizeScope{Path: "features/auth"})
	if err != nil {
		t.Fatalf("ExecuteStandardize: %v", err)
	}
	if res.Mode != StandardizeModeMixed {
		t.Errorf("Mode = %q, want mixed", res.Mode)
	}
	if res.ScanReport == nil || res.ScanReport.TotalFilesScanned != 1 {
		t.Errorf("ScanReport = %+v, want conformance on the existing SS (1 file)", res.ScanReport)
	}
	if res.DraftSSPath != "" {
		t.Errorf("SS exists — no SS skeleton may be generated, got %q", res.DraftSSPath)
	}
	if res.DraftSDPath == "" {
		t.Fatalf("missing SD was not reverse-documented")
	}
	if !strings.Contains(filepath.ToSlash(res.DraftSDPath), "requirements/06-System-Tech-Design/todo/SD-Auth.md") {
		t.Errorf("DraftSDPath = %q, want the todo/ draft", res.DraftSDPath)
	}
	sdData, err := os.ReadFile(res.DraftSDPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(sdData), "Login") {
		t.Errorf("SD backfill must cite collected evidence, got:\n%s", sdData)
	}
	// No SS-Lock pause: business intent (SS) is already human-owned.
	if res.Status != StandardizeStatusCompleted {
		t.Errorf("Status = %q, want completed (mixed backfill does not pause)", res.Status)
	}
	ssLockGatesMu.Lock()
	defer ssLockGatesMu.Unlock()
	for k := range ssLockGates {
		if k.svc == svc {
			t.Errorf("unexpected SS-Lock gate %q on a mixed backfill run", k.runID)
		}
	}
}

// [Error] Scenario: GitNexus không khả dụng -> Graceful degradation
// Input: GitNexus CLI trả về exit code != 0
// Expect: Hệ thống fallback sang static scan, log cảnh báo, không crash
func TestStandardize_GitNexusUnavailable_FallsBackToStaticScan(t *testing.T) {
	svc, root := newStandardizeTestService(t, "", errors.New("exit status 1"))
	writeSandboxFile(t, root, "features/device/device.go", deviceFixtureGo)

	res, err := svc.ExecuteStandardize(context.Background(), StandardizeScope{Path: "features/device"})
	if err != nil {
		t.Fatalf("GitNexus failure must degrade, not crash: %v", err)
	}
	if res.Status != StandardizeStatusWaitingSSLock {
		t.Fatalf("Status = %q, want the reverse-doc flow to complete into the SS-Lock pause", res.Status)
	}
	sdData, err := os.ReadFile(res.DraftSDPath)
	if err != nil {
		t.Fatal(err)
	}
	// The static scan fallback must still produce real evidence citations.
	if !strings.Contains(string(sdData), "NewDeviceRunner (") {
		t.Errorf("static-scan fallback evidence missing from the SD draft, got:\n%s", sdData)
	}
	if !strings.Contains(string(sdData), "GET /client/device/status") {
		t.Errorf("static endpoint evidence missing from the SD draft, got:\n%s", sdData)
	}
	// The flow-level claim has no evidence — it must be a TODO, not fiction.
	if !strings.Contains(string(sdData), "TODO: no execution-flow evidence") {
		t.Errorf("missing flow evidence must be flagged TODO, got:\n%s", sdData)
	}
}

// [Error] Scenario: Scope path không tồn tại
// Input: Scope="features/nonexistent"
// Expect: Trả về error rõ ràng "scope path not found"
func TestStandardize_InvalidScope_ReturnsError(t *testing.T) {
	svc, root := newStandardizeTestService(t, gitnexusFixtureJSON, nil)

	res, err := svc.ExecuteStandardize(context.Background(), StandardizeScope{Path: "features/nonexistent"})
	if err == nil {
		t.Fatalf("ExecuteStandardize = %+v, want a scope error", res)
	}
	if !strings.Contains(err.Error(), "scope path not found") {
		t.Errorf("error = %v, want it to contain \"scope path not found\"", err)
	}
	if !errors.Is(err, ErrStandardizeScopeNotFound) {
		t.Errorf("error must match ErrStandardizeScopeNotFound, got %v", err)
	}
	// Nothing may be written for an invalid scope.
	if _, statErr := os.Stat(filepath.Join(root, "requirements")); !os.IsNotExist(statErr) {
		t.Errorf("invalid scope must not create requirements/, got stat err %v", statErr)
	}
}

// [Edge] Scenario: Người dùng từ chối SS-Lock -> Hủy workflow
// Input: Client gửi reject thay vì confirm tại cổng SS-Lock
// Expect: Workflow kết thúc với status="cancelled", không ghi file nào
func TestStandardize_UserRejectsSSLock_AbortsWorkflow(t *testing.T) {
	svc, root := newStandardizeTestService(t, gitnexusFixtureJSON, nil)
	writeSandboxFile(t, root, "features/device/device.go", deviceFixtureGo)
	res, _ := newBrownfieldRun(t, svc)

	drafts := []string{res.DraftSSPath, res.DraftSDPath}

	mux := http.NewServeMux()
	svc.RegisterInteractiveRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()
	resp, err := http.Post(server.URL+"/client/workflow-runs/"+res.RunID+"/confirm", "application/json",
		strings.NewReader(`{"action": "reject"}`))
	if err != nil {
		t.Fatalf("POST reject: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("reject status = %d, want 200", resp.StatusCode)
	}

	snap, ok := svc.SSLockSnapshot(res.RunID)
	if !ok {
		t.Fatal("gate snapshot vanished after reject")
	}
	if snap.Status != "cancelled" {
		t.Errorf("workflow status = %q, want cancelled", snap.Status)
	}
	if snap.State.Locked {
		t.Errorf("gate must be unlocked after cancellation")
	}
	for _, d := range drafts {
		if _, statErr := os.Stat(d); !os.IsNotExist(statErr) {
			t.Errorf("unapproved draft %s still on disk after rejection", d)
		}
	}
	if n := countMarkdownFiles(t, filepath.Join(root, "requirements")); n != 0 {
		t.Errorf("rejection wrote/kept %d markdown file(s); want none", n)
	}
	// Cancelled gate is terminal.
	if err := svc.HandleSSLockConfirm(context.Background(), res.RunID, ""); err == nil {
		t.Errorf("confirm after reject succeeded; cancelled gate must be terminal")
	}
}

// ---- bonus coverage under the Task-333 run patterns ------------------------

// TestReverseDoc_CollectEvidence_StaticFallback checks the Code Guide
// CollectEvidence signature directly: exported Go declarations, endpoint
// literals, and empty git evidence on a non-git sandbox.
func TestReverseDoc_CollectEvidence_StaticFallback(t *testing.T) {
	_, root := newStandardizeTestService(t, "", errors.New("no cli"))
	writeSandboxFile(t, root, "features/device/device.go", deviceFixtureGo)
	// The Code Guide signature resolves relative scopes from the process cwd.
	t.Chdir(root)

	ev, err := CollectEvidence(context.Background(), StandardizeScope{Path: "features/device"})
	if err != nil {
		t.Fatalf("CollectEvidence: %v", err)
	}
	joined := strings.Join(ev.Symbols, "\n")
	if !strings.Contains(joined, "NewDeviceRunner") || !strings.Contains(joined, "DeviceRunner.Run") {
		t.Errorf("static AST evidence missing exported symbols, got:\n%s", joined)
	}
	if len(ev.PublicAPIs) == 0 {
		t.Errorf("PublicAPIs empty, want exported declarations")
	}
	if len(ev.Endpoints) != 1 || ev.Endpoints[0] != "GET /client/device/status" {
		t.Errorf("Endpoints = %v, want [GET /client/device/status]", ev.Endpoints)
	}
	if len(ev.RecentCommits) != 0 {
		t.Errorf("RecentCommits = %v, want empty outside a git repo (graceful)", ev.RecentCommits)
	}
}

// TestReverseDoc_GenerateDraftSS_NeverInventsIntent asserts the CP-49 Hard
// Ceiling Rule on the Code Guide generator: every intent-bearing section,
// including Acceptance Criteria, is TODO-flagged, never authored by the AI.
func TestReverseDoc_GenerateDraftSS_NeverInventsIntent(t *testing.T) {
	ev := &Evidence{
		Symbols:        []string{"device.NewDeviceRunner (features/device/device.go)"},
		PublicAPIs:     []string{"device.NewDeviceRunner (features/device/device.go)"},
		Endpoints:      []string{"GET /client/device/status"},
		ExecutionFlows: []string{},
		RecentCommits:  []string{},
	}
	content, err := GenerateDraftSS(context.Background(), ev, StandardizeScope{Path: "features/device"})
	if err != nil {
		t.Fatalf("GenerateDraftSS: %v", err)
	}
	sectionBlock := func(heading string) string {
		start := strings.Index(content, "## "+heading)
		if start < 0 {
			t.Fatalf("section %q missing from skeleton:\n%s", heading, content)
		}
		rest := content[start+len("## "+heading):]
		if end := strings.Index(rest, "\n## "); end >= 0 {
			rest = rest[:end]
		}
		return rest
	}
	for _, heading := range []string{"1. Goal", "2. Problem", "4. Non-Goals", "5. User Stories or Primary Use Cases", "7. Business Rules"} {
		if !strings.Contains(sectionBlock(heading), "TODO: human intent needed") {
			t.Errorf("%q section is business intent but carries no TODO marker", heading)
		}
	}
	// The Acceptance Criteria block must contain only TODO markers.
	acBlock := sectionBlock("6. Acceptance Criteria")
	if !strings.Contains(acBlock, "TODO: human intent needed") {
		t.Errorf("Acceptance Criteria must be TODO-flagged:\n%s", acBlock)
	}
	if strings.Contains(strings.ToLower(acBlock), "- the device") || strings.Contains(strings.ToLower(acBlock), "must sync") {
		t.Errorf("AI invented acceptance criteria:\n%s", acBlock)
	}
	if !strings.Contains(content, "Phase: `ss`") || !strings.Contains(content, "Status: `draft`") {
		t.Errorf("SS skeleton must carry draft ss metadata")
	}
}

// TestSSLock_GateLifecycleAndFenceReset covers the full gate lifecycle:
// waiting -> (reject attempt on confirm path is a no-op state change) ->
// confirm -> completed with the turn fence cleared.
func TestSSLock_GateLifecycleAndFenceReset(t *testing.T) {
	svc, root := newStandardizeTestService(t, gitnexusFixtureJSON, nil)
	writeSandboxFile(t, root, "features/device/device.go", deviceFixtureGo)
	res, snap := newBrownfieldRun(t, svc)

	// Re-emitting the pause while already waiting is an idempotent no-op
	// (CP-60 parity: the modal must not render twice).
	eventsBefore := len(snap.Events)
	if err := svc.EmitSSLockEvent(context.Background(), SSLockState{RunID: res.RunID, Locked: true}); err != nil {
		t.Errorf("idempotent re-emit while waiting failed: %v", err)
	}
	if snap2, _ := svc.SSLockSnapshot(res.RunID); len(snap2.Events) != eventsBefore {
		t.Errorf("re-emit duplicated the pause event: %d -> %d", eventsBefore, len(snap2.Events))
	}
	// Unknown run ids 404 at the gate API level.
	if err := svc.HandleSSLockConfirm(context.Background(), "run-none", ""); err == nil {
		t.Errorf("confirm for unknown run must fail")
	}
	// Confirm publishes and clears the fence.
	if err := svc.HandleSSLockConfirm(context.Background(), res.RunID, ""); err != nil {
		t.Fatalf("HandleSSLockConfirm: %v", err)
	}
	snapAfter, ok := svc.SSLockSnapshot(res.RunID)
	if !ok {
		t.Fatal("gate snapshot vanished after confirm")
	}
	snap = snapAfter
	if snap.State.Locked {
		t.Errorf("gate still locked after confirm")
	}
	if e := svc.ssLockTurnFence(res.RunID); e != nil {
		t.Errorf("turn fence still up after confirm: %v", e)
	}
	if len(snap.Published) != 2 {
		t.Errorf("Published = %v, want the SS and SD phase-root docs", snap.Published)
	}
	// DetectPhase must recognize both published docs (they are SS-13 docs).
	for _, p := range snap.Published {
		if phase, ok := docscan.DetectPhase(p); !ok || (phase != "ss" && phase != "sd") {
			t.Errorf("published doc %s detected as phase %q (ok=%v)", p, phase, ok)
		}
	}
}

// countMarkdownFilesOutsideTodo counts .md files outside todo/ dirs.
func countMarkdownFilesOutsideTodo(t *testing.T, root string) int {
	t.Helper()
	n := 0
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && d.Name() == "todo" {
			return filepath.SkipDir
		}
		if !d.IsDir() && strings.EqualFold(filepath.Ext(path), ".md") {
			n++
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// Review-hardening regression (Task-333 §8 follow-up): un-approved SS-Lock
// drafts stranded under a todo/ segment (e.g. after a runner restart lost the
// in-memory gate) are NOT published docs — a re-run must re-offer the
// brownfield reverse-doc flow (a fresh SS-Lock pause) instead of reporting
// conformance/mixed against the stranded drafts.
func TestStandardize_StrandedDraftsInTodo_ReofferReverseDoc(t *testing.T) {
	svc, root := newStandardizeTestService(t, "", nil)
	writeSandboxFile(t, root, "requirements/05-System-Specs/todo/SS-01-Device.md", sampleSSDoc)
	writeSandboxFile(t, root, "features/device/camera.go", "package device\n")

	res, err := svc.ExecuteStandardize(context.Background(), StandardizeScope{Path: "features/device"})
	if err != nil {
		t.Fatalf("ExecuteStandardize: %v", err)
	}
	if res.Mode != StandardizeModeReverseDoc {
		t.Fatalf("stranded todo/ drafts must not count as existing docs; Mode = %q, want %q", res.Mode, StandardizeModeReverseDoc)
	}
	if res.Status != StandardizeStatusWaitingSSLock {
		t.Fatalf("the SS-Lock approval flow must be re-offered, got Status = %q", res.Status)
	}
}
