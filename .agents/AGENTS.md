# FlowPilot Agent Instructions

Before implementing or reviewing code in this repository, read and apply:

1. `.agents/skills/frontend/react-best-practices`
2. `.agents/skills/project/flowpilot-admin-rules.md`
3. `docs/short-term-admin-skeleton-plan.md` when available

## Current project direction

FlowPilot is an AI-assisted engineering workflow platform.

Current phase:

Build Plan 3 first, but only as the Admin MVP Skeleton.

## Current confirmed stack

- Next.js
- TypeScript
- Tailwind CSS
- shadcn/ui
- Supabase Auth
- Supabase Postgres
- Supabase Realtime
- Supabase Storage
- Supabase Edge Functions if needed

## Important boundary

Do not build real AI orchestration yet.

For this phase, build:

Project
→ Feature Intake
→ Context Sources
→ Workflow Run
→ Workflow Step Timeline
→ Mock Output
→ Approval Gate
→ Output History
→ Logs Placeholder

## Do

- Keep workflow state persisted in Supabase.
- Keep approval state persisted in Supabase.
- Store mock outputs in the real `ai_outputs` table.
- Keep workflow logic isolated in service files.
- Use typed models and validation.
- Prefer clear admin UX over complex visual design.
- Make future backend replacement easy.

## Do not

- Do not call real AI providers yet.
- Do not add NestJS yet.
- Do not put AI provider keys in frontend.
- Do not add Jira/Firebase/Context7/RAG/Telegram integrations yet.
- Do not store important workflow state only in React state.
- Do not hardcode workflow logic inside UI components.