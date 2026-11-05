import { createContext, useContext } from "react";
import type { AppNotification } from "./model";

export interface NotificationContextValue {
  unreadCount: number;
  /** Thông báo nhận qua luồng realtime trong phiên này, mới nhất trước */
  live: AppNotification[];
  markRead: (notification: AppNotification) => Promise<void>;
  markAllRead: () => Promise<void>;
}

const noop = async () => {};

const fallback: NotificationContextValue = {
  unreadCount: 0,
  live: [],
  markRead: noop,
  markAllRead: noop,
};

export const NotificationContext =
  createContext<NotificationContextValue>(fallback);

/** Ngoài `NotificationProvider` trả về giá trị rỗng để màn hình vẫn render được. */
export const useNotifications = () => useContext(NotificationContext);
