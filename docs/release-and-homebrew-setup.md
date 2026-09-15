# Hướng Dẫn Cấu Hình Token & Tự Động Hóa Phát Hành (CLI & Desktop App)

Tài liệu này hướng dẫn chi tiết từng bước để cấu hình tự động hóa toàn bộ quy trình phát hành FlowPilot CLI (GoReleaser + Homebrew Tap) và FlowPilot Desktop App (Electron + NSIS/.exe/.dmg) cho **macOS, Linux và Windows** thông qua GitHub Actions.

---

## 🏛️ Toàn Cảnh Quy Trình Phát Hành

```text
               ┌──────────────────────────────────────┐
               │ Bạn đẩy tag lên Git:                 │
               │ git tag v0.1.0 && git push origin... │
               └──────────────────┬───────────────────┘
                                  │
                                  ▼
               ┌──────────────────────────────────────┐
               │ GitHub Actions (release-cli.yml)     │
               │ Kích hoạt GoReleaser                 │
               └──────────────────┬───────────────────┘
                                  │
        ┌─────────────────────────┴─────────────────────────┐
        ▼                                                   ▼
┌───────────────────────────────┐           ┌───────────────────────────────┐
│ 1. GitHub Releases (Storage)  │           │ 2. Homebrew Tap (Formula)     │
│ Repo: dattien96/flowpilot     │           │ Repo: dattien96/homebrew-flowpilot │
├───────────────────────────────┤           ├───────────────────────────────┤
│ • flowpilot_darwin_arm64.tar  │           │ Tự động commit file:          │
│ • flowpilot_darwin_amd64.tar  │           │ Formula/flowpilot.rb          │
│ • flowpilot_linux_amd64.tar   │           │                               │
│ • flowpilot_windows_amd64.zip │           │ Phục vụ lệnh:                 │
│ • checksums.txt (SHA-256)     │           │ brew install dattien96/flowpilot│
└───────────────────────────────┘           └───────────────────────────────┘
```

---

## 📋 Các Bước Thiết Lập Cần Làm (Chỉ cần làm 1 lần duy nhất)

### Bước 1: Tạo Repository `homebrew-flowpilot` trên GitHub

Homebrew cho phép đặt tên repo theo dạng `homebrew-<tên_tool>` để người dùng có thể gõ lệnh cài đặt ngắn gọn và đẹp nhất:

1. Đăng nhập vào tài khoản GitHub của bạn (`dattien96`).
2. Nhấn vào dấu **`+`** ở góc trên cùng bên phải $\to$ chọn **New repository**.
3. Điền các thông tin:
   - **Repository name**: `homebrew-flowpilot` (khuyên dùng để có lệnh `brew install dattien96/flowpilot`).
   - **Visibility**: Chọn **Public** (bắt buộc Public để Homebrew đọc được).
   - **Initialize this repository with**: Tick chọn **Add a README file**.
4. Nhấn **Create repository**.

---

### Bước 2: Tạo GitHub Personal Access Token (PAT)

Mặc định, GitHub Actions trong repo `flowpilot` chỉ có quyền thao tác trên chính repo `flowpilot`, không thể tự ý ghi code sang repo `homebrew-flowpilot`. Bạn cần tạo một Token để cấp quyền này cho GoReleaser:

