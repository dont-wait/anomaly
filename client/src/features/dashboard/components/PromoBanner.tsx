import { ChevronRightIcon, SavingsIcon } from "@/shared/icons";
import type { PromoBanner as PromoBannerType } from "@/features/dashboard/model/types";

interface PromoBannerProps {
  promo: PromoBannerType;
  onClick?: () => void;
}

/** Banner ưu đãi — hiện chỉ 1 banner, dễ đổi thành carousel sau này. */
export const PromoBanner = ({ promo, onClick }: PromoBannerProps) => {
  const Container = onClick ? "button" : "div";

  return (
    <Container
      onClick={onClick}
      type={onClick ? "button" : undefined}
      className="relative flex w-full items-center gap-3 overflow-hidden rounded-2xl bg-gradient-to-r from-[#8e5d8e] to-[#b582b5] p-4 text-left text-white shadow-md shadow-[#b582b5]/20"
    >
      {/* Hoạ tiết mờ trang trí */}
      <div className="pointer-events-none absolute -top-6 -right-6 h-24 w-24 rounded-full bg-white/10 blur-xl" />

      <span className="relative flex h-11 w-11 flex-shrink-0 items-center justify-center rounded-full bg-white/20">
        <SavingsIcon className="h-5 w-5" />
      </span>

      <div className="relative min-w-0">
        <p className="font-bold">{promo.title}</p>
        <p className="mt-0.5 truncate text-xs text-white/85">
          {promo.subtitle}
        </p>
        {onClick && (
          <span className="mt-2 inline-flex items-center gap-1 text-xs font-semibold">
            {promo.ctaLabel}
            <ChevronRightIcon className="h-3.5 w-3.5" />
          </span>
        )}
      </div>
    </Container>
  );
};
