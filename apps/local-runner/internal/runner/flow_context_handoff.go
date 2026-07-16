package runner

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// flowContextHandoffPrefix is the human-readable leading marker of every flow
// context package prompt. Double-inject guards MUST NOT trust this alone —
// users can type it as a whole line. Use flowContextTrustedMarker (run-scoped
// HTML comment) instead (V10R4 P1 PromptEnvelope).
const flowContextHandoffPrefix = "[FlowPilot flow context package]"

// runMarker secrets HMAC-bind double-injection markers (BUG-288 P1-20 / R13-16).
// BUG-288 R16-P1: secrets are keyed by absolute dataDir (multi-store safe).
// A failed I/O init for one dir does not block later inits (no sync.Once trap).
var (
	runMarkerSecretsMu sync.RWMutex
	runMarkerSecrets   = map[string][]byte{} // abs(dataDir) → secret
	runMarkerActive    = newRunMarkerSecret() // default used by runMarkerMAC
)

func newRunMarkerSecret() []byte {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err == nil {
		return b
	}
	// BUG-288 R13-19: never use low-entropy time-only fallback.
	var stackSink int
	sum := sha256.Sum256([]byte(
		fmt.Sprintf("flowpilot-run-marker-fallback|%s|%p|%d",
			time.Now().String(), &stackSink, time.Now().UnixNano()),
	))
	return sum[:]
}

// initRunMarkerSecretForStore loads the durable secret for the store's data
// dir (BUG-288 R14-04 / R16-P1). Prefer loadMarkerSecretForStore which returns
// the secret for InteractiveService.markerSecret (R18-4).
func initRunMarkerSecretForStore(store WorkflowStore) {
	_, _ = loadMarkerSecretForStore(store)
}

// loadMarkerSecretForStore returns (dataDir, secret) for a service instance
// (BUG-288 R18-4). Secret is owned by that service for minting; package-level
// active is still updated for tests/legacy package callers.
func loadMarkerSecretForStore(store WorkflowStore) (dir string, secret []byte) {
	dir = ""
	if ls, ok := store.(*localFileSessionStore); ok {
		dir = ls.DataDir()
	}
	if dir == "" {
		base, err := os.UserConfigDir()
		if err != nil || base == "" {
			base = os.TempDir()
		}
		dir = filepath.Join(base, "flowpilot", "marker")
	}
	return dir, InitRunMarkerSecretFromDir(dir)
}

// InitRunMarkerSecretFromDir loads or creates a 32-byte secret at
// dataDir/run_marker_secret (BUG-288 R13-16 / R16–R18). Returns the secret for
// that dir (nil on failure). Entire load/create/cache is under write lock.
func InitRunMarkerSecretFromDir(dataDir string) []byte {
	if strings.TrimSpace(dataDir) == "" {
		return nil
	}
	key, err := filepath.Abs(dataDir)
	if err != nil {
		key = dataDir
	}
	runMarkerSecretsMu.Lock()
	defer runMarkerSecretsMu.Unlock()
	if sec, ok := runMarkerSecrets[key]; ok {
		runMarkerActive = sec
		out := make([]byte, len(sec))
		copy(out, sec)
		return out
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		log.Printf("[marker] mkdir for run_marker_secret dir=%q: %v", dataDir, err)
		return nil
	}
	path := filepath.Join(dataDir, "run_marker_secret")
	var sec []byte
	if data, rerr := os.ReadFile(path); rerr == nil && len(data) >= 32 {
		sec = make([]byte, 32)
		copy(sec, data[:32])
	} else {
		sec = newRunMarkerSecret()
		tmp := path + ".tmp"
		if werr := os.WriteFile(tmp, sec, 0o600); werr != nil {
			log.Printf("[marker] write run_marker_secret dir=%q: %v (not caching)", dataDir, werr)
			return nil
		}
		if rerr := os.Rename(tmp, path); rerr != nil {
			_ = os.Remove(tmp)
			log.Printf("[marker] rename run_marker_secret dir=%q: %v (not caching)", dataDir, rerr)
			return nil
		}
	}
	runMarkerSecrets[key] = sec
	runMarkerActive = sec
	out := make([]byte, len(sec))
	copy(out, sec)
	return out
}

