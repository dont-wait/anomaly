import type { ReactNode, Ref } from "react";
import { ShieldIcon } from "@/shared/icons";
import { formatVnd, maskAccountNo } from "@/features/transactions/utils/format";
import type { TransferFlowState } from "@/features/transfer/useTransferFlow";
import { StepTitle } from "./primitives";
import { Button } from "@/shared/ui";

const Row = ({ label, children }: { label: string; children: ReactNode }) => (
  <div className="flex items-start justify-between gap-4 py-3">
    <dt className="shrink-0 text-sm text-on-surface-variant">{label}</dt>
    <dd className="min-w-0 text-right text-sm font-medium wrap-break-word text-on-surface">
      {children}
    </dd>
  </div>
);

type Props = Pick<
  TransferFlowState,
  "source" | "recipient" | "amount" | "note" | "confirm"
> & {
  headingRef: Ref<HTMLHeadingElement>;
};

export function ConfirmStep({
  source,
  recipient,
  amount,
  note,
  confirm,
  headingRef,
}: Props) {
  if (!recipient) return null;

  return (
    <div className="space-y-6">
      <StepTitle headingRef={headingRef} title="Xác nhận giao dịch">
        Kiểm tra kỹ thông tin trước khi chuyển.
      </StepTitle>

      <section className="rounded-3xl bg-surface-container-lowest p-5">
        <div className="border-b border-dashed border-outline-variant pb-4 text-center">
          <p className="text-sm text-on-surface-variant">Số tiền chuyển</p>
          <p className="mt-1 text-3xl font-bold tracking-tight text-secondary-strong tabular-nums">
            {formatVnd(amount)}
          </p>
        </div>
        <dl className="divide-y divide-outline-variant/50">
          <Row label="Từ tài khoản">
            <span className="block">{source.ownerName}</span>
            <span className="block text-xs font-normal text-on-surface-variant">
              {maskAccountNo(source.accountNo)}
            </span>
          </Row>
          <Row label="Đến tài khoản">
            <span className="block">{recipient.name}</span>
            <span className="block text-xs font-normal text-on-surface-variant">
              {recipient.accountNo} · {recipient.bank}
            </span>
          </Row>
          <Row label="Nội dung">{note.trim() || "—"}</Row>
          <Row label="Phí giao dịch">
            <span className="text-success">Miễn phí</span>
          </Row>
          <Row label="Tổng trừ tài khoản">
            <span className="font-bold">{formatVnd(amount)}</span>
          </Row>
        </dl>
      </section>

      <p className="flex gap-2 rounded-2xl bg-secondary/10 px-4 py-3 text-sm text-on-surface-variant">
        <ShieldIcon
          className="h-5 w-5 shrink-0 text-secondary-strong"
          aria-hidden="true"
        />
        Giao dịch không thể hoàn tác sau khi xác thực OTP. AnomalyBank không bao
        giờ yêu cầu bạn cung cấp mã OTP.
      </p>

      <Button type="button" onClick={confirm} className="w-full">
        Xác nhận chuyển tiền
      </Button>
    </div>
  );
}
