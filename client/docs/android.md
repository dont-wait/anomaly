# Chạy Android trên NixOS

Dùng **một emulator API 35: `pixel_35`** (Google APIs, x86_64).
Shell có image API 35; SDK 36 vẫn cần để biên dịch thư viện Tauri.
Máy phát triển cần Linux x86_64 và Nix có bật flakes.

## 1. Chuẩn bị lần đầu

Từ root repository:

```bash
cd client
nix develop .#android
mkdir -p "$HOME/.local/bin"
corepack enable --install-directory "$HOME/.local/bin"
export PATH="$HOME/.local/bin:$PATH"
yarn install
make emulator-list
```

Nếu danh sách đã có `pixel_35`, bỏ qua bước tạo. Nếu chưa có, chạy:

```bash
make emulator-create
```

Trả lời `no` nếu được hỏi tạo hardware profile tùy chỉnh.
Nix cung cấp SDK/image nhưng **không tự tạo AVD**.
Hai lệnh Make tự dùng SDK trong flake, chạy được cả khi đã ở Nix shell.
Chỉ tạo một lần; lệnh không ghi đè `pixel_35` đã tồn tại.

Nếu đã vào shell Android và muốn chạy trực tiếp, lệnh tương đương là:

```bash
avdmanager create avd --name pixel_35 --package 'system-images;android-35;google_apis;x86_64'
```

## 2. Terminal 1: mở emulator

Từ root repository:

```bash
cd client
make emulator
```

Chờ màn hình chính Android xuất hiện. Giữ terminal này mở.
Nếu đã ở `client/`, bỏ dòng `cd client`.
Lệnh tự vào Nix shell Android. AVD `pixel_35` cần được tạo ở mục 1.

`make emulator` dùng webcam laptop (`webcam0`) làm **camera trước**.
Mở ứng dụng Camera trong máy ảo, chuyển sang camera trước để kiểm tra hình ảnh.
Tắt máy ảo đang chạy trước khi đổi camera và mở lại bằng lệnh này.

Liệt kê webcam và chọn camera khác nếu cần:

```bash
make emulator-webcams
make emulator CAMERA_FRONT=webcam1
```

Thay `webcam1` bằng tên trong danh sách. Nếu không có webcam, dùng
`make emulator CAMERA_FRONT=emulated` để chạy với camera mô phỏng.

## 3. Terminal 2: chạy app

Từ root repository:

```bash
cd client
make android
```

Lệnh tự vào Nix shell, chuyển tiếp cổng API `8080` và KYC `8090` từ
emulator về laptop bằng `adb reverse`, rồi thiết lập Corepack và chạy app.
Giữ một emulator đang chạy; `adb -e` chọn emulator, bỏ qua điện thoại USB.
Nếu chuyển tiếp lỗi, lệnh dừng trước khi chạy app. Mỗi lần `make android`
sẽ thiết lập lại chuyển tiếp, kể cả sau khi khởi động lại emulator.
Nếu cần kiểm tra thiết bị,
chạy `nix develop .#android --command adb devices -l`; trạng thái phải là `device`.
Tauri tự chạy Vite, build, cài và mở app. Không cần chạy `android init`,
`yarn dev` hoặc build APK riêng. Giữ terminal này chạy; `Ctrl+C` để dừng.
Trên emulator phải thấy **Hello Anomaly**.

Chạy bản Linux desktop bằng `make desktop` trong `client/`.
Android và desktop dùng chung port 1420: nhấn `Ctrl+C` dừng phiên client cũ
trước khi chuyển bản. Từ root có thể dùng `make -C client emulator`,
`make -C client android` hoặc `make -C client desktop`.

### Lệnh gộp đã dùng để kiểm tra

Khi emulator đã bật, chạy từ `client/` trong terminal bình thường:

```bash
nix develop .#android --command bash -c 'set -e; mkdir -p /tmp/anomaly-corepack-bin; corepack enable --install-directory /tmp/anomaly-corepack-bin; export PATH="/tmp/anomaly-corepack-bin:$PATH"; yarn tauri android dev --no-watch'
```

Chỉ chạy một lần. `/tmp` chứa shim Corepack tạm; `set -e` dừng khi lỗi.
`--no-watch` tắt theo dõi thay đổi Rust; bỏ cờ này khi phát triển bình thường.
Lệnh trực tiếp này yêu cầu chuyển tiếp cổng đã được thiết lập bởi `make android`.
Thông thường dùng mục 3 để tự thiết lập kết nối backend.

## 4. Khi có lỗi

| Lỗi                            | Cách xử lý                                                                             |
| ------------------------------ | -------------------------------------------------------------------------------------- |
| `Unknown AVD name`             | Chạy `emulator -list-avds`; nếu chưa có `pixel_35`, tạo theo mục 1.                    |
| `Broken AVD system path`       | Thoát shell cũ, vào lại `nix develop .#android`; kiểm tra image bằng lệnh bên dưới.    |
| ADB không thấy thiết bị        | Chờ emulator boot xong; kiểm tra `adb devices -l`.                                     |
| Emulator không tăng tốc        | Bật virtualization, kiểm tra quyền `/dev/kvm` và nhóm `kvm` của tài khoản.             |
| Port 1420 đã dùng              | Dừng phiên Vite/Tauri cũ rồi chạy lại.                                                 |
| Rust/Java/NDK hoặc `aapt2` lỗi | Dùng shell `.#android`; shell cung cấp JDK 17, Rust targets, NDK và `aapt2` cho NixOS. |

Kiểm tra image trong shell Android:

```bash
ls "$ANDROID_HOME/system-images/android-35/google_apis/x86_64/system.img"
```

Warning `Git tree ... dirty` báo thay đổi chưa commit. Warning
`ignoring untrusted substituter` báo bỏ qua cache Cachix; hai warning này
không gây lỗi thiếu AVD/image.

## 5. Lệnh kiểm tra thêm (không bắt buộc)

Thay serial nếu `adb devices -l` hiển thị giá trị khác:

```bash
adb -s emulator-5554 shell pidof com.dontwait.client
adb -s emulator-5554 logcat -d -t 500 -s AndroidRuntime chromium
adb -s emulator-5554 exec-out screencap -p > /tmp/anomaly-android-screen.png
```

Muốn thử HMR: tạm đổi chữ trong `src/App.tsx`, lưu, xem emulator cập nhật,
rồi hoàn tác phần chỉnh thử.

Backend chạy trên laptop: cấu hình `client/.env` như sau rồi chạy `make android`:

```dotenv
VITE_API_ENDPOINT=http://localhost:8080
VITE_KYC_ENDPOINT=http://localhost:8090
```

Hai dịch vụ cần chạy trên laptop ở các cổng tương ứng. `make android` tự
chuyển tiếp cả hai cổng nên cấu hình localhost dùng được trên emulator.
HTTP KYC qua loopback chỉ dành cho development với dữ liệu giả;
bản production cần endpoint HTTPS. Khởi động lại app sau khi đổi `.env`.

Build APK riêng khi cần:

```bash
nix develop .#android --command bash -c 'set -e; mkdir -p "$HOME/.local/bin"; corepack enable --install-directory "$HOME/.local/bin"; export PATH="$HOME/.local/bin:$PATH"; exec yarn tauri android build --debug --target x86_64 --apk'
```

Kiểm tra frontend theo thứ tự: `yarn lint`, `yarn tsc --noEmit`, `yarn test`.
Các kiểm tra này không thay thế việc nhìn UI trên emulator.
