import type { Account, PromoBanner, QuickAction, Transaction } from "@/features/dashboard/model/types";

export const mockAccount: Account = {
  id: "acc_001",
  ownerName: "PHAM DINH MINH HIEU",
  accountNumber: "99999180105",
  balance: 128_540_000,
  currency: "₫",
  cardLabel: "ANOMALYBANK SIGNATURE",
};

export const mockQuickActions: QuickAction[] = [
  { id: "qa_transfer", label: "Chuyển tiền", icon: "transfer" },
  { id: "qa_topup", label: "Nạp tiền ĐT", icon: "topup" },
  { id: "qa_bill", label: "Hóa đơn", icon: "bill" },
  { id: "qa_savings", label: "Tiết kiệm", icon: "savings" },
  { id: "qa_more", label: "Xem thêm", icon: "more" },
];

export const mockPromoBanner: PromoBanner = {
  id: "promo_001",
  title: "Hoàn tiền 10%",
  subtitle: "Ưu đãi VietQR siêu tốc · Áp dụng tại 150.000+ điểm chấp nhận",
  ctaLabel: "Khám phá ngay",
};

export const mockTransactions: Transaction[] = [
  {
    id: "tx_001",
    amount: 55_000,
    type: "debit",
    description: "Highlands Coffee",
    subtitle: "Hôm nay, 08:30 · Ăn uống",
    category: "food",
    date: new Date(),
  },
  {
    id: "tx_002",
    amount: 2_500_000,
    type: "credit",
    description: "Nhận từ Trần Thị B",
    subtitle: "Hôm qua, 17:45 · Chuyển khoản",
    category: "transfer-in",
    date: new Date(Date.now() - 24 * 60 * 60 * 1000),
  },
  {
    id: "tx_003",
    amount: 348_000,
    type: "debit",
    description: "Siêu thị WinMart",
    subtitle: "Hôm qua, 12:15 · Tiêu dùng",
    category: "shopping",
    date: new Date(Date.now() - 24 * 60 * 60 * 1000),
  },
  {
    id: "tx_004",
    amount: 485_000,
    type: "credit",
    description: "Tiền lãi tiết kiệm T8",
    subtitle: "28/08, 00:01 · AnomalyBank",
    category: "savings",
    date: new Date("2026-08-28T00:01:00"),
  },
];
