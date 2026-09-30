---
version: 6
name: android-adaptive-multi-screen
description: >-
  Use this skill PROACTIVELY when designing, implementing, or reviewing Android Jetpack Compose UI
  for multi-screen support, WindowSizeClass breakpoints, Canonical Two-Pane layouts, foldables,
  and desktop windowing parity (Google Play Tier 1 Quality).
---

# Android Adaptive Multi-Screen & Desktop Windowing Skill

This skill encodes the engineering principles and architectural patterns required to achieve **Tier 1 (Differentiated Experience)** compliance under the **Google Play Adaptive App Quality Guidelines (2024–2026)** and **Android 15/16 native Desktop Windowing policy**.

AI agents working on Jetpack Compose presentation layers **MUST strictly adhere** to these rules.

---

## 🏛️ SECTION 1: HARD GATES & CORE LAWS

### 🚫 LAW 1: ZERO HARDCODED ORIENTATION
* **Never set `android:screenOrientation="portrait"` or `"landscape"` in `AndroidManifest.xml`.**
* Hardcoding orientation triggers aggressive OS letterboxing on tablets, foldables, and desktop freeform windows, and incurs immediate Google Play Store ranking penalties.
* Dynamic configuration changes (rotation, window resize, fold/unfold) MUST be handled gracefully with zero state loss using MVI / ViewModel state hoisting.

### 📐 LAW 2: WINDOW-BASED, NOT HARDWARE-BASED
* **Never check hardware model strings (e.g. `Build.MODEL`), screen density, or hardcoded boolean `isTablet`.**
* Always compute layout dimensions from the **active window bounds** using `WindowSizeClass` (`calculateWindowSizeClass(activity)`).
* In split-screen mode or freeform desktop windows, a tablet can run in a `Compact` width window ($< 600\text{dp}$), while a foldable unfolded can run in `Expanded` width ($\ge 840\text{dp}$). Layouts MUST adapt to the window, not the physical chassis.

### 🛑 LAW 3: DIALOG MAX-WIDTH CEILING ($\le 560\text{dp}$)
* **Never permit modal dialogs or bottom sheets to stretch to full screen width on wide displays.**
* Unconstrained dialogs spanning $1000\text{dp}+$ on tablets severely degrade readability and visual hierarchy.
* Every dialog MUST enforce:
  ```kotlin
  Dialog(
      onDismissRequest = onDismissRequest,
      properties = DialogProperties(usePlatformDefaultWidth = false)
  ) {
      Box(
          modifier = modifier
              .widthIn(min = 280.dp, max = 560.dp)
              .fillMaxWidth()
      ) { /* content */ }
  }
  ```

### 🧭 LAW 4: ADAPTIVE NAVIGATION SUITE
* Use `NavigationSuiteScaffold` from `androidx.compose.material3.adaptive.navigationsuite`:
  * **Compact width ($< 600\text{dp}$):** Persistent Bottom `NavigationBar`.
  * **Medium width ($600\text{dp} - 839\text{dp}$):** Start-edge vertical `NavigationRail`.
  * **Expanded width ($\ge 840\text{dp}$):** Start-edge `NavigationRail` or Permanent `NavigationDrawer`.
* Never duplicate navigation logic across separate phone and tablet activities.

### 🪟 LAW 5: CANONICAL TWO-PANE LAYOUTS FOR EXPANDED SCREENS
* When `WindowWidthSizeClass == Expanded` ($\ge 840\text{dp}$):
  * **Data lists / Galleries:** Use `ListDetailPaneScaffold` (Left: Master List/Grid, Right: Detail View/Player).
  * **Dashboards / Cockpits:** Use `SupportingPaneScaffold` (Left: Hero metrics & Primary CTA, Right: Real-time telemetry & Audit logs).
  * **Destructive / High-stakes flows:** Dual-pane split (Left: Queue & Configuration, Right: Multi-pass progress stepper & Certificate).

### 📖 LAW 6: FOLDABLE TABLETOP & HINGE AWARENESS
* Check for physical screen separation using Jetpack WindowManager `FoldingFeature`:
  * If `foldingFeature.state == HALF_OPENED` (Tabletop Posture):
    * **Upper half:** Non-interactive consumption / visual preview (e.g. video player, radar canvas, progress gauge).
    * **Lower half:** Interactive controls (e.g. keypad, media controls, slider, action buttons).
  * Never place primary buttons or interactive text directly across the physical hinge crease (`foldingFeature.bounds`).

### 🖱️ LAW 7: DESKTOP & PERIPHERAL INPUT PARITY (TIER 1 REQUIREMENT)
* With Android 15/16 Desktop Mode, Samsung DeX, and ChromeOS, users expect desktop-class input handling:
  1. **Mouse Hover Feedback:** All clickable surfaces MUST provide hover indicators and change pointer icon:
     ```kotlin
     Modifier
         .pointerHoverIcon(PointerIcon.Hand)
         .hoverable(interactionSource = interactionSource)
     ```
  2. **Keyboard Shortcuts:** Support essential keyboard accelerators:
     * `Escape` $\to$ Dismiss dialog, navigate back, cancel ongoing operation.
     * `Space` $\to$ Pause / resume media playback or scanning.
     * `Delete` / `Backspace` $\to$ Remove selected items from queue.
     * `Ctrl/Cmd + L` $\to$ Emergency lock / purge memory.
  3. **External Drag & Drop:** Support file drop from system file manager into actionable areas (e.g. Shredder queue or Vault importer) via `Modifier.dragAndDropTarget()`.

---

## 📊 SECTION 2: BREAKPOINT SPECIFICATION MATRIX

