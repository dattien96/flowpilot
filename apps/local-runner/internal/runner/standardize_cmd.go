package runner

// Task-333 T-1 (CP-49 / CP-48 integration): the `/standardize [scope]`
// command. One entry point, two branches:
//
//   - Scope already has documents  -> CP-48 conformance (Task-332 docscan
//     ScanDocument / ScanDirectory + AutoFix on todo/ drafts), reported back.
//   - Scope has no documents       -> CP-49 reverse-documentation: evidence
//     collection, deterministic SD draft + SS skeleton, then the SS-Lock gate
//     pauses the run (EventUserConfirmRequired) until a human approves via
//     POST /client/workflow-runs/{id}/confirm (ss_lock_gate.go).
//
// Provider-agnostic parity note: the whole command path is deterministic Go +
// subprocess evidence (GitNexus CLI / git log); it contains no LLM call, so
// Claude/Codex/Grok sessions behave identically by construction.
//
// Write policy (CP-49 constraints): project source is never modified; the
// reverse-doc flow writes only draft docs under
// requirements/05-System-Specs/todo/ and requirements/06-System-Tech-Design/
// todo/; previously approved documents are never overwritten (AutoFix is
// applied only to todo/ drafts; publication after human approval picks a new
// suffixed name if a phase-root file already exists).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"flowpilot-runner/internal/docscan"
)

// ErrStandardizeScopeNotFound is returned when a path-like scope does not
// exist under the workspace root (Task-333 §10 "scope path not found").
var ErrStandardizeScopeNotFound = errors.New("scope path not found")

// StandardizeResult modes.
const (
	StandardizeModeConformance = "conformance" // CP-48 scan (+ AutoFix on drafts)
	StandardizeModeReverseDoc  = "reverse_doc" // CP-49 full reverse flow, pauses at SS-Lock
	StandardizeModeMixed       = "mixed"       // conformance on existing phase + reverse-doc on the missing one
)

// StandardizeResult statuses.
const (
	StandardizeStatusCompleted     = "completed"
	StandardizeStatusWaitingSSLock = "waiting_ss_lock"
)

// StandardizeScope selects the feature/directory subset to standardize
// (Task-333 Code Guide). An empty Path and FeatureName means the whole
// project.
type StandardizeScope struct {
	Path        string `json:"path"`
	FeatureName string `json:"feature_name"`
}

// StandardizeResult reports what the command did. RunID is set when the flow
// parked at the SS-Lock gate; the client resumes via the confirm endpoint.
type StandardizeResult struct {
	Mode        string              `json:"mode"`
	RunID       string              `json:"run_id,omitempty"`
	ScanReport  *docscan.ScanReport `json:"scan_report,omitempty"`
	DraftSSPath string              `json:"draft_ss_path,omitempty"`
	DraftSDPath string              `json:"draft_sd_path,omitempty"`
	Status      string              `json:"status"`
}

// standardizeRootBySvc pins the workspace per service instance (the shared
// InteractiveService struct cannot be extended without touching
// interactive_service.go). Defaults to the runner process working directory.
var (
	standardizeRootMu    sync.Mutex
	standardizeRootBySvc = map[*InteractiveService]string{}
)

// SetStandardizeWorkspaceRoot pins the workspace root /standardize operates
// on. Callers embed the runner against a specific project directory; tests
// pin it to a t.TempDir() sandbox so the repo's own requirements/ is never
// touched.
func (s *InteractiveService) SetStandardizeWorkspaceRoot(root string) {
	standardizeRootMu.Lock()
	defer standardizeRootMu.Unlock()
	standardizeRootBySvc[s] = root
}

func (s *InteractiveService) standardizeWorkspaceRoot() string {
	standardizeRootMu.Lock()
	defer standardizeRootMu.Unlock()
	if root := standardizeRootBySvc[s]; root != "" {
		return root
	}
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}

