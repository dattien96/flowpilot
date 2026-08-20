# Business Logic Specification Format

This file defines the standard form for business requirements so that the `ut-logic-sync` skill can accurately verify your tests.

## Format Rules

1. Use **Markdown** for clarity.
2. Every requirement should have a unique **ID**.
3. Clearly state the **Logic/Rule** (the "What").
4. (Optional) Specify the **Test Target** file name to speed up discovery.

---

# Example Requirements

## REQ-AUTH-01: Password Length
- **Requirement**: User registration must enforce secure passwords.
- **Logic**: Minimum **8** characters.
- **Test Target**: `RegisterUseCaseTest.kt`, `AuthRepositoryTest.kt`

## REQ-AUTH-02: Email Validation
- **Requirement**: Only valid email formats are allowed.
- **Logic**: Must contain `@` and a valid domain suffix.
- **Test Target**: `ValidatorUtilsTest.kt`

## REQ-PAY-01: Transaction Limit
- **Requirement**: Anti-fraud limit for guest users.
- **Logic**: Maximum amount **$500** per transaction.
- **Test Target**: `PaymentServiceTest.kt`
