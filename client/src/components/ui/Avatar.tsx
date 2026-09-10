import { UserIcon } from "@/components/icons";

interface AvatarProps {
  name?: string;
  src?: string;
  size?: number;
  className?: string;
}

/** Avatar tròn: hiển thị ảnh nếu có `src`, không thì hiện chữ cái đầu tên. */
export const Avatar = ({
  name,
  src,
  size = 40,
  className = "",
}: AvatarProps) => {
  const initials = name?.trim()?.charAt(0)?.toUpperCase();

  if (src) {
    return (
      <img
        src={src}
        alt={name ?? "avatar"}
        width={size}
        height={size}
        className={`rounded-full object-cover ${className}`}
        style={{ width: size, height: size }}
      />
    );
  }

  return (
    <div
      className={`flex items-center justify-center rounded-full bg-brand-secondary/20 text-brand-primary font-semibold ${className}`}
      style={{ width: size, height: size, fontSize: size * 0.4 }}
    >
      {initials ?? <UserIcon width={size * 0.5} height={size * 0.5} />}
    </div>
  );
};
