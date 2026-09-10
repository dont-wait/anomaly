import type { RegistrationFlowState } from "../useRegistrationFlow";
import { Next, StepHeading } from "../components/StepPrimitives";
import { faIdCard } from "@fortawesome/free-solid-svg-icons";

export function ProfileStep({
  error,
  profile,
  setProfile,
  heading,
  submit,
}: Pick<
  RegistrationFlowState,
  "error" | "profile" | "setProfile" | "heading" | "submit"
>) {
  const now = new Date();
  const maxDate = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, "0")}-${String(now.getDate()).padStart(2, "0")}`;
  return (
    <>
      <StepHeading
        headingRef={heading}
        title="Xác nhận thông tin"
        icon={faIdCard}
      >
        Nhập thông tin đúng như trên CCCD của bạn.
      </StepHeading>
      <form className="form-card" onSubmit={submit}>
        <label className="field">
          Họ và tên
          <input
            required
            autoComplete="name"
            value={profile.name}
            onChange={(e) => setProfile({ ...profile, name: e.target.value })}
            placeholder="Nguyễn Văn A"
          />
        </label>
        <label className="field">
          Số CCCD
          <input
            required
            inputMode="numeric"
            pattern="[0-9]{12}"
            maxLength={12}
            value={profile.id}
            onChange={(e) =>
              setProfile({
                ...profile,
                id: e.target.value.replace(/\D/g, ""),
              })
            }
            placeholder="12 chữ số"
          />
        </label>
        <label className="field">
          Ngày sinh
          <input
            required
            type="date"
            max={maxDate}
            value={profile.dob}
            onChange={(e) => setProfile({ ...profile, dob: e.target.value })}
          />
        </label>
        <label className="field">
          Ngày cấp CCCD
          <input
            required
            type="date"
            min={profile.dob}
            max={maxDate}
            value={profile.issuedDate}
            onChange={(e) =>
              setProfile({ ...profile, issuedDate: e.target.value })
            }
          />
        </label>
        {error && (
          <p role="alert" className="error-text">
            {error}
          </p>
        )}
        <Next disabled={!profile.name.trim() || !profile.issuedDate}>
          Xác nhận thông tin đúng
        </Next>
      </form>
    </>
  );
}
