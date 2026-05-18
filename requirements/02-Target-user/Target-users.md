# 1. Solo Dev
**This is one of the best-fit target users.**
Expectation: use one system for the full loop from idea to delivery and feedback.

- Start from a vague idea or feature request and turn it into feature intake, product spec, tech spec, task breakdown, implementation plan, review, release check, and runtime feedback.
- Expect the tool to replace ad hoc prompting with repeatable workflows and stored artifacts.
- Expect strong visibility into what context was used, what changed, what was approved, and what to do next.

# 2. Dev
Expectation: help with execution quality, not product ownership.

- Start from an approved feature, tech spec, or assigned task.
- Use the tool to read context, inspect output history, run workflow steps, review AI drafts, and check bug tracing / PR review / incident reports.
- Expect clear implementation guidance, test expectations, analytics checks, and review findings with severity.
- Do not expect full autonomy or direct production-changing actions without approval.

# 3. Leader
Operate as engineering owner, not just coder.
**This is also the primary target user**

- Use the tool to control workflow state, approval gates, risk review, delivery readiness, incident investigation, and later sprint planning / task assignment.
- Expect auditable outputs, version history, CI/review signals, cost and failure visibility, and team-level decision support.
- Expect the system to help answer: what is risky, what is blocked, what needs approval, what changed, and who should own what.

# 4. PM / Product Owner
Collaborate on clarity and approval, not on code internals.

- Start with business idea, user problem, scope, acceptance criteria, links, and constraints.
- Use the tool to review feature briefs, product specs, open questions, edge cases, metrics, and approval requests.
- Expect transparent business-to-tech traceability: idea -> product spec -> tech spec -> task breakdown -> release/risk output.
- Do not expect to manage low-level implementation details; the value is alignment, scope control, and decision traceability.

# 5. Summary
- Dev: “Tell me exactly what to build, what to test, and what is risky.”
- Solo Dev: “Take me from vague idea to shipped feature with reusable AI workflows.”
- Leader: “Give me control, auditability, approvals, risk visibility, and delivery planning.”
- PM/Product Owner: “Turn business intent into structured specs and make decisions traceable.”

**One important conclusion from the docs: the product is optimized first for Solo Dev and Leader. PM/Product Owner and Dev are important users, but more as collaborators around the core workflow.**