// ExecuteStandardize dispatches the /standardize flow for scope (Task-333
// Code Guide): existing docs -> CP-48 conformance; missing docs -> CP-49
// reverse-doc with the SS-Lock pause; a partial set -> mixed mode.
func (s *InteractiveService) ExecuteStandardize(ctx context.Context, scope StandardizeScope) (*StandardizeResult, error) {
	root := s.standardizeWorkspaceRoot()
	scope.Path = strings.TrimSpace(filepath.ToSlash(scope.Path))
	scope.FeatureName = strings.TrimSpace(scope.FeatureName)

	// Validate path-like scopes up-front (a bare token such as "device" is a
	// feature subset of the whole workspace, not a directory).
	if _, _, err := resolveScopeDir(root, scope); err != nil {
		return nil, err
	}

	reqRoot := filepath.Join(root, "requirements")
	scoped := scope.Path != "" || scope.FeatureName != ""
	if !scoped {
		// Whole project: any governed document anywhere -> full CP-48
		// conformance scan (docscan.ScanDirectory over requirements/);
		// no documents at all -> brownfield reverse flow.
		if wholeProjectHasDocs(reqRoot) {
			return s.standardizeFullProject(reqRoot)
		}
		return s.standardizeReverseDoc(ctx, root, reqRoot, scope)
	}

	ssDocs, sdDocs, cpDocs, err := findScopeDocs(reqRoot, scope)
	if err != nil {
		return nil, err
	}
	switch {
	case len(ssDocs) > 0 && len(sdDocs) > 0:
		// Both governing docs exist -> CP-48 conformance only.
		report, err := scanDocFiles(append(append(append([]string{}, ssDocs...), sdDocs...), cpDocs...))
		if err != nil {
			return nil, err
		}
		if err := autoFixDraftDocs(append(append(append([]string{}, ssDocs...), sdDocs...), cpDocs...)); err != nil {
			return nil, err
		}
		return &StandardizeResult{
			Mode:       StandardizeModeConformance,
			ScanReport: report,
			Status:     StandardizeStatusCompleted,
		}, nil
	case len(ssDocs) == 0 && len(sdDocs) == 0:
		// Brownfield: full CP-49 reverse flow, pausing at SS-Lock.
		return s.standardizeReverseDoc(ctx, root, reqRoot, scope)
	case len(ssDocs) > 0:
		// SS exists, SD missing: conformance on SS + reverse-doc backfill of
		// the SD draft (Task-333 §10 TestStandardize_PartialDocs_MixedMode).
		// No SS-Lock pause: the business intent (SS) is already human-owned
		// and the SD is AI-authorable from evidence (CP-49 Hard Ceiling Rule
		// applies only to SS intent).
		return s.standardizeMixed(ctx, root, reqRoot, scope, append(append([]string{}, ssDocs...), cpDocs...), false)
	default:
		// SD exists, SS missing: conformance on SD + SS skeleton that must be
		// human-approved at the SS-Lock gate.
		return s.standardizeMixed(ctx, root, reqRoot, scope, append(append([]string{}, sdDocs...), cpDocs...), true)
	}
}

// standardizeFullProject runs the CP-48 conformance branch over the whole
// requirements tree (Task-333 §10 TestStandardize_EmptyScope_RunsFullProject).
func (s *InteractiveService) standardizeFullProject(reqRoot string) (*StandardizeResult, error) {
	report, err := docscan.ScanDirectory(reqRoot)
	if err != nil {
		return nil, fmt.Errorf("standardize: CP-48 scan of %s failed: %w", reqRoot, err)
	}
	var drafts []string
	_ = filepath.WalkDir(reqRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.EqualFold(filepath.Ext(path), ".md") {
			return nil
		}
		if filepath.Base(filepath.Dir(path)) == "todo" {
			drafts = append(drafts, path)
		}
		return nil
	})
	if err := autoFixDraftDocs(drafts); err != nil {
		return nil, err
	}
	return &StandardizeResult{
		Mode:       StandardizeModeConformance,
		ScanReport: report,
		Status:     StandardizeStatusCompleted,
	}, nil
}

// standardizeReverseDoc is the CP-49 branch: collect evidence, generate the
// SD draft and SS skeleton, write them as todo/ drafts, then park the run at
// the non-bypassable SS-Lock gate.
func (s *InteractiveService) standardizeReverseDoc(ctx context.Context, root, reqRoot string, scope StandardizeScope) (*StandardizeResult, error) {
	result, err := s.standardizeGenerate(ctx, root, reqRoot, scope, true, true)
	if err != nil {
		return nil, err
	}
	result.Mode = StandardizeModeReverseDoc
	return result, nil
}

// standardizeMixed runs the conformance scan on the existing phase docs and
// the reverse-doc backfill for the missing one. needSS selects whether the
// SS skeleton (which parks the run at SS-Lock) or the SD draft (published as
// a draft immediately) is generated.
func (s *InteractiveService) standardizeMixed(ctx context.Context, root, reqRoot string, scope StandardizeScope, existing []string, needSS bool) (*StandardizeResult, error) {
	report, err := scanDocFiles(existing)
	if err != nil {
		return nil, err
	}
	result, err := s.standardizeGenerate(ctx, root, reqRoot, scope, needSS, !needSS)
	if err != nil {
		return nil, err
	}
	result.Mode = StandardizeModeMixed
	result.ScanReport = report
	return result, nil
}

