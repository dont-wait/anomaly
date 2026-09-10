# Cấu trúc frontend

- `app/`: provider toàn ứng dụng và điều hướng. `routes.ts` khai báo URL; `AppRouter.tsx` chọn trang từ hash (`#/login`, `#/register`) để chạy cùng cách trong web và Tauri.
- `pages/`: điểm vào từng trang, nối tính năng với điều hướng.
- `features/login/`: `LoginView` giữ UI đăng nhập; `useLoginForm` quản lý form và gọi `useAuth`.
- `features/registration/`: `RegistrationFlow` là layout; `useRegistrationFlow` quản lý state/timer/chuyển bước; `RegistrationStep` chọn màn; `steps/` chứa từng màn; `components/` chứa camera, dialog và phần UI dùng chung trong đăng ký; `model.ts` chứa kiểu và quy tắc.
- `auth/`: phiên đăng nhập dùng chung (`AuthProvider`, `AuthContext`, `useAuth`).
- `api/`: HTTP, endpoint và token store; không chứa giao diện.
- `components/ui/`: thành phần UI dùng lại giữa các tính năng.

Thêm trang: khai báo URL trong `app/routes.ts`, tạo trang trong `pages/` và thêm case vào `AppRouter`. Thêm bước đăng ký: cập nhật `model.ts`, chuyển bước trong hook và màn tương ứng trong `steps/`/`RegistrationStep`.

Đăng ký dùng các API thật theo thứ tự:

1. Thu email, ảnh CCCD hai mặt, họ tên (`username`), số CCCD, ngày sinh và ngày cấp. Không có bước OTP vì server chưa có endpoint gửi/xác minh OTP. Endpoint đăng ký chưa nhận địa chỉ thường trú.
2. Ghi video trực tiếp 12 giây với hướng dẫn nhìn thẳng, quay trái/phải và chớp mắt. Gửi `cccd_front_image`, `live_video`, `challenge_type=TURN_HEAD_LEFT_RIGHT_BLINK` qua multipart tới `POST /v1/kyc/verify-face`.
3. Chỉ khi KYC trả `success: true` và `decision: VERIFIED`, cho tạo mật khẩu. Gọi `POST /api/auth/register` với `idempotencyKey` (UUID) cố định trong bộ nhớ cho lần onboarding; retry cùng dữ liệu và key nhận lại tài khoản đã tạo, key dùng với dữ liệu khác trả 409; ngày được gửi theo RFC3339 UTC.
4. Gọi `useAuth().login` với CCCD/mật khẩu. Token lưu qua logic login hiện có (`anomaly.auth.token`). Không tạo token mới hoặc gửi token này sang KYC service.
5. Upload 3 tệp bằng `POST /api/media/upload` (`file`, `key`), sau đó gửi các key qua `POST /api/accounts/{id}/verify` với Bearer token. Dù tên field là URL, server hiện lưu chúng vào `StorageKey`.
6. Refresh profile trong `AuthProvider`, hiển thị hoàn tất. Nếu login/upload/verify lỗi, giữ tài khoản đã tạo và các key upload thành công để thử lại ngay trên trang, không gọi register lại.

Trong `client/.env`, cấu hình `VITE_API_ENDPOINT` trỏ server và `VITE_KYC_ENDPOINT` trỏ KYC (mặc định development: `http://localhost:8090`). Endpoint KYC phải dùng HTTPS; chỉ cho phép HTTP loopback (`localhost`, `127.0.0.1`, `[::1]`) trong development với dữ liệu giả. Android emulator/IP LAN cần endpoint HTTPS truy cập được; `localhost` trong Android là máy ảo. Restart Vite sau khi đổi env. Origin của client phải có trong CORS allowlist của cả server và KYC.

Dữ liệu form/media chỉ giữ trong bộ nhớ; rời trang hoặc reload sẽ mất tiến trình. Camera và request đang chạy được dừng khi rời trang. Android manifest đã khai báo CAMERA; cần rebuild/cài lại app để nhận quyền mới. Trên trình duyệt, camera yêu cầu secure context (HTTPS hoặc localhost). Nếu mất phản hồi register, thử lại ngay trên trang để dùng cùng idempotency key. Nếu đã tạo tài khoản rồi mới reload, dùng trang login với CCCD/mật khẩu; hiện chưa có màn khôi phục onboarding dở dang.

Giới hạn hiện tại của dịch vụ:

- KYC chỉ triển khai stub, mặc định disabled; khi chưa cấu hình pipeline, API trả 503. Client hiển thị lỗi, không tự chuyển thành VERIFIED. Không coi kết quả stub là nhận diện thật.
- KYC stateless, không có session/retry API; giới hạn 3 lượt đang nằm ở UI. RETRY_ALLOWED trừ lượt; FAILED_FINAL khóa lại; lỗi hệ thống/mạng giữ lượt. Đây không phải giới hạn bảo mật phía server.
- `/api/accounts/{id}/verify` hiện tin media keys từ client, chưa xác minh lại kết quả KYC phía server. Tích hợp này gọi đúng hợp đồng đang có; để dùng xác minh danh tính thật cần server kiểm chứng kết quả KYC trước khi ghi verified.
