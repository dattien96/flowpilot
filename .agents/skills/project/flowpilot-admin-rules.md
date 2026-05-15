# FlowPilot Admin Rules

## Purpose

This skill defines project-specific rules for implementing the FlowPilot Admin MVP Skeleton.

## Current goal

Build the admin skeleton first before real AI orchestration.

The admin must prove:

- project management,
- feature intake,
- context source management,
- workflow run state,
- workflow step timeline,
- mock AI output,
- approval gate,
- output history,
- logs placeholder.

## Stack

Use:

- Next.js
- TypeScript
- Tailwind CSS
- shadcn/ui
- Supabase Auth
- Supabase Postgres
- Supabase Realtime
- Supabase Storage
- Supabase Edge Functions if needed

## Architecture rules

- Supabase is the backend for the skeleton phase.
- Next.js handles the admin web.
- Real backend orchestration will be added later only when needed.
- Do not introduce NestJS yet.
- Do not introduce real AI provider calls yet.
- Do not introduce MCP/Jira/Firebase/RAG/Telegram yet.

## Data rules

Workflow state must be persisted.

Important tables:

- projects
- features
- context_sources
- workflow_definitions
- workflow_runs
- workflow_steps
- ai_outputs
- approvals
- ai_call_logs

Mock AI output must still be stored in `ai_outputs`.

Mock logs must still be stored in `ai_call_logs`.

Approval must be stored in `approvals`, not only UI state.

## Workflow rules

The first workflow is:

`feature_to_android_tech_spec`

Steps:

1. collect_context
2. generate_business_summary
3. approval_business_summary
4. generate_product_spec
5. approval_product_spec
6. generate_android_tech_spec
7. approval_android_tech_spec
8. generate_task_breakdown
9. generate_test_plan
10. generate_risk_report

## UI rules

The Workflow Run Detail page should show:

- step timeline,
- current output,
- approval panel,
- context summary,
- logs/metadata.

The Approval Center should show:

- pending approvals,
- output preview,
- approve/reject/request changes actions.

The Output Library should allow reopening generated outputs later.

## Stop conditions

Before coding, stop and ask if:

- schema is unclear,
- workflow state is unclear,
- approval behavior is unclear,
- task would require real AI provider integration,
- task would require NestJS/backend worker,
- task would require MCP/Firebase/Jira/RAG/Telegram integration.

## Definition of done

A task is done only when:

- data persists after refresh,
- important state is not only local React state,
- errors and loading states are handled,
- TypeScript types are clear,
- code follows React best practices,
- future backend replacement is not blocked.