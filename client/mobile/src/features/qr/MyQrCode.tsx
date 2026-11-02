import { useState } from "react";
import { QRCodeSVG } from "qrcode.react";
import { CopyIcon } from "@/shared/icons";
import { buildAccountQr } from "./payload";

export function MyQrCode({
  accountNo,
  ownerName,
}: {
  accountNo: string;
  ownerName: string;
}) {
  const [copyMessage, setCopyMessage] = useState("");

  const copyAccountNo = async () => {
    try {
      await navigator.clipboard.writeText(accountNo);
      setCopyMessage("Đã sao chép số tài khoản");
    } catch {
      setCopyMessage("Không thể sao chép số tài khoản. Vui lòng thử lại.");
    }
  };

  return (
    <section className="flex flex-col items-center gap-4 rounded-3xl bg-surface-container-lowest p-6 text-center shadow-sm shadow-secondary/10">
      <div>
        <h2 className="text-lg font-bold text-primary">{ownerName}</h2>
        <p className="text-sm text-on-surface-variant">AnomalyBank</p>
      </div>
      {/* Luôn nền trắng và viền trống để máy quét đọc được ở mọi giao diện. */}
      <div className="rounded-2xl bg-white p-4">
        <QRCodeSVG
          value={buildAccountQr(accountNo)}
          size={232}
          level="M"
          role="img"
          aria-label={`Mã QR nhận tiền của tài khoản ${accountNo}`}
        />
      </div>
      <button
        type="button"
        onClick={() => void copyAccountNo()}
        aria-label="Sao chép số tài khoản"
        className="flex min-h-11 items-center gap-2 rounded-full bg-surface-container px-4 text-base font-bold tracking-wide text-primary"
      >
        {accountNo}
        <CopyIcon className="h-4 w-4" />
      </button>
      <p role="status" className="min-h-4 text-xs text-on-surface-variant">
        {copyMessage}
      </p>
      <p className="text-sm text-on-surface-variant">
        Đưa mã này cho người dùng AnomalyBank khác quét để nhận tiền.
      </p>
    </section>
  );
}
