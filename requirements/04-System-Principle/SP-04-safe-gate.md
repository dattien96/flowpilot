# 1. Stop before risky actions

If we are in default mode (not YOLO mode),
AI must stop and ask for approval when:
- requirement is ambiguous,
- API contract is missing,
- architecture impact is high,
- database/storage/security changes are involved,
- production/release decision is involved,
- generated output conflicts with existing rules.
- executing critical system commands.