// runMarkerMACWith mints using an explicit secret (BUG-288 R18-4 per-service).
func runMarkerMACWith(secret []byte, kind, id string) string {
	if len(secret) == 0 {
		secret = currentRunMarkerSecret()
	}
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(kind))
	mac.Write([]byte{0})
	mac.Write([]byte(id))
	return hex.EncodeToString(mac.Sum(nil))[:16]
}

// flowContextTrustedMarkerWith uses an explicit secret for mint (R18-4).
func flowContextTrustedMarkerWith(secret []byte, runOrPackageID string) string {
	id := runOrPackageID
	if id == "" {
		id = "anonymous"
	}
	return "<!-- flowpilot-fcp:" + id + ":" + runMarkerMACWith(secret, "fcp", id) + " -->"
}

func currentRunMarkerSecret() []byte {
	runMarkerSecretsMu.RLock()
	defer runMarkerSecretsMu.RUnlock()
	out := make([]byte, len(runMarkerActive))
	copy(out, runMarkerActive)
	return out
}

// runMarkerMAC returns a short, non-forgeable tag binding kind+id using the
// currently active (last-Init'd) directory secret.
func runMarkerMAC(kind, id string) string {
	mac := hmac.New(sha256.New, currentRunMarkerSecret())
	mac.Write([]byte(kind))
	mac.Write([]byte{0})
	mac.Write([]byte(id))
	return hex.EncodeToString(mac.Sum(nil))[:16]
}

// verifyRunMarkerMAC reports whether tag is the correct MAC for kind+id.
// Accepts the active secret or any loaded per-dir secret (multi-store in-process).
// Prefer verifyRunMarkerMACWith for per-service isolation (BUG-288 R19-4).
func verifyRunMarkerMAC(kind, id, tag string) bool {
	if tag == "" {
		return false
	}
	want := runMarkerMAC(kind, id)
	if hmac.Equal([]byte(tag), []byte(want)) {
		return true
	}
	runMarkerSecretsMu.RLock()
	defer runMarkerSecretsMu.RUnlock()
	for _, sec := range runMarkerSecrets {
		m := hmac.New(sha256.New, sec)
		m.Write([]byte(kind))
		m.Write([]byte{0})
		m.Write([]byte(id))
		w := hex.EncodeToString(m.Sum(nil))[:16]
		if hmac.Equal([]byte(tag), []byte(w)) {
			return true
		}
	}
	return false
}

// verifyRunMarkerMACWith checks tag against a single explicit secret only
// (BUG-288 R19-4). Service A must not trust markers minted with B's secret.
func verifyRunMarkerMACWith(secret []byte, kind, id, tag string) bool {
	if tag == "" || len(secret) == 0 {
		return false
	}
	m := hmac.New(sha256.New, secret)
	m.Write([]byte(kind))
	m.Write([]byte{0})
	m.Write([]byte(id))
	w := hex.EncodeToString(m.Sum(nil))[:16]
	return hmac.Equal([]byte(tag), []byte(w))
}

// flowContextTrustedMarker returns a run-scoped inject token only ComposeFlowCodingPrompt
// writes. injectFeatureHistoryPrompt skips only when this token is present.
// BUG-288 P1-20: the id alone used to be trusted, but run/package ids are
// visible to the user (desktop UI, API responses) — a user who copied one
// into their own prompt text could suppress injection. The trailing segment
// is an HMAC over kind+id using a server-only secret, so only this process
// can mint a marker that isFlowContextHandoff will accept.
func flowContextTrustedMarker(runOrPackageID string) string {
	id := runOrPackageID
	if id == "" {
		id = "anonymous"
	}
	return "<!-- flowpilot-fcp:" + id + ":" + runMarkerMAC("fcp", id) + " -->"
}

// isFlowContextHandoff reports whether a prompt already carries a trusted
// FlowContextPackage envelope, preventing double-injection by
// injectFeatureHistoryPrompt.
//
// V10R4 P1: only the trusted HTML-comment marker suppresses inject. The
// human-readable flowContextHandoffPrefix alone is forgeable by user text.
// BUG-288 P1-20: the marker's id:MAC pair is verified against this process's
// runMarkerSecret — a line that merely LOOKS like "<!-- flowpilot-fcp:X -->"
// (any X a user can type, including a real, guessed, or copied run id) is no
// longer sufficient; only a MAC this process itself generated passes.
//
// BUG-288 R13-15: when expectedIDs is non-empty, the marker id must match one
// of them (typically rs.id and plan package id) so a valid MAC from another
// run cannot be replayed cross-run.
func isFlowContextHandoff(prompt string, expectedIDs ...string) bool {
	return isFlowContextHandoffWithSecret(nil, prompt, expectedIDs...)
}

