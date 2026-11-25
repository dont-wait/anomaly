import type { RiskAssessment, RiskFactor } from "@/features/monitor/model";
import { severityClass, severityLabel } from "./scenarioSeverity";

const mono = "font-(family-name:--admin-font-mono)";

function factorText(factor: RiskFactor): string {
  if (factor.reason) return factor.reason;
  if (factor.zScore !== undefined)
    return `${factor.feature} (z=${factor.zScore >= 0 ? "+" : ""}${factor.zScore.toFixed(1)})`;
  if (factor.value !== undefined) return `${factor.feature} = ${factor.value}`;
  return factor.feature;
}

type Props = Pick<
  RiskAssessment,
  "decisionReason" | "scenarios" | "topFactors"
>;

/** Explains an assessment: the server's decision reason plus the ML service's unsigned notes. */
export function RiskReasons({ decisionReason, scenarios, topFactors }: Props) {
  const hasScenarios = !!scenarios?.length;
  const hasFactors = !!topFactors?.length;

  return (
    <section
      className="mt-4 border-t border-(--admin-border) pt-4"
      aria-label="Lý do rủi ro"
    >
      <h3 className="text-[13px] font-semibold text-(--admin-text)">
        Lý do rủi ro
      </h3>
      {decisionReason && (
        <p className={`${mono} mt-2 text-[11px] text-(--admin-text-secondary)`}>
          {decisionReason}
        </p>
      )}

      {hasScenarios && (
        <div className="mt-3">
          <h4 className="text-[11px] font-semibold uppercase tracking-[0.04em] text-(--admin-text-muted)">
            Tình huống nghi vấn
          </h4>
          <ul className="mt-2 grid gap-2">
            {scenarios!.map((scenario) => (
              <li
                key={scenario.code}
                className="rounded-sm border border-(--admin-border) bg-(--admin-surface) p-2.5"
              >
                <div className="flex items-center justify-between gap-2">
                  <span className="text-[12px] font-semibold text-(--admin-text)">
                    {scenario.title}
                  </span>
                  <span
                    className={`${severityClass[scenario.severity]} shrink-0 rounded-sm px-1.5 py-0.5 text-[10px] font-semibold`}
                  >
                    {severityLabel[scenario.severity]}
                  </span>
                </div>
                <p className="mt-1 text-[12px] text-(--admin-text-secondary)">
                  {scenario.reason}
                </p>
                <p
                  className={`${mono} mt-1 text-[10px] text-(--admin-text-muted)`}
                >
                  {scenario.code}
                </p>
              </li>
            ))}
          </ul>
        </div>
      )}

      {hasFactors && (
        <div className="mt-3">
          <h4 className="text-[11px] font-semibold uppercase tracking-[0.04em] text-(--admin-text-muted)">
            Đầu vào bất thường
          </h4>
          <ul className="mt-2 grid list-disc gap-1 pl-4 text-[12px] text-(--admin-text-secondary)">
            {topFactors!.map((factor) => (
              <li key={factor.feature}>{factorText(factor)}</li>
            ))}
          </ul>
        </div>
      )}

      {hasScenarios || hasFactors ? (
        <p className="mt-3 text-[10px] text-(--admin-text-muted)">
          Gợi ý từ dịch vụ mô hình, không nằm trong chữ ký và không phải đóng
          góp của từng đặc trưng vào điểm số. Chỉ dùng để tham khảo khi điều
          tra.
        </p>
      ) : (
        <p className="mt-2 text-[12px] text-(--admin-text-muted)">
          Mô hình không gửi kèm lý do cho giao dịch này.
        </p>
      )}
    </section>
  );
}
