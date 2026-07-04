import { useRef, type ReactNode } from "react";

import { useDismiss } from "../hooks/useDismiss";

export function AdminPageHeader({ title }: { title: string }) {
  return (
    <header>
      <p className="text-sm uppercase tracking-wide text-on-surface-variant">Administration</p>
      <h1 className="text-3xl font-bold text-on-surface">{title}</h1>
    </header>
  );
}

export function Panel({ children }: { children: ReactNode }) {
  return <section className="rounded-md border border-outline-ghost bg-surface-container-low p-4">{children}</section>;
}

export type BadgeTone = "success" | "error" | "warning" | "info" | "neutral";

const badgeTones: Record<BadgeTone, string> = {
  success: "bg-status-success-muted text-status-success",
  error: "bg-status-error-muted text-status-error",
  warning: "bg-status-warning-muted text-status-warning",
  info: "bg-status-info-muted text-status-info",
  neutral: "bg-surface-container-high text-on-surface-variant",
};

export function Badge({ tone, children }: { tone: BadgeTone; children: ReactNode }) {
  return (
    <span className={`inline-flex items-center rounded-pill px-2 py-0.5 text-xs font-semibold ${badgeTones[tone]}`}>
      {children}
    </span>
  );
}

export function formatDateTime(value?: string | null): string {
  if (!value) return "";
  const parsed = Date.parse(value);
  if (!Number.isFinite(parsed)) return value;
  return new Date(parsed).toLocaleString(undefined, {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

export function DateTime({ value, fallback = "—" }: { value?: string | null; fallback?: string }) {
  if (!value) return <span className="text-on-surface-muted">{fallback}</span>;
  return (
    <time dateTime={value} title={value} className="tabular-nums">
      {formatDateTime(value)}
    </time>
  );
}

export interface TableRow {
  key: string;
  className?: string;
  cells: ReactNode[];
}

export function Table({
  headers,
  rows,
  loading,
  error,
  emptyMessage = "No rows",
}: {
  headers: string[];
  rows: TableRow[];
  loading?: boolean;
  error?: string;
  emptyMessage?: string;
}) {
  // Already-loaded rows always win: a failed background refetch (error set
  // while cached data remains) must not blank a populated table.
  const message = rows.length > 0
    ? null
    : error
      ? <span className="text-status-error">{error}</span>
      : loading
        ? "Loading..."
        : emptyMessage;
  return (
    <div className="overflow-x-auto rounded-md border border-outline-ghost">
      <table className="min-w-full divide-y divide-outline-ghost text-left text-sm">
        <thead className="bg-surface-container">
          <tr>
            {headers.map((header) => (
              <th key={header} className="px-3 py-2 font-semibold text-on-surface">{header}</th>
            ))}
          </tr>
        </thead>
        <tbody className="divide-y divide-outline-ghost bg-surface-container-lowest">
          {message !== null ? (
            <tr>
              <td className="px-3 py-4 text-on-surface-variant" colSpan={headers.length}>{message}</td>
            </tr>
          ) : (
            rows.map((row) => (
              <tr key={row.key} className={row.className}>
                {row.cells.map((cell, cellIndex) => (
                  <td
                    key={cellIndex}
                    title={typeof cell === "string" ? cell : undefined}
                    className="max-w-xs truncate px-3 py-2 text-on-surface-variant"
                  >
                    {cell}
                  </td>
                ))}
              </tr>
            ))
          )}
        </tbody>
      </table>
    </div>
  );
}

export function Input({
  label,
  value,
  onChange,
  placeholder,
  type = "text",
  autoComplete,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
  type?: string;
  autoComplete?: string;
}) {
  return (
    <label className="block text-sm text-on-surface-variant">
      {label}
      <input
        type={type}
        value={value}
        placeholder={placeholder}
        autoComplete={autoComplete}
        onChange={(event) => onChange(event.target.value)}
        className="mt-1 w-full rounded-md border border-outline-ghost bg-surface-container px-2 py-1 text-on-surface outline-none focus:border-primary"
      />
    </label>
  );
}

export function Select({
  label,
  value,
  options,
  onChange,
}: {
  label: string;
  value: string;
  options: readonly string[];
  onChange: (value: string) => void;
}) {
  return (
    <label className="block text-sm text-on-surface-variant">
      {label}
      <select
        value={value}
        onChange={(event) => onChange(event.target.value)}
        className="mt-1 w-full rounded-md border border-outline-ghost bg-surface-container px-2 py-1 text-on-surface outline-none focus:border-primary"
      >
        {options.map((option) => (
          <option key={option || "any"} value={option}>{option || "any"}</option>
        ))}
      </select>
    </label>
  );
}

export function Checkbox({
  label,
  checked,
  onChange,
}: {
  label: string;
  checked: boolean;
  onChange: (checked: boolean) => void;
}) {
  return (
    <label className="flex items-center gap-2 text-sm text-on-surface-variant">
      <input
        type="checkbox"
        checked={checked}
        onChange={(event) => onChange(event.target.checked)}
        className="h-4 w-4 rounded border-outline-ghost bg-surface-container accent-primary"
      />
      <span>{label}</span>
    </label>
  );
}

export function Drawer({
  title,
  children,
  onClose,
}: {
  title: string;
  children: ReactNode;
  onClose: () => void;
}) {
  const asideRef = useRef<HTMLElement>(null);
  // Escape + scrim click via the shared overlay stack, so only the top-most
  // of stacked overlays dismisses per keypress/click.
  useDismiss(asideRef, onClose, true);

  return (
    <div className="fixed inset-0 z-50 flex justify-end bg-black/50">
      <aside
        ref={asideRef}
        role="dialog"
        aria-modal="true"
        aria-label={title}
        className="biolume-glow h-full w-full max-w-5xl overflow-auto border-l border-outline-ghost bg-surface-container-lowest p-5"
      >
        <div className="mb-4 flex items-center gap-3">
          <h2 className="text-xl font-bold text-on-surface">{title}</h2>
          <button type="button" onClick={onClose} className="ml-auto btn-secondary">Close</button>
        </div>
        {children}
      </aside>
    </div>
  );
}

export function errorMessage(error: unknown, fallback: string): string {
  return error instanceof Error ? error.message : fallback;
}

/**
 * Row-level action button with optional confirmation and a pending label.
 * Collapses the confirm → mutate → "Saving..." choreography shared by the
 * enable/disable and delete actions across the admin tables.
 */
export function ConfirmActionButton({
  label,
  pending,
  pendingLabel = "Saving...",
  confirmMessage,
  onConfirm,
}: {
  label: string;
  pending: boolean;
  pendingLabel?: string;
  confirmMessage?: string;
  onConfirm: () => void;
}) {
  return (
    <button
      type="button"
      disabled={pending}
      onClick={() => {
        if (confirmMessage && !window.confirm(confirmMessage)) return;
        onConfirm();
      }}
      className="link-button"
    >
      {pending ? pendingLabel : label}
    </button>
  );
}

/** ID of the row a mutation is currently acting on, or null when idle. */
export function pendingActionID<V>(
  mutation: { isPending: boolean; variables?: V },
  getID: (variables: V) => string,
): string | null {
  return mutation.isPending && mutation.variables !== undefined ? getID(mutation.variables) : null;
}

export async function copyText(value: string): Promise<boolean> {
  if (!value) return false;
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(value);
      return true;
    }
  } catch {
    // Fall through to the textarea fallback for local/dev browser edge cases.
  }
  return fallbackCopyText(value);
}

function fallbackCopyText(value: string): boolean {
  const textarea = document.createElement("textarea");
  textarea.value = value;
  textarea.setAttribute("readonly", "true");
  textarea.style.position = "fixed";
  textarea.style.left = "-9999px";
  textarea.style.top = "0";
  document.body.appendChild(textarea);
  textarea.focus();
  textarea.select();
  try {
    return document.execCommand("copy");
  } finally {
    document.body.removeChild(textarea);
  }
}
