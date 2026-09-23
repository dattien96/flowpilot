package runner

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

// CA-921: `devin -p` (and any provider that narrates status updates) writes each
// status to stdout back-to-back with NO separator — no newline, no CR, no ANSI.
// The scaffold feed must re-insert line boundaries so clients render one status
// per line instead of one continuous blob.

// joinerClock gives tests a mutable time source for the quiet-gap rule.
func joinerClock() (*time.Time, func() time.Time) {
	t := new(time.Time)
	*t = time.Now()
	return t, func() time.Time { return *t }
}

func TestScaffoldStreamJoiner_SentenceEndThenUppercaseBreaks(t *testing.T) {
	now, clock := joinerClock()
	j := newScaffoldStreamJoiner(clock)
	if got := j.push("I'll start by reading the four required skill files"); got != "I'll start by reading the four required skill files" {
		t.Fatalf("first chunk = %q, want unchanged", got)
	}
	*now = now.Add(300 * time.Millisecond)
	if got := j.push("."); got != "." {
		t.Fatalf("punctuation continuation = %q, want unchanged (mid-sentence split)", got)
	}
	*now = now.Add(4 * time.Second)
	if got := j.push("All four skills read."); got != "\nAll four skills read." {
		t.Fatalf("new status chunk = %q, want leading newline", got)
	}
}

func TestScaffoldStreamJoiner_SpaceOrLowercaseContinuationNeverBreaks(t *testing.T) {
	now, clock := joinerClock()
	j := newScaffoldStreamJoiner(clock)
	j.push("All four skills read. Now let me check the")
	*now = now.Add(300 * time.Millisecond)
	if got := j.push(" current workspace state."); got != " current workspace state." {
		t.Fatalf("space-led continuation = %q, want unchanged", got)
	}
	*now = now.Add(3 * time.Second)
	if got := j.push("then the rest follows"); got != "then the rest follows" {
		t.Fatalf("lowercase continuation after quiet gap = %q, want unchanged", got)
	}
}

func TestScaffoldStreamJoiner_QuietGapThenUppercaseBreaks(t *testing.T) {
	now, clock := joinerClock()
	j := newScaffoldStreamJoiner(clock)
	j.push("Reading config files") // no terminal punctuation
	*now = now.Add(3 * time.Second)
	if got := j.push("Now writing packages"); got != "\nNow writing packages" {
		t.Fatalf("quiet-gap status = %q, want leading newline", got)
	}
}

func TestScaffoldStreamJoiner_QuickUppercaseWithoutPunctuationStaysGlued(t *testing.T) {
	now, clock := joinerClock()
	j := newScaffoldStreamJoiner(clock)
	j.push("Now let me read")
	// A fast uppercase continuation is mid-sentence streaming, not a new status.
	*now = now.Add(300 * time.Millisecond)
	if got := j.push("I will check"); got != "I will check" {
		t.Fatalf("fast uppercase continuation = %q, want unchanged", got)
	}
}

func TestScaffoldStreamJoiner_PhaseBoundaryForcesBreak(t *testing.T) {
	now, clock := joinerClock()
	j := newScaffoldStreamJoiner(clock)
	j.push("turn one narration ended mid-word")
	j.markBoundary() // e.g. ai_turn -> gate phase event
	*now = now.Add(100 * time.Millisecond)
	if got := j.push("repair turn narrates"); got != "\nrepair turn narrates" {
		t.Fatalf("post-phase chunk = %q, want leading newline", got)
	}
}

func TestScaffoldStreamJoiner_NoDoubleBreakAfterProviderNewline(t *testing.T) {
	now, clock := joinerClock()
	j := newScaffoldStreamJoiner(clock)
	j.push("line one\n")
	j.markBoundary()
	*now = now.Add(3 * time.Second)
	if got := j.push("line two"); got != "line two" {
		t.Fatalf("chunk after provider newline = %q, want unchanged (no double break)", got)
	}
}

func TestScaffoldProgress_OutputDeltasBreakBetweenStatuses(t *testing.T) {
	dir := newScaffoldWorkspace(t)
	executor := &recordingScaffoldExecutor{
		onCall: func(call int, req PromptExecutionRequest) (PromptExecutionResult, error) {
			if req.OnStdoutDelta != nil {
				req.OnStdoutDelta("I'll start by reading the four required skill files")
				req.OnStdoutDelta(".")
				req.OnStdoutDelta("All four skills read. Now let me check the")
				req.OnStdoutDelta(" current workspace state.")
				req.OnStdoutDelta("The workspace has existing content.")
			}
			return PromptExecutionResult{
				Status:      "success",
				RunID:       fmt.Sprintf("scaffold-run-%d", call),
				ProviderKey: req.ProviderKey,
				ExitCode:    0,
			}, nil
		},
	}
	recipe := realScaffoldRecipe(t, "true")
	_, srv := newScaffoldTestService(t, dir, "react-native", dispatcherForRecipe(executor, recipe))

	status, body := doJSON(t, http.MethodPost, srv.URL+"/client/projects/proj-rn/scaffold",
		ScaffoldDispatchRequestBody{WorkingDirectory: dir, Platform: "react-native", ProviderKey: "devin"}, nil)
	if status != http.StatusOK {
		t.Fatalf("dispatch status = %d body=%s", status, body)
	}

	_, body = doJSON(t, http.MethodGet, srv.URL+"/client/projects/proj-rn/scaffold/progress", nil, nil)
	snap := scaffoldProgressFromResponse(t, body)
	got := scaffoldOutputText(snap.Events)
	want := "I'll start by reading the four required skill files.\n" +
		"All four skills read. Now let me check the current workspace state.\n" +
		"The workspace has existing content."
	if got != want {
		t.Fatalf("output text = %q, want one status per line:\n%s", got, want)
	}
}

// A phase event between output runs (ai_turn -> gate -> heal turn) must force
// the next output chunk onto a fresh line — turns never glue onto each other.
func TestWrapScaffoldProgressSink_PhaseSeparatesOutputRuns(t *testing.T) {
	var got []string
	sink := wrapScaffoldProgressSink(func(ev ScaffoldProgressEvent) {
		got = append(got, ev.Kind+":"+ev.Text)
	}, time.Now)
	sink(ScaffoldProgressEvent{Kind: "output", Phase: "ai_turn", Text: "turn one tail"})
	sink(ScaffoldProgressEvent{Kind: "phase", Phase: "gate", Text: "compiler gate attempt 1"})
	sink(ScaffoldProgressEvent{Kind: "output", Phase: "ai_turn", Text: "turn two head"})
	if len(got) != 3 || got[2] != "output:\nturn two head" {
		t.Fatalf("events = %v, want the post-phase output chunk to carry a leading newline", got)
	}
}

func TestWrapScaffoldProgressSink_NilSinkStaysNil(t *testing.T) {
	if wrapScaffoldProgressSink(nil, time.Now) != nil {
		t.Fatal("nil sink must stay nil — executeTurn attaches no tailer without one")
	}
}
