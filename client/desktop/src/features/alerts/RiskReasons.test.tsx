import { render, screen, within } from "@testing-library/react";
import { RiskReasons } from "./RiskReasons";

it("shows the decision reason, scenarios and factor reasons", () => {
  render(
    <RiskReasons
      decisionReason="risk_score 0.93 > block threshold 0.7"
      scenarios={[
        {
          code: "VELOCITY_BURST",
          severity: "high",
          title: "Chuyển tiền dồn dập",
          reason:
            "Trước giao dịch này, tài khoản nguồn đã gửi 3 yêu cầu trong 2 phút.",
          evidence: ["log_src_request_count_120s"],
        },
      ]}
      topFactors={[
        {
          feature: "log_src_request_count_120s",
          zScore: 4.2,
          reason:
            "Tài khoản nguồn đã gửi 3 yêu cầu chuyển tiền trong 2 phút trước đó (z=+4.2)",
        },
        { feature: "log_amount", zScore: -1.25 },
        { feature: "new_device", value: 1 },
      ]}
    />,
  );

  const section = screen.getByRole("region", { name: "Lý do rủi ro" });
  expect(
    within(section).getByText("risk_score 0.93 > block threshold 0.7"),
  ).toBeTruthy();
  expect(within(section).getByText("Chuyển tiền dồn dập")).toBeTruthy();
  expect(within(section).getByText("Cao")).toBeTruthy();
  expect(within(section).getByText("VELOCITY_BURST")).toBeTruthy();
  expect(
    within(section).getByText(
      "Tài khoản nguồn đã gửi 3 yêu cầu chuyển tiền trong 2 phút trước đó (z=+4.2)",
    ),
  ).toBeTruthy();
  // Factors without a reason fall back to the feature name and number.
  expect(within(section).getByText("log_amount (z=-1.3)")).toBeTruthy();
  expect(within(section).getByText("new_device = 1")).toBeTruthy();
  expect(within(section).getByText(/không nằm trong chữ ký/)).toBeTruthy();
});

it("says so when the model sent no reasons", () => {
  render(<RiskReasons />);
  expect(
    screen.getByText("Mô hình không gửi kèm lý do cho giao dịch này."),
  ).toBeTruthy();
  expect(screen.queryByText("Tình huống nghi vấn")).toBeNull();
});
