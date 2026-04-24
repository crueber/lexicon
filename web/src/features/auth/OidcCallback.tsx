import { type Component, onMount } from "solid-js";
import { useNavigate, useSearchParams } from "@solidjs/router";
import { Loader2 } from "lucide-solid";
import { t } from "../../shared/i18n/i18n";

// OIDC callback page.
// The backend handles the actual callback at /api/auth/oidc/callback and
// redirects here (or to /login) with tokens. This component acts as a
// fallback if the OIDC redirect URL is configured to point to the frontend.
// It simply forwards the query params to the backend callback endpoint.
const OidcCallback: Component = () => {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();

  onMount(() => {
    const code = Array.isArray(searchParams.code) ? searchParams.code[0] : searchParams.code;
    const state = Array.isArray(searchParams.state) ? searchParams.state[0] : searchParams.state;
    if (code && state) {
      // Forward to backend callback endpoint.
      window.location.href = `/api/auth/oidc/callback?code=${encodeURIComponent(code)}&state=${encodeURIComponent(state)}`;
    } else {
      // Missing params — redirect to login.
      navigate("/login", { replace: true });
    }
  });

  return (
    <div class="flex min-h-screen items-center justify-center bg-slate-900">
      <div class="flex flex-col items-center gap-4">
        <Loader2 class="h-10 w-10 animate-spin text-indigo-400" />
        <p class="text-sm text-slate-400">{t("auth.processingLogin")}</p>
      </div>
    </div>
  );
};

export default OidcCallback;
