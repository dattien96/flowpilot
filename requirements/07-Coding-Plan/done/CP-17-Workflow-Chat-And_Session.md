// Pending
Hien tai UIUX kho dung
Muon continue o cung 1 workflow hard

# PLan:
# Workflow UX Revamp & Continuation Plan

The user reported two pain points:
1. The Workflow Run view UX is hard to follow.
2. They cannot "continue to prompt" based on the current results.

## Objective
Transform the Workflow Run Detail page from a static, multi-pane diagnostic view into a linear, chat-like Timeline interface, allowing users to intuitively review artifacts and provide follow-up prompts to refine or continue execution.

## Proposed Changes

### 1. Backend: Enable Step Revival
Currently, the `workflow-engine-submit-step-approval` Edge Function only allows requesting changes on steps that are strictly `WAITING_USER_APPROVAL`.
- **Change:** Loosen this restriction to allow `approve=false` (requesting changes / follow-up) on steps that are `DONE`.
- **Mechanism:** When a follow-up prompt is submitted for a `DONE` step, the engine will append the prompt to the step's logs, increment the retry counter, revert the step status to `PENDING`, and revive the overall workflow run state to `RUNNING`. This seamlessly re-queues the step with the new context.

### 2. Frontend UX: Linear Chat-Like Interface
The current `$runId.tsx` uses multiple collapsible sections split across two columns, which makes the flow of execution hard to read.
- **Redesign:** Implement a single-column, top-down timeline feed.
- **Message Bubbles:**
  - **User Messages:** The initial run prompt and any subsequent follow-up prompts will be rendered as right-aligned or distinctly styled user messages.
  - **AI Responses:** The step execution results (generated artifacts) will be rendered as left-aligned AI message blocks using the `ArtifactContentViewer`.
- **Unified Action Box:**
  - Remove the separate "Approval Gate" textarea.
  - Add a persistent Chat Input box at the bottom of the feed whenever the run is `WAITING_USER_APPROVAL` or `DONE`.
  - Submitting text in this box will trigger the "Follow up / Request Changes" flow, reviving the step and continuing the conversation.
  - If a step explicitly requires approval, we will also show an "Approve & Complete" button next to the chat input to finalize it.

## User Review Required

> [!IMPORTANT]
> **UX Transition:** This plan will fundamentally change the Workflow Run page to look more like a conversational agent (similar to Claude/ChatGPT) rather than a rigid CI/CD pipeline view. The AI's generated files will appear as "messages" in the timeline. Does this align with your vision for making the UX easier to follow?

> [!TIP]
> **Continuing the Flow:** By reusing the "Changes Requested" logic, any follow-up prompt you send will automatically be appended to the AI's context window for that step, allowing it to iterate on the exact same artifact. 

Let me know if you approve this approach or if you'd like to adjust the behavior!


# Review part
Findings
High: The plan’s core “continue to prompt” claim is not implemented by the existing retry path. The plan says a follow-up prompt will be appended to context and reused on retry (implementation_plan.md (line 15), implementation_plan.md (line 35)), but the current reset flow only writes rejection_note and a log entry (workflow-engine-runtime.ts (line 614), workflow-engine-state-machine.ts (line 141)). The prompt builder does not read either field when constructing the next step prompt (workflow-start-runtime.ts (line 235)). As written, the step would retry without the user’s follow-up instructions.

High: The plan does not address downstream invalidation when reviving a completed step. It proposes allowing DONE -> PENDING for follow-up (implementation_plan.md (line 14)), but the workflow progress engine skips steps already marked DONE (workflow-engine-state-machine.ts (line 54)). If a user revives step N after step N+1 or later steps already completed, those downstream artifacts remain stale unless the plan explicitly resets or invalidates them too.

Medium: The frontend baseline in the plan is stale. It says the run detail page is still a multi-pane diagnostic layout and needs to become a linear timeline (implementation_plan.md (line 17)), but the current route already renders a timeline-style feed, step bubbles, diagnostics foldouts, and a sticky follow-up composer (\$runId.tsx (line 676), \$runId.tsx (line 772)). Planning from the wrong starting point will produce duplicate or conflicting work.

Medium: The plan does not define which step a run-level follow-up targets once the run is DONE. It proposes showing a chat input whenever the run is WAITING_USER_APPROVAL or DONE (implementation_plan.md (line 25)), but the backend API is step-scoped and requires a concrete stepId (workflow-engine-submit-step-approval/index.ts (line 20)). The plan needs an explicit targeting rule such as “latest step only”, “selected timeline node”, or “latest step with an artifact”, otherwise the UI cannot reliably know which step to revive.

The main gap is backend semantics, not UI styling. I would revise the plan to add: a structured follow-up prompt source that is injected into buildWorkflowStepPrompt, a downstream reset policy for revived steps, and an explicit step-targeting rule for post-completion follow-ups.