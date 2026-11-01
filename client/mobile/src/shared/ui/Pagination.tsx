import {
  faChevronLeft,
  faChevronRight,
} from "@fortawesome/free-solid-svg-icons";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";

interface PaginationProps {
  page: number;
  /** Tổng số trang đã biết. Khi còn dữ liệu trên server thì hasMore là true. */
  pageCount: number;
  hasMore?: boolean;
  disabled?: boolean;
  onPrevious: () => void;
  onNext: () => void;
}

const BUTTON =
  "flex min-h-11 items-center gap-2 rounded-full bg-surface-container px-4 text-sm font-semibold text-primary disabled:opacity-40";

/** Điều hướng trang. Tự ẩn khi toàn bộ dữ liệu nằm gọn trong một trang. */
export function Pagination({
  page,
  pageCount,
  hasMore = false,
  disabled = false,
  onPrevious,
  onNext,
}: PaginationProps) {
  if (pageCount <= 1 && !hasMore) return null;
  return (
    <nav
      aria-label="Phân trang"
      className="flex items-center justify-between gap-2"
    >
      <button
        type="button"
        className={BUTTON}
        disabled={disabled || page <= 1}
        onClick={onPrevious}
      >
        <FontAwesomeIcon icon={faChevronLeft} />
        Trang trước
      </button>
      <span className="text-xs text-on-surface-variant" aria-current="page">
        Trang {page}
        {hasMore ? "" : `/${Math.max(pageCount, 1)}`}
      </span>
      <button
        type="button"
        className={BUTTON}
        disabled={disabled || (page >= pageCount && !hasMore)}
        onClick={onNext}
      >
        Trang sau
        <FontAwesomeIcon icon={faChevronRight} />
      </button>
    </nav>
  );
}
