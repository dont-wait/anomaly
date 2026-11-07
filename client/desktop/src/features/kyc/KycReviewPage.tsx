import { useCallback, useEffect, useState } from "react";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";
import {
  faArrowRight,
  faIdCard,
  faImage,
  faVideo,
} from "@fortawesome/free-solid-svg-icons";
import {
  decideKycReview,
  downloadKycMedia,
  KycReviewApiError,
  listKycReviews,
  type KycReviewDecision,
  type KycReviewIdentity,
  type KycReviewItem,
} from "./api/kycReviewsApi";

function explainKycError(error: unknown): string {
  if (error instanceof KycReviewApiError) {
    if (error.status === 401)
      return "Phiên đăng nhập admin đã hết hạn. Vui lòng đăng nhập lại.";
    if (error.status === 403) {
      return error.hasCode("SELF_KYC_REVIEW")
        ? "Bạn không thể duyệt hồ sơ KYC của chính mình."
        : "Tài khoản hiện tại không có quyền duyệt KYC.";
    }
    if (error.status === 404) return "Không tìm thấy hồ sơ KYC hoặc chứng từ.";
    if (error.status === 409)
      return "Hồ sơ vừa được xử lý ở nơi khác. Vui lòng tải lại danh sách.";
    if (error.status === 0) return error.message;
    return "Hệ thống đang gặp sự cố. Vui lòng thử lại sau.";
  }
  return error instanceof Error
    ? error.message
    : "Có lỗi xảy ra. Vui lòng thử lại.";
}

const panel =
  "rounded-(--admin-radius-md) border border-(--admin-border) bg-(--admin-surface)";
const button =
  "h-9 rounded-sm px-3 text-[12px] font-semibold disabled:opacity-50";

function formatDate(value?: string): string {
  if (!value) return "Chưa có";
  if (/^\d{4}-\d{2}-\d{2}$/.test(value)) {
    const [year, month, day] = value.split("-");
    return `${day}/${month}/${year}`;
  }
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return value;
  return parsed.toLocaleDateString("vi-VN");
}

function MediaBox({
  path,
  label,
  kind,
}: {
  path?: string;
  label: string;
  kind: "image" | "video";
}) {
  const [url, setUrl] = useState<string>();
  const [error, setError] = useState("");

  useEffect(() => {
    if (!path) return;
    let active = true;
    let objectUrl: string | undefined;
    setError("");
    void downloadKycMedia(path)
      .then((blob) => {
        if (!active) return;
        objectUrl = URL.createObjectURL(blob);
        setUrl(objectUrl);
      })
      .catch((cause) => {
        if (active) setError(explainKycError(cause));
      });
    return () => {
      active = false;
      if (objectUrl) URL.revokeObjectURL(objectUrl);
    };
  }, [path]);

  return (
    <figure className={`${panel} grid gap-2 p-3`}>
      <figcaption className="flex items-center gap-2 text-[11px] font-semibold uppercase text-(--admin-text-muted)">
        <FontAwesomeIcon icon={kind === "video" ? faVideo : faImage} />
        {label}
      </figcaption>
      {!path ? (
        <p className="py-8 text-center text-[11px] text-(--admin-text-muted)">
          Không có dữ liệu
        </p>
      ) : error ? (
        <p
          role="alert"
          className="py-8 text-center text-[11px] text-(--admin-danger)"
        >
          {error}
        </p>
      ) : !url ? (
        <p
          role="status"
          className="py-8 text-center text-[11px] text-(--admin-text-muted)"
        >
          Đang tải...
        </p>
      ) : kind === "video" ? (
        <video
          className="max-h-64 w-full rounded-sm bg-black"
          src={url}
          controls
          playsInline
          preload="metadata"
          aria-label={label}
        />
      ) : (
        <a href={url} target="_blank" rel="noreferrer" title="Mở ảnh gốc">
          <img
            className="max-h-64 w-full rounded-sm object-contain bg-(--admin-background)"
            src={url}
            alt={label}
          />
        </a>
      )}
    </figure>
  );
}

const IDENTITY_FIELDS: Array<{ key: keyof KycReviewIdentity; label: string }> =
  [
    { key: "fullName", label: "Họ và tên" },
    { key: "number", label: "Số CCCD" },
    { key: "dateOfBirth", label: "Ngày sinh" },
    { key: "issuedDate", label: "Ngày cấp" },
    { key: "issuedPlace", label: "Nơi cấp" },
    { key: "permanentAddress", label: "Địa chỉ thường trú" },
  ];

function identityValue(
  identity: KycReviewIdentity,
  key: keyof KycReviewIdentity,
): string {
  const value = identity[key];
  if (!value) return "Chưa có";
  if (key === "dateOfBirth" || key === "issuedDate") {
    return formatDate(String(value));
  }
  return String(value);
}

