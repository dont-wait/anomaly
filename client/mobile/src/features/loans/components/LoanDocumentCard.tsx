import {
  faCircleCheck,
  faClock,
  faDownload,
  faFile,
  faFileInvoiceDollar,
  faHouseUser,
  faIdCard,
  faIdCardClip,
  faMobileScreen,
  faShieldHeart,
  faTriangleExclamation,
  type IconDefinition,
} from "@fortawesome/free-solid-svg-icons";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import {
  documentLabels,
  type DocumentType,
  type LoanDocument,
} from "@/features/loans/api";
import { statusLabel } from "@/features/loans/format";
import { FileSourcePicker } from "@/shared/ui";

const GUIDES: Record<DocumentType, { icon: IconDefinition; hint: string }> = {
  IDENTITY_FRONT: {
    icon: faIdCard,
    hint: "Chụp rõ cả 4 góc, không bị lóa.",
  },
  IDENTITY_BACK: {
    icon: faIdCardClip,
    hint: "Chụp rõ cả 4 góc, không bị lóa.",
  },
  VNEID_PERSONAL: {
    icon: faMobileScreen,
    hint: "Ảnh chụp màn hình thông tin cá nhân trên VNeID.",
  },
  VNEID_RESIDENCE: {
    icon: faHouseUser,
    hint: "Ảnh chụp màn hình thông tin cư trú trên VNeID.",
  },
  VSSID_PARTICIPATION: {
    icon: faShieldHeart,
    hint: "Ảnh chụp màn hình quá trình tham gia BHXH trên VssID.",
  },
  VSSID_DETAIL: {
    icon: faShieldHeart,
    hint: "Ảnh chụp màn hình chi tiết BHXH trên VssID.",
  },
  SALARY_STATEMENT_3_MONTHS: {
    icon: faFileInvoiceDollar,
    hint: "Sao kê tài khoản nhận lương 3 tháng gần nhất.",
  },
};

const LOAN_DOCUMENT_ACCEPT = ["application/pdf", "image/jpeg", "image/png"];

interface LoanDocumentCardProps {
  type: DocumentType;
  /** Bản hợp lệ đã tải; bản bị từ chối được truyền qua `rejected`. */
  document?: LoanDocument;
  rejected: boolean;
  disabled: boolean;
  uploading: boolean;
  onSelect: (file: File) => void;
  onDownload: (documentId: string) => void;
}

const STATUS_TONES: Record<
  string,
  { className: string; icon: IconDefinition }
> = {
  pending: { className: "bg-amber-50 text-amber-800", icon: faClock },
  verified: {
    className: "bg-emerald-50 text-emerald-700",
    icon: faCircleCheck,
  },
  rejected: {
    className: "bg-red-50 text-red-700",
    icon: faTriangleExclamation,
  },
};

/** Một chứng từ đã gửi: tên, trạng thái, ngày gửi và nút tải về. */
export function LoanDocumentRow({
  document,
  onDownload,
}: {
  document: LoanDocument;
  onDownload: (documentId: string) => void;
}) {
  const label = documentLabels[document.type] ?? "Chứng từ";
  const tone =
    STATUS_TONES[document.verificationStatus] ?? STATUS_TONES.pending;
  const uploadedAt = new Date(document.uploadedAt);
  return (
    <div className="flex items-center gap-3 rounded-2xl border border-[#e2e7ff] bg-white p-3">
      <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-[#f0eaff] text-[#6b38d4]">
        <FontAwesomeIcon icon={GUIDES[document.type]?.icon ?? faFile} />
      </span>
      <div className="min-w-0 flex-1">
        <p className="text-sm font-semibold text-primary">{label}</p>
        <p className="mt-1 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-on-surface-variant">
          <span
            className={`rounded-full px-2 py-0.5 font-semibold ${tone.className}`}
          >
            <FontAwesomeIcon icon={tone.icon} className="mr-1" />
            {statusLabel(document.verificationStatus)}
          </span>
          {!Number.isNaN(uploadedAt.getTime()) && (
            <span>Gửi {uploadedAt.toLocaleDateString("vi-VN")}</span>
          )}
        </p>
      </div>
      <button
        type="button"
        aria-label={`Tải xuống ${label}`}
        onClick={() => onDownload(document.documentId)}
        className="flex h-11 w-11 shrink-0 items-center justify-center rounded-full bg-[#e5eaff] text-[#00236f] active:bg-[#d7deff]"
      >
        <FontAwesomeIcon icon={faDownload} />
      </button>
    </div>
  );
}

export function LoanDocumentCard({
  type,
  document,
  rejected,
  disabled,
  uploading,
  onSelect,
  onDownload,
}: LoanDocumentCardProps) {
  const guide = GUIDES[type];
  const label = documentLabels[type];

  if (document) {
    return <LoanDocumentRow document={document} onDownload={onDownload} />;
  }

  return (
    <div
      className={`space-y-3 rounded-2xl border bg-white p-3 ${rejected ? "border-red-200" : "border-[#e2e7ff]"}`}
    >
      <div className="flex gap-3">
        <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-[#f0eaff] text-[#6b38d4]">
          <FontAwesomeIcon icon={guide.icon} />
        </span>
        <div className="min-w-0">
          <p className="text-sm font-semibold text-primary">{label}</p>
          <p className="mt-0.5 text-xs text-on-surface-variant">{guide.hint}</p>
        </div>
      </div>
      {rejected && (
        <p className="rounded-xl bg-red-50 p-2 text-xs text-red-700">
          <FontAwesomeIcon icon={faTriangleExclamation} className="mr-1.5" />
          Giấy tờ trước chưa đạt. Vui lòng tải lại.
        </p>
      )}
      {uploading ? (
        <p
          role="status"
          className="py-2 text-center text-xs font-semibold text-secondary"
        >
          Đang tải lên...
        </p>
      ) : (
        <FileSourcePicker
          accessibleName={label}
          accept={LOAN_DOCUMENT_ACCEPT}
          camera="environment"
          disabled={disabled}
          onSelect={onSelect}
        />
      )}
    </div>
  );
}
