# 1. What is context?

Purpose:
- collect and normalize task-specific context
- make external project systems available to workflow runs

## Why do we need context?

Many workflow steps need project data that does not live inside FlowPilot itself.

Examples:
- Jira ticket description, comments, status, assignee
- Figma design spec or component details
- Google Drive files and folders
- Firebase project data
- Telegram notifications or messages

If a workflow requires MCP context and that context is not really connected, the workflow must fail early with a clear reason.

---

# 2. What can be a context?

For MVP now, we support 2 types of context:

## 2.1 Text-based context

Put directly in the flow definition or project/workflow inputs.

This type is used to store text-based information.
Users can:
- type text
- upload files
- paste links

## 2.2 MCP context

Belongs to the parent project.

See [SS-01-Project.md](./SS-01-Project.md) for the project relationship.

MCP must be configured once per project and shared across flows in that project.

---

# 3. Types of MCP for MVP

We want to support these MCP context types for MVP:
- Jira: read ticket description, comments, status, assignee, and related metadata
- Figma: read design spec, file structure, components, and comments
- Google Drive: read or write files in a selected folder or shared drive
- Firebase: read or write project data, crash info, analytics, and environment details
- Telegram: send notifications and optionally receive workflow-related messages

---

# 4. Business expectation for MCP connection

This is the most important expectation for MVP:

**Adding an MCP in the UI must create a real usable connection, not just save a row in Supabase.**

When the user clicks `Add MCP`:
1. user selects provider type and enters provider-specific config such as `folderId`
2. FlowPilot saves the project-scoped MCP record
3. FlowPilot asks the local Go-Runner to connect that MCP
4. the Go-Runner opens the user's local browser to the real provider auth page when auth is required
5. user logs in and approves access
6. the Go-Runner receives the callback, token, or equivalent auth proof locally
7. the Go-Runner verifies real access to the configured resource
8. FlowPilot marks the MCP as `connected` only after verification succeeds

If verification fails, the MCP must stay non-connected and surface the failure reason.

---

# 5. Google Drive example

For Google Drive, entering only `folderId` is not enough by itself.

Expected real flow:
1. user enters `folderId`
2. system saves the project MCP entry
3. Go-Runner opens Google OAuth in the user's local browser
4. user logs into Google and approves Drive access
5. Go-Runner receives the auth callback locally
6. Go-Runner stores the credential securely on the local machine
7. Go-Runner calls Google Drive API to verify it can access the configured folder
8. MCP status becomes `connected`

Later, when a workflow or project action asks to import files from Google Drive, the system uses that real connection.

---

# 6. Security expectation

Provider credentials must not be treated as plain business config.

Rules:
- stable project config may be stored in FlowPilot DB, for example `folderId`, `workspaceUrl`, `board`, `projectId`
- provider auth tokens or equivalent secrets must be handled by the Go-Runner
- auth credentials should be stored securely on the local machine or in a dedicated secure secret store
- the project MCP record should expose status and metadata, not raw provider tokens

---

# 7. Runtime expectation

After an MCP is connected, users must get real value from it.

Examples:
- import Jira tickets into business logic context
- import Google Drive docs into project context
- read Figma design data during workflow execution
- send Telegram notifications from workflow steps

The system must not pretend an MCP is available if the runner cannot actually use it.

---

# 8. Context Memory Model

Beyond MCP and text-based context, FlowPilot maintains a three-tier memory model for workflow-generated content.

This is critical because AI providers cannot receive every artifact ever produced. The system must intelligently select the smallest useful context set per step.

## 8.1 Durable Memory

The raw artifact file — the complete output of a workflow step — stored as the source of truth.

- Stored in Supabase Storage (primary) and optionally synced to Google Drive.
- Used for full viewing, audit, export, and re-indexing.
- Never modified after creation. Retries create new versions.

## 8.2 Working Memory

A compact, structured summary derived from the raw artifact after each step completes.

- Stored in the `artifact_memories` table in Supabase Postgres.
- Contains: summary, key decisions, constraints, assumptions, open questions, keywords, source refs, token estimate, and a vector embedding.
- The embedding allows semantic similarity search across all past artifact outputs.
- Fast to query. Supports search, filtering, ranking, and prompt assembly without loading full files.

## 8.3 Prompt Memory

The final runtime context assembled and injected into the AI provider prompt for one specific workflow step.

- Assembled by the Context Resolver immediately before each step executes.
- Controlled by per-step context slots and token budgets.
- Never a dump of all artifacts — always a selective, ranked, budget-constrained set.
- Audited in `workflow_prompt_context_items` so every step records what memory was actually used.

## 8.4 Context Slots

Each workflow step declares its context needs through named context slots. A context slot is a named resolver — a rule that says "go get this kind of context."

Slot resolver types:

| Resolver | Source | When to use |
|---|---|---|
| `project.brief` | Project record | Always — gives AI the project summary |
| `run.input` | Workflow run intake form | Always — the user's original task description |
| `step.previous.brief` | Previous step's artifact memory summary | Always — gives continuity |
| `step.{n}.brief` | Specific step N artifact memory summary | When a non-adjacent step output is needed |
| `artifact.required` | Input artifact definitions declared on the step | Deterministic — step needs this specific artifact |
| `semantic.search` | pgvector search over `artifact_memories` | Dynamic — find the most relevant past context |
| `drive.file` | Google Drive or local file path | When a specific document must be included |
| `mcp.context` | Live MCP data (Jira ticket, Figma spec, etc.) | When step requires real-time external data |
| `static` | Hardcoded text in step definition | Shared prompt fragments, step rules |

Slots are configured per step definition as JSONB, not hardcoded in application logic. This means context requirements are flexible and editable without code changes.

## 8.5 HyperRAG Query Construction

For `semantic.search` slots, the system must construct a short, focused query (10–50 tokens) before calling pgvector. Embedding a long prompt produces a blurry result. The query is constructed from structured metadata:

1. **Template-based (default):** use a `query_template` field in the slot config, e.g. `"{{feature_name}} {{step.description}}"`.
2. **AI-reformulated (optional):** a cheap micro-call (Gemini Flash / GPT-mini) generates a focused search query from the step context before the main AI call.

The resolved short query is then embedded and used to search `artifact_memories` for the top-K relevant working memory records.

In short: we have a prompt -> use 1 prelight AI (small model) to summary it, use for search RAG before include to main prompt