```
+-----------------------------------------------------------------------------------------+
| WINDOW WIDTH BREAKPOINTS (Horizontal)                                                   |
+------------------------------------+--------------------------------+-------------------+
| Compact (< 600dp)                  | Medium (600dp - 839dp)         | Expanded (≥ 840dp)|
| Standard Phones (Portrait),        | Small Tablets, Foldables       | Large Tablets     |
| Narrow Split-Screen Windows        | Unfolded, Half Split-Screen    | Desktop Freeform  |
+------------------------------------+--------------------------------+-------------------+
| • Bottom Navigation Bar            | • Navigation Rail (Left side)  | • Navigation Rail |
| • Single-column vertical scroll    | • 2-Column Responsive Grid     | • Canonical Panes |
| • Dialogs fill 92% width           | • Dialogs max 560dp            | • Dialogs max 560dp|
+------------------------------------+--------------------------------+-------------------+
```

```
+-----------------------------------------------------------------------------------------+
| WINDOW HEIGHT BREAKPOINTS (Vertical)                                                    |
+------------------------------------+--------------------------------+-------------------+
| Compact (< 480dp)                  | Medium (480dp - 899dp)         | Expanded (≥ 900dp)|
| Phone Landscape                    | Standard Phone Portrait        | Tablets Portrait  |
+------------------------------------+--------------------------------+-------------------+
| • Auto-hide TopBar / Scrollable    | • Standard Header & Actions    | • Generous spacing|
| • Compact Keypad height (48dp)     | • Full Keypad height (68dp)    | • Multi-row feed  |
+------------------------------------+--------------------------------+-------------------+
```

---

## 🛠️ SECTION 3: REUSABLE CODE PATTERNS

### 3.1 Adaptive Navigation Scaffold
```kotlin
@Composable
fun AppAdaptiveNavigationSuite(
    currentRoute: String,
    onNavigate: (String) -> Unit,
    modifier: Modifier = Modifier,
    content: @Composable () -> Unit
) {
    NavigationSuiteScaffold(
        modifier = modifier,
        navigationSuiteItems = {
            Destinations.entries.forEach { destination ->
                item(
                    selected = currentRoute == destination.route,
                    onClick = { onNavigate(destination.route) },
                    icon = { Icon(destination.icon, contentDescription = destination.label) },
                    label = { Text(destination.label) }
                )
            }
        },
        content = content
    )
}
```

### 3.2 Max-Width Constrained Dialog Container
```kotlin
@Composable
fun AdaptiveDialogContainer(
    onDismissRequest: () -> Unit,
    modifier: Modifier = Modifier,
    maxWidth: Dp = 560.dp,
    content: @Composable () -> Unit
) {
    Dialog(
        onDismissRequest = onDismissRequest,
        properties = DialogProperties(usePlatformDefaultWidth = false)
    ) {
        Box(
            modifier = modifier
                .fillMaxWidth()
                .padding(horizontal = 24.dp)
                .wrapContentHeight(),
            contentAlignment = Alignment.Center
        ) {
            Box(
                modifier = Modifier
                    .widthIn(min = 280.dp, max = maxWidth)
                    .fillMaxWidth()
            ) {
                content()
            }
        }
    }
}
```

### 3.3 Mouse Hover & Pointer Cursor Modifier
```kotlin
fun Modifier.adaptiveHoverEffect(
    shape: Shape = RoundedCornerShape(16.dp),
    hoverBorderColor: Color,
    normalBorderColor: Color,
    borderWidth: Dp = 1.dp
): Modifier = composed {
    val interactionSource = remember { MutableInteractionSource() }
    val isHovered by interactionSource.collectIsHoveredAsState()
    
    val animatedBorderColor by animateColorAsState(
        targetValue = if (isHovered) hoverBorderColor else normalBorderColor,
        animationSpec = tween(durationMillis = 200),
        label = "HoverBorderAnim"
    )
    
    this
        .pointerHoverIcon(PointerIcon.Hand)
        .hoverable(interactionSource = interactionSource)
        .border(borderWidth, animatedBorderColor, shape)
}
```

### 3.4 Keyboard Shortcut Interceptor
```kotlin
fun Modifier.handleKeyShortcuts(
    onDismiss: (() -> Unit)? = null,
    onConfirm: (() -> Unit)? = null
): Modifier = onKeyEvent { keyEvent ->
    if (keyEvent.type == KeyEventType.KeyUp) {
        when (keyEvent.key) {
            Key.Escape -> { onDismiss?.invoke(); true }
            Key.Enter -> { onConfirm?.invoke(); true }
            else -> false
        }
    } else false
}
```

---

## ✅ SECTION 4: TIER 1 QUALITY CHECKLIST

Before marking any Compose screen or feature complete, verify against this checklist:

- [ ] **No Letterboxing:** App resizes dynamically without restart or crash.
- [ ] **State Preservation:** Form inputs, scroll positions, and active operations survive dynamic window resize.
- [ ] **Dialog Max Width:** All dialogs and modal bottom sheets are clamped to `widthIn(max = 560.dp)`.
- [ ] **Navigation Adaptation:** Bottom bar switches to start Navigation Rail at width $\ge 600\text{dp}$.
- [ ] **Two-Pane Layout:** Expanded screens ($\ge 840\text{dp}$) utilize dual panes instead of stretching a single column.
- [ ] **Touch Target Safety:** All clickables retain minimum $48\text{dp} \times 48\text{dp}$ touch target.
- [ ] **Mouse & Trackpad Support:** Hand pointer icon displayed on clickable cards/buttons with hover visual feedback.
- [ ] **Keyboard Accessibility:** `Escape` key closes dialogs, sheets, and active focus.
