import { useCallback, useEffect, useRef, useState } from "react";
import type { FormEvent } from "react";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import { faUserPen } from "@fortawesome/free-solid-svg-icons";
import { request } from "@/features/loans/api/loansApi";
import { explainLoanError } from "@/features/loans/format";

type FieldKey =
  | "fullName"
  | "addressLine"
  | "provinceCode"
  | "issuedPlace"
  | "permanentAddress";

const FIELD_LABELS: Record<FieldKey, string> = {
  fullName: "Họ và tên",
  addressLine: "Địa chỉ liên hệ",
  provinceCode: "Tỉnh/thành",
  issuedPlace: "Nơi cấp CCCD",
  permanentAddress: "Địa chỉ thường trú",
};
const FIELDS = Object.keys(FIELD_LABELS) as FieldKey[];

export interface ProfileChangeItem {
  accountId: string;
  username: string;
  current: Record<FieldKey, string>;
  change: Partial<Record<FieldKey, string>> & {
    id: string;
    requestedAt: string;
    status: "PENDING" | "APPROVED" | "REJECTED";
    rejectionReason?: string;
    reviewedAt?: string;
  };
}

type ReviewStatus = "PENDING" | "APPROVED" | "REJECTED" | "ALL";

interface ProfileChangesPageData {
  items: ProfileChangeItem[];
  total: number;
  page: number;
  pageSize: number;
  totalPages: number;
}

const ROOT = "/api/admin/profile-changes";

const panel =
  "rounded-(--admin-radius-md) border border-(--admin-border) bg-(--admin-surface)";
const button =
  "h-9 rounded-sm px-3 text-[12px] font-semibold disabled:opacity-50";

function ChangeCard({
  item,
  busy,
  onDecide,
}: {
  item: ProfileChangeItem;
  busy: boolean;
  onDecide: (decision: "APPROVED" | "REJECTED", reason: string) => void;
}) {
  const [rejecting, setRejecting] = useState(false);
  const [reason, setReason] = useState("");
  const changed = FIELDS.filter((key) => item.change[key]);
  const pending = item.change.status === "PENDING";

  return (
    <article className={`${panel} grid gap-3 p-4`}>
      <div className="flex items-center justify-between gap-2">
        <strong className="text-[14px] text-(--admin-text)">
          {item.username}
        </strong>
        <span className="text-[11px] text-(--admin-text-muted)">
          Gửi {new Date(item.change.requestedAt).toLocaleString("vi-VN")}
        </span>
      </div>
      <table className="w-full text-left text-[12px]">
        <thead className="text-(--admin-text-muted)">
          <tr>
            <th className="py-1 font-semibold">Thông tin</th>
            <th className="py-1 font-semibold">Hiện tại</th>
            <th className="py-1 font-semibold">Đề nghị đổi thành</th>
          </tr>
        </thead>
        <tbody className="text-(--admin-text)">
          {changed.map((key) => (
            <tr key={key} className="border-t border-(--admin-border)">
              <td className="py-1.5 pr-2">{FIELD_LABELS[key]}</td>
              <td className="py-1.5 pr-2">{item.current[key] || "Chưa có"}</td>
              <td className="py-1.5 font-semibold">{item.change[key]}</td>
            </tr>
          ))}
        </tbody>
      </table>
      {!pending ? (
        <div className="grid gap-1 text-[12px] text-(--admin-text-muted)">
          <p className="font-semibold">
            {item.change.status === "APPROVED" ? "Đã duyệt" : "Đã từ chối"}
          </p>
          {item.change.rejectionReason && (
            <p>Lý do: {item.change.rejectionReason}</p>
          )}
          {item.change.reviewedAt && (
            <p>
              Ngày xử lý:{" "}
              {new Date(item.change.reviewedAt).toLocaleString("vi-VN")}
            </p>
          )}
        </div>
      ) : rejecting ? (
        <div className="grid gap-2">
          <label className="grid gap-1 text-[11px] font-semibold text-(--admin-text-muted)">
            Lý do từ chối
            <input
              autoFocus
              maxLength={500}
              value={reason}
              onChange={(event) => setReason(event.target.value)}
              className="h-9 rounded-sm border border-(--admin-border) bg-(--admin-background) px-2.5 text-[12px] text-(--admin-text) outline-none focus:border-(--admin-primary)"
            />
          </label>
          <div className="flex gap-2">
            <button
              type="button"
              disabled={busy || !reason.trim()}
              onClick={() => onDecide("REJECTED", reason.trim())}
              className={`${button} border border-(--admin-danger) bg-(--admin-danger-soft) text-(--admin-danger)`}
            >
              Xác nhận từ chối
            </button>
            <button
              type="button"
              onClick={() => setRejecting(false)}
              className={`${button} border border-(--admin-border) text-(--admin-text)`}
            >
              Hủy
            </button>
          </div>
        </div>
      ) : (
        <div className="flex gap-2">
          <button
            type="button"
            disabled={busy}
            onClick={() => onDecide("APPROVED", "")}
            className={`${button} bg-(--admin-primary) text-white`}
          >
            Duyệt
          </button>
          <button
            type="button"
            disabled={busy}
            onClick={() => setRejecting(true)}
            className={`${button} border border-(--admin-danger) bg-(--admin-danger-soft) text-(--admin-danger)`}
          >
            Từ chối
          </button>
        </div>
      )}
    </article>
  );
}

