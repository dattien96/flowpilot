# CP-05-03 UI Integration Guide

## Overview

This guide helps frontend developers integrate the Google Drive MCP provider configuration system into the FlowPilot admin web UI.

**Backend Status**: ✅ Complete  
**API Endpoint**: ✅ Available  
**UI Status**: ⏳ Pending

---

## UI Components Needed

### 1. Provider Config Status Panel

**Location**: `/settings/google-drive-setup` page

**Purpose**: Show which AI providers are configured for Google Drive MCP

**Data Source**: Extend existing `GET /google-drive-config` response

#### Proposed Extended Response Type

```typescript
// apps/admin-web/src/domain/model/entity/local-runner.ts

interface GoogleDriveMcpProviderConfigStatus {
  providerKey: string;         // "codex" | "gemini" | "claude"
  accountHomePath: string;     // "/Users/example/.codexHome1"
  configPath: string;          // "/Users/example/.codexHome1/config.toml"
  status: string;              // "not_started" | "configured" | "failed"
  lastCheckedAt?: string;      // ISO timestamp
  lastError?: string;          // Error message if failed
}

interface GoogleDriveWorkspaceConfigResponse {
  artifactSync: GoogleDriveArtifactSyncStatus;
  mcp: GoogleDriveMcpStatus;
  providerConfigs?: GoogleDriveMcpProviderConfigStatus[]; // NEW
  runnerReachable: boolean;
  lastError?: string;
  updatedAt?: string;
}
```

#### UI Mockup

```tsx
// Section in google-drive-setup.tsx after MCP block

<Card>
  <CardHeader>
    <CardTitle>AI Provider Configuration</CardTitle>
    <CardDescription>
      Configure AI providers to use Google Drive MCP tools during workflow execution.
    </CardDescription>
  </CardHeader>
  <CardContent>
    <div className="space-y-4">
      {/* Codex Status */}
      <div className="flex items-center justify-between">
        <div>
          <p className="font-medium">Codex</p>
          <p className="text-sm text-muted-foreground">
            {codexStatus === 'configured' 
              ? `Configured at ${codexConfigPath}`
              : 'Not configured'}
          </p>
        </div>
        <Badge variant={codexStatus === 'configured' ? 'success' : 'secondary'}>
          {codexStatus}
        </Badge>
      </div>

      {/* Gemini Status */}
      <div className="flex items-center justify-between">
        <div>
          <p className="font-medium">Gemini</p>
          <p className="text-sm text-muted-foreground">
            {geminiStatus === 'configured' 
              ? `Configured at ${geminiConfigPath}`
              : 'Not configured'}
          </p>
        </div>
        <Badge variant={geminiStatus === 'configured' ? 'success' : 'secondary'}>
          {geminiStatus}
        </Badge>
      </div>

      {/* Claude Status */}
      <div className="flex items-center justify-between">
        <div>
          <p className="font-medium">Claude</p>
          <p className="text-sm text-muted-foreground">
            {claudeStatus === 'configured' 
              ? `Configured at ${claudeConfigPath}`
              : 'Not configured'}
          </p>
        </div>
        <Badge variant={claudeStatus === 'configured' ? 'success' : 'secondary'}>
          {claudeStatus}
        </Badge>
      </div>
    </div>
  </CardContent>
  <CardFooter>
    <Button 
      onClick={handleConfigureProviders}
      disabled={mcpStatus !== 'configured'}
    >
      Configure AI Providers
    </Button>
  </CardFooter>
</Card>
```

---

### 2. Configure Providers Action

**Trigger**: User clicks "Configure AI Providers" button

**Prerequisites**:
- Google Drive MCP status must be `configured`
- At least one provider account must exist

**Flow**:

