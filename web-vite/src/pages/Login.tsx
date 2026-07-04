import { useState, type FormEvent } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { api } from "../api";
import LogoMark from "../components/LogoMark";

export default function Login() {
  const [email, setEmail] = useState("admin@example.com");
  const [error, setError] = useState("");
  const navigate = useNavigate();
  const location = useLocation();
  const from = (location.state as { from?: Location } | null)?.from;
  const queryReturnPath = new URLSearchParams(location.search).get("return_path");
  const returnPath = queryReturnPath || (from ? `${from.pathname}${from.search}` : "/");
  const authConfig = useQuery({
    queryKey: ["auth-config"],
    queryFn: api.authConfig,
    retry: false,
  });

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    setError("");
    try {
      await api.devLogin(email);
      navigate(returnPath, { replace: true });
      window.location.reload();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Login failed");
    }
  };

  const config = authConfig.data;

  return (
    <div className="flex min-h-screen items-center justify-center bg-surface-dim px-4">
      <div className="w-full max-w-sm space-y-6">
        <div className="flex flex-col items-center gap-3 text-center">
          <LogoMark size={64} className="text-primary" />
          <h1 className="text-3xl font-bold text-on-surface">flightrecorder</h1>
          <p className="text-sm text-on-surface-variant">Telemetry collection and review for games.</p>
        </div>

        {authConfig.isLoading ? <p className="text-center text-sm text-on-surface-variant">Loading...</p> : null}

        {config?.google_enabled ? (
          <a href={api.googleLoginURL(returnPath)} className="block w-full rounded-md bg-primary px-4 py-2 text-center font-semibold text-on-primary">
            Continue with Google
          </a>
        ) : null}

        {config?.dev_login_enabled ? (
          <form onSubmit={submit} className="space-y-4 rounded-md border border-outline-ghost bg-surface-container-low p-4">
            <label className="block space-y-2">
              <span className="text-sm text-on-surface-variant">Dev email</span>
              <input
                value={email}
                onChange={(event) => setEmail(event.target.value)}
                className="w-full rounded-md border border-outline-ghost bg-surface-container px-3 py-2 text-on-surface outline-none focus:border-primary"
              />
            </label>

            {error ? <p className="text-sm text-status-error">{error}</p> : null}

            <button type="submit" className="w-full rounded-md bg-primary px-4 py-2 font-semibold text-on-primary">
              Sign in with dev login
            </button>
          </form>
        ) : null}

        {!authConfig.isLoading && !config?.google_enabled && !config?.dev_login_enabled ? (
          <p className="rounded-md border border-status-error bg-status-error-muted p-3 text-sm text-status-error">
            No login method is configured.
          </p>
        ) : null}
      </div>
    </div>
  );
}