1. Bấm vào Avatar góc trên cùng bên phải $\to$ chọn **Settings**.
2. Kéo thanh cuộn xuống góc dưới cùng bên trái, chọn **Developer settings**.
3. Chọn **Personal access tokens** $\to$ **Tokens (classic)**.
   - *Link truy cập trực tiếp*: [https://github.com/settings/tokens/new](https://github.com/settings/tokens/new)
4. Nhấn nút **Generate new token** $\to$ chọn **Generate new token (classic)**.
5. Điền thông tin token:
   - **Note**: `FlowPilot GoReleaser Homebrew Token`
   - **Expiration**: Chọn thời hạn (khuyên dùng `No expiration` hoặc `90 days`).
   - **Select scopes** (Phân quyền): Tick chọn ô **`repo`** (Full control of private repositories / public repositories).
6. Kéo xuống cuối trang, nhấn **Generate token**.
7. **QUAN TRỌNG**: Copy ngay chuỗi token (dạng `ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxx`). *GitHub chỉ hiện chuỗi này một lần duy nhất!*

---

### Bước 3: Thêm Token vào Repository Secret của `flowpilot`

1. Truy cập vào repository chính: [https://github.com/dattien96/flowpilot](https://github.com/dattien96/flowpilot)
2. Bấm vào tab **Settings** của repository.
3. Ở menu bên trái, chọn **Secrets and variables** $\to$ **Actions**.
4. Trong mục *Repository secrets*, nhấn nút **New repository secret**.
5. Điền chính xác thông tin:
   - **Name**: `HOMEBREW_TAP_GITHUB_TOKEN` (bắt buộc đúng tên này).
   - **Secret**: Dán chuỗi token `ghp_...` đã copy ở Bước 2 vào.
6. Nhấn **Add secret**.

---

### Bước 4: Cấp Quyền Ghi cho GitHub Actions (Workflow Permissions)

Để GitHub Actions được phép tự động tạo bản Release và upload file binary lên trang GitHub Releases:

1. Vẫn ở trang **Settings** của repo `flowpilot`.
2. Ở menu bên trái, chọn **Actions** $\to$ **General**.
3. Kéo xuống mục **Workflow permissions**:
   - Chọn ô: **Read and write permissions** (Cho phép đọc và ghi).
   - Tick chọn: **Allow GitHub Actions to create and approve pull requests**.
4. Nhấn nút **Save**.

---

## 🚀 Kích Hoạt Bản Release Đầu Tiên

Sau khi hoàn tất 4 bước cấu hình trên, bạn có thể phát hành phiên bản mới bất kỳ lúc nào chỉ bằng lệnh Git từ máy của bạn:

```bash
# 1. Đảm bảo nhánh hiện tại đã commit sạch
git status

# 2. Tạo một release tag (theo chuẩn SemVer: vMAJOR.MINOR.PATCH)
git tag v0.1.0

# 3. Đẩy tag lên GitHub
git push origin v0.1.0
```

### Quá trình tự động diễn ra:
1. GitHub Actions sẽ phát hiện tag `v0.1.0` và chạy workflow `release-cli`.
2. Trong khoảng **1 - 2 phút**, GoReleaser sẽ tự động:
   - Biên dịch binary cho macOS (M1/M2/M3 & Intel), Linux, Windows.
   - Tạo mục Release `v0.1.0` tại [https://github.com/dattien96/flowpilot/releases](https://github.com/dattien96/flowpilot/releases).
   - Commit công thức `Formula/flowpilot.rb` vào repo `homebrew-flowpilot`.

---

## 🧪 Cách Kiểm Tra Sau Khi Release Hoàn Tất

### 1. Kiểm tra trên macOS / Linux qua Homebrew:
```bash
brew install dattien96/flowpilot
flowpilot --version
flowpilot chat .
```

Để cập nhật khi có phiên bản mới sau này:
```bash
brew update && brew upgrade flowpilot
```

---

### 2. Kiểm tra trên macOS / Linux qua Script 1 dòng (không cần Homebrew):
```bash
curl -fsSL https://raw.githubusercontent.com/dattien96/flowpilot/main/scripts/install.sh | bash
```

---

### 3. Kiểm tra trên Windows qua PowerShell:
Mở PowerShell và chạy:
```powershell
irm https://raw.githubusercontent.com/dattien96/flowpilot/main/scripts/install.ps1 | iex
```
Script sẽ tự tải bản `windows_amd64.zip`, giải nén `flowpilot.exe` vào `$HOME\.flowpilot\bin` và tự thêm vào PATH của Windows.

---

## 🖥️ Phát Hành Bộ Cài Desktop App (FlowPilot Setup.exe, macOS .dmg, Linux .AppImage)

Bên cạnh bản CLI (`flowpilot.exe`), dự án FlowPilot có ứng dụng giao diện đồ họa **Desktop App** (Electron + React) tại thư mục `apps/desktop-flowpilot`. Quy trình build và đóng gói bộ cài đã được cấu hình tự động 100% qua GitHub Actions (`.github/workflows/desktop-release.yml`).

### 1. Cách Kích Hoạt Bản Release Desktop
Khác với CLI kích hoạt bằng tag `v*` (ví dụ `v0.1.1`), Desktop App được kích hoạt khi đẩy tag có tiền tố **`desktop-v*`**:

```bash
# 1. Đảm bảo nhánh hiện tại đã commit sạch
git status

# 2. Tạo một tag riêng cho Desktop (theo chuẩn desktop-vMAJOR.MINOR.PATCH)
git tag desktop-v0.1.0

# 3. Đẩy tag lên GitHub
git push origin desktop-v0.1.0
```

### 2. Quá trình tự động diễn ra:
GitHub Actions (`desktop-release`) sẽ chạy song song trên 3 hệ điều hành (matrix: `windows-latest`, `macos-latest`, `ubuntu-latest`), sử dụng `electron-builder` để tạo ra các bộ cài:
- **Windows**: File cài đặt NSIS `FlowPilot Setup <version>.exe`
- **macOS**: File đĩa ảo `.dmg` và `.zip` (cho cả Apple Silicon & Intel)
- **Linux**: File thực thi độc lập `.AppImage` và gói `.deb`

Các file cài đặt này sẽ được tự động đưa lên trang **GitHub Releases** dưới tên release `desktop-v0.1.0`.

### 3. Lưu ý về Code Signing (Chữ ký số):
- **Windows**: Nếu có cấu hình Secret `WIN_CSC_LINK` & `WIN_CSC_KEY_PASSWORD`, file `.exe` sẽ được ký số. Nếu chưa cấu hình, GitHub Actions vẫn tạo file cài đặt bình thường (unsigned) — khi cài trên Windows chỉ cần bấm **More info** $\to$ **Run anyway** trên màn hình Microsoft Defender SmartScreen.
- **macOS**: Để tránh cảnh báo của Apple Gatekeeper khi phân phối rộng rãi, có thể cấu hình các secret notarization (`APPLE_ID`, `APPLE_APP_SPECIFIC_PASSWORD`, `APPLE_TEAM_ID`, `CSC_LINK`, `CSC_KEY_PASSWORD`). Không có secret này, build macOS vẫn sinh file `.dmg` unsigned để dùng thử nghiệm nội bộ.

---

## 🛠️ Xử Lý Sự Cố Thường Gặp (Troubleshooting)

| Lỗi gặp phải | Nguyên nhân | Cách khắc phục |
| :--- | :--- | :--- |
| `Resource not accessible by integration` | GitHub Actions chưa được cấp quyền ghi | Làm lại **Bước 4**: Vào *Settings* $\to$ *Actions* $\to$ *General* $\to$ chọn **Read and write permissions**. |
| `Repository not found: dattien96/homebrew-flowpilot` | Chưa tạo repo `homebrew-flowpilot` hoặc để Private | Làm lại **Bước 1**: Đảm bảo repo tên là `homebrew-flowpilot` và để chế độ **Public**. |
| `Bad credentials / 401 Unauthorized` | Token `HOMEBREW_TAP_GITHUB_TOKEN` bị sai hoặc hết hạn | Làm lại **Bước 2 & 3**: Tạo lại Personal Access Token mới và cập nhật lại vào Secret. |
| Tag bị lỗi cần tạo lại | Đã lỡ push tag bị lỗi và muốn chạy lại | Xóa tag cũ cục bộ và trên remote: `git tag -d v0.1.0 && git push origin :refs/tags/v0.1.0`, sau đó tạo lại tag mới. |
