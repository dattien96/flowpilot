# Coroutine Testing Examples

## 1. Suspend Function Coverage

**CRITICAL**: Test **2 execution paths** of the coroutine state machine:

### A. Fast Path (Immediate Return)
```kotlin
@Test
fun `test with immediate return`() = runTest {
    val mock = mock<IGetAiKeyUseCase>()
    everySuspend { mock() } returns AppResult.Success("key")
    // Test execution...
}
```

### B. Suspend Path (Actual Suspension) ⚠️
```kotlin
@Test
fun `test with actual suspension`() = runTest {
    val mock = mock<IGetAiKeyUseCase>()
    everySuspend { mock() } calls {
        kotlinx.coroutines.delay(10) // Force actual suspension
        AppResult.Success("key")
    }
    // Test execution...
}
```

## 2. Test Structure Template

```kotlin
@Test
fun `usecase - scenario description`() = runTest {
    // Given
    val repository = mock<Repository>()
    everySuspend { repository.fetch() } returns Success(data)

    val useCase = UseCase(repository)

    // When
    val result = useCase.invoke(params)

    // Then
    assertTrue(result is Success)
    verifySuspend(exactly(1)) { repository.fetch() }
}
```
