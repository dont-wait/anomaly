import { routes, transactionDetailRoute, type AppRoute } from "@/app/routes";
import type { AppNotification } from "./model";

/**
 * Nhận `data` của thông báo (do template server trả về) và trả route cần mở,
 * hoặc `null` nếu thiếu dữ liệu để điều hướng.
 */
type RouteResolver = (data: Record<string, string>) => AppRoute | null;

/**
 * Bảng `type → route` quyết định bấm vào thông báo (trong trung tâm thông báo
 * hoặc banner) thì mở màn nào. Hướng dẫn: docs/how-to-register-noti.md.
 *
 * Thêm type mới:
 *   1. Tạo route trong `src/app/routes.ts` nếu chưa có.
 *   2. Gọi `registerNotificationRoute("TYPE_MOI", (data) => ...)` bên dưới.
 *      Key trong `data` phải khớp với `data` mà template phía server trả về.
 *
 * Type chưa đăng ký: thử mở chi tiết giao dịch theo `data.transferId`, không có
 * thì mở trung tâm thông báo. Nên app bản cũ vẫn hiển thị được type mới.
 */
const resolvers = new Map<string, RouteResolver>();

export function registerNotificationRoute(
  type: string,
  resolve: RouteResolver,
) {
  resolvers.set(type, resolve);
}

const transferDetail: RouteResolver = (data) =>
  data.transferId ? transactionDetailRoute(data.transferId) : null;

// Nhóm giao dịch: template server trả `data.transferId`.
for (const type of ["TRANSFER_SENT", "TRANSFER_RECEIVED", "TRANSFER_BLOCKED"]) {
  registerNotificationRoute(type, transferDetail);
}

// Ví dụ đăng ký cho module khác:
// registerNotificationRoute("LOAN_APPROVED", (data) =>
//   data.applicationId ? loanApplication(data.applicationId) : null,
// );

export function routeForNotification(notification: AppNotification): AppRoute {
  const resolve = resolvers.get(notification.type) ?? transferDetail;
  return resolve(notification.data) ?? routes.notifications;
}
