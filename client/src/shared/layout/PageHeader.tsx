import type { ReactNode } from "react";
import { ArrowLeftIcon } from "@/shared/icons";

interface PageHeaderProps {
  title: string;
  onBack?: () => void;
  backLabel?: string;
  right?: ReactNode;
}

export const PageHeader = ({
  title,
  onBack,
  backLabel = "Quay lại",
  right,
}: PageHeaderProps) => {
  return (
    <header className="flex items-center gap-2 border-b border-outline-variant/60 bg-surface-container-lowest px-2 py-2">
      {onBack ? (
        <button
          type="button"
          onClick={onBack}
          aria-label={backLabel}
          className="flex h-11 w-11 items-center justify-center rounded-full text-secondary-strong transition-colors hover:bg-secondary/10 focus-visible:ring-2 focus-visible:ring-secondary/50 focus-visible:outline-none"
        >
          <ArrowLeftIcon className="h-5 w-5" />
        </button>
      ) : (
        <span className="w-11" />
      )}
      <h1 className="flex-1 truncate text-center text-base font-semibold text-on-surface">
        {title}
      </h1>
      <div className="flex w-11 justify-end">{right}</div>
    </header>
  );
};
