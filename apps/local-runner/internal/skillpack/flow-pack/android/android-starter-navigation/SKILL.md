---
name: android-starter-navigation
description: Implement or review AppStart navigation using a dedicated core navigation module, type-safe route keys, back-stack state, deep links, and feature decoupling. Use for destinations, navigation events, deep links, tabs, or adaptive list-detail flows.
---

# AppStart Navigation

- Define serializable, type-safe destination keys in `core:navigation`; never use ad-hoc string routes.
- Let features emit navigation intents or effects through a navigator interface; they must not know the host graph implementation.
- Follow the MPlan Navigation 3 split: `core:navigation` owns `AppNavHost`, the host-owned back stack, router contract, decorators, and `AppGraphInput`; each feature contributes an `AppFeatureNavGraph` that only registers entries against that input. Hilt aggregates feature graphs at runtime, while the app host attaches the router to its remembered back stack.
- Keep route arguments minimal, stable, and serializable. Load mutable or large data through the destination's ViewModel.
- Save back stacks for top-level tabs and handle system Back through one host policy.
- Add deep links only with a documented URI contract and test both cold-start and warm-start behavior.
- Use Navigation 3 scenes only when the feature needs multi-pane, dialog, sheet, or multiple-back-stack behavior; otherwise keep the type-safe host minimal.

Bind each graph with Hilt `@IntoSet`. `AppNavHost` consumes `Set<AppFeatureNavGraph>` and must not
import concrete feature graph classes.

---

# End-to-End Reference Implementation

This is the full dependency flow, as it exists in this repo (PrivateVault). Copy this shape for
every new feature — do not invent a variation. The independence rule is load-bearing: **features
never import another feature's route, and `:core:navigation`/`:app` never name a feature.**

## 1. `:core:navigation` — contracts + host

```
core/navigation/src/main/java/com/privavault/core/navigation/
├── AppRoute.kt                          // open route contract + namespaced content key
├── ScreenId.kt                          // @JvmInline value class, "<domain>/<screen>" format
├── AppRouter.kt                         // router contract + AppNavigator (owns the back stack)
├── AppFeatureNavGraph.kt                // contribution contract + AppGraphInput + appEntry
├── TopLevelDestination.kt               // adaptive-suite tab contract
├── PrivaVaultAdaptiveNavigationSuite.kt // bottom bar <600dp / Navigation Rail >=600dp
└── AppNavHost.kt                        // the ONLY NavDisplay in the app
```

`AppRoute` is **open on purpose** — sealed routes in core force every feature to edit a shared
module:

```kotlin
// AppRoute.kt
interface AppRoute : NavKey {
    val screen: ScreenId
}

val AppRoute.navContentKey: Any
    get() = "$screen#$this"
```

```kotlin
// ScreenId.kt
@JvmInline
value class ScreenId(val value: String) {
    init { require(FORMAT.matches(value)) { "..." } }   // <domain>/<screen>, lower-kebab
    override fun toString(): String = value
    private companion object { val FORMAT = Regex("[a-z][a-z0-9-]*/[a-z0-9-]+") }
}
```

```kotlin
// AppRouter.kt — the navigator OWNS the stack. No Channel, no attach step, no
// rememberNavBackStack anywhere. @ActivityRetainedScoped survives rotation.
interface AppRouter {
    fun navigate(route: AppRoute)
    fun pop(): Boolean
}

@ActivityRetainedScoped
class AppNavigator @Inject constructor(
    @StartDestination startDestination: NavKey,
) : AppRouter {
    internal val backStack: SnapshotStateList<NavKey> = mutableStateListOf(startDestination)

    override fun navigate(route: AppRoute) { backStack.add(route) }

    override fun pop(): Boolean {          // fail-closed: never pops the root
        if (backStack.size <= 1) return false
        backStack.removeAt(backStack.lastIndex)
        return true
    }
}
```

