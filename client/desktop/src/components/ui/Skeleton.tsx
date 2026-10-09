import type { HTMLAttributes } from "react";

type SkeletonProps = HTMLAttributes<HTMLDivElement>;

export function Skeleton({ className = "", ...props }: SkeletonProps) {
  return (
    <div
      {...props}
      aria-hidden="true"
      className={`animate-pulse rounded-(--admin-radius-sm) bg-(--admin-surface-hover) ${className}`.trim()}
      data-slot="skeleton"
    />
  );
}
