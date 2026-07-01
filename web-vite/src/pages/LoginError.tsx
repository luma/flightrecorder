import { Link, useSearchParams } from "react-router-dom";
import LogoMark from "../components/LogoMark";

const messages: Record<string, string> = {
  disabled: "Your admin account is disabled.",
  domain_denied: "Your email domain is not allowed for this service.",
  invalid_state: "The login session expired. Try signing in again.",
  oauth_failed: "Google sign-in failed.",
  login_failed: "Sign-in failed.",
};

export default function LoginError() {
  const [params] = useSearchParams();
  const reason = params.get("reason") || "login_failed";

  return (
    <div className="flex min-h-screen items-center justify-center bg-surface-dim px-4">
      <div className="w-full max-w-sm space-y-6 text-center">
        <LogoMark size={64} className="mx-auto text-primary" />
        <h1 className="text-2xl font-bold text-on-surface">Unable to sign in</h1>
        <p className="text-sm text-on-surface-variant">{messages[reason] ?? messages.login_failed}</p>
        <Link to="/login" className="inline-flex btn-primary">
          Back to login
        </Link>
      </div>
    </div>
  );
}
