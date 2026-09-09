import { useState } from "react";
import { Badge } from "@/components/ui";
import { CopyIcon, EyeIcon, EyeOffIcon } from "@/components/icons";
import type { Account } from "@/types";

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

    return (
        <div className="relative overflow-hidden rounded-3xl bg-gradient-to-br from-brand-primary via-indigo-700 to-brand-secondary p-5 text-white shadow-xl shadow-brand-primary/30">
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
                    <button className="flex items-center gap-1 rounded-full bg-white/10 px-2.5 py-1 text-white/90 hover:bg-white/20">
                        {maskAccountNumber(account.accountNumber)}
                        <CopyIcon className="h-4 w-4" />
                    </button>
                </div>
            </div>
        </div>
    );
};