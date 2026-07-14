package flowgate

import (
	"strings"
	"testing"
)

func TestMissingTelegramSendsReturnsEmptyWhenNoTargetsRequired(t *testing.T) {
	if got := MissingTelegramSends("anything", nil); got != nil {
		t.Fatalf("expected nil for no required targets, got %v", got)
	}
}

func TestMissingTelegramSendsDetectsMessageIDEvidence(t *testing.T) {
	final := "Sent the notification. message_id: 42"
	got := MissingTelegramSends(final, []string{"-100123456"})
	if got != nil {
		t.Fatalf("expected no missing sends when message_id is present, got %v", got)
	}
}

func TestMissingTelegramSendsFlagsMissingEvidence(t *testing.T) {
	final := "I described sending a message but did not actually call the tool."
	got := MissingTelegramSends(final, []string{"-100123456"})
	if len(got) != 1 || got[0] != "-100123456" {
		t.Fatalf("expected the required chat id reported missing, got %v", got)
	}
}

// TestMissingTelegramSendsRequiresOneMessageIDPerTarget verifies the v1
// simplification documented on MissingTelegramSends: with 2 required
// targets, at least 2 message_id occurrences must be present.
func TestMissingTelegramSendsRequiresOneMessageIDPerTarget(t *testing.T) {
	final := "Sent. message_id: 1"
	got := MissingTelegramSends(final, []string{"-100111", "-100222"})
	if len(got) != 2 {
		t.Fatalf("expected both targets reported missing with only 1 message_id, got %v", got)
	}

	finalTwo := "Sent both. message_id: 1 and message_id: 2"
	if got := MissingTelegramSends(finalTwo, []string{"-100111", "-100222"}); got != nil {
		t.Fatalf("expected no missing sends with 2 message_id occurrences for 2 targets, got %v", got)
	}
}

// TestEvaluateRequiredTelegramSendMissing mirrors
// TestEvaluateRequiredArtifactOutputMissing for the Task-233 gate.
func TestEvaluateRequiredTelegramSendMissing(t *testing.T) {
	tr := TurnResult{
		FinalMessage:          "no evidence of sending anything",
		RequiredTelegramSends: []string{"-100123456"},
	}
	violations := Evaluate(tr, DefaultRules())
	found := false
	for _, v := range violations {
		if v.Rule.ID == "r-artifact-telegram-sent" {
			found = true
			if !strings.Contains(v.Detail, "-100123456") {
				t.Errorf("expected violation detail to mention the chat id, got %q", v.Detail)
			}
		}
	}
	if !found {
		t.Fatal("expected r-artifact-telegram-sent violation when no message_id evidence is present")
	}
}

func TestEvaluateRequiredTelegramSendSatisfied(t *testing.T) {
	tr := TurnResult{
		FinalMessage:          "Sent. message_id: 99",
		RequiredTelegramSends: []string{"-100123456"},
	}
	violations := Evaluate(tr, DefaultRules())
	for _, v := range violations {
		if v.Rule.ID == "r-artifact-telegram-sent" {
			t.Fatalf("expected no violation when message_id evidence is present, got %#v", v)
		}
	}
}
