export const routes = {
  login: "#/admin/login",
  monitor: "#/admin/monitor",
} as const;

export function navigate(to: string) {
  window.location.hash = to;
}
