# BUG-020 Claude terminal showed but did not start

Symptom:
Pressing `Test` on a Claude account opened a new terminal window, but Claude itself did not start.

Root cause:
The Windows interactive-test path was building a raw `cmd.exe /k` command string directly from the resolved provider binary. That was brittle for Claude because the binary is commonly a `.cmd` shim, and the launch path was not consistently setting the provider home environment in the same way the working Codex test path does.

What we changed:
- Switched the Windows provider terminal launcher to generate a temporary `.cmd` script and run that script in the new terminal.
- Quoted the resolved provider binary path before invocation.
- Used `call` for `.cmd` and `.bat` shims so the shell actually executes Claude instead of just opening a prompt.
- Expanded the Windows environment setup for Claude to include `HOME`, `USERPROFILE`, `APPDATA`, `LOCALAPPDATA`, and `XDG_CONFIG_HOME`.
- Kept Codex aligned with its expected home-style environment by also setting `USERPROFILE` in the Windows env block.
- Preserved the provider-specific account home path selection that the runtime already uses for workflow execution.

Current behavior:
- `Test` now opens a terminal that runs the provider CLI from the selected account home.
- The command is launched with the same provider-home isolation expected by the rest of the local runner.
- Workflow execution already resolves the active provider account separately, so this fix only affects the interactive terminal test/login flow.

Verification:
- Added a regression test for Windows working-directory quoting.
- Added a regression test for the Claude Windows environment block.
- Confirmed the targeted runner tests pass:
  - `TestCommandWithWindowsWorkingDirectoryQuotesPath`
  - `TestProviderEnvSetCommandWindowsClaudeIncludesHomeStyleEnv`
