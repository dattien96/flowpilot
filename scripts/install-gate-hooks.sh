#!/usr/bin/env bash
# Install git pre-commit + documents Claude Stop hook path (CP-53 P-3).
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
HOOK_SRC="$ROOT/scripts/hooks/pre-commit"
HOOK_DST="$ROOT/.git/hooks/pre-commit"

if [[ ! -d "$ROOT/.git" ]]; then
  echo "install-gate-hooks: not a git repo: $ROOT" >&2
  exit 1
fi
cp "$HOOK_SRC" "$HOOK_DST"
chmod +x "$HOOK_DST"
echo "Installed pre-commit hook → .git/hooks/pre-commit"
echo "Claude Stop hook: see .claude/settings.json (repo-tracked)"
echo "Uninstall: rm .git/hooks/pre-commit"
