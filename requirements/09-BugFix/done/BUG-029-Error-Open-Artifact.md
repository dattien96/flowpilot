Continue with bug028 i see a new bug

in the tab Artifact of output section in Workflow run

When pressing button Open In New Tab

# OK
If the artifact exist in local -> open done
Response.md
.flowpilot\artifacts\392355a6-5573-44f1-9aa5-4313502a5816\ae89a428-4f25-4294-b934-0123a66e22c8\codex_test\.snapshots\c65976bc-6df6-4802-9a15-d7b857c00957\Response.md

# Bug
If the artifact was in remote, then deleted in local -> open failed

.flowpilot/artifacts/392355a6-5573-44f1-9aa5-4313502a5816/ed9e4ce0-be63-49fb-bfe7-e68f1c376743/codex_test/.snapshots/1e4d95fc-881d-49d7-862e-e6e8cdf43ec4/Response.md

I see you will open this doc. But it is not correct cause this artifact was in remote only
When fixing show artifact tab in BUG 027 - You use wrong path

You can see in /artifacts page in tab remote
When press open any artifact, if it is only in remote -> open by remote URL
For example
It openned this url: /api/local-runner/artifacts/1e4d95fc-881d-49d7-862e-e6e8cdf43ec4/open?remotePath=

