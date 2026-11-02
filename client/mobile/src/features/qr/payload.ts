const QR_PREFIX = "anomalybank://transfer?accountNo=";
const ACCOUNT_NO = /^\d{6,19}$/;

/** Mã chỉ chứa số tài khoản; tên người nhận luôn do server trả về khi tra cứu. */
export const buildAccountQr = (accountNo: string) => `${QR_PREFIX}${accountNo}`;

export function parseAccountQr(text: string): string | null {
  if (!text.startsWith(QR_PREFIX)) return null;
  const accountNo = text.slice(QR_PREFIX.length);
  return ACCOUNT_NO.test(accountNo) ? accountNo : null;
}
