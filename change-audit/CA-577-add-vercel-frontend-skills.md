# CA-577: Add Vercel React, React Native, and Web Design Skills to FlowPack

```flowpilot:change-ledger
feature_key: skill-injection
type: Feature
layer: infra
```

## Changes
- Imported top frontend and mobile skill packs from `vercel-labs/agent-skills`:
  1. `reactjs/react-best-practices`: 70 performance optimization rules (waterfalls, bundle size, server-side, re-render, etc.).
  2. `reactjs/composition-patterns`: Component composition patterns for React 19, avoiding boolean prop proliferation.
  3. `reactjs/react-view-transitions`: View Transitions API guide and animation recipes.
  4. `react-native/react-native-skills`: React Native & Expo mobile app best practices.
  5. `common/web-design-guidelines` & `reactjs/web-design-guidelines`: Web interface UI/UX and A11y review guidelines.