```kotlin
// AppFeatureNavGraph.kt — contribution seam. input.router is how features navigate.
interface AppFeatureNavGraph {
    val screens: Set<ScreenId>
    fun EntryProviderScope<NavKey>.registerEntries(input: AppGraphInput)
}

data class AppGraphInput(val router: AppRouter)

inline fun <reified R : AppRoute> EntryProviderScope<NavKey>.appEntry(
    noinline content: (R) -> Unit,
) { entry<R>(clazzContentKey = { route -> route.navContentKey }, content = content) }

fun checkNoDuplicateScreens(featureGraphs: Set<AppFeatureNavGraph>) { /* throws on clash */ }
```

```kotlin
// AppNavHost.kt — the only NavDisplay. NO feature imports.
@Composable
fun AppNavHost(
    navigator: AppNavigator,
    featureGraphs: Set<AppFeatureNavGraph>,
    modifier: Modifier = Modifier,
) {
    checkNoDuplicateScreens(featureGraphs)
    val input = remember { AppGraphInput(router = navigator) }
    NavDisplay(
        backStack = navigator.backStack,
        onBack = navigator::pop,
        entryDecorators = listOf(
            rememberSaveableStateHolderNavEntryDecorator(),
            rememberViewModelStoreNavEntryDecorator(),
        ),
        entryProvider = entryProvider {
            featureGraphs.forEach { graph -> graph.run { registerEntries(input) } }
        },
        modifier = modifier,
    )
}
```

Module deps: `api(:core:di-qualifiers)` (so `@StartDestination` reaches the Hilt component in
`:app`), `api(navigation3-runtime)`, `implementation(navigation3-ui, lifecycle-viewmodel-navigation3,
compose-material3, material3-adaptive-navigation-suite)`.

## 2. `:core:di-qualifiers` — the qualifier seam

```kotlin
// core/di-qualifiers/.../StartDestination.kt — separate module so modules that only need the
// qualifier never edge into :di (enforceDiBoundary keeps :di reachable from :app only).
package com.privavault.core.diqualifiers

@Qualifier
@Retention(AnnotationRetention.BINARY)
annotation class StartDestination
```

Why a qualifier at all: `NavKey` is a framework type. Without `@StartDestination`, the
`@Provides` for the entry route collides with every other `NavKey` binding and any future
multi-stack host.

## 3. Feature presentation module — owns its routes + graph

```kotlin
// feature/dashboard/presentation/.../graph/DashboardRoutes.kt
data object DashboardRoute : AppRoute {
    override val screen = ScreenId("dashboard/root")
}

// Tab for the adaptive suite — feature-owned label/icon/order.
val DashboardTab = TopLevelDestination(
    screen = DashboardRoute.screen,
    route = DashboardRoute,
    label = "Dashboard",
    icon = Icons.Default.Home,
    order = 0,
)
```

```kotlin
// graph/DashboardFeatureNavGraph.kt — plain class, constructor @Inject for feature deps.
class DashboardFeatureNavGraph @Inject constructor() : AppFeatureNavGraph {
    override val screens = setOf(DashboardRoute.screen)

    override fun EntryProviderScope<NavKey>.registerEntries(input: AppGraphInput) {
        appEntry<DashboardRoute> {
            DashboardScreen(onOpenVault = { input.router.navigate(...) })
        }
    }
}
```

Cross-feature navigation NEVER imports another feature's route. If Dashboard opens Vault, it
calls a port exposed by the feature's own `:api` module (e.g. `OpenVaultPort`); the vault feature
implements that port and navigates to `VaultGridRoute` itself via `input.router`. The
`feature-dependency-policy` convention plugin already makes feature→feature module edges a build
failure.

## 4. `:di` — bindings, installed on ActivityRetainedComponent

```kotlin
// di/.../AppRouterModule.kt
@Module
@InstallIn(ActivityRetainedComponent::class)
abstract class AppRouterModule {
    @Binds abstract fun bindAppRouter(navigator: AppNavigator): AppRouter
}

// di/.../AppDestinationModule.kt — the only place the app picks its entry screen.
@Module
@InstallIn(ActivityRetainedComponent::class)
object AppDestinationModule {
    @Provides @StartDestination fun startDestination(): NavKey = DashboardRoute
}

// di/.../DashboardNavigationModule.kt — per feature: graph AND tab contributions.
@Module
@InstallIn(ActivityRetainedComponent::class)
abstract class DashboardNavigationModule {
    @Binds @IntoSet
    abstract fun bindDashboardFeatureNavGraph(graph: DashboardFeatureNavGraph): AppFeatureNavGraph

    companion object {
        @Provides @IntoSet fun dashboardTab(): TopLevelDestination = DashboardTab
    }
}
```

