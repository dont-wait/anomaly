import { Toaster } from "@/shared/notifications/Toaster";
import { type ReactNode } from "react";
import { AuthProvider } from "@/features/auth/AuthProvider";
import { ThemeProvider } from "@/shared/theme/ThemeContext";

export const Providers = ({ children }: { children: ReactNode }) => {
  return (
    <AuthProvider>
      <ThemeProvider>
        {children}
        <Toaster />
      </ThemeProvider>
    </AuthProvider>
  );
};