export function ProfileChangesPage() {
  const [data, setData] = useState<ProfileChangesPageData | null>(null);
  const [status, setStatus] = useState<ReviewStatus>("PENDING");
  const [searchInput, setSearchInput] = useState("");
  const [search, setSearch] = useState("");
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const loadSequence = useRef(0);

  const load = useCallback(async () => {
    const sequence = ++loadSequence.current;
    setLoading(true);
    setError("");
    const query = new URLSearchParams({
      page: String(page),
      pageSize: "10",
      status,
      search,
    });
    try {
      const body = await request<{ data: ProfileChangesPageData }>(
        `${ROOT}?${query.toString()}`,
      );
      if (sequence !== loadSequence.current) return;
      const result = body.data;
      // If a review removed the last item on the last page, show the last
      // available page rather than leaving the admin on an empty page.
      if (page > Math.max(1, result.totalPages)) {
        setPage(Math.max(1, result.totalPages));
        return;
      }
      setData(result);
    } catch (cause) {
      if (sequence !== loadSequence.current) return;
      setData(null);
      setError(explainLoanError(cause));
    } finally {
      if (sequence === loadSequence.current) setLoading(false);
    }
  }, [page, status, search]);

  useEffect(() => {
    void Promise.resolve().then(load);
  }, [load]);

  const decide = async (
    item: ProfileChangeItem,
    decision: "APPROVED" | "REJECTED",
    reason: string,
  ) => {
    setBusy(true);
    setError("");
    try {
      await request(
        `${ROOT}/${encodeURIComponent(item.accountId)}/${encodeURIComponent(item.change.id)}/decision`,
        { method: "POST", body: JSON.stringify({ decision, reason }) },
      );
      // Refresh from the server so totals, filters and other admin decisions
      // cannot be desynchronized by optimistic removal.
      await load();
    } catch (cause) {
      setError(explainLoanError(cause));
    } finally {
      setBusy(false);
    }
  };

  const submitSearch = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setData(null);
    setPage(1);
    setSearch(searchInput.trim());
  };

  return (
    <div className="grid gap-3">
      <section className="flex items-start gap-3 rounded-(--admin-radius-md) bg-(--admin-surface-subtle) p-4">
        <div className="grid h-9 w-9 place-items-center rounded-(--admin-radius-md) bg-(--admin-primary-soft) text-(--admin-primary)">
          <FontAwesomeIcon icon={faUserPen} aria-hidden="true" />
        </div>
        <div>
          <h1 className="text-[18px] font-bold uppercase text-(--admin-text)">
            Duyệt thông tin khách hàng
          </h1>
          <p className="mt-1 text-[12px] text-(--admin-text-muted)">
            Thông tin chỉ thay đổi sau khi được duyệt. Đối chiếu với giấy tờ của
            khách trước khi quyết định.
          </p>
        </div>
      </section>
      <section className={`${panel} flex flex-wrap items-end gap-3 p-3`}>
        <label className="grid min-w-40 gap-1 text-[12px] text-(--admin-text)">
          Trạng thái yêu cầu
          <select
            aria-label="Trạng thái yêu cầu"
            value={status}
            disabled={busy}
            onChange={(event) => {
              setData(null);
              setPage(1);
              setStatus(event.target.value as ReviewStatus);
            }}
            className="h-9 rounded-sm border border-(--admin-border) bg-(--admin-background) px-2 text-(--admin-text)"
          >
            <option value="PENDING">Chờ duyệt</option>
            <option value="APPROVED">Đã duyệt</option>
            <option value="REJECTED">Đã từ chối</option>
            <option value="ALL">Tất cả</option>
          </select>
        </label>
        <form
          onSubmit={submitSearch}
          className="flex flex-1 flex-wrap items-end gap-2"
        >
          <label className="grid min-w-48 flex-1 gap-1 text-[12px] text-(--admin-text)">
            Tìm theo tên hoặc mã tài khoản
            <input
              aria-label="Tìm khách hàng"
              value={searchInput}
              maxLength={120}
              onChange={(event) => setSearchInput(event.target.value)}
              placeholder="Tên khách hàng, username, mã tài khoản"
              className="h-9 rounded-sm border border-(--admin-border) bg-(--admin-background) px-2 text-(--admin-text)"
            />
          </label>
          <button
            type="submit"
            disabled={busy}
            className={`${button} bg-(--admin-primary) text-white`}
          >
            Tìm kiếm
          </button>
        </form>
        <button
          type="button"
          onClick={() => void load()}
          disabled={busy || loading}
          className={`${button} border border-(--admin-border) text-(--admin-text)`}
        >
          Làm mới
        </button>
      </section>
      {error && (
        <p
          role="alert"
          className="rounded-sm border border-(--admin-danger) bg-(--admin-danger-soft) px-3 py-2 text-[11px] text-(--admin-danger)"
        >
          {error}
        </p>
      )}
      {loading && (
        <p role="status" className="text-[12px] text-(--admin-text-muted)">
          Đang tải...
        </p>
      )}
      {data?.items.length === 0 && !error && !loading && (
        <p className="rounded-sm border border-dashed border-(--admin-border) px-3 py-6 text-center text-[12px] text-(--admin-text-muted)">
          Không có yêu cầu phù hợp với bộ lọc.
        </p>
      )}
      {data?.items.map((item) => (
        <ChangeCard
          key={item.change.id}
          item={item}
          busy={busy}
          onDecide={(decision, reason) => void decide(item, decision, reason)}
        />
      ))}
      {data && !error && (
        <footer
          className={`${panel} flex flex-wrap items-center justify-between gap-2 p-3 text-[12px] text-(--admin-text-muted)`}
        >
          <span>
            {data.total} yêu cầu · Trang {data.page}/
            {Math.max(1, data.totalPages)}
          </span>
          <div className="flex gap-2">
            <button
              type="button"
              disabled={loading || busy || page <= 1}
              onClick={() => {
                setData(null);
                setPage((value) => value - 1);
              }}
              className={`${button} border border-(--admin-border) text-(--admin-text)`}
            >
              Trang trước
            </button>
            <button
              type="button"
              disabled={loading || busy || page >= data.totalPages}
              onClick={() => {
                setData(null);
                setPage((value) => value + 1);
              }}
              className={`${button} border border-(--admin-border) text-(--admin-text)`}
            >
              Trang sau
            </button>
          </div>
        </footer>
      )}
    </div>
  );
}
