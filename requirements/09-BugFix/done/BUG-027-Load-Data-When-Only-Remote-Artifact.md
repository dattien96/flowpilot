There is some UI components can not load full data in case the artifact was deleted from local

Prepare
- Run 2 flow A - B
- We can see the local artifacts generated in local part .flowpilot/artifacts
- Sync both to bucket supabase
- Then Del the artifact of A only in local

Now test

# In Artifact list page - Each artifact item
- For artifact B which has local&remote: we can see the description like: Actual prompt ...
- For A - Empty

-> I think we encounter this one with the title in BUG-024


# In workflow-runs DETAIL page

## For the workflow run that belong to artifact B
We can see 
- The response tab ok
- The Prompt tab ok
- The Artifact tab ok too
- 3 tabs have data
- 
## For the artifact A
- The response tab is ok
BUT
- Tab prompt showed: Prompt Sent
No prompt was captured for this output.
- NO ARTIFACT TAB showed