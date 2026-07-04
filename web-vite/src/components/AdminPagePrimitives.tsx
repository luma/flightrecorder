import type { ReactNode } from "react";

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

export function Table({ headers, rows }: { headers: string[]; rows: ReactNode[][] }) {
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
          {rows.length === 0 ? (
            <tr>
              <td className="px-3 py-4 text-on-surface-variant" colSpan={headers.length}>No rows</td>
            </tr>
          ) : (
            rows.map((row, index) => (
              <tr key={index}>
                {row.map((cell, cellIndex) => (
                  <td key={cellIndex} className="max-w-xs truncate px-3 py-2 text-on-surface-variant">{cell}</td>
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
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  placeholder?: string;
}) {
  return (
    <label className="block text-sm text-on-surface-variant">
      {label}
      <input
        value={value}
        placeholder={placeholder}
        onChange={(event) => onChange(event.target.value)}
        className="mt-1 w-full rounded-md border border-outline-ghost bg-surface-container px-2 py-1 text-on-surface outline-none focus:border-primary"
      />
    </label>
  );
}

export async function copyText(value: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(value);
    return true;
  } catch {
    return false;
  }
}
