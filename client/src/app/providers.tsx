import { type ReactNode } from "react";
import { ThemeProvider } from "@/context/ThemeContext";

export const Providers = ({ children }: { children: ReactNode }) => {
    return <ThemeProvider>{children}</ThemeProvider>;
};
