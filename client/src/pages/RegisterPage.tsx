import { RegistrationFlow } from "@/features/registration/RegistrationFlow";
import { navigate, routes } from "@/app/routes";

export function RegisterPage() {
  return <RegistrationFlow onLogin={() => navigate(routes.login)} />;
}
