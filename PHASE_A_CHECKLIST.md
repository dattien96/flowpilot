# Google Drive MCP Phase A - Implementation Checklist

## ✅ Completed (Backend)

### Core Implementation
- [x] Provider config data structures (Section 11.6)
- [x] Codex config.toml writer
- [x] Gemini settings.json writer  
- [x] Claude .claude.json writer
- [x] Config preservation logic
- [x] Idempotency checks
- [x] Account-home isolation
- [x] Read-only tool allowlists

### Prompt System
- [x] InjectRequiredMcpInstructions() function
- [x] MCP usage section builder
- [x] Failure code definitions
- [x] Read-only vs write differentiation

### Preflight
- [x] PreflightGoogleDriveMcp() function
- [x] Google Drive MCP status checks
- [x] Provider config validation
- [x] Error message mapping

### API
- [x] POST /mcp-provider-config/ensure endpoint
- [x] Request/response types
- [x] Status reporting
- [x] Change detection

### Testing
- [x] Codex config tests (creation, updates, idempotency)
- [x] Gemini config tests
- [x] Claude config tests
- [x] Prompt injection tests (4 scenarios)
- [x] Preflight check tests (2 scenarios)
- [x] **All 10 tests passing**

### Documentation
- [x] Implementation summary
- [x] Quick reference guide
- [x] UI integration guide
- [x] This checklist

---

## ⏳ Pending (UI Integration)

### API Extensions
- [ ] Extend GET /google-drive-config with providerConfigs array
- [ ] Implement resolveGoogleDriveMcpProviderStatuses()
- [ ] Add provider account home path discovery
- [ ] Return provider config status in response

### UI Components
- [ ] Provider status panel in google-drive-setup page
- [ ] Status badges (configured, not_started, failed)
- [ ] "Configure AI Providers" button
- [ ] Loading states
- [ ] Error displays

### Repository Layer
- [ ] Add ensureGoogleDriveMcpProviderConfig() method
- [ ] Add TypeScript types
- [ ] Handle API errors

### Testing
- [ ] Manual UI testing (happy path)
- [ ] Error state testing
- [ ] Idempotency verification
- [ ] Cross-browser testing

---

## ⏳ Pending (Phase B - Workflow Runtime)

### Integration Points
- [ ] Hook PreflightGoogleDriveMcp() before workflow execution
- [ ] Hook InjectRequiredMcpInstructions() in prompt builder
- [ ] Pass requiredMcps through execution pipeline
- [ ] Parse provider output for failure codes

### Workflow Model
- [ ] Ensure requiredMcps field exists on steps
- [ ] Add allowWrite permission field
- [ ] Store provider/account selection

### Error Handling
- [ ] Map failure codes to user actions
- [ ] Show actionable error messages
- [ ] Provide retry mechanisms

### Testing
- [ ] End-to-end workflow test with MCP
- [ ] Failure code handling tests
- [ ] Preflight block tests

---

## ⏳ Pending (Phase C - Write Operations)

### Write Controls
- [ ] Implement per-step write permission
- [ ] Filter write tools by permission
- [ ] Add write approval model

### Audit
- [ ] Record write operations
- [ ] Capture created/modified file IDs
- [ ] Store audit artifacts

### Tool Management
- [ ] Enable write tools in provider configs
- [ ] Update prompt augmentation for write mode
- [ ] Block destructive tools

---

## 📊 Progress Summary

**Phase A Backend**: 100% Complete ✅  
**Phase A UI**: 0% Complete ⏳  
**Phase B**: 0% Complete ⏳  
**Phase C**: 0% Complete ⏳

**Test Pass Rate**: 10/10 (100%) ✅  
**Code Quality**: All files compile ✅  
**Documentation**: Complete ✅

---

## 🎯 Next Action Items

### Priority 1 (UI Integration - Week 1)
1. Extend GET /google-drive-config API response
2. Implement provider status panel UI
3. Add "Configure AI Providers" button
4. Test happy path manually

### Priority 2 (Phase B - Week 2-3)
1. Map workflow execution entry points
2. Add preflight hook
3. Add prompt augmentation hook
4. Implement failure code parsing

### Priority 3 (Phase C - Week 4)
1. Design write approval model
2. Implement write tool filters
3. Create audit system

---

## 📝 Notes

- Backend is production-ready for UI integration
- All core functionality tested and working
- Provider config format matches official schemas
- Read-only phase prevents accidental modifications
- Account isolation ensures security

---

## 📚 Resources

- [Implementation Summary](./requirements/07-Coding-Plan/priority/CP-05-03-Implementation-Phase-A.md)
- [Quick Reference](./requirements/07-Coding-Plan/priority/CP-05-03-Quick-Reference.md)
- [UI Integration Guide](./requirements/07-Coding-Plan/priority/CP-05-03-UI-Integration-Guide.md)
- [Full Plan (CP-05-03)](./requirements/07-Coding-Plan/priority/CP-05-03-Driver-Mcp.md)

---

Last Updated: 2026-06-07  
Phase A Backend: ✅ Complete