```typescript
async function handleConfigureProviders() {
  setLoading(true);
  
  try {
    // Get available provider accounts
    const providers = await getProviderAccounts();
    
    // Configure each provider that has an account
    const results = await Promise.all(
      providers.map(async (provider) => {
        const response = await fetch(
          '/google-drive-config/mcp-provider-config/ensure',
          {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
              providerKey: provider.key,        // "codex", "gemini", "claude"
              accountHomePath: provider.homePath,
              scope: 'account',
              mode: 'read_only'
            })
          }
        );
        
        return await response.json();
      })
    );
    
    // Refresh status
    await refreshGoogleDriveConfig();
    
    // Show results
    const configured = results.filter(r => r.status === 'configured').length;
    const failed = results.filter(r => r.status === 'failed').length;
    
    if (failed > 0) {
      showToast({
        title: 'Partially Configured',
        description: `${configured} providers configured, ${failed} failed`,
        variant: 'warning'
      });
    } else {
      showToast({
        title: 'Providers Configured',
        description: `All ${configured} providers are ready for Google Drive MCP`,
        variant: 'success'
      });
    }
  } catch (error) {
    showToast({
      title: 'Configuration Failed',
      description: error.message,
      variant: 'error'
    });
  } finally {
    setLoading(false);
  }
}
```

---

### 3. Provider Account Selection

**Challenge**: UI needs to know which provider accounts exist and their home paths

**Options**:

#### Option A: Extend Provider Inventory API

```typescript
// Extend existing GET /providers response

interface ProviderAccount {
  id: string;
  providerKey: string;
  displayName: string;
  homePath: string;         // NEW
  authStatus: string;
  models: ProviderModel[];
}

interface ProviderInventory {
  providers: Provider[];
  accounts: ProviderAccount[];  // NEW
}
```

#### Option B: Provider-Specific Account API

```typescript
// New endpoint: GET /providers/:providerKey/accounts

interface ProviderAccountsResponse {
  providerKey: string;
  accounts: {
    id: string;
    displayName: string;
    homePath: string;
    authStatus: string;
  }[];
}
```

**Recommendation**: Option A (extend existing inventory API)

---

### 4. Error State Display

**Show clear error messages** when provider config fails:

```tsx
{providerConfig?.lastError && (
  <Alert variant="destructive">
    <AlertCircle className="h-4 w-4" />
    <AlertTitle>Configuration Error</AlertTitle>
    <AlertDescription>
      {providerConfig.lastError}
      {providerConfig.lastError.includes('credential') && (
        <p className="mt-2">
          <Link href="/settings/google-drive-setup#upload">
            Upload Desktop OAuth JSON
          </Link>
        </p>
      )}
      {providerConfig.lastError.includes('auth') && (
        <p className="mt-2">
          <Button variant="link" onClick={handleStartAuth}>
            Start Google Drive MCP Auth
          </Button>
        </p>
      )}
    </AlertDescription>
  </Alert>
)}
```

---

### 5. Prerequisite Blocking

**Block "Configure AI Providers" until prerequisites are met**:

```typescript
const canConfigureProviders = 
  mcpStatus === 'configured' && 
  hasProviderAccounts &&
  !isConfiguring;

const getBlockReason = () => {
  if (mcpStatus !== 'configured') {
    return 'Complete Google Drive MCP setup first';
  }
  if (!hasProviderAccounts) {
    return 'No AI provider accounts available';
  }
  return null;
};
```

```tsx
<Button
  onClick={handleConfigureProviders}
  disabled={!canConfigureProviders}
  title={getBlockReason() || 'Configure providers for Google Drive MCP'}
>
  Configure AI Providers
</Button>
```

---

## API Integration

### Repository Layer

**File**: `apps/admin-web/src/data/repository/local-runner/http-local-runner-gateway.ts`

```typescript
export class HttpLocalRunnerGateway {
  // ... existing methods ...

  async ensureGoogleDriveMcpProviderConfig(
    request: GoogleDriveMcpProviderConfigRequest
  ): Promise<GoogleDriveMcpProviderConfigResponse> {
    const response = await fetch(
      `${this.baseURL}/google-drive-config/mcp-provider-config/ensure`,
      {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(request)
      }
    );

    if (!response.ok) {
      const error = await response.json();
      throw new Error(error.error || 'Failed to configure provider');
    }

    return await response.json();
  }
}
```

### Type Definitions

**File**: `apps/admin-web/src/domain/model/entity/local-runner.ts`

