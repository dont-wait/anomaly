import { useEffect, useState } from "react";
import { getPackage } from "./api";
import type { PackageKind } from "./model";

/** Gói và phiên bản đã áp dụng; hồ sơ cũ có thể chưa ghi nhận phiên bản. */
export function AppliedPackage({
  kind,
  code,
  version,
}: {
  kind: PackageKind;
  code: string;
  version?: number;
}) {
  const [loaded, setLoaded] = useState<{ key: string; name: string } | null>(
    null,
  );
  const key = `${kind}:${code}:${version}`;

  useEffect(() => {
    let active = true;
    getPackage(kind, code)
      .then((detail) => {
        const applied = detail.versions.find((v) => v.version === version);
        if (active) setLoaded({ key, name: applied?.name ?? detail.name });
      })
      // Gói đã xóa vẫn phải xem được hợp đồng, nên chỉ hiện mã.
      .catch(() => {
        if (active) setLoaded({ key, name: "" });
      });
    return () => {
      active = false;
    };
  }, [kind, code, version, key]);

  const name = loaded?.key === key ? loaded.name : "";
  return (
    <p className="text-[12px] text-(--admin-text)">
      Gói áp dụng: {name ? `${name} (${code})` : code} ·{" "}
      {version ? `phiên bản ${version}` : "chưa ghi nhận phiên bản"}
    </p>
  );
}
