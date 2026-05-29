Until now we have fix many cases related the killing process active on the BUG-013

This is test result from me

- Press Ctrl - C to stop system -> Ai-Processes was killed -> OK
- Press KILL PROCESS button in workflow-run list page -> Ai-Processes was killed -> OK
- Press Kill process button in each detail run page -> Ai-Processes was killed -> OK
- Press DEL workflow-run in page list -> Ai-Processes was killed -> OK
- Press BUTTON STOP server in MEnu -> Ai-Processes was killed -> OK
- Press BUTTON RESTART server in MEnu -> Ai-Processes was killed -> OK then server run again after that

Ok look good BUT EXPECT only 1 case is

WE CAN ONLY KILL PROCESS WHEN WE HAVE REAL PID

Righaway after we trigger a task -> we do not have real ID of process
We only have some temp UI like this
SESSION_ID = claude_stream_session_prompt_xxx for claude
OR SESSION_ID = codex_mcp_session_prompt_yyy for codex
Similar to gemini

If we press KILL them in this state -> failed

Only after wait the 1st response return, the ai provider returns real session ID like 019e-xxx-yyy
Then KILL them sucessfully.