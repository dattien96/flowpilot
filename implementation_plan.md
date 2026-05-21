# Implementation Plan

## Scope

Implement the first production-facing slice of CP-09 in `apps/admin-web` while keeping the existing workflow-engine screens backward-compatible:

1. Replace the placeholder `Prompt Templates` settings page with a working list/create surface.
2. Replace the placeholder `AI Runs` page with a working monitoring surface.
3. Add a dedicated AI orchestration domain/gateway layer instead of expanding the already high-risk `WorkflowEngineGateway` interface.
4. Wire both demo and Supabase implementations so the feature works in local/demo mode and against real tables when available.
5. Add targeted tests for the new route content and gateway behavior.

## Constraints

- Do not refactor the shared workflow-engine interfaces in this pass unless required for compilation.
- Keep the implementation additive and backward-compatible because `WorkflowRunStep`, `ArtifactRun`, and `WorkflowEngineGateway` all have `CRITICAL` upstream impact.
- Align data structures with the revised CP-09 model: `ai_prompt_templates`, `ai_runs`, workflow lineage fields, and prompt template scoping/versioning.

## Design

### 1. New domain lane

Create a new AI orchestration domain model and gateway:

- `AiPromptTemplate`
- `AiRun`
- `AiRunSummary`
- `AiOrchestrationGateway`

This isolates CP-09 from the older workflow and output abstractions.

### 2. Repository implementations

- Demo implementation with seeded prompt templates and AI run history
- Supabase implementation that reads/writes:
  - `ai_prompt_templates`
  - `ai_runs`

The Supabase summary can be computed client-side from fetched runs for the initial slice.

### 3. UI routes

#### Prompt Templates

- Loader fetches templates and projects
- Page shows:
  - template summary cards
  - list/table of templates
  - create form for new template
- Save action writes through `AiOrchestrationGateway` and refreshes local state

#### AI Runs

- Loader fetches AI runs and summary
- Page shows:
  - summary cards
  - filter controls
  - run table with lineage references and token/cost details

### 4. Tests

Add focused tests for:

- `PromptTemplatesContent`
- `AiRunsContent`
- demo gateway list/save behavior if needed for confidence

## Delivery order

1. Add domain/gateway/usecase files
2. Add demo and Supabase repository implementations
3. Expose `aiOrchestrationGateway` from gateway factories
4. Replace placeholder route pages
5. Add/adjust tests
6. Run build/tests
7. Self-review and produce `walkthrough.md`
