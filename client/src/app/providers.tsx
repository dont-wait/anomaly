import { type ReactNode } from "react";
import { AuthProvider } from "@/auth/AuthProvider";
import { ThemeProvider } from "@/context/ThemeContext";

export const Providers = ({ children }: { children: ReactNode }) => {
    return (
        <AuthProvider>
            <ThemeProvider>{children}</ThemeProvider>
        </AuthProvider>
    );
};
