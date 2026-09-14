import { useSyncExternalStore } from "react";
import { LoginPage } from "@/pages/LoginPage";
import { RegisterPage } from "@/pages/RegisterPage";
import DashboardPage from "@/pages/DashboardPage";
import { routes } from "./routes";

function subscribe(onChange: () => void) {
  window.addEventListener("hashchange", onChange);
  return () => window.removeEventListener("hashchange", onChange);
}

// Hash routes work in both Vite and the packaged Tauri WebView.
export function AppRouter() {
  const hash = useSyncExternalStore(
    subscribe,
    () => window.location.hash,
    () => routes.login,
  );
  switch (hash) {
    case routes.dashboard:
      return <DashboardPage />;
    case routes.register:
      return <RegisterPage />;
    default:
      return <LoginPage />;
  }
}
