# Mobile: build, ký và phân phối Android 0.1.0

## 1. Hiện trạng release tooling

Source app: `client/mobile`; Tauri config: `client/mobile/src-tauri/tauri.conf.json`; Android project đã được track tại `client/mobile/src-tauri/gen/android`.

| Mục | Hiện tại | Yêu cầu đóng gói |
| --- | --- | --- |
| Version | package/Cargo/Tauri đều 0.1.0 | Check tự động bằng release version |
| Application ID | `com.dontwait.client` | Chốt trước bản đầu, giữ ổn định cho update |
| Tên app | Tauri `productName=client` | Sửa tên/icon/splash nếu muốn brand Anomaly trước build |
| SDK | Gradle minSdk 24, target/compile 36, build tools 35.0.0 | Pin SDK/NDK/JDK theo build đã kiểm tra; không tự nâng trong release job |
| Gradle release | Minify bật, chưa có signingConfigs release | Bổ sung signing release và fail khi thiếu key |
| Endpoint | `VITE_API_ENDPOINT`; mobile PROD thiếu biến sẽ throw lúc chạy module | Workflow phải validate trước build, không chờ app crash |
| Toolchain | Có Nix `.#android`, JDK17/Android/Rust | Đồng bộ runner với phiên bản được lock trong flake |

## 2. Chuẩn bị một lần

1. Chốt identifier/tên/icon; giữ nguyên namespace/applicationId/manifest/Kotlin path nhất quán nếu đổi ID trước phát hành.
2. Tạo keystore release bằng `keytool` trên máy được kiểm soát; key phải có backup và fingerprint được lưu riêng. Không tạo lại key mỗi run.
3. Thêm Gradle `signingConfigs.release`, gán cho `buildTypes.release`; đọc key path/alias/password từ cấu hình sinh tạm, không hard-code.
4. Keystore và file properties không được track hoặc đưa vào cache/artifact. CI ghi chúng vào vùng tạm, cleanup cả khi build fail.
5. Xác định chính sách versionCode tăng theo mỗi candidate; cơ chế đề xuất là counter được lưu bền, giữ nguyên khi rerun cùng candidate và không lùi với candidate mới. Không dùng số job attempt không ổn định.

Tauri có hướng dẫn dùng `keystore.properties` và cấu hình Gradle ký release; triển khai theo [Android Code Signing](https://v2.tauri.app/distribute/sign/android/). Signing APK khác updater signature của Tauri desktop.

## 3. Input/output script cần tạo

`build-mobile` chạy từ root repo hoặc tự resolve root; không phụ thuộc cwd của người gọi.

| Input | Nguồn | Bắt buộc |
| --- | --- | --- |
| `RELEASE_VERSION` | Candidate metadata | `0.1.0`, bằng source version |
| `RELEASE_CANDIDATE` | Workflow | Ví dụ `rc.1` |
| `ANDROID_VERSION_CODE` | Counter được cấp cho candidate | Số nguyên tăng, kiểm trên APK cuối |
| `VITE_API_ENDPOINT` | GitHub Environment variable | HTTPS domain cuối, cấm localhost/loopback |
| Keystore/alias/password | Protected secrets | Không rỗng, fingerprint khớp key đã đăng ký |
| `OUTPUT_DIR` | Runner temp/artifact dir | Chỉ nhận file phát hành và report |

Output: một APK ARM64 đã ký, metadata JSON, checksum và build report. Nếu thiếu key/endpoint, version lệch, build fail hoặc tìm thấy nhiều APK không phân biệt được variant thì script exit khác 0; không upload file đầu tiên tìm được.

## 4. Trình tự build

Release mặc định chạy trong Ubuntu Android builder riêng trên NixOS/CI, xem [build-environments.md](build-environments.md). Từ `client/` bên trong builder:

```bash
corepack enable
yarn install --immutable
yarn workspace anomaly-mobile lint
yarn workspace anomaly-mobile typecheck
yarn workspace anomaly-mobile test
# Export endpoint và signing inputs từ môi trường được chuẩn bị trước.
yarn workspace anomaly-mobile tauri android build --target aarch64 --apk
```

Nix `.#android` giữ cho development/chạy emulator hoặc thiết bị; release wrapper không dùng toolchain/SDK paths của Nix host.

Tauri `beforeBuildCommand` gọi frontend build trong workspace mobile. Script release phải inject versionCode qua cơ chế cấu hình Tauri/Gradle đã được kiểm thử với CLI được lock; không chỉnh mỗi `defaultConfig` trong khi `tauri.properties` ghi đè giá trị đó.

Không chạy `tauri android init` lại mỗi CI run vì project Android đã track và có cấu hình riêng. Nếu generated setup cần bootstrap, nó phải idempotent và không xóa signing/minify configuration.

## 5. Thu thập và kiểm tra output

1. Đọc Gradle output metadata/build output để xác định đúng APK release ARM64; không lấy APK debug từ lần build trước.
2. Dùng Android build-tools `apksigner verify --verbose --print-certs <apk>` để kiểm chữ ký; so certificate fingerprint với key release.
3. Dùng SDK tool như `apkanalyzer manifest ...` để đọc applicationId, versionName, versionCode và debuggable từ APK đã đóng gói, không chỉ từ source.
4. Kiểm ABI, permission/cleartext policy, minSdk/targetSdk và endpoint build report; app release không cần `adb reverse` hay Vite.
5. Đổi tên thành `anomaly-mobile-0.1.0-rc.1-android-arm64.apk`, sau đó checksum/report. Khi promote có thể bỏ `rc.1` trong tên, nhưng byte hash phải giữ nguyên.

Bước verify phải được viết thành script, chưa tồn tại trong repo. Chuỗi lệnh SDK cụ thể cần đối chiếu version SDK của runner khi triển khai; không bỏ verify vì tool chưa cài.

## 6. Kiểm tra và phân phối

- Cài APK release bằng `adb install <apk>` trên thiết bị sạch, mở app khi rút USB; xác nhận app trỏ đúng domain.
- Cài đè candidate kế tiếp bằng `adb install -r <apk>` với cùng applicationId/key và versionCode cao hơn; dữ liệu/session xử lý như bản update bình thường.
- Nếu máy đang cài debug APK cùng ID nhưng key khác, test trên thiết bị/profile riêng; không hướng dẫn mọi người xóa dữ liệu như quy trình upgrade mặc định.
- Upload APK đã verify vào candidate artifact; sau promote, đính kèm GitHub Release cùng checksum và hướng dẫn cài từ nguồn tải.
- Download lại từ GitHub Release, verify checksum và cài file vừa download. Lần publish đầu phải kiểm quyền tải nếu repository private.
- Khi cần sửa bản đã public, tạo `0.1.1`/versionCode mới, giữ keystore; không ghi đè APK `0.1.0` bằng binary khác.

AAB/Google Play có thể bổ sung sau: upload key/Play App Signing, store listing và track thử nghiệm là pipeline riêng. APK sideload trong spec này chưa được gọi là đã phát hành Google Play.

## 7. Gap cần triển khai

- [ ] Android release signing config/secrets/backup key và ignore rules cho file tạm.
- [ ] Counter versionCode + đọc/verify version từ artifact cuối.
- [ ] Script build/collect/verify APK và upload CI artifact.
- [ ] Pin package manager/toolchain; chặn dependency update ngầm trong CI.
- [ ] Validate endpoint HTTPS trước build và lưu variant/domain trong manifest.
- [ ] CI runner có SDK/NDK/JDK đúng bản và cache không chứa signing material.
- [ ] Hướng dẫn download/install/update và report test APK thật.
