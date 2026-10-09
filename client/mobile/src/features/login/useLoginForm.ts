import { toast } from "@/shared/notifications/toast";
import { AUTH_STATUS } from "@/features/auth/authStatus";
import { useState, type FormEvent } from "react";
import { useAuth } from "@/features/auth/useAuth";
import {
  assertValidLoginInput,
  createKycSession,
  toLoginError,
} from "@/features/auth/api/auth";
import { navigate, routes } from "@/app/routes";
import { ApiError } from "@/shared/lib/http";
import { saveKycResumeSession } from "@/features/registration/kycResume";
export function useLoginForm() {
  const { status: authStatus, user, login, logout } = useAuth();
  const [showPassword, setShowPassword] = useState(false);
  const [cccd, setCccd] = useState("");
  const [password, setPassword] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);

  const isBusy = isSubmitting || authStatus === AUTH_STATUS.RESTORING;
  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (isSubmitting) return;

    setFormError(null);
    toast.dismiss();
    try {
      assertValidLoginInput({ cccdNumber: cccd, password });
    } catch (validationError) {
      setFormError(toLoginError(validationError));
      return;
    }
    setIsSubmitting(true);
    try {
      await login({ cccdNumber: cccd, password });
      setPassword("");
      navigate(routes.dashboard);
    } catch (submitError) {
      if (submitError instanceof ApiError && submitError.hasCode("KYC_REQUIRED")) {
        try {
          const session = await createKycSession({
            cccdNumber: cccd,
            password,
          });
          saveKycResumeSession(session);
          setPassword("");
          navigate(routes.register);
          return;
        } catch (sessionError) {
          setPassword("");
          toast.error(toLoginError(sessionError));
          return;
        }
      }
      toast.error(toLoginError(submitError));
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
    handleSubmit,
    signOut: () => {
      setFormError(null);
      toast.dismiss();
      logout();
    },
  };
}
