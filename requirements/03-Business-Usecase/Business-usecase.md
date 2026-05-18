# FlowPilot - Business Use Cases

This document outlines the real-world business use cases for FlowPilot, categorized by our primary target users. The goal of FlowPilot is to act as an operating layer for engineering work—moving beyond simple prompt engineering into structured Context, Workflow, and Harness Engineering.

---

## 1. Solo Developer (Primary User)
**Goal:** Manage the full software lifecycle from idea to delivery without losing context or requiring external team members.

### UC 1.1: Vague Idea to Shipped Feature
**Context:** The Solo Dev has a raw idea or user request.
**Action:** Triggers the "Full End-to-End Flow". The system sequentially generates a Business Summary, Product Spec, Tech Spec, and TDD plan.
**Value:** Replaces ad-hoc prompting with a deterministic, saved workflow that ensures no architectural or product edge cases are missed.

### UC 1.2: AI-Assisted Implementation Loop
**Context:** The Tech Spec and TDD signatures are ready.
**Action:** The Solo Dev triggers the "Code/Review Loop". The AI agent writes the code and test bodies, compiles the project, and acts as its own peer reviewer. The loop repeats until all unit tests pass and code coverage >80%.
**Value:** Acts as a tireless pair-programmer and reviewer, ensuring high quality without needing a human peer.

### UC 1.3: Post-Release Feedback Loop
**Context:** A feature has been released to production.
**Action:** The Solo Dev uses the "Analytics Review Flow". The system ingests runtime feedback, crash logs, and usage events, automatically proposing bug fixes or follow-up features.
**Value:** Closes the loop between production and planning, maintaining context effortlessly.

---

## 2. Engineering Leader (Primary User)
**Goal:** Maintain control, ensure auditability, manage risk, and plan team delivery efficiently.

### UC 2.1: Architecture & Risk Review
**Context:** A developer proposes a major feature implementation.
**Action:** The Leader reviews the generated Tech Spec. The system highlights security risks, potential bottlenecks, and architecture violations before the Leader clicks "Approve".
**Value:** Provides decision support, ensuring costly architectural mistakes are caught before coding begins.

### UC 2.2: Task Breakdown & Delegation
**Context:** A large feature is approved for development.
**Action:** The Leader triggers the "Task Breakdown & Delegation Flow". The system dissects the Tech Spec into granular tasks and generates a Master Schedule based on the team's available skills.
**Value:** Eliminates the manual overhead of sprint planning and Jira ticket creation.

### UC 2.3: Automated Incident Investigation
**Context:** A critical crash spikes in production.
**Action:** The Leader feeds the crash logs into the "Root Cause Analysis Flow". The system reads the logs, traces back to the relevant code and PRD, identifies the root cause, and assigns a fix plan to a developer.
**Value:** Radically reduces Mean Time To Resolution (MTTR) and prevents context-loss during emergencies.

### UC 2.4: Approval Gates & Auditability
**Context:** A feature is nearing completion.
**Action:** The system strictly enforces the "Release Readiness Step", ensuring CI passed, AI review was completed, and human approval was granted.
**Value:** Provides a permanent, auditable history of *why* decisions were made, *who* approved them, and *what* context was used.

### UC 2.5: Workflow State Monitoring via Telegram
**Context:** A long-running automated workflow is executing in the background.
**Action:** The Leader or Solo Dev receives real-time updates and the final result state directly in Telegram via the "Telegram Notification Step".
**Value:** Keeps the team informed without needing to actively monitor the dashboard.

---

## 3. Developer
**Goal:** Execute implementations quickly with clear guidelines, high quality, and minimal friction.

### UC 3.1: Guided Implementation from Spec
**Context:** A developer is assigned a task with an existing Tech Spec.
**Action:** The developer uses the system to understand the exact scope. The system provides clear implementation steps, test expectations, and necessary analytics hooks.
**Value:** Removes ambiguity ("What exactly should I build?") and prevents scope creep.

### UC 3.2: Local AI Peer Review
**Context:** The developer finishes a draft of the code.
**Action:** Before opening a PR, they run an AI review step locally. The system provides feedback on edge cases, naming conventions, and performance.
**Value:** Acts as a pre-commit guardrail, leading to faster PR approvals from human reviewers later.

### UC 3.3: Context-Rich Bug Fixing
**Context:** The developer is assigned a bug fix.
**Action:** Triggers the "Bug Fix Flow". The system provides the original Product Spec, highlighting the gap between the intended logic and the actual implementation.
**Value:** Eliminates the "archaeology" of debugging undocumented legacy code.

### UC 3.4: Code Traceability to Jira
**Context:** A developer is investigating a suspicious line of code while fixing a bug.
**Action:** Uses the "Bug Traceability Flow" to trace the code change back to its original Jira ticket and PR.
**Value:** Provides immediate context on *why* a specific implementation was chosen, removing guesswork.

### UC 3.5: New Member Onboarding & Walkthrough
**Context:** A new developer joins the project and needs to understand the architecture or a specific module.
**Action:** Triggers the "Onboarding Flow" to generate a high-level summary and read through project docs automatically.
**Value:** Drastically reduces onboarding time and reliance on senior developers for explanations.

---

## 4. PM / Product Owner
**Goal:** Turn business intent into structured specs, maintain alignment, and track product outcomes.

### UC 4.1: PRD & Acceptance Criteria Generation
**Context:** The PM has unstructured customer feedback.
**Action:** Inputs the raw notes into the system. The AI generates a structured Product Requirement Document (PRD) with edge cases and Acceptance Criteria.
**Value:** Standardizes product documentation and saves hours of manual writing.

### UC 4.2: Edge Case Discovery
**Context:** The PM is defining a new feature.
**Action:** The system automatically challenges the PRD, asking questions about conflicting logic or missing edge cases (e.g., "What happens to the user's cart if their session expires during checkout?").
**Value:** Prevents costly requirement changes during the middle of the development cycle.

### UC 4.3: Traceability & Scope Control
**Context:** A feature is ready for release.
**Action:** The PM reviews the "Release Readiness" output, which traces the final code directly back to the initial business intent to verify all acceptance criteria were met.
**Value:** Ensures what was built is exactly what was requested, providing complete business-to-tech traceability.
