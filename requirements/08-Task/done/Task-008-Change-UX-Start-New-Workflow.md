# TASK-008: Change UX Start New Workflow

## Goal
Improve the user experience when launching a new workflow run by avoiding blocking or waiting for the entire workflow execution to complete, and immediately navigating the user to the live workflow run detail page.

## Problem
Previously, when a user started a new workflow run:
- A loading indicator was shown in the UI.
- The UI blocked and waited until ALL steps in the workflow run were completely finished (which could take a long time).
- After completion, it navigated back to the workflow runs list page instead of the specific detail page of that run.

This created a laggy and confusing experience, where the user could not monitor the execution of individual steps in real time.

## Expected Behavior
- Do not wait for the runner to finish all steps before navigating.
- Immediately after starting the run, navigate the user to the specific detail page of that run (e.g., `/workflow-runs/$runId`).
- The execution should continue running in the background, updating the UI dynamically.

## Acceptance Criteria
- Starting a workflow run returns the run ID immediately.
- The user is navigated straight to `/workflow-runs/$runId` right after starting the run.
- The loading screen does not block the UI waiting for the run to complete.
- The workflow steps execute asynchronously in the background.
