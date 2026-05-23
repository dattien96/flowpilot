# CP-15-02: Project Launch And Workspace UX

## 1. Goal

Make the project UI aware of project directory bindings so users can open and run the same shared project from different machines without duplicating the project.

## 2. Main UX Rule

The project detail page remains the workflow launch surface, but execution must fail safely if the Go runner cannot open any bound directory for that project.

## 3. Runtime Validation Flow

When a workflow is triggered:

1. runner loads all directory bindings for the project
2. runner checks each local path
3. if one path is usable, workflow runs there
4. if no path is usable, trigger fails and user sees the root cause

## 4. Scope

This CP covers:

- project create flow behavior
- project detail directory-binding tab
- project create multi-binding flow
- launch failure guidance

This CP does not cover:

- step runtime field editing
- full runner execution internals

## 5. Required Changes

### 5.1 Project Create

Project creation should still support choosing local folders, but the meaning changes:

- it creates the project
- it can also create one or more initial directory bindings for that project

It must no longer imply:

- this path is globally shared by all devices

### 5.2 Project Detail Directory Binding Tab

Add one tab on project detail page:

- `Directory Binding`

This tab must show:

- list of all bindings for the project
- add binding action
- edit binding action
- delete binding action
- clear guidance that the runner will try these paths when execution starts

### 5.3 Project Create Binding UX

Project create page should allow:

1. browse and add one path
2. browse and add more paths
3. edit/remove paths before submit

At least one binding is required at create time.

### 5.4 Launch Failure UX

If workflow trigger fails because no binding path is usable:

1. show the root cause returned by the runner
2. direct the user to the `Directory Binding` tab
3. allow the user to add or update bindings there

This flow must not tell the user to create a duplicate project.

## 6. Acceptance Criteria

- [ ] project create flow can add multiple directory bindings
- [ ] project detail page has a `Directory Binding` tab
- [ ] users can add, edit, and delete project directory bindings
- [ ] project can be opened on a second device without creating a duplicate project
- [ ] failed execution due to missing/invalid paths shows clear root-cause guidance

## 7. Risks

### 7.1 Stale bindings

Some saved bindings may no longer exist on disk.

Mitigation:

- runner validates bindings only at trigger time
- failed paths must be reported clearly
- user can update bindings in the `Directory Binding` tab

## 8. Exit Condition

This CP is complete when a user can reuse the same project on another device and manage multiple directory bindings from the project UI without duplicating the project.
