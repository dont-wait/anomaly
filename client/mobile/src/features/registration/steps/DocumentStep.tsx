import { useEffect, useRef } from "react";
import type { RegistrationFlowState } from "../useRegistrationFlow";
import { Notice, Next, StepHeading } from "../components/StepPrimitives";
import { faCamera, faIdCard } from "@fortawesome/free-solid-svg-icons";
import { FontAwesomeIcon } from "@fortawesome/react-fontawesome";

function ImagePreview({ file, label }: { file: File; label: string }) {
  const image = useRef<HTMLImageElement>(null);
  useEffect(() => {
    const next = URL.createObjectURL(file);
    if (image.current) image.current.src = next;
    return () => URL.revokeObjectURL(next);
  }, [file]);
  return <img ref={image} alt={`Ảnh ${label} đã chọn`} />;
}
export function DocumentStep({
  error,
  setError,
  documents,
  setDocuments,
  heading,
  submit,
}: Pick<
  RegistrationFlowState,
  "error" | "setError" | "documents" | "setDocuments" | "heading" | "submit"
>) {
  return (
    <>
      <StepHeading headingRef={heading} title="Chụp ảnh CCCD" icon={faIdCard}>
        Chụp mặt trước và mặt sau CCCD còn hiệu lực, rõ nét và đầy đủ 4 góc.
      </StepHeading>
      <form className="form-card" onSubmit={submit}>
        {(["front", "back"] as const).map((side) => {
          const label = side === "front" ? "mặt trước CCCD" : "mặt sau CCCD";
          return (
            <label key={side} className="upload-zone">
              <strong>
                {side === "front" ? "Mặt trước CCCD" : "Mặt sau CCCD"}
              </strong>
              {documents[side] ? (
                <ImagePreview file={documents[side]} label={label} />
              ) : (
                <FontAwesomeIcon icon={faCamera} />
              )}
              <span>
                {documents[side]
                  ? "Nhấn để chụp lại hoặc đổi ảnh"
                  : "Nhấn để chụp hoặc tải ảnh lên"}
              </span>
              <input
                aria-label={`Ảnh ${label}`}
                type="file"
                accept="image/jpeg,image/png,image/webp"
                capture="environment"
                onChange={(e) => {
                  const file = e.target.files?.[0];
                  if (!file) return;
                  if (
                    !["image/jpeg", "image/png", "image/webp"].includes(
                      file.type,
                    ) ||
                    !file.size ||
                    file.size > 10 * 1024 * 1024
                  ) {
                    setError(
                      "Chọn ảnh JPG, PNG hoặc WebP không rỗng và không quá 10 MB.",
                    );
                    return;
                  }
                  setDocuments((current) => ({ ...current, [side]: file }));
                  setError("");
                }}
              />
            </label>
          );
        })}
        <p className="field-hint">JPG, PNG, WebP · Tối đa 10 MB mỗi ảnh</p>
        <Notice>
          Đặt thẻ trên nền phẳng, đủ ánh sáng. Tránh bị lóa hoặc che khuất thông
          tin.
        </Notice>
        {error && (
          <p role="alert" className="error-text">
            {error}
          </p>
        )}
        <Next disabled={!documents.front || !documents.back} />
      </form>
    </>
  );
}
