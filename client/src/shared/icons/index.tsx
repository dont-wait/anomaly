import type { SVGProps } from "react";

/**
 * Icon dùng chung cho toàn app. Vẽ tay bằng SVG (stroke, style outline)
 * để không phụ thuộc thư viện ngoài. Muốn đổi bộ icon (vd: lucide-react)
 * sau này chỉ cần sửa trong file này, chỗ dùng icon không cần đổi.
 */
type IconProps = SVGProps<SVGSVGElement>;

const base = (props: IconProps) => ({
  width: 20,
  height: 20,
  viewBox: "0 0 24 24",
  fill: "none",
  stroke: "currentColor",
  strokeWidth: 2,
  strokeLinecap: "round" as const,
  strokeLinejoin: "round" as const,
  ...props,
});

export const BellIcon = (props: IconProps) => (
  <svg {...base(props)}>
    <path d="M6 8a6 6 0 0 1 12 0c0 5 2 6 2 6H4s2-1 2-6" />
    <path d="M10 21a2 2 0 0 0 4 0" />
  </svg>
);

export const QrIcon = (props: IconProps) => (
  <svg {...base(props)}>
    <rect x="3" y="3" width="7" height="7" rx="1" />
    <rect x="14" y="3" width="7" height="7" rx="1" />
    <rect x="3" y="14" width="7" height="7" rx="1" />
    <path d="M14 14h3v3h-3zM20 14v3M17 20h4" />
  </svg>
);

export const EyeIcon = (props: IconProps) => (
  <svg {...base(props)}>
    <path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7-10-7-10-7Z" />
    <circle cx="12" cy="12" r="3" />
  </svg>
);

export const EyeOffIcon = (props: IconProps) => (
  <svg {...base(props)}>
    <path d="M3 3l18 18" />
    <path d="M10.6 5.1A10.9 10.9 0 0 1 12 5c6.5 0 10 7 10 7a16.9 16.9 0 0 1-3.2 4.1M6.5 6.5A16.7 16.7 0 0 0 2 12s3.5 7 10 7c1.4 0 2.6-.2 3.7-.6" />
    <path d="M9.9 9.9a3 3 0 0 0 4.2 4.2" />
  </svg>
);

export const CopyIcon = (props: IconProps) => (
  <svg {...base(props)}>
    <rect x="9" y="9" width="12" height="12" rx="2" />
    <path d="M5 15H4a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1h10a1 1 0 0 1 1 1v1" />
  </svg>
);

export const ChevronRightIcon = (props: IconProps) => (
  <svg {...base(props)}>
    <path d="m9 6 6 6-6 6" />
  </svg>
);

export const TransferIcon = (props: IconProps) => (
  <svg {...base(props)}>
    <path d="M7 7h11l-3-3M17 17H6l3 3" />
  </svg>
);

export const TopUpIcon = (props: IconProps) => (
  <svg {...base(props)}>
    <rect x="7" y="2" width="10" height="20" rx="2" />
    <path d="M11 6h2" />
  </svg>
);

export const BillIcon = (props: IconProps) => (
  <svg {...base(props)}>
    <path d="M6 3h12v18l-3-2-3 2-3-2-3 2Z" />
    <path d="M9 8h6M9 12h6" />
  </svg>
);

export const SavingsIcon = (props: IconProps) => (
  <svg {...base(props)}>
    <path d="M4 12a7 7 0 0 1 12-5l2-1 1 3-2 1a7 7 0 0 1-13 2Z" />
    <circle cx="16" cy="10" r=".5" fill="currentColor" />
    <path d="M4 15v2a2 2 0 0 0 2 2h1" />
  </svg>
);

export const MoreIcon = (props: IconProps) => (
  <svg {...base(props)}>
    <circle cx="5" cy="12" r="1.5" fill="currentColor" stroke="none" />
    <circle cx="12" cy="12" r="1.5" fill="currentColor" stroke="none" />
    <circle cx="19" cy="12" r="1.5" fill="currentColor" stroke="none" />
  </svg>
);

export const HomeIcon = (props: IconProps) => (
  <svg {...base(props)}>
    <path d="M4 11.5 12 4l8 7.5" />
    <path d="M6 10v9a1 1 0 0 0 1 1h10a1 1 0 0 0 1-1v-9" />
  </svg>
);

export const CardIcon = (props: IconProps) => (
  <svg {...base(props)}>
    <rect x="2" y="5" width="20" height="14" rx="2" />
    <path d="M2 10h20" />
  </svg>
);

export const UserIcon = (props: IconProps) => (
  <svg {...base(props)}>
    <circle cx="12" cy="8" r="4" />
    <path d="M4 20c0-4 3.5-6 8-6s8 2 8 6" />
  </svg>
);

export const CoffeeIcon = (props: IconProps) => (
  <svg {...base(props)}>
    <path d="M4 8h13v5a5 5 0 0 1-5 5H9a5 5 0 0 1-5-5Z" />
    <path d="M17 9h1.5a2.5 2.5 0 0 1 0 5H17" />
    <path d="M7 3.5c0 1-1 1-1 2M11 3.5c0 1-1 1-1 2" />
  </svg>
);

export const ShoppingCartIcon = (props: IconProps) => (
  <svg {...base(props)}>
    <circle cx="9" cy="20" r="1" fill="currentColor" stroke="none" />
    <circle cx="18" cy="20" r="1" fill="currentColor" stroke="none" />
    <path d="M3 4h2l2.4 11.4a2 2 0 0 0 2 1.6h7.2a2 2 0 0 0 2-1.6L21 8H6" />
  </svg>
);

export const ArrowDownLeftIcon = (props: IconProps) => (
  <svg {...base(props)}>
    <path d="M17 7 7 17M7 7v10h10" />
  </svg>
);

export const PiggyBankIcon = SavingsIcon;
