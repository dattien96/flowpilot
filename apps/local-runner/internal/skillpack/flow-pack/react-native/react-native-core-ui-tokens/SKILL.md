---
name: react-native-core-ui-tokens
description: Quy chuẩn Design System, Tokens và Atomic UI Components cho packages/core-ui trong React Native & Expo (NativeWind v4). Cung cấp các component chuyên dụng cho utility app như GloveButton, MetricGauge, FractionInput, ScreenWrapper.
version: 6
---

# React Native Core UI Tokens & Component Library

Tài liệu quy chuẩn cho package `packages/core-ui` trong kiến trúc Monorepo của FlowPilot Studio. Cung cấp bộ Design Tokens và các Atomic UI Components chuẩn mực, đặc biệt tối ưu cho **ứng dụng công cụ thực chiến (Utility Apps)**, **chế độ găng tay (Glove-friendly)** và **môi trường ngoài trời / thiếu sáng (High-contrast Dark Mode)**.

---

## 1. HỆ THỐNG DESIGN TOKENS (NATIVEWIND V4)

### 1.1. Bảng Màu (Color Tokens)
```typescript
// file: packages/core-ui/src/tokens/colors.ts
export const Colors = {
  // Backgrounds
  background: '#09090b',       // Zinc-950 (Nền chính Dark Mode sang trọng)
  surface: '#18181b',          // Zinc-900 (Nền thẻ Card, Sheet)
  surfaceHighlight: '#27272a', // Zinc-800 (Nền khi hover/pressed)

  // Borders
  border: '#27272a',           // Zinc-800
  borderFocus: '#3f3f46',      // Zinc-700

  // Text
  textPrimary: '#fafafa',      // Zinc-50
  textSecondary: '#a1a1aa',    // Zinc-400
  textMuted: '#71717a',        // Zinc-500

  // Domain Status Colors
  safetyGreen: '#22c55e',      // Green-500 (An toàn, Đã thanh toán, Tải trọng tốt)
  alertYellow: '#eab308',      // Yellow-500 (Cảnh báo, Sắp đầy tải, Chờ duyệt)
  dangerRed: '#ef4444',        // Red-500 (Quá tải, Nguy hiểm, Lỗi)
  accentBlue: '#3b82f6',       // Blue-500 (Action chính, Link)
};
```

### 1.2. Typography & Numbers
* Mọi con số kết quả tính toán, kích thước đo đạc, hay tiền tệ bắt buộc sử dụng font **Monospace** hoặc thuộc tính `fontVariant: ['tabular-nums']` để không bị nhảy layout khi số thay đổi.

---

## 2. BỘ ATOMIC COMPONENTS CHUẨN TRONG `packages/core-ui`

### 2.1. `ScreenWrapper` (Container chuẩn của mọi màn hình)
Tự động bọc Safe Area Insets, nền Zinc tối và Status Bar:
```tsx
import React from 'react';
import { View, Text } from 'react-native';
import { useSafeAreaInsets } from 'react-native-safe-area-context';
import { StatusBar } from 'expo-status-bar';

interface ScreenWrapperProps {
  title?: string;
  children: React.ReactNode;
  rightAction?: React.ReactNode;
}

export function ScreenWrapper({ title, children, rightAction }: ScreenWrapperProps) {
  const insets = useSafeAreaInsets();

  return (
    <View 
      style={{ paddingTop: insets.top, paddingBottom: insets.bottom }}
      className="flex-1 bg-zinc-950 px-4"
    >
      <StatusBar style="light" />
      {title && (
        <View className="flex-row items-center justify-between py-3 mb-2 border-b border-zinc-800">
          <Text className="text-xl font-bold text-zinc-100">{title}</Text>
          {rightAction}
        </View>
      )}
      <View className="flex-1">{children}</View>
    </View>
  );
}
```

### 2.2. `GloveButton` (Nút bấm chuyên dụng cho thợ)
Nút bấm lớn với chiều cao tối thiểu 56dp, padding rộng, và rung haptic khi bấm:
```tsx
import React from 'react';
import { Pressable, Text, ActivityIndicator } from 'react-native';
import * as Haptics from 'expo-haptics';

interface GloveButtonProps {
  title: string;
  onPress: () => void;
  variant?: 'primary' | 'danger' | 'secondary';
  loading?: boolean;
  disabled?: boolean;
}

export function GloveButton({ 
  title, 
  onPress, 
  variant = 'primary', 
  loading, 
  disabled 
}: GloveButtonProps) {
  const handlePress = () => {
    if (disabled || loading) return;
    Haptics.impactAsync(Haptics.ImpactFeedbackStyle.Medium);
    onPress();
  };

  const variantStyles = {
    primary: 'bg-emerald-600 active:bg-emerald-700 text-white',
    danger: 'bg-red-600 active:bg-red-700 text-white',
    secondary: 'bg-zinc-800 active:bg-zinc-700 text-zinc-200',
  }[variant];

  return (
    <Pressable
      onPress={handlePress}
      disabled={disabled || loading}
      className={`min-h-[56px] rounded-xl flex-row items-center justify-center px-6 py-4 my-2 ${variantStyles} ${
        disabled ? 'opacity-50' : ''
      }`}
    >
      {loading ? (
        <ActivityIndicator color="#fff" />
      ) : (
        <Text className="text-lg font-bold text-center tracking-wide">{title}</Text>
      )}
    </Pressable>
  );
}
```

