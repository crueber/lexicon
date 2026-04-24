import {
  type Component,
  createSignal,
  Show,
  createResource,
  onMount,
} from "solid-js";
import { useNavigate, useSearchParams } from "@solidjs/router";
import { BookOpen } from "lucide-solid";
import { useAuth } from "./AuthProvider";
import { ApiError, setAccessToken, setRefreshToken } from "../../shared/api/client";
import Button from "../../shared/ui/Button";
import Input from "../../shared/ui/Input";
import { t } from "../../shared/i18n/i18n";
import { api } from "../../shared/api/client";

interface OIDCProvider {
  name: string;
}

async function fetchOIDCProviders(): Promise<OIDCProvider[]> {
  return api<OIDCProvider[]>("/auth/oidc/providers");
}

const LoginPage: Component = () => {
  const auth = useAuth();
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();

  const [username, setUsername] = createSignal("");
  const [password, setPassword] = createSignal("");
  const [error, setError] = createSignal("");
  const [submitting, setSubmitting] = createSignal(false);
  const [oidcLoading, setOidcLoading] = createSignal(false);

  const [providers] = createResource(fetchOIDCProviders);

  // Handle OIDC redirect with tokens.
  onMount(() => {
    const token = Array.isArray(searchParams.token) ? searchParams.token[0] : searchParams.token;
    const refresh = Array.isArray(searchParams.refresh) ? searchParams.refresh[0] : searchParams.refresh;
    if (token && refresh) {
      setAccessToken(token);
      setRefreshToken(refresh);
      // Let AuthProvider handle the token validation on next render.
      window.location.href = "/";
    }
  });

  async function handleSubmit(e: SubmitEvent) {
    e.preventDefault();
    setError("");
    setSubmitting(true);

    try {
      await auth.login(username(), password());
      navigate("/", { replace: true });
    } catch (err) {
      if (err instanceof ApiError) {
        try {
          const body = JSON.parse(err.body) as { error?: string };
          setError(body.error ?? t("auth.loginFailed"));
        } catch {
          setError(t("auth.loginFailed"));
        }
      } else {
        setError(t("auth.connectionError"));
      }
    } finally {
      setSubmitting(false);
    }
  }

  function handleOIDCLogin(providerName: string) {
    setOidcLoading(true);
    window.location.href = `/api/auth/oidc/${encodeURIComponent(providerName)}/authorize`;
  }

  const oidcProvider = () => (providers() ?? [])[0];

  return (
    <div class="flex min-h-screen items-center justify-center bg-slate-900">
      <div class="w-full max-w-sm px-6">
        <div class="flex flex-col items-center gap-6 mb-8">
          <BookOpen class="h-14 w-14 text-indigo-400" />
          <h1 class="text-3xl font-bold tracking-tight text-slate-100">
            {t("auth.loginTitle")}
          </h1>
          <p class="text-sm text-slate-400">
            {t("auth.signInPrompt")}
          </p>
        </div>

        <form onSubmit={handleSubmit} class="flex flex-col gap-4">
          <Input
            label={t("common.username")}
            type="text"
            placeholder={t("common.enterYourUsername")}
            value={username()}
            onInput={(e) => setUsername(e.currentTarget.value)}
            autocomplete="username"
            required
          />

          <Input
            label={t("common.password")}
            type="password"
            placeholder={t("common.enterYourPassword")}
            value={password()}
            onInput={(e) => setPassword(e.currentTarget.value)}
            autocomplete="current-password"
            required
          />

          <Show when={error()}>
            <p class="rounded-lg bg-red-950/50 border border-red-500/30 px-3 py-2 text-sm text-red-400">
              {error()}
            </p>
          </Show>

          <Button
            type="submit"
            variant="primary"
            size="lg"
            loading={submitting()}
            class="mt-2 w-full"
          >
            {t("common.signIn")}
          </Button>
        </form>

        {/* OIDC login */}
        <Show when={oidcProvider()}>
          <div class="mt-6 flex flex-col gap-3">
            <div class="relative">
              <div class="absolute inset-0 flex items-center">
                <div class="w-full border-t border-slate-700" />
              </div>
              <div class="relative flex justify-center text-xs">
                <span class="bg-slate-900 px-2 text-slate-500">{t("auth.or")}</span>
              </div>
            </div>
            <Button
              variant="secondary"
              size="lg"
              class="w-full"
              loading={oidcLoading()}
              onClick={() => handleOIDCLogin(oidcProvider()!.name)}
            >
              {t("auth.loginWith")} {oidcProvider()!.name}
            </Button>
          </div>
        </Show>
      </div>
    </div>
  );
};

export default LoginPage;
