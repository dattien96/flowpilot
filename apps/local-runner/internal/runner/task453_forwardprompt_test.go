package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Task-453 (CP-89): the forward entry child's prompt package — settled chat
// transcript (oldest→newest, user+assistant roles only) + the pinned forward
// text, under a hard context budget, oldest material degrading first.

// task453RunWithTurns builds a pending-armed run and injects settled
// transcript events directly (unit-level equivalent of real chat turns).
func task453RunWithTurns(t *testing.T, svc *InteractiveService, turns []transcriptTurn) *interactiveRun {
	t.Helper()
	h, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		FlowRef: "task-harness", FlowArm: "pending",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[h.RunID]
	seq := int64(1)
	for _, tr := range turns {
		rs.events = append(rs.events,
			ProviderEvent{Type: EventTurnStarted, Seq: seq, Prompt: tr.User},
			ProviderEvent{Type: EventTurnCompleted, Seq: seq + 1, FinalMessage: tr.Assistant},
		)
		seq += 2
	}
	svc.mu.Unlock()
	return rs
}

func TestTask453_PackageContainsForwardAndTranscript(t *testing.T) {
	svc := task451Service(t)
	rs := task453RunWithTurns(t, svc, []transcriptTurn{
		{User: "first question", Assistant: "first answer"},
		{User: "second question", Assistant: "second answer"},
	})
	pkg, e := svc.buildForwardPromptPackage(rs, "ship it now", 8000, nil)
	if e != nil {
		t.Fatalf("pack: %v", e)
	}
	for _, want := range []string{"ship it now", "first question", "first answer", "second question", "second answer"} {
		if !strings.Contains(pkg.Prompt, want) {
			t.Fatalf("package must contain %q\n%s", want, pkg.Prompt)
		}
	}
	if pkg.TurnsIncluded != 2 {
		t.Fatalf("TurnsIncluded = %d, want 2", pkg.TurnsIncluded)
	}
	// Forward text must come AFTER the transcript (pinned last).
	if strings.Index(pkg.Prompt, "ship it now") < strings.Index(pkg.Prompt, "second answer") {
		t.Fatal("forward text must follow the transcript block")
	}
}

func TestTask453_OnlySettledTurnsIncluded(t *testing.T) {
	svc := task451Service(t)
	rs := task453RunWithTurns(t, svc, []transcriptTurn{
		{User: "settled question", Assistant: "settled answer"},
	})
	// Append an in-flight tail: TurnStarted with no completion yet.
	svc.mu.Lock()
	rs.events = append(rs.events,
		ProviderEvent{Type: EventTurnStarted, Seq: 99, Prompt: "half written turn"})
	svc.mu.Unlock()
	pkg, e := svc.buildForwardPromptPackage(rs, "forward", 8000, nil)
	if e != nil {
		t.Fatalf("pack: %v", e)
	}
	if strings.Contains(pkg.Prompt, "half written turn") {
		t.Fatalf("in-flight turn leaked into the package:\n%s", pkg.Prompt)
	}
	if pkg.TurnsIncluded != 1 {
		t.Fatalf("TurnsIncluded = %d, want 1", pkg.TurnsIncluded)
	}
}

func TestTask453_RolesFiltered(t *testing.T) {
	svc := task451Service(t)
	rs := task453RunWithTurns(t, svc, []transcriptTurn{
		{User: "real user text", Assistant: "real reply"},
	})
	// A system-frame turn (flow-engine reprompt shape) — must be excluded
	// even though it produced a prompt/assistant pair in the event log.
	svc.mu.Lock()
	rs.events = append(rs.events,
		ProviderEvent{Type: EventTurnStarted, Seq: 50, Prompt: "[flow-engine] Agent results ready"},
		ProviderEvent{Type: EventTurnCompleted, Seq: 51, FinalMessage: "internal system reply"},
	)
	svc.mu.Unlock()
	pkg, e := svc.buildForwardPromptPackage(rs, "go", 8000, nil)
	if e != nil {
		t.Fatalf("pack: %v", e)
	}
	if strings.Contains(pkg.Prompt, "Agent results ready") || strings.Contains(pkg.Prompt, "internal system reply") {
		t.Fatalf("system-frame turn leaked into the package:\n%s", pkg.Prompt)
	}
	if !strings.Contains(pkg.Prompt, "real user text") {
		t.Fatal("real user turn missing")
	}
}

