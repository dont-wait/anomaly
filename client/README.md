# Anomaly client

Ứng dụng Tauri 2 với React, TypeScript và Rust, hỗ trợ Linux desktop và Android.

## Môi trường phát triển

Chạy từ thư mục `client/`. Nix cần bật `nix-command` và `flakes`.

| Lệnh                    | Mục đích                                            |
| ----------------------- | --------------------------------------------------- |
| `nix develop`           | Linux desktop                                       |
| `nix develop .#windows` | Cross-compile Windows qua MinGW                     |
| `nix develop .#android` | Android trên máy phát triển Linux x86_64, gồm NixOS |

Trong shell, bật Corepack và cài dependencies:

```bash
mkdir -p "$HOME/.local/bin"
corepack enable --install-directory "$HOME/.local/bin"
export PATH="$HOME/.local/bin:$PATH"
yarn install
```

Đặt shim Corepack trong thư mục người dùng vì `/nix/store` chỉ đọc.

## Chạy ứng dụng

Sau khi cài dependencies, chạy từ `client/`:

Chạy `make help` (hoặc `make`) để xem danh sách lệnh.

```bash
make emulator-list    # Kiểm tra máy ảo đã có
make emulator-create  # Tạo pixel_35 một lần nếu chưa có
make emulator  # Terminal 1: mở pixel_35
make android   # Terminal 2: chạy client Android
make desktop   # Chạy client Linux desktop
```

Các lệnh tự vào Nix shell phù hợp; lệnh client tự thiết lập Corepack.
Từ root repository dùng `make -C client <tên-lệnh>`.
Android và desktop cùng dùng port 1420: dừng phiên client cũ bằng `Ctrl+C`
trước khi chuyển sang bản còn lại. Emulator có thể tiếp tục chạy.

Xem **[hướng dẫn Android trên NixOS](docs/android.md)** để thiết lập emulator,
chạy lần đầu, kết nối backend, build APK và xử lý lỗi.
Hướng dẫn dùng một emulator API 35 (`pixel_35`), tách rõ terminal mở máy ảo
và terminal chạy app; có cả lệnh gộp để copy chạy trực tiếp.

Nếu cần cấu hình API, sao chép `.env.example` thành `.env` và đặt
`VITE_API_ENDPOINT` theo backend đang dùng. Không commit `.env`.

## Kiểm tra

Chạy theo thứ tự:

```bash
yarn lint
yarn tsc --noEmit
yarn test
```

Các kiểm tra frontend này không thay thế build Android hoặc kiểm tra giao diện trên thiết bị.
