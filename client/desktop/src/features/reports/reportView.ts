import type { TransactionReportItem } from "./api/transactionsReportApi";

export type StatusFilter = "all" | TransactionReportItem["effectiveStatus"];
export type DecisionFilter =
  "all" | "ALLOW" | "BLOCK" | "unassessed" | "unknown";

function searchText(value: string) {
  return value
    .trim()
    .toLowerCase()
    .normalize("NFD")
    .replace(/\p{Diacritic}/gu, "")
    .replace(/đ/g, "d");
}

export function filterReportItems(
  items: TransactionReportItem[],
  status: StatusFilter,
  decision: DecisionFilter,
  query: string,
) {
  const normalized = searchText(query);
  return items.filter((item) => {
    if (status !== "all" && item.effectiveStatus !== status) return false;
    const assessment = item.selectedAssessment;
    if (decision === "unassessed" && assessment) return false;
    if (decision === "unknown" && (!assessment || assessment.decision !== null))
      return false;
    if (
      (decision === "ALLOW" || decision === "BLOCK") &&
      assessment?.decision !== decision
    )
      return false;
    return (
      !normalized ||
      [
        item.transactionId,
        item.sourceAccountNo,
        item.destinationAccountNo,
        item.sourceAccountName,
        item.destinationName,
      ].some((value) => searchText(value).includes(normalized))
    );
  });
}

export function formatReportTimestamp(value?: string | null) {
  if (!value || !Number.isFinite(Date.parse(value))) return "—";
  return new Intl.DateTimeFormat("vi-VN", {
    dateStyle: "medium",
    timeStyle: "short",
    timeZone: "Asia/Ho_Chi_Minh",
  }).format(new Date(value));
}
