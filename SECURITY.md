# Security Policy

The FlowPilot team takes the security of our platform, runner daemon, and integrations seriously.

## Supported Versions

We provide security updates for the following versions of FlowPilot:

| Version | Supported          |
| ------- | ------------------ |
| `latest` (main) | :white_check_mark: |
| `< 1.0.0`       | :white_check_mark: |

---

## Threat Model & Scope

FlowPilot interacts with local developer systems, executes terminal processes, connects to external AI providers, and coordinates MCP servers. 

We consider vulnerabilities in the following areas as in-scope for security reports:

- **Command Injection / Privilege Escalation**: Unauthorized execution of OS commands outside configured tool permissions.
- **Node Isolation Bypasses**: Any method that allows a read-only / verdict-only agent (e.g. Reviewer, Scout) to write or mutate files on the local filesystem (`silent-deny` bypass).
- **Approval Gate Bypasses**: Executing mutating MCP calls (e.g. Google Drive writes) without explicit user approval when YOLO mode is off.
- **Credential Leakage**: Unintended transmission of provider API tokens, Supabase service keys, or OAuth credentials in logs, prompts, or error traces.

---

## Reporting a Vulnerability

**Please do not report security vulnerabilities through public GitHub issues.**

Instead, report vulnerabilities via:

1. **GitHub Private Vulnerability Reporting**: Use the [Security Advisories](https://github.com/dattien96/flowpilot/security/advisories/new) tab on GitHub (recommended).
2. **Email**: Send detailed findings to [security@flowpilot.dev](mailto:security@flowpilot.dev).

### What to include in your report:
- Type of vulnerability (e.g., node isolation escape, unauthenticated RPC call, prompt leakage).
- Step-by-step instructions or Proof of Concept (PoC) to reproduce.
- The environment details: OS, Go version, Provider CLI used.
- Any suggested fix or remediation, if known.

---

## Response Process & Timeline

1. **Acknowledgement**: We will acknowledge receipt of your vulnerability report within **48 hours**.
2. **Triage & Assessment**: We will confirm the issue and provide an estimated timeline for a patch within **5 business days**.
3. **Disclosure**: Once a fix is verified and deployed, we will coordinate public disclosure and credit the reporter.
