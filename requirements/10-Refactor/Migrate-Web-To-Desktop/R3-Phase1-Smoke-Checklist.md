# R3 Phase 1 Manual Smoke Checklist

Run this checklist against a real local runner and a real Supabase project.

## Bootstrap / Auth

- [ ] Start desktop with runner online and no saved Supabase config
- [ ] Confirm desktop opens pre-auth settings instead of login
- [ ] Save valid Supabase config
- [ ] Confirm login becomes available
- [ ] Sign in successfully
- [ ] Sign out successfully

## Runner Offline

- [ ] Stop local runner
- [ ] Confirm runner status header shows offline
- [ ] Open runner modal and confirm error is visible
- [ ] Confirm settings sections that need runner APIs are disabled

## Projects

- [ ] Create a project with one binding
- [ ] Confirm empty binding create is rejected
- [ ] Add a second unique binding
- [ ] Confirm duplicate binding path is rejected
- [ ] Run Validate Fallback and confirm one usable path is returned
- [ ] Save provider session idle TTL and reload project settings

## Workflows / Steps

- [ ] Create a workflow
- [ ] Edit workflow model override using supported model dropdown
- [ ] Create a step definition
- [ ] Edit step model using supported model dropdown

## Teams

- [ ] Create a team
- [ ] Add a member
- [ ] Remove the member
- [ ] Link the team to a project

## Artifacts

- [ ] Open Generated tab and confirm artifact lists load
- [ ] Open Storage tab
- [ ] Select a project and set artifact storage preference
- [ ] Confirm sync cannot be enabled with missing remote root/folder
- [ ] Save valid storage settings
- [ ] Open Catalog tab and create an artifact definition

## AI Providers

- [ ] Open AI Providers
- [ ] Add a supported model
- [ ] Confirm the model appears in workflow/step model selectors
- [ ] Toggle the model enabled state

## Google Drive / Jira MCP

- [ ] Open Google Drive settings
- [ ] Create or test a Google Drive integration
- [ ] Open Jira MCP settings
- [ ] Create or test a Jira integration

## Runner

- [ ] Open Runner page
- [ ] Confirm base URL, version, workspace, platform, started time, and error fields render