// isFlowContextHandoffWithSecret is isFlowContextHandoff verifying against an
// explicit per-service secret when non-nil (BUG-288 R19-4). Nil secret falls
// back to multi-secret package verify (tests / legacy callers).
func isFlowContextHandoffWithSecret(secret []byte, prompt string, expectedIDs ...string) bool {
	for _, line := range strings.Split(prompt, "\n") {
		t := strings.TrimSpace(line)
		if !strings.HasPrefix(t, "<!-- flowpilot-fcp:") || !strings.HasSuffix(t, "-->") {
			continue
		}
		body := strings.TrimSuffix(strings.TrimPrefix(t, "<!-- flowpilot-fcp:"), "-->")
		body = strings.TrimSpace(body)
		idx := strings.LastIndex(body, ":")
		if idx < 0 {
			continue
		}
		id, tag := body[:idx], body[idx+1:]
		if id == "" {
			continue
		}
		if len(secret) > 0 {
			if !verifyRunMarkerMACWith(secret, "fcp", id, tag) {
				continue
			}
		} else if !verifyRunMarkerMAC("fcp", id, tag) {
			continue
		}
		if len(expectedIDs) > 0 {
			ok := false
			for _, e := range expectedIDs {
				if e != "" && e == id {
					ok = true
					break
				}
			}
			if !ok {
				continue
			}
		}
		return true
	}
	return false
}

// classifyStepBehavior resolves a step's canonical behavior, preferring its
// declared BehaviorID over its StepType (BUG-NOTE-CP42 #7): step_type is a
// reusable step_definitions key — for a CP-42 generic flow node it's a
// dispatch category like "flow-agent-delegate", which NormalizeBehaviorID's
// alias table doesn't recognize at all, so classifying by StepType alone
// left every UI-authored generic flow's coding/plan steps unclassifiable.
// Falls back to StepType for a step whose definition predates CP-42 (the
// original hardcoded "coding"/"plan" step_type values ARE recognized
// aliases) or never set a BehaviorID.
func classifyStepBehavior(behaviorID, stepType string) (string, bool) {
	if behaviorID != "" {
		return agentpack.NormalizeBehaviorID(behaviorID)
	}
	return agentpack.NormalizeBehaviorID(stepType)
}

// isCodingStepType returns true for a step that represents a Coding step in
// a Flow Mode workflow, classifying by BehaviorID when set, else StepType.
func isCodingStepType(behaviorID, stepType string) bool {
	canonical, ok := classifyStepBehavior(behaviorID, stepType)
	return ok && canonical == "agent.delegate"
}

// isPlanStepType returns true for a step that represents a Plan step,
// classifying by BehaviorID when set, else StepType.
func isPlanStepType(behaviorID, stepType string) bool {
	canonical, ok := classifyStepBehavior(behaviorID, stepType)
	return ok && canonical == "context.produce"
}

// findPlanStepID returns the ID of the most-recent Plan step that precedes
// codingStepID in the steps slice. Returns ("", false) when none is found.
func findPlanStepID(steps []RuntimeWorkflowStep, codingStepID string) (string, bool) {
	codingIdx := -1
	for i, s := range steps {
		if s.ID == codingStepID {
			codingIdx = i
			break
		}
	}
	if codingIdx <= 0 {
		return "", false
	}
	for i := codingIdx - 1; i >= 0; i-- {
		if isPlanStepType(steps[i].BehaviorID, steps[i].StepType) {
			return steps[i].ID, true
		}
	}
	return "", false
}

// FindFlowContextPackage scans the event list newest-first for an
// EventFlowContextPackage tied to planStepID. When planStepID is empty every
// package event is a candidate. Returns the package and true when found.
func FindFlowContextPackage(events []ProviderEvent, planStepID string) (*FlowContextPackage, bool) {
	for i := len(events) - 1; i >= 0; i-- {
		ev := events[i]
		if ev.Type == EventFlowContextPackage && ev.FlowContextPackage != nil {
			if planStepID == "" || ev.WorkflowStepRunID == planStepID {
				cp := *ev.FlowContextPackage
				return &cp, true
			}
		}
	}
	return nil, false
}

