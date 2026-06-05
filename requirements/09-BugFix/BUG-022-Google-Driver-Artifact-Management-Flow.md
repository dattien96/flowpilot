- Handle accessToken + refreshToken after login + select folder in google driver
- Invoke new token automatically
  - Access tok expired -> use refresh tok to acquire new one
  - Refresh tok expired -> Confirm user login again (Same func with the button CONNECT Google Drive in /artifacts page)
- If project is in supabase storage -> gen Artifact A -> change to google driver and gen artifact B
  - Page /artifacts must show artifact by storage type. We can not show B for supabase or A for driver
  - Change UI :
    - Currently, we use collapsible UI with RUN_ID > multiple steps ID 
    - I want add more layers: Project ID > Storage type > RUN_ID > multiple steps ID 

i want to discuss this item again (5. Separate remote artifact sets by provider

  - Define the rule:
      - if project provider is supabase, remote tab shows only Supabase-backed artifacts
      - if project provider is google_drive, remote tab shows only Google Drive-backed artifacts

  - Do not show old Supabase artifacts in remote view while Google Drive is active
  - Do not show Google Drive artifacts in remote view while Supabase is active
  - Keep local-only artifacts independent from that remote-provider filtering)

even that is my origin requirement. But i have new idea

Let say
Step 1: we set supabase and sync artifact A
Step 2: We gen artifact B but have not sync to supabase
Step 3: we change to driver
Step 4: (New step here) Whenever we change the remote storage of project
-> Show the modal Of Syncing process as loading
-> behind the sync You got all previous artifact in local and supabase : in this case is A and B
-> sync it to google driver

Step5: user can continue gen artifact C -> push to driver
Step6: user change back to supabase => the modal process continue show to sync back C to supabase
SO basically the conditions are
- the data must be same/sync on both storage
- You need to check the artifact ID + project ID + runid to know which artifact is exist -> ignore sync




You said: 9. Multi-PC Semantics
Each runner must authorize Google Drive artifact sync independently.

Allowed:

PC A and PC B can use the same Google Cloud app registration
PC A and PC B can select the same Google Drive folder
Supabase metadata can show that artifacts have Google Drive replicas
Not allowed:

copying PC A refresh token to PC B through Supabase
assuming PC B can open/sync Drive artifacts without its own local authorization
If PC B has no local Google Drive authorization:

remote metadata may be visible
open/sync actions that require Drive access should show "connect Google Drive on this runner"

-> That mean, same project but in different runner, we can have diffrent driver (different data in artifact-storage-google-drive.json)
right?

And because the supabase only see this project is using driver. then it's ok if we use different local driver json data

But 1 question

- Runner A sync artifact A to Drive a@gmail.com
- Runner B with driver b@gmail.com can lost that artifact right ? Because actually in runner B we dont have actual switch provider step -> there is no sync process. And even we have sync process, it still can not know artifact from a@gmail.com
So i think the RULE here is: we need to use same gg acc + folder to sync artifact accross PC/runners


 1. Your case is good. 2 accounts but share 1 folder is ok cause driver has this feature
 2. But if using different accounts with different folders -> then we lost artifact here. So we must note this limitation
 3. And you said that (with the current implementation, there is still a real gap:
  if artifact A exists only in source remote and not locally on Runner B, provider-switch migration from Runner B can
  still fail, because source-remote download/hydration is not implemented yet. That is why the multi-PC and migration
  backfill DoD items remain [todo].) let more detail explain this case by steps

