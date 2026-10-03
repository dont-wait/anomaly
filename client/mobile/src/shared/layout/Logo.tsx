import logoImg from "@/assets/logo.png";

interface LogoProps {
  className?: string;
}

/** Logo AnomalyBank — dùng file ảnh thật trong src/assets. */
export const Logo = ({ className = "" }: LogoProps) => {
  return (
    <img
      src={logoImg}
      alt="AnomalyBank"
      className={`h-16 w-auto object-contain ${className}`}
    />
  );
};
