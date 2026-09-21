---
name: react-native-mobile-plumbing
description: Quy chuẩn triển khai 9 trụ cột cốt lõi (Plumbing) cho ứng dụng React Native / Expo: Splash, Navigation, Safe Area, MMKV + SQLite Local-First, Offline NetInfo, Keyboard, Billing, Ads, và Error Boundary.
version: 6
---

# React Native Mobile Plumbing: 9 Trụ Cột Nền Tảng Của Một Ứng Dụng Chuẩn Store

Tài liệu cung cấp implementation mẫu và quy chuẩn bắt buộc cho 9 thành phần hạ tầng (Plumbing) của mọi ứng dụng React Native (Expo) trong studio. Mọi ứng dụng được sinh ra từ FlowPilot phải đảm bảo 9 trụ cột này hoạt động ổn định và không phát sinh lỗi vặt.

---

## 1. SPLASH SCREEN & ASSET PRELOADING

### Mục tiêu:
Giữ Splash Screen hiển thị trong lúc đọc dữ liệu cache từ MMKV và nạp fonts, sau đó ẩn êm dịu, không nhấp nháy trắng màn hình.

### Implementation:
```tsx
// file: apps/_template/app/_layout.tsx
import * as SplashScreen from 'expo-splash-screen';
import { useEffect, useState } from 'react';

SplashScreen.preventAutoHideAsync();

export default function RootLayout() {
  const [appIsReady, setAppIsReady] = useState(false);

  useEffect(() => {
    async function prepare() {
      try {
        // Nạp font, khởi tạo MMKV và đồng bộ trạng thái ban đầu (< 150ms)
        await Promise.all([
          // Font.loadAsync(...),
        ]);
      } catch (e) {
        console.warn('Lỗi chuẩn bị app:', e);
      } finally {
        setAppIsReady(true);
      }
    }
    prepare();
  }, []);

  useEffect(() => {
    if (appIsReady) {
      SplashScreen.hideAsync();
    }
  }, [appIsReady]);

  if (!appIsReady) return null;

  return <RootNavigation />;
}
```

---

## 2. NAVIGATION & SCREEN ROUTING (EXPO ROUTER V3)

### Cấu trúc file-based routing:
* `app/_layout.tsx`: Root Stack chứa các màn hình chính và modal.
* `app/(tabs)/_layout.tsx`: Bottom Tab Bar.
* `app/(tabs)/index.tsx`: Màn hình Home (Tab 1).
* `app/(tabs)/history.tsx`: Màn hình Lịch sử / Vault (Tab 2).
* `app/(tabs)/settings.tsx`: Màn hình Cài đặt (Tab 3).
* `app/modal.tsx`: Modal màn hình phụ (ví dụ: Paywall hoặc Disclaimer).

### Root Stack mẫu:
```tsx
import { Stack } from 'expo-router';

export function RootNavigation() {
  return (
    <Stack screenOptions={{ headerShown: false }}>
      <Stack.Screen name="(tabs)" />
      <Stack.Screen 
        name="modal" 
        options={{ 
          presentation: 'modal', 
          headerShown: true,
          title: 'Nâng cấp Pro' 
        }} 
      />
    </Stack>
  );
}
```

---

## 3. SAFE AREA & DYNAMIC STATUS BAR

### Quy tắc cứng:
* Không bao giờ dùng `SafeAreaView` từ package `react-native` cũ. Luôn dùng từ `react-native-safe-area-context`.
* Bọc `SafeAreaProvider` ở ngoài cùng Root Layout.
* Các màn hình dùng `useSafeAreaInsets()` để padding chính xác theo Dynamic Island / Tai thỏ và vạch Home indicator.

```tsx
import { StatusBar } from 'expo-status-bar';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { View } from 'react-native';

export function ScreenContainer({ children }: { children: React.ReactNode }) {
  const insets = useSafeAreaInsets();

  return (
    <View 
      style={{
        flex: 1,
        paddingTop: insets.top,
        paddingBottom: insets.bottom,
        paddingLeft: insets.left,
        paddingRight: insets.right,
      }}
      className="bg-zinc-900"
    >
      <StatusBar style="light" />
      {children}
    </View>
  );
}
```

---

## 4. LOCAL STORAGE ENGINE (MMKV + SQLITE LOCAL-FIRST)

