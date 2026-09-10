import { useEffect, useRef } from "react";
export function InfoDialog({
  title,
  onClose,
}: {
  title: string;
  onClose: () => void;
}) {
  const ref = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    ref.current?.showModal();
  }, []);
  return (
    <dialog ref={ref} className="info-dialog" onCancel={onClose}>
      <h2>{title}</h2>
      <p>
        {title === "Hỗ trợ"
          ? "Để chụp CCCD và khuôn mặt, hãy cho phép truy cập camera, chọn nơi đủ sáng và giữ thiết bị ổn định. Kênh hỗ trợ trực tuyến sẽ được bổ sung khi dịch vụ sẵn sàng."
          : title === "Khôi phục mật khẩu"
            ? "Chức năng gửi email khôi phục mật khẩu chưa được kết nối."
            : "Khi tiếp tục, thông tin đăng ký được gửi tới hệ thống AnomalyBank; ảnh CCCD và video được gửi để đối chiếu danh tính. Nội dung điều khoản và chính sách chi tiết đang được cập nhật."}
      </p>
      <button className="primary" onClick={onClose}>
        Đã hiểu
      </button>
    </dialog>
  );
}
