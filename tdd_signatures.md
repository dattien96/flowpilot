# CP-05 TDD Signatures

## 1. Route Behavior

### Target File

- `apps/admin-web/src/routes/_authenticated/projects/$projectId/settings.test.tsx`

### Signatures

```ts
describe("Project settings MCP section", () => {
  it("renders project integrations with label, status, last synced, and last error");
  it("opens the add integration drawer and creates a new integration");
  it("opens the edit flow for an existing integration and saves the patch");
  it("deletes an integration from the project settings surface");
  it("retries a failed integration through the local runner gateway");
  it("shows the awaiting OAuth state with a waiting label");
});
```

- `renders project integrations with label, status, last synced, and last error`
  - Input:
    - loader returns one `connected`, one `awaiting_oauth`, and one `failed` integration
  - Expected:
    - each row renders provider type and label
    - `lastSyncedAt` is visible for connected rows
    - `lastError` is visible for failed rows

- `opens the add integration drawer and creates a new integration`
  - Input:
    - user clicks `Add MCP`
    - selects a provider
    - enters label/config summary
  - Expected:
    - route calls `integrationGateway.createIntegration`
    - route then calls `localRunnerGateway.triggerIntegrationConnection` with `action: "test"`
    - route invalidation is requested after success

- `opens the edit flow for an existing integration and saves the patch`
  - Input:
    - existing integration row selected for edit
    - label/config changed
  - Expected:
    - route calls `integrationGateway.updateIntegration`
    - route then calls `localRunnerGateway.triggerIntegrationConnection` with `action: "test"`

- `deletes an integration from the project settings surface`
  - Input:
    - user confirms delete for a selected integration
  - Expected:
    - route calls `integrationGateway.deleteIntegration`
    - route invalidation is requested

- `retries a failed integration through the local runner gateway`
  - Input:
    - row status is `failed`
    - user clicks retry/reconnect
  - Expected:
    - route calls `localRunnerGateway.triggerIntegrationConnection`
    - request contains the project id, integration id, and `action: "retry"`

- `shows the awaiting OAuth state with a waiting label`
  - Input:
    - integration row status is `awaiting_oauth`
  - Expected:
    - route renders a waiting indicator
    - row does not show the row as connected or failed

## 2. Demo Repository Logic

### Target File

- `apps/admin-web/src/data/repository/demo/demo-gateway-bundle.test.ts`

### Signatures

```ts
describe("DemoGatewayBundle integrations", () => {
  it("lists only integrations belonging to the requested project");
  it("creates a pending integration with label and config");
  it("updates integration label, status, and error fields");
  it("deletes an integration by id");
});
```

- `lists only integrations belonging to the requested project`
  - Input:
    - seeded demo integrations across multiple projects
  - Expected:
    - returned list contains only rows for the requested project id

- `creates a pending integration with label and config`
  - Input:
    - create payload with `projectId`, `type`, `label`, `configEncrypted`
  - Expected:
    - created entity is added to the store
    - `status` defaults to `pending`
    - `lastError` is `null`

- `updates integration label, status, and error fields`
  - Input:
    - patch changes `label`, `status`, and `lastError`
  - Expected:
    - stored entity reflects the patch
    - `updatedAt` changes

- `deletes an integration by id`
  - Input:
    - existing integration id
  - Expected:
    - entity is removed from the demo store

## 3. Supabase Repository Logic

### Target File

- `apps/admin-web/src/data/repository/supabase/supabase-gateway-bundle.test.ts`

### Signatures

```ts
describe("SupabaseGatewayBundle integrations", () => {
  it("maps integration rows including label, awaiting_oauth, and last_error");
  it("inserts the expected integration payload for create");
  it("updates mutable fields and stamps updated_at");
  it("deletes the integration row by id");
});
```

- `maps integration rows including label, awaiting_oauth, and last_error`
  - Input:
    - Supabase row with `label`, `status: "awaiting_oauth"`, `last_error`, `last_synced_at`
  - Expected:
    - mapped domain entity exposes `label`, `status`, `lastError`, `lastSyncedAt`

- `inserts the expected integration payload for create`
  - Input:
    - create payload with project id, type, label, config envelope, and optional status
  - Expected:
    - repository sends `project_id`, `type`, `label`, `config_encrypted`, `status`
    - inserted row maps back to a domain `Integration`

- `updates mutable fields and stamps updated_at`
  - Input:
    - patch updates `label`, `configEncrypted`, `status`, `lastError`
  - Expected:
    - repository issues an update with `updated_at`
    - returned row maps back to the patched entity

- `deletes the integration row by id`
  - Input:
    - integration id
  - Expected:
    - repository targets the `integrations` table row with that id

## 4. Local Runner HTTP Contract

### Target File

- `apps/admin-web/src/data/repository/local-runner/http-local-runner-gateway.test.ts`

### Signatures

```ts
describe("HttpLocalRunnerGateway integration connection", () => {
  it("posts a test request to the integration connection endpoint");
  it("posts a retry request to the integration connection endpoint");
  it("throws a descriptive error when the runner rejects the request");
});
```

- `posts a test request to the integration connection endpoint`
  - Input:
    - request with `projectId`, `integrationId`, `action: "test"`
  - Expected:
    - gateway POSTs to `/integrations/{integrationId}/connection`
    - JSON body contains `projectId` and `action`

- `posts a retry request to the integration connection endpoint`
  - Input:
    - request with `projectId`, `integrationId`, `action: "retry"`
  - Expected:
    - gateway POSTs to the same endpoint
    - response is parsed into the local-runner connection result type

- `throws a descriptive error when the runner rejects the request`
  - Input:
    - non-OK HTTP response from the runner
  - Expected:
    - thrown error includes endpoint purpose and response status