function ReviewDetail({
  item,
  busy,
  onDecide,
}: {
  item: KycReviewItem;
  busy: boolean;
  onDecide: (decision: KycReviewDecision, reason: string) => void;
}) {
  const [mode, setMode] = useState<"idle" | "approve" | "reject">("idle");
  const [reason, setReason] = useState("");

  useEffect(() => {
    setMode("idle");
    setReason("");
  }, [item.accountId]);

  return (
    <article className="grid gap-3">
      <header
        className={`${panel} flex flex-wrap items-start justify-between gap-3 p-4`}
      >
        <div>
          <h2 className="text-[16px] font-bold text-(--admin-text)">
            {item.declared.fullName || item.username}
          </h2>
          <p className="mt-1 text-[11px] text-(--admin-text-muted)">
            {item.email} · Lần nộp {item.attemptNo} ·{" "}
            {new Date(item.submittedAt).toLocaleString("vi-VN")}
          </p>
        </div>
        <span className="rounded-sm bg-(--admin-primary-soft) px-2 py-1 text-[10px] font-semibold uppercase text-(--admin-primary)">
          Chờ duyệt
        </span>
      </header>

      <section className={`${panel} p-4`}>
        <h3 className="mb-2 text-[12px] font-semibold uppercase text-(--admin-text-muted)">
          Đối chiếu thông tin
        </h3>
        <table className="w-full text-left text-[12px]">
          <thead className="text-(--admin-text-muted)">
            <tr>
              <th className="py-1 font-semibold">Thông tin</th>
              <th className="py-1 font-semibold">Khai báo khi đăng ký</th>
              <th className="py-1 font-semibold">Trong hồ sơ KYC</th>
            </tr>
          </thead>
          <tbody className="text-(--admin-text)">
            {IDENTITY_FIELDS.map((field) => {
              const registration = identityValue(item.registration, field.key);
              const declared = identityValue(item.declared, field.key);
              const mismatch = registration !== declared;
              return (
                <tr
                  key={field.key}
                  className="border-t border-(--admin-border)"
                >
                  <td className="py-1.5 pr-2">{field.label}</td>
                  <td className="py-1.5 pr-2">{registration}</td>
                  <td
                    className={`py-1.5 font-semibold ${mismatch ? "text-(--admin-danger)" : ""}`}
                  >
                    {declared}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </section>

      <section className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
        <MediaBox path={item.media.front} label="CCCD mặt trước" kind="image" />
        <MediaBox path={item.media.back} label="CCCD mặt sau" kind="image" />
        <MediaBox
          path={item.media.video}
          label="Video xác thực khuôn mặt"
          kind="video"
        />
        {item.media.faceImage && (
          <MediaBox
            path={item.media.faceImage}
            label="Ảnh chân dung"
            kind="image"
          />
        )}
      </section>

      {mode === "reject" && (
        <div className={`${panel} grid gap-2 p-4`}>
          <label className="grid gap-1 text-[11px] font-semibold text-(--admin-text-muted)">
            Lý do từ chối (bắt buộc)
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
              onClick={() => setMode("idle")}
              className={`${button} border border-(--admin-border) text-(--admin-text)`}
            >
              Hủy
            </button>
          </div>
        </div>
      )}

      {mode === "approve" && (
        <div className={`${panel} grid gap-2 p-4`}>
          <p className="text-[12px] text-(--admin-text)">
            Xác nhận hồ sơ của{" "}
            <strong>{item.declared.fullName || item.username}</strong> khớp với
            giấy tờ và video, đồng ý mở xác thực KYC?
          </p>
          <div className="flex gap-2">
            <button
              type="button"
              disabled={busy}
              onClick={() => onDecide("APPROVED", "")}
              className={`${button} bg-(--admin-primary) text-white`}
            >
              Xác nhận duyệt
            </button>
            <button
              type="button"
              onClick={() => setMode("idle")}
              className={`${button} border border-(--admin-border) text-(--admin-text)`}
            >
              Hủy
            </button>
          </div>
        </div>
      )}

      {mode === "idle" && (
        <div className="flex gap-2">
          <button
            type="button"
            disabled={busy}
            onClick={() => setMode("approve")}
            className={`${button} bg-(--admin-primary) text-white`}
          >
            Duyệt hồ sơ
          </button>
          <button
            type="button"
            disabled={busy}
            onClick={() => setMode("reject")}
            className={`${button} border border-(--admin-danger) bg-(--admin-danger-soft) text-(--admin-danger)`}
          >
            Từ chối
          </button>
        </div>
      )}
    </article>
  );
}

export function KycReviewPage() {
  const [items, setItems] = useState<KycReviewItem[] | null>(null);
  const [selectedId, setSelectedId] = useState<string>();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    try {
      const next = await listKycReviews();
      setItems(next);
      setSelectedId((current) => {
        if (current && next.some((item) => item.accountId === current)) {
          return current;
        }
        return next[0]?.accountId;
      });
    } catch (cause) {
      setItems([]);
      setError(explainKycError(cause));
    }
  }, []);

  useEffect(() => {
    void Promise.resolve().then(load);
  }, [load]);

  const decide = async (
    item: KycReviewItem,
    decision: KycReviewDecision,
    reason: string,
  ) => {
    setBusy(true);
    setError("");
    try {
      await decideKycReview(item.accountId, {
        sessionId: item.sessionId,
        decision,
        reason: decision === "REJECTED" ? reason : undefined,
      });
      setItems((old) => {
        const next = (old ?? []).filter(
          (other) => other.accountId !== item.accountId,
        );
        setSelectedId(next[0]?.accountId);
        return next;
      });
    } catch (cause) {
      setError(explainKycError(cause));
      await load();
    } finally {
      setBusy(false);
    }
  };

  const selected = items?.find((item) => item.accountId === selectedId);
  const pending = items?.length ?? 0;

  return (
    <div className="grid gap-3">
      <section className="flex items-start gap-3 rounded-(--admin-radius-md) bg-(--admin-surface-subtle) p-4">
        <div className="grid h-9 w-9 place-items-center rounded-(--admin-radius-md) bg-(--admin-primary-soft) text-(--admin-primary)">
          <FontAwesomeIcon icon={faIdCard} aria-hidden="true" />
        </div>
        <div className="flex-1">
          <h1 className="text-[18px] font-bold uppercase text-(--admin-text)">
            Duyệt KYC thủ công
          </h1>
          <p className="mt-1 text-[12px] text-(--admin-text-muted)">
            Đối chiếu ảnh CCCD trước/sau, video xác thực và thông tin khai báo
            trước khi xác nhận. Hồ sơ chỉ được mở khi bạn bấm duyệt.
          </p>
        </div>
        <span className="rounded-sm bg-(--admin-surface) px-2 py-1 text-[10px] font-semibold uppercase text-(--admin-text-muted)">
          {pending} hồ sơ chờ
        </span>
      </section>

      {error && (
        <p
          role="alert"
          className="rounded-sm border border-(--admin-danger) bg-(--admin-danger-soft) px-3 py-2 text-[11px] text-(--admin-danger)"
        >
          {error}
        </p>
      )}

      {items === null && (
        <p role="status" className="text-[12px] text-(--admin-text-muted)">
          Đang tải...
        </p>
      )}

      {items?.length === 0 && !error && (
        <p className="rounded-sm border border-dashed border-(--admin-border) px-3 py-6 text-center text-[12px] text-(--admin-text-muted)">
          Không có hồ sơ KYC nào đang chờ duyệt.
        </p>
      )}

      {items && items.length > 0 && (
        <div className="grid gap-3 lg:grid-cols-[minmax(220px,280px)_1fr]">
          <nav
            aria-label="Danh sách hồ sơ chờ duyệt"
            className="grid content-start gap-2"
          >
            {items.map((item) => {
              const active = item.accountId === selectedId;
              return (
                <button
                  key={item.accountId}
                  type="button"
                  aria-current={active ? "true" : undefined}
                  onClick={() => setSelectedId(item.accountId)}
                  className={`${panel} grid gap-1 p-3 text-left ${
                    active ? "border-(--admin-primary)" : ""
                  }`}
                >
                  <strong className="text-[13px] text-(--admin-text)">
                    {item.declared.fullName || item.username}
                  </strong>
                  <span className="text-[11px] text-(--admin-text-muted)">
                    {item.declared.number || "Chưa có CCCD"}
                  </span>
                  <span className="flex items-center gap-1 text-[10px] text-(--admin-text-muted)">
                    Nộp {new Date(item.submittedAt).toLocaleString("vi-VN")}
                    <FontAwesomeIcon icon={faArrowRight} />
                  </span>
                </button>
              );
            })}
          </nav>
          {selected ? (
            <ReviewDetail
              key={selected.accountId}
              item={selected}
              busy={busy}
              onDecide={(decision, reason) =>
                void decide(selected, decision, reason)
              }
            />
          ) : (
            <p
              className={`${panel} p-6 text-center text-[12px] text-(--admin-text-muted)`}
            >
              Chọn một hồ sơ để xem chi tiết.
            </p>
          )}
        </div>
      )}
    </div>
  );
}
