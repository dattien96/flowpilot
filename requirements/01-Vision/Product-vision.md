# 1. Why? 
The key problem when using AI (LLM) in software development is:

- Chat-based AI usage is useful but inconsistent.
- Each conversation can lose context.
- Prompts are hard to reuse.
- Review quality depends on how much context is manually supplied.
- There is no persistent workflow history.
- There is no approval gate.
- There is no cost tracking, model routing, or production monitoring.
- There is no connection between planning, coding, review, release, and runtime feedback.

Therefore, the project should become a structured AI workflow platform.

We want: structured inputs, selected context, persistent outputs, approval gates, logs, cost visibility, and decision history.

# 2. The main shift
The strategic shift is:

Prompt Engineering
→ Context Engineering
→ Workflow Engineering
→ Harness Engineering
→ AI-assisted Engineering Platform

Meaning:
- Prompt Engineering: writing good instructions for one task.
- Context Engineering: selecting the correct source code, docs, logs, tickets, and business rules.
- Workflow Engineering: turning repeated AI usage into deterministic steps with inputs, outputs, approval gates, and validation.
- Harness Engineering: building the runtime environment that safely executes AI workflows, tools, models, logs, retries, and reviews.
- AI-assisted Engineering Platform: a complete system that helps the owner/team plan, build, review, release, and monitor features.

The final system should not be a better ChatGPT window. It should be an operating layer around engineering work.

# 3. Vision

FlowPilot is an intelligent engineering platform that helps developers build better software faster.

Vision statement:
Build a production-ready AI workflow platform that helps an engineering team convert business ideas into product specs, technical specs, implementation plans, coding tasks, AI reviews, release checks, and runtime feedback reports.

The system should help answer these questions clearly:
- What is the business goal of this feature?
- What exactly should be built?
- Which files/layers should change?
- What are the risks?
- What tests are required?
- What analytics events are required?
- What should AI implement?
- What should AI only review?
- Where must a human approve before continuing?
- What happened after release?
- Which production signals should feed back into future planning?

# 4. Output
Good output is not only code. Good output includes:
- business summary,
- product requirement document,
- acceptance criteria,
- technical design,
- task breakdown,
- implementation plan,
- test plan,
- analytics plan,
- AI review report,
- release risk report,
- production health report,
- decision history.
