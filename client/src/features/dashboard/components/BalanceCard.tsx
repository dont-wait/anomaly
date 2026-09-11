import { useState } from "react";
import { Badge } from "@/shared/ui";
import { CopyIcon, EyeIcon, EyeOffIcon } from "@/shared/icons";
import type { Account } from "@/features/dashboard/model/types";

const formatCurrency = (amount: number, currency: string) =>
  `${new Intl.NumberFormat("vi-VN").format(amount)}${currency}`;

const maskAccountNumber = (accountNumber: string) => {
  const visible = accountNumber.slice(-4);
  return `**** ${visible}`;
};

interface BalanceCardProps {
  account: Account;
}

/** Thẻ số dư tài khoản chính — gradient thương hiệu, có chiều sâu bằng hoạ tiết mờ. */
export const BalanceCard = ({ account }: BalanceCardProps) => {
  const [isVisible, setIsVisible] = useState(true);
  const [copyMessage, setCopyMessage] = useState("");

  const copyAccountNumber = async () => {
    try {
      await navigator.clipboard.writeText(account.accountNumber);
      setCopyMessage("Đã sao chép số tài khoản");
    } catch {
      setCopyMessage("Không thể sao chép số tài khoản. Vui lòng thử lại.");
    }
  };

  return (
    <div className="relative overflow-hidden rounded-3xl bg-gradient-to-br from-[#8e5d8e] via-[#b582b5] to-[#d1afd1] p-5 text-white shadow-xl shadow-[#b582b5]/30">
      {/* Hoạ tiết trang trí mờ phía sau, tạo chiều sâu cho thẻ */}
      <div className="pointer-events-none absolute -top-10 -right-10 h-40 w-40 rounded-full bg-white/10 blur-2xl" />
      <div className="pointer-events-none absolute -bottom-14 -left-10 h-36 w-36 rounded-full bg-white/10 blur-2xl" />

      <div className="relative">
        <div className="flex items-center justify-between">
          <span className="text-sm font-semibold tracking-wide">
            AnomalyBank
          </span>
          {account.cardLabel && (
            <Badge variant="light" className="bg-white/25 backdrop-blur-sm">
              {account.cardLabel}
            </Badge>
          )}
        </div>

        <div className="mt-4 flex items-center gap-2 text-sm text-white/80">
          Số dư khả dụng
          <button
            onClick={() => setIsVisible((prev) => !prev)}
            aria-label={isVisible ? "Ẩn số dư" : "Hiện số dư"}
          >
            {isVisible ? (
              <EyeIcon className="h-4 w-4" />
            ) : (
              <EyeOffIcon className="h-4 w-4" />
            )}
          </button>
        </div>

        <p className="mt-1 text-3xl font-bold tracking-tight">
          {isVisible
            ? formatCurrency(account.balance, account.currency)
            : "••••••••" + account.currency}
        </p>

        <div className="mt-5 flex items-center justify-between text-sm">
          <div>
            <p className="text-white/70">Chủ tài khoản</p>
            <p className="font-medium">{account.ownerName}</p>
          </div>
          <button
            type="button"
            onClick={copyAccountNumber}
            aria-label="Sao chép số tài khoản"
            className="flex items-center gap-1 rounded-full bg-white/10 px-2.5 py-1 text-white/90 hover:bg-white/20"
          >
            {maskAccountNumber(account.accountNumber)}
            <CopyIcon className="h-4 w-4" />
          </button>
        </div>
        <p role="status" className="mt-2 text-xs text-white/80">
          {copyMessage}
        </p>
      </div>
    </div>
  );
};
