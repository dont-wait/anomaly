import { useState } from "react";
import { useAuth } from "@/features/auth/useAuth";
import {
  assessLoan,
  attestLoanEvidence,
  decideLoan,
  disburseLoan,
  getLoan,
  issueLoanOffer,
  listLoans,
  startLoanReview,
  type LoanApplication,
  type VerifiedEvidenceInput,
} from "@/features/loans/api";
import { explainLoanError, vnd } from "@/features/loans/format";
import {
  Action,
  LoanLayout,
  Notice,
  Panel,
} from "@/features/loans/components/LoanLayout";

/** Back-office test screen; backend enforces admin privileges for every write. */
export default function LoanAdminPage() {
  const { token } = useAuth();
  const [items, setItems] = useState<LoanApplication[]>([]);
  const [selectedId, setSelectedId] = useState("");
  const [detail, setDetail] = useState<LoanApplication | null>(null);
  const [income, setIncome] = useState(20_000_000);
  const [region, setRegion] = useState("KV01");
  const [debt, setDebt] = useState(0);
  const [insurance, setInsurance] = useState(24);
  const [history, setHistory] = useState("previously_late");
  const [verified, setVerified] = useState(false);
  const [reason, setReason] = useState(
    "Đã xác minh hồ sơ test và phê duyệt thủ công",
  );
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");

  const run = async (fn: () => Promise<unknown>, success: string) => {
    setBusy(true);
    setError("");
    setMessage("");
    try {
      await fn();
      setMessage(success);
      if (token && selectedId) setDetail(await getLoan(token, selectedId));
    } catch (e) {
      setError(explainLoanError(e));
    } finally {
      setBusy(false);
    }
  };
  const evidence: VerifiedEvidenceInput = {
    monthlySalaries: [income, income, income],
    regionCode: region,
    existingMonthlyDebt: debt,
    insuranceMonths: insurance,
    repaymentHistory: history,
    insuranceVerified: verified,
    historyVerified: verified,
    financialDataVerified: verified,
    documentsVerified: verified,
    sources: ["admin-ui-manual-evidence"],
  };
  return (
    <LoanLayout title="Admin · Thẩm định vay (Test)">
      <Notice message="Chỉ dùng với tài khoản admin và dữ liệu kiểm thử. Không giả lập xác minh giấy tờ thật. Backend phải kiểm tra quyền từng bước." />
      {error && <Notice message={error} error />}
      {message && <Notice message={message} />}
      <Panel title="Hồ sơ cần xử lý">
        <Action
          disabled={busy}
          onClick={() => {
            if (!token) return;
            void run(async () => {
              const result = await listLoans(token, undefined, true);
              setItems(result.items);
            }, "Đã tải danh sách dành cho admin.");
          }}
        >
          Tải hồ sơ (admin)
        </Action>
        {items.map((item) => (
          <button
            key={item.id}
            type="button"
            className="block w-full rounded-xl border p-3 text-left text-xs"
            onClick={() => {
              setSelectedId(item.id);
              setDetail(item);
            }}
          >
            {item.applicationNo} · {vnd(item.requestedAmount)} · {item.status}
          </button>
        ))}
        <label className="block text-xs">
          Application ID
          <input
            className="mt-1 w-full rounded-xl border p-3"
            value={selectedId}
            onChange={(e) => {
              setSelectedId(e.target.value);
              setDetail(null);
            }}
          />
        </label>
        <Action
          secondary
          disabled={busy || !selectedId.trim()}
          onClick={() => {
            if (token)
              void run(
                async () => setDetail(await getLoan(token, selectedId.trim())),
                "Đã tải chi tiết hồ sơ.",
              );
          }}
        >
          Kiểm tra trạng thái hồ sơ
        </Action>
      </Panel>
      {detail && (
        <Panel title={`Xử lý hồ sơ ${detail.applicationNo}`}>
          <p className="text-xs">
            Trạng thái: <strong>{detail.status}</strong> · Kết quả:{" "}
            {detail.assessmentDecision || "Chưa assess"}
          </p>
          {detail.status === "submitted" && (
            <Action
              disabled={busy}
              onClick={() => {
                if (token)
                  void run(
                    () => startLoanReview(token, detail.id),
                    "Đã bắt đầu thẩm định.",
                  );
              }}
            >
              Bắt đầu review
            </Action>
          )}
          {detail.status === "under_review" && (
            <>
              <label className="block text-xs">
                Lương mỗi tháng (mẫu 3 tháng)
                <input
                  type="number"
                  value={income}
                  onChange={(e) => setIncome(Number(e.target.value))}
                  className="mt-1 w-full rounded-xl border p-3"
                />
              </label>
              <label className="block text-xs">
                Khu vực
                <select
                  value={region}
                  onChange={(e) => setRegion(e.target.value)}
                  className="mt-1 w-full rounded-xl border p-3"
                >
                  <option value="KV01">KV01</option>
                  <option value="KV02">KV02</option>
                  <option value="KV03">KV03</option>
                </select>
              </label>
              <label className="block text-xs">
                Nợ hàng tháng
                <input
                  type="number"
                  value={debt}
                  onChange={(e) => setDebt(Number(e.target.value))}
                  className="mt-1 w-full rounded-xl border p-3"
                />
              </label>
              <label className="block text-xs">
                Số tháng đóng BHXH
                <input
                  type="number"
                  value={insurance}
                  onChange={(e) => setInsurance(Number(e.target.value))}
                  className="mt-1 w-full rounded-xl border p-3"
                />
              </label>
              <label className="block text-xs">
                Lịch sử trả nợ
                <select
                  value={history}
                  onChange={(e) => setHistory(e.target.value)}
                  className="mt-1 w-full rounded-xl border p-3"
                >
                  <option value="previously_late">
                    previously_late (manual review)
                  </option>
                  <option value="on_time">on_time</option>
                </select>
              </label>
              <label className="flex items-start gap-2 text-xs">
                <input
                  type="checkbox"
                  checked={verified}
                  onChange={(e) => setVerified(e.target.checked)}
                />{" "}
                Tôi đã xác minh Evidence trên môi trường TEST (không tích hợp
                đối soát thật).
              </label>
              <Action
                disabled={busy || !verified}
                onClick={() => {
                  if (token)
                    void run(
                      () => attestLoanEvidence(token, detail.id, evidence),
                      "Đã lưu Evidence. Tiếp theo: assess.",
                    );
                }}
              >
                Ghi Evidence đã kiểm tra
              </Action>
              <Action
                secondary
                disabled={busy}
                onClick={() => {
                  if (token)
                    void run(
                      () => assessLoan(token, detail.id),
                      "Đã chạy assessment. Nếu manual_review, cần quyết định thủ công.",
                    );
                }}
              >
                Chấm điểm / Assess
              </Action>
              <label className="block text-xs">
                Lý do quyết định thủ công
                <textarea
                  value={reason}
                  onChange={(e) => setReason(e.target.value)}
                  className="mt-1 w-full rounded-xl border p-3"
                />
              </label>
              {detail.assessmentDecision === "manual_review" && (
                <>
                  <Action
                    disabled={busy || !reason.trim()}
                    onClick={() => {
                      if (token)
                        void run(
                          () =>
                            decideLoan(token, detail.id, "approved", reason),
                          "Đã phê duyệt thủ công.",
                        );
                    }}
                  >
                    Phê duyệt thủ công
                  </Action>
                  <Action
                    secondary
                    disabled={busy || !reason.trim()}
                    onClick={() => {
                      if (token && window.confirm("Từ chối hồ sơ vay này?"))
                        void run(
                          () =>
                            decideLoan(token, detail.id, "rejected", reason),
                          "Đã từ chối hồ sơ.",
                        );
                    }}
                  >
                    Từ chối hồ sơ
                  </Action>
                </>
              )}
            </>
          )}
          {detail.status === "approved" && (
            <>
              {!detail.offer && (
                <Action
                  disabled={busy}
                  onClick={() => {
                    if (token)
                      void run(
                        () => issueLoanOffer(token, detail.id),
                        "Đã phát hành Offer. Khách hàng cần accept + OTP.",
                      );
                  }}
                >
                  Phát hành Offer
                </Action>
              )}
              {detail.offer && (
                <p className="text-xs">Offer: {detail.offer.status}</p>
              )}
              {detail.offer?.status === "confirmed" && (
                <Action
                  disabled={busy}
                  onClick={() => {
                    if (
                      token &&
                      window.confirm(
                        "Giải ngân khoản vay đã xác nhận hợp đồng?",
                      )
                    )
                      void run(
                        () => disburseLoan(token, detail.id),
                        "Đã gọi giải ngân. Kiểm tra contract projection.",
                      );
                  }}
                >
                  Giải ngân (admin)
                </Action>
              )}
            </>
          )}
        </Panel>
      )}
    </LoanLayout>
  );
}
