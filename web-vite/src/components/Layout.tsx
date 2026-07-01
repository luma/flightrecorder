import { Link, Outlet } from "react-router-dom";
import { useState } from "react";
import { useAuth } from "../auth/AuthContext";
import LogoMark from "./LogoMark";
import ProjectControls from "./ProjectControls";

export default function Layout() {
  const { user, logout } = useAuth();
  const [profileOpen, setProfileOpen] = useState(false);
  const initials = (user?.name || user?.email || "?").slice(0, 1).toUpperCase();

  return (
    <div className="min-h-screen bg-surface-dim text-on-surface">
      <header className="hidden md:block bg-surface-container-low">
        <div className="mx-auto flex max-w-7xl items-center gap-6 px-4 py-3">
          <Link
            to="/"
            className="flex shrink-0 items-center gap-2 transition-opacity hover:opacity-80"
          >
            <LogoMark size={36} className="text-primary" />
            <span className="text-xl font-bold text-on-surface">
              flightrecorder
            </span>
          </Link>

          <ProjectControls className="flex items-start gap-3 text-sm text-on-surface-variant" />

          <div className="relative ml-auto">
            <button
              type="button"
              onClick={() => setProfileOpen((open) => !open)}
              className="flex h-10 w-10 items-center justify-center overflow-hidden rounded-full border border-outline-ghost bg-surface-container text-sm font-semibold text-on-surface"
              aria-label="Account"
            >
              {user?.picture ? (
                <img src={user.picture} alt="" className="h-full w-full object-cover" referrerPolicy="no-referrer" />
              ) : (
                initials
              )}
            </button>

            {profileOpen ? (
              <div className="absolute right-0 top-12 z-40 w-72 rounded-md border border-outline-ghost bg-surface-container-lowest p-3 shadow-xl">
                <div className="mb-3 min-w-0">
                  <p className="truncate text-sm font-semibold text-on-surface">{user?.name || user?.email}</p>
                  <p className="truncate text-xs text-on-surface-variant">{user?.email}</p>
                </div>
                <button type="button" onClick={logout} className="w-full btn-secondary text-left">
                  Logout
                </button>
              </div>
            ) : null}
          </div>
        </div>
      </header>

      <main className="mx-auto max-w-7xl px-4 py-6 pb-24 md:pb-6">
        <Outlet />
      </main>
    </div>
  );
}
