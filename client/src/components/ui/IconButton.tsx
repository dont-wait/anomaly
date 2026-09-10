import { type ButtonHTMLAttributes, type ReactNode } from "react";

type IconButtonTone = "default" | "brand";

interface IconButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
    icon: ReactNode;
    /** Số hiển thị trên badge góc trên phải, vd: số thông báo chưa đọc */
    badgeCount?: number;
    /** Màu nền: "default" xám nhạt, "brand" xanh dương đậm nổi bật */
    tone?: IconButtonTone;
}

const tones: Record<IconButtonTone, string> = {
    default: "bg-gray-100 text-gray-600 hover:bg-gray-200",
    brand: "bg-brand-primary text-white hover:bg-brand-primary/90",
};

/** Nút icon tròn dùng cho header (thông báo, QR, hồ sơ...), có thể gắn badge số. */
export const IconButton = ({
    icon,
    badgeCount,
    tone = "default",
    className = "",
    ...props
}: IconButtonProps) => {
    return (
        <button
            className={`relative flex h-10 w-10 items-center justify-center rounded-full transition-colors ${tones[tone]} ${className}`}
            {...props}
        >
            {icon}
            {typeof badgeCount === "number" && badgeCount > 0 && (
                <span className="absolute -top-1 -right-1 flex h-4 min-w-4 items-center justify-center rounded-full bg-error px-1 text-[10px] font-semibold text-white">
                    {badgeCount > 9 ? "9+" : badgeCount}
                </span>
            )}
        </button>
    );
};