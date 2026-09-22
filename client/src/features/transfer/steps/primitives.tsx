import type { ReactNode, Ref } from "react";
import { Avatar } from "@/shared/ui";
import type { Recipient } from "@/features/transfer/model";

export const StepTitle = ({
  headingRef,
  title,
  children,
}: {
  headingRef: Ref<HTMLHeadingElement>;
  title: string;
  children?: ReactNode;
}) => (
  <div>
    <h2
      ref={headingRef}
      tabIndex={-1}
      className="text-xl font-bold text-on-surface focus:outline-none"
    >
      {title}
    </h2>
    {children && (
      <p className="mt-1 text-sm text-on-surface-variant">{children}</p>
    )}
  </div>
);

export const FieldError = ({ id, message }: { id: string; message: string }) =>
  message ? (
    <p id={id} role="alert" className="mt-2 text-sm font-medium text-error">
      {message}
    </p>
  ) : null;

export const RecipientCard = ({
  recipient,
  action,
}: {
  recipient: Recipient;
  action?: ReactNode;
}) => (
  <div className="flex items-center gap-3 rounded-2xl bg-surface-container-lowest p-3">
    <Avatar name={recipient.name} size={44} />
    <div className="min-w-0 flex-1">
      <p className="truncate text-sm font-semibold text-on-surface">
        {recipient.name}
      </p>
      <p className="truncate text-xs text-on-surface-variant">
        {recipient.accountNo} · {recipient.bank}
      </p>
    </div>
    {action}
  </div>
);
