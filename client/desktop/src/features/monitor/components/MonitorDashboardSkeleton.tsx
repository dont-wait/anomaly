import { Skeleton } from "@/components/ui/Skeleton";

const surface = "bg-(--admin-surface)";
const surfaceSubtle = "bg-(--admin-surface-subtle)";
const mono = "font-(family-name:--admin-font-mono)";

function MetricSkeleton() {
  return (
    <div className={`${surfaceSubtle} flex min-h-23 flex-col justify-between rounded-(--admin-radius-md) p-3`}>
      <div className="flex items-center justify-between gap-2">
        <Skeleton className="h-3 w-20" />
        <Skeleton className="h-3.5 w-3.5 rounded-full" />
      </div>
      <div className="mt-2 flex items-baseline justify-between gap-2">
        <Skeleton className="h-6 w-24" />
        <Skeleton className="h-3 w-9" />
      </div>
      <Skeleton className="mt-1 h-3 w-28" />
    </div>
  );
}

function ChartSkeleton() {
  return (
    <section className={`${surfaceSubtle} flex min-h-75 flex-col rounded-(--admin-radius-md) p-3`}>
      <div className="flex items-center justify-between gap-2 pb-2">
        <div className="flex items-center gap-2">
          <Skeleton className="h-5 w-44" />
          <Skeleton className="h-3 w-28" />
        </div>
        <Skeleton className="h-3 w-28" />
      </div>
      <div className="relative my-2 h-50 overflow-hidden rounded-sm bg-(--admin-surface)">
        <svg className="absolute inset-0 h-full w-full animate-pulse" viewBox="0 0 600 200" preserveAspectRatio="none" aria-hidden="true">
          <g stroke="var(--admin-border)" strokeWidth="1" opacity="0.7">
            <line x1="24" y1="32" x2="576" y2="32" />
            <line x1="24" y1="100" x2="576" y2="100" />
            <line x1="24" y1="168" x2="576" y2="168" />
            <line x1="116" y1="20" x2="116" y2="180" />
            <line x1="208" y1="20" x2="208" y2="180" />
            <line x1="300" y1="20" x2="300" y2="180" />
            <line x1="392" y1="20" x2="392" y2="180" />
            <line x1="484" y1="20" x2="484" y2="180" />
          </g>
          <path
            d="M24 146 C72 138 86 126 116 132 S170 152 208 108 S260 72 300 96 S348 124 392 82 S450 54 484 74 S536 98 576 48 L576 180 L24 180 Z"
            fill="var(--admin-primary)"
            opacity="0.08"
          />
          <path
            d="M24 146 C72 138 86 126 116 132 S170 152 208 108 S260 72 300 96 S348 124 392 82 S450 54 484 74 S536 98 576 48"
            fill="none"
            stroke="var(--admin-primary)"
            strokeWidth="2"
            opacity="0.45"
            vectorEffect="non-scaling-stroke"
          />
          <circle cx="392" cy="82" r="4" fill="var(--admin-primary)" opacity="0.5" />
        </svg>
      </div>
      <div className={`${mono} flex items-center justify-between gap-2 pt-2`}>
        {Array.from({ length: 7 }, (_, index) => <Skeleton key={index} className="h-3 w-10" />)}
      </div>
    </section>
  );
}

