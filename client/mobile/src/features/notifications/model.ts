export type NotificationSeverity = "INFO" | "SUCCESS" | "WARNING" | "CRITICAL";

export interface AppNotification {
  id: string;
  /** Ví dụ TRANSFER_RECEIVED; type mới từ server vẫn hiển thị được */
  type: string;
  category: string;
  severity: NotificationSeverity;
  title: string;
  body: string;
  /** Dữ liệu điều hướng, ví dụ `transferId` */
  data: Record<string, string>;
  occurredAt: Date;
  createdAt: Date;
  read: boolean;
}

export interface NotificationPage {
  items: AppNotification[];
  nextCursor: string | null;
}
