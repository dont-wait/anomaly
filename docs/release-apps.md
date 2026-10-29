# Phát hành bộ cài Anomaly

Branch này đóng gói ứng dụng để người dùng tải và cài:

| Ứng dụng | Nền tảng | File tải về |
| --- | --- | --- |
| Khách hàng | Android ARM64 | APK release đã ký |
| Admin Console | Windows x86_64 | NSIS `.exe` |
| Admin Console | Linux x86_64 | `.AppImage` và `.deb` |

Chưa cấu hình ký Authenticode cho Windows; Windows có thể hiện cảnh báo nhà phát hành chưa xác minh. Chưa có auto-update; bản đầu cập nhật bằng tải và cài đè. Bộ cài không kèm server: cần backend có endpoint HTTPS đang hoạt động.

## CI và release chạy lúc nào?

- PR hoặc merge vào `main`: CI FE/BE chạy theo paths filter hiện có; **không release**.
- Đẩy tag `v0.1.0`: `release-apps.yml` gọi lại CI FE/BE trên commit được tag. Cả hai xanh mới build ba nền tảng. Chỉ khi tất cả build thành công mới tạo GitHub Release và đính kèm bộ cài + checksum + manifest. Không build lại khi upload.
- Actions → **Release application installers** → **Run workflow**: chạy cùng checks/build nhưng chỉ lưu Actions artifacts, **không tạo GitHub Release**. Dùng để kiểm tra trước khi phát hành. Bản build từ tag sẽ build lại; chưa triển khai promote artifact từ lần manual sang tag.
- Luồng này chỉ tạo bộ cài app; không build/push image server và không cần `GATV2_REF`.

## Cấu hình một lần trên GitHub

Settings → Secrets and variables → Actions:

**Variables**:

- `VITE_API_ENDPOINT`: URL HTTPS thật của API, ví dụ cấu trúc `https://api.<domain>`. Không dùng localhost. Giá trị này được nhúng vào frontend khi build.
- `ANDROID_VERSION_CODE`: số nguyên dương, tối đa `2100000000`. Tăng trước mỗi bản Android mới được phân phối; giữ nguyên khi retry cùng bản. Đây là số build Android, khác version ứng dụng `0.1.0`. Khi chạy manual có thể nhập riêng.

**Secrets**:

- `ANDROID_KEYSTORE_BASE64`: nội dung file keystore đã mã hóa base64 (một dòng).
- `ANDROID_STORE_PASSWORD`: mật khẩu keystore.
- `ANDROID_KEY_ALIAS`: alias của khóa ký.
- `ANDROID_KEY_PASSWORD`: mật khẩu khóa ký.

Dùng keystore phát hành đã có và lưu bản sao an toàn; các bản cài đè Android phải dùng cùng key. Không commit `.jks`, mật khẩu hoặc chuỗi base64 vào repo. Hướng dẫn tạo/cấu hình key: [Tauri Android signing](https://v2.tauri.app/distribute/sign/android/).

Workflow từ chối thiếu signing inputs. APK được kiểm bằng `apksigner verify --print-certs` trước khi upload. Checksum SHA-256 được tính sau build/ký. Chưa có bằng chứng cài đặt/chạy ứng dụng trên thiết bị chỉ từ việc build xanh; cần tải artifact và kiểm tra trước khi tag.

## Build local

`make release` **chỉ build**, không publish. Chuẩn bị Node/Corepack/Yarn và Rust cùng SDK/libraries của nền tảng; cài frontend dependencies trước. Repo đang dùng lockfile Yarn v1 nên dùng `--frozen-lockfile`, không chuyển lockfile trong bước release.

Android trên NixOS (trong repo root):

```bash
export VITE_API_ENDPOINT='https://<api-domain>'
export ANDROID_KEYSTORE_PATH='/absolute/path/to/release.jks'
export ANDROID_KEY_ALIAS='<alias>'
export ANDROID_VERSION_CODE='<positive-integer>'
# Nạp ANDROID_STORE_PASSWORD và ANDROID_KEY_PASSWORD từ môi trường riêng.
cd client
nix develop .#android
corepack enable
yarn install --frozen-lockfile --production=false
cd ..
make release-check APP_PLATFORM=android
make release APP_PLATFORM=android
```

Script dùng Python 3.11+ (thư viện chuẩn), Node CLI đã cài trong `client/node_modules`, và toolchain từ shell. Android dùng SDK 36, build-tools 35.0.0, NDK 29.0.14206865 và JDK 17. Không cần emulator để build. Không chạy `tauri android init` lại.

Linux AppImage/deb được workflow build trên **Ubuntu 22.04**, để tránh đóng gói thư viện/linker NixOS vào app. Build Linux local trên Ubuntu với các dependencies trong workflow rồi chạy `make release APP_PLATFORM=linux`. Windows dùng runner native Windows MSVC; có thể chạy `python scripts/release/apps.py --platform windows` trong môi trường Windows tương đương.

Output: `dist/installers/<android|linux|windows>/`. Mỗi nền tảng có bộ cài, `SHA256SUMS-<platform>.txt` và `manifest-<platform>.json`. Script từ chối output đã có để tránh trộn các lần build; dùng checkout sạch cho lần mới. Build local từ working tree hiện tại chỉ dùng để kiểm thử; artifact phát hành phải từ workflow checkout commit đã tag.

## Phát hành

1. Đồng bộ version ở `client/package.json`, package.json của mobile/desktop, hai Cargo.toml và hai tauri.conf.json; cập nhật Cargo.lock tương ứng nếu thay version Rust package.
2. Cấu hình endpoint, signing secrets và Android versionCode.
3. Chạy manual workflow; tải artifacts, kiểm cài/chạy/login/kết nối API trên thiết bị tương ứng.
4. Sau khi xác nhận commit muốn phát hành, tạo và đẩy tag `v<version>`. Workflow kiểm version và chạy lại toàn bộ CI/build trước khi publish.

Workflow không tự tạo/push tag, triển khai server hay publish từ lần merge PR. Nếu tag đã có GitHub Release, job tạo release sẽ báo lỗi, không tự ghi đè assets.
