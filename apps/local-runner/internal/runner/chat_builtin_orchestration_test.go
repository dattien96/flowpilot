package runner

import "testing"

func TestBuiltinOrchestrationOptionsBugModeOffersReviewLoop(t *testing.T) {
	// review-loop is hidden (selectableIn []) — the review-until-clean loop
	// lives inside the harness flows now. The Bug sub-mode offers nothing
	// until a harness opts into the chat picker.
	opts, err := BuiltinOrchestrationOptions("bug")
	if err != nil {
		t.Fatalf("BuiltinOrchestrationOptions: %v", err)
	}
	for _, opt := range opts {
		if opt.FlowRef == "flowpilot-core-flow-pack/review-loop" {
			t.Fatalf("review-loop must stay hidden, got %#v", opts)
		}
	}
}

func TestBuiltinOrchestrationOptionsNormalModeIsEmpty(t *testing.T) {
	opts, err := BuiltinOrchestrationOptions("normal")
	if err != nil {
		t.Fatalf("BuiltinOrchestrationOptions: %v", err)
	}
	if len(opts) != 0 {
		t.Fatalf("expected no options for normal sub-mode, got %#v", opts)
	}
}

func TestBuiltinOrchestrationOptionsTaskModeIsEmpty(t *testing.T) {
	opts, err := BuiltinOrchestrationOptions("task")
	if err != nil {
		t.Fatalf("BuiltinOrchestrationOptions: %v", err)
	}
	if len(opts) != 0 {
		t.Fatalf("expected no options for task sub-mode, got %#v", opts)
	}
}

func TestBuiltinOrchestrationOptionsExcludesChatBaselineFlow(t *testing.T) {
	// rag-harness is chatBaseline=true and must never appear as a picker
	// option regardless of sub-mode.
	for _, subMode := range []string{"bug", "normal", "task"} {
		opts, err := BuiltinOrchestrationOptions(subMode)
		if err != nil {
			t.Fatalf("BuiltinOrchestrationOptions(%q): %v", subMode, err)
		}
		for _, opt := range opts {
			if opt.FlowRef == "flowpilot-core-flow-pack/rag-harness" {
				t.Errorf("rag-harness (chatBaseline) must not appear as a picker option for subMode=%q", subMode)
			}
		}
	}
}

func TestBuiltinOrchestrationOptionsEmptySubModeReturnsEmpty(t *testing.T) {
	opts, err := BuiltinOrchestrationOptions("")
	if err != nil {
		t.Fatalf("BuiltinOrchestrationOptions: %v", err)
	}
	if len(opts) != 0 {
		t.Fatalf("expected no options for empty sub-mode, got %#v", opts)
	}
}

func TestValidateChatOrchestrationSelectionAllowsEmptyFlowRef(t *testing.T) {
	if err := validateChatOrchestrationSelection("normal", ""); err != nil {
		t.Errorf("empty flowRef must always be valid, got: %v", err)
	}
	if err := validateChatOrchestrationSelection("", ""); err != nil {
		t.Errorf("empty subMode+flowRef must always be valid, got: %v", err)
	}
}

func TestValidateChatOrchestrationSelectionAcceptsReviewLoopInBugMode(t *testing.T) {
	// review-loop is hidden: the old Bug-mode ref must now be rejected.
	if err := validateChatOrchestrationSelection("bug", "flowpilot-core-flow-pack/review-loop"); err == nil {
		t.Error("expected error selecting the hidden review-loop under bug sub-mode")
	}
}

func TestValidateChatOrchestrationSelectionRejectsReviewLoopInNormalMode(t *testing.T) {
	if err := validateChatOrchestrationSelection("normal", "flowpilot-core-flow-pack/review-loop"); err == nil {
		t.Error("expected error selecting review-loop under normal sub-mode")
	}
}

func TestValidateChatOrchestrationSelectionRejectsUnknownFlowRef(t *testing.T) {
	if err := validateChatOrchestrationSelection("bug", "flowpilot-core-flow-pack/does-not-exist"); err == nil {
		t.Error("expected error for unknown flowRef")
	}
}

func TestValidateChatOrchestrationSelectionRejectsChatBaselineFlow(t *testing.T) {
	// rag-harness is the always-on baseline; it must never be selectable
	// through the picker contract even if the caller supplies a matching ref.
	if err := validateChatOrchestrationSelection("bug", "flowpilot-core-flow-pack/rag-harness"); err == nil {
		t.Error("expected error selecting the chat-baseline flow explicitly")
	}
}
