import { EmailStep } from "./steps/EmailStep";
import { DocumentStep } from "./steps/DocumentStep";
import { ProfileStep } from "./steps/ProfileStep";
import { FaceStep } from "./steps/FaceStep";
import { ProcessingStep } from "./steps/ProcessingStep";
import { PasswordStep } from "./steps/PasswordStep";
import { SuccessStep } from "./steps/SuccessStep";
import { ErrorStep } from "./steps/ErrorStep";
import type { RegistrationFlowState } from "./useRegistrationFlow";
export function RegistrationStep({
  flow,
  onLogin,
}: {
  flow: RegistrationFlowState;
  onLogin: () => void;
}) {
  switch (flow.screen) {
    case "email":
      return (
        <EmailStep
          email={flow.email}
          setEmail={flow.setEmail}
          consent={flow.consent}
          setConsent={flow.setConsent}
          setDialog={flow.setDialog}
          heading={flow.heading}
          submit={flow.submit}
          onLogin={onLogin}
        />
      );
    case "document":
      return (
        <DocumentStep
          error={flow.error}
          setError={flow.setError}
          documents={flow.documents}
          setDocuments={flow.setDocuments}
          heading={flow.heading}
          submit={flow.submit}
        />
      );
    case "profile":
      return (
        <ProfileStep
          error={flow.error}
          profile={flow.profile}
          setProfile={flow.setProfile}
          heading={flow.heading}
          submit={flow.submit}
        />
      );
    case "face":
      return (
        <FaceStep
          remaining={flow.remaining}
          heading={flow.heading}
          verifyVideo={flow.verifyVideo}
        />
      );
    case "processing":
      return <ProcessingStep heading={flow.heading} progress={flow.progress} />;
    case "password":
      return (
        <PasswordStep
          error={flow.error}
          busy={flow.busy}
          progress={flow.progress}
          createdAccount={flow.createdAccount}
          password={flow.password}
          setPassword={flow.setPassword}
          confirm={flow.confirm}
          setConfirm={flow.setConfirm}
          heading={flow.heading}
          submit={flow.submit}
        />
      );
    case "success":
      return (
        <SuccessStep
          email={flow.email}
          profile={flow.profile}
          heading={flow.heading}
          onLogin={onLogin}
        />
      );
    case "error":
      return (
        <ErrorStep
          error={flow.error}
          remaining={flow.remaining}
          go={flow.go}
          heading={flow.heading}
          retry={flow.retry}
          onLogin={onLogin}
        />
      );
  }
}
