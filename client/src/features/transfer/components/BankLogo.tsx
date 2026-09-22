import { useState } from "react";
import type { Bank } from "@/features/transfer/model";

// Logo VietQR là ảnh ngang (~3:1) nên dùng khung chữ nhật để logo không bị thu quá nhỏ.
const SIZES = {
  sm: "h-10 w-16 rounded-xl",
  md: "h-12 w-[4.5rem] rounded-2xl",
};

/** Logo ngân hàng trong ô trắng; ảnh lỗi thì hiện 2 chữ cái đầu của tên viết tắt. */
export const BankLogo = ({
  bank,
  size = "md",
}: {
  bank: Bank;
  size?: keyof typeof SIZES;
}) => {
  const [failedSrc, setFailedSrc] = useState<string | null>(null);
  const failed = failedSrc === bank.logo;

  return (
    <span
      aria-hidden="true"
      className={`flex shrink-0 items-center justify-center overflow-hidden bg-surface-container-lowest ring-1 ring-outline-variant/70 ${SIZES[size]}`}
    >
      {failed || !bank.logo ? (
        <span className="text-xs font-bold text-secondary-strong">
          {bank.shortName.slice(0, 2).toUpperCase()}
        </span>
      ) : (
        <img
          src={bank.logo}
          alt=""
          loading="lazy"
          width={72}
          height={48}
          onError={() => setFailedSrc(bank.logo)}
          className="h-full w-full object-contain px-1.5 py-1"
        />
      )}
    </span>
  );
};
