Let say with codex
when i have some config in toml in default acc with agent
[agents]

[agents.coder]
description = "Implement features and tests from approved signatures."
config_file = "./agents/coder-agent.toml"
nickname_candidates = ["Builder", "Dex"]

[agents.reviewer]
description = "Find correctness, security, and test risks in code."
config_file = "./agents/reviewer-agent.toml"
nickname_candidates = ["Athena", "Ada"]

So when i connect new account we should check and mirror/copy config to new config file of new acc

Same for other config as well as other ai-providers like gemini/claude