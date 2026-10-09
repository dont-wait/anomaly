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

1. Thu email và xác thực OTP, sau đó thu ảnh CCCD hai mặt, họ tên (`username`), số CCCD, ngày sinh và ngày cấp. Ngày được gửi theo RFC3339 UTC.
2. Gọi `POST /api/auth/register` với `idempotencyKey` (UUID) cố định trong lần onboarding. Tài khoản chưa xác thực nhận `kycToken` và `kycExpiresAt`; password chỉ gửi trong request đăng ký và không được lưu vào storage.
3. Ghi video trực tiếp 12 giây. Gửi ảnh CCCD, video và `challengeType` qua multipart tới `POST /api/kyc/complete` với `Authorization: Bearer <kycToken>`. Backend gọi KYC service server-to-server; client không gọi trực tiếp KYC service.
4. Chỉ khi KYC trả `decision: VERIFIED` mới hiển thị hoàn tất và xoá resume state. Sau đó người dùng tự đăng nhập bằng CCCD/mật khẩu để nhận access token.
5. Nếu login trả `KYC_REQUIRED`, client gọi `POST /api/kyc/session` bằng chính credential vừa nhập, xoá password khỏi state rồi quay lại bước chọn lại giấy tờ. Resume state chỉ nằm trong `sessionStorage` và bị xoá khi token hết hạn hoặc KYC hoàn tất.

Trong `client/.env`, chỉ cấu hình API endpoint trỏ tới server. Client không cấu hình hoặc gọi KYC service; việc tích hợp KYC chỉ diễn ra server-to-server. Android emulator/IP LAN cần API server truy cập được; `localhost` trong Android là máy ảo. Restart Vite sau khi đổi env.

Dữ liệu media chỉ giữ trong bộ nhớ; rời trang hoặc reload sẽ yêu cầu chọn lại giấy tờ. Camera và request đang chạy được dừng khi rời trang. Android manifest đã khai báo CAMERA; cần rebuild/cài lại app để nhận quyền mới. Trên trình duyệt, camera yêu cầu secure context (HTTPS hoặc localhost).

Giới hạn hiện tại của dịch vụ:

- Khi chưa cấu hình pipeline KYC, backend trả `SYSTEM_ERROR`/502. Client giữ nguyên lượt thử và không tự chuyển thành `VERIFIED`.
- KYC stateless; giới hạn 3 lượt đang nằm ở UI. `RETRY_ALLOWED` trừ lượt, `FAILED_FINAL` khóa lại, còn lỗi hệ thống/mạng giữ lượt. Đây không phải giới hạn bảo mật phía server.
- Backend chỉ lưu media và cập nhật `isVerify=true` sau khi KYC service trả kết quả `VERIFIED`; access token và KYC token là hai loại token khác nhau.
