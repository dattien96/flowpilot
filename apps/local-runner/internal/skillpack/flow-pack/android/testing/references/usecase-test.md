# UseCase Unit Test Example

This example demonstrates how to test a UseCase, specifically addressing the **Hard Gate** requirement of testing both Fast and Suspend paths for coroutines.

### Template

```kotlin
class ExampleUseCaseTest {
    private val testRule = BasicTestRule()

    @BeforeTest
    fun setup() {
        testRule.onBeforeTest()
    }

    @Test
    fun `invoke - Fast Path - returns immediately`() = runTest {
        // Arrange
        val mockRepo = mock<IRepository>()
        val data = "Fast Result"
        everySuspend { mockRepo.getData() } returns AppResult.Success(data)
        
        val useCase = ExampleUseCase(mockRepo)

        // Act
        val result = useCase.invoke()

        // Assert
        assertTrue(result is AppResult.Success)
        assertEquals(data, result.data)
        verifySuspend(VerifyMode.exactly(1)) { mockRepo.getData() }
    }

    @Test
    fun `invoke - Suspend Path - handles forced delay`() = runTest {
        // Arrange
        val mockRepo = mock<IRepository>()
        val data = "Delayed Result"
        
        // FORCE: Test the suspend branch with a delay
        everySuspend { mockRepo.getData() } calls {
            delay(10) 
            AppResult.Success(data)
        }
        
        val useCase = ExampleUseCase(mockRepo)

        // Act
        val result = useCase.invoke()

        // Assert
        assertTrue(result is AppResult.Success)
        assertEquals(data, result.data)
        verifySuspend(VerifyMode.exactly(1)) { mockRepo.getData() }
    }
}
```

### Key Principles
- **2-Path Coverage**: For every suspend function, write at least one test for the "Fast Path" (immediate return) and one for the "Suspend Path" (forced `delay`).
- **Domain Layer Target**: Aim for 100% branch coverage in the domain layer.
- **Result Wrapping**: Ensure `AppResult` (Success/Error) is correctly handled and asserted.
