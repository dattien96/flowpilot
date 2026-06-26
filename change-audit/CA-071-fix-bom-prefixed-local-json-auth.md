# CA-071: Fix BOM-Prefixed Local JSON Auth

## Scope

- Corrected a local-runner regression where BOM-prefixed local JSON files could fail with `invalid character 'ï' looking for beginning of value`.
- Scoped the change to runner-side local file reads used by provider account state, provider auth/config validation, shared runner JSON loading, and Claude usage metadata parsing.
- Added focused provider-account regression coverage for a stored default Claude account whose `.claude.json` begins with a UTF-8 BOM.

## Completed

- Added `stripUTF8BOM` in the runner package and used it before `json.Unmarshal` on affected local JSON file bytes.
- Updated provider account state loading and JSON config validation so BOM-prefixed files do not break account sync.
- Updated Claude usage metadata parsing so BOM-prefixed `.claude.json` does not bypass or break metadata reads.
- Preserved existing behavior for genuinely malformed JSON after BOM normalization.

## Verification

- `go test ./internal/runner -run "TestListProviderAccountsKeepsStoredDefaultClaudeAccountConnected|TestListProviderAccountsKeepsStoredDefaultClaudeAccountConnectedWithBOMAuth"` - PASS
- `go test ./internal/runner -run "TestListProviderAccounts|TestDeleteProviderAccount"` - PASS
- `go test ./internal/runner` - FAIL from existing Google Drive MCP setup and Windows `sh` environment failures unrelated to this BOM fix.

## Residual Notes

- GitNexus tools were not exposed in this session, so the required symbol impact check was documented as unavailable and replaced with local code inspection.
- The similarly named CLI-package `readJSONFile` remains separate; no caller evidence tied it to this regression.

# ---8<--- flowpilot:change-ledger
feature_key: ai-providers
source_doc_id: CA-071
change_type: fix
summary: Fix BOM-Prefixed Local JSON Auth
# --->8---
