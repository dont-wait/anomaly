import { MIN_TRANSFER_AMOUNT } from "@/features/transfer/model";

const MAX_SUGGESTIONS = 4;

/**
 * Gợi ý số tiền theo số đang nhập: nhân thêm các bậc 10 (8 → 8.000, 80.000, 800.000...).
 * Chưa nhập gì thì không gợi ý. Chỉ gợi ý các mức từ tối thiểu tới `max` (số dư).
 */
export function suggestAmounts(typed: number, max: number): number[] {
  if (typed <= 0) return [];
  const suggestions: number[] = [];
  for (
    let value = typed * 10;
    value <= max && suggestions.length < MAX_SUGGESTIONS;
    value *= 10
  ) {
    if (value >= MIN_TRANSFER_AMOUNT) suggestions.push(value);
  }
  return suggestions;
}