function BreakdownSkeleton() {
  return (
    <section className={`${surfaceSubtle} flex min-h-75 flex-col justify-between gap-6 rounded-(--admin-radius-md) p-3`}>
      <div>
        <div className="flex items-center justify-between gap-2 pb-2">
          <Skeleton className="h-5 w-48" />
          <Skeleton className="h-3 w-16" />
        </div>
        <Skeleton className="my-2 h-3 w-full" />
        <div className="grid grid-cols-2 gap-1">
          {Array.from({ length: 4 }, (_, index) => (
            <div key={index} className={`${surface} flex items-center justify-between rounded-sm px-2 py-1`}>
              <Skeleton className="h-3 w-16" />
              <Skeleton className="h-3 w-5" />
            </div>
          ))}
        </div>
      </div>
      <div>
        <Skeleton className="mb-2 h-3 w-36" />
        <div className="grid gap-2">
          {Array.from({ length: 4 }, (_, index) => (
            <div key={index}>
              <div className="mb-0.5 flex items-center justify-between gap-2">
                <Skeleton className="h-3 w-28" />
                <Skeleton className="h-3 w-8" />
              </div>
              <Skeleton className="h-1.5 w-full" />
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}

function AccountsTableSkeleton() {
  return (
    <section className={`${surfaceSubtle} flex min-h-80 flex-col justify-between rounded-(--admin-radius-md) p-3`}>
      <div>
        <div className="flex items-center justify-between gap-2 pb-2">
          <div className="flex items-center gap-2">
            <Skeleton className="h-5 w-48" />
            <Skeleton className="h-4 w-24" />
          </div>
          <Skeleton className="h-3 w-20" />
        </div>
        <div className="overflow-hidden rounded-sm">
          <div className={`${surface} grid grid-cols-6 gap-2 p-2`}>
            {Array.from({ length: 6 }, (_, index) => <Skeleton key={index} className="h-3 w-full" />)}
          </div>
          <div className="grid gap-px">
            {Array.from({ length: 5 }, (_, row) => (
              <div key={row} className={`${surface} grid grid-cols-6 items-center gap-2 p-2`}>
                {Array.from({ length: 6 }, (_, column) => <Skeleton key={column} className={`h-3 ${column === 1 ? "w-24" : column === 4 ? "ml-auto w-20" : "w-full"}`} />)}
              </div>
            ))}
          </div>
        </div>
      </div>
      <div className={`${mono} flex items-center justify-between gap-2 pt-2`}>
        <Skeleton className="h-3 w-36" />
        <Skeleton className="h-3 w-32" />
      </div>
    </section>
  );
}

function AlertStreamSkeleton() {
  return (
    <section className={`${surfaceSubtle} flex min-h-80 flex-col justify-between rounded-(--admin-radius-md) p-3`}>
      <div>
        <div className="flex items-center justify-between gap-2 pb-2">
          <div className="flex items-center gap-1.5">
            <Skeleton className="h-3.5 w-3.5 rounded-full" />
            <Skeleton className="h-5 w-40" />
          </div>
          <Skeleton className="h-4 w-24" />
        </div>
        <div className="grid gap-1">
          {Array.from({ length: 4 }, (_, index) => (
            <div key={index} className={`${surface} flex items-start justify-between gap-2 rounded-sm p-2`}>
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-1.5">
                  <Skeleton className="h-4 w-14" />
                  <Skeleton className="h-3 w-32" />
                </div>
                <div className="mt-1 flex items-center gap-1">
                  <Skeleton className="h-3 w-16" />
                  <Skeleton className="h-3 w-28" />
                </div>
              </div>
              <Skeleton className="h-3 w-10" />
            </div>
          ))}
        </div>
      </div>
      <div className={`${mono} flex items-center justify-between gap-2 pt-2`}>
        <Skeleton className="h-3 w-44" />
        <Skeleton className="h-3 w-20" />
      </div>
    </section>
  );
}

export function MonitorDashboardSkeleton() {
  return (
    <div className="grid gap-1" aria-label="Đang tải dashboard" role="status">
      <div className="grid grid-cols-2 gap-1 lg:grid-cols-5">
        {Array.from({ length: 5 }, (_, index) => <MetricSkeleton key={index} />)}
      </div>
      <div className="grid grid-cols-12 gap-1">
        <div className="col-span-12 xl:col-span-8"><ChartSkeleton /></div>
        <div className="col-span-12 xl:col-span-4"><BreakdownSkeleton /></div>
      </div>
      <div className="grid grid-cols-12 gap-1">
        <div className="col-span-12 lg:col-span-7"><AccountsTableSkeleton /></div>
        <div className="col-span-12 lg:col-span-5"><AlertStreamSkeleton /></div>
      </div>
    </div>
  );
}
