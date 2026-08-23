# Grep Commands Cheatsheet for SOLID Scan

Đây là danh sách các lệnh bash/grep thường dùng trong quá trình khám mã nguồn Android để phát hiện vi phạm SOLID.

## 0. Build & Architecture
```bash
# Đếm số api() dependency (nghi ngờ lộ lọt scope)
grep -r "api(" . --include="*.gradle.kts"

# Tìm Hilt plugin trong Core/Domain
grep -r "dagger.hilt.android.plugin" ./core ./domain --include="*.gradle.kts"

# Tìm cấu hình Room phá huỷ database (Nguy hiểm ở production)
grep -r "fallbackToDestructiveMigration" .
```

## 1. Single Responsibility (SRP)
```bash
# Lọc các file KT lớn hơn 400 dòng
find . -name "*.kt" -exec wc -l {} + | awk '$1 > 400' | sort -nr

# Phân tích import Android trong ViewModel
grep -r "^import android" . | grep "ViewModel.kt"

# Tìm lộ MutableStateFlow
grep -r "val .*: MutableStateFlow" .

# Phát hiện số tham số Constructor quá dài (tìm class)
grep -r "class " . | grep -o -E "\([^)]+\)" | awk '{print length($0), $0}' | sort -nr | head -n 20
```

## 2. Open/Closed (OCP)
```bash
# Tìm chuỗi "when (" khổng lồ
grep -r "when (" . -A 5

# Tìm cờ boolean trong Data layer
grep -r -E "val is[A-Z].*: Boolean" .
```

## 3. Liskov Substitution (LSP)
```bash
# Tìm hàm rỗng hoặc TODO
grep -rn "TODO(" .
grep -rn "NotImplementedError" .

# Tìm nuốt lỗi
grep -rn "catch(.*Exception.*).*{" . -A 2 | grep -v "Log" | grep -v "timber"
```

## 4. Interface Segregation (ISP)
```bash
# Tìm interface khổng lồ
grep -rn "interface " . -A 20
```

## 5. Dependency Inversion (DIP)
```bash
# Tìm framework bên thứ 3 trong domain
grep -rn "import retrofit2" ./domain
grep -rn "import androidx.room" ./domain
grep -rn "import com.google.firebase" ./domain
```
