# Môi trường build: NixOS host, Ubuntu containers và native runners

## 1. Quyết định cho release 0.1.0

Dùng NixOS làm máy phát triển và Docker host. Build phát hành Linux desktop và Android bên trong **hai builder image riêng**, không lấy toolchain/linker/lib từ Nix shell của host. CI dùng lại builder image digest như local để giảm khác biệt môi trường.

| Target | Môi trường build đề xuất | Toolchain đặc thù | Môi trường kiểm tra artifact |
| --- | --- | --- | --- |
| Admin Linux x86_64 | Ubuntu 22.04 container, image pin digest | Rust GNU + GCC/pkg-config + GTK/WebKitGTK + linuxdeploy/AppImage tooling | Ubuntu baseline sạch và distro hỗ trợ; NixOS test thêm theo cách chạy phù hợp |
| Android ARM64 APK | Ubuntu container riêng cho Android | Node/Yarn/Rust Android target, JDK17, SDK36/build-tools35, NDK đã pin | Điện thoại thật/emulator trên host hoặc máy test riêng |
| Admin Windows x86_64 | Windows runner hoặc Windows VM | MSVC/Windows SDK/WebView tooling/NSIS | Windows sạch |
| Go server | Docker multi-stage hiện có | Go builder → Alpine runtime, CGO_ENABLED=0 hiện tại | Image runtime Linux đúng architecture |
| KYC | Dockerfile Python slim riêng | Python3.11 + CV/FFmpeg/system libs đúng image | Chính image runtime, model/config mount |
| ML CPU | Dockerfile Python slim riêng | Python3.12 + CPU torch + pinned GATv2 source/bundle | Chính image runtime với dependencies thật |
| macOS/iOS nếu bổ sung | macOS runner/máy Mac | Xcode/Apple SDK/signing | Thiết bị/OS Apple tương ứng |

Ubuntu là **builder** cho client Linux/Android; không cần đổi mọi runtime server thành Ubuntu. Container Linux không cung cấp Windows hoặc Apple SDK tự động.

## 2. Vì sao Ubuntu container phù hợp trên NixOS

Nix shell có thể build được Rust binary nhưng công cụ đóng gói prebuilt như `linuxdeploy` thường giả định filesystem/linker/library layout kiểu FHS. Đây là vấn đề khác với compile Rust thành công. Builder Ubuntu cung cấp userland và toolchain của Ubuntu, tách khỏi đường dẫn `/nix/store` của host.