// renderFlowContextPrompt dispatches the context.render behavior (Task-176)
// instead of calling ComposeFlowCodingPrompt directly, so the active
// execution path is behavior-ID driven rather than hardcoding the render
// step. A dispatch failure falls back to the direct call — the handler wraps
// the same function, so failure here would indicate a registry defect, not a
// legitimate "no context" case, and must not silently drop the package.
func renderFlowContextPrompt(ctx context.Context, pkg FlowContextPackage, prompt string) string {
	return renderFlowContextPromptWithSecret(ctx, pkg, prompt, nil)
}

// renderFlowContextPromptWithSecret uses an explicit marker secret when
// falling back to ComposeFlowCodingPrompt (BUG-288 R18-4).
func renderFlowContextPromptWithSecret(ctx context.Context, pkg FlowContextPackage, prompt string, secret []byte) string {
	out, err := DefaultBehaviorRegistry().Dispatch(ctx, string(BehaviorContextRender), BehaviorInput{
		Prompt:  prompt,
		Payload: map[string]any{"package": pkg, "markerSecret": secret},
	})
	if err != nil || len(out.NextPromptFragments) == 0 {
		if len(secret) > 0 {
			return ComposeFlowCodingPromptWithSecret(pkg, prompt, secret)
		}
		return ComposeFlowCodingPrompt(pkg, prompt)
	}
	return out.NextPromptFragments[0]
}

// produceFlowContextPackage dispatches the context.produce behavior
// (Task-176) instead of calling BuildFlowContextPackage directly, so the
// active execution path selects context production by behavior ID.
func produceFlowContextPackage(ctx context.Context, workspace string, hints FlowContextHints) (FlowContextPackage, error) {
	out, err := DefaultBehaviorRegistry().Dispatch(ctx, string(BehaviorContextProduce), BehaviorInput{
		WorkspaceCwd:  workspace,
		WorkflowRunID: hints.WorkflowRunID,
		StepRunID:     hints.PlanStepRunID,
		Prompt:        hints.UserPrompt,
		Payload:       map[string]any{"sourceDocId": hints.SourceDocID},
	})
	if err != nil {
		return FlowContextPackage{}, err
	}
	pkg, ok := out.Payload["package"].(FlowContextPackage)
	if !ok {
		return FlowContextPackage{}, fmt.Errorf("context.produce: behavior output missing package")
	}
	return pkg, nil
}

// ComposeFlowCodingPrompt prepends the rendered FlowContextPackage plus a brief
// "use this as context" instruction before the Coding step's user instruction.
// The flowContextHandoffPrefix sentinel prevents injectFeatureHistoryPrompt from
// injecting a duplicate feature block (T-5, Task-169).
func ComposeFlowCodingPrompt(pkg FlowContextPackage, codingInstruction string) string {
	var sb strings.Builder
	sb.WriteString(flowContextHandoffPrefix + "\n")
	// Trusted envelope marker (not user-forgeable without knowing run/package id).
	trustID := pkg.WorkflowRunID
	if trustID == "" {
		trustID = pkg.PackageID
	}
	sb.WriteString(flowContextTrustedMarker(trustID) + "\n\n")
	sb.WriteString(RenderFlowContextPackage(pkg))
	sb.WriteString("\n---\n\n")
	sb.WriteString("[Context use instructions: Use the Flow Context Package above as " +
		"the source of truth for prior work on this feature. " +
		"Do not broaden retrieval unless explicitly instructed. " +
		"Preserve source references when explaining changes.]\n\n")
	sb.WriteString(codingInstruction)
	return sb.String()
}

// ComposeFlowCodingPromptWithSecret is ComposeFlowCodingPrompt using an
// explicit marker secret (BUG-288 R18-4 per-service mint).
func ComposeFlowCodingPromptWithSecret(pkg FlowContextPackage, codingInstruction string, secret []byte) string {
	var sb strings.Builder
	sb.WriteString(flowContextHandoffPrefix + "\n")
	trustID := pkg.WorkflowRunID
	if trustID == "" {
		trustID = pkg.PackageID
	}
	sb.WriteString(flowContextTrustedMarkerWith(secret, trustID) + "\n\n")
	sb.WriteString(RenderFlowContextPackage(pkg))
	sb.WriteString("\n---\n\n")
	sb.WriteString("[Context use instructions: Use the Flow Context Package above as " +
		"the source of truth for prior work on this feature. " +
		"Do not broaden retrieval unless explicitly instructed. " +
		"Preserve source references when explaining changes.]\n\n")
	sb.WriteString(codingInstruction)
	return sb.String()
}