func TestTask453_BudgetCapTruncatesOldestFirst(t *testing.T) {
	svc := task451Service(t)
	var turns []transcriptTurn
	for i := 0; i < 12; i++ {
		turns = append(turns, transcriptTurn{
			User:      "turn-" + strings.Repeat(string(rune('a'+i)), 400),
			Assistant: "answer-" + strings.Repeat(string(rune('a'+i)), 400),
		})
	}
	rs := task453RunWithTurns(t, svc, turns)
	budget := int64(900) // tokens → ~3600 bytes; far under the ~9.6KB transcript
	pkg, e := svc.buildForwardPromptPackage(rs, "pin this forward text", budget, nil)
	if e != nil {
		t.Fatalf("pack: %v", e)
	}
	if pkg.Bytes > int(budget)*4+512 {
		t.Fatalf("package %d bytes exceeds budget bound", pkg.Bytes)
	}
	if !strings.Contains(pkg.Prompt, "pin this forward text") {
		t.Fatal("forward text must survive the cap intact")
	}
	if !pkg.Degraded {
		t.Fatal("oversized transcript must flag Degraded")
	}
	// Oldest material dropped first: turn 'aaaa...' gone, newest 'llll' kept.
	if strings.Contains(pkg.Prompt, strings.Repeat("a", 400)) {
		t.Fatal("oldest turn survived the cap — degradation order wrong")
	}
	if !strings.Contains(pkg.Prompt, strings.Repeat("l", 400)) {
		t.Fatal("newest turn missing — cap must keep the freshest context")
	}
}

func TestTask453_EmptyTranscriptForwardsTextOnly(t *testing.T) {
	svc := task451Service(t)
	rs := task453RunWithTurns(t, svc, nil)
	pkg, e := svc.buildForwardPromptPackage(rs, "just the forward text", 8000, nil)
	if e != nil {
		t.Fatalf("pack: %v", e)
	}
	if pkg.TurnsIncluded != 0 {
		t.Fatalf("TurnsIncluded = %d, want 0", pkg.TurnsIncluded)
	}
	if !strings.Contains(pkg.Prompt, "just the forward text") {
		t.Fatal("forward text missing from empty-transcript package")
	}
}

func TestTask453_BareForwardEmptyTextStillPacks(t *testing.T) {
	svc := task451Service(t)
	rs := task453RunWithTurns(t, svc, []transcriptTurn{
		{User: "earlier context", Assistant: "earlier reply"},
	})
	pkg, e := svc.buildForwardPromptPackage(rs, "", 8000, nil)
	if e != nil {
		t.Fatalf("bare forward pack: %v", e)
	}
	if !strings.Contains(pkg.Prompt, "earlier context") {
		t.Fatal("transcript-only package must still carry the transcript")
	}
}

func TestTask453_DeterministicReplay(t *testing.T) {
	svc := task451Service(t)
	rs := task453RunWithTurns(t, svc, []transcriptTurn{
		{User: "u1", Assistant: "a1"}, {User: "u2", Assistant: "a2"},
	})
	p1, e1 := svc.buildForwardPromptPackage(rs, "fwd", 2000, nil)
	p2, e2 := svc.buildForwardPromptPackage(rs, "fwd", 2000, nil)
	if e1 != nil || e2 != nil {
		t.Fatalf("pack errors: %v %v", e1, e2)
	}
	if p1 != p2 {
		t.Fatal("identical state must produce identical packages")
	}
}

func TestTask453_AuditEntryRecorded(t *testing.T) {
	diagDir := t.TempDir()
	t.Setenv("FLOWPILOT_FLOW_DIAG_DIR", diagDir)
	svc := task451Service(t)
	rs := task453RunWithTurns(t, svc, []transcriptTurn{
		{User: "hello", Assistant: "hi"},
	})
	if _, e := svc.startTurn(rs.id, TurnInput{StepID: "chat", ForwardFlow: true, Prompt: "go"}, "", ""); e != nil {
		t.Fatalf("forward turn: %v", e)
	}
	raw, err := os.ReadFile(filepath.Join(diagDir, rs.id+".ndjson"))
	if err != nil {
		t.Fatalf("audit file: %v", err)
	}
	content := string(raw)
	for _, want := range []string{"forward_prompt_packed", "turns_included", "bytes", "degraded"} {
		if !strings.Contains(content, want) {
			t.Fatalf("audit entry missing %q:\n%s", want, content)
		}
	}
}