### 2.3. `MetricGauge` (Đồng hồ bán nguyệt đo an toàn / cảnh báo)
Vòng cung hiển thị mức % dung lượng / tải trọng với 3 dải màu Xanh - Vàng - Đỏ:
```tsx
import React from 'react';
import { View, Text } from 'react-native';

interface MetricGaugeProps {
  value: number; // 0 đến 100
  label: string;
  unit?: string;
  warningThreshold?: number; // Mặc định 80%
  dangerThreshold?: number;  // Mặc định 100%
}

export function MetricGauge({ 
  value, 
  label, 
  unit = '%',
  warningThreshold = 80, 
  dangerThreshold = 100 
}: MetricGaugeProps) {
  let statusColor = 'text-emerald-500';
  let barBg = 'bg-emerald-500';

  if (value >= dangerThreshold) {
    statusColor = 'text-red-500';
    barBg = 'bg-red-500';
  } else if (value >= warningThreshold) {
    statusColor = 'text-yellow-500';
    barBg = 'bg-yellow-500';
  }

  const clampedWidth = Math.min(Math.max(value, 0), 100);

  return (
    <View className="bg-zinc-900 border border-zinc-800 rounded-2xl p-5 my-2">
      <View className="flex-row justify-between items-baseline mb-2">
        <Text className="text-sm font-medium text-zinc-400 uppercase tracking-wider">{label}</Text>
        <Text className={`text-2xl font-black font-mono ${statusColor}`}>
          {value.toFixed(1)}{unit}
        </Text>
      </View>
      <View className="h-3 bg-zinc-800 rounded-full overflow-hidden">
        <View style={{ width: `${clampedWidth}%` }} className={`h-full ${barBg}`} />
      </View>
    </View>
  );
}
```

### 2.4. `FractionInput` (Nhập phân số inch: 1/8", 1/4", 3/8", 1/2")
Dành riêng cho app thợ mộc, cơ khí, xây dựng:
```tsx
import React from 'react';
import { View, Text, TouchableOpacity } from 'react-native';

const FRACTIONS = ['0', '1/8', '1/4', '3/8', '1/2', '5/8', '3/4', '7/8'];

interface FractionInputProps {
  selectedFraction: string;
  onSelect: (fraction: string) => void;
}

export function FractionInput({ selectedFraction, onSelect }: FractionInputProps) {
  return (
    <View className="my-2">
      <Text className="text-xs font-semibold text-zinc-400 mb-1">Phân số inch (Fraction):</Text>
      <View className="flex-row flex-wrap gap-2">
        {FRACTIONS.map((f) => (
          <TouchableOpacity
            key={f}
            onPress={() => onSelect(f)}
            className={`px-3 py-2 rounded-lg border ${
              selectedFraction === f
                ? 'bg-emerald-600/20 border-emerald-500'
                : 'bg-zinc-800/60 border-zinc-700'
            }`}
          >
            <Text className={`text-sm font-mono font-medium ${
              selectedFraction === f ? 'text-emerald-400' : 'text-zinc-300'
            }`}>
              {f === '0' ? '0"' : `${f}"`}
            </Text>
          </TouchableOpacity>
        ))}
      </View>
    </View>
  );
}
```

### 2.5. `StatusBadge` (Huy hiệu trạng thái rõ ràng)
```tsx
import React from 'react';
import { View, Text } from 'react-native';

export function StatusBadge({ status }: { status: 'SAFE' | 'WARNING' | 'DANGER' | 'PRO' }) {
  const configs = {
    SAFE: { bg: 'bg-emerald-950', border: 'border-emerald-700', text: 'text-emerald-400', label: 'AN TOÀN' },
    WARNING: { bg: 'bg-yellow-950', border: 'border-yellow-700', text: 'text-yellow-400', label: 'CẢNH BÁO' },
    DANGER: { bg: 'bg-red-950', border: 'border-red-700', text: 'text-red-400', label: 'QUÁ TẢI' },
    PRO: { bg: 'bg-amber-950', border: 'border-amber-600', text: 'text-amber-300', label: 'PRO UNLOCKED' },
  }[status];

  return (
    <View className={`px-2.5 py-1 rounded-md border ${configs.bg} ${configs.border} self-start`}>
      <Text className={`text-xs font-black tracking-wider ${configs.text}`}>{configs.label}</Text>
    </View>
  );
}
```

---

## 3. NGUYÊN TẮC KIỂM TRA SOLID TRONG `core-ui`

1. **Pure UI / Zero Logic:** Không component nào trong `core-ui` được phép gọi database, API, hoặc thư viện lưu trữ.
2. **Prop Segregation:** Props của component phải tường minh, không truyền object thừa.
3. **Accessibility:** Kích thước tương tác tối thiểu phải đạt 48x48dp (chuẩn Apple & Google A11y), chế độ thợ tối thiểu 56x56dp.
