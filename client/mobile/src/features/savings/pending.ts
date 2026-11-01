/** Pending financial requests must survive tab closure and never be sent unless saved. */
export type PendingSavingsCommand =
  | { kind: "open"; key: string; quoteId: string; sourceAccountId: string; acceptedTermsVersion: string }
  | { kind: "withdraw"; key: string; quoteId: string; savingsId: string };

const keyFor = (actorId: string) => `anomaly:savings:pending:${actorId}`;

function isCommand(value: unknown): value is PendingSavingsCommand {
  if (!value || typeof value !== "object") return false;
  const c = value as Record<string, unknown>;
  if (typeof c.key !== "string" || !c.key || typeof c.quoteId !== "string" || !c.quoteId) return false;
  if (c.kind === "open") return typeof c.sourceAccountId === "string" &&
    typeof c.acceptedTermsVersion === "string" && !!c.acceptedTermsVersion;
  return c.kind === "withdraw" && typeof c.savingsId === "string";
}

export function loadPending(actorId: string): PendingSavingsCommand | null {
  try {
    const raw = localStorage.getItem(keyFor(actorId));
    // Legacy versions used sessionStorage; never silently forget such commands.
    if (!raw && sessionStorage.getItem(keyFor(actorId))) {
      throw new Error("Có yêu cầu giao dịch từ phiên bản trước chưa được xác minh");
    }
    const parsed: unknown = raw ? JSON.parse(raw) : null;
    if (raw && !isCommand(parsed)) throw new Error("Invalid saved savings transaction");
    if (isCommand(parsed)) return parsed;
  } catch { /* fail closed: caller must not generate another financial command */
    throw new Error("Không thể đọc yêu cầu đang xử lý. Vui lòng kiểm tra dữ liệu ứng dụng trước khi thực hiện giao dịch mới.");
  }
  return null;
}

export function savePending(actorId: string, operation: PendingSavingsCommand): void {
  // Never issue a financial command if durable storage is blocked, full or unavailable.
  localStorage.setItem(keyFor(actorId), JSON.stringify(operation));
  if (!isCommand(loadPending(actorId)) || loadPending(actorId)?.key !== operation.key) {
    throw new Error("Không thể lưu yêu cầu giao dịch an toàn. Chưa thực hiện giao dịch.");
  }
}
export function clearPending(actorId: string): void {
  localStorage.removeItem(keyFor(actorId));
}
