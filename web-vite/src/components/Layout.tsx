import { Link, Outlet } from "react-router-dom";
import { useAuth } from "../auth/AuthContext";
import LogoMark from "./LogoMark";
import ProjectControls from "./ProjectControls";

export default function Layout() {
  const { user, logout } = useAuth();

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

          <div className="ml-auto flex items-end gap-3 text-sm text-on-surface-variant">
            <span>{user?.email}</span>
            <button
              type="button"
              onClick={logout}
              className="rounded-md border border-outline-ghost px-3 py-1 text-on-surface"
            >
              Logout
            </button>
          </div>
        </div>
      </header>

      <main className="mx-auto max-w-7xl px-4 py-6 pb-24 md:pb-6">
        <Outlet />
      </main>
    </div>
  );
}
