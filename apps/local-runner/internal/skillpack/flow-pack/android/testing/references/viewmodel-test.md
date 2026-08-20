# ViewModel Unit Test Example

This example demonstrates how to test a ViewModel using Mokkery for mocking, `runTest` for coroutines, and verifying both state updates and use case interactions.

### Template

```kotlin
class ExampleViewModelTest {
    private val testRule = BasicTestRule()

    @BeforeTest
    fun setup() {
        testRule.onBeforeTest()
    }

    @AfterTest
    fun tearDown() {
        testRule.onAfterTest()
    }

    @Test
    fun `handleAction - success - updates state correctly`() = runTest {
        // 1. Arrange
        val viewModelHandler = createMockViewModelHandler()
        val mockUseCase = mock<ISomeUseCase>()
        val data = SomeData(id = "123")
        
        // Mocking the success path
        everySuspend { mockUseCase() } returns AppResult.Success(data)

        val viewModel = ExampleViewModel(viewModelHandler, mockUseCase)

        // 2. Act
        viewModel.handleSomeAction()
        waitForFlowCollectors() // Ensure async operations complete

        // 3. Assert
        val currentState = viewModelHandler.state.value
        assertTrue(currentState is BaseScreenState.Success)
        assertEquals(data, (currentState as BaseScreenState.Success).data)

        // Verify interaction
        verifySuspend(VerifyMode.exactly(1)) { mockUseCase() }
    }
}
```

### Key Principles
- **State Validation**: Always check the `viewModelHandler.state` (or equivalent) to ensure the UI state is updated correctly.
- **Async Synchronization**: Use `waitForFlowCollectors()` or similar mechanisms to ensure all `StateFlow` updates are processed before assertions.
- **Mocking**: Use `mock<T>()` and `everySuspend { ... } returns ...` from Mokkery.
- **Verification**: Use `verifySuspend` to ensure crucial side effects or use case calls occurred.
