# Verification: bản đóng gói có phát hành được hay không

## 1. Phạm vi kiểm tra

Gate ở đây kiểm chính binary/image/config được phân phối, khả năng cài/chạy/update/deploy và provenance. Không thay thế test nghiệp vụ của dự án. CI checks hiện hữu vẫn cần đạt, nhưng `yarn build` không chứng minh APK/bộ cài cài được, và Docker build không chứng minh migration/deployment thành công.

Trạng thái lúc viết spec: **Not run** cho build/sign/install/deploy. Chưa tạo workflow hoặc dùng secrets thật.

## 2. Verification matrix

| ID | Kiểm tra | Điều kiện đạt | Evidence |
| --- | --- | --- | --- |
| V01 | Source/version | Candidate SHA cố định; package/Cargo/Tauri version đúng 0.1.0 | Version check report |
| V02 | Toolchain/dependencies | Node/Yarn/Rust/JDK/SDK/NDK/container deps được ghi nhận, lockfile không bị sửa khi build | Build report + lock checksums |
| V03 | Inventory | Đủ artifact cho target matrix, không lẫn debug/consumer/admin hoặc binary run cũ | Manifest + file list |
| V04 | Endpoint | Client release trỏ đúng domain, không cần localhost/Vite/adb reverse | Runtime smoke + build variant |
| V05 | Android signing | APK signature hợp lệ, certificate đúng key release | apksigner report/fingerprint |
| V06 | Android identity | Application ID/versionName/versionCode/ABI/debuggable đúng | Packaged manifest metadata |
| V07 | Android install/update | Cài sạch và cài đè candidate mới, mở khi rút USB | Thiết bị/OS/WebView + install log |
| V08 | Linux package | Architecture/version/dependencies đúng; launcher chạy từ AppImage/deb trên máy sạch | Package metadata + machine report |
| V09 | Windows installer | Install/start/update/uninstall ở user context được hỗ trợ | Windows VM/machine report |
| V10 | Windows signature | Đúng certificate/timestamp hoặc ghi unsigned theo policy đã chọn | Signature report |
| V11 | Image content/start | Đúng entrypoints/migrations/model references, chạy non-root khi cấu hình yêu cầu | Container startup/binary report |
| V12 | Image registry | Digest/platform/commit khớp manifest, host pull được | Registry inspect + pull report |
| V13 | Config render | Image-only, không tag latest, secret refs/volumes/network/ports đúng | Sanitized config validation |
| V14 | Migration | Chạy bằng RC image, status/dirty đúng; failure chặn writer mới | Migration/deploy report |
| V15 | Deploy | Actual containers/services chạy đúng digest; API/TLS/WS/dependency probes đạt | Host inspect + smoke report |
| V16 | Persistence | Restart/recreate service không tạo DB rỗng/volume mới, không mất file cần giữ | Before/after volume/deployment report |
| V17 | Rollback/restore | Bản image tương thích hoặc roll-forward plan dùng được; backup restore đạt | Drill report |
| V18 | Promotion | Final bytes/digests bằng candidate đã verify, tag đúng SHA | Hash/digest comparison |
| V19 | Download | File tải từ final Release khớp checksum và cài được | Download verification |

Không pass bằng cách chỉ đọc source config. Ví dụ versionName phải đọc từ APK, image digest đọc từ registry/runtime, package version đọc từ installer/package cuối.

Bổ sung gate môi trường: lưu builder digest; không mount/cache build output Nix vào Ubuntu; inspect ELF interpreter/RPATH/native dependencies rồi test trên Linux baseline. Android cài/chạy ngoài container. Xem [build-environments.md](build-environments.md).

## 3. Build sanity checks

Từ `client/`, môi trường toolchain theo target:

```bash
corepack enable
yarn install --immutable
yarn lint
yarn typecheck
yarn test
```

Sau đó native build theo [mobile.md](mobile.md) và [desktop.md](desktop.md), cung cấp `VITE_API_ENDPOINT` trước Tauri hook frontend build. `yarn typecheck` là script workspace hiện hành; không dùng lệnh root cũ làm gate cho app không được check.

Server/Python chạy checks hiện có trong `ci-be.yml` trước build/push. E2E Docker cần dependency thực; workflow release phải báo fail/not run rõ khi môi trường thiếu runtime, không gộp thành passed.

Release script cần tự kiểm git diff sau dependency/bootstrap/build để phát hiện lockfile hoặc generated source bị thay đổi ngoài dự kiến. File output build không được tính thành source changes cần commit.

## 4. Smoke test tối thiểu sau cài/deploy

Đây là kiểm chứng kết nối của bản đóng gói, không phải danh sách tính năng release:

1. Mở admin từ launcher và mobile từ icon, xác nhận branding/version, không yêu cầu máy development.
2. Đăng nhập tài khoản test qua endpoint cuối, thực hiện một API read và một write thử được phép trong môi trường kiểm tra.
3. Kiểm route/API dùng đúng TLS/domain, không vô tình gọi localhost hoặc domain staging trong production artifact.
4. Kiểm Go process, projectors, relay và ML/KYC theo thành phần được deploy; probe health không thay readiness/model load.
5. Restart/recreate bằng cùng manifest, xác nhận dữ liệu cần giữ và secrets/config vẫn đúng.
6. Download lại installer từ kênh phân phối và lặp install/start bằng file tải đó.

Nếu application hiện có lỗi nghiệp vụ, ghi nhận qua backlog/suite nghiệp vụ riêng; không biến release packaging spec thành kế hoạch xây thêm màn hình.

## 5. Failure và rerun

- Build thiếu key/endpoint: fail trước tạo artifact, không fallback debug hoặc localhost.
- Một target fail: candidate incomplete, không promote release hoàn chỉnh. Rerun chỉ được reuse output có cùng candidate/SHA/input hash đã kiểm chứng.
- Checksum/signature mismatch: hủy candidate verification; không chỉ sửa checksum để hợp thức hóa binary không rõ nguồn.
- Migration fail/dirty: stop rollout, giữ bản trước nếu còn tương thích; không auto-force/down.
- Deploy smoke fail: giữ draft/unpublished, báo state thực tế của host; không đánh dấu deploy success chỉ vì SSH exit 0.
- Final asset đã tồn tại nhưng khác hash: refuse overwrite. Phát hành bản sửa bằng version mới.

## 6. Verification report bắt buộc

```text
Release version / candidate:
Commit / CI run / artifact source run:
Build environment + API endpoint:
Toolchain versions / lockfile hashes:
Android: file/hash/appId/versionName/versionCode/ABI/cert/device:
Linux: file/hash/arch/packageVersion/test distro:
Windows: file/hash/arch/version/signing status/test OS:
Images: name/digest/platform/commit:
External ML source revision / model checksums:
Deployment config revision / migration status / target host:
Install/update/deploy/restore reports:
Matrix: passed / failed / not run:
Promotion eligibility + reviewer/UTC time:
```

Chỉ promote khi tất cả target trong matrix đã chọn có đủ file và V01–V18 đạt theo phần áp dụng. V19 kiểm ngay sau publish; fail thì dừng phân phối tiếp và mở sự cố release. Báo cáo không chứa keystore, private token, runtime.env hoặc media người dùng.
