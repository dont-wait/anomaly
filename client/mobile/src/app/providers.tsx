import { Toaster } from "@/shared/notifications/Toaster";
import { type ReactNode } from "react";
import { AuthProvider } from "@/features/auth/AuthProvider";
import { NotificationProvider } from "@/features/notifications";
import { ThemeProvider } from "@/shared/theme/ThemeContext";

export const Providers = ({ children }: { children: ReactNode }) => {
  return (
    <AuthProvider>
      <NotificationProvider>
        <ThemeProvider>
          {children}
          <Toaster />
        </ThemeProvider>
      </NotificationProvider>
    </AuthProvider>
  );
};
