Sometimes, some redundant artifacts that not match any row in supabase artifact_runs table but still live in local .flowpilot/artifacts folder

When starting server, run a golang bg job to check and delete those local ones

For example:
You create artifacts by run in PCA

But you del those flowrun in PC -B
then it can only del supbase row in pbB

cause the local artifacts still be leak in PC A