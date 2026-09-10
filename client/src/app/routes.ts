export const routes = {
  login: "#/login",
  register: "#/register",
} as const;

export function navigate(to: (typeof routes)[keyof typeof routes]) {
  window.location.hash = to;
}
