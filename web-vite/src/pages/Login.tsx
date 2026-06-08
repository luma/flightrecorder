import { useState, type FormEvent } from "react";
import { useNavigate } from "react-router-dom";
import { api } from "../api";
import LogoMark from "../components/LogoMark";

export default function Login() {
  const [email, setEmail] = useState("admin@example.com");
  const [error, setError] = useState("");
  const navigate = useNavigate();

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    setError("");
    try {
      await api.devLogin(email);
      navigate("/", { replace: true });
      window.location.reload();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Login failed");
    }
  };

  return (
    <div className="flex min-h-screen items-center justify-center bg-surface-dim">
      <form onSubmit={submit} className="w-full max-w-sm space-y-6">
        <div className="flex flex-col items-center gap-3 text-center">
          <LogoMark size={64} className="text-primary" />
          <h1 className="text-3xl font-bold text-on-surface">flightrecorder</h1>
          <p className="text-sm text-on-surface-variant">
            Telemetry collection and review for games.
          </p>
        </div>

        <label className="block space-y-2">
          <span className="text-sm text-on-surface-variant">Admin email</span>
          <input
            value={email}
            onChange={(event) => setEmail(event.target.value)}
            className="w-full rounded-md border border-outline-ghost bg-surface-container px-3 py-2 text-on-surface outline-none focus:border-primary"
          />
        </label>

        {error ? <p className="text-sm text-status-error">{error}</p> : null}

        <button
          type="submit"
          className="w-full rounded-md bg-primary px-4 py-2 font-semibold text-on-primary"
        >
          Sign in
        </button>
      </form>
    </div>
  );
}
