# Repository Unit Test Example

This example demonstrates how to test a Repository implementation, typically involving data mapping, local/remote data source mocking, and error handling.

### Template

```kotlin
class RepositoryImplTest {
    private val testRule = BasicTestRule()

    @BeforeTest
    fun setup() {
        testRule.onBeforeTest()
    }

    @Test
    fun `getMetrics - authenticated user - returns metrics from local data source`() = runTest {
        // 1. Arrange
        val mockUserUseCase = mock<IGetCurrentUserUseCase>()
        val mockLocalDataSource = mock<ILocalDataSource>()
        val user = User(uid = "user-123")
        val localData = listOf(LocalMetric(id = "1"))
        
        everySuspend { mockUserUseCase() } returns AppResult.Success(user)
        everySuspend { mockLocalDataSource.getMetrics(user.uid) } returns AppResult.Success(localData)

        val repository = MetricsRepositoryImpl(mockUserUseCase, mockLocalDataSource)

        // 2. Act
        val result = repository.getMetrics()

        // 3. Assert
        assertTrue(result is AppResult.Success)
        assertEquals(1, result.data.size)
        assertEquals("1", result.data[0].id)
        
        verifySuspend(VerifyMode.exactly(1)) { mockLocalDataSource.getMetrics(any()) }
    }

    @Test
    fun `getMetrics - unauthenticated - returns AuthError`() = runTest {
        // Arrange
        val mockUserUseCase = mock<IGetCurrentUserUseCase>()
        val mockLocalDataSource = mock<ILocalDataSource>()
        
        everySuspend { mockUserUseCase() } returns AppResult.Success(null)

        val repository = MetricsRepositoryImpl(mockUserUseCase, mockLocalDataSource)

        // Act
        val result = repository.getMetrics()

        // Assert
        assertTrue(result is AppResult.Error)
        assertTrue((result as AppResult.Error).errorEntity is ErrorEntity.AuthError)
    }
}
```

### Key Principles
- **Mocking Boundaries**: Mock the data sources (Local/Remote) and any user/auth context builders.
- **Mapping Correctness**: Ensure data is correctly mapped from DAO/Entity models to Domain models.
- **Error Propagation**: Verify that errors from data sources are correctly translated into domain-level `AppResult.Error`.
- **Serialization**: If testing data storage, verify serialization/deserialization logic.