// standardizeGenerate collects evidence, generates the requested drafts and —
// when the SS skeleton is among them — registers the SS-Lock gate and emits
// the pause event.
func (s *InteractiveService) standardizeGenerate(ctx context.Context, root, reqRoot string, scope StandardizeScope, needSS, needSD bool) (*StandardizeResult, error) {
	evidence, err := collectEvidenceIn(ctx, root, scope)
	if err != nil {
		return nil, err
	}
	base := standardizeFeatureTitle(scope)
	result := &StandardizeResult{Status: StandardizeStatusCompleted}

	if needSD {
		sdContent, err := GenerateDraftSD(ctx, evidence, scope)
		if err != nil {
			return nil, err
		}
		sdPath, err := writeDraftDoc(reqRoot, "06-System-Tech-Design", "SD", base, sdContent)
		if err != nil {
			return nil, err
		}
		if err := autoFixDraftDocs([]string{sdPath}); err != nil {
			return nil, err
		}
		result.DraftSDPath = sdPath
	}
	if needSS {
		ssContent, err := GenerateDraftSS(ctx, evidence, scope)
		if err != nil {
			return nil, err
		}
		ssPath, err := writeDraftDoc(reqRoot, "05-System-Specs", "SS", base, ssContent)
		if err != nil {
			return nil, err
		}
		result.DraftSSPath = ssPath

		// T-3/T-4: park the run at the SS-Lock gate. There is intentionally
		// no code path that continues the workflow from here — only the
		// human confirm endpoint (ss_lock_gate.go) can resume it.
		runID := s.nextID("run")
		s.registerSSLockGate(&ssLockGate{
			runID:          runID,
			featureBase:    base,
			workspaceRoot:  root,
			status:         ssLockWaiting,
			draftSSPath:    ssPath,
			draftSDPath:    result.DraftSDPath,
			draftSSContent: ssContent,
			draftSDContent: draftContentAt(result.DraftSDPath),
		})
		if err := s.EmitSSLockEvent(ctx, SSLockState{RunID: runID, DraftSSPath: ssPath, Locked: true}); err != nil {
			return nil, err
		}
		result.RunID = runID
		result.Status = StandardizeStatusWaitingSSLock
	}
	return result, nil
}

// draftContentAt reads a file's content; a read failure yields "" (the gate
// then treats the SD as absent, publish simply skips it).
func draftContentAt(path string) string {
	if path == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

// ---- document discovery / scan / autofix helpers ---------------------------

// scopePhaseDir is one governed requirements/ phase directory relevant to the
// /standardize doc-existence check.
type scopePhaseDir struct {
	phase string // docscan phase id
	dir   string // directory name under requirements/
}

// standardizePhaseDirs lists the phases /standardize consults, in
// deterministic order (SS and SD drive the branch; CP contributes to reports).
var standardizePhaseDirs = []scopePhaseDir{
	{"ss", "05-System-Specs"},
	{"sd", "06-System-Tech-Design"},
	{"cp", "07-Coding-Plan"},
}

// wholeProjectHasDocs reports whether any governed phase document exists
// under requirements/.
func wholeProjectHasDocs(reqRoot string) bool {
	for _, pd := range standardizePhaseDirs {
		if dirHasMarkdown(filepath.Join(reqRoot, pd.dir)) {
			return true
		}
	}
	return dirHasMarkdown(filepath.Join(reqRoot, "08-Task")) || dirHasMarkdown(filepath.Join(reqRoot, "09-BugFix"))
}

func dirHasMarkdown(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".md") {
			return true
		}
	}
	return false
}

// findScopeDocs returns the SS/SD/CP markdown files relevant to scope:
// filename token match for scoped runs (e.g. "auth" matches SS-01-Auth.md),
// prefix-filtered so FORMAT-REFERENCE writing guides and stray notes are not
// treated as feature docs. Results are sorted for deterministic reports.
func findScopeDocs(reqRoot string, scope StandardizeScope) (ss, sd, cp []string, err error) {
	token := strings.ToLower(scope.FeatureName)
	if token == "" {
		token = strings.ToLower(filepath.Base(filepath.ToSlash(scope.Path)))
	}
	if token == "" || token == "." {
		return nil, nil, nil, nil
	}
	buckets := map[string]*[]string{"ss": &ss, "sd": &sd, "cp": &cp}
	for _, pd := range standardizePhaseDirs {
		bucket := buckets[pd.phase]
		prefix := pd.phase + "-"
		matches, merr := matchPhaseDocs(filepath.Join(reqRoot, pd.dir), prefix, token)
		if merr != nil {
			return nil, nil, nil, merr
		}
		*bucket = matches
	}
	return ss, sd, cp, nil
}

