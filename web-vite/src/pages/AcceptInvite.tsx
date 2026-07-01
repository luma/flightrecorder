import { useState, type FormEvent } from "react";
import { useNavigate } from "react-router-dom";
import { useQueryClient } from "@tanstack/react-query";
import { api } from "../api";
import LogoMark from "../components/LogoMark";

export default function AcceptInvite() {
  const [code, setCode] = useState("");
  const [error, setError] = useState("");
  const [pending, setPending] = useState(false);
  const navigate = useNavigate();
  const queryClient = useQueryClient();

  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (!code.trim() || pending) return;
    setPending(true);
    setError("");
    try {
      const resp = await api.acceptInviteCode(code.trim());
      queryClient.setQueryData(["me"], resp.user);
      navigate("/", { replace: true });
    } catch (err) {
      setError(err instanceof Error ? err.message : "Invalid invitation code");
    } finally {
      setPending(false);
    }
  };

  return (
    <div className="flex min-h-screen items-center justify-center bg-surface-dim px-4">
      <form onSubmit={submit} className="w-full max-w-sm space-y-6">
        <div className="flex flex-col items-center gap-3 text-center">
          <LogoMark size={64} className="text-primary" />
          <h1 className="text-2xl font-bold text-on-surface">Enter invitation code</h1>
        </div>

        <label className="block space-y-2">
          <span className="text-sm text-on-surface-variant">Invitation code</span>
          <input
            value={code}
            onChange={(event) => setCode(event.target.value)}
            className="w-full rounded-md border border-outline-ghost bg-surface-container px-3 py-2 text-on-surface outline-none focus:border-primary"
          />
        </label>

        {error ? <p className="text-sm text-status-error">{error}</p> : null}

        <button type="submit" disabled={!code.trim() || pending} className="w-full btn-primary disabled:opacity-50">
          {pending ? "Checking..." : "Accept Invite"}
        </button>
      </form>
    </div>
  );
}
