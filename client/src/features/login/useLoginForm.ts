import { useState, type FormEvent } from "react";
import { useAuth } from "@/features/auth/useAuth";
import { toLoginError } from "@/features/auth/api/auth";
import { navigate, routes } from "@/app/routes";
interface StatusMessage {
  tone: "success" | "error";
  text: string;
}

export function useLoginForm() {
  const {
    status: authStatus,
    user,
    error: authError,
    login,
    logout,
  } = useAuth();
  const [showPassword, setShowPassword] = useState(false);
  const [cccd, setCccd] = useState("");
  const [password, setPassword] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [message, setMessage] = useState<StatusMessage | null>(null);

  const isBusy = isSubmitting || authStatus === "restoring";
  const formError = message?.tone === "error" ? message.text : authError;
  const successMessage = message?.tone === "success" ? message.text : null;

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (isSubmitting) return;

    setIsSubmitting(true);
    setMessage(null);
    try {
      await login({ cccdNumber: cccd, password });
      setPassword("");
      setMessage({ tone: "success", text: "Đăng nhập thành công." });
      navigate(routes.dashboard);
    } catch (submitError) {
      setMessage({
        tone: "error",
        text: toLoginError(submitError),
      });
    } finally {
      setIsSubmitting(false);
    }
  };

  return {
    authStatus,
    user,
    showPassword,
    setShowPassword,
    cccd,
    setCccd,
    password,
    setPassword,
    isBusy,
    isSubmitting,
    formError,
    successMessage,
    handleSubmit,
    signOut: () => {
      setMessage(null);
      logout();
    },
  };
}