Tauri khuyến nghị Linux build trên baseline cũ nhất trong phạm vi hỗ trợ có WebKitGTK4.1, nêu Ubuntu22.04/Debian12 và Docker/CI là các lựa chọn. Build trên distro mới hơn có thể làm tăng glibc minimum của binary. Chọn Ubuntu22.04 cho builder đầu tiên rồi kiểm tra dependencies thực tế thay vì dùng `ubuntu:latest`. [Nguồn Tauri AppImage](https://v2.tauri.app/distribute/appimage/).

Container giảm lỗi do trộn môi trường Nix/FHS; **không đảm bảo không còn linker error**. Thiếu package dev, sai Rust target/ABI, native crate hoặc SDK không khớp vẫn có thể làm build lỗi. Container cũng không chứng minh app chạy trên mọi Linux.

## 3. Hai builder images cần tạo

### Desktop builder

Đề xuất file `deploy/builders/desktop-linux.Dockerfile`:

- Base Ubuntu22.04 theo digest, platform linux/amd64.
- Native build packages: GCC/build-essential, pkg-config, SSL, GTK/WebKitGTK4.1, librsvg và packaging helpers đúng Tauri dependency set.
- Node/Yarn/Rust pin theo release toolchain, không sử dụng binaries bind-mount từ `/nix/store`.
- AppImage tools được pin/ghi nhận version; build headless, không đòi desktop session chỉ để compile/bundle.
- Entrypoint gọi build script trong repo để install immutable, compile đúng workspace admin, bundle, collect/verify rồi export artifacts.

Package list tối thiểu được đối chiếu từ [Tauri prerequisites](https://v2.tauri.app/start/prerequisites/); builder spike cần chốt thêm tool đóng gói theo target, không giả định chỉ có GCC là đủ.

### Android builder

Đề xuất file `deploy/builders/android.Dockerfile`:

- Ubuntu base được pin, Node/Yarn, Rust + `aarch64-linux-android`, JDK17 và SDK command-line tools.
- Theo checkout hiện tại: compile/target SDK36, build-tools35.0.0, NDK29.0.14206865 từ `client/flake.nix`; giữ các bản này nhất quán khi chuyển sang Docker.
- SDK licenses được xử lý trong setup builder có chủ ý; không để build treo ở prompt tương tác.
- Signing key được cấp lúc chạy candidate, không bake vào image hoặc layer/cache.
- Build APK không cần emulator, USB hoặc quyền privileged. Chạy/kiểm camera/app dùng host hoặc máy test riêng.

Không bắt buộc tải system image/emulator vào Android builder nếu job chỉ đóng gói APK. Nix `.#android` vẫn dùng được cho phát triển/chạy thiết bị, không phải môi trường build release mặc định sau khi có Docker builder.

## 4. Cách chạy local và CI

Cần tạo wrapper script `build-desktop`/`build-mobile` theo interface của specs. Từ NixOS, wrapper gọi `docker run` với builder **digest** và môi trường input; local và CI dùng cùng wrapper/image.

Luồng một run:

1. Resolve release SHA, tạo source snapshot sạch từ tracked files tại SHA đó; thêm submodule/external source được pin nếu cần.
2. Mount snapshot read-only vào container, copy sang working volume Linux sạch. Không mang theo `.env`, `node_modules`, `target`, `.gradle`, `local.properties` hoặc build output của host.
3. Cấp endpoint/version/candidate và signing inputs cần thiết; không forward toàn bộ environment của `nix develop`.
4. Install/build/sign/collect trong container bằng toolchain image; export chỉ artifact/report sang output directory của host.
5. Verify checksum/metadata và ghi builder digest/toolchain/platform vào manifest.

Các file Dockerfile/wrapper nói trên **chưa tồn tại**; đây là contract cần triển khai, chưa phải command đã chạy đạt.

Không mount host `/usr`, `/lib`, `/nix/store`, `$HOME` toàn bộ, SSH directory hoặc Docker socket vào client builder. Client builder không cần tự điều khiển Docker host. Giữ user/output UID/GID phù hợp để không sinh artifact root-owned trên workspace.

## 5. Cache và tránh nhiễm linker

Tách named volumes/cache namespaces ít nhất theo builder digest, OS/arch, toolchain, target và lockfile:

- Rust `target` Nix ≠ Rust `target` Ubuntu ≠ Rust `target` Android; không dùng chung directory.
- Host `node_modules` có thể chứa native binary, symlink hoặc postinstall output của môi trường khác; install lại trong container.
- Cargo registry/download cache có thể reuse có kiểm soát, nhưng build artifact cache cần đúng target/toolchain/system libraries.
- Gradle/SDK/Cargo/Yarn cache không được chứa key/properties signing. Không reuse `local.properties` trỏ SDK của Nix host.
- Không forward `NIX_*`, `LD_LIBRARY_PATH`, `LIBRARY_PATH`, `PKG_CONFIG_PATH`, `CC`, `CXX`, `RUSTFLAGS` từ host. Container tự đặt các biến cần thiết bằng đường dẫn bên trong image.
- Không chạy `nix develop` bên trong Ubuntu builder nếu mục đích là build bằng Ubuntu libraries.

Thay builder digest hoặc native dependencies phải đổi cache namespace; khi nghi nhiễm environment, tái hiện một lần với cache build sạch trước khi kết luận source lỗi.

## 6. AppImage trong Docker và runtime tests

AppImage helper có thể cần FUSE khi tự chạy bên trong container. Ưu tiên extraction mode được tool/version hỗ trợ hoặc giải nén/chạy helper theo cách đã thử; không mặc định cấp `--privileged` chỉ để bundle. Nếu thật sự cần FUSE, ghi cụ thể device/capability và lý do sau builder spike.

Build không cần chuyển X11/Wayland socket vào container. GUI/runtime test là bước khác, trên máy/VM có display và runtime libraries phù hợp. NixOS có thể cần wrapper như appimage-run để chạy AppImage; việc build trong Ubuntu không làm NixOS trở thành hệ FHS.

Verify ELF executable/shared objects bên trong artifact bằng `readelf`/tool tương đương: interpreter, RPATH/RUNPATH và native dependency resolution không phụ thuộc `/nix/store` của máy build. Kiểm glibc requirement và runtime trên baseline thật; không chỉ quét chuỗi `/nix/store` rồi coi portable.

## 7. Windows và Apple

Windows release baseline dùng Windows runner/VM, target MSVC và NSIS. Tauri có hướng cross-compile NSIS từ Linux nhưng ít được kiểm thử hơn, có caveat signing/toolchain; giữ như phương án phụ nếu sau này cần. MSI cần Windows theo [Tauri Windows Installer](https://v2.tauri.app/distribute/windows-installer/).

Nếu bổ sung iOS/macOS, cần macOS/Xcode runner và signing riêng. Ubuntu container trên NixOS không thay thế toolchain Apple cho pipeline này.

## 8. Gap và tiêu chí hoàn tất

- [ ] Hai builder Dockerfiles + image publishing/pin digest; setup toolchain và licensing inputs đúng.
- [ ] Wrapper local/CI tạo source snapshot sạch, cache riêng, output UID/GID đúng.
- [ ] Android SDK/NDK/signing và desktop native libraries không kéo đường dẫn Nix host.
- [ ] AppImage helper hoạt động trong container không cần privileged theo mặc định.
- [ ] Linux desktop compile **và** bundle hoàn toàn trong container; inspect ELF và chạy trên baseline sạch đạt.
- [ ] APK build trong container, cài/update và chạy trên thiết bị ngoài container đạt.
- [ ] Local NixOS host và CI dùng cùng builder digest cho cùng target; build report lưu đủ môi trường.

Chưa build thử builder/container trong lượt viết spec; không đánh dấu linker/packaging đã được xác nhận hết lỗi.
