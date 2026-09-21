---
name: react-native-scaffold-bootstrap
description: Quy chuẩn khởi tạo dự án Step 0 cho React Native & Expo Monorepo (Turborepo + pnpm). Hướng dẫn AI tự động sinh cấu trúc Monorepo, root configs, packages/core-*, và apps/_template đạt chuẩn biên dịch.
version: 6
---

# React Native & Expo Monorepo Scaffold Bootstrap (Step 0)

Tài liệu quy chuẩn dành cho FlowPilot Runner và AI Agent khi thực thi lệnh khởi tạo dự án (`init`) cho nền tảng React Native / Expo. Đảm bảo toàn bộ cấu trúc Monorepo, cấu hình build và template app được sinh ra với độ chính xác tuyệt đối (100% biên dịch thành công qua TypeScript).

---

## 1. CẤU TRÚC THƯ MỤC CHUẨN MONOREPO

```text
<project-root>/
├── package.json                               # Root package.json (Turborepo orchestration)
├── pnpm-workspace.yaml                        # Quản lý workspaces: packages/* và apps/*
├── turbo.json                                 # Cấu hình pipeline: dev, build, typecheck, test
├── tsconfig.base.json                         # TypeScript base config & path aliases
├── .gitignore                                 # Ignore chuẩn cho Expo, Node, Turbo
│
├── packages/                                  # REUSABLE CORE MODULES (100% Shared)
│   ├── core-ui/                               # Design tokens, NativeWind, Atomic components
│   │   ├── package.json
│   │   ├── tsconfig.json
│   │   └── src/index.ts
│   ├── core-storage/                          # Local-first: MMKV + SQLite BaseRepository
│   │   ├── package.json
│   │   ├── tsconfig.json
│   │   └── src/index.ts
│   ├── core-billing/                          # RevenueCat IAP abstraction & Paywall UI
│   │   ├── package.json
│   │   ├── tsconfig.json
│   │   └── src/index.ts
│   ├── core-ads/                              # Google AdMob coordinator & Frequency capper
│   │   ├── package.json
│   │   ├── tsconfig.json
│   │   └── src/index.ts
│   ├── core-pdf/                              # expo-print HTML engine & document templates
│   │   ├── package.json
│   │   ├── tsconfig.json
│   │   └── src/index.ts
│   ├── core-security/                         # Legal disclaimer modal & SafeMath
│   │   ├── package.json
│   │   ├── tsconfig.json
│   │   └── src/index.ts
│   └── core-common/                           # Imperial math, date, currency helpers
│       ├── package.json
│       ├── tsconfig.json
│       └── src/index.ts
│
└── apps/
    └── _template/                             # GOLDEN TEMPLATE APP (Plumbing có sẵn)
        ├── package.json
        ├── tsconfig.json
        ├── app.json
        ├── app.config.ts                      # Dynamic Expo config (CNG)
        └── app/
            ├── _layout.tsx                    # Root Layout (SafeArea, Context, ErrorBoundary)
            ├── +not-found.tsx
            ├── modal.tsx
            └── (tabs)/
                ├── _layout.tsx                # Bottom Tab Bar
                └── index.tsx                  # Home Screen mẫu
```

---

## 2. CÁC TẬP TIN CẤU HÌNH GỐC (ROOT CONFIGS)

### 2.1. `package.json` (Root)
```json
{
  "name": "flowpilot-mobile-studio",
  "version": "1.0.0",
  "private": true,
  "packageManager": "pnpm@9.1.0",
  "scripts": {
    "dev": "turbo run dev --parallel",
    "build": "turbo run build",
    "typecheck": "turbo run typecheck",
    "test": "turbo run test",
    "clean": "turbo run clean && rm -rf node_modules"
  },
  "devDependencies": {
    "turbo": "^2.0.0",
    "typescript": "~5.3.3",
    "prettier": "^3.2.5"
  }
}
```

### 2.2. `pnpm-workspace.yaml`
```yaml
packages:
  - "packages/*"
  - "apps/*"
```

### 2.3. `turbo.json`
```json
{
  "$schema": "https://turbo.build/schema.json",
  "tasks": {
    "build": {
      "dependsOn": ["^build"],
      "outputs": ["dist/**", ".next/**", "build/**"]
    },
    "typecheck": {
      "dependsOn": ["^typecheck"],
      "cache": true
    },
    "test": {
      "dependsOn": ["^build"],
      "cache": true
    },
    "dev": {
      "cache": false,
      "persistent": true
    },
    "clean": {
      "cache": false
    }
  }
}
```

