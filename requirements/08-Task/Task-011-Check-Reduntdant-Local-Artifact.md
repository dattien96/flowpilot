Sometimes, some redundant artifacts that not match any row in supabase artifact_runs table but still live in local .flowpilot/artifacts folder

When starting server, run a golang bg job to check and delete those local ones