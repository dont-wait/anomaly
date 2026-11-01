import type { RegistrationFlowState } from "../useRegistrationFlow";
import { Next, StepHeading } from "../components/StepPrimitives";
import { faIdCard } from "@fortawesome/free-solid-svg-icons";
import { PROVINCES } from "@/features/profile/provinces";
import { SelectSheet } from "@/shared/ui";

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
        Nhập đúng như trên CCCD. Sau khi mở tài khoản, chỉ số điện thoại, email
        và ảnh đại diện có thể tự đổi.
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
        <label className="field">
          Nơi cấp CCCD
          <input
            required
            maxLength={200}
            value={profile.issuedPlace}
            onChange={(e) =>
              setProfile({ ...profile, issuedPlace: e.target.value })
            }
            placeholder="Cục CS QLHC về TTXH"
          />
        </label>
        <label className="field">
          Địa chỉ thường trú
          <input
            required
            maxLength={200}
            value={profile.permanentAddress}
            onChange={(e) =>
              setProfile({ ...profile, permanentAddress: e.target.value })
            }
            placeholder="Theo thông tin trên CCCD"
          />
        </label>
        <label className="field">
          Số điện thoại
          <input
            required
            type="tel"
            inputMode="tel"
            autoComplete="tel"
            value={profile.phone}
            onChange={(e) => setProfile({ ...profile, phone: e.target.value })}
            placeholder="0912 345 678"
          />
        </label>
        <label className="field">
          Địa chỉ liên hệ
          <input
            required
            maxLength={200}
            value={profile.addressLine}
            onChange={(e) =>
              setProfile({ ...profile, addressLine: e.target.value })
            }
            placeholder="Số nhà, đường, phường/xã"
          />
        </label>
        <div className="field">
          Tỉnh/thành
          <SelectSheet
            label="Tỉnh/thành"
            searchable
            value={profile.provinceCode}
            placeholder="Chọn tỉnh/thành"
            onChange={(value) =>
              setProfile({ ...profile, provinceCode: value })
            }
            options={PROVINCES.map((item) => ({
              value: item.code,
              label: item.name,
            }))}
          />
        </div>
        {error && (
          <p role="alert" className="error-text">
            {error}
          </p>
        )}
        <Next
          disabled={
            !profile.name.trim() ||
            !profile.issuedDate ||
            !profile.issuedPlace.trim() ||
            !profile.permanentAddress.trim() ||
            !profile.phone.trim() ||
            !profile.addressLine.trim() ||
            !profile.provinceCode
          }
        >
          Xác nhận thông tin đúng
        </Next>
      </form>
    </>
  );
}
