# CA-025: Workflow Run Tabs, Delete, and Prompt Fixes

## Scope

This audit covers the workflow run detail tab layout, delete actions on the run list page, and the prompt handling fixes that followed.

## Completed

- Added tabbed display for response, prompt, and artifact views in the workflow run detail page.
- Added delete actions for workflow runs so users can remove selected runs or clear the list.
- Fixed the prompt display so empty sections are removed instead of showing placeholder `None` content.
- Adjusted the run detail badge and step metadata so repeated attempts and artifact presence are easier to read.

## Verification

- Verified the run detail page shows the expected tab set and delete controls.
- Confirmed the prompt tab no longer renders empty sections.


# ---8<--- flowpilot:change-ledger
feature_key: workflow-runtime
source_doc_id: CA-025
change_type: feature
summary: Workflow Run Tabs, Delete, and Prompt Fixes
# --->8---
