import {
  buildTransactionReportCsv,
  downloadTransactionReportCsv,
} from "./reportExport";
import { reportFixture, reportItem } from "./testFixtures";

describe("transaction report CSV export", () => {
  it("exports full filtered rows with metadata, separated scopes and safe text cells", () => {
    const report = reportFixture();
    const items = [
      reportItem({
        transactionId: '=HYPERLINK("https://evil.test")',
        sourceAccountNo: "00123456789012",
        destinationAccountNo: "@destination",
      }),
      reportItem({
        transactionId: "second-row",
        sourceAccountNo: "00000000000002",
      }),
    ];
    const csv = buildTransactionReportCsv(report, items, {
      status: "pending",
      decision: "unassessed",
      query: "0012",
    });
    expect(csv.startsWith("\ufeff")).toBe(true);
    expect(csv).toContain(`"As of","${report.asOf}"`);
    expect(csv).toContain(
      '"Summary scope","Toàn ngày; không bị ảnh hưởng bởi bộ lọc bảng"',
    );
    expect(csv).toContain(
      '"Filtered list scope","Toàn bộ dòng khớp bộ lọc; không chỉ trang đang xem"',
    );
    expect(csv).toContain('"Status filter","Chờ xử lý"');
    expect(csv).toContain('"Risk filter","Chưa có đánh giá"');
    expect(csv).toContain('"Search","0012"');
    expect(csv).toContain("'=HYPERLINK(");
    expect(csv).toContain("https://evil.test");
    expect(csv).toContain('"\'00123456789012"');
    expect(csv).toContain('"\'@destination"');
    expect(csv.match(/second-row/g)).toHaveLength(1);
    expect(csv).not.toContain("application/vnd.ms-excel");
  });

  it("downloads CSV with a deterministic date-based filename", () => {
    const click = vi.fn();
    const anchor = document.createElement("a");
    anchor.click = click;
    const createElement = vi
      .spyOn(document, "createElement")
      .mockReturnValue(anchor);
    const createObjectURL = vi
      .spyOn(URL, "createObjectURL")
      .mockReturnValue("blob:report");
    const revokeObjectURL = vi
      .spyOn(URL, "revokeObjectURL")
      .mockImplementation(() => undefined);
    const report = reportFixture();
    downloadTransactionReportCsv(report.date, report, [], {
      status: "all",
      decision: "all",
      query: "",
    });
    expect(anchor.download).toBe("bao-cao-giao-dich-2026-11-08.csv");
    expect(anchor.href).toBe("blob:report");
    expect(click).toHaveBeenCalledOnce();
    expect(createObjectURL).toHaveBeenCalledOnce();
    expect(revokeObjectURL).toHaveBeenCalledWith("blob:report");
    createElement.mockRestore();
    createObjectURL.mockRestore();
    revokeObjectURL.mockRestore();
  });
});
