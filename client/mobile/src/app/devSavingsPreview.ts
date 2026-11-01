// An explicit UI-only preview. This flag never enables an existing protected
// route, and the production build must never serve it.
export const SAVINGS_PREVIEW_HASH = "#/preview/savings";

export function shouldShowSavingsPreview(
  hash: string,
  isDev: boolean,
): boolean {
  return isDev && hash === SAVINGS_PREVIEW_HASH;
}
