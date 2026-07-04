import { Link, NavLink, Outlet } from "react-router-dom";
import { useRef, useState, type ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import { useAuth } from "../auth/AuthContext";
import { useDismiss } from "../hooks/useDismiss";
import { api } from "../api";
import { useProjectScope } from "../hooks/useProjectScope";
import { REJECTED_EVENT_COUNT_KEY } from "../pages/DataQuality";
import LogoMark from "./LogoMark";
import ProjectControls from "./ProjectControls";

const headerNavClass = ({ isActive }: { isActive: boolean }) =>
  `border-b-2 px-1 py-1 text-sm font-medium transition-colors ${
    isActive
      ? "border-primary text-primary"
      : "border-transparent text-on-surface-variant hover:text-on-surface"
  }`;

const menuNavClass = ({ isActive }: { isActive: boolean }) =>
  `block rounded-md px-3 py-2 text-sm font-medium transition-colors ${
    isActive
      ? "bg-surface-container text-primary"
      : "text-on-surface-variant hover:bg-surface-container hover:text-on-surface"
  }`;

// Recent-activity badge: the count of rejection groups seen in the last 24h that
// are newer than the last acknowledgement, so it flags live problems, not history.
function RejectedBadge({ count }: { count: number }) {
  if (count <= 0) return null;
  return (
    <span className="ml-1.5 inline-flex min-w-4 items-center justify-center rounded-pill bg-status-warning-muted px-1.5 py-0.5 text-xs font-semibold text-status-warning tabular-nums">
      {count}
    </span>
  );
}

export default function Layout() {
  const { user, logout } = useAuth();
  const [profileOpen, setProfileOpen] = useState(false);
  const profileRef = useRef<HTMLDivElement>(null);
  useDismiss(profileRef, () => setProfileOpen(false), profileOpen);
  const initials = (user?.name || user?.email || "?").slice(0, 1).toUpperCase();

  const { projectScope } = useProjectScope();
  const rejectedCount = useQuery({
    queryKey: [REJECTED_EVENT_COUNT_KEY, projectScope],
    queryFn: () => api.rejectedEventCount(projectScope as string),
    enabled: Boolean(projectScope),
    refetchInterval: 60_000,
  });
  const rejectedBadge: ReactNode = <RejectedBadge count={rejectedCount.data?.active_group_count ?? 0} />;

  return (
    <div className="min-h-screen bg-surface-dim text-on-surface">
      <header className="bg-surface-container-low">
        <div className="mx-auto flex max-w-7xl flex-wrap items-center gap-x-6 gap-y-3 px-4 py-3">
          <Link
            to="/"
            className="flex shrink-0 items-center gap-2 transition-opacity hover:opacity-80"
          >
            <LogoMark size={36} className="text-primary" />
            <span className="text-xl font-bold text-on-surface">
              flightrecorder
            </span>
          </Link>

          <nav aria-label="Primary" className="hidden items-center gap-4 sm:flex">
            <NavLink to="/" end className={headerNavClass}>
              Dashboard
            </NavLink>
            <NavLink to="/data-quality" className={headerNavClass}>
              Data Quality
              {rejectedBadge}
            </NavLink>
            <NavLink to="/users" className={headerNavClass}>
              Users
            </NavLink>
            <NavLink to="/agents" className={headerNavClass}>
              Agents
            </NavLink>
          </nav>

          <ProjectControls className="order-last w-full md:order-none md:ml-auto md:w-auto" />

          <div className="relative ml-auto md:ml-0" ref={profileRef}>
            <button
              type="button"
              onClick={() => setProfileOpen((open) => !open)}
              aria-label="Account"
              aria-haspopup="menu"
              aria-expanded={profileOpen}
              className="flex h-10 w-10 cursor-pointer items-center justify-center overflow-hidden rounded-full border border-outline-ghost bg-surface-container text-sm font-semibold text-on-surface transition-colors hover:border-primary"
            >
              {user?.picture ? (
                <img src={user.picture} alt="" className="h-full w-full object-cover" referrerPolicy="no-referrer" />
              ) : (
                initials
              )}
            </button>

            {profileOpen ? (
              <div className="glass-panel biolume-glow absolute right-0 top-12 z-40 w-72 rounded-md border border-outline-ghost p-3">
                <div className="mb-3 min-w-0">
                  <p className="truncate text-sm font-semibold text-on-surface">{user?.name || user?.email}</p>
                  <p className="truncate text-xs text-on-surface-variant">{user?.email}</p>
                </div>
                <nav aria-label="Account" className="mb-3 border-y border-outline-ghost py-2">
                  <NavLink to="/" end onClick={() => setProfileOpen(false)} className={menuNavClass}>
                    Dashboard
                  </NavLink>
                  <NavLink to="/data-quality" onClick={() => setProfileOpen(false)} className={menuNavClass}>
                    Data Quality
                    {rejectedBadge}
                  </NavLink>
                  <NavLink to="/users" onClick={() => setProfileOpen(false)} className={menuNavClass}>
                    Users
                  </NavLink>
                  <NavLink to="/agents" onClick={() => setProfileOpen(false)} className={menuNavClass}>
                    Agents
                  </NavLink>
                </nav>
                <button type="button" onClick={logout} className="w-full btn-secondary text-left">
                  Logout
                </button>
              </div>
            ) : null}
          </div>
        </div>
      </header>

      <main className="mx-auto max-w-7xl px-4 py-6">
        <Outlet />
      </main>
    </div>
  );
}
