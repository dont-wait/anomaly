import { useSyncExternalStore } from "react";
import App from "@/App";
import { Providers } from "@/app/providers";
import SavingsPage from "@/features/savings/SavingsPage";
import { ThemeProvider } from "@/shared/theme/ThemeContext";
import { shouldShowSavingsPreview } from "./devSavingsPreview";

function subscribeHash(onChange: () => void) {
  window.addEventListener("hashchange", onChange);
  return () => window.removeEventListener("hashchange", onChange);
}

// The preview renders without AuthProvider, so it cannot restore a saved JWT
// or contact the API just to show the mock Savings screens. All normal routes
// still use the existing auth-protected application.
export function MobileRoot() {
  const hash = useSyncExternalStore(
    subscribeHash,
    () => window.location.hash,
    () => "",
  );

  if (shouldShowSavingsPreview(hash, import.meta.env.DEV)) {
    return (
      <ThemeProvider>
        <SavingsPage />
      </ThemeProvider>
    );
  }

  return (
    <Providers>
      <App />
    </Providers>
  );
}
