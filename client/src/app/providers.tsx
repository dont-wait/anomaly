import { type ReactNode } from "react";
import { AuthProvider } from "@/features/auth/AuthProvider";
import { ThemeProvider } from "@/shared/theme/ThemeContext";

export const Providers = ({ children }: { children: ReactNode }) => {
  return (
    <AuthProvider>
      <ThemeProvider>{children}</ThemeProvider>
    </AuthProvider>
  );
};
