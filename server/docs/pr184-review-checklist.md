# PR #184 — Kiểm tra sau review

Các mục đã được xử lý trong patch bổ sung:

- #1: lưu yêu cầu ghi tài chính trong `localStorage` **trước** khi gửi; nếu không lưu được thì không gửi và khóa thao tác.
- #2: bắt buộc gửi `acceptedTermsVersion`; Backend kiểm tra phiên bản đang ACTIVE cho **yêu cầu mở mới**; retry đã mở với cùng khóa tiếp tục lấy kết quả idempotent.
- #3: `SAVINGS_WRITES_DISABLED` là lỗi từ chối trước ghi, không coi là lệnh chưa rõ kết quả.
- #4: migration 16 down xóa riêng document seed; re-up có thể insert lại.
- #5: script bootstrap kiểm tra trùng ACTIVE và báo `_id` cụ thể trước khi tạo unique index; migration tiếp tục fail-closed khi trùng.
- #6–#9: làm mới cursor sau ghi, nạp ba nguồn dữ liệu độc lập, chặn response sai actor, giữ cập nhật command mới hơn response đang bay.
- #10: lãi ước tính dùng đúng lãi suất năm trả từ Backend.
- #11: không hiển thị số tổng toàn bộ tài khoản nếu còn trang chưa tải hay read model chưa xác nhận thay đổi sau ghi.
- #12: bao gồm sổ khóa trong số sổ chưa tất toán và bộ lọc.
- #14: biên nhận rút dùng dữ liệu thật từ response; không tạo contract giả với kỳ hạn/lãi suất bằng 0.
- #15: đã đồng bộ tên nút/nhãn test từ patch trước; test bổ sung cho lưu lệnh thất bại.
- #16: chuyển đăng ký route sang `savings_routes.go`.

Các mục cần quyết định kiến trúc với nhóm, không tự ý thay đổi tương thích API/hạ tầng:

- #13: Các API Savings hiện tại thống nhất trả response trực tiếp (`/packages`, `/quotes`, `/contracts`), khác API Auth. `GET /terms` tiếp tục đúng hợp đồng Savings đó; nếu muốn chuyển toàn bộ sang `{data,...}`, cần version lại spec/client và toàn bộ endpoint liên quan.
- #17: MongoDB là nơi nhóm yêu cầu lưu **nội dung sản phẩm**, không phải nguồn sự thật của **giao dịch tiền**. Banking Stream trên EventStoreDB vẫn lưu nguồn sự thật tài chính. Nếu quy ước CQRS yêu cầu cả nội dung sản phẩm phải event sourced, cần thiết kế luồng ProductTermsPublished + projector riêng sau khi thống nhất.

**Chưa hoàn tất**: Chấp thuận điều khoản mới được kiểm tra theo phiên bản tại điểm mở; chưa có sự kiện ký nhận văn bản pháp lý bất biến trong Banking Stream. Không thể xác nhận tính pháp lý chỉ bằng `acceptedTermsVersion` client gửi. Các thay đổi phải được review và chạy test full-stack trước khi merge. Không kiểm thử ghi tiền thật bằng cách tự động mở/rút trên dữ liệu dùng chung.