### 2.4. `tsconfig.base.json`
```json
{
  "compilerOptions": {
    "target": "ES2022",
    "module": "ESNext",
    "moduleResolution": "Bundler",
    "jsx": "react-native",
    "strict": true,
    "esModuleInterop": true,
    "skipLibCheck": true,
    "resolveJsonModule": true,
    "baseUrl": ".",
    "paths": {
      "@flowpilot/core-ui": ["packages/core-ui/src/index.ts"],
      "@flowpilot/core-storage": ["packages/core-storage/src/index.ts"],
      "@flowpilot/core-billing": ["packages/core-billing/src/index.ts"],
      "@flowpilot/core-ads": ["packages/core-ads/src/index.ts"],
      "@flowpilot/core-pdf": ["packages/core-pdf/src/index.ts"],
      "@flowpilot/core-security": ["packages/core-security/src/index.ts"],
      "@flowpilot/core-common": ["packages/core-common/src/index.ts"]
    }
  }
}
```

### 2.5. `.gitignore`
```text
node_modules/
.pnpm-store/
.expo/
dist/
build/
*.log
.DS_Store

# Android & iOS build artifacts (Vì dùng Expo CNG, không commit native dirs)
android/
ios/

# Env & secrets
.env
.env.local
```

---

## 3. QUY CHUẨN PACKAGES NỀN TẢNG (`packages/core-*`)

Mỗi package trong `packages/` bắt buộc phải có:
1. **`package.json` độc lập**:
   * Khai báo `"name": "@flowpilot/core-<slug>"`.
   * Khai báo `"main": "./src/index.ts"`.
   * Khai báo `"types": "./src/index.ts"`.
2. **`tsconfig.json`**:
   * Extends từ `"../../tsconfig.base.json"`.
3. **`src/index.ts`**:
   * Chỉ xuất khẩu các Public Interfaces, Models, Facades.
   * **Tuyệt đối cấm** xuất khẩu các chi tiết nội bộ hoặc import sâu từ ngoài vào.

---

## 4. QUY CHUẨN APP TEMPLATE (`apps/_template`)

App template là khuôn mẫu cho mọi app tương lai trong studio:
* **Tech Baseline:** Expo SDK 51+, `expo-router` v3, `nativewind` v4.
* **`package.json` của app:**
  * Import tất cả 7 package nội bộ:
    ```json
    {
      "name": "@flowpilot/app-template",
      "version": "1.0.0",
      "dependencies": {
        "@flowpilot/core-ui": "workspace:*",
        "@flowpilot/core-storage": "workspace:*",
        "@flowpilot/core-billing": "workspace:*",
        "@flowpilot/core-ads": "workspace:*",
        "@flowpilot/core-pdf": "workspace:*",
        "@flowpilot/core-security": "workspace:*",
        "@flowpilot/core-common": "workspace:*",
        "expo": "~51.0.0",
        "expo-router": "~3.5.0",
        "react": "18.2.0",
        "react-native": "0.74.5",
        "react-native-safe-area-context": "4.10.5",
        "react-native-screens": "~3.31.1"
      }
    }
    ```
* **Composition Root (`app/_layout.tsx`):**
  * Tích hợp sẵn `SafeAreaProvider`, `ErrorBoundary`, và Theme Context.

---

## 5. QUY TRÌNH THỰC THI STEP 0 DÀNH CHO AI (BOOTSTRAP CHECKLIST)

Khi nhận lệnh khởi tạo dự án, AI bắt buộc thực hiện tuần tự 5 bước:

1. **Bước 1: Sinh Root Configs:**
   * Tạo `package.json`, `pnpm-workspace.yaml`, `turbo.json`, `tsconfig.base.json`, `.gitignore`.
2. **Bước 2: Dựng Foundation Packages:**
   * Tạo `packages/core-common`, `packages/core-security`, `packages/core-storage`.
3. **Bước 3: Dựng Service & UI Packages:**
   * Tạo `packages/core-ui`, `packages/core-billing`, `packages/core-ads`, `packages/core-pdf`.
4. **Bước 4: Dựng App Golden Template:**
   * Tạo `apps/_template` với routing và composition root hoàn chỉnh.
5. **Bước 5: Chạy Compiler Gate:**
   * Chạy lệnh: `pnpm tsc --noEmit`
   * Kiểm tra toàn bộ mã nguồn không còn bất kỳ lỗi type hay thiếu module nào.
