package runner

import (
	"context"
	"fmt"
	"time"
)

// fakeAdapterDelay is the inter-event pause the fake adapter uses to simulate
// streaming. Tests set it to 0 for speed.
var fakeAdapterDelay = 8 * time.Millisecond

// fakeProviderAdapter is the Phase 2 test backbone: it emits scripted
// ProviderEvents mirroring the Phase 1 desktop mock scenarios (04-01), exercising
// persistence, replay, the SSE stream, and the approval/question bridges before the
// real Codex adapter (P3) exists.
type fakeProviderAdapter struct {
	key ProviderKey
}

func newFakeProviderAdapter(key ProviderKey) *fakeProviderAdapter {
	return &fakeProviderAdapter{key: key}
}

func (a *fakeProviderAdapter) Key() ProviderKey { return a.key }

func (a *fakeProviderAdapter) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{
		Streaming: true, Resume: true, ApprovalEvents: true, FileEvents: true,
		SkillSelection: true, Mcp: true, Interrupt: true,
	}
}

func (a *fakeProviderAdapter) SendTurn(ctx context.Context, req TurnRequest, b TurnBridge) error {
	scenario := req.Scenario
	if scenario == "" {
		scenario = "normal"
	}

	wait := func() error {
		if fakeAdapterDelay <= 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(fakeAdapterDelay):
			return nil
		}
	}
	delta := func(text string) error {
		if err := wait(); err != nil {
			return err
		}
		b.Emit(ProviderEvent{Type: EventMessageDelta, Text: text})
		return nil
	}
	toolStart := func(name string, input any) error {
		if err := wait(); err != nil {
			return err
		}
		b.Emit(ProviderEvent{Type: EventToolStarted, ToolName: name, Input: input})
		return nil
	}
	toolDone := func(name, status string, output any) error {
		if err := wait(); err != nil {
			return err
		}
		b.Emit(ProviderEvent{Type: EventToolCompleted, ToolName: name, Status: status, Output: output})
		return nil
	}
	fileChanged := func(path, change string) error {
		if err := wait(); err != nil {
			return err
		}
		b.Emit(ProviderEvent{Type: EventFileChanged, Path: path, ChangeType: change})
		return nil
	}
	complete := func(final string) error {
		if err := wait(); err != nil {
			return err
		}
		b.Emit(ProviderEvent{Type: EventMessageCompleted, Text: final})
		b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: final})
		return nil
	}

	return runScenario(scenario, b, wait, delta, toolStart, toolDone, fileChanged, complete)
}

// runScenario keeps SendTurn readable; each closure already short-circuits on ctx.
func runScenario(
	scenario string,
	b TurnBridge,
	wait func() error,
	delta func(string) error,
	toolStart func(string, any) error,
	toolDone func(string, string, any) error,
	fileChanged func(string, string) error,
	complete func(string) error,
) error {
	chain := func(steps ...func() error) error {
		for _, s := range steps {
			if err := s(); err != nil {
				return err
			}
		}
		return nil
	}

	switch scenario {
	case "tool-heavy":
		return chain(
			func() error { return delta("Investigating the codebase...\n") },
			func() error { return toolStart("grep", map[string]any{"pattern": "useAuth("}) },
			func() error { return toolDone("grep", "success", map[string]any{"matches": 7}) },
			func() error { return toolStart("run_tests", map[string]any{"suite": "auth"}) },
			func() error { return toolDone("run_tests", "failed", map[string]any{"failed": 1}) },
			func() error { return delta("One test failed; fixing it.\n") },
			func() error { return toolStart("run_tests", map[string]any{"suite": "auth"}) },
			func() error { return toolDone("run_tests", "success", map[string]any{"passed": 24}) },
			func() error { return complete("All 24 auth tests pass.") },
		)

	case "file-changes":
		return chain(
			func() error { return delta("Applying the edits across the module.\n") },
			func() error { return fileChanged("src/features/cart/cartSlice.ts", "modified") },
			func() error { return fileChanged("src/features/cart/Cart.tsx", "modified") },
			func() error { return fileChanged("src/features/cart/cart.test.ts", "created") },
			func() error { return complete("Refactored the cart module: 2 changed, 1 created.") },
		)

	case "failed":
		return chain(
			func() error { return delta("Attempting the operation...\n") },
			func() error { return toolStart("build", map[string]any{"target": "web"}) },
			func() error { return toolDone("build", "failed", map[string]any{"code": 1}) },
			func() error { return wait() },
			func() error {
				b.Emit(ProviderEvent{Type: EventTurnFailed, Error: "Build failed: type error in cartSlice.ts:42", Recoverable: true})
				return nil
			},
		)

	case "approval-required":
		if err := delta("I need to run a shell command to apply the migration.\n"); err != nil {
			return err
		}
		decision, err := b.RequestApproval(ApprovalDetails{
			Command: "rm -rf ./dist && npm run migrate",
			Cwd:     "/Users/dev/acme-web",
			Reason:  "Runs a database migration after clearing the build output.",
			Decisions: []ApprovalDecisionOption{
				{Value: "approve", Label: "Approve"},
				{Value: "deny", Label: "Deny"},
			},
		})
		if err != nil {
			return err
		}
		if decision == "deny" {
			return chain(
				func() error { return toolDone("shell", "cancelled", map[string]any{"reason": "denied"}) },
				func() error { return complete("Command was denied; I stopped without running it.") },
			)
		}
		return chain(
			func() error { return toolStart("shell", map[string]any{"cmd": "npm run migrate"}) },
			func() error { return toolDone("shell", "success", map[string]any{"code": 0}) },
			func() error { return complete("Migration applied successfully.") },
		)

	case "question-required":
		if err := delta("Before I continue I need a decision from you.\n"); err != nil {
			return err
		}
		choice, err := b.AskQuestion(
			"Which styling approach should I use for the new component?",
			[]QuestionOption{
				{Label: "Tailwind utility classes", Value: "tailwind", Description: "Matches the rest of the app"},
				{Label: "CSS Modules", Value: "css-modules"},
			},
			false,
		)
		if err != nil {
			return err
		}
		picked := "tailwind"
		if len(choice) > 0 {
			picked = choice[0]
		}
		return chain(
			func() error { return delta(fmt.Sprintf("Got it — using %q.\n", picked)) },
			func() error { return fileChanged("src/components/NewWidget.tsx", "created") },
			func() error { return complete(fmt.Sprintf("Component created using %s.", picked)) },
		)

	default: // "normal"
		return chain(
			func() error { return delta("Sure — let me work through this step.\n") },
			func() error { return delta("I reviewed the relevant files; the approach looks sound. ") },
			func() error { return delta("Proceeding with the implementation now.") },
			func() error { return complete("Done. The change is implemented and the step is complete.") },
		)
	}
}
