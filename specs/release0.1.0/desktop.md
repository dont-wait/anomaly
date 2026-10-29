# Desktop: đóng gói và phân phối Anomaly Admin 0.1.0

## 1. App và target

App cần phát hành là `client/desktop`, identifier `com.dontwait.anomaly.admin`. `make desktop` hiện chạy consumer workspace mobile trên Linux; không dùng target đó để build admin.

| Target | Runner đề xuất | Rust target | Artifact |
| --- | --- | --- | --- |
| Linux x86_64 | Ubuntu 22.04 builder container đã pin digest | `x86_64-unknown-linux-gnu` | `.AppImage`, `.deb` |
| Windows x86_64 | Windows runner đã pin | `x86_64-pc-windows-msvc` | NSIS installer `.exe` |

Linux release compile và bundle hoàn toàn trong Ubuntu22.04 container trên NixOS/CI, tách cache khỏi host. Dùng native runner Windows cho MSVC installer. Chi tiết isolation, linker, AppImage/FUSE và cache trong [build-environments.md](build-environments.md).

## 2. Cấu hình cần bổ sung trước build

- `client/desktop/src-tauri/tauri.conf.json`: bật `bundle.active`, cấu hình icons, metadata publisher/description/license theo giá trị dự án và target cụ thể thay `all` không kiểm soát.
- Giữ title/productName `Anomaly Admin Console`, version `0.1.0` đồng bộ package/Cargo/Tauri.
- Bổ sung lockfile Rust desktop vào version control nếu chưa track; candidate build phải dùng dependency graph đã lock.
- `VITE_API_ENDPOINT` phải là input bắt buộc ở release script. Desktop hiện fallback localhost nếu thiếu biến; workflow cần fail trước build.
- Khai báo CSP/capabilities phù hợp endpoint release; kiểm bản đóng gói để tránh policy hoạt động ở dev nhưng chặn API ở release.
- Kiểm `beforeBuildCommand` chạy đúng desktop workspace và frontendDist trỏ output desktop. Không build frontend mobile rồi bundle vào admin.

## 3. Build scripts

Input `RELEASE_VERSION`, `RELEASE_CANDIDATE`, `VITE_API_ENDPOINT`, `TARGET_PLATFORM`, `OUTPUT_DIR`; Windows thêm signing inputs khi dùng certificate. Script tự xác định repo/workspace root.

Linux: các lệnh sau chạy **bên trong Ubuntu builder**, từ `client/` của source snapshot, sau setup toolchain/dependencies:

```bash
corepack enable
yarn install --immutable
yarn workspace anomaly-desktop lint
yarn workspace anomaly-desktop typecheck
yarn workspace anomaly-desktop test
yarn workspace anomaly-desktop tauri build --bundles appimage,deb
```

Windows chạy cùng workspace trên MSVC runner, sau cài WebView/build prerequisites:

```powershell
corepack enable
yarn install --immutable
yarn workspace anomaly-desktop tauri build --bundles nsis
```

Các lệnh trên là đường build cần đưa vào CI sau khi config hoàn chỉnh, chưa được chạy nghiệm thu. Native dependencies và runner version cần pin qua một build spike; không dùng `latest` rồi kỳ vọng binary tương thích lâu dài.

Nếu dùng `tauri-action`, đặt `projectPath` vào `client/desktop` và setup Yarn tại root `client/` trước khi gọi action. Tham khảo [Tauri GitHub pipeline](https://v2.tauri.app/distribute/pipelines/github/); action và dependencies phải pin theo revision được kiểm tra.

## 4. Signing và update

| Loại | Phương án 0.1.0 |
| --- | --- |
| Linux packages | SHA-256 và provenance trong GitHub Release; không tuyên bố package repository signing nếu chưa thiết lập repo |
| Windows Authenticode | Cấu hình certificate/provider + timestamp server nếu có; kiểm chữ ký trên file cuối |
| Windows chưa có certificate | Ghi `unsigned` trong manifest/hướng dẫn và chốt kênh nội bộ chấp nhận trước promote; không gọi signed chỉ vì installer build được |
| Tauri updater signature | Chưa bắt buộc vì chưa triển khai updater; khác certificate Windows |

Update đầu tiên: tải package mới và cài đè, giữ identifier và đường dẫn dữ liệu của app. Test NSIS update/uninstall và deb upgrade. AppImage update thay file sau khi đóng process; không xóa user data. Nếu muốn auto-update, phải thêm updater plugin, signing key, endpoint/update manifest và test update riêng trước khi mô tả tính năng đó có sẵn.

## 5. Collect và verify

- Xác định bundle output từ build report; kiểm đủ một artifact mỗi target/format bắt buộc, không pick file từ consumer/mobile hoặc cache cũ.
- Linux: kiểm AppImage/deb architecture, package metadata/version, executable bit và dependencies runtime trên máy sạch.
- Windows: kiểm executable/installer architecture, product/version metadata, certificate chain/timestamp nếu signed; `Get-AuthenticodeSignature` có thể dùng trong verification job.
- Artifact chỉ được rename sau signing; tính checksum sau mọi thay đổi binary.
- Tên phân phối: `anomaly-admin-0.1.0-rc.1-linux-x86_64.AppImage`, tương tự deb/windows-x86_64.exe.
- Cài từ package và mở từ launcher khi không có Node/Rust/Vite, xác nhận HTTPS API endpoint, đóng/mở lại và uninstall/update.
- Upload vào candidate artifact, promote cùng hash và download lại từ GitHub Release để kiểm tra file được phân phối thực tế.

## 6. Gap cần triển khai

- [ ] Bật bundle, thêm icon/metadata và cấu hình targets Linux/Windows.
- [ ] Release endpoint validation cho desktop; giữ variant trong manifest.
- [ ] Desktop Rust lockfile và cache key riêng workspace/OS/target.
- [ ] Linux build baseline/system dependencies và Windows MSVC runner.
- [ ] Signing policy Windows + secret/provider setup nếu ký.
- [ ] Collect/verify scripts kiểm đúng package/version/arch/hash.
- [ ] Test cài sạch, update, gỡ và startup từ launcher trên artifact cuối.
