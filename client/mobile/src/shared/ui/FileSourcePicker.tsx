import type { ChangeEvent } from "react";
import { faCamera, faFolderOpen } from "@fortawesome/free-solid-svg-icons";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";

interface FileSourcePickerProps {
  label?: string;
  /** Định dạng cho nút chọn tệp. Nút camera luôn dùng image/*. */
  accept: string[];
  camera: "user" | "environment";
  disabled?: boolean;
  /** Phân biệt các nút khi một màn có nhiều bộ chọn tệp. */
  accessibleName?: string;
  onSelect: (file: File) => void;
}

// `relative` giữ input sr-only (position: absolute) trong nút. Nếu không, input
// thoát khỏi vùng cuộn và trình duyệt cuộn nhầm khung ngoài khi focus vào nó.
const BUTTON =
  "relative flex min-h-11 cursor-pointer items-center justify-center gap-2 rounded-2xl bg-[#e5eaff] px-3 py-2 text-xs font-bold text-[#00236f] focus-within:outline-2 focus-within:outline-offset-2 focus-within:outline-[#6b38d4]";

/** Hai nguồn tệp tách biệt: mở camera hoặc chọn tệp có sẵn. */
export function FileSourcePicker({
  label,
  accept,
  camera,
  disabled = false,
  accessibleName,
  onSelect,
}: FileSourcePickerProps) {
  const handleChange = (event: ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    // Cho phép chọn lại đúng tệp đó sau khi hủy hoặc gặp lỗi.
    event.target.value = "";
    if (file) onSelect(file);
  };
  const buttonClass = `${BUTTON}${disabled ? " cursor-default opacity-45" : ""}`;

  return (
    <div className="flex flex-col gap-2">
      {label && <p className="text-xs font-semibold text-[#444651]">{label}</p>}
      <div className="grid grid-cols-2 gap-2">
        <label className={buttonClass}>
          <FontAwesomeIcon icon={faCamera} />
          Mở camera
          {/* WebView Android của Tauri chỉ mở camera khi accept đúng là image/*;
              định dạng thực tế được kiểm tra sau khi chụp. */}
          <input
            className="sr-only"
            type="file"
            accept="image/*"
            capture={camera}
            disabled={disabled}
            aria-label={accessibleName && `${accessibleName}: mở camera`}
            onChange={handleChange}
          />
        </label>
        <label className={buttonClass}>
          <FontAwesomeIcon icon={faFolderOpen} />
          Chọn tệp
          <input
            className="sr-only"
            type="file"
            accept={accept.join(",")}
            disabled={disabled}
            aria-label={accessibleName && `${accessibleName}: chọn tệp`}
            onChange={handleChange}
          />
        </label>
      </div>
    </div>
  );
}