```typescript
export interface GoogleDriveMcpProviderConfigRequest {
  providerKey: string;      // "codex" | "gemini" | "claude"
  accountHomePath: string;  // Path to provider account home
  scope: string;            // "account" | "workspace"
  mode: string;             // "read_only" | "read_write"
}

export interface GoogleDriveMcpProviderConfigResponse {
  providerKey: string;
  serverName: string;       // Always "google-drive"
  status: string;           // "configured" | "failed"
  changed: boolean;
  configPath: string;
  lastError?: string;
}

export interface GoogleDriveMcpProviderConfigStatus {
  providerKey: string;
  accountHomePath: string;
  configPath: string;
  status: string;
  lastCheckedAt?: string;
  lastError?: string;
}
```

---

## User Flow

### Happy Path

1. User completes Google Cloud setup (CP-27)
2. User uploads Desktop OAuth JSON (CP-28 Step 5)
3. User clicks "Install" in MCP block
4. User clicks "Start Auth" and completes sign-in
5. User clicks "Refresh MCP status" → status becomes `configured`
6. **User clicks "Configure AI Providers"** ← NEW
7. UI fetches provider accounts
8. UI calls `/mcp-provider-config/ensure` for each provider
9. UI shows success: "All 3 providers configured"
10. Provider status badges show "configured"

### Error Paths

**Google Drive MCP not ready**:
```
User clicks "Configure AI Providers"
→ Button is disabled
→ Tooltip: "Complete Google Drive MCP setup first"
```

**No provider accounts**:
```
User clicks "Configure AI Providers"
→ Button is disabled
→ Tooltip: "No AI provider accounts available"
```

**Config fails for one provider**:
```
User clicks "Configure AI Providers"
→ API returns mixed results
→ Toast: "2 providers configured, 1 failed"
→ Failed provider shows error badge with message
→ User can retry or check provider setup
```

---

## Backend Changes Needed

### 1. Extend GET /google-drive-config Response

**Current**:
```json
{
  "artifactSync": {...},
  "mcp": {...},
  "runnerReachable": true
}
```

**Needed**:
```json
{
  "artifactSync": {...},
  "mcp": {...},
  "providerConfigs": [
    {
      "providerKey": "codex",
      "accountHomePath": "/Users/example/.codexHome1",
      "configPath": "/Users/example/.codexHome1/config.toml",
      "status": "configured",
      "lastCheckedAt": "2026-06-07T23:00:00Z"
    }
  ],
  "runnerReachable": true
}
```

**Implementation**:

```go
// apps/local-runner/internal/runner/google_drive_config.go

func (r *Runner) LoadGoogleDriveWorkspaceConfig() (GoogleDriveWorkspaceConfigResponse, error) {
    status := r.resolveGoogleDriveWorkspaceStatus()
    
    // NEW: Add provider config statuses
    providerConfigs, _ := r.resolveGoogleDriveMcpProviderStatuses()
    status.ProviderConfigs = providerConfigs
    
    return status, nil
}

func (r *Runner) resolveGoogleDriveMcpProviderStatuses() ([]GoogleDriveMcpProviderConfigStatus, error) {
    // TODO: Check each provider's config file
    // TODO: Return status for codex, gemini, claude
    return []GoogleDriveMcpProviderConfigStatus{}, nil
}
```

### 2. Provider Account Home Path Discovery

**Challenge**: How does UI know provider account home paths?

**Solution**: Extend provider inventory to include account home paths

```go
// apps/local-runner/internal/runner/providers.go

type ProviderAccount struct {
    ID              string `json:"id"`
    ProviderKey     string `json:"providerKey"`
    DisplayName     string `json:"displayName"`
    HomePath        string `json:"homePath"`        // NEW
    AuthStatus      string `json:"authStatus"`
}

func (r *Runner) ListProviderAccounts() ([]ProviderAccount, error) {
    // Scan for Codex accounts in ~/.codexHome*
    // Scan for Gemini accounts in ~/.geminiHome*
    // Scan for Claude accounts in ~/.claudeHome*
    // Return list with home paths
}
```

---

## Testing Checklist

### Manual UI Testing

1. **Prerequisites**:
   - [ ] Google Drive MCP status is `configured`
   - [ ] At least one provider account exists
   - [ ] Provider account home path is known

2. **Happy Path**:
   - [ ] Navigate to `/settings/google-drive-setup`
   - [ ] See "AI Provider Configuration" panel
   - [ ] See provider status badges (all "not_started" initially)
   - [ ] Click "Configure AI Providers"
   - [ ] See loading indicator
   - [ ] See success toast
   - [ ] See provider badges change to "configured"
   - [ ] Refresh page - status persists