// injectFlowContextIfCoding prepends a FlowContextPackage to providerPrompt
// when stepID is a Coding step in a multi-step workflow run. The package is
// built once and cached on rs.planContextPackage; subsequent calls (retries)
// return the cached package unchanged (T-3, Task-169).
//
// The raw user prompt (rawPrompt = in.Prompt before mode-prefix assembly) is
// used as the feature resolution hint so the catalog scores on the user's
// intent rather than the assembled prompt text.
//
// Returns providerPrompt unchanged when:
//   - workflowStore is nil or has no steps for this run
//   - the step is not a Coding step
//   - BuildFlowContextPackage returns an error
func (s *InteractiveService) injectFlowContextIfCoding(
	ctx context.Context,
	rs *interactiveRun,
	stepID, rawPrompt, providerPrompt string,
) string {
	if s.workflowStore == nil || stepID == "" {
		return providerPrompt
	}
	steps, err := s.workflowStore.LoadRunSteps(ctx, rs.id)
	if err != nil || len(steps) == 0 {
		return providerPrompt
	}

	var thisStepType, thisBehaviorID string
	for _, st := range steps {
		if st.ID == stepID {
			thisStepType = st.StepType
			thisBehaviorID = st.BehaviorID
			break
		}
	}
	if !isCodingStepType(thisBehaviorID, thisStepType) {
		return providerPrompt
	}
	planStepID, _ := findPlanStepID(steps, stepID)

	// Fast path: cached package survives retries without rebuilding.
	s.mu.Lock()
	cached := rs.planContextPackage
	if cached == nil {
		if found, ok := FindFlowContextPackage(rs.events, planStepID); ok {
			cached = found
			rs.planContextPackage = cached
		}
	}
	s.mu.Unlock()

	if cached != nil {
		out := renderFlowContextPromptWithSecret(ctx, *cached, providerPrompt, s.markerSecret)
		s.mu.Lock()
		rs.flowContextInjected = true
		s.mu.Unlock()
		return out
	}

	// Slow path: build a fresh package.
	hints := FlowContextHints{
		WorkflowRunID: rs.id,
		PlanStepRunID: planStepID,
		UserPrompt:    rawPrompt,
		SourceDocID:   rs.sourceDocID,
	}
	built, buildErr := produceFlowContextPackage(ctx, rs.workspaceCwd, hints)
	if buildErr != nil {
		return providerPrompt
	}
	if planStepID == "" {
		built.Warnings = append(built.Warnings,
			"no_plan_step: flow context assembled without a prior plan step")
	}

	pkg := &built
	s.mu.Lock()
	if rs.planContextPackage == nil {
		rs.planContextPackage = pkg
		s.emitLocked(rs, ProviderEvent{
			Type:               EventFlowContextPackage,
			WorkflowRunID:      rs.id,
			WorkflowStepRunID:  planStepID,
			FlowContextPackage: pkg,
		})
	} else {
		// Another path (unlikely — single-threaded turn flow) already set it.
		pkg = rs.planContextPackage
	}
	s.mu.Unlock()

	out := renderFlowContextPromptWithSecret(ctx, *pkg, providerPrompt, s.markerSecret)
	s.mu.Lock()
	// Structural envelope flag — injectFeatureHistory must not trust user text alone.
	if r := s.runs[rs.id]; r != nil {
		r.flowContextInjected = true
	} else {
		rs.flowContextInjected = true
	}
	s.mu.Unlock()
	return out
}

// maybeClearPlanContextForPlanStep clears rs.planContextPackage when stepID
// belongs to a Plan step. Called at the start of each turn so a Plan rerun
// forces the next Coding step to rebuild the package (T-3, Task-169).
func (s *InteractiveService) maybeClearPlanContextForPlanStep(ctx context.Context, runID string, rs *interactiveRun, stepID string) {
	if s.workflowStore == nil || stepID == "" {
		return
	}
	steps, err := s.workflowStore.LoadRunSteps(ctx, runID)
	if err != nil {
		return
	}
	for _, st := range steps {
		if st.ID == stepID && isPlanStepType(st.BehaviorID, st.StepType) {
			s.mu.Lock()
			rs.planContextPackage = nil
			s.mu.Unlock()
			return
		}
	}
}