func matchPhaseDocs(dir, prefix, token string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // missing phase dir or unreadable subtree — skip
		}
		// Un-approved drafts live under a todo/ segment (SS-Lock pending).
		// They are NOT published docs — counting them here would make a
		// post-restart /standardize re-run report conformance/mixed and never
		// re-offer the approval modal (review hardening). Skip the subtree.
		if d.IsDir() {
			if strings.EqualFold(d.Name(), "todo") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.EqualFold(filepath.Ext(path), ".md") {
			return nil
		}
		base := filepath.Base(path)
		if strings.HasPrefix(strings.ToLower(base), "format-reference-") {
			return nil // SS-13 §11: writing guides, not business truth
		}
		lower := strings.ToLower(base)
		if strings.HasPrefix(lower, prefix) && strings.Contains(lower, token) {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

// scanDocFiles scans the given documents with the CP-48 scanner (Task-332)
// and aggregates the violations into one ScanReport.
func scanDocFiles(paths []string) (*docscan.ScanReport, error) {
	report := &docscan.ScanReport{Issues: []docscan.ScanIssue{}}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("standardize: reading %s: %w", path, err)
		}
		issues, err := docscan.ScanDocument(path, string(data))
		if err != nil {
			return nil, fmt.Errorf("standardize: scanning %s: %w", path, err)
		}
		report.TotalFilesScanned++
		if len(issues) == 0 {
			report.ConformingFiles++
		}
		report.Issues = append(report.Issues, issues...)
	}
	return report, nil
}

// autoFixDraftDocs applies the CP-48 AutoFix to draft (todo/) documents only;
// approved phase-root documents are never rewritten (CP-49 constraint: no
// overwriting previously approved docs).
func autoFixDraftDocs(paths []string) error {
	for _, path := range paths {
		if filepath.Base(filepath.Dir(path)) != "todo" {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("standardize: reading draft %s: %w", path, err)
		}
		phase, ok := docscan.DetectPhase(path)
		if !ok {
			continue
		}
		fixed, err := docscan.AutoFixDocument(string(data), phase)
		if err != nil {
			return fmt.Errorf("standardize: autofix %s: %w", path, err)
		}
		if fixed == string(data) {
			continue
		}
		if err := os.WriteFile(path, []byte(fixed), 0o644); err != nil {
			return fmt.Errorf("standardize: writing autofixed draft %s: %w", path, err)
		}
	}
	return nil
}

// writeDraftDoc writes an AI draft into requirements/<phaseDir>/todo/,
// picking a -2/-3 suffix so an existing draft is never overwritten.
func writeDraftDoc(reqRoot, phaseDir, prefix, base, content string) (string, error) {
	dir := filepath.Join(reqRoot, phaseDir, "todo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("standardize: creating draft dir: %w", err)
	}
	path := filepath.Join(dir, fmt.Sprintf("%s-%s.md", prefix, base))
	for i := 2; ; i++ {
		if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
			break
		}
		path = filepath.Join(dir, fmt.Sprintf("%s-%s-%d.md", prefix, base, i))
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("standardize: writing draft %s: %w", path, err)
	}
	return path, nil
}

// asAPIErr converts a plain error into a 500 apiErr, passing *apiErr through.
func asAPIErr(err error) *apiErr {
	if e, ok := err.(*apiErr); ok {
		return e
	}
	return newAPIErr(http.StatusInternalServerError, "internal", err.Error())
}

// ---- HTTP handlers (registered in RegisterInteractiveRoutes) ---------------

// handleStandardize serves POST /client/standardize with a StandardizeScope
// body ({"path": "features/auth"} / {"feature_name": "device"} / {}).
func (s *InteractiveService) handleStandardize(w http.ResponseWriter, r *http.Request) {
	var scope StandardizeScope
	if err := json.NewDecoder(r.Body).Decode(&scope); err != nil && err != io.EOF {
		writeInteractiveError(w, newAPIErr(http.StatusBadRequest, "invalid_request", "invalid request body"))
		return
	}
	result, err := s.ExecuteStandardize(r.Context(), scope)
	if err != nil {
		if errors.Is(err, ErrStandardizeScopeNotFound) {
			writeInteractiveError(w, newAPIErr(http.StatusBadRequest, "scope_not_found", err.Error()))
			return
		}
		writeInteractiveError(w, newAPIErr(http.StatusInternalServerError, "standardize_failed", err.Error()))
		return
	}
	writeInteractiveJSON(w, http.StatusOK, result)
}
