# Module Dependency Matrix

Tài liệu này định nghĩa chi tiết các quy tắc phụ thuộc (dependency) giữa các module trong hệ thống Superapp. Việc vi phạm các quy tắc này sẽ làm tăng build time, gây ra circular dependency hoặc phá vỡ nguyên tắc ranh giới (boundary principles).

## Ma trận phụ thuộc (Allowed / Forbidden)

Ký hiệu:
- `[V]` = Cho phép (Allowed)
- `[X]` = Bị cấm (Forbidden)
- `[-]` = Không áp dụng (N/A)

| From \ To | `:app` | `:di` | `:feature:*:presentation` | `:feature:*:domain` | `:feature:*:data` | `:feature:*:api` | `:core` | `:foundation` |
|-----------|--------|-------|---------------------------|---------------------|-------------------|------------------|---------|---------------|
| **`:app`**| - | [V] | [V] | [V] | [V] | [V] | [V] | [V] |
| **`:di`** | [X] | - | [V] | [V] | [V] | [V] | [V] | [V] |
| **`:feature:*:presentation`** | [X] | [X] | [X] (1) | [V] | [X] | [V] | [V] | [V] |
| **`:feature:*:domain`** | [X] | [X] | [X] | - | [X] | [V] | [V] (2)| [V] (3) |
| **`:feature:*:data`** | [X] | [X] | [X] | [V] | [X] (1)| [V] | [V] | [V] |
| **`:feature:*:api`** | [X] | [X] | [X] | [X] | [X] | - | [V] (4)| [V] |
| **`:core`** | [X] | [X] | [X] | [X] | [X] | [V] (5) | [V] | [V] |
| **`:foundation`**| [X] | [X] | [X] | [X] | [X] | [X] | [X] | [V] |

### Ghi chú:
(1) Một feature không được phép phụ thuộc trực tiếp vào `presentation` hay `data` của feature khác. Phải thông qua `api`.
(2) Domain layer chỉ nên phụ thuộc vào các core interface, không phụ thuộc vào framework.
(3) Foundation nên giới hạn ở các logic không phụ thuộc platform hoặc các pure data models.
(4) API module nên cực kì nhẹ, ưu tiên phụ thuộc rất ít.
(5) Core đôi khi phải biết tới một vài Feature APIs nếu cung cấp cơ sở hạ tầng cross-feature (như DeepLink Router).

## Cách kiểm tra vi phạm (Grep Commands)

Bạn có thể chạy các lệnh shell sau ở gốc dự án để kiểm tra các vi phạm phổ biến.

### 1. Kiểm tra `:feature` phụ thuộc trực tiếp vào `:feature:*:presentation` của feature khác
```bash
grep -rnw './feature/' -e 'implementation project(":feature:.*:presentation")'
```

### 2. Kiểm tra `:foundation` phụ thuộc vào `:core`
```bash
grep -rnw './foundation/' -e 'implementation project(":core'
```

### 3. Kiểm tra `:domain` phụ thuộc vào `:data`
```bash
grep -rnw './feature/' -e 'implementation project(":feature:.*:data")' | grep 'domain/build.gradle'
```

### 4. Kiểm tra `:foundation` sử dụng Hilt (Dagger)
```bash
grep -rnw './foundation/' -e 'dagger.hilt'
grep -rnw './foundation/' -e '@Inject'
```

## DiBoundaryPolicy Plugin (Tuỳ chọn)

Khuyến khích tích hợp một custom Gradle Plugin (`DiBoundaryPolicyPlugin`) để quét Dependency Graph trong lúc `afterEvaluate` hoặc thông qua task `checkDependencies` trên CI. Nếu biểu đồ phụ thuộc vi phạm các rules trên, build sẽ văng lỗi ngay lập tức.