3. **Error Path - MCP Not Ready**:
   - [ ] Set MCP status to `needs_auth` (remove token file)
   - [ ] Navigate to setup page
   - [ ] "Configure AI Providers" button is disabled
   - [ ] Hover shows tooltip explaining why

4. **Error Path - Provider Config Fails**:
   - [ ] Create invalid provider config file
   - [ ] Click "Configure AI Providers"
   - [ ] See partial success toast
   - [ ] Failed provider shows error badge
   - [ ] Error message is actionable

5. **Idempotency**:
   - [ ] Click "Configure AI Providers" twice
   - [ ] Second click shows "no changes" or instant success
   - [ ] Provider configs are not rewritten unnecessarily

---

## Future Enhancements (Phase B)

### Workflow Builder Integration

**Show MCP status when selecting Google Drive as required MCP**:

```tsx
// In workflow step editor

<Select value={requiredMcps} onValueChange={setRequiredMcps}>
  <SelectItem value="google_drive">
    Google Drive
    {!googleDriveMcpConfigured && (
      <Badge variant="warning" className="ml-2">
        Not Configured
      </Badge>
    )}
  </SelectItem>
</Select>

{requiredMcps.includes('google_drive') && !googleDriveMcpConfigured && (
  <Alert>
    <AlertCircle />
    <AlertTitle>Configuration Required</AlertTitle>
    <AlertDescription>
      Google Drive MCP is not configured for the selected provider.
      <Link href="/settings/google-drive-setup">
        Configure now
      </Link>
    </AlertDescription>
  </Alert>
)}
```

### Workflow Execution Feedback

**Show MCP-specific errors during workflow runs**:

```tsx
{workflowError.code === 'MCP_UNAVAILABLE' && (
  <Alert variant="destructive">
    <AlertTitle>Google Drive MCP Unavailable</AlertTitle>
    <AlertDescription>
      The AI provider could not access Google Drive MCP tools.
      <Link href="/settings/google-drive-setup">
        Check configuration
      </Link>
    </AlertDescription>
  </Alert>
)}

{workflowError.code === 'MCP_AUTH_REQUIRED' && (
  <Alert variant="destructive">
    <AlertTitle>Google Drive Auth Required</AlertTitle>
    <AlertDescription>
      Google Drive MCP authentication has expired.
      <Button onClick={handleStartAuth}>
        Re-authenticate
      </Button>
    </AlertDescription>
  </Alert>
)}
```

---

## Timeline Estimate

| Task | Effort | Dependencies |
|------|--------|--------------|
| Extend provider inventory API | 2h | None |
| Add provider config status to GET endpoint | 3h | Provider inventory |
| Create UI provider status panel | 4h | API extensions |
| Implement configure action | 3h | Status panel |
| Add error state handling | 2h | Configure action |
| Manual testing | 3h | All above |
| **Total** | **17h** | |

---

## Questions to Resolve

1. **Provider Account Discovery**:
   - How should runner discover provider account home paths?
   - Should UI allow manual path entry as fallback?

2. **Multi-Account Support**:
   - What if a user has multiple Codex accounts?
   - Should UI let user choose which account to configure?

3. **Workspace vs Account Scope**:
   - Should UI expose scope selection?
   - Or always use "account" scope for Phase A?

4. **Config Refresh**:
   - Should provider config status auto-refresh?
   - Or only refresh on page load / explicit refresh?

---

## Success Criteria

UI integration is complete when:

1. ✅ User can see provider config status
2. ✅ User can configure all providers with one click
3. ✅ Provider status persists across page refreshes
4. ✅ Error messages are clear and actionable
5. ✅ Prerequisites are enforced (MCP configured first)
6. ✅ UI matches FlowPilot design system

---

## References

- [Backend Implementation](./CP-05-03-Implementation-Phase-A.md)
- [Quick Reference](./CP-05-03-Quick-Reference.md)
- [Full Plan](./CP-05-03-Driver-Mcp.md)
- Current UI: `apps/admin-web/src/routes/_authenticated/settings/google-drive-setup.tsx`