`:di` may import feature classes — it is the composition root. `AppNavHost` may not.

## 5. `:app` — wiring only, zero navigation logic

```kotlin
// MainActivity.kt
@AndroidEntryPoint
class MainActivity : ComponentActivity() {
    @Inject lateinit var navigator: AppNavigator
    // @JvmSuppressWildcards is REQUIRED on injected Set<T> — Kotlin wildcards
    // break the Hilt multibinding lookup without it.
    @Inject lateinit var featureGraphs: @JvmSuppressWildcards Set<AppFeatureNavGraph>
    @Inject lateinit var topLevelDestinations: @JvmSuppressWildcards Set<TopLevelDestination>

    override fun onCreate(savedInstanceState: Bundle?) {
        ...
        setContent {
            PrivaVaultTheme {
                PrivaVaultAdaptiveNavigationSuite(navigator, topLevelDestinations) {
                    AppNavHost(navigator, featureGraphs)
                }
            }
        }
    }
}
```

## 6. Adaptive chrome — `PrivaVaultAdaptiveNavigationSuite`

`NavigationSuiteScaffold` renders a **bottom navigation bar under 600dp window width** and a
**Navigation Rail at 600dp+** — the library decides; no `WindowSizeClass` plumbing in app code.
Source plan: `design/ADAPTIVE-MULTI-SCREEN.md` §3.

- Tab model: `TopLevelDestination(screen, route, label, icon, order)` — same `@IntoSet`
  contribution shape as graphs, so adding a tab = feature file + one `:di` provider.
- Selected state reads `navigator.backStack.lastOrNull()` cast to `AppRoute` → `screen`. The back
  stack is the single source of truth; deep links and restores highlight correctly for free.
- Re-tapping the current tab is a no-op (`if (currentScreen != destination.screen) navigate(...)`) —
  tabs push fresh entries rather than duplicating the same screen.
- Colors come from `MaterialTheme.colorScheme` (already themed by `PrivaVaultTheme`), keeping
  `:core:navigation` free of a design-system edge.
- Hide the suite on screens that are not top-level (lock keypad, onboarding, player full-screen):
  gate the scaffold on whether `currentScreen` is in `tabs.map { it.screen }` when those features
  land — spec lives in `design/screens/*` (unlock has no bottom nav; settings is a tab).

## 7. Tests that pin this architecture

- `AppNavigatorTest` — starts on injected destination, append order, `pop()` fails closed at root.
- `ScreenIdTest` — format gate + `navContentKey` namespaces.
- `CheckNoDuplicateScreensTest` — clashing `screens` across graphs fails with both names.
- Contract harness: `requirements/.flowpilot/vibe/appbootstrap/navigation_contract_test.go` locks
  every signature above and bans `sealed interface`, `NavCommand`, `Channel`, `NavController`,
  `rememberNavBackStack`, and `import com.privavault.feature.` inside `:core:navigation`.

## Forbidden patterns (all banned by contract test)

- `sealed`/`object` route declarations inside `:core:navigation` — routes are open, feature-owned.
- `NavCommand` / `Channel<...>` / `receiveAsFlow` / a suspend navigator — commands are dead code;
  `AppRouter` is synchronous and owns the stack.
- `rememberNavBackStack`, composition-owned `SnapshotStateList`, attach/detach lifecycle.
- `NavController`, string routes, `createRoute()`, a second `NavDisplay` anywhere else.
- Feature → feature route imports; `AppNavHost`/`MainActivity` importing feature classes.
- Injecting `Set<AppFeatureNavGraph>`/`Set<TopLevelDestination>` without `@JvmSuppressWildcards`.
