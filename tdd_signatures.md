# TDD Signatures - CP-02 Project & Team Management

Date: 2026-05-18

Rules:

- signatures only
- no implementation code
- focus on project-management risk

## 1. Domain Models

### File

`apps/admin-web/src/domain/model/entity/project.test.ts`

#### `describe("project entity extension")`

- `it("includes directoryPath, ownerId, status, and artifactStoragePreference fields")`
- `it("preserves the existing project identity and timestamps")`

### File

`apps/admin-web/src/domain/model/entity/team.test.ts`

#### `describe("team domain entities")`

- `it("exports Team with timestamps")`
- `it("exports TeamMember with role and level label unions")`
- `it("allows nullable email and jiraAccountId fields")`

### File

`apps/admin-web/src/domain/model/entity/integration.test.ts`

#### `describe("integration entity")`

- `it("models project-scoped integration records with status and sync timestamps")`

## 2. Gateway Contracts

### File

`apps/admin-web/src/domain/gateway/project-gateway.test.ts`

#### `describe("ProjectGateway")`

- `it("adds updateProject and deleteProject support for project management screens")`
- `it("can list teams linked to a project when the detail page needs them")`

### File

`apps/admin-web/src/domain/gateway/team-gateway.test.ts`

#### `describe("TeamGateway")`

- `it("lists teams and fetches a team by id")`
- `it("creates, updates, and deletes teams")`
- `it("lists, adds, updates, and removes team members")`
- `it("links and unlinks a team to a project")`
- `it("lists teams by project")`

## 3. Repository And Mapping

### File

`apps/admin-web/src/data/repository/supabase/supabase-gateway-bundle.test.ts`

#### `describe("SupabaseGatewayBundle project management mapping")`

- `it("maps projects with the new CP-02 columns")`
- `it("maps teams, team members, and project-team join rows")`
- `it("persists and loads team records without losing role or level labels")`
- `it("links teams to projects using the join table")`

## 4. Route Surface

### File

`apps/admin-web/src/routes/projects/$projectId.test.tsx`

#### `describe("project detail management route")`

- `it("renders the tabbed project management shell")`
- `it("keeps the header stable while tabs change")`

### File

`apps/admin-web/src/routes/projects/$projectId/members.test.tsx`

#### `describe("project members route")`

- `it("renders the members table and add-member entry point")`
- `it("shows workload summary state from the project team data")`

### File

`apps/admin-web/src/routes/projects/$projectId/settings.test.tsx`

#### `describe("project settings route")`

- `it("renders artifact storage preference controls")`
- `it("renders MCP context status placeholders without installation flows")`

## 5. Build Safety

### File

`apps/admin-web/src/app/(protected)/projects/[projectId]/page.test.tsx`

#### `describe("project detail page")`

- `it("no longer assumes the old feature-and-workflow-only layout")`

### File

`apps/admin-web/src/features/projects/queries.test.ts`

#### `describe("project queries")`

- `it("exposes keys for project detail and project team data")`

### File

`apps/admin-web/src/features/members/queries.test.ts`

#### `describe("member queries")`

- `it("exposes keys for members-by-team data")`