### 4.1. Fast Key-Value: MMKV (`packages/core-storage/src/kv`)
Dùng để đọc/ghi cấu hình, trạng thái `isPro`, onboarded token (đọc đồng bộ 0ms):
```typescript
import { MMKV } from 'react-native-mmkv';

export const appStorage = new MMKV({ id: 'flowpilot-storage' });

export const StorageService = {
  getBoolean: (key: string, defaultValue = false): boolean => {
    return appStorage.getBoolean(key) ?? defaultValue;
  },
  setBoolean: (key: string, value: boolean): void => {
    appStorage.set(key, value);
  },
  getString: (key: string): string | undefined => {
    return appStorage.getString(key);
  },
  setString: (key: string, value: string): void => {
    appStorage.set(key, value);
  }
};
```

### 4.2. Relational SQLite Repository (`packages/core-storage/src/db`)
Dùng `expo-sqlite` lưu trữ quan hệ (Báo giá, Sơ đồ, Xe kéo):
```typescript
export interface IBaseRepository<T> {
  findById(id: string): Promise<T | null>;
  findAll(): Promise<T[]>;
  save(entity: T): Promise<void>;
  delete(id: string): Promise<void>;
}
```

---

## 5. NETWORK & GRACEFUL OFFLINE RESILIENCE

### Quy tắc $0 Server Cost:
App phải hoạt động bình thường khi không có Internet. Network chỉ dùng cho RevenueCat & AdMob.

```typescript
import NetInfo from '@react-native-community/netinfo';

export const NetworkWatcher = {
  async isOnline(): Promise<boolean> {
    const state = await NetInfo.fetch();
    return !!state.isConnected && !!state.isInternetReachable;
  }
};
```
* **Khi Offline:** Bỏ qua hoàn toàn việc tải quảng cáo (AdMob), không làm đơ ứng dụng. Sử dụng cache quyền Pro từ MMKV.

---

## 6. KEYBOARD & SCREEN LAYOUT (CHỐNG CHE NÚT SUBMIT)

```tsx
import { KeyboardAvoidingView, Platform, ScrollView } from 'react-native';

export function FormScrollWrapper({ children }: { children: React.ReactNode }) {
  return (
    <KeyboardAvoidingView
      behavior={Platform.OS === 'ios' ? 'padding' : 'height'}
      style={{ flex: 1 }}
      keyboardVerticalOffset={Platform.OS === 'ios' ? 64 : 0}
    >
      <ScrollView 
        keyboardShouldPersistTaps="handled"
        contentContainerStyle={{ flexGrow: 1, padding: 16 }}
      >
        {children}
      </ScrollView>
    </KeyboardAvoidingView>
  );
}
```

---

## 7. MONETIZATION GATE (REVENUECAT IAP & PAYWALL)

* Trừu tượng hóa hoàn toàn qua interface `IBillingService`.
* Trạng thái `isPro` được cache trong Zustand Store và đồng bộ vào MMKV.
* Hook `useIsPro()` trả về boolean tức thì từ RAM (0ms).

```typescript
export function useIsPro(): boolean {
  return useBillingStore((state) => state.isPro);
}
```

---

## 8. ADS COORDINATOR (ADMOB & NATURAL BREAKPOINTS)

* **Chỉ hiện tại Natural Breakpoint:** Sau khi người dùng hoàn thành 1 tác vụ lớn (Lưu PDF, Nén xong video).
* **Frequency Capping:** Giãn cách tối thiểu 3-5 phút giữa 2 lần hiển thị quảng cáo toàn màn hình.
* **Auto Pro Bypass:** Nếu `isPro === true`, bỏ qua 100% quảng cáo.

```typescript
export async function triggerNaturalBreakpoint(actionName: string): Promise<void> {
  if (useBillingStore.getState().isPro) return;
  if (!FrequencyCapper.canShow()) return;

  await AdManager.showInterstitial();
  FrequencyCapper.recordImpression();
}
```

---

## 9. ERROR BOUNDARY & SAFEMATH (CHỐNG SẬP APP)

### 9.1. Global Error Boundary
Bọc toàn bộ app bằng `react-error-boundary` để hiển thị màn hình báo lỗi lịch sự kèm nút restart thay vì bị crash văng ra ngoài màn hình chính.

### 9.2. SafeMath Helpers (`packages/core-security/src/math/SafeMath.ts`)
Khử hoàn toàn `NaN`, `Infinity`, và chia cho 0:
```typescript
export const SafeMath = {
  safeDivide: (numerator: number, denominator: number, fallback = 0): number => {
    if (denominator === 0 || isNaN(denominator) || isNaN(numerator)) {
      return fallback;
    }
    const result = numerator / denominator;
    return isFinite(result) ? result : fallback;
  },
  clamp: (value: number, min: number, max: number): number => {
    if (isNaN(value)) return min;
    return Math.min(Math.max(value, min), max);
  }
};
```
